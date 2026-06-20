package sync

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"otapi-hub/db"
	"otapi-hub/otapi"
	"regexp"
	"strings"
	"time"
)

const detailTTL = 7 * 24 * 3600 // 7 дней: как часто обновляем полные данные товара

type Importer struct {
	store  *db.Store
	client *otapi.Client
}

func NewImporter(store *db.Store, client *otapi.Client) *Importer {
	return &Importer{store: store, client: client}
}

type ImportResult struct {
	CategoryID  string
	Processed   int
	Skipped     int
	Filtered    int // отбраковано пост-фильтром «мусор/запчасти» (импортировано, но enabled=0)
	Errors      int
	APIRequests int
	Log         []string
}

// junkTitleRe — пост-фильтр «мусор/запчасти». OTAPI отдаёт Title уже на RU (language=ru),
// поэтому матчим по русским словам (+ часть китайских). Совпавшие товары импортируются,
// но приходят выключенными (enabled=0) — оператор не публикует их случайно.
// Это «программный отсев» из договорённости: запчасти, инструменты, комплектующие,
// аксессуары-мелочёвка, наклейки/плёнки, образцы/пробники.
// Консервативно: ловим, только когда деталь/инструмент — ГЛАВНОЕ слово (название
// начинается с него) ИЛИ есть однозначная «деталь»-фраза. Это не трогает реальный товар
// в других категориях («Часы … с кожаным ремешком», «Браслет» в бижутерии, «Чехол» для телефона).
var junkTitleRe = regexp.MustCompile(`(?i)(^\s*(ремешок|ремешк|инструмент|отвёртк|отвертк|пинцет|циферблат)|для замены|ремешк\w* для часов|часовой механизм|механизм для|запасн\w+ част|запчаст|комплектующ|пробник|образец товара|配件|零件|工具|表带|机芯|贴膜)`)

// computeQualityScore — композитная оценка качества товара 0-100 по данным 1688.
// rating (0-5) 40% + goodRates/normRating (% положительных) 30% + спрос (payOrder30/totalSales) 30%.
// Чем больше данных, тем точнее; при отсутствии части метрик score меньше (консервативно).
func computeQualityScore(rating, goodRates, normRating float64, payOrder30, totalSales int) int {
	score := 0.0
	// Рейтинг товара/магазина (0-5) → 0-40
	if rating > 0 {
		if rating > 5 {
			rating = 5
		}
		score += (rating / 5.0) * 40
	} else if normRating > 0 { // запасной: нормализованный 0-1
		score += normRating * 40
	}
	// Доля положительных отзывов (0-100) → 0-30
	if goodRates > 0 {
		if goodRates > 100 {
			goodRates = 100
		}
		score += (goodRates / 100.0) * 30
	}
	// Спрос: оплаченные заказы за 30 дней (лог-шкала) → 0-20, + всего продаж → 0-10
	if payOrder30 > 0 {
		s := math.Log10(float64(payOrder30)+1) / 5.0 // 100k заказов ≈ максимум
		if s > 1 {
			s = 1
		}
		score += s * 20
	}
	if totalSales > 0 {
		s := math.Log10(float64(totalSales)+1) / 6.0 // 1M продаж ≈ максимум
		if s > 1 {
			s = 1
		}
		score += s * 10
	}
	if score > 100 {
		score = 100
	}
	return int(score + 0.5)
}

func (r *ImportResult) log(msg string) {
	r.Log = append(r.Log, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg))
	log.Println(msg)
}

