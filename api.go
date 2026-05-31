package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"otapi-hub/db"
	"otapi-hub/sync"
	"otapi-hub/translate"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

func jsonOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func jsonErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonData(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data, "ok": true})
}

func parseJSON(r *http.Request, dest interface{}) error {
	return json.NewDecoder(r.Body).Decode(dest)
}

// ── AUTH ──────────────────────────────────────────────────────────

func apiLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := parseJSON(r, &body); err != nil {
		jsonErr(w, 400, "invalid json")
		return
	}
	if body.Username != cfg.Auth.Username || body.Password != cfg.Auth.Password {
		jsonErr(w, 401, "invalid credentials")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "otweb_session", Value: sessionToken, Path: "/otweb",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	jsonOK(w)
}

func apiLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "otweb_session", Value: "", Path: "/otweb", MaxAge: -1})
	jsonOK(w)
}

func apiMe(w http.ResponseWriter, r *http.Request) {
	jsonData(w, map[string]string{"username": cfg.Auth.Username})
}

// ── DASHBOARD ────────────────────────────────────────────────────

func apiDashboard(w http.ResponseWriter, r *http.Request) {
	stats, _ := store.GetDashboardStats()
	jobs, _ := store.GetRecentSyncJobs(10)
	jsonData(w, map[string]interface{}{"stats": stats, "jobs": jobs})
}

// ── CATEGORIES ───────────────────────────────────────────────────

func apiCategories(w http.ResponseWriter, r *http.Request) {
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

	allNodes := make(map[string]*CategoryNode)
	for i := range cats {
		c := &cats[i]
		node := &CategoryNode{CategoryWithConfig: *c}
		if m, ok := mappingMap[c.ID]; ok {
			node.CSCategoryName = m.CSCategoryName
		}
		node.ItemCountM = strconv.FormatFloat(float64(c.ItemCount)/1_000_000, 'f', 1, 64)
		node.ItemCountK = strconv.FormatFloat(float64(c.ItemCount)/1_000, 'f', 0, 64)
		allNodes[c.ID] = node
	}

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

	switch sortFilter {
	case "items_desc":
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].ItemCount > filtered[j].ItemCount })
	case "name":
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name < filtered[j].Name })
	case "synced":
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].LocalCount > filtered[j].LocalCount })
	}

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

	jsonData(w, map[string]interface{}{
		"categories": filtered,
		"tree":       treeNodes,
		"total":      len(cats),
	})
}

func apiSyncMeta(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CleanFirst bool `json:"clean_first"`
	}
	parseJSON(r, &body)
	go imp.SyncCategories(body.CleanFirst)
	jsonOK(w)
}

func apiCategoriesTranslate(w http.ResponseWriter, r *http.Request) {
	if cfg.DeepSeek.APIKey == "" {
		jsonErr(w, 400, "DeepSeek API key not configured")
		return
	}
	go func() {
		dsClient := translate.NewDeepSeekClient(cfg.DeepSeek.APIKey, cfg.DeepSeek.BaseURL)
		rows, err := store.Hub.Query(`SELECT id, name_ru FROM categories WHERE name_ru = name_zh AND name_ru != '' ORDER BY id`)
		if err != nil {
			return
		}
		defer rows.Close()
		type catItem struct{ ID, Name string }
		var items []catItem
		for rows.Next() {
			var it catItem
			rows.Scan(&it.ID, &it.Name)
			items = append(items, it)
		}
		rows.Close()
		for i := 0; i < len(items); i += 20 {
			end := i + 20
			if end > len(items) {
				end = len(items)
			}
			batch := items[i:end]
			names := make([]string, len(batch))
			for j, it := range batch {
				names[j] = it.Name
			}
			prompt := `Переведи названия категорий товаров с китайского на русский. Верни JSON: {"translations": [{"original": "...", "ru": "..."}]}
Категории: ` + strings.Join(names, ", ")
			resp, err := dsClient.RawChat(prompt)
			if err != nil {
				continue
			}
			var result struct {
				Translations []struct {
					Original string `json:"original"`
					Ru       string `json:"ru"`
				} `json:"translations"`
			}
			if err := json.Unmarshal([]byte(resp), &result); err != nil {
				continue
			}
			origToRu := make(map[string]string)
			for _, t := range result.Translations {
				origToRu[t.Original] = t.Ru
			}
			for _, it := range batch {
				if ru, ok := origToRu[it.Name]; ok && ru != "" {
					store.Hub.Exec(`UPDATE categories SET name_ru=? WHERE id=?`, ru, it.ID)
				}
			}
		}
	}()
	jsonOK(w)
}

func apiCategoryToggle(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var enabled bool
	store.Hub.QueryRow(`SELECT IFNULL(enabled, 0) FROM category_config WHERE category_id=?`, id).Scan(&enabled)
	store.UpsertCategoryConfig(id, !enabled, "manual", 500, nil, "")
	jsonData(w, map[string]bool{"enabled": !enabled})
}

