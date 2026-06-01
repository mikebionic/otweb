// sync-cs-features — загружает все CS-Cart характеристики (features+variants) в локальный кеш.
// Запуск: go run ./cmd/sync-cs-features/ (из корня проекта)
// Результат: таблицы cs_features_cache + cs_feature_variants_cache заполнены.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"otapi-hub/cscart"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := flag.String("dsn", "otapi_hub:OtHub2026Pass@tcp(localhost:3306)/otapi_hub?charset=utf8mb4&parseTime=true", "MySQL DSN")
	csURL := flag.String("csurl", "", "CS-Cart base URL")
	csEmail := flag.String("csemail", "", "CS-Cart email")
	csKey := flag.String("cskey", "", "CS-Cart API key")
	flag.Parse()

	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		log.Fatal("db open:", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal("db ping:", err)
	}

	// Читаем настройки из БД если не заданы флагами
	getStr := func(key, fallback string) string {
		if fallback != "" {
			return fallback
		}
		var v string
		db.QueryRow(`SELECT value FROM settings WHERE key_name=?`, key).Scan(&v)
		return v
	}
	url := getStr("cscart_base_url", *csURL)
	email := getStr("cscart_email", *csEmail)
	key := getStr("cscart_api_key", *csKey)

	if url == "" || email == "" || key == "" {
		log.Fatal("CS-Cart credentials required (cscart_base_url, cscart_email, cscart_api_key in settings or flags)")
	}

	log.Printf("Connecting to CS-Cart: %s", url)
	client := cscart.NewClient(url, email, key)

	log.Println("Fetching features from CS-Cart...")
	features, err := client.GetAllFeatures()
	if err != nil {
		log.Fatal("GetAllFeatures:", err)
	}
	log.Printf("Got %d features from CS-Cart", len(features))

	// Сохраняем в кеш
	now := time.Now().Unix()
	tx, err := db.Begin()
	if err != nil {
		log.Fatal("begin tx:", err)
	}
	tx.Exec(`DELETE FROM cs_features_cache`)
	tx.Exec(`DELETE FROM cs_feature_variants_cache`)

	totalVariants := 0
	for _, f := range features {
		_, err := tx.Exec(`INSERT INTO cs_features_cache (feature_id, feature_name, feature_type, fetched_at) VALUES (?,?,?,?)`,
			f.FeatureID, f.Name, f.FeatureType, now)
		if err != nil {
			log.Printf("  WARN insert feature %d %q: %v", f.FeatureID, f.Name, err)
			continue
		}
		for _, v := range f.Variants {
			tx.Exec(`INSERT IGNORE INTO cs_feature_variants_cache (variant_id, feature_id, variant_value) VALUES (?,?,?)`,
				v.VariantID, f.FeatureID, v.Value)
			totalVariants++
		}
		log.Printf("  Feature %4d: %-30s  type=%-2s  variants=%d", f.FeatureID, f.Name, f.FeatureType, len(f.Variants))
	}

	if err := tx.Commit(); err != nil {
		log.Fatal("commit:", err)
	}

	log.Printf("\nДотуп: %d features, %d variants — сохранено в кеш.", len(features), totalVariants)

	// Выводим итоговый список для проверки
	fmt.Println("\n=== FEATURES IN CACHE ===")
	rows, _ := db.Query(`
		SELECT f.feature_id, f.feature_name, f.feature_type, COUNT(v.variant_id) as cnt
		FROM cs_features_cache f
		LEFT JOIN cs_feature_variants_cache v ON f.feature_id = v.feature_id
		GROUP BY f.feature_id, f.feature_name, f.feature_type
		ORDER BY f.feature_name`)
	defer rows.Close()
	for rows.Next() {
		var fid int
		var name, ftype string
		var cnt int
		rows.Scan(&fid, &name, &ftype, &cnt)
		fmt.Printf("  [%4d] %-30s type=%-2s  variants=%d\n", fid, name, ftype, cnt)
	}
}