// HasChinese возвращает true если строка содержит китайские иероглифы (CJK unified ideographs).
func HasChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// SyncCategories загружает категории из OTAPI и сохраняет в БД.
// Сохраняет все категории (включая китайские) - для работы синка они нужны.
// Перевод на русский делается отдельно через кнопку "Перевести категории".
// UI дропдауны фильтруют китайские названия отдельно.
func (imp *Importer) SyncCategories(cleanFirst bool) error {
	if cleanFirst {
		// Удаляем категории у которых нет товаров в БД (чтобы не потерять данные)
		imp.store.Hub.Exec(`DELETE FROM categories WHERE id NOT IN (SELECT DISTINCT category_id FROM products)`)
		log.Printf("[sync] cleaned categories without products")
	}

	cats, err := imp.client.GetCatalog()
	if err != nil {
		return fmt.Errorf("get catalog: %w", err)
	}
	total := 0
	for _, cat := range cats {
		provider := strings.ToLower(cat.ProviderType)
		imp.store.UpsertCategory(cat.ID, provider, cat.ExternalID, "", cat.Name, cat.Name, cat.IsParent)
		total++

		// Рекурсивно загружаем подкатегории (1 уровень вглубь)
		if cat.IsParent {
			subcats, err := imp.client.GetSubcategories(cat.ID)
			if err != nil {
				log.Printf("[sync] subcategories %s: %v", cat.ID, err)
				continue
			}
			for _, sub := range subcats {
				imp.store.UpsertCategory(sub.ID, provider, sub.ExternalID, cat.ID, sub.Name, sub.Name, sub.IsParent)
				total++
			}
		}
	}
	log.Printf("[sync] categories synced: %d (root: %d)", total, len(cats))
	return nil
}

// SyncProducts - двухфазный синк категории.
//
// Фаза 1 (дешево): SearchProducts по страницам.
//   - 1 запрос на 50 товаров
//   - Сохраняет базовые данные: цена, количество, название, главное фото
//   - Новые товары получают detail_fetched_at = NULL
//
// Фаза 2 (дорого, только когда нужно): GetProduct для товаров без полных данных.
//   - Вызывается только если detail_fetched_at IS NULL или старше 7 дней
//   - Сохраняет SKU, атрибуты, все фото
//
// При повторном синке категории с 500 товарами:
//   - Первый раз:   10 + 500 = 510 запросов
//   - Повторно:     10 запросов (все товары уже имеют свежие детали)
// SyncOptions - параметры синхронизации (фильтры для API).
type SyncOptions struct {
	// Основные
	MinVolume     int
	MinPrice      int
	MaxPrice      int
	MaxPriceLimit int    // Постфильтр аномалий (0=откл). Применяется ПОСЛЕ получения от API.
	ItemTitle     string
	OrderBy       string
	StuffStatus   string

	// Продавец
	MinVendorRating int
	MaxVendorRating int
	VendorName      string

	// Лот (только 1688)
	FirstLotMin int
	FirstLotMax int

	// Метод поиска
	SearchMethod string // "" = Default, "Official" = Tmall only

	// Features
	FeatureComplete bool
	FeatureDiscount bool
	FeatureTmall    bool

	// Прочее
	BrandName      string
	PropertySearch string

	JobID int64 // для real-time лога в БД
}

