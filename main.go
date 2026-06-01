package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"otapi-hub/config"
	"otapi-hub/cscart"
	"otapi-hub/db"
	"otapi-hub/otapi"
	"otapi-hub/push"
	"otapi-hub/sync"
	"otapi-hub/translate"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

//go:embed web/templates/*.html
var templateFS embed.FS

var (
	cfg          *config.Config
	store        *db.Store
	imp          *sync.Importer
	apiPusher    *push.APIPusher
	csClient     *cscart.Client
	sessionToken string
)

var funcMap template.FuncMap

// authMiddleware - проверяет cookie "otweb_session".
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Login/logout и img-proxy не требуют авторизации
		if r.URL.Path == "/otweb/login" || r.URL.Path == "/otweb/img-proxy" || r.URL.Path == "/otweb/api/delivery-date" {
			next.ServeHTTP(w, r)
			return
		}
		// Если auth не настроен - пропускаем
		if cfg.Auth.Username == "" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie("otweb_session")
		if err != nil || cookie.Value != sessionToken {
			// Для JSON API возвращаем 401, для HTML — редирект
			if strings.HasPrefix(r.URL.Path, "/otweb/api/v1/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			http.Redirect(w, r, "/otweb/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

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

	// Override cfg with keys saved in DB (takes priority over config.yaml)
	if dbSettings := store.GetAllSettings(); dbSettings != nil {
		if v := dbSettings["deepseek_api_key"]; v != "" {
			cfg.DeepSeek.APIKey = v
		}
		if v := dbSettings["otapi_instance_key"]; v != "" {
			cfg.OTAPI.InstanceKey = v
		}
		if v := dbSettings["cscart_api_key"]; v != "" {
			cfg.CSCart.APIKey = v
		}
		log.Printf("[config] DB keys loaded: deepseek=%v, otapi=%v, cscart=%v",
			cfg.DeepSeek.APIKey != "", cfg.OTAPI.InstanceKey != "", cfg.CSCart.APIKey != "")
	}

	client := otapi.NewClient(cfg.OTAPI.InstanceKey, cfg.OTAPI.LegacyURL)
	imp = sync.NewImporter(store, client)

	csClient = cscart.NewClient(cfg.CSCart.BaseURL, cfg.CSCart.Email, cfg.CSCart.APIKey)
	dsClient := newDSClient()
	apiPusher = push.NewAPIPusher(store, csClient, dsClient, cfg.CSCart.CompanyID)
	apiPusher.SetProxyBase(cfg.CSCart.BaseURL)
	// При пуше: скачиваем фото на сервер → CS-Cart получает локальный URL
	// CS-Cart сам сохранит эти фото у себя → после пуша фото доступны с wabrum.com
	apiPusher.SetImageDownloader(
		"/var/www/www-root/data/www/wabrum.com/images/otapi",
		cfg.CSCart.BaseURL+"/images/otapi",
	)

	funcMap = template.FuncMap{
		"p":       func(path string) string { return "/otweb" + path },
		"hasCSID": func(p *int) bool { return p != nil && *p > 0 },
		"deref":   func(p *int) int { if p != nil { return *p }; return 0 },
		"imgProxy": func(url string) string {
			if strings.Contains(url, "cbu01.alicdn.com") || strings.Contains(url, "cbu02.alicdn.com") || strings.Contains(url, "cbu03.alicdn.com") {
				return "/otweb/img-proxy?url=" + url
			}
			return url
		},
		"filterQuery": func(f db.ProductFilter) string {
			params := url.Values{}
			if f.CategoryID != "" { params.Set("category", f.CategoryID) }
			if f.Provider != "" { params.Set("provider", f.Provider) }
			if f.TranslateStatus != "" { params.Set("translate", f.TranslateStatus) }
			if f.Search != "" { params.Set("search", f.Search) }
			if f.SortBy != "" { params.Set("sort", f.SortBy) }
			if f.PushedOnly { params.Set("pushed", "1") }
			if f.UnpushedOnly { params.Set("unpushed", "1") }
			if f.EnabledOnly { params.Set("enabled", "1") }
			if f.DisabledOnly { params.Set("disabled", "1") }
			return params.Encode()
		},
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

	sessionToken = generateToken()

	r := mux.NewRouter()
	prefix := "/otweb"
	s := r.PathPrefix(prefix).Subrouter()
	s.Use(authMiddleware)
	s.HandleFunc("/login", handleLogin).Methods("GET", "POST")
	s.HandleFunc("/logout", handleLogout).Methods("GET")
	s.HandleFunc("/", handleDashboard).Methods("GET")
	s.HandleFunc("/categories", handleCategories).Methods("GET")
	s.HandleFunc("/categories/sync-all-meta", handleSyncMeta).Methods("POST")
	s.HandleFunc("/categories/translate", handleCategoriesTranslate).Methods("POST")
	s.HandleFunc("/attrs/translate", handleAttrsTranslate).Methods("POST")
	s.HandleFunc("/attrs", handleAttrs).Methods("GET")
	s.HandleFunc("/categories/{id}/toggle", handleCategoryToggle).Methods("POST")
	s.HandleFunc("/categories/{id}/config", handleCategoryConfig).Methods("POST")
	s.HandleFunc("/categories/{id}/products", handleCategoryProducts).Methods("GET")
	s.HandleFunc("/products/translate-locations", handleTranslateLocations).Methods("POST")
	s.HandleFunc("/products", handleProducts).Methods("GET")
	s.HandleFunc("/products/{id}", handleProductDetail).Methods("GET")
	s.HandleFunc("/products/{id}/translate", handleProductTranslate).Methods("POST")
	s.HandleFunc("/products/{id}/push", handleProductPush).Methods("POST")
	s.HandleFunc("/products/{id}/toggle-enabled", handleProductToggleEnabled).Methods("POST")
	s.HandleFunc("/products/bulk-action", handleBulkAction).Methods("POST")
	s.HandleFunc("/products/bulk-translate", handleBulkTranslate).Methods("POST")
	s.HandleFunc("/sync", handleSyncPage).Methods("GET")
	s.HandleFunc("/sync/run", handleSyncRun).Methods("POST")
	s.HandleFunc("/sync/log/{id}", handleSyncLog).Methods("GET")
	s.HandleFunc("/push", handlePushPage).Methods("GET")
	s.HandleFunc("/push/add", handlePushAdd).Methods("POST")
	s.HandleFunc("/push/execute", handlePushExecute).Methods("POST")
	s.HandleFunc("/sync/prices", handleSyncPrices).Methods("POST")
	s.HandleFunc("/mapping", handleMapping).Methods("GET")
	s.HandleFunc("/mapping/add", handleMappingAdd).Methods("POST")
	s.HandleFunc("/mapping/delete", handleMappingDelete).Methods("POST")
	s.HandleFunc("/mapping/set-weight", handleMappingSetWeight).Methods("POST")
	s.HandleFunc("/mapping/set-filters", handleMappingSetFilters).Methods("POST")
	s.HandleFunc("/mapping/refresh-cscart", handleRefreshCSCart).Methods("POST")
	s.HandleFunc("/push/api", handleAPIPush).Methods("POST")
	s.HandleFunc("/settings", handleSettings).Methods("GET")
	s.HandleFunc("/settings/keys", handleSettingsKeys).Methods("POST")
	s.HandleFunc("/settings/product", handleSettingsProduct).Methods("POST")
	s.HandleFunc("/settings/prompt", handleSettingsPrompt).Methods("POST")
	s.HandleFunc("/settings/providers", handleSettingsProviders).Methods("POST")
	s.HandleFunc("/settings/pricing", handleSettingsPricing).Methods("POST")
	s.HandleFunc("/settings/delivery", handleSettingsDelivery).Methods("POST")
	s.HandleFunc("/settings/cron", handleSettingsCron).Methods("POST")
	// Image proxy (no auth - CS-Cart needs access)
	s.HandleFunc("/img-proxy", handleImageProxy).Methods("GET")
	// Delivery date API (no auth - used by wabrum.com frontend JS)
	s.HandleFunc("/api/delivery-date", handleDeliveryDate).Methods("GET")

	// ── JSON API v1 ──────────────────────────────────────────────
	api := s.PathPrefix("/api/v1").Subrouter()
	// Auth
	api.HandleFunc("/auth/login", apiLogin).Methods("POST")
	api.HandleFunc("/auth/logout", apiLogout).Methods("POST")
	api.HandleFunc("/auth/me", apiMe).Methods("GET")
	// Dashboard
	api.HandleFunc("/dashboard", apiDashboard).Methods("GET")
	// Categories
	api.HandleFunc("/categories", apiCategories).Methods("GET")
	api.HandleFunc("/categories/sync-meta", apiSyncMeta).Methods("POST")
	api.HandleFunc("/categories/translate", apiCategoriesTranslate).Methods("POST")
	api.HandleFunc("/categories/{id}/toggle", apiCategoryToggle).Methods("POST")
	api.HandleFunc("/categories/{id}/config", apiCategoryConfig).Methods("POST")
	// Attrs
	api.HandleFunc("/attrs", apiAttrs).Methods("GET")
	api.HandleFunc("/attrs/translate", apiAttrsTranslate).Methods("POST")
	api.HandleFunc("/attrs/translate-selected", apiAttrsTranslateSelected).Methods("POST")
	api.HandleFunc("/attrs/save", apiAttrsSave).Methods("POST")
	api.HandleFunc("/settings/sync-cs-features", apiSyncCSFeatures).Methods("POST")
	// Products
	api.HandleFunc("/products", apiProducts).Methods("GET")
	api.HandleFunc("/products/bulk-translate", apiBulkTranslate).Methods("POST")
	api.HandleFunc("/products/translate-locations", apiTranslateLocations).Methods("POST")
	api.HandleFunc("/products/bulk", apiBulkAction).Methods("POST")
	api.HandleFunc("/products/{id}", apiProductDetail).Methods("GET")
	api.HandleFunc("/products/{id}/translate", apiProductTranslate).Methods("POST")
	api.HandleFunc("/products/{id}/translate-attrs", apiProductTranslateAttrs).Methods("POST")
	api.HandleFunc("/products/{id}/push", apiProductPush).Methods("POST")
	api.HandleFunc("/products/{id}/toggle", apiProductToggle).Methods("POST")
	// Sync
	api.HandleFunc("/sync", apiSyncPage).Methods("GET")
	api.HandleFunc("/sync/run", apiSyncRun).Methods("POST")
	api.HandleFunc("/sync/prices", apiSyncPrices).Methods("POST")
	api.HandleFunc("/sync/jobs/{id}", apiSyncJobStatus).Methods("GET")
	api.HandleFunc("/sync/brands", apiSyncBrands).Methods("GET")
	api.HandleFunc("/sync/properties", apiSyncProperties).Methods("GET")
	// Push
	api.HandleFunc("/push", apiPushPage).Methods("GET")
	api.HandleFunc("/push/api", apiPushCategory).Methods("POST")
	// Mapping
	api.HandleFunc("/mapping", apiMappingPage).Methods("GET")
	api.HandleFunc("/mapping/add", apiMappingAdd).Methods("POST")
	api.HandleFunc("/mapping/delete", apiMappingDelete).Methods("POST")
	api.HandleFunc("/mapping/set-weight", apiMappingSetWeight).Methods("POST")
	api.HandleFunc("/mapping/set-filters", apiMappingSetFilters).Methods("POST")
	api.HandleFunc("/mapping/refresh-cscart", apiRefreshCSCart).Methods("POST")
	// Settings
	api.HandleFunc("/settings", apiSettings).Methods("GET")
	api.HandleFunc("/settings/keys", apiSettingsKeys).Methods("POST")
	api.HandleFunc("/settings/product", apiSettingsProduct).Methods("POST")
	api.HandleFunc("/settings/pricing", apiSettingsPricing).Methods("POST")
	api.HandleFunc("/settings/providers", apiSettingsProviders).Methods("POST")
	api.HandleFunc("/settings/cron", apiSettingsCron).Methods("POST")
	api.HandleFunc("/settings/prompt", apiSettingsPrompt).Methods("POST")
	api.HandleFunc("/settings/delivery", apiSettingsDelivery).Methods("POST")

	// ── React SPA (статика из frontend/dist) ────────────────────
	s.PathPrefix("/app/").HandlerFunc(handleSPA)

	// Корень редиректит на SPA
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, prefix+"/app/", http.StatusMovedPermanently)
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

// CategoryNode - узел дерева категорий для шаблонов
type CategoryNode struct {
	db.CategoryWithConfig
	CSCategoryName string
	ItemCountM     string
	ItemCountK     string
	Children       []CategoryNode
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		t, _ := template.New("").ParseFS(templateFS, "web/templates/login.html")
		t.ExecuteTemplate(w, "login", map[string]string{})
		return
	}

	r.ParseForm()
	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == cfg.Auth.Username && password == cfg.Auth.Password {
		http.SetCookie(w, &http.Cookie{
			Name:     "otweb_session",
			Value:    sessionToken,
			Path:     "/otweb",
			HttpOnly: true,
			MaxAge:   86400 * 7, // 7 дней
		})
		http.Redirect(w, r, "/otweb/app/", http.StatusSeeOther)
		return
	}

	t, _ := template.New("").ParseFS(templateFS, "web/templates/login.html")
	t.ExecuteTemplate(w, "login", map[string]string{"Error": "Неверный логин или пароль"})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "otweb_session",
		Value:  "",
		Path:   "/otweb",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/otweb/login", http.StatusSeeOther)
}

// handleDeliveryDate возвращает JSON с датой следующей доставки.
// Логика: заказ до четверга (включительно) → следующий понедельник.
// Пятница/суббота/воскресенье → понедельник через 2 недели (следующий рейс).
// Формат: {"date":"26 maý","date_ru":"26 мая","days":3}
func handleDeliveryDate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	now := time.Now()
	weekday := now.Weekday() // 0=Sunday, 1=Monday ... 6=Saturday

	// Считаем дни до следующего понедельника
	var daysUntilMonday int
	switch weekday {
	case time.Monday:
		daysUntilMonday = 7 // уже пн — следующий понедельник через неделю
	case time.Tuesday:
		daysUntilMonday = 6
	case time.Wednesday:
		daysUntilMonday = 5
	case time.Thursday:
		daysUntilMonday = 4
	case time.Friday:
		daysUntilMonday = 10 // пт — рейс уже уходит, следующий через 10 дней
	case time.Saturday:
		daysUntilMonday = 9
	case time.Sunday:
		daysUntilMonday = 8
	}

	delivery := now.AddDate(0, 0, daysUntilMonday)

	tkMonths := []string{"", "ýan", "few", "mart", "apr", "maý", "iýun", "iýul", "awg", "sen", "okt", "noý", "dek"}
	ruMonths := []string{"", "января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}

	m := int(delivery.Month())
	dateTK := fmt.Sprintf("%d %s", delivery.Day(), tkMonths[m])
	dateRU := fmt.Sprintf("%d %s", delivery.Day(), ruMonths[m])

	resp := map[string]interface{}{
		"date":    dateTK,
		"date_ru": dateRU,
		"days":    daysUntilMonday,
		"weekday": int(weekday),
	}
	json.NewEncoder(w).Encode(resp)
}

func handleImageProxy(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		http.Error(w, "missing url", 400)
		return
	}
	// URL параметр может быть URL-кодирован, нужно декодировать
	imgURL, err := url.QueryUnescape(rawURL)
	if err != nil {
		imgURL = rawURL  // fallback
	}
	log.Printf("[proxy] Fetching: %s (raw: %s)", imgURL, rawURL)

	req, err := http.NewRequest("GET", imgURL, nil)
	if err != nil {
		log.Printf("[proxy] ERROR: bad url %s: %v", imgURL, err)
		http.Error(w, "bad url", 400)
		return
	}
	req.Header.Set("Referer", "https://detail.1688.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: 60 * time.Second}  // увеличил с 30 на 60 сек
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[proxy] ERROR: fetch failed for %s: %v", imgURL, err)
		http.Error(w, fmt.Sprintf("fetch failed: %v", err), 502)
		return
	}
	defer resp.Body.Close()

	log.Printf("[proxy] Response status: %d, content-type: %s, content-length: %d",
		resp.StatusCode, resp.Header.Get("Content-Type"), resp.ContentLength)

	if resp.StatusCode != 200 {
		// Читаем тело ошибки для логирования
		bodyErr, _ := io.ReadAll(resp.Body)
		errLen := len(bodyErr)
		if errLen > 200 {
			errLen = 200
		}
		log.Printf("[proxy] ERROR: remote returned %d for %s, body: %s", resp.StatusCode, imgURL, string(bodyErr[:errLen]))
		http.Error(w, fmt.Sprintf("remote error: %d", resp.StatusCode), resp.StatusCode)
		return
	}

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("[proxy] ERROR: copy failed for %s (copied %d bytes): %v", imgURL, n, err)
		return
	}
	log.Printf("[proxy] OK: %s (%d bytes, content-type: %s)", imgURL, n, resp.Header.Get("Content-Type"))
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, _ := store.GetDashboardStats()
	jobs, _ := store.GetRecentSyncJobs(10)
	render(w, "dashboard", "Dashboard", D{
		"Stats": stats,
		"Jobs":  jobs,
	})
}

