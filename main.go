package main

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os/exec"
	"encoding/json"
	"otapi-hub/config"
	"otapi-hub/cscart"
	"otapi-hub/db"
	"otapi-hub/otapi"
	"otapi-hub/push"
	"otapi-hub/sync"
	"otapi-hub/translate"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

//go:embed web/templates/*.html
var templateFS embed.FS

var (
	cfg       *config.Config
	store     *db.Store
	imp       *sync.Importer
	pusher    *push.Pusher
	apiPusher *push.APIPusher
	csClient  *cscart.Client
)

var funcMap template.FuncMap

func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

func main() {
	// Пробуем загрузить config.yaml, если нет - используем Default()
	loaded, loadErr := config.Load("config.yaml")
	if loadErr == nil {
		cfg = loaded
		log.Println("[config] Загружен config.yaml")
	} else {
		cfg = config.Default()
		log.Println("[config] Используется Default() конфигурация")
	}

	var err error
	store, err = db.New(cfg.Database.HubDSN, cfg.Database.MirrorDSN)
	if err != nil {
		log.Fatalf("DB init: %v", err)
	}
	defer store.Close()

	client := otapi.NewClient(cfg.OTAPI.InstanceKey, cfg.OTAPI.LegacyURL)
	imp = sync.NewImporter(store, client)
	pusher = push.New(store)

	csClient = cscart.NewClient(cfg.CSCart.BaseURL, cfg.CSCart.Email, cfg.CSCart.APIKey)
	dsClient := translate.NewDeepSeekClient(cfg.DeepSeek.APIKey, cfg.DeepSeek.BaseURL)
	apiPusher = push.NewAPIPusher(store, csClient, dsClient, cfg.CSCart.CompanyID)

	funcMap = template.FuncMap{
		"p": func(path string) string { return "/otweb" + path },
		"inc": func(i interface{}) int {
			if v, ok := toInt(i); ok {
				return v + 1
			}
			return 0
		},
		"dec": func(i interface{}) int {
			if v, ok := toInt(i); ok {
				return v - 1
			}
			return 0
		},
		"not": func(b bool) bool { return !b },
		"gt": func(a, b interface{}) bool {
			ai, aok := toInt(a)
			bi, bok := toInt(b)
			return aok && bok && ai > bi
		},
		"lt": func(a, b interface{}) bool {
			ai, aok := toInt(a)
			bi, bok := toInt(b)
			return aok && bok && ai < bi
		},
		"fmtTime": func(ts *int64) string {
			if ts == nil {
				return "-"
			}
			return time.Unix(*ts, 0).Format("02.01 15:04")
		},
		"fmtUnix": func(ts interface{}) string {
			switch v := ts.(type) {
			case int64:
				if v == 0 {
					return "-"
				}
				return time.Unix(v, 0).Format("02.01 15:04")
			case int:
				if v == 0 {
					return "-"
				}
				return time.Unix(int64(v), 0).Format("02.01 15:04")
			}
			return "-"
		},
	}

	r := mux.NewRouter()
	prefix := "/otweb"
	s := r.PathPrefix(prefix).Subrouter()
	s.HandleFunc("/", handleDashboard).Methods("GET")
	s.HandleFunc("/categories", handleCategories).Methods("GET")
	s.HandleFunc("/categories/sync-all-meta", handleSyncMeta).Methods("POST")
	s.HandleFunc("/categories/{id}/toggle", handleCategoryToggle).Methods("POST")
	s.HandleFunc("/categories/{id}/config", handleCategoryConfig).Methods("POST")
	s.HandleFunc("/products", handleProducts).Methods("GET")
	s.HandleFunc("/products/{id}", handleProductDetail).Methods("GET")
	s.HandleFunc("/sync", handleSyncPage).Methods("GET")
	s.HandleFunc("/sync/run", handleSyncRun).Methods("POST")
	s.HandleFunc("/push", handlePushPage).Methods("GET")
	s.HandleFunc("/push/add", handlePushAdd).Methods("POST")
	s.HandleFunc("/push/execute", handlePushExecute).Methods("POST")
	s.HandleFunc("/sync/prices", handleSyncPrices).Methods("POST")
	s.HandleFunc("/mapping", handleMapping).Methods("GET")
	s.HandleFunc("/mapping/add", handleMappingAdd).Methods("POST")
	s.HandleFunc("/mapping/delete", handleMappingDelete).Methods("POST")
	s.HandleFunc("/mapping/refresh-cscart", handleRefreshCSCart).Methods("POST")
	s.HandleFunc("/push/api", handleAPIPush).Methods("POST")
	s.HandleFunc("/settings", handleSettings).Methods("GET")
	s.HandleFunc("/settings/keys", handleSettingsKeys).Methods("POST")
	s.HandleFunc("/settings/product", handleSettingsProduct).Methods("POST")
	s.HandleFunc("/settings/prompt", handleSettingsPrompt).Methods("POST")
	s.HandleFunc("/settings/providers", handleSettingsProviders).Methods("POST")
	s.HandleFunc("/settings/pricing", handleSettingsPricing).Methods("POST")
	s.HandleFunc("/settings/cron", handleSettingsCron).Methods("POST")
	// Корень редиректит на /otweb/
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, prefix+"/", http.StatusMovedPermanently)
	})

	addr := ":" + cfg.Server.Port
	log.Printf("OTAPI Hub запущен на http://localhost%s%s/", addr, prefix)
	log.Fatal(http.ListenAndServe(addr, r))
}