func apiCategoryConfig(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Enabled     bool   `json:"enabled"`
		Schedule    string `json:"schedule"`
		MaxProducts int    `json:"max_products"`
	}
	if err := parseJSON(r, &body); err != nil {
		jsonErr(w, 400, "invalid json")
		return
	}
	if body.MaxProducts == 0 {
		body.MaxProducts = 500
	}
	store.UpsertCategoryConfig(id, body.Enabled, body.Schedule, body.MaxProducts, nil, "")
	jsonOK(w)
}

// ── ATTRS ────────────────────────────────────────────────────────

func apiAttrs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}
	search := q.Get("search")
	status := q.Get("status") // all | translated | pending

	var totalAll, translated int
	store.Hub.QueryRow(`SELECT COUNT(DISTINCT pid, vid) FROM product_attrs WHERE pid != '' AND vid != ''`).Scan(&totalAll)
	store.Hub.QueryRow(`SELECT COUNT(*) FROM attr_translations`).Scan(&translated)
	pending := totalAll - translated

	// Build filtered query
	var whereClauses []string
	var args []interface{}

	switch status {
	case "translated":
		whereClauses = append(whereClauses, `at.pid IS NOT NULL AND at.property_name_ru != ''`)
	case "pending":
		whereClauses = append(whereClauses, `(at.pid IS NULL OR at.property_name_ru = '')`)
	}

	if search != "" {
		like := "%" + search + "%"
		whereClauses = append(whereClauses, `(pa.pid LIKE ? OR pa.vid LIKE ? OR pa.property_name LIKE ? OR pa.value LIKE ? OR at.property_name_ru LIKE ? OR at.value_ru LIKE ?)`)
		args = append(args, like, like, like, like, like, like)
	}

	baseQuery := `
		FROM (SELECT DISTINCT pid, vid, property_name, value FROM product_attrs WHERE pid != '' AND vid != '') pa
		LEFT JOIN attr_translations at ON at.pid=pa.pid AND at.vid=pa.vid`
	if len(whereClauses) > 0 {
		baseQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	var totalFiltered int
	store.Hub.QueryRow(`SELECT COUNT(*) `+baseQuery, args...).Scan(&totalFiltered)

	offset := (page - 1) * perPage
	selectArgs := append(args, perPage, offset)
	rows, _ := store.Hub.Query(`
		SELECT pa.pid, pa.vid,
		       COALESCE(at.property_name_zh, pa.property_name) AS property_name_zh,
		       COALESCE(at.value_zh, pa.value) AS value_zh,
		       COALESCE(at.property_name_ru, '') AS property_name_ru,
		       COALESCE(at.value_ru, '') AS value_ru,
		       IFNULL(at.translated_at, 0) AS translated_at
		`+baseQuery+`
		ORDER BY at.translated_at DESC
		LIMIT ? OFFSET ?`, selectArgs...)

	var items []db.AttrTranslation
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var at db.AttrTranslation
			rows.Scan(&at.Pid, &at.Vid, &at.PropertyNameZh, &at.ValueZh, &at.PropertyNameRu, &at.ValueRu, &at.TranslatedAt)
			items = append(items, at)
		}
	}

	jsonData(w, map[string]interface{}{
		"total_all":      totalAll,
		"total_filtered": totalFiltered,
		"translated":     translated,
		"pending":        pending,
		"page":           page,
		"per_page":       perPage,
		"items":          items,
	})
}

func apiAttrsTranslateSelected(w http.ResponseWriter, r *http.Request) {
	if cfg.DeepSeek.APIKey == "" {
		jsonErr(w, 400, "DeepSeek API key not configured")
		return
	}
	var body struct {
		Pairs []struct {
			Pid string `json:"pid"`
			Vid string `json:"vid"`
		} `json:"pairs"`
	}
	if err := parseJSON(r, &body); err != nil {
		jsonErr(w, 400, "invalid json")
		return
	}
	if len(body.Pairs) == 0 {
		jsonErr(w, 400, "no pairs")
		return
	}
	type pairData struct {
		Pid, Vid, Name, Value string
	}
	var pairs []pairData
	for _, p := range body.Pairs {
		var name, value string
		store.Hub.QueryRow(`SELECT property_name, value FROM product_attrs WHERE pid=? AND vid=? LIMIT 1`, p.Pid, p.Vid).Scan(&name, &value)
		pairs = append(pairs, pairData{Pid: p.Pid, Vid: p.Vid, Name: name, Value: value})
	}
	go func() {
		dsClient := translate.NewDeepSeekClient(cfg.DeepSeek.APIKey, cfg.DeepSeek.BaseURL)
		for i := 0; i < len(pairs); i += 50 {
			end := i + 50
			if end > len(pairs) {
				end = len(pairs)
			}
			batch := pairs[i:end]
			apairs := make([]translate.AttrPair, len(batch))
			for j, p := range batch {
				apairs[j] = translate.AttrPair{Pid: p.Pid, Vid: p.Vid, Name: p.Name, Value: p.Value}
			}
			results, err := dsClient.TranslateAttrs(apairs)
			if err != nil {
				break
			}
			for _, res := range results {
				var nameZh, valueZh string
				for _, p := range batch {
					if p.Pid == res.Pid && p.Vid == res.Vid {
						nameZh, valueZh = p.Name, p.Value
						break
					}
				}
				store.SaveAttrTranslation(res.Pid, res.Vid, nameZh, res.NameRu, valueZh, res.ValueRu)
			}
		}
	}()
	jsonData(w, map[string]interface{}{"translating": len(pairs)})
}

