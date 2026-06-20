package cscart

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type ProductInput struct {
	Product          string      `json:"product"`
	CategoryIDs      []int       `json:"category_ids"`
	Price            string      `json:"price"`
	Amount           int         `json:"amount"`
	MinQty           int         `json:"min_qty,omitempty"`
	ProductCode      string      `json:"product_code,omitempty"`
	CompanyID        int         `json:"company_id,omitempty"`
	Status           string      `json:"status"`
	FullDescription  string      `json:"full_description,omitempty"`
	Weight           float64     `json:"weight,omitempty"`
	MainPair         *ImagePair  `json:"main_pair,omitempty"`
	ImagePairs       []ImagePair `json:"image_pairs,omitempty"`
	AvailSince       int64       `json:"avail_since,omitempty"`
	OutOfStockActions string     `json:"out_of_stock_actions,omitempty"`
}

type ProductUpdate struct {
	Price           string      `json:"price,omitempty"`
	Amount          int         `json:"amount,omitempty"`
	MinQty          int         `json:"min_qty,omitempty"`
	Status          string      `json:"status,omitempty"`
	Product         string      `json:"product,omitempty"`
	CategoryIDs     []int       `json:"category_ids,omitempty"`
	FullDescription string      `json:"full_description,omitempty"`
	Weight          float64     `json:"weight,omitempty"`
	MainPair        *ImagePair  `json:"main_pair,omitempty"`
	ImagePairs      []ImagePair `json:"image_pairs,omitempty"`
}

type ImagePair struct {
	Detailed ImageDetailed `json:"detailed"`
}

type ImageDetailed struct {
	ImagePath string `json:"image_path"`
}

type ProductOutput struct {
	ProductID   int    `json:"product_id"`
	Product     string `json:"product"`
	Price       string `json:"price"`
	Amount      int    `json:"amount"`
	Status      string `json:"status"`
	ProductCode string `json:"product_code"`
	MainPair    *struct {
		Detailed *struct {
			ImagePath string `json:"image_path"`
			HTTPPath  string `json:"http_image_path"`
		} `json:"detailed"`
	} `json:"main_pair"`
}

func NewProductInput(title string, categoryID, companyID int, priceTMT float64, amount int, otapiID string,
	description string, weight float64, mainImageURL string, additionalImageURLs []string) ProductInput {

	p := ProductInput{
		Product:            title,
		CategoryIDs:        []int{categoryID},
		CompanyID:          companyID,
		Price:              fmt.Sprintf("%.2f", priceTMT),
		Amount:             amount,
		ProductCode:        otapiID,
		Status:             "D",
		FullDescription:    description,
		Weight:             weight,
		AvailSince:         time.Now().Add(7 * 24 * time.Hour).Unix(),
		OutOfStockActions:  "B",
	}

	if mainImageURL != "" {
		p.MainPair = &ImagePair{
			Detailed: ImageDetailed{ImagePath: mainImageURL},
		}
	}

	for _, url := range additionalImageURLs {
		p.ImagePairs = append(p.ImagePairs, ImagePair{
			Detailed: ImageDetailed{ImagePath: url},
		})
	}

	return p
}

var (
	reLetterSize = regexp.MustCompile(`(?i)^(XXXXL|XXXL|XXL|XL|4XL|3XL|2XL|XS|S|M|L)\b`)
	reNumSlash   = regexp.MustCompile(`^(\d{2,3}/\d{2,3})\s*\(?(\d{2})\)?`)
	reNumStart   = regexp.MustCompile(`^(\d{2})\s`)
)

// NormalizeSize extracts a clean size label from OTAPI's raw size string.
// "S подходит для 85-105 фунтов" -> "S"
// "170/84 (44)" -> "44"
// "27 двух футов [от 95 до 104 фунтов]" -> "27"
func NormalizeSize(raw string) string {
	raw = strings.TrimSpace(raw)

	if m := reLetterSize.FindString(raw); m != "" {
		return strings.ToUpper(m)
	}
	if m := reNumSlash.FindStringSubmatch(raw); len(m) >= 3 {
		return m[2]
	}
	if m := reNumStart.FindStringSubmatch(raw); len(m) >= 2 {
		return m[1]
	}
	if len(raw) > 15 {
		return raw[:15]
	}
	return raw
}

// NormalizeSizes deduplicates and normalizes a list of raw size strings.
func NormalizeSizes(rawSizes []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, raw := range rawSizes {
		n := NormalizeSize(raw)
		if !seen[n] {
			seen[n] = true
			result = append(result, n)
		}
	}
	return result
}