func render(w http.ResponseWriter, pageName, title string, data interface{}) {
	bd, ok := data.(map[string]interface{})
	if !ok {
		bd = map[string]interface{}{}
	}
	bd["Title"] = title
	bd["Page"] = pageName
	bd["Prefix"] = "/otweb"

	t, err := template.New("").Funcs(funcMap).ParseFS(templateFS,
		"web/templates/layout.html",
		"web/templates/"+pageName+".html",
	)
	if err != nil {
		log.Printf("template parse error: %v", err)
		http.Error(w, "Template error: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout.html", bd); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Template error: "+err.Error(), 500)
	}
}

type D = map[string]interface{}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, _ := store.GetDashboardStats()
	jobs, _ := store.GetRecentSyncJobs(10)
	render(w, "dashboard", "Dashboard", D{
		"Stats": stats,
		"Jobs":  jobs,
	})
}

func handleCategories(w http.ResponseWriter, r *http.Request) {
	cats, _ := store.GetCategoriesWithConfig()
	render(w, "categories", "Категории", D{
		"Categories": cats,
	})
}

func handleSyncMeta(w http.ResponseWriter, r *http.Request) {
	go func() {
		if err := imp.SyncCategories(); err != nil {
			log.Printf("sync meta error: %v", err)
		}
	}()
	http.Redirect(w, r, "/otweb/categories", http.StatusSeeOther)
}

func handleCategoryToggle(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var enabled bool
	store.Hub.QueryRow(`SELECT IFNULL(enabled, 0) FROM category_config WHERE category_id=?`, id).Scan(&enabled)
	store.UpsertCategoryConfig(id, !enabled, "manual", 500, nil, "")
	http.Redirect(w, r, "/otweb/categories", http.StatusSeeOther)
}

func handleCategoryConfig(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	r.ParseForm()
	schedule := r.FormValue("schedule")
	maxP, _ := strconv.Atoi(r.FormValue("max_products"))
	if maxP == 0 {
		maxP = 500
	}
	enabled := r.FormValue("enabled") == "true"
	store.UpsertCategoryConfig(id, enabled, schedule, maxP, nil, "")
	http.Redirect(w, r, "/otweb/categories", http.StatusSeeOther)
}

func handleProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 40

	filter := db.ProductFilter{
		CategoryID:      q.Get("category"),
		Provider:        q.Get("provider"),
		TranslateStatus: q.Get("translate"),
		Search:          q.Get("search"),
		PushedOnly:      q.Get("pushed") == "1",
		UnpushedOnly:    q.Get("unpushed") == "1",
	}

	products, total, _ := store.GetProductsFiltered(filter, page, limit)
	cats, _ := store.GetCategoriesWithConfig()
	totalPages := (total + limit - 1) / limit

	render(w, "products", "Товары", D{
		"Products":         products,
		"Total":            total,
		"TotalPages":       totalPages,
		"Page":             page,
		"Filter":           filter,
		"Categories":       cats,
	})
}

func handleProductDetail(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)

	product, err := store.GetProductByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	type sku struct {
		SKUID         string
		Quantity      int
		PriceCNY      float64
		Configurators string
	}
	var skus []sku
	rows, _ := store.Hub.Query(`SELECT sku_id, quantity, price_cny, IFNULL(configurators,'') FROM product_skus WHERE product_id=? ORDER BY sku_id`, id)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var s sku
			rows.Scan(&s.SKUID, &s.Quantity, &s.PriceCNY, &s.Configurators)
			skus = append(skus, s)
		}
	}

	type attr struct {
		PropertyName   string
		Value          string
		IsConfigurator bool
	}
	var attrs []attr
	arows, _ := store.Hub.Query(`SELECT property_name, value, is_configurator FROM product_attrs WHERE product_id=? ORDER BY is_configurator DESC, property_name`, id)
	if arows != nil {
		defer arows.Close()
		for arows.Next() {
			var a attr
			arows.Scan(&a.PropertyName, &a.Value, &a.IsConfigurator)
			attrs = append(attrs, a)
		}
	}

	type img struct {
		URL string
	}
	var images []img
	irows, _ := store.Hub.Query(`SELECT url FROM product_images WHERE product_id=? ORDER BY is_main DESC, position LIMIT 8`, id)
	if irows != nil {
		defer irows.Close()
		for irows.Next() {
			var im img
			irows.Scan(&im.URL)
			images = append(images, im)
		}
	}

	// Загружаем raw_json и форматируем для отображения
	var rawJSON string
	store.Hub.QueryRow(`SELECT IFNULL(raw_json,'') FROM products WHERE id=?`, id).Scan(&rawJSON)
	if rawJSON != "" {
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, []byte(rawJSON), "", "  "); err == nil {
			rawJSON = prettyJSON.String()
		}
	}

	render(w, "product_detail", product.TitleRu, D{
		"Product": product,
		"SKUs":    skus,
		"Attrs":   attrs,
		"Images":  images,
		"RawJSON": rawJSON,
	})
}

func handleSyncPage(w http.ResponseWriter, r *http.Request) {
	jobs, _ := store.GetRecentSyncJobs(20)
	cats, _ := store.GetCategoriesWithConfig()
	render(w, "sync", "Синхронизация", D{
		"Jobs":       jobs,
		"Categories": cats,
	})
}

func handleSyncRun(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	categoryID := r.FormValue("category_id")
	maxP, _ := strconv.Atoi(r.FormValue("max_products"))
	if maxP == 0 {
		maxP = 100
	}

	jobID, err := store.CreateSyncJob("products", categoryID, "manual")
	if err != nil {
		http.Error(w, "create job: "+err.Error(), 500)
		return
	}

	go func() {
		store.UpdateSyncJob(jobID, "running", 0, 0, 0, 0, "")
		result := imp.SyncProducts(categoryID, maxP, nil)
		logText := strings.Join(result.Log, "\n")
		status := "done"
		if result.Errors > 0 && result.Processed == 0 {
			status = "error"
		}
		store.UpdateSyncJob(jobID, status, result.Processed, result.Skipped, result.Errors, result.APIRequests, logText)

		if categoryID != "" {
			store.Hub.Exec(`UPDATE category_config SET last_synced_at=?, products_imported=products_imported+?
				WHERE category_id=?`, time.Now().Unix(), result.Processed, categoryID)
		}
	}()

	http.Redirect(w, r, fmt.Sprintf("/otweb/sync?started=%d", jobID), http.StatusSeeOther)
}

