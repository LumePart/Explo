package main

import (
	"bytes"
	"explo/src/debug"
	"fmt"
	"log"
	"strings"
)

type EmbyPaths []struct {
	Name           string         `json:"Name"`
	Locations      []string       `json:"Locations"`
	CollectionType string         `json:"CollectionType"`
	ItemID         string         `json:"ItemId"`
	RefreshStatus  string         `json:"RefreshStatus"`
}

type EmbyItemSearch struct {
	Items            []Items `json:"Items"`
	TotalRecordCount int     `json:"TotalRecordCount"`
}

type EmbyItems struct {
	Name              string          `json:"Name"`
	ServerID          string          `json:"ServerId"`
	ID                string          `json:"Id"`
	Path			  string		  `json:"Path"`
	Album             string          `json:"Album,omitempty"`
	AlbumArtist       string          `json:"AlbumArtist,omitempty"`
}

type EmbyPlaylist struct {
	ID string `json:"Id"`
}

func (cfg *Credentials) embyHeader() {
	cfg.Headers = make(map[string]string)

	cfg.Headers["X-Emby-Token"] = cfg.APIKey
	
}

func embyAllPaths(cfg Config) (EmbyPaths, error) {
	params := "/emby/Library/VirtualFolders"

	body, err := makeRequest("GET", cfg.URL+params, nil, cfg.Creds.Headers)
	if err != nil {
		return nil, fmt.Errorf("embyAllPaths(): %s", err.Error())
	}

	var paths EmbyPaths
	if err = parseResp(body, &paths); err != nil {
		return nil, fmt.Errorf("embyAllPaths(): %s", err.Error())
	}
	return paths, nil
}

func (cfg *Config) getEmbyPath()  { // Gets Librarys ID
	paths, err := embyAllPaths(*cfg)
	if err != nil {
		log.Fatalf("failed to get Emby paths: %s", err.Error())
	}

	for _, path := range paths {
		if path.Name == cfg.Jellyfin.LibraryName {
			cfg.Jellyfin.LibraryID = path.ItemID
		}
	}
}

func embyAddPath(cfg Config)  { // adds Jellyfin library, if not set
	params := "/emby/Library/VirtualFolders"
	payload := []byte(fmt.Sprintf(`{
		"Name": "%s",
		"CollectionType": "Music",
		"RefreshLibrary": true,
		"Paths": "%s"
		"LibraryOptions": {
		  "Enabled": true,
		  "EnableRealtimeMonitor": true,
		  "EnableLUFSScan": false
		}
	  }`, cfg.Jellyfin.LibraryName, cfg.Youtube.DownloadDir))

	body, err := makeRequest("POST", cfg.URL+params, bytes.NewReader(payload), cfg.Creds.Headers)
	if err != nil {
		debug.Debug(fmt.Sprintf("response: %s", body))
		log.Fatalf("failed to add library to Emby using the download path, please define a library name using LIBRARY_NAME in .env: %s", err.Error())
	}
}

func refreshEmbyLibrary(cfg Config) error {
	params := fmt.Sprintf("/emby/Items/%s/Refresh", cfg.Jellyfin.LibraryID)

	if _, err := makeRequest("POST", cfg.URL+params, nil, cfg.Creds.Headers); err != nil {
		return fmt.Errorf("refreshEmbyLibrary(): %s", err.Error())
	}
	return nil
}

func getEmbySong(cfg Config, track Track) (string, error) { // Gets all files in Explo library and filters out new ones
	params := fmt.Sprintf("/emby/Items?parentId=%s&fields=Path", cfg.Jellyfin.LibraryID)

	body, err := makeRequest("GET", cfg.URL+params, nil, cfg.Creds.Headers)
	if err != nil {
		return "", fmt.Errorf("getEmbySong(): %s", err.Error())
	}

	var results Audios
	if err = parseResp(body, &results); err != nil {
		return "", fmt.Errorf("getEmbySong(): %s", err.Error())
	}

	for _, item := range results.Items {
		if strings.Contains(item.Path, track.File) {
			return item.ID, nil
		}
	}
	return "", nil
}

func findEmbyPlaylist(cfg Config) (string, error) {
	params := fmt.Sprintf("/emby/Items?SearchTerm=%s&Recursive=true&IncludeItemTypes=Playlist", cfg.PlaylistName)

	body, err := makeRequest("GET", cfg.URL+params, nil, cfg.Creds.Headers)
	if err != nil {
		return "", fmt.Errorf("failed to find playlist: %s", err.Error())
	}

	var results EmbyItemSearch
	if err = parseResp(body, &results); err != nil {
		return "", fmt.Errorf("findJfPlaylist(): %s", err.Error())
	}
	if len(results.Items) != 0 {
		return results.Items[0].ID, nil
	} else {
		return "", fmt.Errorf("no results found for %s", cfg.PlaylistName)
	}
}

func createEmbyPlaylist(cfg Config, tracks []Track) (string, error) {
	var songIDs []string
	
	for _, track := range tracks {
		if track.ID == "" {
			songID, err := getEmbySong(cfg, track)
			if songID == "" || err != nil {
				debug.Debug(fmt.Sprintf("could not get %s", track.File))
				continue
			}
			track.ID = songID
		}
		songIDs = append(songIDs, track.ID)
	}
	IDs := strings.Join(songIDs, ",")

	params := fmt.Sprintf("/emby/Playlists?Name=%s&Ids=%s&MediaType=Music", cfg.PlaylistName, IDs)


	body, err := makeRequest("POST", cfg.URL+params, nil, cfg.Creds.Headers)
	if err != nil {
		return "", fmt.Errorf("createEmbyPlaylist(): %s", err.Error())
	}
	var playlist EmbyPlaylist
	if err = parseResp(body, &playlist); err != nil {
		return "", fmt.Errorf("createEmbyPlaylist(): %s", err.Error())
	}
	return playlist.ID, nil
}

/* func updateEmbyPlaylist(cfg Config, ID, overview string) error {
	params := fmt.Sprintf("/emby/Items/%s", ID)

	payload := []byte(fmt.Sprintf(`
		{
		"Id":"%s",
		"Name":"%s",
		"Overview":"%s",
		"Genres":[],
		"Tags":[],
		"ProviderIds":{}
		}`, ID, cfg.PlaylistName, overview)) // the additional fields have to be added, otherwise JF returns code 400

	if _, err := makeRequest("POST", cfg.URL+params, bytes.NewBuffer(payload), cfg.Creds.Headers); err != nil {
		return err
	}
	return nil
} */

func deleteEmbyPlaylist(cfg Config, ID string) error {
	params := fmt.Sprintf("/emby/Items/Delete?Ids=%s", ID)

	if _, err := makeRequest("POST", cfg.URL+params, nil, cfg.Creds.Headers); err != nil {
		return fmt.Errorf("deleteEmbyPlaylist(): %s", err.Error())
	}
	return nil
}