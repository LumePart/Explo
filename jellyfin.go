package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
)

type Paths []struct {
	Name           string         `json:"Name"`
	Locations      []string       `json:"Locations"`
	CollectionType string         `json:"CollectionType"`
	ItemID         string         `json:"ItemId"`
	RefreshStatus  string         `json:"RefreshStatus"`
}

type Search struct {
	SearchHints      []SearchHints `json:"SearchHints"`
	TotalRecordCount int           `json:"TotalRecordCount"`
}
type SearchHints struct {
	ItemID                  string    `json:"ItemId"`
	ID                      string    `json:"Id"`
	Name                    string    `json:"Name"`
	Album                   string    `json:"Album"`
	AlbumID                 string    `json:"AlbumId"`
	AlbumArtist             string    `json:"AlbumArtist"`
}

type Audios struct {
	Items            []Items `json:"Items"`
	TotalRecordCount int     `json:"TotalRecordCount"`
	StartIndex       int     `json:"StartIndex"`
}

type Items struct {
	Name              string          `json:"Name"`
	ServerID          string          `json:"ServerId"`
	ID                string          `json:"Id"`
	Path			  string		  `json:"Path"`
	Album             string          `json:"Album,omitempty"`
	AlbumArtist       string          `json:"AlbumArtist,omitempty"`

}

func (cfg *Credentials) jfHeader() {
	cfg.Headers = make(map[string]string)

	cfg.Headers["Authorization"] = fmt.Sprintf("MediaBrowser Token=%s, Client=Explo", cfg.APIKey)
	
}

func jfAllPaths(cfg Config) (Paths, error) {
	params := "/Library/VirtualFolders"

	body, err := makeRequest("GET", cfg.URL+params, nil, cfg.Creds.Headers)
	if err != nil {
		return nil, err
	}

	var paths Paths
	json.Unmarshal(body, &paths)

	return paths, nil
}

func (cfg *Config) getJfPath()  { // Gets Librarys ID
	paths, err := jfAllPaths(*cfg)
	if err != nil {
		log.Fatalf("failed to get paths: %s", err.Error())
	}

	for _, path := range paths {
		if path.Name == cfg.Jellyfin.LibraryName {
			cfg.Jellyfin.LibraryID = path.ItemID
		}
	}
}

func jfAddPath(cfg Config)  { // adds Jellyfin library, if not set
	cleanPath := url.PathEscape(cfg.Youtube.DownloadDir)
	params := fmt.Sprintf("/Library/VirtualFolders?name=%s&paths=%s&collectionType=music&refreshLibrary=true", cfg.Jellyfin.LibraryName, cleanPath)
	payload := []byte(`{
		"LibraryOptions": {
		  "Enabled": true,
		  "EnableRealtimeMonitor": true,
		  "EnableLUFSScan": false
		}
	  }`)

	_, err := makeRequest("POST", cfg.URL+params, bytes.NewReader(payload), cfg.Creds.Headers)
	if err != nil {
		log.Fatalf("failed to add path: %s", err.Error())
	}
}

func refreshJfLibrary(cfg Config) error {
	params := fmt.Sprintf("/Items/%s/Refresh", cfg.Jellyfin.LibraryID)

	_, err := makeRequest("POST", cfg.URL+params, nil, cfg.Creds.Headers)
	if err != nil {
		return fmt.Errorf("failed to refresh library: %s", err.Error())
	}
	return nil
}

func getJfSongs(cfg Config, files []string) ([]string, error) { // Gets all files in Explo library and filters out new ones
	params := fmt.Sprintf("/Items?parentId=%s&fields=Path", cfg.Jellyfin.LibraryID)
	var songIDs []string

	body, err := makeRequest("GET", cfg.URL+params, nil, cfg.Creds.Headers)
	if err != nil {
		return songIDs, fmt.Errorf("failed to find song: %s", err.Error())
	}

	var results Audios

	err = json.Unmarshal(body, &results)
	if err != nil {
		return songIDs, fmt.Errorf("failed to unmarshal body: %s", err.Error())
	}


	for _, file := range files {
		for _, item := range results.Items {
			if fmt.Sprintf("%s%s.mp3", cfg.Youtube.DownloadDir, file) == item.Path {
				songIDs = append(songIDs, item.ID)
			}
		}
	}
	return songIDs, nil
}

func findJfPlaylist(cfg Config) (string, error) {
	params := fmt.Sprintf("/Search/Hints?searchTerm=%s&mediaTypes=Playlist", cfg.PlaylistName)

	body, err := makeRequest("GET", cfg.URL+params, nil, cfg.Creds.Headers)
	if err != nil {
		return "", fmt.Errorf("failed to find playlist: %s", err.Error())
	}

	var results Search

	err = json.Unmarshal(body, &results)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal body: %s", err.Error())
	}
	return results.SearchHints[0].ID, nil
}

func createJfPlaylist(cfg Config, files []string) error {
	
	songIDs, err := getJfSongs(cfg, files)
	if err != nil {
		return err
	}
	
	params := "/Playlists"

	IDs, err := json.Marshal(songIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal songIDs: %s", err.Error())
	}

	payload := []byte(fmt.Sprintf(`
		{
		"Name": "%s",
		"Ids": %s,
		"MediaType": "Audio",
		"UserId": "%s"
		}`, cfg.PlaylistName, IDs, cfg.Creds.APIKey))
	

	_, err = makeRequest("POST", cfg.URL+params, bytes.NewReader(payload), cfg.Creds.Headers)
	if err != nil {
		return fmt.Errorf("failed to create playlist: %s", err.Error())
	}
	return nil
}

func deleteJfPlaylist(cfg Config, ID string) error {
	params := fmt.Sprintf("/Items/%s", ID)

	_, err := makeRequest("DELETE", cfg.URL+params, nil, cfg.Creds.Headers)
	if err != nil {
		return fmt.Errorf("failed to delete playlist: %s", err.Error())
	}
	return nil
}