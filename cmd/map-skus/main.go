// map-skus — нормализует значения вариаций (цвет/размер) для пуша в CS-Cart.
// Берёт уникальные значения из product_attrs (is_configurator=1),
// переводит через DeepSeek (цвета) или нормализует программно (размеры),
// сохраняет в sku_option_mapping.
// Запуск: go run ./cmd/map-skus/ [-dry] [-colors] [-sizes]
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
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := flag.String("dsn", "otapi_hub:OtHub2026Pass@tcp(localhost:3306)/otapi_hub?charset=utf8mb4&parseTime=true", "MySQL DSN")
	doColors := flag.Bool("colors", true, "обрабатывать цвета")
	doSizes := flag.Bool("sizes", true, "обрабатывать размеры")
	dry := flag.Bool("dry", false, "не сохранять")
	flag.Parse()

	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		log.Fatal("db open:", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal("db ping:", err)
	}

	var dsKey, dsURL string
	db.QueryRow(`SELECT value FROM settings WHERE key_name='deepseek_api_key'`).Scan(&dsKey)
	db.QueryRow(`SELECT value FROM settings WHERE key_name='deepseek_base_url'`).Scan(&dsURL)
	if dsKey == "" {
		log.Fatal("deepseek_api_key не задан")
	}
	if dsURL == "" {
		dsURL = "https://api.deepseek.com"
	}

	client := &http.Client{Timeout: 60 * time.Second}

	if *doColors {
		processColors(db, client, dsKey, dsURL, *dry)
	}
	if *doSizes {
		processSizes(db, *dry)
	}
}

// --- COLORS ---

func processColors(db *sql.DB, client *http.Client, dsKey, dsURL string, dry bool) {
	// Собираем уникальные сырые значения цветов которых ещё нет в маппинге
	rows, err := db.Query(`
		SELECT DISTINCT pa.value
		FROM product_attrs pa
		WHERE pa.is_configurator=1
		  AND pa.property_name IN ('Цвет','Color','颜色','Классификация цветов')
		  AND NOT EXISTS (
		    SELECT 1 FROM sku_option_mapping m
		    WHERE m.raw_value=pa.value AND m.option_type='color'
		  )
		ORDER BY pa.value`)
	if err != nil {
		log.Fatal("query colors:", err)
	}
	defer rows.Close()

	var rawColors []string
	for rows.Next() {
		var v string
		rows.Scan(&v)
		rawColors = append(rawColors, v)
	}
	log.Printf("Цветов без маппинга: %d", len(rawColors))

	if len(rawColors) == 0 {
		log.Println("Все цвета уже замаппированы.")
		return
	}

	// Переводим через DeepSeek батчами по 50
	batchSize := 50
	now := time.Now().Unix()
	saved, skipped := 0, 0

	for start := 0; start < len(rawColors); start += batchSize {
		end := start + batchSize
		if end > len(rawColors) {
			end = len(rawColors)
		}
		batch := rawColors[start:end]
		log.Printf("Батч цветов %d-%d / %d...", start+1, end, len(rawColors))

		translated, err := translateColors(client, dsKey, dsURL, batch)
		if err != nil {
			log.Printf("  WARN: %v", err)
			skipped += len(batch)
			continue
		}

		for i, raw := range batch {
			canonical := translated[i]
			if canonical == "" {
				canonical = raw
			}
			log.Printf("  %q → %q", raw, canonical)
			if dry {
				continue
			}
			db.Exec(`INSERT IGNORE INTO sku_option_mapping (raw_value, option_type, canonical_value, cs_variant_id, mapped_at)
				VALUES (?,?,?,0,?)`, raw, "color", canonical, now)
			saved++
		}
		time.Sleep(300 * time.Millisecond)
	}

	log.Printf("Цвета: сохранено=%d пропущено=%d", saved, skipped)
}

