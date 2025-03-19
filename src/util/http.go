package util

import (
	"net/http"
	"encoding/json"
	"fmt"
	"io"
	"explo/src/debug"
)

func MakeRequest(method, url string, payload io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize request: %s", err.Error())
	}
	req.Header.Add("Content-Type","application/json")
	req.Header.Add("Accept", "application/json")

	for key, value := range headers {
		req.Header.Add(key,value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %s", err.Error())
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err.Error())
	}
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("got %d from %s", resp.StatusCode, url)
	}

	return body, nil
}

func ParseResp[T any](body []byte, target *T) error {
	
	if err := json.Unmarshal(body, target); err != nil {
		debug.Debug(fmt.Sprintf("full response: %s", string(body)))
		return fmt.Errorf("error unmarshaling response body: %s", err.Error())
	}
	return nil
}