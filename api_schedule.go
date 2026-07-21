package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"otapi-hub/push"
	"otapi-hub/sync"

	"github.com/gorilla/mux"
)

// SyncRunBody — параметры запуска синхронизации (страница синка). Используется
// и ручным запуском (apiSyncRun), и планировщиком (runScheduler).
type SyncRunBody struct {
	CategoryID      string  `json:"category_id"`
	ItemTitle       string  `json:"item_title"`
	MaxProducts     int     `json:"max_products"`
	MinVolume       int     `json:"min_volume"`
	MinPrice        float64 `json:"min_price"`
	MaxPrice        float64 `json:"max_price"`
	MaxPriceLimit   float64 `json:"max_price_limit"`
	MinQuality      *int    `json:"min_quality"` // nil = дефолт 60; 0 = без фильтра; >0 = порог
	VendorName      string  `json:"vendor_name"`
	BrandName       string  `json:"brand_name"`
	PropertySearch  string  `json:"property_search"`
	OrderBy         string  `json:"order_by"`
	StuffStatus     string  `json:"stuff_status"`
	SearchMethod    string  `json:"search_method"`
	MinVendorRating int     `json:"min_vendor_rating"`
	MaxVendorRating int     `json:"max_vendor_rating"`
	FirstLotMin     int     `json:"first_lot_min"`
	FirstLotMax     int     `json:"first_lot_max"`
	FeatureComplete bool    `json:"feature_complete"`
	FeatureDiscount bool    `json:"feature_discount"`
	FeatureTmall    bool    `json:"feature_tmall"`
	PricesOnly      bool    `json:"prices_only"`
	// Для запланированного ПУША: цель выгрузки.
	PushScope  string  `json:"push_scope"`  // "category" | "products" | "all_unpushed"
	ProductIDs []int64 `json:"product_ids"` // для push_scope=products
}

// launchSyncJob создаёт задачу sync_jobs и запускает синхронизацию в фоне. Возвращает job_id.
func launchSyncJob(body SyncRunBody, triggeredBy string) (int64, error) {
	if body.MaxProducts == 0 {
		body.MaxProducts = 500
	}
	if body.StuffStatus == "" {
		body.StuffStatus = "New"
	}
	jobType := "products"
	if body.PricesOnly {
		jobType = "prices"
	}
	jobID, err := store.CreateSyncJob(jobType, body.CategoryID, triggeredBy)
	if err != nil {
		return 0, err
	}
	go func() {
		store.UpdateSyncJob(jobID, "running", 0, 0, 0, 0, "")
		if body.PricesOnly {
			updated, apiReqs, syncErr := imp.SyncPricesOnly(body.CategoryID)
			status := "done"
			logText := fmt.Sprintf("Обновлено цен: %d, API: %d", updated, apiReqs)
			if syncErr != nil {
				status = "error"
				logText += "\nERROR: " + syncErr.Error()
			}
			store.UpdateSyncJob(jobID, status, updated, 0, 0, apiReqs, logText)
			return
		}
		opts := sync.SyncOptions{
			ItemTitle: body.ItemTitle, MinVolume: body.MinVolume,
			MinPrice: int(body.MinPrice), MaxPrice: int(body.MaxPrice), MaxPriceLimit: int(body.MaxPriceLimit),
			VendorName: body.VendorName, BrandName: body.BrandName, PropertySearch: body.PropertySearch,
			OrderBy: body.OrderBy, StuffStatus: body.StuffStatus, SearchMethod: body.SearchMethod,
			MinVendorRating: body.MinVendorRating, MaxVendorRating: body.MaxVendorRating,
			FirstLotMin: body.FirstLotMin, FirstLotMax: body.FirstLotMax,
			FeatureComplete: body.FeatureComplete, FeatureDiscount: body.FeatureDiscount,
			FeatureTmall: body.FeatureTmall, JobID: jobID,
		}
		// Пресет качества форсится ВСЕГДА: минимум 60, выключить нельзя.
		// Разрешено только ПОВЫСИТЬ порог (70/80…). Значения <60 (в т.ч. 0=выкл) → 60.
		opts.MinQuality = 60
		if body.MinQuality != nil && *body.MinQuality > 60 {
			opts.MinQuality = *body.MinQuality
		}
		result := imp.SyncProducts(body.CategoryID, body.MaxProducts, opts, nil)
		status := "done"
		if result.Errors > 0 && result.Processed == 0 {
			status = "error"
		}
		store.Hub.Exec(`UPDATE sync_jobs SET status=?, finished_at=?, items_processed=?, items_skipped=?, errors_count=?, api_requests_made=? WHERE id=?`,
			status, time.Now().Unix(), result.Processed, result.Skipped, result.Errors, result.APIRequests, jobID)
		if cfg.DeepSeek.APIKey != "" && result.Processed > 0 {
			go autoTranslateCategory(body.CategoryID)
		}
	}()
	return jobID, nil
}

