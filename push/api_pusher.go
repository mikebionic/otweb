// APIPusher - push товаров из Hub БД в CS-Cart через REST API.
// Заменяет legacy pusher.go (прямая запись в БД).
// Полный flow: DeepSeek нормализация -> CreateProduct -> CreateOption -> UpdateProductFeatures.
package push

import (
	"encoding/json"
	"fmt"
	"log"
	"otapi-hub/cscart"
	"otapi-hub/db"
	"otapi-hub/translate"
	"regexp"
	"sort"
	"strings"
	"time"
)

type PushResult struct {
	Pushed int
	Errors int
	Log    []string
}

func (r *PushResult) logMsg(msg string) {
	r.Log = append(r.Log, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg))
	log.Println(msg)
}

// Маппинг DeepSeek полей -> CS-Cart feature_id (из wabrum.com/api/features)
var featureMap = map[string]int{
	"Цвет":                567,
	"Ткань":                563,
	"Модель":               574,
	"Высота талии":         575,
	"Штанина":              573,
	"Длина":                570,
	"Толщина":              571,
	"Повод":                566,
	"Подкладка":            565,
	"Капюшон":              562,
	"Страна производства":  578,
}

type APIPusher struct {
	store     *db.Store
	csClient  *cscart.Client
	dsClient  *translate.DeepSeekClient
	companyID int
}

func NewAPIPusher(store *db.Store, csClient *cscart.Client, dsClient *translate.DeepSeekClient, companyID int) *APIPusher {
	return &APIPusher{store: store, csClient: csClient, dsClient: dsClient, companyID: companyID}
}

// PushCategoryAuto - push товаров категории с автоматическим маппингом.
// Берёт CS-Cart category_id из таблицы category_map.
// Если маппинга нет - возвращает ошибку.
func (p *APIPusher) PushCategoryAuto(categoryOT string) *PushResult {
	var categoryCS int
	err := p.store.Hub.QueryRow(`SELECT cs_category_id FROM category_map WHERE otapi_category_id = ?`, categoryOT).Scan(&categoryCS)
	if err != nil || categoryCS == 0 {
		res := &PushResult{}
		res.logMsg(fmt.Sprintf("ERROR: нет маппинга для OT категории %s. Добавьте в category_map.", categoryOT))
		return res
	}
	return p.PushCategory(categoryOT, categoryCS)
}

// PushCategory - push всех непушеных товаров категории в CS-Cart (status=D Hidden).
// Для каждого товара: 1) DeepSeek 2) CreateProduct 3) CreateOption 4) UpdateFeatures.
// Среднее время: ~12 сек/товар (DeepSeek ~3 сек + CS-Cart скачка фото ~5 сек + пауза 2 сек).
func (p *APIPusher) PushCategory(categoryOT string, categoryCS int) *PushResult {
	res := &PushResult{}

	rows, err := p.store.Hub.Query(`
		SELECT p.id, p.otapi_id, p.title_ru, p.title_original,
		       p.price_tmt, p.master_quantity, p.weight_kg,
		       IFNULL(p.description_html,''), IFNULL(p.main_image_url,'')
		FROM products p
		WHERE p.category_id = ? AND p.is_sell_allowed = 1 AND p.is_expired = 0 AND p.master_quantity > 0
		  AND p.cs_product_id IS NULL
		ORDER BY p.id ASC`, categoryOT)
	if err != nil {
		res.logMsg(fmt.Sprintf("ERROR query: %v", err))
		return res
	}
	defer rows.Close()

	type row struct {
		ID          int64
		OtapiID     string
		TitleRu     string
		TitleOrig   string
		PriceTMT    float64
		Quantity    int
		Weight      float64
		Description string
		MainImage   string
	}

	var products []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.OtapiID, &r.TitleRu, &r.TitleOrig,
			&r.PriceTMT, &r.Quantity, &r.Weight, &r.Description, &r.MainImage); err != nil {
			res.logMsg(fmt.Sprintf("ERROR scan: %v", err))
			continue
		}
		products = append(products, r)
	}
	rows.Close()

	res.logMsg(fmt.Sprintf("Категория %s -> CS %d: %d товаров для push", categoryOT, categoryCS, len(products)))

	for i, pr := range products {
		res.logMsg(fmt.Sprintf("[%d/%d] %s (%.0f TMT)...", i+1, len(products), pr.OtapiID, pr.PriceTMT))
		csID, err := p.PushSingleProduct(pr.ID, categoryCS)
		if err != nil {
			res.logMsg(fmt.Sprintf("  ERROR: %v", err))
			res.Errors++
			continue
		}
		res.logMsg(fmt.Sprintf("  OK -> cs_product_id=%d", csID))
		res.Pushed++

		time.Sleep(2 * time.Second)
	}

	res.logMsg(fmt.Sprintf("Готово: %d pushed, %d errors", res.Pushed, res.Errors))
	return res
}

