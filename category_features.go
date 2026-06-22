package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// catFeature — характеристика, встречающаяся у товаров категории.
type catFeature struct {
	FeatureID    int    `json:"feature_id"`
	Name         string `json:"name"`
	ProductCount int    `json:"product_count"`
	InWhitelist  bool   `json:"in_whitelist"`
}

// apiCategoryFeatures — GET: список CS-Cart характеристик, которые встречаются у товаров
// категории (через product_attrs → attr_cs_mapping), с числом товаров и отметкой whitelist.
func apiCategoryFeatures(w http.ResponseWriter, r *http.Request) {
	cat := mux.Vars(r)["cat"]
	if cat == "" {
		jsonErr(w, 400, "no category")
		return
	}

	// текущий whitelist категории
	wl := make(map[int]bool)
	if rows, err := store.Hub.Query(`SELECT cs_feature_id FROM category_feature_whitelist WHERE category_id=?`, cat); err == nil {
		for rows.Next() {
			var fid int
			rows.Scan(&fid)
			wl[fid] = true
		}
		rows.Close()
	}

	rows, err := store.Hub.Query(`
		SELECT m.cs_feature_id, COALESCE(f.feature_name, CONCAT('feature #', m.cs_feature_id)) AS name,
		       COUNT(DISTINCT pa.product_id) AS cnt
		FROM product_attrs pa
		JOIN products p ON p.id = pa.product_id
		JOIN attr_cs_mapping m ON pa.pid = m.pid AND pa.vid = m.vid AND m.cs_feature_id > 0
		LEFT JOIN cs_features_cache f ON f.feature_id = m.cs_feature_id
		WHERE p.category_id = ? AND pa.is_configurator = 0
		GROUP BY m.cs_feature_id, name
		ORDER BY cnt DESC`, cat)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	var features []catFeature
	for rows.Next() {
		var f catFeature
		rows.Scan(&f.FeatureID, &f.Name, &f.ProductCount)
		f.InWhitelist = wl[f.FeatureID]
		features = append(features, f)
	}

	jsonData(w, map[string]interface{}{
		"category_id":      cat,
		"features":         features,
		"whitelist_active": len(wl) > 0,
	})
}

// apiSaveCategoryFeatures — POST: сохранить whitelist характеристик категории.
func apiSaveCategoryFeatures(w http.ResponseWriter, r *http.Request) {
	cat := mux.Vars(r)["cat"]
	var body struct {
		FeatureIDs []int `json:"feature_ids"`
	}
	if err := parseJSON(r, &body); err != nil {
		jsonErr(w, 400, "invalid json")
		return
	}
	tx, err := store.Hub.Begin()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	tx.Exec(`DELETE FROM category_feature_whitelist WHERE category_id=?`, cat)
	for _, fid := range body.FeatureIDs {
		if fid > 0 {
			tx.Exec(`INSERT IGNORE INTO category_feature_whitelist (category_id, cs_feature_id) VALUES (?,?)`, cat, fid)
		}
	}
	if err := tx.Commit(); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	log.Printf("[cat-features] категория %s: whitelist сохранён (%d характеристик)", cat, len(body.FeatureIDs))
	jsonOK(w)
}

