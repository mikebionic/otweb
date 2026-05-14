package cscart_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"otapi-hub/cscart"
)

func newTestClient(srv *httptest.Server) *cscart.Client {
	return cscart.NewClient(srv.URL, "admin@test.com", "test-api-key")
}

func TestCreateProduct_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		var input map[string]interface{}
		json.Unmarshal(body, &input)

		if input["product"] != "Тестовый товар" {
			t.Errorf("expected product name, got %v", input["product"])
		}
		catIDs, ok := input["category_ids"].([]interface{})
		if !ok || len(catIDs) != 1 || catIDs[0].(float64) != 224 {
			t.Errorf("expected category_ids=[224], got %v", input["category_ids"])
		}
		if input["status"] != "D" {
			t.Errorf("expected status=D, got %v", input["status"])
		}
		if input["main_pair"] == nil {
			t.Error("expected main_pair")
		}
		if input["full_description"] != "<p>Описание</p>" {
			t.Errorf("expected description, got %v", input["full_description"])
		}
		if input["weight"].(float64) != 1.5 {
			t.Errorf("expected weight=1.5, got %v", input["weight"])
		}

		w.WriteHeader(201)
		w.Write([]byte(`{"product_id": 12345}`))
	}))
	defer srv.Close()

	p := cscart.NewProductInput(
		"Тестовый товар", 224, 376, 91.60, 100, "otapi-123",
		"<p>Описание</p>", 1.5,
		"https://img.example.com/main.jpg",
		[]string{"https://img.example.com/2.jpg", "https://img.example.com/3.jpg"},
	)

	id, err := newTestClient(srv).CreateProduct(p)
	if err != nil {
		t.Fatalf("CreateProduct error: %v", err)
	}
	if id != 12345 {
		t.Errorf("expected product_id=12345, got %d", id)
	}
}

func TestCreateProduct_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error": "internal"}`))
	}))
	defer srv.Close()

	p := cscart.NewProductInput("Test", 1, 0, 10.0, 1, "x", "", 0, "", nil)
	_, err := newTestClient(srv).CreateProduct(p)
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestUpdateProduct_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/products/12345") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"product_id": 12345}`))
	}))
	defer srv.Close()

	err := newTestClient(srv).UpdateProduct(12345, cscart.ProductUpdate{
		Price:  "55.00",
		Amount: 50,
	})
	if err != nil {
		t.Fatalf("UpdateProduct error: %v", err)
	}
}

func TestDeleteProduct_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	err := newTestClient(srv).DeleteProduct(999)
	if err != nil {
		t.Fatalf("DeleteProduct error: %v", err)
	}
}

func TestGetProduct_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"product_id":42,"product":"Куртка","price":"120.50","amount":10,"status":"A","product_code":"otapi-42"}`))
	}))
	defer srv.Close()

	product, err := newTestClient(srv).GetProduct(42)
	if err != nil {
		t.Fatalf("GetProduct error: %v", err)
	}
	if product.ProductID != 42 {
		t.Errorf("expected id=42, got %d", product.ProductID)
	}
	if product.Amount != 10 {
		t.Errorf("expected amount=10, got %d", product.Amount)
	}
}

func TestGetProduct_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv).GetProduct(99999)
	if err == nil {
		t.Fatal("expected error on 404")
	}
}

func TestNewProductInput_Images(t *testing.T) {
	p := cscart.NewProductInput("Товар", 10, 376, 50.0, 5, "abc", "", 0,
		"https://main.jpg", []string{"https://add1.jpg", "https://add2.jpg"})

	if p.MainPair == nil {
		t.Fatal("expected main_pair")
	}
	if len(p.ImagePairs) != 2 {
		t.Fatalf("expected 2 image_pairs, got %d", len(p.ImagePairs))
	}
	if p.CategoryIDs[0] != 10 {
		t.Errorf("expected category 10, got %d", p.CategoryIDs[0])
	}
	if p.Status != "D" {
		t.Errorf("expected status=D, got %s", p.Status)
	}
}

func TestNormalizeSize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"S", "S"},
		{"XL", "XL"},
		{"2XL", "2XL"},
		{"S подходит для 85-105 фунтов", "S"},
		{"M подходит для 106-125 кот", "M"},
		{"Xs (менее 86 кот)", "XS"},
		{"4xl код", "4XL"},
		{"3XL -код", "3XL"},
		{"170/84 44", "44"},
		{"170/84 (44)", "44"},
		{"155/64 34", "34"},
		{"27 двух футов [от 95 до 104 фунтов]", "27"},
		{"30 136-150 Catties", "30"},
	}
	for _, c := range cases {
		got := cscart.NormalizeSize(c.in)
		if got != c.want {
			t.Errorf("NormalizeSize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNewProductInput_NoImages(t *testing.T) {
	p := cscart.NewProductInput("Товар", 10, 0, 50.0, 5, "abc", "", 0, "", nil)
	if p.MainPair != nil {
		t.Error("expected nil main_pair when no image")
	}
	if p.ImagePairs != nil {
		t.Error("expected nil image_pairs when no additional")
	}
}