// PushSingleProduct - push одного товара в CS-Cart (status=D Hidden).
// Возвращает cs_product_id или ошибку.
func (p *APIPusher) PushSingleProduct(hubProductID int64, categoryCS int) (int, error) {
	var otapiID, titleRu, titleOrig, description, mainImage string
	var priceTMT float64
	var qty int
	var weight float64
	err := p.store.Hub.QueryRow(`
		SELECT otapi_id, title_ru, title_original, price_tmt, master_quantity,
		       weight_kg, IFNULL(description_html,''), IFNULL(main_image_url,'')
		FROM products WHERE id=? AND enabled=1`, hubProductID).Scan(
		&otapiID, &titleRu, &titleOrig, &priceTMT, &qty, &weight, &description, &mainImage)
	if err != nil {
		return 0, fmt.Errorf("product %d not found or disabled", hubProductID)
	}

	title := titleRu
	if title == "" {
		title = titleOrig
	}

	// DeepSeek: перевод на 3 языка + нормализация характеристик (graceful)
	var normalized *translate.NormalizeOutput
	if p.dsClient != nil {
		normalized = p.normalize(hubProductID, titleRu, titleOrig)
		if normalized != nil {
			// Обновляем переводы в hub БД
			if normalized.TitleRU != "" {
				title = normalized.TitleRU
				p.store.Hub.Exec(`UPDATE products SET title_ru=?, title_en=?, title_tk=?, translate_status='deepseek' WHERE id=?`,
					normalized.TitleRU, normalized.TitleEN, normalized.TitleTK, hubProductID)
			} else if normalized.Title != "" {
				title = normalized.Title // backward compat
			}
		}
	}

	// Убираем img теги из описания (изображения идут отдельно)
	cleanDesc := regexp.MustCompile(`<img[^>]*>`).ReplaceAllString(description, "")
	cleanDesc = strings.TrimSpace(cleanDesc)

	addImages := p.getAdditionalImages(hubProductID)

	// Извлекаем ВСЕ изображения из description_html и добавляем к доп. фото
	descImgs := regexp.MustCompile(`src="(https?://[^"]+)"`).FindAllStringSubmatch(description, -1)
	for _, m := range descImgs {
		if len(m) > 1 && !strings.Contains(m[1], "spaceball") && !strings.Contains(m[1], "display:none") {
			addImages = append(addImages, m[1])
		}
	}

	input := cscart.NewProductInput(title, categoryCS, p.companyID, priceTMT, qty, otapiID, cleanDesc, weight, mainImage, addImages)

	csID, err := p.csClient.CreateProduct(input)
	if err != nil {
		return 0, err
	}

	p.store.Hub.Exec(`UPDATE products SET cs_product_id=?, pushed_to_cs_at=? WHERE id=?`, csID, time.Now().Unix(), hubProductID)

	// Создаём опции и комбинации (вариации с остатками)
	sizeOptID, sizeVariants := p.pushSizeOption(hubProductID, csID)
	colorOptID, colorVariants := p.pushColorOption(hubProductID, csID)
	if sizeOptID > 0 || colorOptID > 0 {
		p.pushCombinations(hubProductID, csID, sizeOptID, sizeVariants, colorOptID, colorVariants)
	}

	p.pushFeatures(csID, normalized)

	return csID, nil
}