// scheduleRequest — тело запроса «Запланировать»: параметры + время + тип задачи.
type scheduleRequest struct {
	SyncRunBody
	TaskType    string `json:"task_type"`    // "sync" | "push" (по умолчанию sync)
	ScheduledAt int64  `json:"scheduled_at"` // unix-секунды, когда запустить
	Label       string `json:"label"`        // подпись для списка (напр. название категории)
}

// apiSyncSchedule — POST /api/v1/sync/schedule: сохранить запланированную задачу (синк или пуш).
func apiSyncSchedule(w http.ResponseWriter, r *http.Request) {
	var req scheduleRequest
	if err := parseJSON(r, &req); err != nil {
		jsonErr(w, 400, "invalid json")
		return
	}
	if req.ScheduledAt <= 0 {
		jsonErr(w, 400, "scheduled_at обязателен (unix-секунды)")
		return
	}
	if req.ScheduledAt < time.Now().Unix()-60 {
		jsonErr(w, 400, "время в прошлом")
		return
	}
	taskType := req.TaskType
	if taskType != "push" {
		taskType = "sync"
	}
	if taskType == "push" {
		if req.PushScope == "" {
			req.PushScope = "category"
		}
		switch req.PushScope {
		case "category":
			if req.CategoryID == "" {
				jsonErr(w, 400, "для пуша категории нужна категория")
				return
			}
		case "products":
			if len(req.ProductIDs) == 0 {
				jsonErr(w, 400, "не выбраны товары")
				return
			}
		case "all_unpushed":
			// без параметров
		default:
			jsonErr(w, 400, "неизвестный push_scope")
			return
		}
	}
	paramsJSON, err := json.Marshal(req.SyncRunBody)
	if err != nil {
		jsonErr(w, 500, "marshal: "+err.Error())
		return
	}
	label := req.Label
	if label == "" {
		if taskType == "push" && req.PushScope == "all_unpushed" {
			label = "все незапушенные товары"
		} else if taskType == "push" && req.PushScope == "products" {
			label = fmt.Sprintf("%d выбранных товаров", len(req.ProductIDs))
		} else if req.CategoryID != "" {
			label = req.CategoryID
		} else if req.ItemTitle != "" {
			label = req.ItemTitle
		} else {
			label = "все включённые категории"
		}
	}
	id, err := store.CreateScheduledSync(taskType, req.CategoryID, label, string(paramsJSON), req.ScheduledAt)
	if err != nil {
		jsonErr(w, 500, "create: "+err.Error())
		return
	}
	jsonData(w, map[string]interface{}{"id": id, "task_type": taskType, "scheduled_at": req.ScheduledAt})
}

// apiSyncSchedules — GET /api/v1/sync/schedules: список запланированных.
func apiSyncSchedules(w http.ResponseWriter, r *http.Request) {
	list, err := store.GetScheduledSyncs()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	out := make([]map[string]interface{}, 0, len(list))
	for _, x := range list {
		var pb SyncRunBody
		json.Unmarshal([]byte(x.ParamsJSON), &pb)
		out = append(out, map[string]interface{}{
			"id":           x.ID,
			"task_type":    x.TaskType,
			"push_scope":   pb.PushScope,
			"category_id":  x.CategoryID,
			"label":        x.Label,
			"scheduled_at": x.ScheduledAt,
			"status":       x.Status,
			"job_id":       x.JobIDValue(),
			"created_at":   x.CreatedAt,
			"run_at":       x.RunAtValue(),
		})
	}
	jsonData(w, out)
}

