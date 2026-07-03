package sync

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"otapi-hub/translate"
)

// attrPipelineRunning — защита от параллельных прогонов (напр. синк во время бэкфилла).
var attrPipelineRunning int32

// TranslateAndMapPendingAttrs — автоматическая обработка атрибутов после синка:
//  1. переводит непереведённые атрибуты 1688 → attr_translations (RU),
//  2. умно маппит переведённые атрибуты в характеристики CS-Cart → attr_cs_mapping (DeepSeek).
//
// Идемпотентно: обрабатывает только то, для чего ещё нет перевода/маппинга — поэтому
// безопасно вызывать после каждого синка и не перезатирает ручные правки.
// Раньше это делалось вручную (кнопка «перевод» + CLI-утилиты). Теперь — автоматом.
func (imp *Importer) TranslateAndMapPendingAttrs(dsKey, dsURL string) {
	if dsKey == "" {
		log.Printf("[attr-pipeline] DeepSeek key пуст — пропуск")
		return
	}
	if dsURL == "" {
		dsURL = "https://api.deepseek.com"
	}
	// Только один прогон одновременно (идемпотентность защищает данные, но экономим вызовы).
	if !atomic.CompareAndSwapInt32(&attrPipelineRunning, 0, 1) {
		log.Printf("[attr-pipeline] уже выполняется — пропуск")
		return
	}
	defer atomic.StoreInt32(&attrPipelineRunning, 0)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[attr-pipeline] panic recovered: %v", r)
		}
	}()
	// Сначала маппим уже переведённые (быстрый эффект для характеристик CS-Cart),
	// затем переводим новые — их маппинг подхватится следующим прогоном (после синка).
	n2 := imp.mapPendingAttrs(dsKey, dsURL)
	n1 := imp.translatePendingAttrs(dsKey, dsURL)
	log.Printf("[attr-pipeline] готово: замаплено %d, переведено %d атрибутов", n2, n1)
}

