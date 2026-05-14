package sync_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"otapi-hub/otapi"
	"otapi-hub/sync"

	_ "github.com/go-sql-driver/mysql"
)

func searchResponse(items []map[string]interface{}, maxPages int) map[string]interface{} {
	return map[string]interface{}{
		"ErrorCode": "Ok",
		"Result": map[string]interface{}{
			"Items": map[string]interface{}{
				"Items": map[string]interface{}{
					"Content":    items,
					"TotalCount": len(items),
				},
				"MaximumPageCount": maxPages,
				"Provider":         "taobao",
			},
		},
	}
}

func TestProviderFromCategoryID(t *testing.T) {
	tests := []struct {
		id       string
		expected string
	}{
		{"otc-121", "jd"},
		{"otc-122", "poizon"},
		{"otc-10", "taobao"},
		{"otc-999", "taobao"},
		{"", "taobao"},
	}
	for _, tt := range tests {
		got := otapi.ProviderFromCategoryID(tt.id)
		if got != tt.expected {
			t.Errorf("ProviderFromCategoryID(%q) = %q, want %q", tt.id, got, tt.expected)
		}
	}
}

func TestSyncCategories_ClientCall(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if !strings.Contains(r.URL.Path, "GetRootCategoryInfoList") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"ErrorCode":        "Ok",
			"CategoryInfoList": map[string]interface{}{"Content": []interface{}{}},
		}
		b, _ := json.Marshal(resp)
		w.Write(b)
	}))
	defer srv.Close()

	imp := sync.NewImporter(nil, otapi.NewClient("key", srv.URL))
	err := imp.SyncCategories()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected GetCatalog to be called")
	}
}

func TestSyncCategories_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]interface{}{
			"ErrorCode":        "AuthFailed",
			"CategoryInfoList": map[string]interface{}{"Content": []interface{}{}},
		})
		w.Write(b)
	}))
	defer srv.Close()

	imp := sync.NewImporter(nil, otapi.NewClient("bad-key", srv.URL))
	if err := imp.SyncCategories(); err == nil {
		t.Error("expected error on AuthFailed")
	}
}

func TestSyncPricesOnly_EmptyCategory(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		b, _ := json.Marshal(searchResponse([]map[string]interface{}{}, 0))
		w.Write(b)
	}))
	defer srv.Close()

	imp := sync.NewImporter(nil, otapi.NewClient("key", srv.URL))
	updated, apiReqs, err := imp.SyncPricesOnly("otc-10")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated != 0 {
		t.Errorf("expected 0 updated, got %d", updated)
	}
	if apiReqs != 1 {
		t.Errorf("expected 1 API request, got %d", apiReqs)
	}
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call, got %d", callCount)
	}
}