// apiSyncScheduleDelete — DELETE /api/v1/sync/schedule/{id}: удалить план полностью.
func apiSyncScheduleDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	ok, err := store.DeleteScheduledSync(id)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if !ok {
		jsonErr(w, 400, "нельзя удалить (задача выполняется)")
		return
	}
	jsonData(w, map[string]interface{}{"deleted": id})
}

// apiSyncScheduleUpdate — PUT /api/v1/sync/schedule/{id}: изменить ВРЕМЯ плана
// (цель и параметры не меняем — только перенос на другое время).
func apiSyncScheduleUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	var req struct {
		ScheduledAt int64 `json:"scheduled_at"`
	}
	if err := parseJSON(r, &req); err != nil {
		jsonErr(w, 400, "invalid json")
		return
	}
	if req.ScheduledAt <= 0 {
		jsonErr(w, 400, "scheduled_at обязателен")
		return
	}
	if req.ScheduledAt < time.Now().Unix()-60 {
		jsonErr(w, 400, "время в прошлом")
		return
	}
	ok, err := store.UpdateScheduledSyncTime(id, req.ScheduledAt)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if !ok {
		jsonErr(w, 400, "нельзя изменить (уже запущена или не найдена)")
		return
	}
	jsonData(w, map[string]interface{}{"updated": id, "scheduled_at": req.ScheduledAt})
}

// runScheduler — фоновый планировщик. Каждые 30с проверяет запланированные
// синхронизации, которым пора, и запускает их. Стартует в main.
func runScheduler() {
	log.Printf("[scheduler] запущен (проверка каждые 30с)")
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		due, err := store.ClaimDueScheduledSyncs()
		if err != nil {
			log.Printf("[scheduler] ошибка выборки: %v", err)
			continue
		}
		for _, sc := range due {
			// ПУШ в CS-Cart (долгая ночная операция)
			if sc.TaskType == "push" {
				var body SyncRunBody
				json.Unmarshal([]byte(sc.ParamsJSON), &body)
				scope := body.PushScope
				if scope == "" {
					scope = "category" // обратная совместимость
				}
				scID := sc.ID
				catOT := sc.CategoryID
				ids := body.ProductIDs
				store.FinishScheduledSync(scID, 0, "done") // помечаем «запущена» сразу
				go func() {
					var res *push.PushResult
					switch scope {
					case "products":
						res = apiPusher.PushProductsAuto(ids)
					case "all_unpushed":
						res = apiPusher.PushAllUnpushed()
					default:
						res = apiPusher.PushCategoryAuto(catOT)
					}
					log.Printf("[scheduler] запланированный ПУШ #%d (%s): выгружено %d, ошибок %d", scID, scope, res.Pushed, res.Errors)
				}()
				log.Printf("[scheduler] запланированный ПУШ #%d стартовал (scope=%s, категория=%s, товаров=%d)", scID, scope, catOT, len(ids))
				continue
			}
			// СИНК из OT в хаб
			var body SyncRunBody
			if err := json.Unmarshal([]byte(sc.ParamsJSON), &body); err != nil {
				log.Printf("[scheduler] #%d битые params: %v", sc.ID, err)
				store.FinishScheduledSync(sc.ID, 0, "error")
				continue
			}
			jobID, err := launchSyncJob(body, "scheduled")
			if err != nil {
				log.Printf("[scheduler] #%d запуск не удался: %v", sc.ID, err)
				store.FinishScheduledSync(sc.ID, 0, "error")
				continue
			}
			store.FinishScheduledSync(sc.ID, jobID, "done")
			log.Printf("[scheduler] запланированный СИНК #%d -> job %d (категория %s)", sc.ID, jobID, sc.CategoryID)
		}
	}
}