// translatePendingAttrs переводит непереведённые (pid,vid) атрибуты батчами по 50.
func (imp *Importer) translatePendingAttrs(dsKey, dsURL string) int {
	client := translate.NewDeepSeekClient(dsKey, dsURL)
	total := 0
	for {
		rows, err := imp.store.Hub.Query(`
			SELECT DISTINCT a.pid, a.vid, a.property_name, a.value
			FROM product_attrs a
			LEFT JOIN attr_translations t ON t.pid=a.pid AND t.vid=a.vid
			WHERE t.pid IS NULL AND a.pid != '' AND a.vid != ''
			LIMIT 50`)
		if err != nil {
			log.Printf("[attr-pipeline] translate query: %v", err)
			return total
		}
		var pairs []translate.AttrPair
		for rows.Next() {
			var p translate.AttrPair
			rows.Scan(&p.Pid, &p.Vid, &p.Name, &p.Value)
			pairs = append(pairs, p)
		}
		rows.Close()
		if len(pairs) == 0 {
			break
		}
		results, err := client.TranslateAttrs(pairs)
		if err != nil {
			log.Printf("[attr-pipeline] TranslateAttrs: %v", err)
			return total
		}
		pairMap := make(map[string]translate.AttrPair, len(pairs))
		for _, p := range pairs {
			pairMap[p.Pid+"\x00"+p.Vid] = p
		}
		for _, res := range results {
			p := pairMap[res.Pid+"\x00"+res.Vid]
			if _, err := imp.store.Hub.Exec(`
				INSERT INTO attr_translations (pid, vid, property_name_zh, property_name_ru, value_zh, value_ru, translated_at)
				VALUES (?,?,?,?,?,?,UNIX_TIMESTAMP())
				ON DUPLICATE KEY UPDATE property_name_ru=VALUES(property_name_ru), value_ru=VALUES(value_ru), translated_at=VALUES(translated_at)`,
				res.Pid, res.Vid, p.Name, res.NameRu, p.Value, res.ValueRu); err == nil {
				total++
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return total
}

// ── Умный маппинг атрибут → CS-Cart характеристика (адаптировано из cmd/map-attrs-to-cs) ──

type csFeature struct {
	FeatureID   int
	FeatureName string
	FeatureType string
	Variants    []csVariant
}

type csVariant struct {
	VariantID int
	Value     string
}

type attrPair struct {
	Pid, Vid, NameRu, ValueRu string
}

type mappingResult struct {
	ID             int    `json:"id"`
	CSFeatureID    int    `json:"cs_feature_id"`
	CSVariantID    int    `json:"cs_variant_id"`
	CanonicalValue string `json:"canonical_value"`
}

// mapPendingAttrs маппит переведённые, но ещё не замапленные атрибуты через DeepSeek.
func (imp *Importer) mapPendingAttrs(dsKey, dsURL string) int {
	features, err := loadCSFeatures(imp.store.Hub)
	if err != nil || len(features) == 0 {
		log.Printf("[attr-pipeline] loadCSFeatures: %v (features=%d)", err, len(features))
		return 0
	}
	featureRef := buildFeatureRef(features)

	pairs, err := loadUnmappedAttrs(imp.store.Hub)
	if err != nil {
		log.Printf("[attr-pipeline] loadUnmappedAttrs: %v", err)
		return 0
	}
	if len(pairs) == 0 {
		return 0
	}

	httpClient := &http.Client{Timeout: 90 * time.Second}
	saved := 0
	const batchSize = 50
	for start := 0; start < len(pairs); start += batchSize {
		end := start + batchSize
		if end > len(pairs) {
			end = len(pairs)
		}
		batch := pairs[start:end]
		results, err := mapBatch(httpClient, dsKey, dsURL, batch, featureRef)
		if err != nil {
			log.Printf("[attr-pipeline] mapBatch %d-%d: %v — пропуск", start, end, err)
			continue
		}
		now := time.Now().Unix()
		for _, r := range results {
			if r.ID < 0 || r.ID >= len(batch) {
				continue
			}
			p := batch[r.ID]
			// cs_feature_id=0 → null-маппинг (чтобы не переобрабатывать)
			imp.store.Hub.Exec(`INSERT IGNORE INTO attr_cs_mapping (pid, vid, cs_feature_id, cs_variant_id, canonical_value, mapped_at) VALUES (?,?,?,?,?,?)`,
				p.Pid, p.Vid, r.CSFeatureID, r.CSVariantID, r.CanonicalValue, now)
			if r.CSFeatureID > 0 {
				saved++
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return saved
}

func loadCSFeatures(db *sql.DB) ([]csFeature, error) {
	rows, err := db.Query(`SELECT feature_id, feature_name, feature_type FROM cs_features_cache ORDER BY feature_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var features []csFeature
	for rows.Next() {
		var f csFeature
		if err := rows.Scan(&f.FeatureID, &f.FeatureName, &f.FeatureType); err != nil {
			return nil, err
		}
		features = append(features, f)
	}
	for i := range features {
		vrows, err := db.Query(`SELECT variant_id, variant_value FROM cs_feature_variants_cache WHERE feature_id=? ORDER BY variant_id`, features[i].FeatureID)
		if err != nil {
			continue
		}
		for vrows.Next() {
			var v csVariant
			vrows.Scan(&v.VariantID, &v.Value)
			features[i].Variants = append(features[i].Variants, v)
		}
		vrows.Close()
	}
	return features, nil
}

func buildFeatureRef(features []csFeature) string {
	var sb strings.Builder
	sb.WriteString("CS-CART ХАРАКТЕРИСТИКИ (feature_id: название [variant_id: значение, ...]):\n")
	for _, f := range features {
		if len(f.Variants) == 0 {
			sb.WriteString(fmt.Sprintf("  %d: %s (тип: %s, вариантов нет — свободный текст)\n", f.FeatureID, f.FeatureName, f.FeatureType))
		} else {
			sb.WriteString(fmt.Sprintf("  %d: %s — варианты: ", f.FeatureID, f.FeatureName))
			parts := make([]string, len(f.Variants))
			for i, v := range f.Variants {
				parts[i] = fmt.Sprintf("[%d: %s]", v.VariantID, v.Value)
			}
			sb.WriteString(strings.Join(parts, ", "))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func loadUnmappedAttrs(db *sql.DB) ([]attrPair, error) {
	rows, err := db.Query(`
		SELECT at.pid, at.vid, at.property_name_ru, at.value_ru
		FROM attr_translations at
		WHERE at.property_name_ru != '' AND at.value_ru != ''
		  AND NOT EXISTS (SELECT 1 FROM attr_cs_mapping m WHERE m.pid=at.pid AND m.vid=at.vid)
		ORDER BY at.pid, at.vid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pairs []attrPair
	for rows.Next() {
		var p attrPair
		if err := rows.Scan(&p.Pid, &p.Vid, &p.NameRu, &p.ValueRu); err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
	}
	return pairs, nil
}

func mapBatch(client *http.Client, apiKey, baseURL string, batch []attrPair, featureRef string) ([]mappingResult, error) {
	type inputItem struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	items := make([]inputItem, len(batch))
	for i, p := range batch {
		items[i] = inputItem{ID: i, Name: p.NameRu, Value: p.ValueRu}
	}
	inputJSON, _ := json.MarshalIndent(items, "", "  ")

	prompt := featureRef + `

ЗАДАЧА: Для каждого атрибута товара найди наиболее подходящую CS-Cart характеристику и вариант.

Правила:
1. Ищи совпадение по смыслу (синонимы, транслитерация, аббревиатуры). Напр. «Цвет: жёлтый» → характеристика «Цвет», вариант «Жёлтый».
2. Если нашёл подходящий вариант — верни cs_feature_id и cs_variant_id из списка выше.
3. Если характеристика с открытым текстом (вариантов нет) — cs_variant_id=0, canonical_value=value.
4. Если атрибут не подходит ни к одной характеристике CS-Cart — cs_feature_id=0, cs_variant_id=0.
5. Возвращай ТОЛЬКО JSON без пояснений.

АТРИБУТЫ ТОВАРА:
` + string(inputJSON) + `

Ответ ТОЛЬКО в формате:
{"results":[{"id":0,"cs_feature_id":567,"cs_variant_id":1234,"canonical_value":"Жёлтый"},...]}

Если нет совпадения: {"id":N,"cs_feature_id":0,"cs_variant_id":0,"canonical_value":""}`

	reqBody := map[string]interface{}{
		"model": "deepseek-v4-flash",
		"messages": []map[string]interface{}{
			{"role": "system", "content": "Ты — система маппинга характеристик товаров. Возвращай только JSON без объяснений."},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.0,
	}
	data, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/chat/completions", strings.TrimRight(baseURL, "/"))
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil || len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	var out struct {
		Results []mappingResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(apiResp.Choices[0].Message.Content), &out); err != nil {
		return nil, fmt.Errorf("parse results: %w", err)
	}
	return out.Results, nil
}