func handlePushPage(w http.ResponseWriter, r *http.Request) {
	type queueItem struct {
		ID        int
		ProductID int64
		TitleRu   string
		Action    string
		Status    string
		CreatedAt int64
	}
	var items []queueItem
	rows, _ := store.Hub.Query(`
		SELECT q.id, q.product_id, IFNULL(NULLIF(p.title_ru,''), p.title_original), q.action, q.status, q.created_at
		FROM push_queue q
		JOIN products p ON p.id = q.product_id
		WHERE q.status = 'pending'
		ORDER BY q.created_at DESC LIMIT 100`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var item queueItem
			rows.Scan(&item.ID, &item.ProductID, &item.TitleRu, &item.Action, &item.Status, &item.CreatedAt)
			items = append(items, item)
		}
	}

	var pendingCount int
	store.Hub.QueryRow(`SELECT COUNT(*) FROM push_queue WHERE status='pending'`).Scan(&pendingCount)

	render(w, "push", "Push - Wabrum", D{
		"Items":        items,
		"PendingCount": pendingCount,
	})
}

func handlePushAdd(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	productIDStr := r.FormValue("product_id")
	productID, _ := strconv.ParseInt(productIDStr, 10, 64)
	if productID > 0 {
		store.Hub.Exec(`INSERT IGNORE INTO push_queue (product_id, action, status, created_at)
			VALUES (?, 'create', 'pending', ?)`, productID, time.Now().Unix())
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/push"
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

func readCronConfig() (pricesH, syncH int, lines []string, active bool) {
	out, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		return 6, 0, nil, false
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "otapi-hub") || strings.Contains(line, "sync/prices") || strings.Contains(line, "sync/run") {
			lines = append(lines, line)
			active = true
			// Парсим интервал из строки вида: 0 */6 * * * ...
			parts := strings.Fields(line)
			if len(parts) >= 2 && strings.HasPrefix(parts[1], "*/") {
				h, _ := strconv.Atoi(strings.TrimPrefix(parts[1], "*/"))
				if strings.Contains(line, "prices") {
					pricesH = h
				} else {
					syncH = h
				}
			}
		}
	}
	if pricesH == 0 {
		pricesH = 6
	}
	return
}

// Список всех провайдеров OT Commerce
type providerInfo struct {
	ID      string
	Name    string
	Enabled bool
}

var allProviders = []providerInfo{
	{"taobao", "Taobao", true},
	{"jd", "JD.com", true},
	{"poizon", "Poizon (Dewu)", true},
	{"alibaba", "Alibaba", false},
	{"aliexpress", "AliExpress", false},
	{"1688", "1688.com", false},
	{"amazon", "Amazon", false},
	{"ebay", "eBay", false},
	{"shein", "Shein", false},
	{"trendyol", "Trendyol", false},
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	markup, _ := store.GetGlobalMarkup()
	settings := store.GetAllSettings()
	pricesH, syncH, cronLines, cronActive := readCronConfig()

	// Effective rate: exchange_rate * (1 + markup/100)
	var effectiveRate float64
	if markup != nil && markup.ExchangeRate != nil {
		effectiveRate = *markup.ExchangeRate * (1 + markup.MarkupPct/100)
	} else {
		effectiveRate = 0.57 * 1.35
	}

	// Загружаем статус провайдеров из settings
	enabledProviders := settings["enabled_providers"]
	providers := make([]providerInfo, len(allProviders))
	copy(providers, allProviders)
	if enabledProviders != "" {
		for i := range providers {
			providers[i].Enabled = strings.Contains(enabledProviders, providers[i].ID)
		}
	}

	render(w, "settings", "Настройки", D{
		"Markup":        markup,
		"EffectiveRate": effectiveRate,
		"Settings":      settings,
		"DefaultPrompt": translate.DefaultPromptTemplate,
		"Providers":     providers,
		"CronPricesH":   pricesH,
		"CronSyncH":    syncH,
		"CronLines":    cronLines,
		"CronStatus":   cronActive,
	})
}

