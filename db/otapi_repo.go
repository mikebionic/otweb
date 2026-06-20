package db

import (
	"encoding/json"
	"strings"
	"time"

	"otapi-hub/cscart"
)

type Category struct {
	ID        string
	Provider  string
	Name      string
	ParentID  string
	IsParent  bool
	ItemCount int
	FetchedAt int64
}

type CategoryConfig struct {
	CategoryID       string
	Enabled          bool
	SyncSchedule     string
	MaxProducts      int
	LastSyncedAt     *int64
	ProductsImported int
	CSCategoryID     *int
	Notes            string
}

type Product struct {
	ID              int64
	OtapiID         string
	Provider        string
	CategoryID      string
	TitleOriginal   string
	TitleRu         string
	TitleEn         string
	TitleTk         string
	TranslateStatus string
	PriceCNY        float64
	PriceTMT        float64
	MasterQuantity  int
	IsFakeQty       bool
	IsSellAllowed   bool
	IsExpired       bool
	IsTmall         bool
	MainImageURL    string
	PlatformURL     string
	VendorName      string
	BrandName       string
	LocationCity    string
	LocationCityRu  string
	LocationState   string
	LocationStateRu string
	VolumeSales     int
	SalesLast30     int
	FavCount        int
	ReviewsCount    int
	Rating          float64
	GoodRates       float64
	PayOrder30Day   int
	QualityScore    int
	HasHierConf     bool
	FetchedAt       int64
	UpdatedAt       int64
	PushedToCsAt    *int64
	CsProductID     *int
	DescriptionRU   string
	DescriptionEN   string
	DescriptionTK   string
	Enabled         bool
	HiddenAt        *int64
	Gender          string // male|female|unisex
	AgeGroup        string // CSV: baby,kids,children
}

type ProductSKU struct {
	ID            int64
	ProductID     int64
	SKUID         string
	Quantity      int
	PriceCNY      float64
	Configurators string
}

type SyncJob struct {
	ID             int
	JobType        string
	CategoryID     string
	CategoryName   string
	Status         string
	StartedAt      *int64
	FinishedAt     *int64
	ItemsProcessed int
	ItemsSkipped   int
	ErrorsCount    int
	APIRequests    int
	Log            string
	TriggeredBy    string
}

type MarkupRule struct {
	ID           int
	ScopeType    string
	ScopeID      *string
	MarkupPct    float64
	FixedAddon   float64
	ExchangeRate *float64
	IsActive     bool
	Notes        string
}

func (s *Store) GetCategories() ([]Category, error) {
	rows, err := s.Hub.Query(`SELECT id, provider, name_ru, IFNULL(parent_id,''), is_parent, item_count, fetched_at FROM categories ORDER BY provider, name_ru`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Provider, &c.Name, &c.ParentID, &c.IsParent, &c.ItemCount, &c.FetchedAt); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, nil
}

func (s *Store) UpsertCategory(id, provider, externalID, parentID, nameRu, nameEn string, isParent bool) error {
	_, err := s.Hub.Exec(`
		INSERT INTO categories (id, provider, external_id, parent_id, name_ru, name_en, name_zh, is_parent, fetched_at)
		VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE name_zh=VALUES(name_zh), is_parent=VALUES(is_parent), fetched_at=VALUES(fetched_at)`,
		id, provider, externalID, parentID, nameRu, nameEn, nameRu, isParent, time.Now().Unix())
	return err
}

func (s *Store) GetCategoriesWithConfig() ([]CategoryWithConfig, error) {
	rows, err := s.Hub.Query(`
		SELECT COALESCE(c.id, cc.category_id),
		       COALESCE(c.provider, ''),
		       COALESCE(NULLIF(c.name_ru,''), NULLIF(c.name_en,''), c.name_zh, cc.category_id, c.id),
		       COALESCE(c.name_en, ''), COALESCE(c.name_zh, ''),
		       IFNULL(c.parent_id,''), IFNULL(c.is_parent,0), IFNULL(c.item_count,0),
		       IFNULL(cc.enabled, 0), IFNULL(cc.sync_schedule,'manual'),
		       IFNULL(cc.max_products, 500), cc.last_synced_at,
		       IFNULL(cc.products_imported, 0), cc.cs_category_id, IFNULL(cc.notes,''),
		       (SELECT COUNT(*) FROM products p WHERE p.category_id = COALESCE(c.id, cc.category_id)) AS local_count
		FROM categories c
		LEFT JOIN category_config cc ON cc.category_id = c.id
		WHERE c.is_hidden = 0
		UNION
		SELECT cc2.category_id, '', cc2.category_id, '', '',
		       '', 0, 0,
		       cc2.enabled, cc2.sync_schedule,
		       cc2.max_products, cc2.last_synced_at,
		       cc2.products_imported, cc2.cs_category_id, IFNULL(cc2.notes,''),
		       (SELECT COUNT(*) FROM products p WHERE p.category_id = cc2.category_id)
		FROM category_config cc2
		WHERE cc2.category_id NOT IN (SELECT id FROM categories)
		ORDER BY 2, 6, 3`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CategoryWithConfig
	for rows.Next() {
		var cc CategoryWithConfig
		if err := rows.Scan(&cc.ID, &cc.Provider, &cc.Name, &cc.NameEn, &cc.NameZh,
			&cc.ParentID, &cc.IsParent, &cc.ItemCount,
			&cc.Enabled, &cc.SyncSchedule, &cc.MaxProducts, &cc.LastSyncedAt,
			&cc.ProductsImported, &cc.CSCategoryID, &cc.Notes, &cc.LocalCount); err != nil {
			return nil, err
		}
		result = append(result, cc)
	}
	return result, nil
}

func (s *Store) DeleteCategory(id string) error {
	// Hide category and all its children
	s.Hub.Exec(`UPDATE categories SET is_hidden=1 WHERE id=? OR parent_id=?`, id, id)
	// Disable children in category_config too
	s.Hub.Exec(`UPDATE category_config SET enabled=0 WHERE category_id=? OR category_id IN (SELECT id FROM categories WHERE parent_id=?)`, id, id)
	// For orphan category_config rows (no matching categories entry), delete them
	_, err := s.Hub.Exec(`DELETE FROM category_config WHERE category_id=? AND category_id NOT IN (SELECT id FROM categories)`, id)
	return err
}

func (s *Store) UpsertCategoryConfig(categoryID string, enabled bool, schedule string, maxProducts int, csCategoryID *int, notes string) error {
	now := time.Now().Unix()
	_, err := s.Hub.Exec(`
		INSERT INTO category_config (category_id, enabled, sync_schedule, max_products, cs_category_id, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		  enabled=VALUES(enabled), sync_schedule=VALUES(sync_schedule),
		  max_products=VALUES(max_products), cs_category_id=VALUES(cs_category_id),
		  notes=VALUES(notes), updated_at=VALUES(updated_at)`,
		categoryID, enabled, schedule, maxProducts, csCategoryID, notes, now, now)
	return err
}

// SetCategoryEnabled меняет ТОЛЬКО флаг enabled у списка категорий,
// сохраняя cs_category_id / max_products / schedule (в отличие от UpsertCategoryConfig).
// Для новых строк создаёт запись с дефолтами.
func (s *Store) SetCategoryEnabled(ids []string, enabled bool) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().Unix()
	for _, id := range ids {
		if _, err := s.Hub.Exec(`
			INSERT INTO category_config (category_id, enabled, sync_schedule, max_products, notes, created_at, updated_at)
			VALUES (?, ?, 'manual', 500, '', ?, ?)
			ON DUPLICATE KEY UPDATE enabled=VALUES(enabled), updated_at=VALUES(updated_at)`,
			id, enabled, now, now); err != nil {
			return err
		}
	}
	return nil
}