// PushProducts - push нескольких товаров по ID. Возвращает результат.
func (p *APIPusher) PushProducts(productIDs []int64, categoryCS int) *PushResult {
	res := &PushResult{}
	for i, id := range productIDs {
		res.logMsg(fmt.Sprintf("[%d/%d] product_id=%d...", i+1, len(productIDs), id))
		csID, err := p.PushSingleProduct(id, categoryCS)
		if err != nil {
			res.logMsg(fmt.Sprintf("  ERROR: %v", err))
			res.Errors++
			continue
		}
		res.logMsg(fmt.Sprintf("  OK -> cs_product_id=%d", csID))
		res.Pushed++
		time.Sleep(2 * time.Second)
	}
	res.logMsg(fmt.Sprintf("Готово: %d pushed, %d errors", res.Pushed, res.Errors))
	return res
}

// normalize - отправляет данные товара в DeepSeek для нормализации.
// Собирает атрибуты и цвета из Hub БД, формирует промпт Азата.
// Если DeepSeek недоступен - возвращает nil (graceful degradation).
func (p *APIPusher) normalize(hubProductID int64, titleRu, titleOrig string) *translate.NormalizeOutput {
	if p.dsClient == nil {
		return nil
	}

	rows, err := p.store.Hub.Query(`
		SELECT property_name, value FROM product_attrs
		WHERE product_id = ? AND is_configurator = 0`, hubProductID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	attrs := make(map[string]string)
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		attrs[k] = v
	}

	rows2, err := p.store.Hub.Query(`
		SELECT DISTINCT value FROM product_attrs
		WHERE product_id = ? AND is_configurator = 1 AND property_name != 'Размер'`, hubProductID)
	if err != nil {
		return nil
	}
	defer rows2.Close()

	var colors []string
	for rows2.Next() {
		var v string
		rows2.Scan(&v)
		colors = append(colors, v)
	}

	result, err := p.dsClient.Normalize(translate.NormalizeInput{
		TitleRu:       titleRu,
		TitleOriginal: titleOrig,
		Attributes:    attrs,
		Colors:        colors,
	})
	if err != nil {
		log.Printf("[push] DeepSeek error: %v", err)
		return nil
	}
	return result
}

// pushFeatures - записывает нормализованные характеристики в CS-Cart.
// Для каждого значения DeepSeek (Цвет, Ткань, Модель...) находит variant_id
// в CS-Cart features и отправляет PUT /api/products/{id} с product_features.
func (p *APIPusher) pushFeatures(csProductID int, n *translate.NormalizeOutput) {
	if n == nil {
		return
	}

	features := map[string]string{
		"Цвет":               n.Color,
		"Ткань":               n.Fabric,
		"Модель":              n.Model,
		"Высота талии":        n.WaistHeight,
		"Штанина":             n.LegType,
		"Длина":               n.Length,
		"Толщина":             n.Thickness,
		"Повод":               n.Occasion,
		"Подкладка":           n.Lining,
		"Капюшон":             n.Hood,
		"Страна производства": n.Country,
	}

	resolved := make(map[int]string)
	for name, value := range features {
		if value == "" {
			continue
		}
		fid, ok := featureMap[name]
		if !ok {
			continue
		}
		variantID, found := p.csClient.ResolveFeatureVariant(fid, value)
		if !found {
			log.Printf("[push] feature %q=%q: variant not found in CS-Cart (fid=%d)", name, value, fid)
			continue
		}
		resolved[fid] = variantID
	}

	if len(resolved) == 0 {
		return
	}

	err := p.csClient.UpdateProductFeatures(csProductID, resolved)
	if err != nil {
		log.Printf("[push] features error cs_product=%d: %v", csProductID, err)
	} else {
		log.Printf("[push] features: %d set for cs_product=%d", len(resolved), csProductID)
	}
}