func handleSettingsKeys(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	keys := []string{"otapi_instance_key", "cscart_base_url", "cscart_email", "cscart_api_key", "deepseek_api_key"}
	for _, k := range keys {
		if v := r.FormValue(k); v != "" {
			store.SaveSetting(k, v)
		}
	}
	http.Redirect(w, r, "/otweb/settings", http.StatusSeeOther)
}

func handleSettingsProduct(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	for _, k := range []string{"default_product_status", "cscart_company_id"} {
		if v := r.FormValue(k); v != "" {
			store.SaveSetting(k, v)
		}
	}
	http.Redirect(w, r, "/otweb/settings", http.StatusSeeOther)
}

func handleSettingsPrompt(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	store.SaveSetting("deepseek_prompt", r.FormValue("deepseek_prompt"))
	http.Redirect(w, r, "/otweb/settings", http.StatusSeeOther)
}

func handleSettingsProviders(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	providers := r.Form["providers"] // multiple checkboxes
	store.SaveSetting("enabled_providers", strings.Join(providers, ","))
	http.Redirect(w, r, "/otweb/settings", http.StatusSeeOther)
}

func handleSettingsPricing(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	markupPct, _ := strconv.ParseFloat(r.FormValue("markup_pct"), 64)
	exchangeRate, _ := strconv.ParseFloat(r.FormValue("exchange_rate"), 64)
	fixedAddon, _ := strconv.ParseFloat(r.FormValue("fixed_addon"), 64)

	store.Hub.Exec(`UPDATE markup_rules SET markup_pct=?, exchange_rate=?, fixed_addon=?
		WHERE scope_type='global'`, markupPct, exchangeRate, fixedAddon)

	http.Redirect(w, r, "/otweb/settings", http.StatusSeeOther)
}

func handleSettingsCron(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	pricesH, _ := strconv.Atoi(r.FormValue("prices_every_h"))
	syncH, _ := strconv.Atoi(r.FormValue("sync_every_h"))
	if pricesH < 1 {
		pricesH = 6
	}

	// Читаем текущий crontab
	existing, _ := exec.Command("crontab", "-l").Output()

	// Убираем старые otapi-hub строки
	var newLines []string
	scanner := bufio.NewScanner(bytes.NewReader(existing))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "otapi-hub") && !strings.Contains(line, "sync/prices") && !strings.Contains(line, "sync/run-cron") {
			newLines = append(newLines, line)
		}
	}

	// Добавляем новые строки
	// Цены: каждые N часов
	newLines = append(newLines, fmt.Sprintf("0 */%d * * * curl -s -X POST http://localhost:5500/sync/prices -d 'category_id=' > /dev/null 2>&1 # otapi-hub prices", pricesH))

	// Полный синк (если задан)
	if syncH > 0 {
		newLines = append(newLines, fmt.Sprintf("0 */%d * * * curl -s -X POST http://localhost:5500/sync/run-cron > /dev/null 2>&1 # otapi-hub sync", syncH))
	}

	newCrontab := strings.Join(newLines, "\n") + "\n"
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(newCrontab)
	if err := cmd.Run(); err != nil {
		log.Printf("crontab update error: %v", err)
	}

	http.Redirect(w, r, "/otweb/settings", http.StatusSeeOther)
}

func handlePushExecute(w http.ResponseWriter, r *http.Request) {
	go func() {
		result := pusher.ExecuteQueue()
		log.Printf("[push] done: %d pushed, %d errors", result.Pushed, result.Errors)
	}()
	http.Redirect(w, r, "/otweb/push", http.StatusSeeOther)
}

