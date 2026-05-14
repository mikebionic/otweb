package otapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"otapi-hub/otapi"
)

func newTestClient(srv *httptest.Server) *otapi.Client {
	return otapi.NewClient("test-key", srv.URL)
}

func okJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// --- ProviderFromCategoryID ---

func TestProviderFromCategoryID(t *testing.T) {
	cases := []struct {
		catID    string
		expected string
	}{
		{"otc-121", "jd"},
		{"otc-122", "poizon"},
		{"otc-123", "taobao"},
		{"", "taobao"},
		{"anything-else", "taobao"},
	}
	for _, c := range cases {
		got := otapi.ProviderFromCategoryID(c.catID)
		if got != c.expected {
			t.Errorf("ProviderFromCategoryID(%q) = %q, want %q", c.catID, got, c.expected)
		}
	}
}

// --- GetCatalog ---

func TestGetCatalog_Success(t *testing.T) {
	response := map[string]interface{}{
		"ErrorCode": "Ok",
		"CategoryInfoList": map[string]interface{}{
			"Content": []map[string]interface{}{
				{"Id": "otc-1", "ProviderType": "Taobao", "Name": "Одежда", "IsParent": true},
				{"Id": "otc-121", "ProviderType": "Jd", "Name": "JD", "IsParent": false},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "GetRootCategoryInfoList") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("instanceKey") != "test-key" {
			t.Errorf("missing or wrong instanceKey param")
		}
		w.Write(okJSON(response))
	}))
	defer srv.Close()

	cats, err := newTestClient(srv).GetCatalog()
	if err != nil {
		t.Fatalf("GetCatalog returned error: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}
	if cats[0].ID != "otc-1" {
		t.Errorf("expected first cat ID otc-1, got %s", cats[0].ID)
	}
	if cats[1].ID != "otc-121" {
		t.Errorf("expected second cat ID otc-121, got %s", cats[1].ID)
	}
}

func TestGetCatalog_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(okJSON(map[string]interface{}{
			"ErrorCode":        "AuthFailed",
			"CategoryInfoList": map[string]interface{}{"Content": []interface{}{}},
		}))
	}))
	defer srv.Close()

	_, err := newTestClient(srv).GetCatalog()
	if err == nil {
		t.Fatal("expected error on AuthFailed, got nil")
	}
}

func TestGetCatalog_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	_, err := newTestClient(srv).GetCatalog()
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
}

// --- SearchProducts ---

func TestSearchProducts_Success(t *testing.T) {
	response := map[string]interface{}{
		"ErrorCode": "Ok",
		"Result": map[string]interface{}{
			"Items": map[string]interface{}{
				"Items": map[string]interface{}{
					"Content": []map[string]interface{}{
						{
							"Id":           "item-1",
							"ProviderType": "Taobao",
							"Title":        "Тестовый товар",
							"Price":        map[string]interface{}{"OriginalPrice": 45.5},
						},
					},
					"TotalCount": 1,
				},
				"MaximumPageCount": 1,
				"Provider":         "Taobao",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "BatchSearchItemsFrame") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		if !strings.Contains(q.Get("xmlParameters"), "otc-1") {
			t.Errorf("xmlParameters missing category ID, got: %s", q.Get("xmlParameters"))
		}
		if q.Get("framePosition") != "0" {
			t.Errorf("expected framePosition=0, got %s", q.Get("framePosition"))
		}
		w.Write(okJSON(response))
	}))
	defer srv.Close()

	resp, err := newTestClient(srv).SearchProducts("taobao", "otc-1", 1, 50)
	if err != nil {
		t.Fatalf("SearchProducts error: %v", err)
	}
	items := resp.Result.Items.Items.Content
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != "item-1" {
		t.Errorf("expected item ID item-1, got %s", items[0].ID)
	}
	if items[0].Price.OriginalPrice != 45.5 {
		t.Errorf("expected price 45.5, got %f", items[0].Price.OriginalPrice)
	}
}

func TestSearchProducts_PageOffset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// page=3, limit=50 → framePosition=100
		if q.Get("framePosition") != "100" {
			t.Errorf("expected framePosition=100, got %s", q.Get("framePosition"))
		}
		w.Write(okJSON(map[string]interface{}{
			"ErrorCode": "Ok",
			"Result": map[string]interface{}{
				"Items": map[string]interface{}{
					"Items":            map[string]interface{}{"Content": []interface{}{}, "TotalCount": 0},
					"MaximumPageCount": 0,
				},
			},
		}))
	}))
	defer srv.Close()

	newTestClient(srv).SearchProducts("taobao", "otc-1", 3, 50)
}

func TestSearchProducts_EmptyResult(t *testing.T) {
	response := map[string]interface{}{
		"ErrorCode": "Ok",
		"Result": map[string]interface{}{
			"Items": map[string]interface{}{
				"Items":            map[string]interface{}{"Content": []interface{}{}, "TotalCount": 0},
				"MaximumPageCount": 0,
				"Provider":         "Taobao",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(okJSON(response))
	}))
	defer srv.Close()

	resp, err := newTestClient(srv).SearchProducts("taobao", "otc-empty", 1, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Result.Items.Items.Content) != 0 {
		t.Errorf("expected empty content")
	}
}

// --- GetProduct ---

func TestGetProduct_Success(t *testing.T) {
	response := map[string]interface{}{
		"ErrorCode": "Ok",
		"OtapiItemFullInfo": map[string]interface{}{
			"Id":            "item-42",
			"ProviderType":  "Taobao",
			"Title":         "Куртка зимняя",
			"OriginalTitle": "冬季夹克",
			"Price":         map[string]interface{}{"OriginalPrice": 120.0},
			"MasterQuantity": 50,
			"IsSellAllowed": true,
			"ConfiguredItems": []map[string]interface{}{
				{
					"Id":       "sku-1",
					"Quantity": 10,
					"Price":    map[string]interface{}{"OriginalPrice": 120.0},
				},
			},
			"Attributes": []map[string]interface{}{
				{"Pid": "1", "Vid": "2", "PropertyName": "Цвет", "Value": "Чёрный", "IsConfigurator": true},
			},
			"Features": []string{"Tmall"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "GetItemFullInfo") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("itemId") != "item-42" {
			t.Errorf("expected itemId=item-42, got %s", r.URL.Query().Get("itemId"))
		}
		w.Write(okJSON(response))
	}))
	defer srv.Close()

	product, err := newTestClient(srv).GetProduct("taobao", "item-42")
	if err != nil {
		t.Fatalf("GetProduct error: %v", err)
	}
	if product.ID != "item-42" {
		t.Errorf("expected ID item-42, got %s", product.ID)
	}
	if product.Price.OriginalPrice != 120.0 {
		t.Errorf("expected price 120.0, got %f", product.Price.OriginalPrice)
	}
	if len(product.ConfiguredItems) != 1 {
		t.Errorf("expected 1 SKU, got %d", len(product.ConfiguredItems))
	}
	if len(product.Attributes) != 1 {
		t.Errorf("expected 1 attribute, got %d", len(product.Attributes))
	}
	if len(product.Features) != 1 || product.Features[0] != "Tmall" {
		t.Errorf("expected Feature Tmall")
	}
}

func TestGetProduct_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(okJSON(map[string]interface{}{
			"ErrorCode":         "ItemNotFound",
			"OtapiItemFullInfo": map[string]interface{}{},
		}))
	}))
	defer srv.Close()

	_, err := newTestClient(srv).GetProduct("taobao", "nonexistent")
	if err == nil {
		t.Fatal("expected error for ItemNotFound, got nil")
	}
}