func (p *APIPusher) getAdditionalImages(hubProductID int64) []string {
	rows, err := p.store.Hub.Query(`
		SELECT url FROM product_images
		WHERE product_id = ? AND is_main = 0 ORDER BY position ASC`, hubProductID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var u string
		rows.Scan(&u)
		if u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}

// pushSizeOption - создаёт опцию "Размер" с вариантами для товара в CS-Cart.
// Читает is_configurator=1 атрибуты из Hub БД, нормализует через NormalizeSizes,
// вызывает POST /api/options с вариантами (S, M, L, XL...).
// pushColorOption - создаёт опцию "Цвет" если у товара есть цветовые вариации.
// pushColorOption создаёт опцию Цвет и возвращает optionID + map[rawColor]->variantID.
func (p *APIPusher) pushColorOption(hubProductID int64, csProductID int) (int, map[string]string) {
	rows, err := p.store.Hub.Query(`
		SELECT value FROM product_attrs
		WHERE product_id = ? AND is_configurator = 1 AND property_name IN ('Цвет','Классификация цветов','Color')
		ORDER BY vid`, hubProductID)
	if err != nil {
		return 0, nil
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var colors []string
	for rows.Next() {
		var v string
		rows.Scan(&v)
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			colors = append(colors, v)
		}
	}
	if len(colors) == 0 {
		return 0, nil
	}

	optionID, err := p.csClient.CreateOption(csProductID, "Цвет", colors)
	if err != nil {
		log.Printf("[push] ERROR color option: %v", err)
		return 0, nil
	}
	variantMap := p.getOptionVariants(optionID)
	log.Printf("[push] Цвет id=%d (%d variants) for cs=%d", optionID, len(variantMap), csProductID)

	rawToVariant := make(map[string]string)
	for _, c := range colors {
		// Ищем по точному совпадению
		for vname, vid := range variantMap {
			if vname == c {
				rawToVariant[strings.Trim(c, "[]")] = vid
				break
			}
		}
	}
	return optionID, rawToVariant
}

// getOptionVariants загружает variant_name -> variant_id для опции из CS-Cart.
func (p *APIPusher) getOptionVariants(optionID int) map[string]string {
	body, status, _ := p.csClient.Do("GET", fmt.Sprintf("options/%d", optionID), nil)
	if status != 200 {
		return nil
	}
	var resp struct {
		Variants map[string]struct {
			VariantName string `json:"variant_name"`
		} `json:"variants"`
	}
	json.Unmarshal(body, &resp)
	result := make(map[string]string)
	for vid, v := range resp.Variants {
		result[v.VariantName] = vid
	}
	return result
}

// pushCombinations записывает SKU комбинации (размер+цвет) с остатками в CS-Cart.
func (p *APIPusher) pushCombinations(hubProductID int64, csProductID int,
	sizeOptID int, sizeVariants map[string]string,
	colorOptID int, colorVariants map[string]string) {

	rows, err := p.store.Hub.Query(`SELECT sku_id, quantity, configurators FROM product_skus WHERE product_id=?`, hubProductID)
	if err != nil {
		return
	}
	defer rows.Close()

	type combo struct {
		skuID       string
		qty         int
		combination string
		hash        uint32
	}

	var combos []combo
	for rows.Next() {
		var skuID, confsJSON string
		var qty int
		rows.Scan(&skuID, &qty, &confsJSON)

		var confs []struct{ Pid, Vid string }
		json.Unmarshal([]byte(confsJSON), &confs)

		var sizeVID, colorVID string
		for _, c := range confs {
			pid := c.Pid
			vid := c.Vid

			// Определяем тип по Pid (может быть на CN или RU)
			isSize := pid == "尺码" || pid == "Размер" || pid == "Size" || pid == "码数"
			isColor := pid == "颜色" || pid == "Цвет" || pid == "Color" || pid == "Классификация цветов"

			if isSize && sizeOptID > 0 {
				normalized := cscart.NormalizeSize(vid)
				if v, ok := sizeVariants[vid]; ok {
					sizeVID = v
				} else if v, ok := sizeVariants[normalized]; ok {
					sizeVID = v
				}
			}
			if isColor && colorOptID > 0 {
				cleaned := strings.Trim(vid, "[]")
				if v, ok := colorVariants[cleaned]; ok {
					colorVID = v
				} else if v, ok := colorVariants[vid]; ok {
					colorVID = v
				}
			}
		}

		// Строим combination string
		var parts []string
		if sizeOptID > 0 && sizeVID != "" {
			parts = append(parts, fmt.Sprintf("%d_%s", sizeOptID, sizeVID))
		}
		if colorOptID > 0 && colorVID != "" {
			parts = append(parts, fmt.Sprintf("%d_%s", colorOptID, colorVID))
		}
		if len(parts) == 0 {
			continue
		}

		sort.Strings(parts)
		combStr := strings.Join(parts, "_")
		h := crc32Hash(combStr)
		combos = append(combos, combo{skuID: skuID, qty: qty, combination: combStr, hash: h})
	}

	if len(combos) == 0 {
		return
	}

	// Batch insert через прямую запись (Combinations API не существует в CS-Cart)
	for _, c := range combos {
		p.store.Mirror.Exec(`INSERT INTO cscart_product_options_inventory
			(product_id, product_code, combination_hash, combination, amount, temp, position)
			VALUES (?, ?, ?, ?, ?, 'N', 0)
			ON DUPLICATE KEY UPDATE amount=VALUES(amount)`,
			csProductID, c.skuID, c.hash, c.combination, c.qty)
	}

	// Включаем tracking по опциям
	p.store.Mirror.Exec(`UPDATE cscart_products SET tracking='O' WHERE product_id=?`, csProductID)

	log.Printf("[push] %d combinations inserted for cs=%d", len(combos), csProductID)
}

func crc32Hash(s string) uint32 {
	h := uint32(0)
	for _, b := range []byte(s) {
		h = h ^ uint32(b)
		for i := 0; i < 8; i++ {
			if h&1 != 0 {
				h = (h >> 1) ^ 0xEDB88320
			} else {
				h = h >> 1
			}
		}
	}
	return h
}

// pushSizeOption создаёт опцию Размер и возвращает optionID + map[normalizedSize]->variantID.
func (p *APIPusher) pushSizeOption(hubProductID int64, csProductID int) (int, map[string]string) {
	rows, err := p.store.Hub.Query(`
		SELECT value FROM product_attrs
		WHERE product_id = ? AND is_configurator = 1 AND property_name = 'Размер'
		ORDER BY vid`, hubProductID)
	if err != nil {
		return 0, nil
	}
	defer rows.Close()

	var rawSizes []string
	for rows.Next() {
		var v string
		rows.Scan(&v)
		rawSizes = append(rawSizes, v)
	}
	if len(rawSizes) == 0 {
		return 0, nil
	}

	sizes := cscart.NormalizeSizes(rawSizes)
	optionID, err := p.csClient.CreateOption(csProductID, "Размер", sizes)
	if err != nil {
		log.Printf("[push] ERROR size option: %v", err)
		return 0, nil
	}

	// Получаем variant IDs обратно
	variantMap := p.getOptionVariants(optionID)
	log.Printf("[push] Размер id=%d (%d variants) for cs=%d", optionID, len(variantMap), csProductID)

	// Маппинг raw -> normalized -> variant_id
	rawToVariant := make(map[string]string)
	for _, raw := range rawSizes {
		normalized := cscart.NormalizeSize(raw)
		if vid, ok := variantMap[normalized]; ok {
			rawToVariant[raw] = vid
		}
	}
	return optionID, rawToVariant
}

