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

// pushDeepSeekFeatures — пушить ли характеристики, выведенные DeepSeek-нормализацией.
// ВЫКЛ: DeepSeek заполнял фичи (Капюшон/Толщина/Модель/Повод…) правдоподобными значениями
// даже когда у товара НЕТ соответствующего исходного атрибута 1688 → на карточке появлялись
// характеристики «не от товара». Теперь источник характеристик — только реальные OT-атрибуты
// (attr_cs_mapping). Включить обратно = true (тогда DeepSeek дополняет незаполненные фичи).
const pushDeepSeekFeatures = false

// Маппинг DeepSeek полей -> CS-Cart feature_id (из CS-Cart/api/features)
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
	proxyBase string // URL proxy для 1688 фото (fallback)
	imgDL     *ImageDownloader
}

func NewAPIPusher(store *db.Store, csClient *cscart.Client, dsClient *translate.DeepSeekClient, companyID int) *APIPusher {
	return &APIPusher{store: store, csClient: csClient, dsClient: dsClient, companyID: companyID}
}

func (p *APIPusher) SetProxyBase(base string) {
	p.proxyBase = base
}

// SetImageDownloader настраивает скачивание изображений на локальный сервер.
// localDir - путь на диске, publicURL - базовый URL для CS-Cart.
func (p *APIPusher) SetImageDownloader(localDir, publicURL string) {
	if localDir != "" && publicURL != "" {
		p.imgDL = NewImageDownloader(localDir, publicURL)
		log.Printf("[push] Image downloader: localDir=%s publicURL=%s", localDir, publicURL)
	}
}

// resolveImageURL - для фото 1688:
//  1. Сначала пробует скачать на локальный сервер (через ImageDownloader)
//  2. Если ImageDownloader не настроен или не смог — fallback на img-proxy
func (p *APIPusher) resolveImageURL(imgURL string) string {
	if imgURL == "" {
		return ""
	}
	is1688 := strings.Contains(imgURL, "cbu01.alicdn.com") ||
		strings.Contains(imgURL, "cbu02.alicdn.com") ||
		strings.Contains(imgURL, "cbu03.alicdn.com")
	if !is1688 {
		return imgURL
	}

	// Попытка 1: скачать и отдать локальный URL
	if p.imgDL != nil {
		if local := p.imgDL.DownloadAndGetURL(imgURL); local != "" {
			return local
		}
		log.Printf("[push] img-dl failed, falling back to proxy for %s", imgURL)
	}

	// Попытка 2: proxy (fallback)
	if p.proxyBase != "" {
		return p.proxyBase + "/otweb/img-proxy?url=" + imgURL
	}
	return imgURL
}

// PushCategoryAuto - push товаров категории с автоматическим маппингом.
// Берёт CS-Cart category_id из таблицы category_map.
// Если маппинга нет - возвращает ошибку.
func (p *APIPusher) PushCategoryAuto(categoryOT string) *PushResult {
	mapping, err := p.store.GetCategoryMappingByOT(categoryOT)
	if err != nil || mapping.CSCategoryID == 0 {
		res := &PushResult{}
		res.logMsg(fmt.Sprintf("ERROR: нет маппинга для OT категории %s. Добавьте в category_map.", categoryOT))
		return res
	}
	return p.PushCategoryWithMapping(categoryOT, mapping)
}

// PushCategory - push товаров с фиксированным CS category_id (без keyword-фильтра).
func (p *APIPusher) PushCategory(categoryOT string, categoryCS int) *PushResult {
	return p.PushCategoryWithMapping(categoryOT, &db.CategoryMapping{
		OTCategoryID: categoryOT,
		CSCategoryID: categoryCS,
	})
}