// DescendantCategoryIDs возвращает ID всех потомков категории (рекурсивно, BFS по parent_id).
func (s *Store) DescendantCategoryIDs(rootID string) ([]string, error) {
	var out []string
	queue := []string{rootID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		rows, err := s.Hub.Query(`SELECT id FROM categories WHERE parent_id=?`, cur)
		if err != nil {
			return out, err
		}
		var kids []string
		for rows.Next() {
			var cid string
			if err := rows.Scan(&cid); err == nil {
				kids = append(kids, cid)
			}
		}
		rows.Close()
		out = append(out, kids...)
		queue = append(queue, kids...)
	}
	return out, nil
}

// AncestorCategoryIDs возвращает ID всех предков категории (вверх по parent_id).
func (s *Store) AncestorCategoryIDs(id string) ([]string, error) {
	var out []string
	cur := id
	for i := 0; i < 30; i++ { // защита от циклов
		var pid string
		if err := s.Hub.QueryRow(`SELECT IFNULL(parent_id,'') FROM categories WHERE id=?`, cur).Scan(&pid); err != nil || pid == "" {
			break
		}
		out = append(out, pid)
		cur = pid
	}
	return out, nil
}

// ProductFilter - фильтры для списка товаров.
type ProductFilter struct {
	CategoryID      string
	Provider        string
	TranslateStatus string
	Search          string
	SortBy          string
	LocationState   string
	HasWeight       bool
	PushedOnly      bool
	UnpushedOnly    bool
	EnabledOnly     bool
	DisabledOnly    bool
	Gender          string // male|female|unisex
	AgeGroup        string // baby|kids|children (FIND_IN_SET по CSV)
	PropPid         string // фильтр по свойству товара: pid атрибута
	PropVid         string // фильтр по свойству товара: vid (значение); пусто = любое значение свойства
	FetchedAfter    int64  // только товары, синхронизированные не раньше этого времени (unix); 0 = без ограничения
	MinQuality      int    // мин. quality_score (0-100); 0 = без ограничения
}

func (f ProductFilter) orderClause() string {
	switch f.SortBy {
	case "quality":
		return "quality_score DESC"
	case "sales":
		return "volume_sales DESC"
	case "sales30":
		return "sales_last_30days DESC"
	case "price_asc":
		return "price_tmt ASC"
	case "price_desc":
		return "price_tmt DESC"
	case "qty":
		return "master_quantity DESC"
	case "reviews":
		return "reviews_count DESC"
	case "fav":
		return "fav_count DESC"
	default:
		return "fetched_at DESC"
	}
}

func (s *Store) GetProducts(categoryID string, page, limit int) ([]Product, int, error) {
	return s.GetProductsFiltered(ProductFilter{CategoryID: categoryID}, page, limit)
}

