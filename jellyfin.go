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
	params := "/Library/VirtualFolders/Paths"
	payload := []byte(fmt.Sprintf(`{
		"Name": "%s",
		"Path": "%s"
		}`, cfg.Jellyfin.LibraryName, cfg.Youtube.DownloadDir))

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

func findJfSong(cfg Config, song Song) (string, error) {
	frmtSong := url.QueryEscape(song.Title)
	params := fmt.Sprintf("/Search/Hints?searchTerm=%s&mediaTypes=Audio", frmtSong)

	body, err := makeRequest("GET", cfg.URL+params, nil, cfg.Creds.Headers)
	if err != nil {
		return "", fmt.Errorf("failed to find song: %s", err.Error())
	}

	var results Search

	err = json.Unmarshal(body, &results)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal body: %s", err.Error())
	}
	if results.TotalRecordCount > 0 {
		for _, result := range results.SearchHints {
			if result.Name == song.Title && result.Album == song.Album && result.AlbumArtist == song.Artist {
				return result.ID, nil
			}
		}
	} else if results.TotalRecordCount == 0 {
		log.Printf("did not find any result for %s, does it exist in library?", song.Title)
		return "", nil
	}
	return "", fmt.Errorf("failed to find song")
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

func createJfPlaylist(cfg Config, songs []Song) error {
	var songIDs []string

	for _, song := range songs {
		ID, err := findJfSong(cfg, song)
		if err != nil {
			return err
		}

		songIDs = append(songIDs, ID)
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