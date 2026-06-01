// map-attrs-to-cs — сопоставляет переведённые атрибуты товаров с характеристиками CS-Cart.
// Читает attr_translations, берёт cs_features_cache как эталон,
// отправляет батчи в DeepSeek и сохраняет маппинг в attr_cs_mapping.
// Запуск: go run ./cmd/map-attrs-to-cs/ [-dry] [-batch 50] [-limit 0]
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type attrPair struct {
	Pid     string
	Vid     string
	NameRu  string
	ValueRu string
}

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

type mappingResult struct {
	ID            int    `json:"id"`
	CSFeatureID   int    `json:"cs_feature_id"`
	CSVariantID   int    `json:"cs_variant_id"`
	CanonicalValue string `json:"canonical_value"`
}

func main() {
	dsn := flag.String("dsn", "otapi_hub:OtHub2026Pass@tcp(localhost:3306)/otapi_hub?charset=utf8mb4&parseTime=true", "MySQL DSN")
	batchSize := flag.Int("batch", 50, "атрибутов за один DeepSeek запрос")
	limit := flag.Int("limit", 0, "максимум атрибутов для обработки (0 = все)")
	dry := flag.Bool("dry", false, "не сохранять, только вывести результат")
	flag.Parse()

	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		log.Fatal("db open:", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal("db ping:", err)
	}

	// Читаем API ключ DeepSeek из настроек
	var dsKey, dsURL string
	db.QueryRow(`SELECT value FROM settings WHERE key_name='deepseek_api_key'`).Scan(&dsKey)
	db.QueryRow(`SELECT value FROM settings WHERE key_name='deepseek_base_url'`).Scan(&dsURL)
	if dsKey == "" {
		log.Fatal("deepseek_api_key не задан в settings")
	}
	if dsURL == "" {
		dsURL = "https://api.deepseek.com"
	}

	// Загружаем CS-Cart характеристики из кеша
	features, err := loadCSFeatures(db)
	if err != nil {
		log.Fatal("loadCSFeatures:", err)
	}
	log.Printf("Загружено %d CS-Cart характеристик из кеша", len(features))

	// Строим краткую справку для промпта
	featureRef := buildFeatureRef(features)

	// Загружаем непереведённые атрибуты (не в attr_cs_mapping)
	pairs, err := loadUnmappedAttrs(db, *limit)
	if err != nil {
		log.Fatal("loadUnmappedAttrs:", err)
	}
	log.Printf("Атрибутов без маппинга: %d", len(pairs))

	if len(pairs) == 0 {
		log.Println("Нечего обрабатывать.")
		return
	}

	// Обрабатываем батчами
	httpClient := &http.Client{Timeout: 90 * time.Second}
	total, saved, skipped := 0, 0, 0

	for start := 0; start < len(pairs); start += *batchSize {
		end := start + *batchSize
		if end > len(pairs) {
			end = len(pairs)
		}
		batch := pairs[start:end]

		log.Printf("Батч %d-%d / %d ...", start+1, end, len(pairs))

		results, err := mapBatch(httpClient, dsKey, dsURL, batch, featureRef)
		if err != nil {
			log.Printf("  WARN батч %d-%d: %v — пропускаем", start+1, end, err)
			skipped += len(batch)
			continue
		}

		if *dry {
			for _, r := range results {
				p := batch[r.ID]
				log.Printf("  DRY  [%s/%s] %q=%q → feature=%d variant=%d %q",
					p.Pid, p.Vid, p.NameRu, p.ValueRu, r.CSFeatureID, r.CSVariantID, r.CanonicalValue)
			}
			total += len(batch)
			continue
		}

		// Сохраняем результаты
		now := time.Now().Unix()
		for _, r := range results {
			p := batch[r.ID]
			if r.CSFeatureID == 0 {
				// DeepSeek не нашёл совпадения — пишем null-маппинг чтобы не переобрабатывать
				_, err := db.Exec(`INSERT IGNORE INTO attr_cs_mapping (pid, vid, cs_feature_id, cs_variant_id, canonical_value, mapped_at) VALUES (?,?,0,0,'',?)`,
					p.Pid, p.Vid, now)
				if err != nil {
					log.Printf("  WARN save null-mapping %s/%s: %v", p.Pid, p.Vid, err)
				}
				skipped++
				continue
			}
			_, err := db.Exec(`INSERT IGNORE INTO attr_cs_mapping (pid, vid, cs_feature_id, cs_variant_id, canonical_value, mapped_at) VALUES (?,?,?,?,?,?)`,
				p.Pid, p.Vid, r.CSFeatureID, r.CSVariantID, r.CanonicalValue, now)
			if err != nil {
				log.Printf("  WARN save mapping %s/%s: %v", p.Pid, p.Vid, err)
				skipped++
			} else {
				log.Printf("  OK   [%s/%s] %q=%q → feature=%d variant=%d %q",
					p.Pid, p.Vid, p.NameRu, p.ValueRu, r.CSFeatureID, r.CSVariantID, r.CanonicalValue)
				saved++
			}
		}
		total += len(batch)

		// Небольшая пауза между запросами
		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("\nГотово: обработано=%d сохранено=%d пропущено=%d", total, saved, skipped)

	// Итоговая статистика по маппингам
	var mappedCount, nullCount int
	db.QueryRow(`SELECT COUNT(*) FROM attr_cs_mapping WHERE cs_feature_id > 0`).Scan(&mappedCount)
	db.QueryRow(`SELECT COUNT(*) FROM attr_cs_mapping WHERE cs_feature_id = 0`).Scan(&nullCount)
	log.Printf("attr_cs_mapping итого: %d с маппингом, %d без совпадения", mappedCount, nullCount)
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

	// Загружаем варианты
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

func loadUnmappedAttrs(db *sql.DB, limit int) ([]attrPair, error) {
	q := `
		SELECT at.pid, at.vid, at.property_name_ru, at.value_ru
		FROM attr_translations at
		WHERE at.property_name_ru != ''
		  AND at.value_ru != ''
		  AND NOT EXISTS (
		    SELECT 1 FROM attr_cs_mapping m WHERE m.pid=at.pid AND m.vid=at.vid
		  )
		ORDER BY at.pid, at.vid`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.Query(q)
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
1. Ищи совпадение по смыслу (синонимы, транслитерация, аббревиатуры).
2. Если нашёл подходящий вариант — верни cs_feature_id и cs_variant_id из списка выше.
3. Если характеристика с открытым текстом (вариантов нет) — cs_variant_id=0, canonical_value=value.
4. Если атрибут не подходит ни к одной характеристике CS-Cart — cs_feature_id=0, cs_variant_id=0.
5. Возвращай ТОЛЬКО JSON без пояснений.

АТРИБУТЫ ТОВАРА:
` + string(inputJSON) + `

Ответ ТОЛЬКО в формате:
{"results":[{"id":0,"cs_feature_id":567,"cs_variant_id":1234,"canonical_value":"Красный"},...]}

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
		return nil, fmt.Errorf("parse response: %w (body: %.300s)", err, string(body))
	}

	content := apiResp.Choices[0].Message.Content
	log.Printf("  DeepSeek raw (first 500): %.500s", content)

	var out struct {
		Results []mappingResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("parse results: %w (content: %.300s)", err, content)
	}

	return out.Results, nil
}
