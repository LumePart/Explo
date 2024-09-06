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
		Playlists     struct {
			Playlist []struct {
				ID        string    `json:"id"`
				Name      string    `json:"name"`
				Comment   string    `json:"comment,omitempty"`
				SongCount int       `json:"songCount"`
				Duration  int       `json:"duration"`
				Public    bool      `json:"public"`
				Owner     string    `json:"owner"`
				Created   time.Time `json:"created"`
				Changed   time.Time `json:"changed"`
				CoverArt  string    `json:"coverArt"`
			} `json:"playlist"`
		} `json:"playlists,omitempty"`
	} `json:"subsonic-response"`
}


func genToken(cfg Subsonic) Subsonic {
	
	var salt = make([]byte, 6)


	_, err := rand.Read(salt)
	if err != nil {
		log.Fatalf("failed to read salt: %v", err)
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
func subsonicPlaylist(cfg Subsonic, songs []string, playlistName string) error {

	var trackIDs string
	var reqParam string

	for _, song := range songs { // Get track IDs from app and format them
		ID, err := searchTrack(cfg, song)
		if ID  == "" || err != nil  { // if ID is empty, skip song
			continue
		}
		trackIDs += "&songId="+ID
	}
	reqParam = fmt.Sprintf("createPlaylist?name=%s%s", playlistName, trackIDs)
	
	_, err := subsonicRequest(reqParam, cfg)
	if err != nil {
		return err
	}
	return nil
}

func scan(cfg Subsonic) error {

	reqParam := "startScan?f=json"
	
	_, err := subsonicRequest(reqParam, cfg)
	if err != nil {
		return err
	}
	return nil
}

func getDiscoveryPlaylist(cfg Subsonic) ([]string, error) {

	var resp Response
	var playlists []string
	reqParam := "getPlaylists?f=json"

	body, err := subsonicRequest(reqParam, cfg)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &resp); err != nil {
        return nil, err
    }

	for _, playlist := range resp.SubsonicResponse.Playlists.Playlist {
		if strings.Contains(playlist.Name, "Discover-Weekly") {
			playlists = append(playlists, playlist.ID)

		}
	}

	return playlists, nil
}

func delPlaylists(playlists []string, cfg Subsonic) error {

	for _, id := range playlists {
		reqParam := fmt.Sprintf("deletePlaylist?id=%s", id)
		_, err := subsonicRequest(reqParam, cfg)
		if err != nil {
			return err
		}
	}
	return nil
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