// apiSuggestCategoryFeatures — POST: AI предлагает, какие характеристики нужны покупателю
// в этой категории. Возвращает рекомендованные feature_id.
func apiSuggestCategoryFeatures(w http.ResponseWriter, r *http.Request) {
	cat := mux.Vars(r)["cat"]
	if cfg.DeepSeek.APIKey == "" {
		jsonErr(w, 400, "DeepSeek API key not configured")
		return
	}

	var catName string
	store.Hub.QueryRow(`SELECT name_ru FROM categories WHERE id=?`, cat).Scan(&catName)

	type fe struct {
		ID   int
		Name string
	}
	var feats []fe
	rows, err := store.Hub.Query(`
		SELECT DISTINCT m.cs_feature_id, COALESCE(f.feature_name, CONCAT('feature #', m.cs_feature_id))
		FROM product_attrs pa
		JOIN products p ON p.id = pa.product_id
		JOIN attr_cs_mapping m ON pa.pid = m.pid AND pa.vid = m.vid AND m.cs_feature_id > 0
		LEFT JOIN cs_features_cache f ON f.feature_id = m.cs_feature_id
		WHERE p.category_id = ? AND pa.is_configurator = 0`, cat)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	for rows.Next() {
		var f fe
		rows.Scan(&f.ID, &f.Name)
		feats = append(feats, f)
	}
	rows.Close()
	if len(feats) == 0 {
		jsonData(w, map[string]interface{}{"suggested": []int{}})
		return
	}

	var sb strings.Builder
	for _, f := range feats {
		sb.WriteString(strings.TrimSpace(f.Name))
		sb.WriteString(" | ")
	}
	prompt := `Ты помогаешь оформить карточку товара в интернет-магазине.
Категория товаров: "` + catName + `".
Вот список характеристик (через "|"), которые приходят от поставщика:
` + sb.String() + `
Отбери ТОЛЬКО те характеристики, которые реально полезны и важны покупателю в этой категории
(остальное — мусор/служебное/нерелевантное). Верни СТРОГО JSON без пояснений:
{"useful": ["точное имя характеристики", ...]}`

	dsClient := newDSClient()
	resp, err := dsClient.RawChat(prompt)
	if err != nil {
		jsonErr(w, 502, "deepseek: "+err.Error())
		return
	}
	resp = strings.TrimSpace(resp)
	if i := strings.Index(resp, "{"); i > 0 {
		resp = resp[i:]
	}
	if i := strings.LastIndex(resp, "}"); i >= 0 {
		resp = resp[:i+1]
	}
	var parsed struct {
		Useful []string `json:"useful"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		jsonErr(w, 502, "deepseek bad json")
		return
	}
	usefulSet := make(map[string]bool)
	for _, u := range parsed.Useful {
		usefulSet[strings.ToLower(strings.TrimSpace(u))] = true
	}
	var suggested []int
	for _, f := range feats {
		if usefulSet[strings.ToLower(strings.TrimSpace(f.Name))] {
			suggested = append(suggested, f.ID)
		}
	}
	jsonData(w, map[string]interface{}{"suggested": suggested})
}

// apiCleanupCategoryAttrs — POST: удаляет из БД (product_attrs) характеристики товаров
// категории, которых НЕТ в whitelist. Чистит уже импортированные лишние атрибуты.
func apiCleanupCategoryAttrs(w http.ResponseWriter, r *http.Request) {
	cat := mux.Vars(r)["cat"]

	var wlCount int
	store.Hub.QueryRow(`SELECT COUNT(*) FROM category_feature_whitelist WHERE category_id=?`, cat).Scan(&wlCount)
	if wlCount == 0 {
		jsonErr(w, 400, "whitelist для категории пуст — нечего чистить (сначала утвердите характеристики)")
		return
	}

	// удаляем product_attrs товаров категории, чьи фичи (через attr_cs_mapping) НЕ в whitelist
	res, err := store.Hub.Exec(`
		DELETE pa FROM product_attrs pa
		JOIN products p ON p.id = pa.product_id
		JOIN attr_cs_mapping m ON pa.pid = m.pid AND pa.vid = m.vid AND m.cs_feature_id > 0
		WHERE p.category_id = ? AND pa.is_configurator = 0
		  AND m.cs_feature_id NOT IN (SELECT cs_feature_id FROM category_feature_whitelist WHERE category_id = ?)`,
		cat, cat)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	log.Printf("[cat-features] категория %s: удалено %d лишних атрибутов из product_attrs", cat, n)
	jsonData(w, map[string]interface{}{"deleted": n})
}
