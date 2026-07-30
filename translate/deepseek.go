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
	"strconv"
	"time"
)

type DeepSeekClient struct {
	apiKey       string
	baseURL      string
	customPrompt string
	http         *http.Client
}

func NewDeepSeekClient(apiKey, baseURL string) *DeepSeekClient {
	return &DeepSeekClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *DeepSeekClient) SetCustomPrompt(prompt string) {
	c.customPrompt = prompt
}

// usageInfo — токены из ответа DeepSeek для учёта стоимости. cache_hit оплачивается
// по ~10% ставки input, поэтому важно видеть долю попаданий в кэш префикса-схемы.
type usageInfo struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
}

func (u usageInfo) log(tag string) {
	log.Printf("[deepseek][cost] %s in=%d (cache_hit=%d miss=%d) out=%d",
		tag, u.PromptTokens, u.PromptCacheHitTokens, u.PromptCacheMissTokens, u.CompletionTokens)
}

type NormalizeInput struct {
	TitleRu       string
	TitleOriginal string
	Attributes    map[string]string // property_name -> value
	Colors        []string
}

type NormalizeOutput struct {
	// Названия на 3 языках
	TitleRU string `json:"title_ru"`
	TitleEN string `json:"title_en"`
	TitleTK string `json:"title_tk"`
	// Описания на 3 языках
	DescriptionRU string `json:"description_ru"`
	DescriptionEN string `json:"description_en"`
	DescriptionTK string `json:"description_tk"`
	// SEO ключевые слова
	KeywordsRU string `json:"keywords_ru"`
	KeywordsEN string `json:"keywords_en"`
	// Характеристики (из схемы Азата)
	Color       string `json:"color"`
	Category1   string `json:"category_1"`
	Category2   string `json:"category_2"`
	Fabric      string `json:"fabric"`
	Material    string `json:"material"`
	Lining      string `json:"lining"`
	Occasion    string `json:"occasion"`
	Length      string `json:"length"`
	Thickness   string `json:"thickness"`
	Pattern     string `json:"pattern"`
	LegType     string `json:"leg_type"`
	Model       string `json:"model"`
	WaistHeight string `json:"waist_height"`
	SleeveLen   string `json:"sleeve_length"`
	CollarType  string `json:"collar_type"`
	Country              string `json:"country"`
	Hood                 string `json:"hood"`
	EstimatedWeightGrams int    `json:"estimated_weight_grams"`

	// backward compat
	Title       string `json:"Название товара"`
	Description string `json:"Описание"`
}

