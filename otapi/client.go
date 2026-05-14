// Package otapi - клиент к OT Commerce Legacy API (otapi.net/service-json).
// Предоставляет методы для получения категорий, поиска товаров и карточек товаров.
// Аутентификация через instanceKey в query-параметре.
package otapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
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

// SearchProducts - поиск товаров в категории.
// API: GET BatchSearchItemsFrame (1 платный вызов за запрос).
// Параметры: xmlParameters с CategoryId, framePosition (offset), frameSize (лимит).
// page - 1-based номер страницы. limit - товаров на страницу (рекомендуется 20, макс 50).
// Ответ: Result.Items.Items.Content[] -> []SearchItem + MaximumPageCount.
func (c *Client) SearchProducts(provider, categoryID string, page, limit int) (*SearchResponse, error) {
	framePosition := (page - 1) * limit
	xmlParams := fmt.Sprintf(
		"<SearchItemsParameters><CategoryId>%s</CategoryId></SearchItemsParameters>",
		categoryID,
	)
	params := url.Values{
		"xmlParameters": {xmlParams},
		"framePosition": {strconv.Itoa(framePosition)},
		"frameSize":     {strconv.Itoa(limit)},
		"blockList":     {"SearchItems"},
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
// otc-121 = JD, otc-122 = Poizon, всё остальное = Taobao.
func ProviderFromCategoryID(catID string) string {
	switch catID {
	case "otc-121":
		return "jd"
	case "otc-122":
		return "poizon"
	default:
		return "taobao"
	}
}
