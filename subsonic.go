package main

import (
	"fmt"
	"log"
	"time"
	"strings"

	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/url"
)

type FailedResp struct {
	SubsonicResponse struct {
		Status        string `json:"status"`
		Error         struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"subsonic-response"`
}

type SubResponse struct {
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


func (cfg *Credentials) genToken() {
	
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

}

func searchTrack(cfg Config, track string) (string, error) {

    cleanedTrack := url.QueryEscape(track)
    
    reqParam := fmt.Sprintf("search3?query=%s&f=json", cleanedTrack)
	
    body, err := subsonicRequest(reqParam, cfg)
    if err != nil {
        return "", err
    }
    
    var resp SubResponse
    if err := json.Unmarshal(body, &resp); err != nil {
        return "", err
    }
    
    if len(resp.SubsonicResponse.SearchResult3.Song) < 1 {
        return "", nil
    }
    return resp.SubsonicResponse.SearchResult3.Song[0].ID, nil
}

func subsonicPlaylist(cfg Config, songs []Song) error {

	var trackIDs string
	var reqParam string

	for _, song := range songs { // Get track IDs from app and format them
		ID, err := searchTrack(cfg, fmt.Sprintf("%s %s %s", song.Title, song.Artist, song.Album))
		if ID  == "" || err != nil  { // if ID is empty, skip song
			continue
		}
		trackIDs += "&songId="+ID
	}
	reqParam = fmt.Sprintf("createPlaylist?name=%s%s", cfg.PlaylistName, trackIDs)
	
	_, err := subsonicRequest(reqParam, cfg)
	if err != nil {
		return err
	}
	return nil
}

func subsonicScan(cfg Config) error {
	reqParam := "startScan?f=json"
	
	_, err := subsonicRequest(reqParam, cfg)
	if err != nil {
		return err
	}
	return nil
}

func getDiscoveryPlaylist(cfg Config) ([]string, error) {

	var resp SubResponse
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

func delSubsonicPlaylists(playlists []string, cfg Config) error {

	for _, id := range playlists {
		reqParam := fmt.Sprintf("deletePlaylist?id=%s", id)
		_, err := subsonicRequest(reqParam, cfg)
		if err != nil {
			return err
		}
	}
	return nil
}

func subsonicRequest(reqParams string, cfg Config) ([]byte, error) {

	reqURL := fmt.Sprintf("%s/rest/%s&u=%s&t=%s&s=%s&v=%s&c=%s", cfg.URL, reqParams, cfg.Creds.User, cfg.Creds.Token, cfg.Creds.Salt, cfg.Subsonic.Version, cfg.Subsonic.ID)

	body, err := makeRequest("GET", reqURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to make request %v", err)
	}

	var checkResp FailedResp

	err = json.Unmarshal(body, &checkResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal request %v", err)
	} else if checkResp.SubsonicResponse.Status == "failed" {
		return nil, fmt.Errorf("%s", checkResp.SubsonicResponse.Error.Message)
	}

	return body, nil
}