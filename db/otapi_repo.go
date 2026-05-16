package db

import (
	"encoding/json"
	"time"
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
	VolueSales      int
	HasHierConf     bool
	FetchedAt       int64
	UpdatedAt       int64
	PushedToCsAt    *int64
	CsProductID     *int
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
		INSERT INTO categories (id, provider, external_id, parent_id, name_ru, name_en, is_parent, fetched_at)
		VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE name_ru=VALUES(name_ru), name_en=VALUES(name_en), fetched_at=VALUES(fetched_at)`,
		id, provider, externalID, parentID, nameRu, nameEn, isParent, time.Now().Unix())
	return err
}

func (s *Store) GetCategoriesWithConfig() ([]CategoryWithConfig, error) {
	rows, err := s.Hub.Query(`
		SELECT c.id, c.provider, c.name_ru, c.item_count,
		       IFNULL(cc.enabled, 0), IFNULL(cc.sync_schedule,'manual'),
		       IFNULL(cc.max_products, 500), cc.last_synced_at,
		       IFNULL(cc.products_imported, 0), cc.cs_category_id, IFNULL(cc.notes,''),
		       (SELECT COUNT(*) FROM products p WHERE p.category_id = c.id) AS local_count
		FROM categories c
		LEFT JOIN category_config cc ON cc.category_id = c.id
		WHERE c.parent_id IS NULL
		ORDER BY c.provider, c.name_ru`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CategoryWithConfig
	for rows.Next() {
		var cc CategoryWithConfig
		if err := rows.Scan(&cc.ID, &cc.Provider, &cc.Name, &cc.ItemCount,
			&cc.Enabled, &cc.SyncSchedule, &cc.MaxProducts, &cc.LastSyncedAt,
			&cc.ProductsImported, &cc.CSCategoryID, &cc.Notes, &cc.LocalCount); err != nil {
			return nil, err
		}
		result = append(result, cc)
	}
	return result, nil
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

// ProductFilter - фильтры для списка товаров.
type ProductFilter struct {
	CategoryID      string
	Provider        string
	TranslateStatus string
	Search          string
	SortBy          string // volume_sales, sales_last_30days, price_tmt, fetched_at
	PushedOnly      bool
	UnpushedOnly    bool
}

func (f ProductFilter) orderClause() string {
	switch f.SortBy {
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

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	s.Hub.QueryRow("SELECT COUNT(*) FROM products WHERE "+where, countArgs...).Scan(&total)

	queryArgs := append(args, limit, offset)
	rows, err := s.Hub.Query(`
		SELECT id, otapi_id, provider, category_id, title_original, title_ru, title_en, title_tk,
		       translate_status, price_cny, price_tmt, master_quantity, is_fake_quantity,
		       is_sell_allowed, is_expired, is_tmall, IFNULL(main_image_url,''),
		       IFNULL(platform_url,''), vendor_name, brand_name, volume_sales,
		       has_hierarchical_conf, fetched_at, updated_at
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
			&p.VolueSales, &p.HasHierConf, &p.FetchedAt, &p.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		products = append(products, p)
	}
	return products, total, nil
}

