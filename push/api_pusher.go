// APIPusher - push товаров из Hub БД в CS-Cart через REST API.
// Заменяет legacy pusher.go (прямая запись в БД).
// Полный flow: DeepSeek нормализация -> CreateProduct -> CreateOption -> UpdateProductFeatures.
package push

import (
	"fmt"
	"log"
	"otapi-hub/cscart"
	"otapi-hub/db"
	"otapi-hub/translate"
	"time"
)

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
		WHERE p.category_id = ? AND p.is_sell_allowed = 1 AND p.is_expired = 0
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
		title := pr.TitleRu
		if title == "" {
			title = pr.TitleOrig
		}

		// DeepSeek нормализация (graceful: если упал - товар создаётся с оригинальным названием)
		var normalized *translate.NormalizeOutput
		if p.dsClient != nil {
			normalized = p.normalize(pr.ID, pr.TitleRu, pr.TitleOrig)
			if normalized != nil && normalized.Title != "" {
				title = normalized.Title
				res.logMsg(fmt.Sprintf("  [ds] %s -> %s", pr.TitleRu[:min(40, len(pr.TitleRu))], normalized.Title))
			} else if normalized == nil {
				res.logMsg("  [ds] DeepSeek пропущен (ошибка или недоступен)")
			}
		}

		addImages := p.getAdditionalImages(pr.ID)

		input := cscart.NewProductInput(
			title, categoryCS, p.companyID, pr.PriceTMT, pr.Quantity, pr.OtapiID,
			pr.Description, pr.Weight, pr.MainImage, addImages,
		)

		res.logMsg(fmt.Sprintf("[%d/%d] %s (%.0f TMT)...", i+1, len(products), pr.OtapiID, pr.PriceTMT))

		csID, err := p.csClient.CreateProduct(input)
		if err != nil {
			res.logMsg(fmt.Sprintf("  ERROR create: %v", err))
			res.Errors++
			continue
		}

		p.store.Hub.Exec(`UPDATE products SET cs_product_id=?, pushed_to_cs_at=? WHERE id=?`,
			csID, time.Now().Unix(), pr.ID)

		p.pushSizeOption(pr.ID, csID)
		p.pushFeatures(csID, normalized)

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
func (p *APIPusher) pushSizeOption(hubProductID int64, csProductID int) {
	rows, err := p.store.Hub.Query(`
		SELECT value FROM product_attrs
		WHERE product_id = ? AND is_configurator = 1 AND property_name = 'Размер'
		ORDER BY vid`, hubProductID)
	if err != nil {
		return
	}
	defer rows.Close()

	var rawSizes []string
	for rows.Next() {
		var v string
		rows.Scan(&v)
		rawSizes = append(rawSizes, v)
	}

	if len(rawSizes) == 0 {
		return
	}

	sizes := cscart.NormalizeSizes(rawSizes)

	optionID, err := p.csClient.CreateOption(csProductID, "Размер", sizes)
	if err != nil {
		log.Printf("[push] ERROR option for cs_product=%d: %v", csProductID, err)
		return
	}
	log.Printf("[push] option Размер id=%d (%d variants) for cs_product=%d", optionID, len(sizes), csProductID)
}