func (imp *Importer) SyncProducts(categoryID string, maxProducts int, opts SyncOptions, logCh chan<- string) *ImportResult {
	result := &ImportResult{CategoryID: categoryID}
	sendLog := func(msg string) {
		result.log(msg)
		// Real-time: пишем в БД сразу
		if opts.JobID > 0 {
			imp.store.Hub.Exec(`UPDATE sync_jobs SET log_text = CONCAT(IFNULL(log_text,''), ?, '\n') WHERE id=?`, msg, opts.JobID)
		}
		if logCh != nil {
			select {
			case logCh <- msg:
			default:
			}
		}
	}

	// Прогресс/ретраи OTAPI-клиента пишем в лог задачи.
	imp.client.LogFunc = sendLog
	defer func() { imp.client.LogFunc = nil }()

	provider := otapi.ProviderFromCategoryID(categoryID)
	sendLog(fmt.Sprintf("Синк категории %s (провайдер: %s, лимит: %d товаров)", categoryID, provider, maxProducts))
	sendLog("ПРОВЕРКИ ПЕРЕД ИМПОРТОМ: price > 0 (обязательно), price <= MaxPriceLimit (если задан)")
	if opts.MinVolume > 0 || opts.MinPrice > 0 || opts.MaxPrice > 0 || opts.ItemTitle != "" || opts.VendorName != "" || opts.BrandName != "" {
		sendLog(fmt.Sprintf("Фильтры API: MinVolume=%d, Price=%d-%d, Title=%q, Vendor=%q, Brand=%q",
			opts.MinVolume, opts.MinPrice, opts.MaxPrice, opts.ItemTitle, opts.VendorName, opts.BrandName))
	}
	if opts.MaxPriceLimit > 0 {
		sendLog(fmt.Sprintf("Фильтр на аномалии: MaxPriceLimit=%d CNY (товары дороже будут пропущены)", opts.MaxPriceLimit))
	} else {
		sendLog("Фильтр на аномалии: ОТКЛЮЧЕН (будут приняты товары с любой ценой, но > 0)")
	}

	// Фильтры для API
	filters := otapi.SearchFilters{
		MinVolume:       opts.MinVolume,
		MinPrice:        opts.MinPrice,
		MaxPrice:        opts.MaxPrice,
		ItemTitle:       opts.ItemTitle,
		VendorName:      opts.VendorName,
		BrandName:       opts.BrandName,
		PropertySearch:  opts.PropertySearch,
		OrderBy:         opts.OrderBy,
		StuffStatus:     opts.StuffStatus,
		MinVendorRating: opts.MinVendorRating,
		MaxVendorRating: opts.MaxVendorRating,
		FirstLotMin:     opts.FirstLotMin,
		FirstLotMax:     opts.FirstLotMax,
		SearchMethod:    opts.SearchMethod,
		FeatureComplete: opts.FeatureComplete,
		FeatureDiscount: opts.FeatureDiscount,
		FeatureTmall:    opts.FeatureTmall,
	}

	// --- Фаза 1: SearchProducts ---
	page := 1
	pageSize := 100 // frameSize=100 работает стабильно с MinVolume фильтром (~2.5 сек)
	totalFetched := 0

	for totalFetched < maxProducts {
		if pageSize > maxProducts-totalFetched {
			pageSize = maxProducts - totalFetched
		}

		sendLog(fmt.Sprintf("API: SearchProducts page=%d size=%d category=%s", page, pageSize, categoryID))
		resp, err := imp.client.SearchProducts(provider, categoryID, page, pageSize, filters)
		result.APIRequests++
		if err != nil {
			sendLog(fmt.Sprintf("ERROR search page %d: %v", page, err))
			result.Errors++
			break
		}

		total := resp.Result.Items.Items.TotalCount
		items := resp.Result.Items.Items.Content
		if page == 1 {
			sendLog(fmt.Sprintf("Всего в OT: %d товаров (с фильтрами)", total))
		}
		if len(items) == 0 {
			break
		}

		for _, item := range items {
			// Пропускаем товары с нулевым остатком
			if item.MasterQuantity <= 0 {
				result.Skipped++
				continue
			}

			// === КРИТИЧНАЯ ПРОВЕРКА: Цена товара ===
			// Fallback: если OriginalPrice = 0, пробуем MarginPrice
			price := item.Price.OriginalPrice
			if price == 0 && item.Price.MarginPrice > 0 {
				price = item.Price.MarginPrice
			}

			// SKIP: товары без цены вообще НЕ сохраняются в БД
			if price == 0 {
				log.Printf("[sync] SKIP: item %s (%s) has ZERO PRICE (no fallback), cannot import", item.ID, item.Title)
				result.Skipped++
				continue
			}

			// Проверка на аномальные цены (фильтр на outliers)
			if opts.MaxPriceLimit > 0 {
				if price > float64(opts.MaxPriceLimit) {
					log.Printf("[sync] WARN: item %s (%s) price %.2f CNY exceeds limit %d, skipping (outlier)", item.ID, item.Title, price, opts.MaxPriceLimit)
					result.Skipped++
					continue
				}
			}

			imp.upsertBasic(provider, categoryID, item)

			// Пост-фильтр «мусор/запчасти»: совпавшие импортируются, но выключены (enabled=0).
			if junkTitleRe.MatchString(item.Title) {
				imp.store.Hub.Exec(`UPDATE products SET enabled=0 WHERE otapi_id=?`, item.ID)
				result.Filtered++
				sendLog(fmt.Sprintf("  ⚠ отбраковано (мусор/запчасть, выключено): %s", item.Title))
			}

			totalFetched++
			if totalFetched >= maxProducts {
				break
			}
		}

		if page >= resp.Result.Items.MaximumPageCount {
			break
		}
		page++
		time.Sleep(200 * time.Millisecond)
	}

	sendLog(fmt.Sprintf("Фаза 1: %d товаров из SearchProducts (%d API запросов)", totalFetched, result.APIRequests))
	if result.Filtered > 0 {
		sendLog(fmt.Sprintf("Пост-фильтр: отбраковано %d (мусор/запчасти) — импортированы выключенными, см. фильтр «На ревью»", result.Filtered))
	}

	// --- Фаза 2: GetProduct для новых и устаревших ---
	staleThreshold := time.Now().Unix() - detailTTL
	rows, err := imp.store.Hub.Query(`
		SELECT id, otapi_id FROM products
		WHERE category_id = ?
		  AND (detail_fetched_at IS NULL OR detail_fetched_at < ?)
		ORDER BY id ASC
		LIMIT ?`,
		categoryID, staleThreshold, maxProducts)

	if err != nil {
		sendLog(fmt.Sprintf("ERROR query stale products: %v", err))
		return result
	}
	defer rows.Close()

	type staleItem struct {
		id      int64
		otapiID string
	}
	var staleItems []staleItem
	for rows.Next() {
		var s staleItem
		rows.Scan(&s.id, &s.otapiID)
		staleItems = append(staleItems, s)
	}
	rows.Close()

	sendLog(fmt.Sprintf("Фаза 2: %d товаров требуют полного GetProduct", len(staleItems)))

	for i, s := range staleItems {
		sendLog(fmt.Sprintf("  [%d/%d] API: GetItemFullInfo id=%s", i+1, len(staleItems), s.otapiID))
		if err := imp.fetchDetails(provider, s.id, s.otapiID, result, sendLog); err != nil {
			result.Errors++
		} else {
			result.Processed++
		}
		result.APIRequests++
		time.Sleep(100 * time.Millisecond)
	}

	sendLog(fmt.Sprintf("Готово: %d полных, %d ошибок, %d API запросов", result.Processed, result.Errors, result.APIRequests))
	return result
}