func apiAttrsSave(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Pid           string `json:"pid"`
		Vid           string `json:"vid"`
		PropertyNameRu string `json:"property_name_ru"`
		ValueRu       string `json:"value_ru"`
	}
	if err := parseJSON(r, &body); err != nil {
		jsonErr(w, 400, "invalid json")
		return
	}
	var nameZh, valueZh string
	store.Hub.QueryRow(`SELECT COALESCE(property_name_zh,''), COALESCE(value_zh,'') FROM attr_translations WHERE pid=? AND vid=?`, body.Pid, body.Vid).Scan(&nameZh, &valueZh)
	if nameZh == "" {
		store.Hub.QueryRow(`SELECT property_name, value FROM product_attrs WHERE pid=? AND vid=? LIMIT 1`, body.Pid, body.Vid).Scan(&nameZh, &valueZh)
	}
	if err := store.SaveAttrTranslation(body.Pid, body.Vid, nameZh, body.PropertyNameRu, valueZh, body.ValueRu); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w)
}

func apiAttrsTranslate(w http.ResponseWriter, r *http.Request) {
	if cfg.DeepSeek.APIKey == "" {
		jsonErr(w, 400, "DeepSeek API key not configured")
		return
	}
	go func() {
		dsClient := translate.NewDeepSeekClient(cfg.DeepSeek.APIKey, cfg.DeepSeek.BaseURL)
		for {
			untranslated, err := store.GetUntranslatedAttrs(50)
			if err != nil || len(untranslated) == 0 {
				break
			}
			pairs := make([]translate.AttrPair, len(untranslated))
			for i, a := range untranslated {
				pairs[i] = translate.AttrPair{Pid: a.Pid, Vid: a.Vid, Name: a.PropertyNameZh, Value: a.ValueZh}
			}
			results, err := dsClient.TranslateAttrs(pairs)
			if err != nil {
				break
			}
			for _, res := range results {
				var nameZh, valueZh string
				for _, p := range pairs {
					if p.Pid == res.Pid && p.Vid == res.Vid {
						nameZh, valueZh = p.Name, p.Value
						break
					}
				}
				store.SaveAttrTranslation(res.Pid, res.Vid, nameZh, res.NameRu, valueZh, res.ValueRu)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()
	jsonOK(w)
}

// ── PRODUCTS ────────────────────────────────────────────────────

func apiProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 || perPage > 200 {
		perPage = 50
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
	products, total, err := store.GetProductsFiltered(filter, page, perPage)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	totalPages := (total + perPage - 1) / perPage
	var untranslatedCount int
	store.Hub.QueryRow(`SELECT COUNT(*) FROM products WHERE translate_status='pending' OR translate_status='' OR translate_status IS NULL`).Scan(&untranslatedCount)
	cats, _ := store.GetCategoriesWithConfig()
	jsonData(w, map[string]interface{}{
		"products": products, "total": total,
		"total_pages": totalPages, "current_page": page, "per_page": perPage,
		"untranslated_count": untranslatedCount,
		"categories": cats,
	})
}

func apiProductDetail(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	product, err := store.GetProductByID(id)
	if err != nil {
		jsonErr(w, 404, "not found")
		return
	}
	type skuRow struct {
		SkuID    string  `json:"sku_id"`
		Qty      int     `json:"qty"`
		PriceCNY float64 `json:"price_cny"`
	}
	var skus []skuRow
	skuRows, _ := store.Hub.Query(`SELECT sku_id, qty, price_cny FROM product_skus WHERE product_id=?`, id)
	if skuRows != nil {
		defer skuRows.Close()
		for skuRows.Next() {
			var s skuRow
			skuRows.Scan(&s.SkuID, &s.Qty, &s.PriceCNY)
			skus = append(skus, s)
		}
	}
	type attrRow struct {
		Name           string `json:"name"`
		Value          string `json:"value"`
		NameRu         string `json:"name_ru"`
		ValueRu        string `json:"value_ru"`
		IsConfigurator bool   `json:"is_configurator"`
	}
	var attrs []attrRow
	rows, _ := store.Hub.Query(`
		SELECT pa.property_name, pa.value, pa.is_configurator,
		       COALESCE(at.property_name_ru,''), COALESCE(at.value_ru,'')
		FROM product_attrs pa
		LEFT JOIN attr_translations at ON at.pid=pa.pid AND at.vid=pa.vid
		WHERE pa.product_id=?
		ORDER BY pa.is_configurator DESC, pa.property_name`, id)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var a attrRow
			rows.Scan(&a.Name, &a.Value, &a.IsConfigurator, &a.NameRu, &a.ValueRu)
			attrs = append(attrs, a)
		}
	}
	var untranslatedAttrs int
	store.Hub.QueryRow(`
		SELECT COUNT(*) FROM product_attrs pa
		LEFT JOIN attr_translations at ON at.pid=pa.pid AND at.vid=pa.vid
		WHERE pa.product_id=? AND (at.pid IS NULL OR at.property_name_ru='')`, id).Scan(&untranslatedAttrs)
	type imageRow struct {
		URL string `json:"url"`
	}
	var images []imageRow
	irows, _ := store.Hub.Query(`SELECT IFNULL(medium_url, url) FROM product_images WHERE product_id=? ORDER BY sort_order`, id)
	if irows != nil {
		defer irows.Close()
		for irows.Next() {
			var img imageRow
			irows.Scan(&img.URL)
			images = append(images, img)
		}
	}
	jsonData(w, map[string]interface{}{
		"product": product, "skus": skus, "attrs": attrs, "images": images,
		"untranslated_attrs": untranslatedAttrs,
	})
}

func apiProductTranslate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	var body struct {
		Action  string `json:"action"`
		TitleRu string `json:"title_ru"`
		TitleEn string `json:"title_en"`
		TitleTk string `json:"title_tk"`
		DescRu  string `json:"desc_ru"`
		DescEn  string `json:"desc_en"`
		DescTk  string `json:"desc_tk"`
	}
	parseJSON(r, &body)
	if body.Action == "manual" {
		store.Hub.Exec(`UPDATE products SET title_ru=?, title_en=?, title_tk=?, description_ru=?, description_en=?, description_tk=?, translate_status='done', updated_at=? WHERE id=?`,
			body.TitleRu, body.TitleEn, body.TitleTk, body.DescRu, body.DescEn, body.DescTk, time.Now().Unix(), id)
		jsonOK(w)
		return
	}
	go func() {
		if cfg.DeepSeek.APIKey == "" {
			return
		}
		product, err := store.GetProductByID(id)
		if err != nil {
			return
		}
		dsClient := translate.NewDeepSeekClient(cfg.DeepSeek.APIKey, cfg.DeepSeek.BaseURL)
		attrs := make(map[string]string)
		rows, _ := store.Hub.Query(`SELECT property_name, value FROM product_attrs WHERE product_id=? AND is_configurator=0`, id)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var k, v string
				rows.Scan(&k, &v)
				attrs[k] = v
			}
		}
		result, err := dsClient.Normalize(translate.NormalizeInput{
			TitleOriginal: product.TitleOriginal, TitleRu: product.TitleRu, Attributes: attrs,
		})
		if err != nil {
			return
		}
		store.Hub.Exec(`UPDATE products SET title_ru=?, description_ru=?, translate_status='done', updated_at=? WHERE id=?`,
			result.TitleRU, result.DescriptionRU, time.Now().Unix(), id)
	}()
	jsonData(w, map[string]string{"status": "started"})
}