func translateColors(client *http.Client, apiKey, baseURL string, colors []string) ([]string, error) {
	type item struct {
		ID    int    `json:"id"`
		Value string `json:"v"`
	}
	items := make([]item, len(colors))
	for i, c := range colors {
		items[i] = item{ID: i, Value: c}
	}
	inputJSON, _ := json.Marshal(items)

	prompt := `Переведи каждое название цвета на русский язык.
Правила:
- Используй короткие нормальные названия: "Чёрный", "Белый", "Серый", "Красный", "Синий" и т.д.
- Для составных: "Тёмно-синий", "Светло-серый", "Тёмно-серый"
- Латинские буквы/цифры в скобках убери: "黑色-FG" → "Чёрный"
- Если уже по-русски или латиница — оставь как есть
- Возвращай ТОЛЬКО JSON: {"t":[{"id":0,"ru":"..."},...]}

Цвета:
` + string(inputJSON)

	reqBody := map[string]interface{}{
		"model": "deepseek-v4-flash",
		"messages": []map[string]interface{}{
			{"role": "system", "content": "Ты переводчик цветов. Возвращай только JSON {\"t\":[{\"id\":N,\"ru\":\"...\"}]}."},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.0,
	}

	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil || len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("parse: %w", err)
	}

	content := apiResp.Choices[0].Message.Content
	var result struct {
		T []struct {
			ID int    `json:"id"`
			Ru string `json:"ru"`
		} `json:"t"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse t: %w (%.200s)", err, content)
	}

	out := make([]string, len(colors))
	for _, r := range result.T {
		if r.ID >= 0 && r.ID < len(out) {
			out[r.ID] = r.Ru
		}
	}
	return out, nil
}

// --- SIZES ---

var (
	reChineseAnnotation = regexp.MustCompile(`[（(][^）)]*[）)]`)
	reChineseChars      = regexp.MustCompile(`[\p{Han}]+`)
	reSizeClean         = regexp.MustCompile(`\s+`)
)

// normalizeSize очищает размер от китайских аннотаций и нормализует написание.
func normalizeSize(raw string) string {
	s := raw

	// Убираем китайские аннотации в скобках: "L（90斤~120斤）" → "L"
	s = reChineseAnnotation.ReplaceAllString(s, "")
	// Убираем остатки китайских символов
	s = reChineseChars.ReplaceAllString(s, "")
	// Нормализуем пробелы
	s = reSizeClean.ReplaceAllString(strings.TrimSpace(s), "")

	// Нормализуем числовые суффиксы: 2XL → XXL, 3XL → XXXL, 4XL → XXXXL
	s = strings.ReplaceAll(s, "2XL", "XXL")
	s = strings.ReplaceAll(s, "3XL", "XXXL")
	s = strings.ReplaceAll(s, "4XL", "XXXXL")
	s = strings.ReplaceAll(s, "5XL", "XXXXXL")

	// Убираем лишние слэши с размерами вроде "XL/115/175"
	if idx := strings.Index(s, "/"); idx > 0 {
		part := s[:idx]
		if isStandardSize(part) {
			s = part
		}
	}

	s = strings.ToUpper(strings.TrimSpace(s))

	if s == "" {
		return raw // не смогли нормализовать — оставляем как есть
	}
	return s
}

func isStandardSize(s string) bool {
	standard := map[string]bool{
		"XS": true, "S": true, "M": true, "L": true,
		"XL": true, "XXL": true, "XXXL": true, "XXXXL": true, "XXXXXL": true,
		"2XL": true, "3XL": true, "4XL": true, "5XL": true,
	}
	return standard[strings.ToUpper(s)]
}

func processSizes(db *sql.DB, dry bool) {
	rows, err := db.Query(`
		SELECT DISTINCT pa.value
		FROM product_attrs pa
		WHERE pa.is_configurator=1
		  AND pa.property_name IN ('Размер','Size','尺码')
		  AND NOT EXISTS (
		    SELECT 1 FROM sku_option_mapping m
		    WHERE m.raw_value=pa.value AND m.option_type='size'
		  )
		ORDER BY pa.value`)
	if err != nil {
		log.Fatal("query sizes:", err)
	}
	defer rows.Close()

	var rawSizes []string
	for rows.Next() {
		var v string
		rows.Scan(&v)
		rawSizes = append(rawSizes, v)
	}
	log.Printf("Размеров без маппинга: %d", len(rawSizes))

	now := time.Now().Unix()
	saved := 0

	for _, raw := range rawSizes {
		canonical := normalizeSize(raw)
		log.Printf("  %q → %q", raw, canonical)
		if dry {
			continue
		}
		db.Exec(`INSERT IGNORE INTO sku_option_mapping (raw_value, option_type, canonical_value, cs_variant_id, mapped_at)
			VALUES (?,?,?,0,?)`, raw, "size", canonical, now)
		saved++
	}

	log.Printf("Размеры: сохранено=%d", saved)
}
