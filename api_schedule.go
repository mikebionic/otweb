package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

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
		opts.MinQuality = 60 // дефолт
		if body.MinQuality != nil {
			opts.MinQuality = *body.MinQuality // 0 = выкл, >0 = порог
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
	if taskType == "push" && req.CategoryID == "" {
		jsonErr(w, 400, "для пуша нужна категория")
		return
	}
	paramsJSON, err := json.Marshal(req.SyncRunBody)
	if err != nil {
		jsonErr(w, 500, "marshal: "+err.Error())
		return
	}
	label := req.Label
	if label == "" {
		if req.CategoryID != "" {
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
		out = append(out, map[string]interface{}{
			"id":           x.ID,
			"task_type":    x.TaskType,
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

// apiSyncScheduleDelete — DELETE /api/v1/sync/schedule/{id}: отменить.
func apiSyncScheduleDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	ok, err := store.CancelScheduledSync(id)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if !ok {
		jsonErr(w, 400, "нельзя отменить (уже запущена или не найдена)")
		return
	}
	jsonData(w, map[string]interface{}{"cancelled": id})
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
				catOT := sc.CategoryID
				scID := sc.ID
				store.FinishScheduledSync(scID, 0, "done") // помечаем «запущена» сразу
				go func() {
					res := apiPusher.PushCategoryAuto(catOT)
					log.Printf("[scheduler] запланированный ПУШ #%d категории %s: выгружено %d", scID, catOT, res.Pushed)
				}()
				log.Printf("[scheduler] запланированный ПУШ #%d стартовал (категория %s)", scID, catOT)
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