func (s *Store) GetProductByID(id int64) (*Product, error) {
	var p Product
	err := s.Hub.QueryRow(`
		SELECT id, otapi_id, provider, category_id, title_original, title_ru, title_en, title_tk,
		       translate_status, price_cny, price_tmt, master_quantity, is_fake_quantity,
		       is_sell_allowed, is_expired, is_tmall, IFNULL(main_image_url,''),
		       IFNULL(platform_url,''), vendor_name, brand_name, volume_sales,
		       has_hierarchical_conf, fetched_at, updated_at
		FROM products WHERE id=?`, id).Scan(
		&p.ID, &p.OtapiID, &p.Provider, &p.CategoryID,
		&p.TitleOriginal, &p.TitleRu, &p.TitleEn, &p.TitleTk,
		&p.TranslateStatus, &p.PriceCNY, &p.PriceTMT, &p.MasterQuantity,
		&p.IsFakeQty, &p.IsSellAllowed, &p.IsExpired, &p.IsTmall,
		&p.MainImageURL, &p.PlatformURL, &p.VendorName, &p.BrandName,
		&p.VolueSales, &p.HasHierConf, &p.FetchedAt, &p.UpdatedAt,
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
		p.VolueSales, p.HasHierConf, string(rawJSON), p.FetchedAt, p.UpdatedAt,
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

	tmt := priceCNY*exchangeRate*(1+markupPct/100) + fixedAddon
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
	rows, err := s.Hub.Query(`SELECT id, job_type, IFNULL(category_id,''), status,
		started_at, finished_at, items_processed, items_skipped, errors_count, api_requests_made,
		IFNULL(log_text,''), triggered_by
		FROM sync_jobs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []SyncJob
	for rows.Next() {
		var j SyncJob
		if err := rows.Scan(&j.ID, &j.JobType, &j.CategoryID, &j.Status,
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
	PendingPush       int
	LastSync          *SyncJob
}

func (s *Store) GetDashboardStats() (*DashboardStats, error) {
	var stats DashboardStats
	s.Hub.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&stats.TotalProducts)
	s.Hub.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&stats.TotalCategories)
	s.Hub.QueryRow(`SELECT COUNT(*) FROM category_config WHERE enabled=1`).Scan(&stats.EnabledCategories)
	s.Hub.QueryRow(`SELECT COUNT(*) FROM push_queue WHERE status='pending'`).Scan(&stats.PendingPush)

	jobs, _ := s.GetRecentSyncJobs(1)
	if len(jobs) > 0 {
		stats.LastSync = &jobs[0]
	}
	return &stats, nil
}

type CategoryWithConfig struct {
	ID               string
	Provider         string
	Name             string
	ItemCount        int
	Enabled          bool
	SyncSchedule     string
	MaxProducts      int
	LastSyncedAt     *int64
	ProductsImported int
	CSCategoryID     *int
	Notes            string
	LocalCount       int
}

type CategoryMapping struct {
	OTCategoryID   string
	CSCategoryID   int
	CSCategoryName string
	Notes          string
}

func (s *Store) GetCategoryMappings() ([]CategoryMapping, error) {
	rows, err := s.Hub.Query(`SELECT otapi_category_id, cs_category_id, cs_category_name, notes FROM category_map ORDER BY cs_category_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CategoryMapping
	for rows.Next() {
		var m CategoryMapping
		rows.Scan(&m.OTCategoryID, &m.CSCategoryID, &m.CSCategoryName, &m.Notes)
		result = append(result, m)
	}
	return result, nil
}

func (s *Store) UpsertCategoryMapping(otCatID string, csCatID int, csCatName, notes string) error {
	_, err := s.Hub.Exec(`INSERT INTO category_map (otapi_category_id, cs_category_id, cs_category_name, notes)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE cs_category_id=VALUES(cs_category_id), cs_category_name=VALUES(cs_category_name), notes=VALUES(notes)`,
		otCatID, csCatID, csCatName, notes)
	return err
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
}

// CacheCSCartCategories - сохраняет список CS-Cart категорий в hub БД.
func (s *Store) CacheCSCartCategories(categories []CSCartCategory) error {
	now := time.Now().Unix()
	for _, c := range categories {
		s.Hub.Exec(`INSERT INTO cs_categories (category_id, parent_id, name, fetched_at)
			VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE name=VALUES(name), parent_id=VALUES(parent_id), fetched_at=VALUES(fetched_at)`,
			c.CategoryID, c.ParentID, c.Name, now)
	}
	return nil
}

// GetCSCartCategories - читает закэшированные CS-Cart категории.
func (s *Store) GetCSCartCategories() ([]CSCartCategory, error) {
	rows, err := s.Hub.Query(`SELECT category_id, parent_id, name FROM cs_categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CSCartCategory
	for rows.Next() {
		var c CSCartCategory
		rows.Scan(&c.CategoryID, &c.ParentID, &c.Name)
		result = append(result, c)
	}
	return result, nil
}