func apiProductPush(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	// find category mapping
	var categoryCS int
	store.Hub.QueryRow(`
		SELECT IFNULL(cm.cs_category_id, 0) FROM products p
		LEFT JOIN category_map cm ON cm.otapi_category_id = p.category_id
		WHERE p.id=?`, id).Scan(&categoryCS)
	go apiPusher.PushSingleProduct(id, categoryCS)
	jsonData(w, map[string]string{"status": "started"})
}

func apiProductTranslateAttrs(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if cfg.DeepSeek.APIKey == "" {
		jsonErr(w, 400, "DeepSeek API key not configured")
		return
	}
	// Gather untranslated (pid,vid) pairs for this product
	type pair struct{ Pid, Vid, Name, Value string }
	var pairs []pair
	rows, _ := store.Hub.Query(`
		SELECT pa.pid, pa.vid, pa.property_name, pa.value
		FROM product_attrs pa
		LEFT JOIN attr_translations at ON at.pid=pa.pid AND at.vid=pa.vid
		WHERE pa.product_id=? AND pa.pid!='' AND (at.pid IS NULL OR at.property_name_ru='')
		GROUP BY pa.pid, pa.vid`, id)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var p pair
			rows.Scan(&p.Pid, &p.Vid, &p.Name, &p.Value)
			pairs = append(pairs, p)
		}
	}
	if len(pairs) == 0 {
		jsonData(w, map[string]interface{}{"translated": 0, "message": "all attrs already translated"})
		return
	}
	go func() {
		log.Printf("[translate-attrs] product %d: starting translation of %d pairs", id, len(pairs))
		dsClient := translate.NewDeepSeekClient(cfg.DeepSeek.APIKey, cfg.DeepSeek.BaseURL)
		totalSaved := 0
		// Batch by 50
		for i := 0; i < len(pairs); i += 50 {
			end := i + 50
			if end > len(pairs) {
				end = len(pairs)
			}
			batch := pairs[i:end]
			apairs := make([]translate.AttrPair, len(batch))
			for j, p := range batch {
				apairs[j] = translate.AttrPair{Pid: p.Pid, Vid: p.Vid, Name: p.Name, Value: p.Value}
			}
			results, err := dsClient.TranslateAttrs(apairs)
			if err != nil {
				log.Printf("[translate-attrs] product %d: batch %d-%d error: %v", id, i, end, err)
				break
			}
			log.Printf("[translate-attrs] product %d: batch %d-%d got %d results", id, i, end, len(results))
			for _, res := range results {
				var nameZh, valueZh string
				for _, p := range batch {
					if p.Pid == res.Pid && p.Vid == res.Vid {
						nameZh, valueZh = p.Name, p.Value
						break
					}
				}
				if err := store.SaveAttrTranslation(res.Pid, res.Vid, nameZh, res.NameRu, valueZh, res.ValueRu); err != nil {
					log.Printf("[translate-attrs] SaveAttrTranslation(%s,%s): %v", res.Pid, res.Vid, err)
				} else {
					totalSaved++
				}
			}
		}
		log.Printf("[translate-attrs] product %d: done, saved %d translations", id, totalSaved)
	}()
	jsonData(w, map[string]interface{}{"translating": len(pairs), "message": "translation started"})
}

