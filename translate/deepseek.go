// Package translate - нормализация характеристик товаров через DeepSeek API.
// Принимает сырые данные из OT Commerce (машинный перевод с китайского)
// и возвращает нормализованный JSON с допустимыми значениями из схемы Азата.
// Пример: "Retro Huai Old Blue" -> "Синий", "Прямая трубка" -> "Прямая".
package translate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type DeepSeekClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewDeepSeekClient(apiKey, baseURL string) *DeepSeekClient {
	return &DeepSeekClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

type NormalizeInput struct {
	TitleRu       string
	TitleOriginal string
	Attributes    map[string]string // property_name -> value
	Colors        []string
}

type NormalizeOutput struct {
	Title       string `json:"Название товара"`
	Description string `json:"Описание"`
	Color       string `json:"Цвет"`
	Category1   string `json:"Категория - уровень 1"`
	Category2   string `json:"Категория - уровень 2"`
	Fabric      string `json:"Ткань"`
	Material    string `json:"Материал"`
	Lining      string `json:"Подкладка"`
	Occasion    string `json:"Повод"`
	Length      string `json:"Длина"`
	Thickness   string `json:"Толщина"`
	Pattern     string `json:"Узор"`
	LegType     string `json:"Штанина"`
	Model       string `json:"Модель"`
	WaistHeight string `json:"Высота талии"`
	SleeveLen   string `json:"Длина рукава"`
	CollarType  string `json:"Тип воротника"`
	Country     string `json:"Страна производства"`
	Hood        string `json:"Капюшон"`
}

// Normalize - отправляет промпт с данными товара в DeepSeek и парсит JSON ответ.
// Промпт содержит схему допустимых значений (25 цветов, 45 тканей, 16 материалов...).
// DeepSeek возвращает JSON с нормализованными характеристиками.
// Время: ~3 сек, стоимость: ~$0.001/вызов.
func (c *DeepSeekClient) Normalize(input NormalizeInput) (*NormalizeOutput, error) {
	prompt := buildPrompt(input)

	reqBody := map[string]interface{}{
		"model": "deepseek-chat",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.1,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/chat/completions", c.baseURL)
	log.Printf("[deepseek] POST %s (%d bytes)", url, len(data))

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices")
	}

	content := apiResp.Choices[0].Message.Content
	var output NormalizeOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, fmt.Errorf("unmarshal output: %w (content: %s)", err, content[:min(200, len(content))])
	}

	return &output, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
