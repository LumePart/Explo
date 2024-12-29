package main

import (
	"fmt"
	"log"
	"time"
	"strings"

	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"net/url"
	"explo/debug"
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
			Playlist []Playlist `json:"playlist"`
		} `json:"playlists,omitempty"`
	} `json:"subsonic-response"`
}

type Playlist struct {
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
}


func (cfg *Credentials) genToken() {
	
	var salt = make([]byte, 6)


	_, err := rand.Read(salt)
	if err != nil {
		log.Fatalf("failed to read salt: %s", err.Error())
	}

	saltStr := base64.RawURLEncoding.EncodeToString(salt)
	passSalt := fmt.Sprintf("%s%s", cfg.Password, saltStr)

	token := fmt.Sprintf("%x", md5.Sum([]byte(passSalt)))

	cfg.Token = url.PathEscape(token)
	cfg.Salt = url.PathEscape(saltStr)

}

func searchTrack(cfg Config, track Track) (string, error) {

	searchQuery := fmt.Sprintf("%s %s %s", track.Title, track.Artist, track.Album)

    cleanedQuery := url.QueryEscape(searchQuery)
    
    reqParam := fmt.Sprintf("search3?query=%s&f=json", cleanedQuery)
	
    body, err := subsonicRequest(reqParam, cfg)
    if err != nil {
        return "", err
    }
    
    var resp SubResponse
    if err := parseResp(body, &resp); err != nil {
        return "", err
    }
    

	switch len(resp.SubsonicResponse.SearchResult3.Song) {
	case 0:
		return "", fmt.Errorf("no results found for %s", searchQuery)
	case 1:
		return resp.SubsonicResponse.SearchResult3.Song[0].ID, nil
	default:
		for _, song := range resp.SubsonicResponse.SearchResult3.Song {
			if song.Title == track.Title {
				return song.ID, nil
			}
		}
		return "", fmt.Errorf("multiple songs found for: %s, but titles do not match with the actual track", searchQuery)
	}
}

func subsonicPlaylist(cfg Config, tracks []Track) (string, error) {

	var trackIDs string

	for _, track := range tracks { // Get track IDs from app and format them
		if track.ID == "" {
			songID, err := searchTrack(cfg, track)
			if songID  == "" || err != nil  { // if ID is empty, skip song
				debug.Debug(fmt.Sprintf("could not get %s", track.File))
				continue
			}
			track.ID = songID
		}
		trackIDs += "&songId="+track.ID
	}
	
	reqParam := fmt.Sprintf("createPlaylist?name=%s%s&f=json", cfg.PlaylistName, trackIDs)
	
	body, err := subsonicRequest(reqParam, cfg)

	var playlist Playlist
	if err := parseResp(body, &playlist); err != nil {
        return "", err
    }
	if err != nil {
		return "", err
	}
	return playlist.ID, nil
}

func subsonicScan(cfg Config) error {
	reqParam := "startScan?f=json"
	
	if _, err := subsonicRequest(reqParam, cfg); err != nil {
		return err
	}
	return nil
}

func getDiscoveryPlaylist(cfg Config) ([]string, error) {
	reqParam := "getPlaylists?f=json"

	body, err := subsonicRequest(reqParam, cfg)
	if err != nil {
		return nil, err
	}

	var resp SubResponse
	if err := parseResp(body, &resp); err != nil {
        return nil, err
    }

	var playlists []string
	for _, playlist := range resp.SubsonicResponse.Playlists.Playlist {
		if strings.Contains(playlist.Name, "Discover-Weekly") {
			playlists = append(playlists, playlist.ID)

		}
	}
	return playlists, nil
}

func updSubsonicPlaylist(cfg Config, ID, comment string) error {
	reqParam := fmt.Sprintf("updatePlaylist?id=%s&comment=%s", ID, comment)

	if _, err := subsonicRequest(reqParam, cfg); err != nil {
		return err
	}
	return nil
}

func delSubsonicPlaylists(playlists []string, cfg Config) error {

	for _, id := range playlists {
		reqParam := fmt.Sprintf("deletePlaylist?id=%s&f=json", id)
		if _, err := subsonicRequest(reqParam, cfg); err != nil {
			return err
		}
	}
	return nil
}

func subsonicRequest(reqParams string, cfg Config) ([]byte, error) {

	reqURL := fmt.Sprintf("%s/rest/%s&u=%s&t=%s&s=%s&v=%s&c=%s", cfg.URL, reqParams, cfg.Creds.User, cfg.Creds.Token, cfg.Creds.Salt, cfg.Subsonic.Version, cfg.Subsonic.ID)

	body, err := makeRequest("GET", reqURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to make request %s", err.Error())
	}

	var checkResp FailedResp
	if err = parseResp(body, &checkResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request %s", err.Error())
	} else if checkResp.SubsonicResponse.Status == "failed" {
		return nil, fmt.Errorf("%s", checkResp.SubsonicResponse.Error.Message)
	}
	return body, nil
}