func apiProductToggle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	var enabled bool
	store.Hub.QueryRow(`SELECT enabled FROM products WHERE id=?`, id).Scan(&enabled)
	store.Hub.Exec(`UPDATE products SET enabled=? WHERE id=?`, !enabled, id)
	jsonData(w, map[string]bool{"enabled": !enabled})
}

func apiBulkAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action     string  `json:"action"`
		ProductIDs []int64 `json:"product_ids"`
	}
	if err := parseJSON(r, &body); err != nil {
		jsonErr(w, 400, "invalid json")
		return
	}
	if len(body.ProductIDs) == 0 {
		jsonErr(w, 400, "no product_ids")
		return
	}
	switch body.Action {
	case "enable":
		for _, id := range body.ProductIDs {
			store.Hub.Exec(`UPDATE products SET enabled=1 WHERE id=?`, id)
		}
	case "disable":
		for _, id := range body.ProductIDs {
			store.Hub.Exec(`UPDATE products SET enabled=0 WHERE id=?`, id)
		}
	case "delete":
		for _, id := range body.ProductIDs {
			store.Hub.Exec(`DELETE FROM products WHERE id=?`, id)
		}
	}
	jsonOK(w)
}

func apiBulkTranslate(w http.ResponseWriter, r *http.Request) {
	go func() {
		rows, _ := store.Hub.Query(`SELECT id FROM products WHERE (translate_status='pending' OR translate_status='') AND enabled=1 LIMIT 100`)
		if rows == nil {
			return
		}
		var ids []int64
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
		// trigger individual translations
		for _, id := range ids {
			if cfg.DeepSeek.APIKey == "" {
				break
			}
			product, err := store.GetProductByID(id)
			if err != nil {
				continue
			}
			dsClient := translate.NewDeepSeekClient(cfg.DeepSeek.APIKey, cfg.DeepSeek.BaseURL)
			result, err := dsClient.Normalize(translate.NormalizeInput{
				TitleOriginal: product.TitleOriginal, TitleRu: product.TitleRu,
			})
			if err != nil {
				continue
			}
			store.Hub.Exec(`UPDATE products SET title_ru=?, description_ru=?, translate_status='done', updated_at=? WHERE id=?`,
				result.TitleRU, result.DescriptionRU, time.Now().Unix(), id)
			time.Sleep(200 * time.Millisecond)
		}
	}()
	jsonOK(w)
}

func apiTranslateLocations(w http.ResponseWriter, r *http.Request) {
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
	jsonData(w, map[string]int{"states": stateUpdated, "cities": cityUpdated})
}

// ── SYNC ─────────────────────────────────────────────────────────

func apiSyncPage(w http.ResponseWriter, r *http.Request) {
	jobs, _ := store.GetRecentSyncJobs(20)
	cats, _ := store.GetCategoriesWithConfig()
	jsonData(w, map[string]interface{}{"jobs": jobs, "categories": cats})
}

func apiSyncRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CategoryID      string  `json:"category_id"`
		ItemTitle       string  `json:"item_title"`
		MaxProducts     int     `json:"max_products"`
		MinVolume       int     `json:"min_volume"`
		MinPrice        float64 `json:"min_price"`
		MaxPrice        float64 `json:"max_price"`
		MaxPriceLimit   float64 `json:"max_price_limit"`
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
	if err := parseJSON(r, &body); err != nil {
		jsonErr(w, 400, "invalid json")
		return
	}
	if body.MaxProducts == 0 {
		body.MaxProducts = 500
	}
	jobType := "products"
	if body.PricesOnly {
		jobType = "prices"
	}
	jobID, err := store.CreateSyncJob(jobType, body.CategoryID, "manual")
	if err != nil {
		jsonErr(w, 500, "create job: "+err.Error())
		return
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
		} else {
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
			result := imp.SyncProducts(body.CategoryID, body.MaxProducts, opts, nil)
			status := "done"
			if result.Errors > 0 && result.Processed == 0 {
				status = "error"
			}
			store.Hub.Exec(`UPDATE sync_jobs SET status=?, finished_at=?, items_processed=?, items_skipped=?, errors_count=?, api_requests_made=? WHERE id=?`,
				status, time.Now().Unix(), result.Processed, result.Skipped, result.Errors, result.APIRequests, jobID)
		}
	}()
	jsonData(w, map[string]interface{}{"job_id": jobID})
}

func apiSyncPrices(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CategoryID string `json:"category_id"`
	}
	parseJSON(r, &body)
	jobID, err := store.CreateSyncJob("prices", body.CategoryID, "manual")
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	go func() {
		store.UpdateSyncJob(jobID, "running", 0, 0, 0, 0, "")
		updated, apiReqs, syncErr := imp.SyncPricesOnly(body.CategoryID)
		status := "done"
		logText := fmt.Sprintf("Обновлено цен: %d, API: %d", updated, apiReqs)
		if syncErr != nil {
			status = "error"
			logText += "\nERROR: " + syncErr.Error()
		}
		store.UpdateSyncJob(jobID, status, updated, 0, 0, apiReqs, logText)
	}()
	jsonData(w, map[string]interface{}{"job_id": jobID})
}

func apiSyncJobStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	var status, logText string
	var processed int
	store.Hub.QueryRow(`SELECT status, IFNULL(log_text,''), IFNULL(items_processed,0) FROM sync_jobs WHERE id=?`, id).
		Scan(&status, &logText, &processed)
	jsonData(w, map[string]interface{}{
		"status": status, "log": logText, "items_processed": processed,
	})
}

// ── PUSH ─────────────────────────────────────────────────────────

func apiPushPage(w http.ResponseWriter, r *http.Request) {
	var pushedCount, unpushedCount int
	store.Hub.QueryRow(`SELECT COUNT(*) FROM products WHERE pushed_to_cs_at IS NOT NULL AND enabled=1`).Scan(&pushedCount)
	store.Hub.QueryRow(`SELECT COUNT(*) FROM products WHERE pushed_to_cs_at IS NULL AND enabled=1`).Scan(&unpushedCount)
	mappings, _ := store.GetCategoryMappings()
	jsonData(w, map[string]interface{}{
		"pushed_count": pushedCount, "unpushed_count": unpushedCount, "mappings": mappings,
	})
}

func apiPushCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CategoryID string `json:"category_id"`
	}
	if err := parseJSON(r, &body); err != nil {
		jsonErr(w, 400, "invalid json")
		return
	}
	go apiPusher.PushCategoryAuto(body.CategoryID)
	jsonOK(w)
}

// ── MAPPING ──────────────────────────────────────────────────────

// otCatOption - плоская запись для дропдауна с префиксом └ для подкатегорий
type otCatOption struct {
	ID        string `json:"ID"`
	Name      string `json:"Name"`
	ItemCount int    `json:"ItemCount"`
	IsChild   bool   `json:"IsChild"`
}

func apiMappingPage(w http.ResponseWriter, r *http.Request) {
	mappings, _ := store.GetCategoryMappings()
	otCats, _ := store.GetCategoriesWithConfig()
	csCats, _ := store.GetCSCartCategories()
	settings := store.GetAllSettings()
	mappedSet := make(map[string]bool)
	for _, m := range mappings {
		mappedSet[m.OTCategoryID] = true
	}

	// Строим индексы: родители и дети
	parentMap := make(map[string][]db.CategoryWithConfig) // parentID -> []children
	var parents []db.CategoryWithConfig
	catIndex := make(map[string]db.CategoryWithConfig)
	for _, c := range otCats {
		catIndex[c.ID] = c
		if c.ParentID == "" {
			parents = append(parents, c)
		} else {
			parentMap[c.ParentID] = append(parentMap[c.ParentID], c)
		}
	}
	sort.Slice(parents, func(i, j int) bool { return parents[i].Name < parents[j].Name })

	// Составляем иерархический список: родитель, потом его дети
	var unmappedOT []otCatOption
	for _, p := range parents {
		if !mappedSet[p.ID] && !sync.HasChinese(p.Name) {
			unmappedOT = append(unmappedOT, otCatOption{ID: p.ID, Name: p.Name, ItemCount: p.ItemCount})
		}
		kids := parentMap[p.ID]
		sort.Slice(kids, func(i, j int) bool { return kids[i].Name < kids[j].Name })
		for _, k := range kids {
			if !mappedSet[k.ID] && !sync.HasChinese(k.Name) {
				unmappedOT = append(unmappedOT, otCatOption{ID: k.ID, Name: k.Name, ItemCount: k.ItemCount, IsChild: true})
			}
		}
	}

	jsonData(w, map[string]interface{}{
		"mappings": mappings, "unmapped_ot": unmappedOT,
		"cs_categories": csCats, "settings": settings,
	})
}

func apiMappingAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OTCategoryID   string `json:"ot_category_id"`
		CSCategoryID   int    `json:"cs_category_id"`
		CSCategoryName string `json:"cs_category_name"`
		Notes          string `json:"notes"`
	}
	if err := parseJSON(r, &body); err != nil {
		jsonErr(w, 400, "invalid json")
		return
	}
	store.Hub.Exec(`INSERT INTO category_map (otapi_category_id, cs_category_id, cs_category_name, notes)
		VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE cs_category_id=VALUES(cs_category_id), cs_category_name=VALUES(cs_category_name), notes=VALUES(notes)`,
		body.OTCategoryID, body.CSCategoryID, body.CSCategoryName, body.Notes)
	jsonOK(w)
}

func apiMappingDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OTCategoryID string `json:"ot_category_id"`
	}
	parseJSON(r, &body)
	store.Hub.Exec(`DELETE FROM category_map WHERE otapi_category_id=?`, body.OTCategoryID)
	jsonOK(w)
}

func apiMappingSetWeight(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OTCategoryID string `json:"ot_category_id"`
		WeightG      int    `json:"weight_g"`
	}
	parseJSON(r, &body)
	store.Hub.Exec(`UPDATE category_map SET weight_g=? WHERE otapi_category_id=?`, body.WeightG, body.OTCategoryID)
	jsonOK(w)
}

func apiMappingSetFilters(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OTCategoryID string `json:"ot_category_id"`
		MinPriceCNY  int    `json:"min_price_cny"`
		MaxPriceCNY  int    `json:"max_price_cny"`
		MinVolume    int    `json:"min_volume"`
	}
	parseJSON(r, &body)
	store.Hub.Exec(`UPDATE category_map SET min_price_cny=?, max_price_cny=?, min_volume=? WHERE otapi_category_id=?`,
		body.MinPriceCNY, body.MaxPriceCNY, body.MinVolume, body.OTCategoryID)
	jsonOK(w)
}

func apiRefreshCSCart(w http.ResponseWriter, r *http.Request) {
	body, status, err := csClient.Do("GET", "categories?items_per_page=500", nil)
	if err != nil || status != 200 {
		jsonErr(w, 500, fmt.Sprintf("CS-Cart error: %v (status %d)", err, status))
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
		jsonErr(w, 500, "unmarshal: "+err.Error())
		return
	}
	var cats []db.CSCartCategory
	for _, c := range resp.Categories {
		catID, _ := strconv.Atoi(c.CategoryID.String())
		parentID, _ := strconv.Atoi(c.ParentID.String())
		cats = append(cats, db.CSCartCategory{CategoryID: catID, ParentID: parentID, Name: c.Category})
	}
	store.CacheCSCartCategories(cats)
	jsonData(w, map[string]int{"count": len(cats)})
}

// ── SETTINGS ────────────────────────────────────────────────────

func apiSettings(w http.ResponseWriter, r *http.Request) {
	settings := store.GetAllSettings()
	// Sync cfg from DB values (DB takes priority over YAML for keys)
	if v := settings["otapi_instance_key"]; v != "" {
		cfg.OTAPI.InstanceKey = v
	}
	if v := settings["deepseek_api_key"]; v != "" {
		cfg.DeepSeek.APIKey = v
	}
	if v := settings["cscart_api_key"]; v != "" {
		cfg.CSCart.APIKey = v
	}
	markup, _ := store.GetGlobalMarkup()
	var markupPct, fixedAddon, exchangeRate float64
	if markup != nil {
		markupPct = markup.MarkupPct
		fixedAddon = markup.FixedAddon
		if markup.ExchangeRate != nil {
			exchangeRate = *markup.ExchangeRate
		}
	}
	jsonData(w, map[string]interface{}{
		"settings":            settings,
		"markup_pct":          markupPct,
		"fixed_addon":         fixedAddon,
		"exchange_rate":        exchangeRate,
		"otapi_key":           cfg.OTAPI.InstanceKey,
		"deepseek_key":        cfg.DeepSeek.APIKey,
		"cscart_key":          cfg.CSCart.APIKey,
		"cscart_url":          cfg.CSCart.BaseURL,
		"cscart_email":        cfg.CSCart.Email,
		"deepseek_base_url":   cfg.DeepSeek.BaseURL,
		"delivery_included":   settings["delivery_included"] == "true" || settings["delivery_included"] == "1",
		"delivery_cost_per_kg": settings["delivery_cost_per_kg"],
		"usd_to_cny":          settings["usd_to_cny"],
		"cron_prices_h":       settings["cron_prices_h"],
		"cron_sync_h":         settings["cron_sync_h"],
		"enabled_providers":   settings["enabled_providers"],
		"deepseek_prompt":     settings["deepseek_prompt"],
	})
}