// SyncPricesOnly - только цены через SearchProducts.
// Стоимость: ceil(N/50) запросов. Детальные данные не затрагиваются.
func (imp *Importer) SyncPricesOnly(categoryID string) (updated int, apiReqs int, err error) {
	provider := otapi.ProviderFromCategoryID(categoryID)
	page := 1

	for {
		resp, reqErr := imp.client.SearchProducts(provider, categoryID, page, 20)
		apiReqs++
		if reqErr != nil {
			return updated, apiReqs, fmt.Errorf("search page %d: %w", page, reqErr)
		}

		items := resp.Result.Items.Items.Content
		if len(items) == 0 {
			break
		}

		for _, item := range items {
			// Fallback: если OriginalPrice = 0, пробуем MarginPrice
			price := item.Price.OriginalPrice
			if price == 0 && item.Price.MarginPrice > 0 {
				price = item.Price.MarginPrice
				log.Printf("[importer] WARN: item %s has OriginalPrice=0, using MarginPrice=%.2f", item.ID, price)
			}
			if price == 0 {
				log.Printf("[importer] WARN: item %s (%s) has ZERO PRICE, skipping", item.ID, item.Title)
				continue
			}

			priceTMT := imp.store.CalculatePriceTMT(price, categoryID, item.ID)
			// Обновляем цены только для enabled товаров
			res, upErr := imp.store.Hub.Exec(`
				UPDATE products SET price_cny=?, price_tmt=?, updated_at=?
				WHERE otapi_id=? AND provider=? AND enabled=1`,
				price, priceTMT, time.Now().Unix(),
				item.ID, strings.ToLower(item.ProviderType))
			if upErr == nil {
				if n, _ := res.RowsAffected(); n > 0 {
					updated++
				}
			}
		}

		if page >= resp.Result.Items.MaximumPageCount {
			break
		}
		page++
		time.Sleep(200 * time.Millisecond)
	}
	return updated, apiReqs, nil
}