// PushCategoryWithMapping - push всех непушеных товаров категории в CS-Cart.
// Если в маппинге задан TitleKeyword, то товары где он встречается идут в AltCSCategoryID.
func (p *APIPusher) PushCategoryWithMapping(categoryOT string, mapping *db.CategoryMapping) *PushResult {
	res := &PushResult{}

	rows, err := p.store.Hub.Query(`
		SELECT p.id, p.otapi_id, p.title_ru, p.title_original,
		       p.price_tmt, p.master_quantity, p.weight_kg,
		       IFNULL(p.description_html,''), IFNULL(p.main_image_url,''), IFNULL(p.gender,'')
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
		Gender      string
	}

	var products []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.OtapiID, &r.TitleRu, &r.TitleOrig,
			&r.PriceTMT, &r.Quantity, &r.Weight, &r.Description, &r.MainImage, &r.Gender); err != nil {
			res.logMsg(fmt.Sprintf("ERROR scan: %v", err))
			continue
		}
		products = append(products, r)
	}
	rows.Close()

	if mapping.TitleKeyword != "" {
		res.logMsg(fmt.Sprintf("Keyword-фильтр: \"%s\" → CS %d, иначе → CS %d",
			mapping.TitleKeyword, mapping.AltCSCategoryID, mapping.CSCategoryID))
	}
	res.logMsg(fmt.Sprintf("Категория %s -> CS %d: %d товаров для push", categoryOT, mapping.CSCategoryID, len(products)))

	for i, pr := range products {
		categoryCS := mapping.ResolveCategoryID(pr.TitleRu, pr.TitleOrig, pr.Gender)
		res.logMsg(fmt.Sprintf("[%d/%d] %s (%.0f TMT) -> CS cat %d...", i+1, len(products), pr.OtapiID, pr.PriceTMT, categoryCS))
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
// Если товар уже был отправлен (cs_product_id != NULL), обновляет существующий.
// Возвращает cs_product_id или ошибку.
func (p *APIPusher) PushSingleProduct(hubProductID int64, categoryCS int) (int, error) {
	var otapiID, titleRu, titleOrig, description, mainImage string
	var priceTMT float64
	var qty int
	var weight float64
	var existingCSID *int
	err := p.store.Hub.QueryRow(`
		SELECT otapi_id, title_ru, title_original, price_tmt, master_quantity,
		       weight_kg, IFNULL(description_html,''), IFNULL(main_image_url,''),
		       cs_product_id
		FROM products WHERE id=? AND enabled=1`, hubProductID).Scan(
		&otapiID, &titleRu, &titleOrig, &priceTMT, &qty, &weight, &description, &mainImage,
		&existingCSID)
	if err != nil {
		return 0, fmt.Errorf("product %d not found or disabled", hubProductID)
	}

	title := titleRu
	if title == "" {
		title = titleOrig
	}

	// DeepSeek: перевод на 3 языка + описания + нормализация характеристик (graceful)
	var normalized *translate.NormalizeOutput
	if p.dsClient != nil {
		normalized = p.normalize(hubProductID, titleRu, titleOrig)
		if normalized != nil {
			if normalized.TitleRU != "" {
				title = normalized.TitleRU
				p.store.Hub.Exec(`UPDATE products SET title_ru=?, title_en=?, title_tk=?,
					description_ru=?, description_en=?, description_tk=?,
					translate_status='deepseek' WHERE id=?`,
					normalized.TitleRU, normalized.TitleEN, normalized.TitleTK,
					normalized.DescriptionRU, normalized.DescriptionEN, normalized.DescriptionTK,
					hubProductID)
				// Сохраняем AI-оценку веса если API не дал реальный
				if normalized.EstimatedWeightGrams > 0 {
					p.store.Hub.Exec(`UPDATE products SET weight_kg=?, weight_estimated=1 WHERE id=? AND weight_kg=0`,
						float64(normalized.EstimatedWeightGrams)/1000.0, hubProductID)
				}
			} else if normalized.Title != "" {
				title = normalized.Title
			}
		}
	}

	// Используем DeepSeek описание если есть, иначе очищенный HTML
	var descRu string
	p.store.Hub.QueryRow(`SELECT IFNULL(description_ru,'') FROM products WHERE id=?`, hubProductID).Scan(&descRu)

	var cleanDesc string
	if descRu != "" {
		cleanDesc = descRu
	} else {
		// Очищаем сырой HTML: убираем img, div, span, style теги - оставляем только текст
		cleanDesc = regexp.MustCompile(`<img[^>]*>`).ReplaceAllString(description, "")
		cleanDesc = regexp.MustCompile(`<div[^>]*>|</div>|<span[^>]*>|</span>`).ReplaceAllString(cleanDesc, "")
		cleanDesc = regexp.MustCompile(`style="[^"]*"`).ReplaceAllString(cleanDesc, "")
		cleanDesc = regexp.MustCompile(`\s+`).ReplaceAllString(cleanDesc, " ")
		cleanDesc = strings.TrimSpace(cleanDesc)
		if cleanDesc == "" || len(cleanDesc) < 10 {
			cleanDesc = ""
		}
	}

	addImages := p.getAdditionalImages(hubProductID)

	// Извлекаем изображения из description (макс 5 доп. фото чтобы не было timeout)
	descImgs := regexp.MustCompile(`src="(https?://[^"]+)"`).FindAllStringSubmatch(description, -1)
	for _, m := range descImgs {
		if len(addImages) >= 5 {
			break
		}
		if len(m) > 1 && !strings.Contains(m[1], "spaceball") && !strings.Contains(m[1], "display:none") {
			addImages = append(addImages, m[1])
		}
	}

	// Скачиваем 1688 изображения на локальный сервер (anti-hotlinking)
	if resolved := p.resolveImageURL(mainImage); resolved != mainImage && resolved != "" {
		mainImage = resolved
		p.store.Hub.Exec(`UPDATE products SET main_image_url=? WHERE id=?`, mainImage, hubProductID)
	} else {
		mainImage = resolved
	}
	addImageIDs := p.getAdditionalImageIDs(hubProductID)
	for i := range addImages {
		if resolved := p.resolveImageURL(addImages[i]); resolved != addImages[i] && resolved != "" {
			addImages[i] = resolved
			if i < len(addImageIDs) {
				p.store.Hub.Exec(`UPDATE product_images SET url=? WHERE id=?`, resolved, addImageIDs[i])
			}
		} else {
			addImages[i] = resolved
		}
	}

	// MOQ (мин. кол-во в заказе): по резолву CS-категории с наследованием вверх по дереву.
	moq := p.store.ResolveMOQ(categoryCS)

	var csID int

	if existingCSID != nil && *existingCSID > 0 {
		// UPDATE существующего товара в CS-Cart
		csID = *existingCSID
		log.Printf("[push] updating existing CS-Cart product %d", csID)

		update := cscart.ProductUpdate{
			Product:         title,
			Price:           fmt.Sprintf("%.2f", priceTMT),
			Amount:          qty,
			MinQty:          moq,
			CategoryIDs:     []int{categoryCS},
			FullDescription: cleanDesc,
			Weight:          weight,
		}
		if mainImage != "" {
			update.MainPair = &cscart.ImagePair{
				Detailed: cscart.ImageDetailed{ImagePath: mainImage},
			}
		}
		for _, url := range addImages {
			update.ImagePairs = append(update.ImagePairs, cscart.ImagePair{
				Detailed: cscart.ImageDetailed{ImagePath: url},
			})
		}

		if err := p.csClient.UpdateProduct(csID, update); err != nil {
			return 0, fmt.Errorf("update cs product %d: %w", csID, err)
		}

		p.store.Hub.Exec(`UPDATE products SET pushed_to_cs_at=? WHERE id=?`, time.Now().Unix(), hubProductID)
	} else {
		// CREATE нового товара в CS-Cart
		input := cscart.NewProductInput(title, categoryCS, p.companyID, priceTMT, qty, otapiID, cleanDesc, weight, mainImage, addImages)
		input.MinQty = moq

		csID, err = p.csClient.CreateProduct(input)
		if err != nil {
			return 0, err
		}

		p.store.Hub.Exec(`UPDATE products SET cs_product_id=?, pushed_to_cs_at=? WHERE id=?`, csID, time.Now().Unix(), hubProductID)
	}

	// Создаём/обновляем опции и комбинации (вариации с остатками)
	sizeOptID, sizeVariants := p.pushSizeOption(hubProductID, csID, priceTMT)
	colorOptID, colorVariants := p.pushColorOption(hubProductID, csID, priceTMT)
	if sizeOptID > 0 || colorOptID > 0 {
		p.pushCombinations(hubProductID, csID, sizeOptID, sizeVariants, colorOptID, colorVariants)
	}

	p.pushFeatures(hubProductID, csID, normalized)

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

	// Загружаем атрибуты с RU переводами для более информативного промпта.
	// Формат: "RU_название (ZH_название)" = "RU_значение (ZH_значение)"
	// Если перевода нет — используем сырой китайский текст.
	rows, err := p.store.Hub.Query(`
		SELECT pa.property_name, pa.value,
		       COALESCE(NULLIF(at.property_name_ru,''), pa.property_name),
		       COALESCE(NULLIF(at.value_ru,''), pa.value)
		FROM product_attrs pa
		LEFT JOIN attr_translations at ON pa.pid = at.pid AND pa.vid = at.vid
		WHERE pa.product_id = ? AND pa.is_configurator = 0`, hubProductID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	attrs := make(map[string]string)
	for rows.Next() {
		var nameZh, valueZh, nameRu, valueRu string
		rows.Scan(&nameZh, &valueZh, &nameRu, &valueRu)
		// Ключ: "RU название (ZH)" — даёт DeepSeek контекст на двух языках
		key := nameRu
		if nameZh != nameRu {
			key = fmt.Sprintf("%s (%s)", nameRu, nameZh)
		}
		val := valueRu
		if valueZh != valueRu {
			val = fmt.Sprintf("%s (%s)", valueRu, valueZh)
		}
		attrs[key] = val
	}

	rows2, err := p.store.Hub.Query(`
		SELECT DISTINCT value FROM product_attrs
		WHERE product_id = ? AND is_configurator = 1 AND property_name NOT IN ('Размер','Size','尺码')`, hubProductID)
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

// pushFeatures - записывает характеристики товара в CS-Cart.
// Источник 1 (основной): attr_cs_mapping — прямой маппинг (pid,vid) → cs_feature_id + cs_variant_id.
// Источник 2 (резервный): DeepSeek NormalizeOutput — для текстовых фич (Сезон, Бренд) и fallback.
// applyCategoryFeatureBlacklist убирает из resolved характеристики, занесённые в чёрный
// список категории (category_feature_blacklist) — их не пушим. Данные в БД не трогаем.
func (p *APIPusher) applyCategoryFeatureBlacklist(hubProductID int64, resolved map[int]string) {
	var categoryID string
	p.store.Hub.QueryRow(`SELECT category_id FROM products WHERE id=?`, hubProductID).Scan(&categoryID)
	if categoryID == "" {
		return
	}
	rows, err := p.store.Hub.Query(`SELECT cs_feature_id FROM category_feature_blacklist WHERE category_id=?`, categoryID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var fid int
		rows.Scan(&fid)
		delete(resolved, fid)
	}
}

func (p *APIPusher) pushFeatures(hubProductID int64, csProductID int, n *translate.NormalizeOutput) {
	resolved := make(map[int]string) // cs_feature_id -> cs_variant_id (строка)

	// Источник 1: attr_cs_mapping — прямые маппинги для атрибутов этого товара
	rows, err := p.store.Hub.Query(`
		SELECT m.cs_feature_id, m.cs_variant_id, m.canonical_value
		FROM product_attrs pa
		JOIN attr_cs_mapping m ON pa.pid = m.pid AND pa.vid = m.vid
		WHERE pa.product_id = ? AND pa.is_configurator = 0
		  AND m.cs_feature_id > 0`, hubProductID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var fid, vid int
			var canonical string
			rows.Scan(&fid, &vid, &canonical)
			if _, already := resolved[fid]; already {
				continue // первый встреченный для данной фичи — приоритет
			}
			if vid > 0 {
				resolved[fid] = fmt.Sprintf("%d", vid)
			} else if canonical != "" {
				// Свободный текст (тип E/T) — передаём значение напрямую
				resolved[fid] = canonical
			}
		}
		log.Printf("[push] attr_cs_mapping: %d features from product attrs", len(resolved))
	}

	// Источник 2: DeepSeek NormalizeOutput (только если значение не перекрывается маппингом)
	if pushDeepSeekFeatures && n != nil {
		dsFields := map[string]string{
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
		for name, value := range dsFields {
			if value == "" {
				continue
			}
			fid, ok := featureMap[name]
			if !ok {
				continue
			}
			if _, already := resolved[fid]; already {
				continue // attr_cs_mapping уже установил эту фичу
			}
			variantID, found := p.csClient.ResolveFeatureVariant(fid, value)
			if !found {
				log.Printf("[push] DS feature %q=%q: variant not found (fid=%d)", name, value, fid)
				continue
			}
			resolved[fid] = variantID
		}
	}

	// Чёрный список характеристик категории: убираем скрытые фичи перед пушем.
	p.applyCategoryFeatureBlacklist(hubProductID, resolved)

	if len(resolved) == 0 {
		return
	}

	// Типы фич (S/M/E/T…) для корректного payload: M (мультичекбокс, напр. Узор)
	// нужно слать массивом вариантов, иначе CS-Cart молча его не сохраняет.
	types := p.featureTypes(resolved)

	err = p.csClient.UpdateProductFeatures(csProductID, resolved, types)
	if err != nil {
		log.Printf("[push] features error cs_product=%d: %v", csProductID, err)
	} else {
		log.Printf("[push] features: %d set for cs_product=%d", len(resolved), csProductID)
	}
}

// featureTypes возвращает feature_id -> тип фичи CS-Cart (S,M,E,T,N,C…) из зеркала
// cs_features_cache. Нужно, чтобы UpdateProductFeatures собрал корректный payload.
func (p *APIPusher) featureTypes(resolved map[int]string) map[int]string {
	types := make(map[int]string, len(resolved))
	if len(resolved) == 0 {
		return types
	}
	placeholders := make([]string, 0, len(resolved))
	args := make([]interface{}, 0, len(resolved))
	for fid := range resolved {
		placeholders = append(placeholders, "?")
		args = append(args, fid)
	}
	q := "SELECT feature_id, feature_type FROM cs_features_cache WHERE feature_id IN (" + strings.Join(placeholders, ",") + ")"
	rows, err := p.store.Hub.Query(q, args...)
	if err != nil {
		return types
	}
	defer rows.Close()
	for rows.Next() {
		var fid int
		var ft string
		rows.Scan(&fid, &ft)
		types[fid] = ft
	}
	return types
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

func (p *APIPusher) getAdditionalImageIDs(hubProductID int64) []int64 {
	rows, err := p.store.Hub.Query(`
		SELECT id FROM product_images
		WHERE product_id = ? AND is_main = 0 ORDER BY position ASC`, hubProductID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

// pushSizeOption - создаёт опцию "Размер" с вариантами для товара в CS-Cart.
// Читает is_configurator=1 атрибуты из Hub БД, нормализует через NormalizeSizes,
// вызывает POST /api/options с вариантами (S, M, L, XL...).
// pushColorOption - создаёт опцию "Цвет" если у товара есть цветовые вариации.
// pushColorOption создаёт опцию Цвет и возвращает optionID + map[rawColor]->variantID.
// mappedVidOption — информация об одном configurator-виде с маппингом.
type mappedVidOption struct {
	pid          string
	vid          string
	rawValue     string
	imageURL     string
	canonicalName string // из cs_feature_variants_cache
}

// loadMappedVidOptions возвращает только те configurator vids указанных pids,
// у которых есть валидный маппинг в attr_cs_mapping (cs_variant_id > 0).
// canonicalName берётся из cs_feature_variants_cache.variant_value.
func (p *APIPusher) loadMappedVidOptions(hubProductID int64, pids ...string) []mappedVidOption {
	if len(pids) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(pids))
	placeholders = placeholders[:len(placeholders)-1]
	args := []interface{}{hubProductID}
	for _, pid := range pids {
		args = append(args, pid)
	}
	rows, err := p.store.Hub.Query(`
		SELECT pa.pid, pa.vid, pa.value, IFNULL(pa.image_url,''), fvc.variant_value
		FROM product_attrs pa
		JOIN attr_cs_mapping m ON m.pid=pa.pid AND m.vid=pa.vid
		JOIN cs_feature_variants_cache fvc ON fvc.variant_id=m.cs_variant_id
		WHERE pa.product_id=? AND pa.is_configurator=1 AND pa.pid IN (`+placeholders+`)
		  AND m.cs_variant_id > 0
		ORDER BY pa.vid`, args...)
	if err != nil {
		log.Printf("[push] WARN loadMappedVidOptions: %v", err)
		return nil
	}
	defer rows.Close()
	seen := make(map[string]bool) // pid:vid — дедупликация
	var result []mappedVidOption
	for rows.Next() {
		var opt mappedVidOption
		rows.Scan(&opt.pid, &opt.vid, &opt.rawValue, &opt.imageURL, &opt.canonicalName)
		key := opt.pid + ":" + opt.vid
		if !seen[key] {
			seen[key] = true
			result = append(result, opt)
		}
	}
	return result
}

// lookupOptionMapping возвращает map[raw_value]canonical_value из sku_option_mapping.
// Значения которых нет в таблице — возвращаются как есть (raw).
func (p *APIPusher) lookupOptionMapping(rawValues []string, optionType string) map[string]string {
	result := make(map[string]string, len(rawValues))
	for _, v := range rawValues {
		result[v] = v // дефолт — сырое значение
	}
	if len(rawValues) == 0 {
		return result
	}
	placeholders := strings.Repeat("?,", len(rawValues))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(rawValues)+1)
	for i, v := range rawValues {
		args[i] = v
	}
	args[len(rawValues)] = optionType
	rows, err := p.store.Hub.Query(
		`SELECT raw_value, canonical_value FROM sku_option_mapping WHERE raw_value IN (`+placeholders+`) AND option_type=?`,
		args...)
	if err != nil {
		log.Printf("[push] WARN lookupOptionMapping: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var raw, canonical string
		rows.Scan(&raw, &canonical)
		if canonical != "" {
			result[raw] = canonical
		}
	}
	return result
}

// pushColorOption создаёт опцию Цвет только из замапленных вариантов (attr_cs_mapping).
// Возвращает optionID и map[pid:vid] -> cs_option_variant_id.
func (p *APIPusher) pushColorOption(hubProductID int64, csProductID int, basePriceTMT float64) (int, map[string]string) {
	// Только замапленные цвета (cs_variant_id > 0 в attr_cs_mapping)
	opts := p.loadMappedVidOptions(hubProductID, "颜色", "颜色分类")
	if len(opts) == 0 {
		return 0, nil
	}

	colorPriceMod := p.calcColorPriceMods(hubProductID, basePriceTMT)

	// Дедупликация по canonicalName (несколько vid могут иметь одно каноническое имя)
	seenCanonical := make(map[string]bool)
	var variants []cscart.OptionVariant
	for _, opt := range opts {
		if seenCanonical[opt.canonicalName] {
			continue
		}
		seenCanonical[opt.canonicalName] = true
		variants = append(variants, cscart.OptionVariant{
			Name:     opt.canonicalName,
			ImageURL: opt.imageURL,
			PriceMod: colorPriceMod[opt.rawValue],
		})
	}

	optionID, err := p.csClient.CreateOptionAdvanced(csProductID, "Цвет", variants)
	if err != nil {
		log.Printf("[push] ERROR color option: %v", err)
		return 0, nil
	}
	variantMap := p.getOptionVariants(optionID)
	imgCount := 0
	for _, opt := range opts {
		if opt.imageURL != "" {
			imgCount++
		}
	}
	log.Printf("[push] Цвет id=%d (%d variants, %d with images) for cs=%d (filtered from attr_cs_mapping)",
		optionID, len(variantMap), imgCount, csProductID)

	// vidToVariant: ключ — pid:vid, значение — CS-Cart option_variant_id
	vidToVariant := make(map[string]string)
	for _, opt := range opts {
		if csVid, ok := variantMap[opt.canonicalName]; ok {
			vidToVariant[opt.pid+":"+opt.vid] = csVid
		}
	}
	return optionID, vidToVariant
}

// calcColorPriceMods вычисляет price modifier для каждого цвета.
// Берёт среднюю цену SKU по каждому цвету, вычитает basePriceTMT.
func (p *APIPusher) calcColorPriceMods(hubProductID int64, basePriceTMT float64) map[string]float64 {
	return p.calcPriceMods(hubProductID, basePriceTMT, "Цвет", "Color", "Классификация цветов")
}

// calcSizePriceMods вычисляет price modifier для каждого размера.
func (p *APIPusher) calcSizePriceMods(hubProductID int64, basePriceTMT float64) map[string]float64 {
	return p.calcPriceMods(hubProductID, basePriceTMT, "Размер", "Size", "")
}

// calcPriceMods - общая логика для price modifiers по свойству.
// Группирует SKU по значению свойства, считает среднюю цену, возвращает разницу с базовой.
func (p *APIPusher) calcPriceMods(hubProductID int64, basePriceTMT float64, propNames ...string) map[string]float64 {
	result := make(map[string]float64)

	// Строим маппинг pid:vid -> value для нужных свойств
	vidToValue := make(map[string]string)
	for _, pn := range propNames {
		if pn == "" {
			continue
		}
		rows, _ := p.store.Hub.Query(`SELECT pid, vid, value FROM product_attrs WHERE product_id=? AND is_configurator=1 AND property_name=?`, hubProductID, pn)
		if rows != nil {
			for rows.Next() {
				var pid, vid, val string
				rows.Scan(&pid, &vid, &val)
				vidToValue[pid+":"+vid] = val
			}
			rows.Close()
		}
	}
	if len(vidToValue) == 0 {
		return result
	}

	// Группируем цены SKU по значению свойства
	type priceAgg struct {
		sum   float64
		count int
	}
	agg := make(map[string]*priceAgg)

	skuRows, _ := p.store.Hub.Query(`SELECT price_cny, configurators FROM product_skus WHERE product_id=? AND price_cny > 0`, hubProductID)
	if skuRows == nil {
		return result
	}
	defer skuRows.Close()

	for skuRows.Next() {
		var priceCNY float64
		var confsJSON string
		skuRows.Scan(&priceCNY, &confsJSON)

		var confs []struct{ Pid, Vid string }
		json.Unmarshal([]byte(confsJSON), &confs)

		for _, c := range confs {
			key := c.Pid + ":" + c.Vid
			if val, ok := vidToValue[key]; ok {
				if agg[val] == nil {
					agg[val] = &priceAgg{}
				}
				agg[val].sum += priceCNY
				agg[val].count++
			}
		}
	}

	// Берём exchange rate из settings
	var exchangeRate float64
	p.store.Hub.QueryRow(`SELECT IFNULL(exchange_rate, 2.74) FROM markup_rules WHERE scope_type='global' LIMIT 1`).Scan(&exchangeRate)
	if exchangeRate == 0 {
		exchangeRate = 2.74
	}

	// Вычисляем modifier: (avgCNY * rate) - basePriceTMT
	for val, a := range agg {
		avgCNY := a.sum / float64(a.count)
		avgTMT := avgCNY * exchangeRate
		diff := avgTMT - basePriceTMT
		// Только если разница существенная (> 1 TMT)
		if diff > 1 || diff < -1 {
			result[val] = float64(int(diff*10+0.5)) / 10 // round to 0.1
		}
	}

	return result
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
// sizeVariants и colorVariants теперь имеют ключ pid:vid (не raw value).
func (p *APIPusher) pushCombinations(hubProductID int64, csProductID int,
	sizeOptID int, sizeVariants map[string]string,
	colorOptID int, colorVariants map[string]string) {

	// sizeVariants и colorVariants уже содержат map[pid:vid] -> cs_option_variant_id
	// (формируются в pushSizeOption и pushColorOption через loadMappedVidOptions)
	log.Printf("[push] vid mappings: %d sizes, %d colors", len(sizeVariants), len(colorVariants))

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

		// Прямой lookup: pid:vid -> CS-Cart option_variant_id
		var sizeVID, colorVID string
		for _, c := range confs {
			key := c.Pid + ":" + c.Vid
			if v, ok := sizeVariants[key]; ok {
				sizeVID = v
			}
			if v, ok := colorVariants[key]; ok {
				colorVID = v
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
			log.Printf("[push] SKU %s: no matching variants (sizeVID=%s colorVID=%s)", skuID, sizeVID, colorVID)
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
	inserted := 0
	for _, c := range combos {
		_, err := p.store.Mirror.Exec(`INSERT INTO cscart_product_options_inventory
			(product_id, product_code, combination_hash, combination, amount, temp, position)
			VALUES (?, ?, ?, ?, ?, 'N', 0)
			ON DUPLICATE KEY UPDATE amount=VALUES(amount)`,
			csProductID, c.skuID, c.hash, c.combination, c.qty)
		if err != nil {
			log.Printf("[push] ERROR inserting combination for SKU %s: %v", c.skuID, err)
		} else {
			inserted++
		}
	}

	// Включаем tracking по опциям
	if _, err := p.store.Mirror.Exec(`UPDATE cscart_products SET tracking='O' WHERE product_id=?`, csProductID); err != nil {
		log.Printf("[push] ERROR setting tracking='O' for cs=%d: %v", csProductID, err)
	}

	log.Printf("[push] %d/%d combinations inserted for cs=%d", inserted, len(combos), csProductID)
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

// pushSizeOption создаёт опцию Размер только из замапленных вариантов (attr_cs_mapping).
// Для незамапленных vids — fallback на NormalizeSize.
// Возвращает optionID и map[pid:vid] -> cs_option_variant_id.
func (p *APIPusher) pushSizeOption(hubProductID int64, csProductID int, basePriceTMT float64) (int, map[string]string) {
	// Сначала пробуем загрузить через attr_cs_mapping (замапленные)
	mappedOpts := p.loadMappedVidOptions(hubProductID, "尺码", "适合身高", "规格", "尺寸", "童袜尺码")

	// Дополнительно загружаем незамапленные (fallback на NormalizeSize)
	fallbackRows, _ := p.store.Hub.Query(`
		SELECT pa.pid, pa.vid, pa.value
		FROM product_attrs pa
		LEFT JOIN attr_cs_mapping m ON m.pid=pa.pid AND m.vid=pa.vid
		WHERE pa.product_id=? AND pa.is_configurator=1
		  AND pa.pid IN ('尺码','适合身高','规格','尺寸','童袜尺码')
		  AND (m.cs_variant_id IS NULL OR m.cs_variant_id = 0)
		ORDER BY pa.vid`, hubProductID)

	var fallbackOpts []mappedVidOption
	if fallbackRows != nil {
		defer fallbackRows.Close()
		for fallbackRows.Next() {
			var opt mappedVidOption
			fallbackRows.Scan(&opt.pid, &opt.vid, &opt.rawValue)
			opt.canonicalName = cscart.NormalizeSize(opt.rawValue)
			fallbackOpts = append(fallbackOpts, opt)
		}
	}

	allOpts := append(mappedOpts, fallbackOpts...)
	if len(allOpts) == 0 {
		return 0, nil
	}

	sizePriceMod := p.calcSizePriceMods(hubProductID, basePriceTMT)

	seenCanonical := make(map[string]bool)
	var variants []cscart.OptionVariant
	for _, opt := range allOpts {
		if opt.canonicalName == "" || seenCanonical[opt.canonicalName] {
			continue
		}
		seenCanonical[opt.canonicalName] = true
		variants = append(variants, cscart.OptionVariant{
			Name:     opt.canonicalName,
			PriceMod: sizePriceMod[opt.rawValue],
		})
	}
	if len(variants) == 0 {
		return 0, nil
	}

	optionID, err := p.csClient.CreateOptionAdvanced(csProductID, "Размер", variants)
	if err != nil {
		log.Printf("[push] ERROR size option: %v", err)
		return 0, nil
	}

	variantMap := p.getOptionVariants(optionID)
	log.Printf("[push] Размер id=%d (%d variants, %d mapped) for cs=%d",
		optionID, len(variantMap), len(mappedOpts), csProductID)

	// vidToVariant: ключ — pid:vid, значение — CS-Cart option_variant_id
	vidToVariant := make(map[string]string)
	for _, opt := range allOpts {
		if csVid, ok := variantMap[opt.canonicalName]; ok {
			vidToVariant[opt.pid+":"+opt.vid] = csVid
		}
	}
	return optionID, vidToVariant
}

