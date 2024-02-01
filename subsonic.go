package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/url"
)

type Response struct {
	SubsonicResponse struct {
		Status        string        `json:"status"`
		Version       string        `json:"version"`
		Type          string        `json:"type"`
		ServerVersion string        `json:"serverVersion"`
		SearchResult3 struct {
			Song []struct {
				ID          string    `json:"id"`
				Title       string    `json:"title"`
			} `json:"song"`
		} `json:"searchResult3,omitempty"`
	} `json:"subsonic-response"`
}

func genToken(cfg Subsonic) Subsonic { // Generate token for subsonic authentication
	var salt = make([]byte, 6)


	_, err := rand.Read(salt[:])
	if err != nil {
		log.Fatalf("Failed to read salt: %v", err)
	}

	saltStr := base64.StdEncoding.EncodeToString(salt)
	passSalt := fmt.Sprintf("%s%s", cfg.Password, saltStr)

	token := fmt.Sprintf("%x", md5.Sum([]byte(passSalt)))

	cfg.Token = url.PathEscape(token)
	cfg.Salt = url.PathEscape(saltStr)

	return cfg
}

func searchTrack(cfg Subsonic, client http.Client, track string) string {
	cleanedTrack := url.QueryEscape(track)

	reqParam := fmt.Sprintf("search3?query=%s&f=json",cleanedTrack)
	body := subsonicRequest(client, reqParam, cfg)
	var resp Response

	json.Unmarshal(body, &resp)

	return resp.SubsonicResponse.SearchResult3.Song[0].ID
	
}

func createPlaylist(cfg Subsonic, tracks []string) {
	client := http.Client{}

	var trackIDs string
	for _, track := range tracks { // Get track IDs from app and format them
		ID := searchTrack(cfg, client, track)
		trackIDs += "&songId="+ID
	}
	year, week := time.Now().ISOWeek()
	reqParam := fmt.Sprintf("createPlaylist?name=Discover-Weekly-%v-Week%v&%s", year, week, trackIDs)
	subsonicRequest(client, reqParam, cfg)
}

func scan(cfg Subsonic) {
	client := http.Client{}

	reqParam := "startScan?f=json"
	subsonicRequest(client, reqParam, cfg)
}

func subsonicRequest(client http.Client, reqParams string, cfg Subsonic) []byte { // Handle subsonic API requests
	reqURL := fmt.Sprintf("%s/rest/%s&u=%s&t=%s&s=%s&v=%s&c=%s", cfg.URL, reqParams, cfg.User, cfg.Token, cfg.Salt, cfg.Version, cfg.ID)

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		log.Fatalf("Failed to initialize request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to make request: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}

	return body
}