func apiSettingsKeys(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OtapiKey    string `json:"otapi_key"`
		DeepseekKey string `json:"deepseek_key"`
		CscartKey   string `json:"cscart_key"`
		CscartURL   string `json:"cscart_url"`
		CscartEmail string `json:"cscart_email"`
	}
	parseJSON(r, &body)
	if body.OtapiKey != "" {
		cfg.OTAPI.InstanceKey = body.OtapiKey
		store.SaveSetting("otapi_instance_key", body.OtapiKey)
	}
	if body.DeepseekKey != "" {
		cfg.DeepSeek.APIKey = body.DeepseekKey
		store.SaveSetting("deepseek_api_key", body.DeepseekKey)
	}
	if body.CscartKey != "" {
		cfg.CSCart.APIKey = body.CscartKey
		store.SaveSetting("cscart_api_key", body.CscartKey)
	}
	if body.CscartURL != "" {
		cfg.CSCart.BaseURL = body.CscartURL
		store.SaveSetting("cscart_base_url", body.CscartURL)
	}
	if body.CscartEmail != "" {
		cfg.CSCart.Email = body.CscartEmail
		store.SaveSetting("cscart_email", body.CscartEmail)
	}
	jsonOK(w)
}

func apiSettingsProduct(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DefaultStatus string `json:"default_product_status"`
	}
	parseJSON(r, &body)
	if body.DefaultStatus != "" {
		store.SaveSetting("default_product_status", body.DefaultStatus)
	}
	jsonOK(w)
}

func apiSettingsPricing(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MarkupPct    float64 `json:"markup_pct"`
		ExchangeRate float64 `json:"exchange_rate"`
		FixedAddon   float64 `json:"fixed_addon"`
	}
	parseJSON(r, &body)
	store.Hub.Exec(`UPDATE markup_rules SET markup_pct=?, exchange_rate=?, fixed_addon=? WHERE scope_type='global'`,
		body.MarkupPct, body.ExchangeRate, body.FixedAddon)
	jsonOK(w)
}

func apiSettingsProviders(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Providers []string `json:"providers"`
	}
	parseJSON(r, &body)
	store.SaveSetting("enabled_providers", strings.Join(body.Providers, ","))
	jsonOK(w)
}

func apiSettingsCron(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PricesEveryH int `json:"prices_every_h"`
		SyncEveryH   int `json:"sync_every_h"`
	}
	parseJSON(r, &body)
	if body.PricesEveryH > 0 {
		store.SaveSetting("cron_prices_h", strconv.Itoa(body.PricesEveryH))
	}
	if body.SyncEveryH > 0 {
		store.SaveSetting("cron_sync_h", strconv.Itoa(body.SyncEveryH))
	}
	jsonOK(w)
}

func apiSettingsPrompt(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prompt string `json:"prompt"`
	}
	parseJSON(r, &body)
	store.SaveSetting("deepseek_prompt", body.Prompt)
	jsonOK(w)
}

func apiSettingsDelivery(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeliveryCostPerKg float64 `json:"delivery_cost_per_kg"`
		USDtoCNY          float64 `json:"usd_to_cny"`
		DeliveryIncluded  bool    `json:"delivery_included"`
	}
	parseJSON(r, &body)
	if body.DeliveryCostPerKg > 0 {
		store.SaveSetting("delivery_cost_per_kg", strconv.FormatFloat(body.DeliveryCostPerKg, 'f', 2, 64))
	}
	if body.USDtoCNY > 0 {
		store.SaveSetting("usd_to_cny", strconv.FormatFloat(body.USDtoCNY, 'f', 4, 64))
	}
	store.SaveSetting("delivery_included", strconv.FormatBool(body.DeliveryIncluded))
	jsonOK(w)
}

// ── SPA ──────────────────────────────────────────────────────────

func handleSPA(w http.ResponseWriter, r *http.Request) {
	distDir := "/opt/otapi-hub-src/frontend/dist"
	path := strings.TrimPrefix(r.URL.Path, "/otweb/app")
	if path == "" || path == "/" || !strings.Contains(path, ".") {
		http.ServeFile(w, r, distDir+"/index.html")
		return
	}
	http.ServeFile(w, r, distDir+path)
}
