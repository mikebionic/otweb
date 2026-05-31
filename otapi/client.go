// Package otapi - клиент к OT Commerce Legacy API (otapi.net/service-json).
// Предоставляет методы для получения категорий, поиска товаров и карточек товаров.
// Аутентификация через instanceKey в query-параметре.
package otapi

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client - HTTP клиент к OT Commerce Legacy API.
type Client struct {
	instanceKey string
	baseURL     string // https://otapi.net/service-json
	http        *http.Client
}

func NewClient(instanceKey, baseURL string) *Client {
	return &Client{
		instanceKey: instanceKey,
		baseURL:     baseURL,
		http:        &http.Client{Timeout: 60 * time.Second},
	}
}

// get - базовый GET запрос к Legacy API.
// Добавляет instanceKey и language=ru. Логирует URL.
// Возвращает тело ответа или ошибку.
func (c *Client) get(method string, params url.Values) ([]byte, error) {
	params.Set("instanceKey", c.instanceKey)
	params.Set("language", "ru")
	u := fmt.Sprintf("%s/%s?%s", c.baseURL, method, params.Encode())
	log.Printf("[otapi] GET %s", u)

	resp, err := c.http.Get(u)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// GetCatalog - получает список корневых категорий (122 шт).
// API: GET GetRootCategoryInfoList (бесплатный метод).
// Ответ: CategoryInfoList.Content[] -> []Category
func (c *Client) GetCatalog() ([]Category, error) {
	body, err := c.get("GetRootCategoryInfoList", url.Values{})
	if err != nil {
		return nil, err
	}
	var resp struct {
		ErrorCode        string `json:"ErrorCode"`
		ErrorDescription string `json:"ErrorDescription"`
		CategoryInfoList struct {
			Content []Category `json:"Content"`
		} `json:"CategoryInfoList"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal catalog: %w", err)
	}
	if resp.ErrorCode != "Ok" {
		return nil, fmt.Errorf("api error: %s", resp.ErrorCode)
	}
	return resp.CategoryInfoList.Content, nil
}

// GetSubcategories - получает подкатегории для parentCategoryId.
func (c *Client) GetSubcategories(parentCategoryID string) ([]Category, error) {
	body, err := c.get("GetCategorySubcategoryInfoList", url.Values{
		"parentCategoryId": {parentCategoryID},
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		ErrorCode        string `json:"ErrorCode"`
		CategoryInfoList struct {
			Content []Category `json:"Content"`
		} `json:"CategoryInfoList"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal subcategories: %w", err)
	}
	if resp.ErrorCode != "Ok" {
		return nil, fmt.Errorf("api error: %s", resp.ErrorCode)
	}
	return resp.CategoryInfoList.Content, nil
}

// SearchProducts - поиск товаров в категории.
// SearchFilters - фильтры для поиска товаров через API.
// SearchFilters — все доступные фильтры OTAPI BatchSearchItemsFrame.
// Документация: https://docs.otapi.net/en/Documentations/Method/BatchSearchItemsFrame
type SearchFilters struct {
	// --- Основные ---
	MinVolume      int    // Мин. кол-во продаж (штук). Рекомендуется 50+.
	MinPrice       int    // Мин. цена CNY. Рекомендуется 30+ для отсева хлама.
	MaxPrice       int    // Макс. цена CNY. Рекомендуется 800-1000.
	ItemTitle      string // Ключевые слова (CN или RU). Опционально при поиске по категории.
	OrderBy        string // Сортировка: "" (relevance, рекомендуется), "Price:Asc", "Price:Desc", "Volume:Desc", "UpdatedTime:Desc"
	StuffStatus    string // Состояние: "New" (всегда), "Used" (никогда)

	// --- Продавец ---
	MinVendorRating int // Мин. рейтинг продавца (система 1688: ~1-20+). Рекомендуется 8+.
	MaxVendorRating int // Макс. рейтинг продавца. Обычно не нужен.
	VendorName      string // Фильтр по имени продавца.

	// --- Лот (только 1688) ---
	FirstLotMin int // Мин. первый лот (мин. кол-во в заказе). Обычно 1.
	FirstLotMax int // Макс. первый лот. Рекомендуется 10 — отсекает чисто оптовые позиции.

	// --- Метод поиска ---
	SearchMethod string // "" = Default (нативный relevance), "Official" = только Tmall магазины.

	// --- Features (флаги качества) ---
	FeatureComplete bool // IsComplete: только полностью заполненные карточки (рекомендуется включить).
	FeatureDiscount bool // Discount: только товары со скидкой.
	FeatureTmall    bool // Tmall: только Tmall магазины (аналог SearchMethod=Official, но через Features).

	// --- Прочее (расширенные) ---
	BrandName      string // Фильтр по бренду.
	PropertySearch string // Фильтр по свойствам (pid:value).
}

// SearchProducts - поиск товаров с фильтрами.
// API: GET BatchSearchItemsFrame (1 платный вызов за запрос).
// page - 1-based, limit - товаров на страницу (рекомендуется 20).
func (c *Client) SearchProducts(provider, categoryID string, page, limit int, filters ...SearchFilters) (*SearchResponse, error) {
	framePosition := (page - 1) * limit

	// Формируем XML с фильтрами
	xml := "<SearchItemsParameters>"
	if categoryID != "" && categoryID != "search" {
		xml += "<CategoryId>" + categoryID + "</CategoryId>"
	}
	if len(filters) > 0 {
		f := filters[0]
		if f.MinVolume > 0 {
			xml += fmt.Sprintf("<MinVolume>%d</MinVolume>", f.MinVolume)
		}
		if f.MinPrice > 0 {
			xml += fmt.Sprintf("<MinPrice>%d</MinPrice>", f.MinPrice)
		}
		if f.MaxPrice > 0 {
			xml += fmt.Sprintf("<MaxPrice>%d</MaxPrice>", f.MaxPrice)
		}
		if f.ItemTitle != "" {
			xml += "<ItemTitle>" + html.EscapeString(f.ItemTitle) + "</ItemTitle>"
		}
		if f.VendorName != "" {
			xml += "<VendorName>" + html.EscapeString(f.VendorName) + "</VendorName>"
		}
		if f.BrandName != "" {
			xml += "<BrandName>" + html.EscapeString(f.BrandName) + "</BrandName>"
		}
		if f.PropertySearch != "" {
			xml += "<PropertySearch><![CDATA[" + f.PropertySearch + "]]></PropertySearch>"
		}
		if f.OrderBy != "" {
			xml += "<OrderBy>" + f.OrderBy + "</OrderBy>"
		}
		if f.StuffStatus != "" {
			xml += "<StuffStatus>" + f.StuffStatus + "</StuffStatus>"
		}
		// Рейтинг продавца
		if f.MinVendorRating > 0 || f.MaxVendorRating > 0 {
			xml += "<VendorRatingRange>"
			if f.MinVendorRating > 0 {
				xml += fmt.Sprintf("<Min>%d</Min>", f.MinVendorRating)
			}
			if f.MaxVendorRating > 0 {
				xml += fmt.Sprintf("<Max>%d</Max>", f.MaxVendorRating)
			}
			xml += "</VendorRatingRange>"
		}
		// Первый лот (мин. заказ, только 1688)
		if f.FirstLotMax > 0 {
			xml += "<FirstLotRange>"
			if f.FirstLotMin > 0 {
				xml += fmt.Sprintf("<Min>%d</Min>", f.FirstLotMin)
			}
			xml += fmt.Sprintf("<Max>%d</Max>", f.FirstLotMax)
			xml += "</FirstLotRange>"
		}
		// Features
		var features string
		if f.FeatureTmall {
			features += `<Feature Name="Tmall">true</Feature>`
		}
		if f.FeatureComplete {
			features += `<Feature Name="IsComplete">true</Feature>`
		}
		if f.FeatureDiscount {
			features += `<Feature Name="Discount">true</Feature>`
		}
		if features != "" {
			xml += "<Features>" + features + "</Features>"
		}
	}
	xml += "</SearchItemsParameters>"
	params := url.Values{
		"xmlParameters": {xml},
		"framePosition": {strconv.Itoa(framePosition)},
		"frameSize":     {strconv.Itoa(limit)},
		"blockList":     {"SearchItems"},
	}
	if len(filters) > 0 && filters[0].SearchMethod != "" {
		params.Set("searchMethod", filters[0].SearchMethod)
	}
	body, err := c.get("BatchSearchItemsFrame", params)
	if err != nil {
		return nil, err
	}
	var resp SearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal search: %w", err)
	}
	if resp.ErrorCode != "Ok" {
		return nil, fmt.Errorf("api error: %s", resp.ErrorCode)
	}
	return &resp, nil
}

// GetProduct - полная карточка одного товара.
// API: GET GetItemFullInfo (1 платный вызов).
// Ответ: OtapiItemFullInfo -> ProductItem (SKU, фото, атрибуты, описание, вес).
// Дорогой запрос - вызывать только для новых товаров или раз в 7 дней.
func (c *Client) GetProduct(provider, itemID string) (*ProductItem, error) {
	body, err := c.get("GetItemFullInfo", url.Values{"itemId": {itemID}})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		ErrorCode         string      `json:"ErrorCode"`
		ErrorDescription  string      `json:"ErrorDescription"`
		OtapiItemFullInfo ProductItem `json:"OtapiItemFullInfo"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("unmarshal product: %w", err)
	}
	if wrapper.ErrorCode != "Ok" {
		return nil, fmt.Errorf("api error: %s", wrapper.ErrorCode)
	}
	return &wrapper.OtapiItemFullInfo, nil
}

// ProviderFromCategoryID - определяет провайдер по ID категории.
func ProviderFromCategoryID(catID string) string {
	switch {
	case catID == "otc-121":
		return "jd"
	case catID == "otc-122":
		return "poizon"
	case strings.HasPrefix(catID, "abb-"):
		return "alibaba1688"
	default:
		return "taobao"
	}
}
