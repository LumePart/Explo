package util

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"explo/src/debug"
)

type HttpClientConfig struct {
	Timeout int
}

type HttpClient struct {
	Client *http.Client
}

func NewHttp(cfg HttpClientConfig) *HttpClient {
	return &HttpClient{
		Client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
	}
}

func (c *HttpClient) MakeRequest(method, url string, payload io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize request: %s", err.Error())
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

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
		slog.Debug("response info", debug.RuntimeAttr(string(body)))
		return nil, fmt.Errorf("got %d from %s", resp.StatusCode, url)
	}

	return body, nil
}

func ParseResp[T any](body []byte, target *T) error {

	if err := json.Unmarshal(body, target); err != nil {
		slog.Debug("response info", debug.RuntimeAttr(string(body)))
		return fmt.Errorf("error unmarshaling response body: %s", err.Error())
	}
	return nil
}
