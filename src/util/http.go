package util

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"explo/src/logging"
)

type HttpClientConfig struct {
	Timeout int
}

type HttpClient struct {
	Client *http.Client
	UserAgent string
}

func NewHttp(cfg HttpClientConfig) *HttpClient {
	return &HttpClient{
		Client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		UserAgent: "Explo (+https://github.com/LumePart/explo))",
	}
}

func (c *HttpClient) MakeRequest(method, url string, payload io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize request: %s", err.Error())
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", c.UserAgent)

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %s", err.Error())
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("response body close failed", "context", err.Error())
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Debug("response info", logging.RuntimeAttr(string(body)))
		return nil, fmt.Errorf("got %d from %s", resp.StatusCode, url)
	}

	return body, nil
}

func ParseResp[T any](body []byte, target *T) error {

	if err := json.Unmarshal(body, target); err != nil {
		slog.Debug("response info", logging.RuntimeAttr(string(body)))
		return fmt.Errorf("error unmarshaling response body: %s", err.Error())
	}
	return nil
}

// DownloadFile downloads a URL to destPath, creating parent directories as needed.
// No-op if destPath already exists. Returns the resolved local path on success.
func DownloadFile(url, destPath string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("empty url")
	}
	if _, err := os.Stat(destPath); err == nil {
		return destPath, nil
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return "", fmt.Errorf("get: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Warn("DownloadFile: close failed", "err", cerr.Error())
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d from %s", resp.StatusCode, url)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	return destPath, nil
}

// DownloadCover downloads coverURL into coversDir and returns "/api/covers/<id>.jpg".
// For CoverArtArchive URLs the id is the MusicBrainz release MBID (second-to-last
// path segment). For Spotify CDN URLs (i.scdn.co) the id is the image hash (last
// path segment). Returns "" if url is empty.
func DownloadCover(url, coversDir string) string {
	if url == "" {
		return ""
	}
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	// Spotify CDN: https://i.scdn.co/image/<hash>  → use last segment
	// CAA:         https://coverartarchive.org/release/<mbid>/front-250 → use second-to-last
	id := parts[len(parts)-2]
	if strings.Contains(url, "scdn.co") || strings.Contains(url, "spotifycdn.com") {
		id = parts[len(parts)-1]
	}
	destPath := filepath.Join(coversDir, id+".jpg")
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		resp, err := http.Get(url) //nolint:noctx
		if err == nil {
			func() {
				defer func() {
					if cerr := resp.Body.Close(); cerr != nil {
						slog.Error("failed to close cover response", "err", cerr.Error())
					}
				}()
				if resp.StatusCode == http.StatusOK {
					if data, err := io.ReadAll(resp.Body); err == nil {
						if err := os.WriteFile(destPath, data, 0644); err != nil {
							slog.Error("failed writing cover", "path", destPath, "err", err.Error())
						}
					}
				}
			}()
		}
	}
	return "/api/covers/" + id + ".jpg"
}