func handleSyncPrices(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	categoryID := r.FormValue("category_id")

	jobID, err := store.CreateSyncJob("prices", categoryID, "manual")
	if err != nil {
		http.Error(w, "create job: "+err.Error(), 500)
		return
	}

	go func() {
		store.UpdateSyncJob(jobID, "running", 0, 0, 0, 0, "")
		updated, apiReqs, syncErr := imp.SyncPricesOnly(categoryID)
		status := "done"
		logText := fmt.Sprintf("Обновлено цен: %d, API запросов: %d", updated, apiReqs)
		if syncErr != nil {
			status = "error"
			logText += "\nERROR: " + syncErr.Error()
		}
		store.UpdateSyncJob(jobID, status, updated, 0, 0, apiReqs, logText)
	}()

	http.Redirect(w, r, fmt.Sprintf("/otweb/sync?started=%d", jobID), http.StatusSeeOther)
}

func handleMapping(w http.ResponseWriter, r *http.Request) {
	mappings, _ := store.GetCategoryMappings()
	otCats, _ := store.GetCategoriesWithConfig()
	csCats, _ := store.GetCSCartCategories()
	settings := store.GetAllSettings()
	render(w, "mapping", "Category Mapping", D{
		"Mappings":     mappings,
		"OTCategories": otCats,
		"CSCategories": csCats,
		"Settings":     settings,
	})
}

func handleMappingAdd(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	otCatID := r.FormValue("ot_category_id")
	csCatID, _ := strconv.Atoi(r.FormValue("cs_category_id"))
	csCatName := r.FormValue("cs_category_name")
	notes := r.FormValue("notes")

	if otCatID != "" && csCatID > 0 {
		store.UpsertCategoryMapping(otCatID, csCatID, csCatName, notes)
	}
	http.Redirect(w, r, "/otweb/mapping", http.StatusSeeOther)
}

func handleMappingDelete(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	otCatID := r.FormValue("ot_category_id")
	if otCatID != "" {
		store.DeleteCategoryMapping(otCatID)
	}
	http.Redirect(w, r, "/otweb/mapping", http.StatusSeeOther)
}

func handleRefreshCSCart(w http.ResponseWriter, r *http.Request) {
	// Загружаем категории из CS-Cart API и кэшируем в hub БД
	go func() {
		body, status, err := csClient.Do("GET", "categories?items_per_page=500", nil)
		if err != nil || status != 200 {
			log.Printf("[mapping] CS-Cart categories refresh error: %v (status %d)", err, status)
			return
		}
		var resp struct {
			Categories []struct {
				CategoryID json.Number `json:"category_id"`
				ParentID   json.Number `json:"parent_id"`
				Category   string      `json:"category"`
			} `json:"categories"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			log.Printf("[mapping] unmarshal categories: %v", err)
			return
		}
		var cats []db.CSCartCategory
		for _, c := range resp.Categories {
			catID, _ := strconv.Atoi(c.CategoryID.String())
			parentID, _ := strconv.Atoi(c.ParentID.String())
			cats = append(cats, db.CSCartCategory{
				CategoryID: catID,
				ParentID:   parentID,
				Name:       c.Category,
			})
		}
		store.CacheCSCartCategories(cats)
		log.Printf("[mapping] CS-Cart categories refreshed: %d", len(cats))
	}()
	http.Redirect(w, r, "/otweb/mapping", http.StatusSeeOther)
}

func handleAPIPush(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	categoryID := r.FormValue("category_id")

	go func() {
		log.Printf("[api-push] Запуск push категории %s через CS-Cart API", categoryID)
		result := apiPusher.PushCategoryAuto(categoryID)
		log.Printf("[api-push] Готово: %d pushed, %d errors", result.Pushed, result.Errors)
	}()

	http.Redirect(w, r, "/otweb/sync", http.StatusSeeOther)
}

