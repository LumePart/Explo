package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
	"strings"

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

func genToken(cfg Subsonic) Subsonic {
	var salt = make([]byte, 6)


	_, err := rand.Read(salt[:])
	if err != nil {
		log.Fatalf("failed to read salt: %s", err.Error())
	}

	saltStr := base64.StdEncoding.EncodeToString(salt)
	passSalt := fmt.Sprintf("%s%s", cfg.Password, saltStr)

	token := fmt.Sprintf("%x", md5.Sum([]byte(passSalt)))

	cfg.Token = url.PathEscape(token)
	cfg.Salt = url.PathEscape(saltStr)

	return cfg
}

func searchTrack(cfg Subsonic, track string) (string, error) {
    cleanedTrack := url.QueryEscape(track)
    
    reqParam := fmt.Sprintf("search3?query=%s&f=json", cleanedTrack)
    body, err := subsonicRequest(reqParam, cfg)
    if err != nil {
        return "", err
    }
    
    var resp Response
    if err := json.Unmarshal(body, &resp); err != nil {
        return "", err
    }
    
    if len(resp.SubsonicResponse.SearchResult3.Song) < 1 {
        return "", nil
    }
    return resp.SubsonicResponse.SearchResult3.Song[0].ID, nil
}

func createPlaylist(cfg Subsonic, tracks []string) error {
	var trackIDs strings.Builder

	for _, track := range tracks { // Get track IDs from app and format them
		ID, err := searchTrack(cfg, track)
		if err != nil {
            return err
        }
        if ID == "" { // If no ID is found, ignore track
			log.Printf("can't find ID for %s", track)
            continue
        }
        trackIDs.WriteString("&songId=" + ID)
	}
	year, week := time.Now().ISOWeek()
	reqParam := fmt.Sprintf("createPlaylist?name=Discover-Weekly-%v-Week%v%s", year, week, trackIDs.String())
	_, err := subsonicRequest(reqParam, cfg)

	return err
}

func scan(cfg Subsonic) {

	reqParam := "startScan?f=json"
	_, err := subsonicRequest(reqParam, cfg)
	if err != nil {
		log.Println("failed to initialize music folder scan")
	}
}

func subsonicRequest(reqParams string, cfg Subsonic) ([]byte, error) {
	reqURL := fmt.Sprintf("%s/rest/%s&u=%s&t=%s&s=%s&v=%s&c=%s", cfg.URL, reqParams, cfg.User, cfg.Token, cfg.Salt, cfg.Version, cfg.ID)

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return body, nil
}