func handleCategories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	providerFilter := q.Get("provider")
	statusFilter := q.Get("status")
	sortFilter := q.Get("sort")
	searchFilter := q.Get("search")
	if sortFilter == "" {
		sortFilter = "items_desc"
	}

	cats, _ := store.GetCategoriesWithConfig()
	mappings, _ := store.GetCategoryMappings()
	mappingMap := make(map[string]db.CategoryMapping)
	for _, m := range mappings {
		mappingMap[m.OTCategoryID] = m
	}

	// Обогащаем данными маппинга, строим карту
	allNodes := make(map[string]*CategoryNode)
	for i := range cats {
		c := &cats[i]
		node := &CategoryNode{CategoryWithConfig: *c}
		if m, ok := mappingMap[c.ID]; ok {
			node.CSCategoryName = m.CSCategoryName
		}
		node.ItemCountM = fmt.Sprintf("%.1f", float64(c.ItemCount)/1000000)
		node.ItemCountK = fmt.Sprintf("%.0f", float64(c.ItemCount)/1000)
		allNodes[c.ID] = node
	}

	// Фильтрация
	var filtered []CategoryNode
	for _, c := range cats {
		if providerFilter != "" && c.Provider != providerFilter {
			continue
		}
		if searchFilter != "" && !strings.Contains(strings.ToLower(c.Name), strings.ToLower(searchFilter)) {
			continue
		}
		if statusFilter == "enabled" && !c.Enabled {
			continue
		}
		if statusFilter == "with_products" && c.LocalCount == 0 {
			continue
		}
		if statusFilter == "mapped" {
			if _, ok := mappingMap[c.ID]; !ok {
				continue
			}
		}
		filtered = append(filtered, *allNodes[c.ID])
	}

	// Сортировка
	switch sortFilter {
	case "items_desc":
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].ItemCount > filtered[j].ItemCount })
	case "name":
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name < filtered[j].Name })
	case "synced":
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].LocalCount > filtered[j].LocalCount })
	}

	// Строим дерево из всех категорий
	var treeNodes []CategoryNode
	for _, c := range cats {
		if c.ParentID == "" {
			node := *allNodes[c.ID]
			for _, child := range cats {
				if child.ParentID == c.ID {
					node.Children = append(node.Children, *allNodes[child.ID])
				}
			}
			treeNodes = append(treeNodes, node)
		}
	}
	sort.Slice(treeNodes, func(i, j int) bool { return treeNodes[i].Name < treeNodes[j].Name })

	render(w, "categories", "Категории", D{
		"Categories":         cats,
		"FilteredCategories": filtered,
		"TreeNodes":          treeNodes,
		"ProviderFilter":     providerFilter,
		"StatusFilter":       statusFilter,
		"SortFilter":         sortFilter,
		"SearchFilter":       searchFilter,
	})
}