// upsertBasic сохраняет все доступные данные товара из SearchProducts без вызова GetProduct.
// ТРЕБОВАНИЕ: вызывающий код должен убедиться что price > 0 (даже после fallback на MarginPrice)
// raw_json содержит полный JSON ответа для этого товара.
func (imp *Importer) upsertBasic(provider, categoryID string, item otapi.SearchItem) {
	// Fallback: если OriginalPrice = 0, пробуем MarginPrice
	price := item.Price.OriginalPrice
	if price == 0 && item.Price.MarginPrice > 0 {
		price = item.Price.MarginPrice
		log.Printf("[importer] INFO: item %s OriginalPrice=0, using MarginPrice=%.2f", item.ID, price)
	}
	// SAFETY CHECK: не должно быть товаров с price=0 (должны быть отсеяны раньше)
	if price == 0 {
		log.Printf("[importer] ERROR: item %s (%s) still has ZERO PRICE at upsertBasic, BUG in caller!", item.ID, item.Title)
		return // не сохраняем
	}

	priceTMT := imp.store.CalculatePriceTMT(price, categoryID, item.ID)
	now := time.Now().Unix()

	// Вес по категории (если задан Азатом в маппинге)
	weightKg := 0.0
	if weightG := imp.store.GetCategoryWeightG(categoryID); weightG > 0 {
		weightKg = float64(weightG) / 1000.0
	}

	isFakeQty, isExpired, isTmall := false, false, false
	for _, f := range item.Features {
		switch f {
		case "FakeQuantity":
			isFakeQty = true
		case "Expired":
			isExpired = true
		case "Tmall":
			isTmall = true
		}
	}

	// Извлекаем данные по продажам и КАЧЕСТВУ из FeaturedValues.
	// Богатые метрики, которых нет в фильтрах запроса, но они приходят в ответе:
	// goodRates (% положительных), rating (0-5), normalizedRating (0-1), payOrder30Day (оплаченных заказов/30д).
	var totalSales, salesLast30, favCount, payOrder30 int
	var rating, goodRates, normRating float64
	for _, fv := range item.FeaturedValues {
		switch fv.Name {
		case "TotalSales":
			fmt.Sscanf(fv.Value, "%d", &totalSales)
		case "SalesInLast30Days":
			fmt.Sscanf(fv.Value, "%d", &salesLast30)
		case "favCount":
			fmt.Sscanf(fv.Value, "%d", &favCount)
		case "payOrder30Day":
			fmt.Sscanf(fv.Value, "%d", &payOrder30)
		case "rating":
			fmt.Sscanf(fv.Value, "%f", &rating)
		case "goodRates":
			fmt.Sscanf(fv.Value, "%f", &goodRates)
		case "normalizedRating":
			fmt.Sscanf(fv.Value, "%f", &normRating)
		}
	}
	// Композитный score качества 0-100 (для сортировки/ревью): рейтинг 40% + отзывы 30% + спрос 30%.
	qualityScore := computeQualityScore(rating, goodRates, normRating, payOrder30, totalSales)

	rawJSON, _ := json.Marshal(item)

	var locCity, locState, locCityRu, locStateRu string
	if item.Location != nil {
		locCity = item.Location.City
		locState = item.Location.State
		locCityRu = TranslateCity(locCity)
		locStateRu = TranslateState(locState)
	}

	_, err := imp.store.Hub.Exec(`
		INSERT INTO products
		  (otapi_id, provider, category_id, external_category_id,
		   vendor_id, vendor_name, vendor_name_original, vendor_score,
		   brand_id, brand_name, brand_name_original,
		   location_city, location_city_ru, location_state, location_state_ru,
		   title_original, title_ru, title_en,
		   price_cny, price_tmt,
		   weight_kg,
		   master_quantity, is_fake_quantity, is_sell_allowed, is_expired, is_tmall,
		   stuff_status, main_image_url, platform_url,
		   volume_sales, sales_last_30days, fav_count, has_hierarchical_conf,
		   rating, good_rates, normalized_rating, pay_order_30day, quality_score,
		   raw_json, fetched_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		  id=LAST_INSERT_ID(id),
		  external_category_id=VALUES(external_category_id),
		  vendor_score=VALUES(vendor_score),
		  location_city=VALUES(location_city), location_city_ru=VALUES(location_city_ru),
		  location_state=VALUES(location_state), location_state_ru=VALUES(location_state_ru),
		  price_cny=VALUES(price_cny), price_tmt=VALUES(price_tmt),
		  weight_kg=IF(weight_kg=0, VALUES(weight_kg), weight_kg),
		  master_quantity=VALUES(master_quantity),
		  is_fake_quantity=VALUES(is_fake_quantity),
		  is_sell_allowed=VALUES(is_sell_allowed),
		  is_expired=VALUES(is_expired),
		  stuff_status=VALUES(stuff_status),
		  main_image_url=VALUES(main_image_url),
		  volume_sales=GREATEST(volume_sales, VALUES(volume_sales)),
		  sales_last_30days=VALUES(sales_last_30days),
		  fav_count=GREATEST(fav_count, VALUES(fav_count)),
		  rating=VALUES(rating), good_rates=VALUES(good_rates),
		  normalized_rating=VALUES(normalized_rating), pay_order_30day=VALUES(pay_order_30day),
		  quality_score=VALUES(quality_score),
		  raw_json=IF(detail_fetched_at IS NULL, VALUES(raw_json), raw_json),
		  updated_at=VALUES(updated_at)`,
		item.ID, provider, categoryID, item.ExternalCategory,
		item.VendorID, item.VendorName, item.VendorID, item.VendorScore,
		item.BrandID, item.BrandName, item.BrandID,
		locCity, locCityRu, locState, locStateRu,
		item.OriginalTitle, item.Title, item.Title,
		price, priceTMT,
		weightKg,
		item.MasterQuantity, isFakeQty, item.IsSellAllowed, isExpired, isTmall,
		item.StuffStatus, item.MainPictureURL, item.TaobaoItemURL,
		totalSales, salesLast30, favCount,
		rating, goodRates, normRating, payOrder30, qualityScore,
		string(rawJSON), now, now,
	)
	if err != nil {
		log.Printf("[sync] upsertBasic %s ERROR: %v", item.ID, err)
	}
}

