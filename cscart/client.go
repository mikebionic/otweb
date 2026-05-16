package cscart

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Client struct {
	baseURL  string
	email    string
	apiKey   string
	http     *http.Client
}

func NewClient(baseURL, email, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		email:   email,
		apiKey:  apiKey,
		http: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Do - выполняет HTTP запрос к CS-Cart API. Экспортирован для использования в хэндлерах.
func (c *Client) Do(method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	url := fmt.Sprintf("%s/api/%s", c.baseURL, path)
	log.Printf("[cscart] %s %s", method, url)

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(c.email, c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// CreateProduct creates a product and returns its CS-Cart product_id.
func (c *Client) CreateProduct(p ProductInput) (int, error) {
	body, status, err := c.Do("POST", "products", p)
	if err != nil {
		return 0, err
	}
	if status < 200 || status >= 300 {
		return 0, fmt.Errorf("status %d: %s", status, string(body))
	}
	var result struct {
		ProductID int `json:"product_id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("unmarshal: %w (body: %s)", err, string(body))
	}
	return result.ProductID, nil
}

// UpdateProduct updates price, amount, status of an existing product.
func (c *Client) UpdateProduct(productID int, p ProductUpdate) error {
	body, status, err := c.Do("PUT", fmt.Sprintf("products/%d", productID), p)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("status %d: %s", status, string(body))
	}
	return nil
}

// DeleteProduct removes a product by ID.
func (c *Client) DeleteProduct(productID int) error {
	body, status, err := c.Do("DELETE", fmt.Sprintf("products/%d", productID), nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("status %d: %s", status, string(body))
	}
	return nil
}

// CreateOption creates a product option (e.g. Size, Color) and returns option_id.
func (c *Client) CreateOption(productID int, name string, variants []string) (int, error) {
	variantList := make([]map[string]string, len(variants))
	for i, v := range variants {
		variantList[i] = map[string]string{"variant_name": v}
	}

	payload := map[string]interface{}{
		"product_id":  productID,
		"option_name": name,
		"option_type": "S",
		"required":    "Y",
		"variants":    variantList,
	}

	body, status, err := c.Do("POST", "options", payload)
	if err != nil {
		return 0, err
	}
	if status < 200 || status >= 300 {
		return 0, fmt.Errorf("status %d: %s", status, string(body))
	}
	var result struct {
		OptionID int `json:"option_id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("unmarshal: %w (body: %s)", err, string(body))
	}
	return result.OptionID, nil
}

// UpdateProductFeatures sets product features by feature_id -> variant_id.
// For S-type features (Select), variant_id must be a valid variant from the feature.
func (c *Client) UpdateProductFeatures(productID int, features map[int]string) error {
	featurePayload := make(map[string]string)
	for fid, variantID := range features {
		featurePayload[fmt.Sprintf("%d", fid)] = variantID
	}

	payload := map[string]interface{}{
		"product_features": featurePayload,
	}

	body, status, err := c.Do("PUT", fmt.Sprintf("products/%d", productID), payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("status %d: %s", status, string(body))
	}
	return nil
}

// LoadFeatureVariants loads variant lookup for a feature (variant_name -> variant_id).
func (c *Client) LoadFeatureVariants(featureID int) (map[string]string, error) {
	body, status, err := c.Do("GET", fmt.Sprintf("features/%d", featureID), nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("status %d", status)
	}

	var resp struct {
		Variants map[string]struct {
			Variant string `json:"variant"`
		} `json:"variants"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for vid, v := range resp.Variants {
		result[strings.ToLower(v.Variant)] = vid
	}
	return result, nil
}

// featureCache caches variant lookups to avoid repeated API calls.
var (
	featureCache   = make(map[int]map[string]string)
	featureCacheMu sync.RWMutex
)

// ResolveFeatureVariant finds the variant_id for a feature value, using cache.
func (c *Client) ResolveFeatureVariant(featureID int, value string) (string, bool) {
	featureCacheMu.RLock()
	lookup, exists := featureCache[featureID]
	featureCacheMu.RUnlock()

	if !exists {
		variants, err := c.LoadFeatureVariants(featureID)
		if err != nil {
			return "", false
		}
		featureCacheMu.Lock()
		featureCache[featureID] = variants
		featureCacheMu.Unlock()
		lookup = variants
	}

	vid, ok := lookup[strings.ToLower(value)]
	return vid, ok
}

// GetProduct retrieves a product by ID.
func (c *Client) GetProduct(productID int) (*ProductOutput, error) {
	body, status, err := c.Do("GET", fmt.Sprintf("products/%d", productID), nil)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, fmt.Errorf("product %d not found", productID)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("status %d: %s", status, string(body))
	}
	var product ProductOutput
	if err := json.Unmarshal(body, &product); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &product, nil
}