func handleSyncMeta(w http.ResponseWriter, r *http.Request) {
	cleanFirst := r.FormValue("clean_first") == "1"
	go func() {
		if err := imp.SyncCategories(cleanFirst); err != nil {
			log.Printf("sync meta error: %v", err)
		}
	}()
	http.Redirect(w, r, "/otweb/categories", http.StatusSeeOther)
}

func handleCategoriesTranslate(w http.ResponseWriter, r *http.Request) {
	if cfg.DeepSeek.APIKey == "" {
		http.Redirect(w, r, "/otweb/categories", http.StatusSeeOther)
		return
	}

	go func() {
		dsClient := newDSClient()

		// Берём категории где name_ru = name_zh (не переведены)
		rows, err := store.Hub.Query(`SELECT id, name_ru FROM categories WHERE name_ru = name_zh AND name_ru != '' ORDER BY id`)
		if err != nil {
			log.Printf("[cat-translate] query error: %v", err)
			return
		}
		defer rows.Close()

		type catItem struct {
			ID   string
			Name string
		}
		var items []catItem
		for rows.Next() {
			var it catItem
			rows.Scan(&it.ID, &it.Name)
			items = append(items, it)
		}
		rows.Close()

		if len(items) == 0 {
			log.Println("[cat-translate] all categories already translated")
			return
		}

		log.Printf("[cat-translate] translating %d categories", len(items))

		// Batch по 20 категорий за один запрос DeepSeek
		batchSize := 20
		for i := 0; i < len(items); i += batchSize {
			end := i + batchSize
			if end > len(items) {
				end = len(items)
			}
			batch := items[i:end]

			// Строим промпт для batch перевода
			var lines []string
			for _, it := range batch {
				lines = append(lines, fmt.Sprintf("%s: %s", it.ID, it.Name))
			}

			prompt := fmt.Sprintf(`Переведи названия категорий товаров с китайского на русский и английский. Это категории маркетплейса 1688.com.

Правила:
- Краткие, понятные названия для e-commerce (1-3 слова)
- Без иероглифов в переводе
- Формат ответа: JSON объект, ключ = ID категории, значение = {"ru": "...", "en": "..."}

Категории:
%s

Ответ: только JSON.`, strings.Join(lines, "\n"))

			result, err := dsClient.RawChat(prompt)
			if err != nil {
				log.Printf("[cat-translate] batch %d-%d error: %v", i, end, err)
				continue
			}

			// Парсим JSON ответ
			var translations map[string]struct {
				RU string `json:"ru"`
				EN string `json:"en"`
			}
			if err := json.Unmarshal([]byte(result), &translations); err != nil {
				log.Printf("[cat-translate] parse error: %v (response: %s)", err, result[:min(200, len(result))])
				continue
			}

			for catID, tr := range translations {
				if tr.RU != "" {
					store.Hub.Exec(`UPDATE categories SET name_ru=?, name_en=? WHERE id=?`, tr.RU, tr.EN, catID)
				}
			}
			log.Printf("[cat-translate] batch %d-%d: %d translated", i, end, len(translations))
			time.Sleep(500 * time.Millisecond)
		}
		log.Printf("[cat-translate] done")
	}()

	http.Redirect(w, r, "/otweb/categories", http.StatusSeeOther)
}