// fetchDetails вызывает GetProduct и сохраняет SKU, атрибуты, все фото.
func (imp *Importer) fetchDetails(provider string, productDBID int64, otapiID string, result *ImportResult, logFn func(string)) error {
	product, err := imp.client.GetProduct(provider, otapiID)
	if err != nil {
		logFn(fmt.Sprintf("  ERROR GetProduct %s: %v", otapiID, err))
		return err
	}

	rawJSON, _ := json.Marshal(product)

	isFakeQty, isExpired, isTmall := false, false, false
	for _, f := range product.Features {
		switch f {
		case "FakeQuantity":
			isFakeQty = true
		case "Expired":
			isExpired = true
		case "Tmall":
			isTmall = true
		}
	}

	var weightKg float64
	if product.PhysicalParameters != nil {
		weightKg = product.PhysicalParameters.Weight
	}

	var locCity, locState, locCityRu, locStateRu string
	if product.Location != nil {
		locCity = product.Location.City
		locState = product.Location.State
		locCityRu = TranslateCity(locCity)
		locStateRu = TranslateState(locState)
	}

	// Извлекаем реальные данные продаж из FeaturedValues
	var totalSales, salesLast30, favCount, reviewsCount int
	for _, fv := range product.FeaturedValues {
		switch fv.Name {
		case "TotalSales":
			fmt.Sscanf(fv.Value, "%d", &totalSales)
		case "SalesInLast30Days":
			fmt.Sscanf(fv.Value, "%d", &salesLast30)
		case "favCount":
			fmt.Sscanf(fv.Value, "%d", &favCount)
		case "reviews":
			fmt.Sscanf(fv.Value, "%d", &reviewsCount)
		}
	}

	now := time.Now().Unix()
	imp.store.Hub.Exec(`
		UPDATE products SET
		  title_original=?,
		  vendor_id=?, vendor_name=?, vendor_score=?,
		  brand_id=?, brand_name=?,
		  location_city=?, location_city_ru=?, location_state=?, location_state_ru=?,
		  description_html=?,
		  volume_sales=?, sales_last_30days=?, fav_count=?, reviews_count=?,
		  weight_kg=?,
		  is_fake_quantity=?, is_expired=?, is_tmall=?,
		  has_hierarchical_conf=?,
		  raw_json=?, detail_fetched_at=?, updated_at=?
		WHERE id=?`,
		product.OriginalTitle,
		product.VendorID, product.VendorDisplayName, product.VendorScore,
		product.BrandID, product.BrandName,
		locCity, locCityRu, locState, locStateRu,
		product.Description,
		totalSales, salesLast30, favCount, reviewsCount,
		weightKg,
		isFakeQty, isExpired, isTmall,
		product.HasHierarchicalConf,
		string(rawJSON), now, now,
		productDBID,
	)

	// Если цена на уровне товара = 0, выводим из SKU (типично для ювелирки/обуви)
	if product.Price.OriginalPrice == 0 && product.Price.MarginPrice == 0 {
		var minSKUPrice float64
		for _, sku := range product.ConfiguredItems {
			if sku.Price.OriginalPrice > 0 && (minSKUPrice == 0 || sku.Price.OriginalPrice < minSKUPrice) {
				minSKUPrice = sku.Price.OriginalPrice
			}
		}
		if minSKUPrice > 0 {
			var catID string
			imp.store.Hub.QueryRow(`SELECT category_id FROM products WHERE id=?`, productDBID).Scan(&catID)
			priceTMT := imp.store.CalculatePriceTMT(minSKUPrice, catID, otapiID)
			imp.store.Hub.Exec(`UPDATE products SET price_cny=?, price_tmt=? WHERE id=? AND (price_cny=0 OR price_cny IS NULL)`,
				minSKUPrice, priceTMT, productDBID)
			log.Printf("[importer] INFO: product %d price derived from min SKU: %.2f CNY -> %.2f TMT", productDBID, minSKUPrice, priceTMT)
		} else {
			log.Printf("[importer] WARN: product %d has ZERO price and no valid SKU prices", productDBID)
		}
	}

	for _, sku := range product.ConfiguredItems {
		skuPrice := sku.Price.OriginalPrice
		if skuPrice == 0 {
			log.Printf("[importer] WARN: product %d SKU %s has ZERO PRICE", productDBID, sku.ID)
		}
		imp.store.UpsertSKU(productDBID, sku.ID, sku.Quantity, skuPrice, sku.Configurators)
	}

	imp.store.Hub.Exec(`DELETE FROM product_images WHERE product_id=?`, productDBID)
	for i, pic := range product.Pictures {
		imp.store.InsertImage(productDBID, pic.URL, pic.Small.URL, pic.Medium.URL, pic.Large.URL, pic.IsMain, i)
	}

	imp.store.Hub.Exec(`DELETE FROM product_attrs WHERE product_id=?`, productDBID)
	for _, attr := range product.Attributes {
		imp.store.InsertAttr(productDBID, attr.Pid, attr.Vid, attr.PropertyName, attr.Value, attr.IsConfigurator, attr.ImageURL)
	}

	// Пол и возрастная группа из атрибутов (для фильтрации и роутинга по подкатегориям)
	gender, ageGroup := NormalizeGenderAge(product.Attributes)
	imp.store.Hub.Exec(`UPDATE products SET gender=?, age_group=? WHERE id=?`, gender, ageGroup, productDBID)

	return nil
}
