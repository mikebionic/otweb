package push

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImageDownloader скачивает изображения с alicdn.com и сохраняет на локальный сервер.
// CS-Cart получает URL вида https://wabrum.com/images/otapi/<hash>.jpg
type ImageDownloader struct {
	localDir string // /var/www/.../images/otapi/
	baseURL  string // https://wabrum.com/images/otapi
	client   *http.Client
}

func NewImageDownloader(localDir, baseURL string) *ImageDownloader {
	return &ImageDownloader{
		localDir: localDir,
		baseURL:  strings.TrimRight(baseURL, "/"),
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// DownloadAndGetURL скачивает изображение и возвращает локальный URL.
// Если файл уже скачан — возвращает кэшированный URL без повторного скачивания.
// При ошибке — возвращает пустую строку (пуш продолжится без изображения).
func (d *ImageDownloader) DownloadAndGetURL(imgURL string) string {
	if imgURL == "" || d.localDir == "" {
		return ""
	}

	// Имя файла = MD5 от URL (уникально, детерминировано)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(imgURL)))
	ext := d.guessExt(imgURL)
	filename := hash + ext
	localPath := filepath.Join(d.localDir, filename)

	// Уже скачано — сразу возвращаем URL
	if _, err := os.Stat(localPath); err == nil {
		return d.baseURL + "/" + filename
	}

	// Скачиваем с заголовком Referer (обходит anti-hotlinking Alibaba)
	req, err := http.NewRequest("GET", imgURL, nil)
	if err != nil {
		log.Printf("[img-dl] ERROR build request for %s: %v", imgURL, err)
		return ""
	}
	req.Header.Set("Referer", "https://detail.1688.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := d.client.Do(req)
	if err != nil {
		log.Printf("[img-dl] ERROR fetch %s: %v", imgURL, err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[img-dl] WARN remote returned %d for %s", resp.StatusCode, imgURL)
		return ""
	}

	// Сохраняем во временный файл, потом переименовываем (атомарно)
	tmpPath := localPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		log.Printf("[img-dl] ERROR create file %s: %v", tmpPath, err)
		return ""
	}

	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil || written == 0 {
		os.Remove(tmpPath)
		log.Printf("[img-dl] ERROR write %s: %v (written=%d)", tmpPath, err, written)
		return ""
	}

	if err := os.Rename(tmpPath, localPath); err != nil {
		os.Remove(tmpPath)
		log.Printf("[img-dl] ERROR rename %s: %v", tmpPath, err)
		return ""
	}

	log.Printf("[img-dl] OK %s -> %s (%d bytes)", imgURL, filename, written)
	return d.baseURL + "/" + filename
}

// DownloadAll скачивает список изображений и возвращает локальные URL.
// URL которые не удалось скачать — пропускаются.
func (d *ImageDownloader) DownloadAll(urls []string) []string {
	var result []string
	for _, u := range urls {
		if local := d.DownloadAndGetURL(u); local != "" {
			result = append(result, local)
		}
	}
	return result
}

func (d *ImageDownloader) guessExt(url string) string {
	lower := strings.ToLower(url)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif"} {
		if strings.Contains(lower, ext) {
			return ".jpg" // всегда сохраняем как jpg
		}
	}
	return ".jpg"
}