func handleAttrs(w http.ResponseWriter, r *http.Request) {
	// Статистика по атрибутам
	var total, translated int
	store.Hub.QueryRow(`SELECT COUNT(DISTINCT pid, vid) FROM product_attrs WHERE pid != '' AND vid != ''`).Scan(&total)
	store.Hub.QueryRow(`SELECT COUNT(*) FROM attr_translations`).Scan(&translated)

	// Последние 100 переводов
	rows, _ := store.Hub.Query(`SELECT pid, vid, property_name_zh, value_zh, property_name_ru, value_ru FROM attr_translations ORDER BY translated_at DESC LIMIT 100`)
	var recent []db.AttrTranslation
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var at db.AttrTranslation
			rows.Scan(&at.Pid, &at.Vid, &at.PropertyNameZh, &at.ValueZh, &at.PropertyNameRu, &at.ValueRu)
			recent = append(recent, at)
		}
	}

	render(w, "attrs", "Атрибуты", D{
		"Total":      total,
		"Translated": translated,
		"Pending":    total - translated,
		"Recent":     recent,
	})
}

func handleAttrsTranslate(w http.ResponseWriter, r *http.Request) {
	if cfg.DeepSeek.APIKey == "" {
		http.Redirect(w, r, "/otweb/attrs", http.StatusSeeOther)
		return
	}
	go func() {
		dsClient := newDSClient()
		const batchSize = 50
		translated := 0
		for {
			untranslated, err := store.GetUntranslatedAttrs(batchSize)
			if err != nil || len(untranslated) == 0 {
				break
			}
			pairs := make([]translate.AttrPair, len(untranslated))
			for i, a := range untranslated {
				pairs[i] = translate.AttrPair{Pid: a.Pid, Vid: a.Vid, Name: a.PropertyNameZh, Value: a.ValueZh}
			}
			results, err := dsClient.TranslateAttrs(pairs)
			if err != nil {
				log.Printf("[attrs-translate] error: %v", err)
				break
			}
			for _, res := range results {
				// Ищем оригинал
				var nameZh, valueZh string
				for _, p := range pairs {
					if p.Pid == res.Pid && p.Vid == res.Vid {
						nameZh, valueZh = p.Name, p.Value
						break
					}
				}
				store.SaveAttrTranslation(res.Pid, res.Vid, nameZh, res.NameRu, valueZh, res.ValueRu)
			}
			translated += len(results)
			log.Printf("[attrs-translate] batch done: %d translated (total so far: %d)", len(results), translated)
			time.Sleep(500 * time.Millisecond)
		}
		log.Printf("[attrs-translate] done. Total translated: %d", translated)
	}()
	http.Redirect(w, r, "/otweb/attrs", http.StatusSeeOther)
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

// handleTranslateLocations заполняет location_city_ru / location_state_ru
// для всех товаров у которых оригинал есть, но перевод пустой.
func handleTranslateLocations(w http.ResponseWriter, r *http.Request) {
	// Обновляем state
	stateRows, _ := store.Hub.Query(`SELECT DISTINCT location_state FROM products WHERE location_state != '' AND location_state_ru = ''`)
	var states []string
	if stateRows != nil {
		for stateRows.Next() {
			var s string
			stateRows.Scan(&s)
			states = append(states, s)
		}
		stateRows.Close()
	}
	stateUpdated := 0
	for _, s := range states {
		ru := sync.TranslateState(s)
		if ru != "" {
			store.Hub.Exec(`UPDATE products SET location_state_ru=? WHERE location_state=? AND location_state_ru=''`, ru, s)
			stateUpdated++
		}
	}

	// Обновляем city
	cityRows, _ := store.Hub.Query(`SELECT DISTINCT location_city FROM products WHERE location_city != '' AND location_city_ru = ''`)
	var cities []string
	if cityRows != nil {
		for cityRows.Next() {
			var s string
			cityRows.Scan(&s)
			cities = append(cities, s)
		}
		cityRows.Close()
	}
	cityUpdated := 0
	for _, s := range cities {
		ru := sync.TranslateCity(s)
		if ru != "" {
			store.Hub.Exec(`UPDATE products SET location_city_ru=? WHERE location_city=? AND location_city_ru=''`, ru, s)
			cityUpdated++
		}
	}

	log.Printf("[locations] translated %d states, %d cities", stateUpdated, cityUpdated)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"states":%d,"cities":%d}`, stateUpdated, cityUpdated)
}

func handleProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("per_page"))
	if limit != 80 && limit != 120 && limit != 200 {
		limit = 40
	}

	filter := db.ProductFilter{
		CategoryID:      q.Get("category"),
		Provider:        q.Get("provider"),
		TranslateStatus: q.Get("translate"),
		Search:          q.Get("search"),
		SortBy:          q.Get("sort"),
		LocationState:   q.Get("location"),
		HasWeight:       q.Get("has_weight") == "1",
		PushedOnly:      q.Get("pushed") == "1",
		UnpushedOnly:    q.Get("unpushed") == "1",
		EnabledOnly:     q.Get("enabled") == "1",
		DisabledOnly:    q.Get("disabled") == "1",
	}

	products, total, _ := store.GetProductsFiltered(filter, page, limit)
	cats, _ := store.GetCategoriesWithConfig()
	totalPages := (total + limit - 1) / limit

	var untranslatedCount int
	store.Hub.QueryRow(`SELECT COUNT(*) FROM products WHERE (translate_status IS NULL OR translate_status IN ('','none')) AND enabled = 1`).Scan(&untranslatedCount)

	// Уникальные провинции для фильтра
	type LocationOption struct {
		State   string
		StateRu string
	}
	var locations []LocationOption
	locRows, _ := store.Hub.Query(`SELECT DISTINCT location_state, location_state_ru FROM products WHERE location_state != '' ORDER BY COALESCE(NULLIF(location_state_ru,''), location_state)`)
	if locRows != nil {
		for locRows.Next() {
			var opt LocationOption
			locRows.Scan(&opt.State, &opt.StateRu)
			locations = append(locations, opt)
		}
		locRows.Close()
	}

	render(w, "products", "Товары", D{
		"Products":           products,
		"Total":              total,
		"TotalPages":         totalPages,
		"CurrentPage":        page,
		"PerPage":            limit,
		"Filter":             filter,
		"Categories":         cats,
		"Locations":          locations,
		"UntranslatedCount":  untranslatedCount,
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

	// Загружаем маппинг Pid:Vid -> человеческое имя из атрибутов-конфигураторов
	confMap := make(map[string]string) // "Pid:Vid" -> "Размер: XL"
	confRows, _ := store.Hub.Query(`SELECT pid, vid, property_name, value FROM product_attrs WHERE product_id=? AND is_configurator=1`, id)
	if confRows != nil {
		for confRows.Next() {
			var pid, vid, name, val string
			confRows.Scan(&pid, &vid, &name, &val)
			confMap[pid+":"+vid] = name + ": " + val
		}
		confRows.Close()
	}

	type sku struct {
		SKUID         string
		Quantity      int
		PriceCNY      float64
		Configurators string
		HumanName     string
	}
	var skus []sku
	rows, _ := store.Hub.Query(`SELECT sku_id, quantity, price_cny, IFNULL(configurators,'') FROM product_skus WHERE product_id=? ORDER BY sku_id`, id)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var s sku
			rows.Scan(&s.SKUID, &s.Quantity, &s.PriceCNY, &s.Configurators)
			// Расшифровываем Pid:Vid в человеческие имена
			var names []string
			var confs []struct{ Pid, Vid string }
			json.Unmarshal([]byte(s.Configurators), &confs)
			for _, c := range confs {
				if name, ok := confMap[c.Pid+":"+c.Vid]; ok {
					names = append(names, name)
				}
			}
			s.HumanName = strings.Join(names, " / ")
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

func handleCategoryProducts(w http.ResponseWriter, r *http.Request) {
	catID := mux.Vars(r)["id"]
	// Redirect to products page with category filter pre-set
	http.Redirect(w, r, fmt.Sprintf("/otweb/products?category=%s&sort=sales", catID), http.StatusSeeOther)
}

func handleProductPush(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)

	product, err := store.GetProductByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Находим CS-Cart категорию
	var categoryCS int
	store.Hub.QueryRow(`SELECT cs_category_id FROM category_map WHERE otapi_category_id=?`, product.CategoryID).Scan(&categoryCS)
	if categoryCS == 0 {
		categoryCS = 343 // fallback
	}

	csID, pushErr := apiPusher.PushSingleProduct(id, categoryCS)
	if pushErr != nil {
		log.Printf("[push] ERROR product %d: %v", id, pushErr)
	} else {
		log.Printf("[push] OK product %d -> cs_product_id=%d", id, csID)
	}

	http.Redirect(w, r, fmt.Sprintf("/otweb/products/%d", id), http.StatusSeeOther)
}

func handleProductToggleEnabled(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	product, err := store.GetProductByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	store.SetProductEnabled(id, !product.Enabled)
	http.Redirect(w, r, fmt.Sprintf("/otweb/products/%d", id), http.StatusSeeOther)
}

func handleBulkAction(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	action := r.FormValue("action")
	idStrs := r.Form["product_ids"]

	var ids []int64
	for _, s := range idStrs {
		id, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		http.Redirect(w, r, "/otweb/products", http.StatusSeeOther)
		return
	}

	switch action {
	case "enable":
		store.BulkSetEnabled(ids, true)
		http.Redirect(w, r, "/otweb/products", http.StatusSeeOther)
	case "disable":
		store.BulkSetEnabled(ids, false)
		http.Redirect(w, r, "/otweb/products", http.StatusSeeOther)
	case "delete":
		store.BulkDeleteProducts(ids)
		http.Redirect(w, r, "/otweb/products", http.StatusSeeOther)
	case "translate":
		go func() {
			if cfg.DeepSeek.APIKey == "" {
				return
			}
			dsClient := newDSClient()
			log.Printf("[bulk-translate] Translating %d products", len(ids))
			for i, id := range ids {
				product, err := store.GetProductByID(id)
				if err != nil {
					continue
				}
				attrRows, _ := store.Hub.Query(`SELECT property_name, value FROM product_attrs WHERE product_id=? AND is_configurator=0`, id)
				attrs := make(map[string]string)
				if attrRows != nil {
					for attrRows.Next() {
						var k, v string
						attrRows.Scan(&k, &v)
						attrs[k] = v
					}
					attrRows.Close()
				}
				result, err := dsClient.Normalize(translate.NormalizeInput{
					TitleRu:       product.TitleRu,
					TitleOriginal: product.TitleOriginal,
					Attributes:    attrs,
				})
				if err != nil {
					log.Printf("[bulk-translate] [%d/%d] ERROR %d: %v", i+1, len(ids), id, err)
					continue
				}
				saveTranslateResult(id, result)
				log.Printf("[bulk-translate] [%d/%d] OK %d: %s", i+1, len(ids), id, result.TitleRU)
				time.Sleep(500 * time.Millisecond)
			}
			log.Printf("[bulk-translate] Done")
		}()
		http.Redirect(w, r, "/otweb/products", http.StatusSeeOther)
	case "push":
		// Находим CS-Cart категорию через маппинг
		go func() {
			log.Printf("[bulk-push] Pushing %d products", len(ids))
			for _, id := range ids {
				product, err := store.GetProductByID(id)
				if err != nil || !product.Enabled {
					log.Printf("[bulk-push] Skip %d (disabled or not found)", id)
					continue
				}
				// Находим CS-Cart категорию из маппинга
				var categoryCS int
				store.Hub.QueryRow(`SELECT cs_category_id FROM category_map WHERE otapi_category_id=?`, product.CategoryID).Scan(&categoryCS)
				if categoryCS == 0 {
					categoryCS = 343 // fallback: Женщинам - Джинсы
					log.Printf("[bulk-push] No mapping for %s, using default %d", product.CategoryID, categoryCS)
				}
				csID, err := apiPusher.PushSingleProduct(id, categoryCS)
				if err != nil {
					log.Printf("[bulk-push] ERROR %d: %v", id, err)
				} else {
					log.Printf("[bulk-push] OK %d -> cs_product_id=%d", id, csID)
				}
			}
			log.Printf("[bulk-push] Done")
		}()
		http.Redirect(w, r, "/otweb/products", http.StatusSeeOther)
	default:
		http.Redirect(w, r, "/otweb/products", http.StatusSeeOther)
	}
}

func saveTranslateResult(id int64, result *translate.NormalizeOutput) {
	if result == nil || result.TitleRU == "" {
		return
	}
	store.Hub.Exec(`UPDATE products SET title_ru=?, title_en=?, title_tk=?,
		description_ru=?, description_en=?, description_tk=?,
		translate_status='deepseek' WHERE id=?`,
		result.TitleRU, result.TitleEN, result.TitleTK,
		result.DescriptionRU, result.DescriptionEN, result.DescriptionTK, id)
	if result.EstimatedWeightGrams > 0 {
		store.Hub.Exec(`UPDATE products SET weight_kg=?, weight_estimated=1 WHERE id=? AND weight_kg=0`,
			float64(result.EstimatedWeightGrams)/1000.0, id)
	}
}

func handleProductTranslate(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	r.ParseForm()

	action := r.FormValue("action") // "deepseek" or "manual"

	if action == "manual" {
		titleRu := r.FormValue("title_ru")
		titleEn := r.FormValue("title_en")
		titleTk := r.FormValue("title_tk")
		descRu := r.FormValue("desc_ru")
		descEn := r.FormValue("desc_en")
		descTk := r.FormValue("desc_tk")
		if titleRu != "" {
			store.Hub.Exec(`UPDATE products SET title_ru=? WHERE id=?`, titleRu, id)
		}
		if titleEn != "" {
			store.Hub.Exec(`UPDATE products SET title_en=? WHERE id=?`, titleEn, id)
		}
		if titleTk != "" {
			store.Hub.Exec(`UPDATE products SET title_tk=? WHERE id=?`, titleTk, id)
		}
		store.Hub.Exec(`UPDATE products SET description_ru=?, description_en=?, description_tk=? WHERE id=?`,
			descRu, descEn, descTk, id)
		store.Hub.Exec(`UPDATE products SET translate_status='manual' WHERE id=?`, id)
	} else {
		// DeepSeek перевод на 3 языка одним запросом
		product, err := store.GetProductByID(id)
		if err == nil && cfg.DeepSeek.APIKey != "" {
			dsClient := newDSClient()

			rows, _ := store.Hub.Query(`SELECT property_name, value FROM product_attrs WHERE product_id=? AND is_configurator=0`, id)
			attrs := make(map[string]string)
			if rows != nil {
				for rows.Next() {
					var k, v string
					rows.Scan(&k, &v)
					attrs[k] = v
				}
				rows.Close()
			}

			result, err := dsClient.Normalize(translate.NormalizeInput{
				TitleRu:       product.TitleRu,
				TitleOriginal: product.TitleOriginal,
				Attributes:    attrs,
			})
			if err == nil {
				saveTranslateResult(id, result)
			} else {
				log.Printf("[translate] DeepSeek error for product %d: %v", id, err)
			}
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/otweb/products/%d", id), http.StatusSeeOther)
}

func handleBulkTranslate(w http.ResponseWriter, r *http.Request) {
	if cfg.DeepSeek.APIKey == "" {
		http.Redirect(w, r, "/otweb/products", http.StatusSeeOther)
		return
	}

	go func() {
		dsClient := newDSClient()

		rows, err := store.Hub.Query(`
			SELECT id, title_ru, title_original FROM products
			WHERE (translate_status IS NULL OR translate_status IN ('','none')) AND enabled = 1
			ORDER BY id ASC LIMIT 100`)
		if err != nil {
			log.Printf("[bulk-translate] query error: %v", err)
			return
		}
		defer rows.Close()

		type item struct {
			ID        int64
			TitleRu   string
			TitleOrig string
		}
		var items []item
		for rows.Next() {
			var it item
			rows.Scan(&it.ID, &it.TitleRu, &it.TitleOrig)
			items = append(items, it)
		}
		rows.Close()

		log.Printf("[bulk-translate] starting: %d products", len(items))

		for i, it := range items {
			attrRows, _ := store.Hub.Query(`SELECT property_name, value FROM product_attrs WHERE product_id=? AND is_configurator=0`, it.ID)
			attrs := make(map[string]string)
			if attrRows != nil {
				for attrRows.Next() {
					var k, v string
					attrRows.Scan(&k, &v)
					attrs[k] = v
				}
				attrRows.Close()
			}

			result, err := dsClient.Normalize(translate.NormalizeInput{
				TitleRu:       it.TitleRu,
				TitleOriginal: it.TitleOrig,
				Attributes:    attrs,
			})
			if err != nil {
				log.Printf("[bulk-translate] [%d/%d] ERROR product %d: %v", i+1, len(items), it.ID, err)
				continue
			}
			saveTranslateResult(it.ID, result)
			log.Printf("[bulk-translate] [%d/%d] OK product %d: %s", i+1, len(items), it.ID, result.TitleRU)
			time.Sleep(500 * time.Millisecond)
		}
		log.Printf("[bulk-translate] done: %d products", len(items))
	}()

	http.Redirect(w, r, "/otweb/products", http.StatusSeeOther)
}

func handleSyncPage(w http.ResponseWriter, r *http.Request) {
	jobs, _ := store.GetRecentSyncJobs(20)
	cats, _ := store.GetCategoriesWithConfig()

	// Обогащаем категории item_count для отображения
	type catInfo struct {
		db.CategoryWithConfig
		ItemCountM string
		ItemCountK string
	}
	var enriched []catInfo
	for _, c := range cats {
		// Показываем только переведённые категории (без китайских иероглифов)
		if sync.HasChinese(c.Name) {
			continue
		}
		ci := catInfo{CategoryWithConfig: c}
		ci.ItemCountM = fmt.Sprintf("%.1f", float64(c.ItemCount)/1000000)
		ci.ItemCountK = fmt.Sprintf("%.0f", float64(c.ItemCount)/1000)
		enriched = append(enriched, ci)
	}

	selectedCat := r.URL.Query().Get("category")

	render(w, "sync", "Синхронизация", D{
		"Jobs":             jobs,
		"Categories":      enriched,
		"SelectedCategory": selectedCat,
	})
}

func handleSyncLog(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)

	var status, logText string
	store.Hub.QueryRow(`SELECT status, IFNULL(log_text,'') FROM sync_jobs WHERE id=?`, id).Scan(&status, &logText)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": status,
		"log":    logText,
	})
}

func handleSyncRun(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	categoryID := r.FormValue("category_id")
	itemTitle := r.FormValue("item_title")
	// Разрешаем синк по названию без категории (для 1688 где категорий нет)
	if categoryID == "" && itemTitle == "" {
		http.Redirect(w, r, "/otweb/sync", http.StatusSeeOther)
		return
	}
	if categoryID == "" {
		categoryID = "search" // виртуальная категория для поиска по названию
	}

	maxP, _ := strconv.Atoi(r.FormValue("max_products"))
	if maxP == 0 {
		maxP = 30
	}

	// Читаем ВСЕ фильтры из формы
	minVolume, _ := strconv.Atoi(r.FormValue("min_volume"))
	minPrice, _ := strconv.Atoi(r.FormValue("min_price"))
	maxPrice, _ := strconv.Atoi(r.FormValue("max_price"))
	maxPriceLimit, _ := strconv.Atoi(r.FormValue("max_price_limit"))

	// Если форма не переопределила — берём per-category лимиты из category_map
	catMinPrice, catMaxPrice, catMinVolume := store.GetCategoryFilters(categoryID)
	if minVolume == 0 && catMinVolume > 0 {
		minVolume = catMinVolume
	}
	if minPrice == 0 && catMinPrice > 0 {
		minPrice = catMinPrice
	}
	if maxPrice == 0 && catMaxPrice > 0 {
		maxPrice = catMaxPrice
	}
	// MaxPriceLimit: per-category переопределяет глобальный дефолт
	if maxPriceLimit == 0 {
		if catMaxPrice > 0 {
			maxPriceLimit = catMaxPrice // используем per-category как outlier-лимит
		} else {
			maxPriceLimit = 1000 // глобальный дефолт: 1000 CNY
		}
	}
	if itemTitle == "" {
		itemTitle = r.FormValue("item_title")
	}
	vendorName := r.FormValue("vendor_name")
	brandName := r.FormValue("brand_name")
	orderBy := r.FormValue("order_by")
	stuffStatus := r.FormValue("stuff_status")
	if stuffStatus == "" {
		stuffStatus = "New" // всегда только новые товары по умолчанию
	}
	minVendorRating, _ := strconv.Atoi(r.FormValue("min_vendor_rating"))
	maxVendorRating, _ := strconv.Atoi(r.FormValue("max_vendor_rating"))
	firstLotMin, _ := strconv.Atoi(r.FormValue("first_lot_min"))
	firstLotMax, _ := strconv.Atoi(r.FormValue("first_lot_max"))
	searchMethod := r.FormValue("search_method")
	featureComplete := r.FormValue("feature_complete") == "1"
	featureDiscount := r.FormValue("feature_discount") == "1"
	featureTmall := r.FormValue("feature_tmall") == "1"
	pricesOnly := r.FormValue("prices_only") == "1"

	jobType := "products"
	if pricesOnly {
		jobType = "prices"
	}

	jobID, err := store.CreateSyncJob(jobType, categoryID, "manual")
	if err != nil {
		http.Error(w, "create job: "+err.Error(), 500)
		return
	}

	go func() {
		store.UpdateSyncJob(jobID, "running", 0, 0, 0, 0, "")

		if pricesOnly {
			updated, apiReqs, syncErr := imp.SyncPricesOnly(categoryID)
			status := "done"
			logText := fmt.Sprintf("Обновлено цен: %d, API: %d", updated, apiReqs)
			if syncErr != nil {
				status = "error"
				logText += "\nERROR: " + syncErr.Error()
			}
			store.UpdateSyncJob(jobID, status, updated, 0, 0, apiReqs, logText)
		} else {
			opts := sync.SyncOptions{
				MinVolume:       minVolume,
				MinPrice:        minPrice,
				MaxPrice:        maxPrice,
				MaxPriceLimit:   maxPriceLimit,
				ItemTitle:       itemTitle,
				VendorName:      vendorName,
				BrandName:       brandName,
				OrderBy:         orderBy,
				StuffStatus:     stuffStatus,
				MinVendorRating: minVendorRating,
				MaxVendorRating: maxVendorRating,
				FirstLotMin:     firstLotMin,
				FirstLotMax:     firstLotMax,
				SearchMethod:    searchMethod,
				FeatureComplete: featureComplete,
				FeatureDiscount: featureDiscount,
				FeatureTmall:    featureTmall,
				JobID:           jobID,
			}
			result := imp.SyncProducts(categoryID, maxP, opts, nil)
			status := "done"
			if result.Errors > 0 && result.Processed == 0 {
				status = "error"
			}
			// log_text уже записан в real-time через AppendSyncLog - не перезаписываем
			store.Hub.Exec(`UPDATE sync_jobs SET status=?, finished_at=?,
				items_processed=?, items_skipped=?, errors_count=?, api_requests_made=?
				WHERE id=?`, status, time.Now().Unix(),
				result.Processed, result.Skipped, result.Errors, result.APIRequests, jobID)

			store.Hub.Exec(`UPDATE category_config SET last_synced_at=?, products_imported=products_imported+?
				WHERE category_id=?`, time.Now().Unix(), result.Processed, categoryID)
		}
	}()

	http.Redirect(w, r, fmt.Sprintf("/otweb/sync?started=%d", jobID), http.StatusSeeOther)
}

func handlePushPage(w http.ResponseWriter, r *http.Request) {
	// Pushed products (already in CS-Cart)
	pushed, _, _ := store.GetProductsFiltered(db.ProductFilter{PushedOnly: true, SortBy: "sales"}, 1, 100)

	// Unpushed enabled products (ready to push)
	unpushed, _, _ := store.GetProductsFiltered(db.ProductFilter{UnpushedOnly: true, EnabledOnly: true, SortBy: "sales"}, 1, 100)

	render(w, "push", "Push - Wabrum", D{
		"PushedProducts":  pushed,
		"UnpushedProducts": unpushed,
		"PushedCount":     len(pushed),
		"UnpushedCount":   len(unpushed),
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
	// 0 = выключен, не подменяем на 6
	return
}

// Список всех провайдеров OT Commerce
type providerInfo struct {
	ID      string
	Name    string
	Enabled bool
}

var allProviders = []providerInfo{
	{"alibaba1688", "1688.com", true},
	{"taobao", "Taobao", false},
	{"jd", "JD.com", false},
	{"poizon", "Poizon (Dewu)", false},
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	markup, _ := store.GetGlobalMarkup()
	settings := store.GetAllSettings()
	pricesH, syncH, cronLines, cronActive := readCronConfig()

	// Dereferenced pricing values for template
	var exchangeRate, markupPct, fixedAddon, effectiveRate float64
	exchangeRate = 2.74
	markupPct = 35.0
	if markup != nil {
		markupPct = markup.MarkupPct
		fixedAddon = markup.FixedAddon
		if markup.ExchangeRate != nil {
			exchangeRate = *markup.ExchangeRate
		}
	}
	effectiveRate = exchangeRate * (1 + markupPct/100)

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
		"ExchangeRate":  exchangeRate,
		"MarkupPct":     markupPct,
		"FixedAddon":    fixedAddon,
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

	// Сначала пробуем обновить существующую global запись
	res, _ := store.Hub.Exec(`UPDATE markup_rules SET markup_pct=?, exchange_rate=?, fixed_addon=?
		WHERE scope_type='global'`, markupPct, exchangeRate, fixedAddon)
	if n, _ := res.RowsAffected(); n == 0 {
		store.Hub.Exec(`INSERT INTO markup_rules (scope_type, scope_id, markup_pct, exchange_rate, fixed_addon, is_active, notes, created_at)
			VALUES ('global', '', ?, ?, ?, 1, 'Global markup', UNIX_TIMESTAMP())`,
			markupPct, exchangeRate, fixedAddon)
	}

	http.Redirect(w, r, "/otweb/settings", http.StatusSeeOther)
}

func handleSettingsDelivery(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	for _, k := range []string{"delivery_cost_per_kg", "usd_to_cny", "delivery_included"} {
		if v := r.FormValue(k); v != "" {
			store.SaveSetting(k, v)
		}
	}
	http.Redirect(w, r, "/otweb/settings", http.StatusSeeOther)
}

func handleSettingsCron(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	pricesH, _ := strconv.Atoi(r.FormValue("prices_every_h"))
	syncH, _ := strconv.Atoi(r.FormValue("sync_every_h"))
	// 0 = выключен (не добавляем cron)

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

	// Добавляем новые строки только если > 0
	if pricesH > 0 {
		newLines = append(newLines, fmt.Sprintf("0 */%d * * * curl -s -X POST http://localhost:5500/otweb/sync/prices -d 'category_id=' > /dev/null 2>&1 # otapi-hub prices", pricesH))
	}
	if syncH > 0 {
		newLines = append(newLines, fmt.Sprintf("0 */%d * * * curl -s -X POST http://localhost:5500/otweb/sync/run-cron > /dev/null 2>&1 # otapi-hub sync", syncH))
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
		log.Println("[push] Legacy push disabled - use API push from Mapping page")
		
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

	// Unmapped OT categories (не имеют маппинга)
	mappedSet := make(map[string]bool)
	for _, m := range mappings {
		mappedSet[m.OTCategoryID] = true
	}
	var unmappedOT []db.CategoryWithConfig
	for _, c := range otCats {
		if !mappedSet[c.ID] && !sync.HasChinese(c.Name) {
			unmappedOT = append(unmappedOT, c)
		}
	}

	// Только переведённые OT категории (без китайских)
	var otCatsRu []db.CategoryWithConfig
	for _, c := range otCats {
		if !sync.HasChinese(c.Name) {
			otCatsRu = append(otCatsRu, c)
		}
	}

	// Строим дерево OT категорий
	otNodeMap := make(map[string]*CategoryNode)
	for i := range otCats {
		c := &otCats[i]
		if !sync.HasChinese(c.Name) {
			otNodeMap[c.ID] = &CategoryNode{CategoryWithConfig: *c}
		}
	}
	var otTree []CategoryNode
	for _, c := range otCats {
		if c.ParentID == "" && !sync.HasChinese(c.Name) {
			node := *otNodeMap[c.ID]
			for _, child := range otCats {
				if child.ParentID == c.ID && !sync.HasChinese(child.Name) {
					if cn, ok := otNodeMap[child.ID]; ok {
						node.Children = append(node.Children, *cn)
					}
				}
			}
			otTree = append(otTree, node)
		}
	}
	sort.Slice(otTree, func(i, j int) bool { return otTree[i].Name < otTree[j].Name })

	render(w, "mapping", "Category Mapping", D{
		"Mappings":     mappings,
		"OTCategories": otCatsRu,
		"OTTree":       otTree,
		"CSCategories": csCats,
		"UnmappedOT":   unmappedOT,
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

func handleMappingSetWeight(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	otCatID := r.FormValue("ot_category_id")
	weightG, _ := strconv.Atoi(r.FormValue("weight_g"))
	if otCatID != "" {
		store.UpdateCategoryWeight(otCatID, weightG)
	}
	http.Redirect(w, r, "/otweb/mapping", http.StatusSeeOther)
}

func handleMappingSetFilters(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	otCatID := r.FormValue("ot_category_id")
	minPrice, _ := strconv.Atoi(r.FormValue("min_price_cny"))
	maxPrice, _ := strconv.Atoi(r.FormValue("max_price_cny"))
	minVolume, _ := strconv.Atoi(r.FormValue("min_volume"))
	if otCatID != "" {
		store.UpdateCategoryPriceFilters(otCatID, minPrice, maxPrice, minVolume)
	}
	http.Redirect(w, r, "/otweb/mapping", http.StatusSeeOther)
}

func handleRefreshCSCart(w http.ResponseWriter, r *http.Request) {
	// Загружаем категории из CS-Cart API синхронно (чтобы данные были при redirect)
	func() {
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

