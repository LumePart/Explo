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

	"explo/src/config"
	"explo/src/logging"
)

// Bounds for ad-hoc remote fetches: a stalled server must not hang the
// app forever and an oversized response must not be read into memory
// without limit (BUG-20).
const (
	maxAPIResponseBytes = 32 << 20
	maxDownloadBytes    = 64 << 20
	maxCoverBytes       = 16 << 20
)

// remoteHTTPClient replaces http.Get/DefaultClient, which have no
// timeout at all, for file and cover downloads.
var remoteHTTPClient = &http.Client{Timeout: 60 * time.Second}

// readLimited reads at most max bytes; one extra byte is read so an
// over-limit response is an error, not a silent truncation.
func readLimited(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("response exceeds %d bytes", max)
	}
	return data, nil
}

type HttpClientConfig struct {
	Timeout int
}

type HttpClient struct {
	Client    *http.Client
	UserAgent string
}

func NewHttp(cfg HttpClientConfig) *HttpClient {
	return &HttpClient{
		Client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		UserAgent: fmt.Sprintf("Explo/%s ( https://github.com/LumePart/explo )", config.Version),
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

	body, err := readLimited(resp.Body, maxAPIResponseBytes)
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
	resp, err := remoteHTTPClient.Get(url)
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
	data, err := readLimited(resp.Body, maxDownloadBytes)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	return destPath, nil
}

// DownloadCover downloads coverURL into coversDir and returns cover api and filesystem path.
// For CoverArtArchive URLs the id is the MusicBrainz release MBID (second-to-last
// path segment). For Spotify CDN URLs (i.scdn.co) the id is the image hash (last
// path segment). Returns "" if url is empty.
func DownloadCover(url, coversDir string) (string, string) {
	if url == "" {
		return "", ""
	}
	parts := strings.Split(strings.TrimRight(url, "/"), "/")

	if len(parts) < 2 {
	return "", ""
}
	// Spotify CDN: https://i.scdn.co/image/<hash>  → use last segment
	// CAA:         https://coverartarchive.org/release/<mbid>/front-250 → use second-to-last
	id := parts[len(parts)-2]
	if strings.Contains(url, "scdn.co") || strings.Contains(url, "spotifycdn.com") {
		id = parts[len(parts)-1]
	}
	destPath := filepath.Join(coversDir, id+".jpg")
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		resp, err := remoteHTTPClient.Get(url)
		if err == nil {
			func() {
				defer func() {
					if cerr := resp.Body.Close(); cerr != nil {
						slog.Error("failed to close cover response", "err", cerr.Error())
					}
				}()
				if resp.StatusCode == http.StatusOK {
					if data, err := readLimited(resp.Body, maxCoverBytes); err == nil {
						if err := os.WriteFile(destPath, data, 0644); err != nil {
							slog.Error("failed writing cover", "path", destPath, "err", err.Error())
						}
					}
				}
			}()
		}
	}
	// A failed fetch or write used to return the destination path anyway,
	// so callers embedded a cover from a file that does not exist and the
	// track save failed with it (BUG-25). Only hand out paths that exist.
	if _, err := os.Stat(destPath); err != nil {
		return "", ""
	}
	apiURL := fmt.Sprintf("/api/covers/%s.jpg", id)
	return apiURL, destPath
}
