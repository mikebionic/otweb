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
	Blacklisted  bool   `json:"blacklisted"`
}

// apiCategoryFeatures — GET: список CS-Cart характеристик, которые встречаются у товаров
// категории (через product_attrs → attr_cs_mapping), с числом товаров и отметкой чёрного списка.
func apiCategoryFeatures(w http.ResponseWriter, r *http.Request) {
	cat := mux.Vars(r)["cat"]
	if cat == "" {
		jsonErr(w, 400, "no category")
		return
	}

	black := make(map[int]bool)
	if rows, err := store.Hub.Query(`SELECT cs_feature_id FROM category_feature_blacklist WHERE category_id=?`, cat); err == nil {
		for rows.Next() {
			var fid int
			rows.Scan(&fid)
			black[fid] = true
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
		f.Blacklisted = black[f.FeatureID]
		features = append(features, f)
	}

	jsonData(w, map[string]interface{}{
		"category_id":      cat,
		"features":         features,
		"blacklist_count":  len(black),
	})
}

// apiToggleCategoryFeature — POST: добавить/убрать характеристику из чёрного списка категории.
// body: {"feature_id": 123, "blacklist": true|false}
func apiToggleCategoryFeature(w http.ResponseWriter, r *http.Request) {
	cat := mux.Vars(r)["cat"]
	var body struct {
		FeatureID int  `json:"feature_id"`
		Blacklist bool `json:"blacklist"`
	}
	if err := parseJSON(r, &body); err != nil || body.FeatureID <= 0 {
		jsonErr(w, 400, "invalid json")
		return
	}
	if body.Blacklist {
		store.Hub.Exec(`INSERT IGNORE INTO category_feature_blacklist (category_id, cs_feature_id) VALUES (?,?)`, cat, body.FeatureID)
		log.Printf("[cat-features] %s: характеристика #%d → чёрный список", cat, body.FeatureID)
	} else {
		store.Hub.Exec(`DELETE FROM category_feature_blacklist WHERE category_id=? AND cs_feature_id=?`, cat, body.FeatureID)
		log.Printf("[cat-features] %s: характеристика #%d возвращена", cat, body.FeatureID)
	}
	jsonOK(w)
}

// apiSuggestCategoryFeatures — POST: AI подсказывает, какие характеристики нужны покупателю
// в этой категории (остальные — кандидаты в чёрный список). Возвращает рекомендованные feature_id.
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