func (s *Store) GetProductsFiltered(f ProductFilter, page, limit int) ([]Product, int, error) {
	offset := (page - 1) * limit

	where := "1=1"
	var args []interface{}

	if f.CategoryID != "" {
		where += " AND category_id = ?"
		args = append(args, f.CategoryID)
	}
	if f.Provider != "" {
		where += " AND provider = ?"
		args = append(args, f.Provider)
	}
	if f.TranslateStatus != "" {
		where += " AND translate_status = ?"
		args = append(args, f.TranslateStatus)
	}
	if f.Search != "" {
		where += " AND (title_ru LIKE ? OR title_original LIKE ? OR otapi_id LIKE ?)"
		s := "%" + f.Search + "%"
		args = append(args, s, s, s)
	}
	if f.PushedOnly {
		where += " AND cs_product_id IS NOT NULL"
	}
	if f.UnpushedOnly {
		where += " AND cs_product_id IS NULL"
	}
	if f.EnabledOnly {
		where += " AND enabled = 1"
	}
	if f.DisabledOnly {
		where += " AND enabled = 0"
	}
	if f.LocationState != "" {
		where += " AND location_state = ?"
		args = append(args, f.LocationState)
	}
	if f.HasWeight {
		where += " AND weight_kg > 0"
	}
	if f.Gender != "" {
		where += " AND gender = ?"
		args = append(args, f.Gender)
	}
	if f.AgeGroup != "" {
		where += " AND FIND_IN_SET(?, age_group)"
		args = append(args, f.AgeGroup)
	}
	if f.PropPid != "" {
		if f.PropVid != "" {
			where += " AND id IN (SELECT product_id FROM product_attrs WHERE pid = ? AND vid = ?)"
			args = append(args, f.PropPid, f.PropVid)
		} else {
			where += " AND id IN (SELECT product_id FROM product_attrs WHERE pid = ?)"
			args = append(args, f.PropPid)
		}
	}
	if f.FetchedAfter > 0 {
		where += " AND fetched_at >= ?"
		args = append(args, f.FetchedAfter)
	}
	if f.MinQuality > 0 {
		where += " AND quality_score >= ?"
		args = append(args, f.MinQuality)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	s.Hub.QueryRow("SELECT COUNT(*) FROM products WHERE "+where, countArgs...).Scan(&total)

	queryArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := s.Hub.Query(`
		SELECT id, otapi_id, provider, category_id, title_original, title_ru, title_en, title_tk,
		       translate_status, price_cny, price_tmt, master_quantity, is_fake_quantity,
		       is_sell_allowed, is_expired, is_tmall, IFNULL(main_image_url,''),
		       IFNULL(platform_url,''), vendor_name, brand_name,
		       location_city, location_city_ru, location_state, location_state_ru,
		       volume_sales, sales_last_30days, fav_count, reviews_count,
		       rating, good_rates, pay_order_30day, quality_score,
		       has_hierarchical_conf, fetched_at, updated_at,
		       cs_product_id, pushed_to_cs_at,
		       IFNULL(description_ru,''), IFNULL(description_en,''), IFNULL(description_tk,''),
		       enabled, hidden_at, IFNULL(gender,''), IFNULL(age_group,'')
		FROM products WHERE `+where+`
		ORDER BY `+f.orderClause()+` LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(
			&p.ID, &p.OtapiID, &p.Provider, &p.CategoryID,
			&p.TitleOriginal, &p.TitleRu, &p.TitleEn, &p.TitleTk,
			&p.TranslateStatus, &p.PriceCNY, &p.PriceTMT, &p.MasterQuantity,
			&p.IsFakeQty, &p.IsSellAllowed, &p.IsExpired, &p.IsTmall,
			&p.MainImageURL, &p.PlatformURL, &p.VendorName, &p.BrandName,
			&p.LocationCity, &p.LocationCityRu, &p.LocationState, &p.LocationStateRu,
			&p.VolumeSales, &p.SalesLast30, &p.FavCount, &p.ReviewsCount,
			&p.Rating, &p.GoodRates, &p.PayOrder30Day, &p.QualityScore,
			&p.HasHierConf, &p.FetchedAt, &p.UpdatedAt,
			&p.CsProductID, &p.PushedToCsAt,
			&p.DescriptionRU, &p.DescriptionEN, &p.DescriptionTK,
			&p.Enabled, &p.HiddenAt, &p.Gender, &p.AgeGroup,
		); err != nil {
			return nil, 0, err
		}
		products = append(products, p)
	}
	return products, total, nil
}

// SetProductEnabled - включить/выключить товар (скрытие без удаления).
func (s *Store) SetProductEnabled(id int64, enabled bool) error {
	if enabled {
		_, err := s.Hub.Exec(`UPDATE products SET enabled=1, hidden_at=NULL WHERE id=?`, id)
		return err
	}
	_, err := s.Hub.Exec(`UPDATE products SET enabled=0, hidden_at=? WHERE id=?`, time.Now().Unix(), id)
	return err
}

// BulkSetEnabled - включить/выключить несколько товаров.
func (s *Store) BulkSetEnabled(ids []int64, enabled bool) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	idList := strings.Join(placeholders, ",")
	if enabled {
		_, err := s.Hub.Exec(`UPDATE products SET enabled=1, hidden_at=NULL WHERE id IN (`+idList+`)`, args...)
		return err
	}
	args = append([]interface{}{time.Now().Unix()}, args...)
	_, err := s.Hub.Exec(`UPDATE products SET enabled=0, hidden_at=? WHERE id IN (`+idList+`)`, args...)
	return err
}

// BulkDeleteProducts - удаляет товары и связанные данные (SKU, фото, атрибуты).
func (s *Store) BulkDeleteProducts(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	idList := strings.Join(placeholders, ",")
	s.Hub.Exec(`DELETE FROM product_attrs WHERE product_id IN (`+idList+`)`, args...)
	s.Hub.Exec(`DELETE FROM product_images WHERE product_id IN (`+idList+`)`, args...)
	s.Hub.Exec(`DELETE FROM product_skus WHERE product_id IN (`+idList+`)`, args...)
	_, err := s.Hub.Exec(`DELETE FROM products WHERE id IN (`+idList+`)`, args...)
	return err
}

func (s *Store) GetProductByID(id int64) (*Product, error) {
	var p Product
	err := s.Hub.QueryRow(`
		SELECT id, otapi_id, provider, category_id, title_original, title_ru, title_en, title_tk,
		       translate_status, price_cny, price_tmt, master_quantity, is_fake_quantity,
		       is_sell_allowed, is_expired, is_tmall, IFNULL(main_image_url,''),
		       IFNULL(platform_url,''), vendor_name, brand_name,
		       location_city, location_city_ru, location_state, location_state_ru,
		       volume_sales, sales_last_30days, fav_count, reviews_count,
		       has_hierarchical_conf, fetched_at, updated_at,
		       cs_product_id, pushed_to_cs_at,
		       IFNULL(description_ru,''), IFNULL(description_en,''), IFNULL(description_tk,''),
		       enabled, hidden_at
		FROM products WHERE id=?`, id).Scan(
		&p.ID, &p.OtapiID, &p.Provider, &p.CategoryID,
		&p.TitleOriginal, &p.TitleRu, &p.TitleEn, &p.TitleTk,
		&p.TranslateStatus, &p.PriceCNY, &p.PriceTMT, &p.MasterQuantity,
		&p.IsFakeQty, &p.IsSellAllowed, &p.IsExpired, &p.IsTmall,
		&p.MainImageURL, &p.PlatformURL, &p.VendorName, &p.BrandName,
		&p.LocationCity, &p.LocationCityRu, &p.LocationState, &p.LocationStateRu,
		&p.VolumeSales, &p.SalesLast30, &p.FavCount, &p.ReviewsCount,
		&p.HasHierConf, &p.FetchedAt, &p.UpdatedAt,
		&p.CsProductID, &p.PushedToCsAt,
		&p.DescriptionRU, &p.DescriptionEN, &p.DescriptionTK,
		&p.Enabled, &p.HiddenAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) UpsertProduct(p *Product, rawJSON []byte) (int64, error) {
	result, err := s.Hub.Exec(`
		INSERT INTO products
		  (otapi_id, provider, category_id, title_original, title_ru, title_en,
		   price_cny, price_tmt, master_quantity, is_fake_quantity, is_sell_allowed,
		   is_expired, is_tmall, main_image_url, platform_url, vendor_name, brand_name,
		   volume_sales, has_hierarchical_conf, raw_json, fetched_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		  id=LAST_INSERT_ID(id),
		  title_ru=VALUES(title_ru), title_en=VALUES(title_en),
		  price_cny=VALUES(price_cny), price_tmt=VALUES(price_tmt),
		  master_quantity=VALUES(master_quantity), is_sell_allowed=VALUES(is_sell_allowed),
		  is_expired=VALUES(is_expired), main_image_url=VALUES(main_image_url),
		  raw_json=VALUES(raw_json), updated_at=VALUES(updated_at)`,
		p.OtapiID, p.Provider, p.CategoryID, p.TitleOriginal, p.TitleRu, p.TitleEn,
		p.PriceCNY, p.PriceTMT, p.MasterQuantity, p.IsFakeQty, p.IsSellAllowed,
		p.IsExpired, p.IsTmall, p.MainImageURL, p.PlatformURL, p.VendorName, p.BrandName,
		p.VolumeSales, p.HasHierConf, string(rawJSON), p.FetchedAt, p.UpdatedAt,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) UpsertSKU(productID int64, skuID string, qty int, priceCNY float64, configurators interface{}) error {
	confJSON, _ := json.Marshal(configurators)
	_, err := s.Hub.Exec(`
		INSERT INTO product_skus (product_id, sku_id, quantity, price_cny, configurators)
		VALUES (?,?,?,?,?)
		ON DUPLICATE KEY UPDATE quantity=VALUES(quantity), price_cny=VALUES(price_cny)`,
		productID, skuID, qty, priceCNY, string(confJSON))
	return err
}

func (s *Store) InsertImage(productID int64, imageURL, small, medium, large string, isMain bool, position int) error {
	_, err := s.Hub.Exec(`
		INSERT IGNORE INTO product_images (product_id, url, url_small, url_medium, url_large, is_main, position)
		VALUES (?,?,?,?,?,?,?)`,
		productID, imageURL, small, medium, large, isMain, position)
	return err
}

func (s *Store) InsertAttr(productID int64, pid, vid, name, value string, isConf bool, imgURL string) error {
	var imgVal *string
	if imgURL != "" {
		imgVal = &imgURL
	}
	_, err := s.Hub.Exec(`
		INSERT IGNORE INTO product_attrs (product_id, pid, vid, property_name, value, is_configurator, image_url)
		VALUES (?,?,?,?,?,?,?)`,
		productID, pid, vid, name, value, isConf, imgVal)
	return err
}

// AttrTranslation - перевод атрибута (pid:vid)
type AttrTranslation struct {
	Pid            string `json:"pid"`
	Vid            string `json:"vid"`
	PropertyNameZh string `json:"property_name_zh"`
	ValueZh        string `json:"value_zh"`
	PropertyNameRu string `json:"property_name_ru"`
	ValueRu        string `json:"value_ru"`
	TranslatedAt   int64  `json:"translated_at"`
}

// GetUntranslatedAttrs - уникальные (pid, vid) без перевода
func (s *Store) GetUntranslatedAttrs(limit int) ([]AttrTranslation, error) {
	rows, err := s.Hub.Query(`
		SELECT DISTINCT a.pid, a.vid, a.property_name, a.value
		FROM product_attrs a
		LEFT JOIN attr_translations t ON t.pid = a.pid AND t.vid = a.vid
		WHERE t.pid IS NULL
		  AND a.pid != '' AND a.vid != ''
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AttrTranslation
	for rows.Next() {
		var at AttrTranslation
		rows.Scan(&at.Pid, &at.Vid, &at.PropertyNameZh, &at.ValueZh)
		result = append(result, at)
	}
	return result, nil
}

// SaveAttrTranslation - сохраняет перевод pid:vid
func (s *Store) SaveAttrTranslation(pid, vid, nameZh, nameRu, valueZh, valueRu string) error {
	_, err := s.Hub.Exec(`
		INSERT INTO attr_translations (pid, vid, property_name_zh, property_name_ru, value_zh, value_ru, translated_at)
		VALUES (?,?,?,?,?,?,UNIX_TIMESTAMP())
		ON DUPLICATE KEY UPDATE property_name_ru=VALUES(property_name_ru), value_ru=VALUES(value_ru), translated_at=VALUES(translated_at)`,
		pid, vid, nameZh, nameRu, valueZh, valueRu)
	return err
}

// GetAttrTranslationsMap - карта pid:vid -> translation для быстрого lookup
func (s *Store) GetAttrTranslationsMap() (map[string]AttrTranslation, error) {
	rows, err := s.Hub.Query(`SELECT pid, vid, property_name_zh, value_zh, property_name_ru, value_ru FROM attr_translations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]AttrTranslation)
	for rows.Next() {
		var at AttrTranslation
		rows.Scan(&at.Pid, &at.Vid, &at.PropertyNameZh, &at.ValueZh, &at.PropertyNameRu, &at.ValueRu)
		result[at.Pid+":"+at.Vid] = at
	}
	return result, nil
}

func (s *Store) GetGlobalMarkup() (*MarkupRule, error) {
	var m MarkupRule
	err := s.Hub.QueryRow(`SELECT id, scope_type, scope_id, markup_pct, fixed_addon, exchange_rate, is_active, notes
		FROM markup_rules WHERE scope_type='global' LIMIT 1`).Scan(
		&m.ID, &m.ScopeType, &m.ScopeID, &m.MarkupPct, &m.FixedAddon, &m.ExchangeRate, &m.IsActive, &m.Notes)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) CalculatePriceTMT(priceCNY float64, categoryID, productID string) float64 {
	return s.CalculatePriceTMTWithWeight(priceCNY, 0, categoryID, productID)
}

func (s *Store) CalculatePriceTMTWithWeight(priceCNY, weightKg float64, categoryID, productID string) float64 {
	var markupPct, fixedAddon, exchangeRate float64
	exchangeRate = 0.57
	markupPct = 35.0

	s.Hub.QueryRow(`SELECT markup_pct, fixed_addon, IFNULL(exchange_rate, 0.57)
		FROM markup_rules WHERE scope_type='global' AND is_active=1 LIMIT 1`).
		Scan(&markupPct, &fixedAddon, &exchangeRate)

	var catPct, catFixed float64
	err := s.Hub.QueryRow(`SELECT markup_pct, fixed_addon FROM markup_rules
		WHERE scope_type='category' AND scope_id=? AND is_active=1 LIMIT 1`, categoryID).
		Scan(&catPct, &catFixed)
	if err == nil {
		markupPct = catPct
		fixedAddon = catFixed
	}

	var prodPct, prodFixed float64
	err = s.Hub.QueryRow(`SELECT markup_pct, fixed_addon FROM markup_rules
		WHERE scope_type='product' AND scope_id=? AND is_active=1 LIMIT 1`, productID).
		Scan(&prodPct, &prodFixed)
	if err == nil {
		markupPct = prodPct
		fixedAddon = prodFixed
	}

	// Доставка: если включена и есть вес, добавляем стоимость доставки к цене в CNY
	baseCNY := priceCNY
	var deliveryIncluded string
	s.Hub.QueryRow(`SELECT v FROM settings WHERE k='delivery_included'`).Scan(&deliveryIncluded)
	if deliveryIncluded == "1" && weightKg > 0 {
		var deliveryCostPerKg, usdToCNY float64
		deliveryCostPerKg = 7.0
		usdToCNY = 7.3
		s.Hub.QueryRow(`SELECT v FROM settings WHERE k='delivery_cost_per_kg'`).Scan(&deliveryCostPerKg)
		s.Hub.QueryRow(`SELECT v FROM settings WHERE k='usd_to_cny'`).Scan(&usdToCNY)
		baseCNY += weightKg * deliveryCostPerKg * usdToCNY
	}

	tmt := baseCNY*exchangeRate*(1+markupPct/100) + fixedAddon
	return float64(int(tmt*10+0.5)) / 10
}

func (s *Store) CreateSyncJob(jobType, categoryID, triggeredBy string) (int64, error) {
	result, err := s.Hub.Exec(`INSERT INTO sync_jobs (job_type, category_id, status, triggered_by)
		VALUES (?,?,?,?)`, jobType, categoryID, "pending", triggeredBy)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) UpdateSyncJob(id int64, status string, processed, skipped, errors, apiReqs int, logText string) error {
	now := time.Now().Unix()
	_, err := s.Hub.Exec(`UPDATE sync_jobs SET status=?,
		started_at=IFNULL(started_at,?),
		finished_at=CASE WHEN ? IN ('done','error') THEN ? ELSE finished_at END,
		items_processed=?, items_skipped=?, errors_count=?,
		api_requests_made=?, log_text=? WHERE id=?`,
		status, now, status, now, processed, skipped, errors, apiReqs, logText, id)
	return err
}

func (s *Store) GetRecentSyncJobs(limit int) ([]SyncJob, error) {
	rows, err := s.Hub.Query(`SELECT sj.id, sj.job_type, IFNULL(sj.category_id,''),
		COALESCE(NULLIF(c.name_ru,''), NULLIF(c.name_en,''), sj.category_id, ''),
		sj.status, sj.started_at, sj.finished_at, sj.items_processed, sj.items_skipped,
		sj.errors_count, sj.api_requests_made, IFNULL(sj.log_text,''), sj.triggered_by
		FROM sync_jobs sj
		LEFT JOIN categories c ON c.id = sj.category_id
		ORDER BY sj.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []SyncJob
	for rows.Next() {
		var j SyncJob
		if err := rows.Scan(&j.ID, &j.JobType, &j.CategoryID, &j.CategoryName, &j.Status,
			&j.StartedAt, &j.FinishedAt, &j.ItemsProcessed, &j.ItemsSkipped,
			&j.ErrorsCount, &j.APIRequests, &j.Log, &j.TriggeredBy); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

type DashboardStats struct {
	TotalProducts     int
	TotalCategories   int
	EnabledCategories int
	MappedCategories  int
	PendingPush       int
	PendingTranslate  int
	PushedProducts    int
	LastSync          *SyncJob
}

func (s *Store) GetDashboardStats() (*DashboardStats, error) {
	var stats DashboardStats
	s.Hub.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&stats.TotalProducts)
	s.Hub.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&stats.TotalCategories)
	s.Hub.QueryRow(`SELECT COUNT(*) FROM category_config WHERE enabled=1`).Scan(&stats.EnabledCategories)
	s.Hub.QueryRow(`SELECT COUNT(*) FROM category_map`).Scan(&stats.MappedCategories)
	s.Hub.QueryRow(`SELECT COUNT(*) FROM push_queue WHERE status='pending'`).Scan(&stats.PendingPush)
	s.Hub.QueryRow(`SELECT COUNT(*) FROM products WHERE translate_status IN ('pending','') OR translate_status IS NULL`).Scan(&stats.PendingTranslate)
	s.Hub.QueryRow(`SELECT COUNT(*) FROM products WHERE pushed_to_cs_at IS NOT NULL`).Scan(&stats.PushedProducts)

	jobs, _ := s.GetRecentSyncJobs(1)
	if len(jobs) > 0 {
		stats.LastSync = &jobs[0]
	}
	return &stats, nil
}

type CategoryWithConfig struct {
	ID               string
	Provider         string
	Name             string // name_ru, fallback name_en, name_zh
	NameEn           string
	NameZh           string
	ParentID         string
	IsParent         bool
	ItemCount        int
	Enabled          bool
	SyncSchedule     string
	MaxProducts      int
	LastSyncedAt     *int64
	ProductsImported int
	CSCategoryID     *int
	Notes            string
	LocalCount       int
	Path             string `json:",omitempty"` // полный путь "Родитель / Категория" (заполняется в хендлерах)
}

type CategoryMapping struct {
	OTCategoryID     string
	OTCategoryName   string // human-readable name from categories table
	CSCategoryID     int
	CSCategoryName   string
	Notes            string
	WeightG          int    // Стандартный вес в граммах (0 = не задан)
	MOQ              int    // Мин. кол-во в заказе (0 = наследовать от родителя / дефолт 1)
	MinPriceCNY      int    // Мин. цена для API фильтра (0 = отключено)
	MaxPriceCNY      int    // Макс. цена для API фильтра и отсева аномалий (0 = отключено)
	MinVolume        int    // Мин. продаж для API фильтра (0 = отключено)
	TitleKeyword     string // Ключевое слово в названии → использовать AltCSCategoryID
	AltCSCategoryID  int    // Альтернативная CS категория если TitleKeyword найдено в названии
	CSCategoryMale   int    // CS-категория для товаров с gender=male (0 = использовать базовую)
	CSCategoryFemale int    // CS-категория для товаров с gender=female (0 = использовать базовую)
	OTCategoryPath   string `json:",omitempty"` // полный путь OT "Родитель / Категория"
	CSCategoryPath   string `json:",omitempty"` // полный путь CS "Родитель / Категория"
}

// ResolveCategoryID - возвращает CS category_id с учётом keyword-фильтра по названию товара.
// TitleKeyword может содержать несколько слов через "|", напр. "шорт|short|短裤".
// Если titleRu не пустой — проверяется titleRu, иначе только titleOrig.
func (m *CategoryMapping) ResolveCategoryID(titleRu, titleOrig, gender string) int {
	// Роутинг по полу (если для маппинга заданы пол-цели). Унисекс/пусто → базовая.
	if gender == "male" && m.CSCategoryMale != 0 {
		return m.CSCategoryMale
	}
	if gender == "female" && m.CSCategoryFemale != 0 {
		return m.CSCategoryFemale
	}
	if m.TitleKeyword != "" && m.AltCSCategoryID != 0 {
		titleOrigL := strings.ToLower(titleOrig)
		titleRuL := strings.ToLower(titleRu)
		for _, kw := range strings.Split(m.TitleKeyword, "|") {
			kw = strings.TrimSpace(strings.ToLower(kw))
			if kw == "" {
				continue
			}
			if titleRu != "" && strings.Contains(titleRuL, kw) {
				return m.AltCSCategoryID
			}
			if strings.Contains(titleOrigL, kw) {
				return m.AltCSCategoryID
			}
		}
	}
	return m.CSCategoryID
}

func (s *Store) GetCategoryMappings() ([]CategoryMapping, error) {
	rows, err := s.Hub.Query(`SELECT cm.otapi_category_id, IFNULL(c.name_ru, cm.otapi_category_id),
		cm.cs_category_id, cm.cs_category_name, cm.notes, cm.weight_g, IFNULL(cm.moq,0),
		IFNULL(cm.min_price_cny,0), IFNULL(cm.max_price_cny,0), IFNULL(cm.min_volume,0),
		IFNULL(cm.title_keyword,''), IFNULL(cm.alt_cs_category_id,0),
		IFNULL(cm.cs_category_male,0), IFNULL(cm.cs_category_female,0)
		FROM category_map cm
		LEFT JOIN categories c ON c.id = cm.otapi_category_id
		ORDER BY cm.cs_category_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CategoryMapping
	for rows.Next() {
		var m CategoryMapping
		rows.Scan(&m.OTCategoryID, &m.OTCategoryName, &m.CSCategoryID, &m.CSCategoryName, &m.Notes, &m.WeightG, &m.MOQ,
			&m.MinPriceCNY, &m.MaxPriceCNY, &m.MinVolume, &m.TitleKeyword, &m.AltCSCategoryID,
			&m.CSCategoryMale, &m.CSCategoryFemale)
		result = append(result, m)
	}
	return result, nil
}

func (s *Store) GetCategoryMappingByOT(otCatID string) (*CategoryMapping, error) {
	var m CategoryMapping
	err := s.Hub.QueryRow(`SELECT otapi_category_id, cs_category_id, cs_category_name, notes, weight_g,
		IFNULL(min_price_cny,0), IFNULL(max_price_cny,0), IFNULL(min_volume,0),
		IFNULL(title_keyword,''), IFNULL(alt_cs_category_id,0),
		IFNULL(cs_category_male,0), IFNULL(cs_category_female,0)
		FROM category_map WHERE otapi_category_id=?`, otCatID).Scan(
		&m.OTCategoryID, &m.CSCategoryID, &m.CSCategoryName, &m.Notes, &m.WeightG,
		&m.MinPriceCNY, &m.MaxPriceCNY, &m.MinVolume, &m.TitleKeyword, &m.AltCSCategoryID,
		&m.CSCategoryMale, &m.CSCategoryFemale)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) UpdateCategoryKeyword(otCatID, keyword string, altCatID int) error {
	_, err := s.Hub.Exec(`UPDATE category_map SET title_keyword=?, alt_cs_category_id=? WHERE otapi_category_id=?`,
		keyword, altCatID, otCatID)
	return err
}

func (s *Store) GetCategoryFilters(otCatID string) (minPriceCNY, maxPriceCNY, minVolume int) {
	s.Hub.QueryRow(`SELECT IFNULL(min_price_cny,0), IFNULL(max_price_cny,0), IFNULL(min_volume,0)
		FROM category_map WHERE otapi_category_id=?`, otCatID).Scan(&minPriceCNY, &maxPriceCNY, &minVolume)
	return
}

func (s *Store) UpdateCategoryGenderCats(otCatID string, male, female int) error {
	_, err := s.Hub.Exec(`UPDATE category_map SET cs_category_male=?, cs_category_female=? WHERE otapi_category_id=?`,
		male, female, otCatID)
	return err
}

func (s *Store) UpdateCategoryPriceFilters(otCatID string, minPriceCNY, maxPriceCNY, minVolume int) error {
	_, err := s.Hub.Exec(`UPDATE category_map SET min_price_cny=?, max_price_cny=?, min_volume=? WHERE otapi_category_id=?`,
		minPriceCNY, maxPriceCNY, minVolume, otCatID)
	return err
}

func (s *Store) UpsertCategoryMapping(otCatID string, csCatID int, csCatName, notes string) error {
	_, err := s.Hub.Exec(`INSERT INTO category_map (otapi_category_id, cs_category_id, cs_category_name, notes)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE cs_category_id=VALUES(cs_category_id), cs_category_name=VALUES(cs_category_name), notes=VALUES(notes)`,
		otCatID, csCatID, csCatName, notes)
	return err
}

func (s *Store) UpdateCategoryWeight(otCatID string, weightG int) error {
	_, err := s.Hub.Exec(`UPDATE category_map SET weight_g=? WHERE otapi_category_id=?`, weightG, otCatID)
	return err
}

// UpdateCategoryMOQ - задаёт MOQ (мин. кол-во в заказе) для строки маппинга.
func (s *Store) UpdateCategoryMOQ(otCatID string, moq int) error {
	_, err := s.Hub.Exec(`UPDATE category_map SET moq=? WHERE otapi_category_id=?`, moq, otCatID)
	return err
}

// ResolveMOQ - возвращает MOQ для CS-категории с наследованием вверх по дереву
// (cs_categories.parent_id): берётся ближайшая категория с заданным moq>0.
// Если ни у одной из родительских категорий MOQ не задан — дефолт 1.
func (s *Store) ResolveMOQ(csCategoryID int) int {
	cur := csCategoryID
	for depth := 0; depth < 20 && cur > 0; depth++ {
		var moq int
		s.Hub.QueryRow(`SELECT IFNULL(MAX(moq),0) FROM category_map WHERE cs_category_id=? AND moq>0`, cur).Scan(&moq)
		if moq > 0 {
			return moq
		}
		var parent int
		if err := s.Hub.QueryRow(`SELECT parent_id FROM cs_categories WHERE category_id=?`, cur).Scan(&parent); err != nil {
			break
		}
		cur = parent
	}
	return 1
}

func (s *Store) GetCategoryWeightG(otCatID string) int {
	var w int
	s.Hub.QueryRow(`SELECT weight_g FROM category_map WHERE otapi_category_id=?`, otCatID).Scan(&w)
	return w
}

func (s *Store) DeleteCategoryMapping(otCatID string) error {
	_, err := s.Hub.Exec(`DELETE FROM category_map WHERE otapi_category_id=?`, otCatID)
	return err
}

// --- Settings (key-value конфигурация) ---

// GetSetting - читает значение настройки из таблицы settings.
func (s *Store) GetSetting(key string) string {
	var val string
	s.Hub.QueryRow(`SELECT value FROM settings WHERE key_name=?`, key).Scan(&val)
	return val
}

// GetAllSettings - все настройки как map.
func (s *Store) GetAllSettings() map[string]string {
	rows, err := s.Hub.Query(`SELECT key_name, value FROM settings`)
	if err != nil {
		return map[string]string{}
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		result[k] = v
	}
	return result
}

// SaveSetting - сохраняет настройку в таблицу settings.
func (s *Store) SaveSetting(key, value string) error {
	_, err := s.Hub.Exec(`INSERT INTO settings (key_name, value, updated_at) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE value=VALUES(value), updated_at=VALUES(updated_at)`,
		key, value, time.Now().Unix())
	return err
}

// --- CS-Cart Categories Cache ---

type CSCartCategory struct {
	CategoryID int
	ParentID   int
	Name       string
	ParentName string
	Status     string // A=active, H=hidden
	Path       string `json:",omitempty"` // полный путь "Родитель / Категория" (заполняется в apiMappingPage)
}

// CacheCSCartCategories - сохраняет список CS-Cart категорий в hub БД.
func (s *Store) CacheCSCartCategories(categories []CSCartCategory) error {
	now := time.Now().Unix()
	// Build id→name and id→parentID maps for all categories
	idToName := make(map[int]string, len(categories))
	idToParent := make(map[int]int, len(categories))
	for _, c := range categories {
		idToName[c.CategoryID] = c.Name
		idToParent[c.CategoryID] = c.ParentID
	}
	// Build full ancestor path for each category (e.g. "Женская одежда / Брюки и шорты")
	var buildPath func(id int) string
	buildPath = func(id int) string {
		parentID := idToParent[id]
		if parentID == 0 {
			return ""
		}
		grandPath := buildPath(parentID)
		if grandPath != "" {
			return grandPath + " / " + idToName[parentID]
		}
		return idToName[parentID]
	}
	for _, c := range categories {
		parentName := buildPath(c.CategoryID)
		s.Hub.Exec(`INSERT INTO cs_categories (category_id, parent_id, parent_name, name, status, fetched_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE name=VALUES(name), parent_id=VALUES(parent_id), parent_name=VALUES(parent_name), status=VALUES(status), fetched_at=VALUES(fetched_at)`,
			c.CategoryID, c.ParentID, parentName, c.Name, c.Status, now)
	}
	return nil
}

// GetCSCartCategories - читает закэшированные CS-Cart категории.
func (s *Store) GetCSCartCategories() ([]CSCartCategory, error) {
	rows, err := s.Hub.Query(`SELECT category_id, parent_id, IFNULL(parent_name,''), name, IFNULL(status,'A') FROM cs_categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CSCartCategory
	for rows.Next() {
		var c CSCartCategory
		rows.Scan(&c.CategoryID, &c.ParentID, &c.ParentName, &c.Name, &c.Status)
		result = append(result, c)
	}
	return result, nil
}


// --- CS-Cart features cache ---

// SaveCSFeatures сохраняет список CS-Cart характеристик + вариантов в кеш.
func (s *Store) SaveCSFeatures(features []cscart.FeatureInfo) error {
	now := time.Now().Unix()
	tx, err := s.Hub.Begin()
	if err != nil {
		return err
	}
	tx.Exec(`DELETE FROM cs_features_cache`)
	tx.Exec(`DELETE FROM cs_feature_variants_cache`)
	for _, f := range features {
		tx.Exec(`INSERT INTO cs_features_cache (feature_id, feature_name, feature_type, fetched_at) VALUES (?,?,?,?)`,
			f.FeatureID, f.Name, f.FeatureType, now)
		for _, v := range f.Variants {
			tx.Exec(`INSERT INTO cs_feature_variants_cache (variant_id, feature_id, variant_value) VALUES (?,?,?)`,
				v.VariantID, f.FeatureID, v.Value)
		}
	}
	return tx.Commit()
}

// AttrPidMapping — маппинг OT-атрибута (pid) на CS-Cart характеристику.
type AttrPidMapping struct {
	PID         string `json:"pid"`
	NameRU      string `json:"name_ru"`
	NameZH      string `json:"name_zh"`
	CSFeatureID int    `json:"cs_feature_id"`
	CSFeatureName string `json:"cs_feature_name"`
	ValuesTotal int    `json:"values_total"`
	ValuesMapped int   `json:"values_mapped"`
}

// GetAttrPidMappings возвращает все уникальные OT-атрибуты с текущим маппингом на CS-Cart фичи.
func (s *Store) GetAttrPidMappings() ([]AttrPidMapping, error) {
	rows, err := s.Hub.Query(`
		SELECT
			pa.pid,
			IFNULL(MAX(t.property_name_ru), '') as name_ru,
			IFNULL(MAX(t.property_name_zh), '') as name_zh,
			IFNULL(MAX(CASE WHEN m.cs_feature_id > 0 THEN m.cs_feature_id END), 0) as cs_feature_id,
			IFNULL(MAX(CASE WHEN m.cs_feature_id > 0 THEN f.feature_name END), '') as cs_feature_name,
			COUNT(DISTINCT pa.vid) as values_total,
			COUNT(DISTINCT CASE WHEN m.cs_feature_id > 0 THEN pa.vid END) as values_mapped
		FROM product_attrs pa
		LEFT JOIN attr_translations t ON t.pid=pa.pid AND t.vid=pa.vid
		LEFT JOIN attr_cs_mapping m ON m.pid=pa.pid AND m.vid=pa.vid
		LEFT JOIN cs_features_cache f ON f.feature_id=m.cs_feature_id
		GROUP BY pa.pid
		ORDER BY values_total DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AttrPidMapping
	for rows.Next() {
		var a AttrPidMapping
		rows.Scan(&a.PID, &a.NameRU, &a.NameZH, &a.CSFeatureID, &a.CSFeatureName, &a.ValuesTotal, &a.ValuesMapped)
		result = append(result, a)
	}
	return result, nil
}

// SetAttrPidFeature устанавливает cs_feature_id для всех значений данного pid в attr_cs_mapping.
// Если cs_feature_id=0 — сбрасывает маппинг (устанавливает 0).
func (s *Store) SetAttrPidFeature(pid string, csFeatureID int) error {
	_, err := s.Hub.Exec(`
		UPDATE attr_cs_mapping SET cs_feature_id=?, cs_variant_id=0, canonical_value='', mapped_at=?
		WHERE pid=?`, csFeatureID, time.Now().Unix(), pid)
	return err
}

// GetCSFeatures читает закэшированные CS-Cart характеристики с вариантами.
func (s *Store) GetCSFeatures() ([]cscart.FeatureInfo, error) {
	rows, err := s.Hub.Query(`SELECT feature_id, feature_name, feature_type FROM cs_features_cache ORDER BY feature_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []cscart.FeatureInfo
	for rows.Next() {
		var f cscart.FeatureInfo
		rows.Scan(&f.FeatureID, &f.Name, &f.FeatureType)
		result = append(result, f)
	}
	// Загружаем варианты для каждой фичи
	for i, f := range result {
		vrows, err := s.Hub.Query(`SELECT variant_id, variant_value FROM cs_feature_variants_cache WHERE feature_id=? ORDER BY variant_value`, f.FeatureID)
		if err != nil {
			continue
		}
		for vrows.Next() {
			var v cscart.FeatureVariant
			vrows.Scan(&v.VariantID, &v.Value)
			result[i].Variants = append(result[i].Variants, v)
		}
		vrows.Close()
	}
	return result, nil
}

// ── Расширенный маппинг атрибутов (suggest + verified) ──────────────────────

// AttrPidMappingExt — расширенная версия с AI-suggest и верификацией.
type AttrPidMappingExt struct {
	PID                string `json:"pid"`
	NameRU             string `json:"name_ru"`
	NameZH             string `json:"name_zh"`
	CSFeatureID        int    `json:"cs_feature_id"`
	CSFeatureName      string `json:"cs_feature_name"`
	CSFeatureType      string `json:"cs_feature_type"`
	ValuesTotal        int    `json:"values_total"`
	ValuesMapped       int    `json:"values_mapped"`
	SuggestFeatureID   int    `json:"suggest_feature_id"`
	SuggestScore       int    `json:"suggest_score"`
	SuggestFeatureName string `json:"suggest_feature_name"`
	Verified           int    `json:"verified"`
}

// GetAttrPidMappingsExt возвращает все OT-атрибуты с расширенной информацией.
func (s *Store) GetAttrPidMappingsExt() ([]AttrPidMappingExt, error) {
	rows, err := s.Hub.Query(`
		SELECT
			pa.pid,
			IFNULL(MAX(t.property_name_ru), '') as name_ru,
			IFNULL(MAX(t.property_name_zh), '') as name_zh,
			IFNULL((SELECT m2.cs_feature_id FROM attr_cs_mapping m2 WHERE m2.pid=pa.pid AND m2.cs_feature_id > 0 GROUP BY m2.cs_feature_id ORDER BY COUNT(*) DESC LIMIT 1), 0) as cs_feature_id,
			IFNULL((SELECT fc2.feature_name FROM attr_cs_mapping m2 JOIN cs_features_cache fc2 ON fc2.feature_id=m2.cs_feature_id WHERE m2.pid=pa.pid AND m2.cs_feature_id > 0 GROUP BY m2.cs_feature_id ORDER BY COUNT(*) DESC LIMIT 1), '') as cs_feature_name,
			IFNULL((SELECT fc2.feature_type FROM attr_cs_mapping m2 JOIN cs_features_cache fc2 ON fc2.feature_id=m2.cs_feature_id WHERE m2.pid=pa.pid AND m2.cs_feature_id > 0 GROUP BY m2.cs_feature_id ORDER BY COUNT(*) DESC LIMIT 1), '') as cs_feature_type,
			COUNT(DISTINCT pa.vid) as values_total,
			COUNT(DISTINCT CASE WHEN m.cs_feature_id > 0 AND (
				(SELECT fc3.feature_type FROM cs_features_cache fc3 WHERE fc3.feature_id=m.cs_feature_id) = 'T'
				OR m.cs_variant_id > 0
			) THEN pa.vid END) as values_mapped,
			IFNULL(MAX(m.suggest_feature_id), 0) as suggest_feature_id,
			IFNULL(MAX(m.suggest_score), 0) as suggest_score,
			IFNULL(MAX(CASE WHEN m.suggest_feature_id > 0 THEN sf.feature_name END), '') as suggest_feature_name,
			IFNULL(MAX(m.verified), 0) as verified
		FROM product_attrs pa
		LEFT JOIN attr_translations t ON t.pid=pa.pid AND t.vid=pa.vid
		LEFT JOIN attr_cs_mapping m ON m.pid=pa.pid AND m.vid=pa.vid
		LEFT JOIN cs_features_cache sf ON sf.feature_id=m.suggest_feature_id
		GROUP BY pa.pid
		ORDER BY values_total DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AttrPidMappingExt
	for rows.Next() {
		var a AttrPidMappingExt
		rows.Scan(&a.PID, &a.NameRU, &a.NameZH, &a.CSFeatureID, &a.CSFeatureName, &a.CSFeatureType,
			&a.ValuesTotal, &a.ValuesMapped, &a.SuggestFeatureID, &a.SuggestScore,
			&a.SuggestFeatureName, &a.Verified)
		result = append(result, a)
	}
	return result, nil
}

// AttrVidMapping — маппинг конкретного значения (vid) атрибута.
type AttrVidMapping struct {
	PID                 string `json:"pid"`
	VID                 string `json:"vid"`
	ValueRU             string `json:"value_ru"`
	ValueZH             string `json:"value_zh"`
	CSFeatureID         int    `json:"cs_feature_id"`
	CSFeatureType       string `json:"cs_feature_type"`
	CSVariantID         int    `json:"cs_variant_id"`
	VariantValue        string `json:"variant_value"`
	SuggestVariantID    int    `json:"suggest_variant_id"`
	SuggestVariantValue string `json:"suggest_variant_value"`
	Verified            int    `json:"verified"`
}

// GetAttrVidMappings возвращает все vid-маппинги для заданного pid.
func (s *Store) GetAttrVidMappings(pid string) ([]AttrVidMapping, error) {
	rows, err := s.Hub.Query(`
		SELECT
			pa.pid, pa.vid,
			IFNULL(t.value_ru, '') as value_ru,
			IFNULL(t.value_zh, '') as value_zh,
			IFNULL(m.cs_feature_id, 0) as cs_feature_id,
			IFNULL(fc.feature_type, '') as cs_feature_type,
			IFNULL(m.cs_variant_id, 0) as cs_variant_id,
			IFNULL(v.variant_value, '') as variant_value,
			IFNULL(m.suggest_variant_id, 0) as suggest_variant_id,
			IFNULL(sv.variant_value, '') as suggest_variant_value,
			IFNULL(m.verified, 0) as verified
		FROM product_attrs pa
		LEFT JOIN attr_translations t ON t.pid=pa.pid AND t.vid=pa.vid
		LEFT JOIN attr_cs_mapping m ON m.pid=pa.pid AND m.vid=pa.vid
		LEFT JOIN cs_features_cache fc ON fc.feature_id=m.cs_feature_id
		LEFT JOIN cs_feature_variants_cache v ON v.variant_id=m.cs_variant_id
		LEFT JOIN cs_feature_variants_cache sv ON sv.variant_id=m.suggest_variant_id
		WHERE pa.pid=?
		GROUP BY pa.pid, pa.vid
		ORDER BY value_ru`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AttrVidMapping
	for rows.Next() {
		var a AttrVidMapping
		rows.Scan(&a.PID, &a.VID, &a.ValueRU, &a.ValueZH,
			&a.CSFeatureID, &a.CSFeatureType, &a.CSVariantID, &a.VariantValue,
			&a.SuggestVariantID, &a.SuggestVariantValue, &a.Verified)
		result = append(result, a)
	}
	return result, nil
}

// SetAttrVidVariant устанавливает cs_variant_id для конкретного pid:vid.
func (s *Store) SetAttrVidVariant(pid, vid string, csFeatureID, csVariantID int) error {
	_, err := s.Hub.Exec(`
		UPDATE attr_cs_mapping SET cs_variant_id=?, cs_feature_id=?, mapped_at=?
		WHERE pid=? AND vid=?`, csVariantID, csFeatureID, time.Now().Unix(), pid, vid)
	return err
}

// SetAttrVerified устанавливает флаг verified для всех строк pid.
func (s *Store) SetAttrVerified(pid string, verified int) error {
	_, err := s.Hub.Exec(`UPDATE attr_cs_mapping SET verified=? WHERE pid=?`, verified, pid)
	return err
}

// SetAttrSuggest сохраняет AI-предложение маппинга фичи для pid.
// Сначала создаёт строки в attr_cs_mapping для всех vid данного pid (если их нет),
// чтобы следующий запуск GetPidsNeedingSuggest не возвращал этот pid снова.
func (s *Store) SetAttrSuggest(pid string, suggestFeatureID, suggestScore int) error {
	// Гарантируем наличие строк — даже если suggest ниже порога, pid не будет обрабатываться повторно
	_, err := s.Hub.Exec(`
		INSERT IGNORE INTO attr_cs_mapping (pid, vid, cs_feature_id, cs_variant_id, canonical_value, mapped_at)
		SELECT DISTINCT pid, vid, 0, 0, '', 0 FROM product_attrs WHERE pid=?`, pid)
	if err != nil {
		return err
	}
	if suggestFeatureID > 0 {
		_, err = s.Hub.Exec(`
			UPDATE attr_cs_mapping SET suggest_feature_id=?, suggest_score=?
			WHERE pid=? AND cs_feature_id=0`, suggestFeatureID, suggestScore, pid)
	}
	return err
}

// SetAttrVidSuggestVariant сохраняет AI-предложение варианта для pid:vid.
func (s *Store) SetAttrVidSuggestVariant(pid, vid string, suggestVariantID int) error {
	_, err := s.Hub.Exec(`
		UPDATE attr_cs_mapping SET suggest_variant_id=?
		WHERE pid=? AND vid=?`, suggestVariantID, pid, vid)
	return err
}

// AcceptAttrSuggest копирует suggest_feature_id в cs_feature_id для всех строк pid.
func (s *Store) AcceptAttrSuggest(pid string) error {
	_, err := s.Hub.Exec(`
		UPDATE attr_cs_mapping
		SET cs_feature_id=suggest_feature_id, cs_variant_id=0, suggest_feature_id=0, suggest_score=0, verified=1, mapped_at=?
		WHERE pid=? AND suggest_feature_id > 0`, time.Now().Unix(), pid)
	return err
}

// AcceptAttrValueSuggests принимает все suggest_variant_id для данного pid.
func (s *Store) AcceptAttrValueSuggests(pid string) error {
	_, err := s.Hub.Exec(`
		UPDATE attr_cs_mapping
		SET cs_variant_id=suggest_variant_id, suggest_variant_id=0, mapped_at=?
		WHERE pid=? AND suggest_variant_id > 0`, time.Now().Unix(), pid)
	return err
}

// GetPidsNeedingSuggest возвращает pid-ы с переводом, без маппинга и без suggest,
// у которых ЕЩЁ НЕТ строк в attr_cs_mapping (т.е. suggest ни разу не запускался).
// Пиды с существующими строками (даже suggest_feature_id=0) уже обработаны — пропускаем.
func (s *Store) GetPidsNeedingSuggest(limit int) ([]struct{ PID, NameRU string }, error) {
	rows, err := s.Hub.Query(`
		SELECT pa.pid, MAX(t.property_name_ru) as name_ru
		FROM product_attrs pa
		JOIN attr_translations t ON t.pid=pa.pid AND t.vid=pa.vid
		WHERE t.property_name_ru != ''
		  AND NOT EXISTS (SELECT 1 FROM attr_cs_mapping m WHERE m.pid=pa.pid)
		GROUP BY pa.pid
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []struct{ PID, NameRU string }
	for rows.Next() {
		var r struct{ PID, NameRU string }
		rows.Scan(&r.PID, &r.NameRU)
		result = append(result, r)
	}
	return result, nil
}
