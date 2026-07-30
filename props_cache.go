package main

import (
	"sync"
	"time"
)

// propItem — свойство (pid) с названием и числом товаров, для фильтра OTWeb.
type propItem struct {
	Pid   string `json:"pid"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// Кэш глобального списка свойств. Запрос тяжёлый (GROUP BY + COUNT(DISTINCT product_id)
// по product_attrs ~1.9M строк, ~19с) и вызывался фильтром внахлёст → клал mysqld и
// делал сайт недоступным (инцидент 23.07). Данные меняются медленно (только при синке),
// поэтому кэшируем на TTL; мьютекс сериализует параллельные вызовы, чтобы тяжёлый
// запрос выполнялся ТОЛЬКО ОДИН за раз, а остальные брали готовый результат.
var (
	propsCacheMu   sync.Mutex
	propsCacheData []propItem
	propsCacheAt   time.Time
)

// TTL 60 мин: список свойств меняется только при синке новых атрибутов, а сам
// запрос по мере роста product_attrs (2.5M+) стоит ~30с. При TTL 5 мин кэш протухал
// и пересобирался каждые ~5 мин — 30с тяжёлого скана до 12 раз/час нагружали общий
// mysqld (29.07). Час свежести для админ-фильтра достаточно.
const propsCacheTTL = 60 * time.Minute

// getGlobalProperties возвращает топ-100 свойств с кэшем на propsCacheTTL.
func getGlobalProperties() ([]propItem, error) {
	propsCacheMu.Lock()
	defer propsCacheMu.Unlock()
	if propsCacheData != nil && time.Since(propsCacheAt) < propsCacheTTL {
		return propsCacheData, nil
	}
	rows, err := store.Hub.Query(`
		SELECT pa.pid, COALESCE(NULLIF(at.property_name_ru,''), pa.property_name) as label, COUNT(DISTINCT pa.product_id) as cnt
		FROM product_attrs pa
		LEFT JOIN attr_translations at ON pa.pid=at.pid AND pa.vid=at.vid
		GROUP BY pa.pid, at.property_name_ru, pa.property_name
		ORDER BY cnt DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []propItem{}
	for rows.Next() {
		var it propItem
		rows.Scan(&it.Pid, &it.Label, &it.Count)
		result = append(result, it)
	}
	propsCacheData = result
	propsCacheAt = time.Now()
	return result, nil
}
