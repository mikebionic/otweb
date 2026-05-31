// translate-attrs — переводит все непереведённые атрибуты через DeepSeek.
// Запуск: go run ./cmd/translate-attrs/ (из корня проекта)
// Или: ./translate-attrs -dskey=<key> -dsurl=https://api.deepseek.com
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"otapi-hub/translate"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsKey := flag.String("dskey", "", "DeepSeek API key")
	dsURL := flag.String("dsurl", "https://api.deepseek.com", "DeepSeek base URL")
	dsn := flag.String("dsn", "otapi_hub:OtHub2026Pass@tcp(localhost:3306)/otapi_hub?charset=utf8mb4&parseTime=true", "MySQL DSN")
	batchSize := flag.Int("batch", 50, "batch size")
	flag.Parse()

	if *dsKey == "" {
		// Попробуем взять из БД
		db, err := sql.Open("mysql", *dsn)
		if err != nil {
			log.Fatal("db open:", err)
		}
		db.QueryRow(`SELECT value FROM settings WHERE key='deepseek_api_key'`).Scan(dsKey)
		db.Close()
	}
	if *dsKey == "" {
		log.Fatal("DeepSeek API key required (-dskey flag or deepseek_api_key in settings table)")
	}

	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		log.Fatal("db open:", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal("db ping:", err)
	}

	client := translate.NewDeepSeekClient(*dsKey, *dsURL)

	total := 0
	for {
		rows, err := db.Query(`
			SELECT DISTINCT a.pid, a.vid, a.property_name, a.value
			FROM product_attrs a
			LEFT JOIN attr_translations t ON t.pid=a.pid AND t.vid=a.vid
			WHERE t.pid IS NULL AND a.pid != '' AND a.vid != ''
			LIMIT ?`, *batchSize)
		if err != nil {
			log.Fatal("query:", err)
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

		log.Printf("Translating batch of %d pairs...", len(pairs))
		results, err := client.TranslateAttrs(pairs)
		if err != nil {
			log.Printf("TranslateAttrs error: %v — retrying in 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}

		pairMap := make(map[string]translate.AttrPair)
		for _, p := range pairs {
			pairMap[p.Pid+"\x00"+p.Vid] = p
		}

		for _, res := range results {
			p := pairMap[res.Pid+"\x00"+res.Vid]
			_, err := db.Exec(`
				INSERT INTO attr_translations (pid, vid, property_name_zh, property_name_ru, value_zh, value_ru, translated_at)
				VALUES (?,?,?,?,?,?,UNIX_TIMESTAMP())
				ON DUPLICATE KEY UPDATE property_name_ru=VALUES(property_name_ru), value_ru=VALUES(value_ru), translated_at=VALUES(translated_at)`,
				res.Pid, res.Vid, p.Name, res.NameRu, p.Value, res.ValueRu)
			if err != nil {
				log.Printf("save error pid=%s vid=%s: %v", res.Pid, res.Vid, err)
			} else {
				total++
			}
		}

		log.Printf("Saved %d so far", total)
		time.Sleep(300 * time.Millisecond)
	}

	var remaining int
	db.QueryRow(`SELECT COUNT(*) FROM product_attrs a LEFT JOIN attr_translations t ON t.pid=a.pid AND t.vid=a.vid WHERE t.pid IS NULL AND a.pid!=''`).Scan(&remaining)
	fmt.Printf("\nDone. Total saved: %d. Remaining untranslated: %d\n", total, remaining)
}
