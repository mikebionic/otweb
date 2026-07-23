package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Дисковый кэш проксируемых картинок. alicdn (China CDN) из РФ-сервера тянется с
// «плавающей» латентностью (первый запрос ~5с даже для thumbnail). Кэшируем на диск:
// первая загрузка наполняет кэш, дальше отдаём мгновенно всем пользователям.
const imgCacheDir = "/opt/otapi-hub-src/imgcache"

var imgCacheOnce sync.Once

func imgCacheKey(u string) string {
	h := sha1.Sum([]byte(u))
	return hex.EncodeToString(h[:])
}

// cachedImageFetch отдаёт байты картинки + content-type, используя дисковый кэш.
func cachedImageFetch(imgURL string) ([]byte, string, error) {
	imgCacheOnce.Do(func() { _ = os.MkdirAll(imgCacheDir, 0o755) })
	fpath := filepath.Join(imgCacheDir, imgCacheKey(imgURL))
	if data, err := os.ReadFile(fpath); err == nil && len(data) > 0 {
		return data, http.DetectContentType(data), nil
	}
	req, err := http.NewRequest("GET", imgURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Referer", "https://detail.1688.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("remote %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	_ = os.WriteFile(fpath, data, 0o644) // best-effort, кэш не критичен
	return data, ct, nil
}