// Normalize - отправляет промпт с данными товара в DeepSeek и парсит JSON ответ.
// Промпт содержит схему допустимых значений (25 цветов, 45 тканей, 16 материалов...).
// DeepSeek возвращает JSON с нормализованными характеристиками.
// Время: ~3 сек, стоимость: ~$0.001/вызов.
func (c *DeepSeekClient) Normalize(input NormalizeInput) (*NormalizeOutput, error) {
	prompt := buildPromptWith(input, c.customPrompt)

	reqBody := map[string]interface{}{
		"model": "deepseek-v4-flash",
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
		Usage usageInfo `json:"usage"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices")
	}
	apiResp.Usage.log("Normalize")

	content := apiResp.Choices[0].Message.Content
	var output NormalizeOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, fmt.Errorf("unmarshal output: %w (content: %s)", err, content[:min(200, len(content))])
	}

	return &output, nil
}

// RawChat - отправляет промпт и возвращает сырой текст ответа (JSON string).
func (c *DeepSeekClient) RawChat(prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model": "deepseek-v4-flash",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.1,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	apiURL := fmt.Sprintf("%s/chat/completions", c.baseURL)
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("unmarshal: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("empty choices")
	}
	return apiResp.Choices[0].Message.Content, nil
}

// AttrPair - пара для перевода (название характеристики + значение)
type AttrPair struct {
	Pid   string
	Vid   string
	Name  string
	Value string
}

// AttrTranslated - результат перевода
type AttrTranslated struct {
	Pid     string
	Vid     string
	NameRu  string
	ValueRu string
}

// translateTerms отправляет список китайских терминов и получает русские переводы.
// Каждый элемент: {"id":"0","zh":"..."} → {"id":"0","ru":"..."}
func (c *DeepSeekClient) translateTerms(terms []string, context string) (map[int]string, error) {
	type item struct {
		ID int    `json:"id"`
		Zh string `json:"zh"`
	}
	items := make([]item, len(terms))
	for i, t := range terms {
		items[i] = item{ID: i, Zh: t}
	}
	inputJSON, _ := json.Marshal(items)

	prompt := "Translate each Chinese " + context + " term to Russian. Keep Latin/English/numbers as-is.\n" +
		"Input: [{\"id\":0,\"zh\":\"...\"},...]\n" +
		"Output ONLY valid JSON: {\"t\":[{\"id\":0,\"ru\":\"...\"},...]}\n\n" +
		string(inputJSON)

	reqBody := map[string]interface{}{
		"model": "deepseek-v4-flash",
		"messages": []map[string]interface{}{
			{"role": "system", "content": "Translate Chinese to Russian. Return only JSON {\"t\":[{\"id\":N,\"ru\":\"...\"}]}."},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.0,
	}

	data, _ := json.Marshal(reqBody)
	apiURL := fmt.Sprintf("%s/chat/completions", c.baseURL)
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(data))
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
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
		Usage usageInfo `json:"usage"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil || len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	apiResp.Usage.log("translateTerms/" + context)

	content := apiResp.Choices[0].Message.Content
	log.Printf("[deepseek] translateTerms(%s) raw: %.500s", context, content)

	// ID может быть числом или строкой — используем json.Number
	var result struct {
		T []struct {
			ID json.Number `json:"id"`
			Ru string      `json:"ru"`
		} `json:"t"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse terms: %w (content: %.300s)", err, content)
	}

	out := make(map[int]string, len(result.T))
	for _, r := range result.T {
		idx, _ := strconv.Atoi(r.ID.String())
		out[idx] = r.Ru
	}
	log.Printf("[deepseek] translateTerms(%s): %d in, %d out", context, len(terms), len(out))
	return out, nil
}

// TranslateAttrs переводит батч атрибутов с китайского на русский через DeepSeek.
// Имена атрибутов и значения переводятся отдельными запросами чтобы не путать их местами.
func (c *DeepSeekClient) TranslateAttrs(pairs []AttrPair) ([]AttrTranslated, error) {
	if len(pairs) == 0 {
		return nil, nil
	}

	// Дедуплицируем: уникальные pid-ы (названия атрибутов) и vid-ы (значения)
	pidIndex := map[string]int{}
	vidIndex := map[string]int{}
	var pidTerms, vidTerms []string
	for _, p := range pairs {
		if _, ok := pidIndex[p.Name]; !ok {
			pidIndex[p.Name] = len(pidTerms)
			pidTerms = append(pidTerms, p.Name)
		}
		if _, ok := vidIndex[p.Value]; !ok {
			vidIndex[p.Value] = len(vidTerms)
			vidTerms = append(vidTerms, p.Value)
		}
	}

	// Переводим названия атрибутов
	nameMap, err := c.translateTerms(pidTerms, "product attribute name")
	if err != nil {
		return nil, fmt.Errorf("translate names: %w", err)
	}

	// Переводим значения
	valueMap, err := c.translateTerms(vidTerms, "product attribute value")
	if err != nil {
		return nil, fmt.Errorf("translate values: %w", err)
	}

	var out []AttrTranslated
	for _, p := range pairs {
		nameRu := nameMap[pidIndex[p.Name]]
		valueRu := valueMap[vidIndex[p.Value]]
		if nameRu == "" {
			nameRu = p.Name
		}
		if valueRu == "" {
			valueRu = p.Value
		}
		out = append(out, AttrTranslated{
			Pid:     p.Pid,
			Vid:     p.Vid,
			NameRu:  nameRu,
			ValueRu: valueRu,
		})
	}
	log.Printf("[deepseek] TranslateAttrs: %d pairs in, %d translated", len(pairs), len(out))
	return out, nil
}


