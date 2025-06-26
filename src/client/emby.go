package client

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"
	"net/url"

	"explo/src/config"
	"explo/src/debug"
	"explo/src/models"
	"explo/src/util"
)

type EmbyPaths []struct {
	Name           string         `json:"Name"`
	Locations      []string       `json:"Locations"`
	CollectionType string         `json:"CollectionType"`
	ItemID         string         `json:"ItemId"`
	RefreshStatus  string         `json:"RefreshStatus"`
}

type EmbyItemSearch struct {
	Items            []EmbyItems `json:"Items"`
	TotalRecordCount int     `json:"TotalRecordCount"`
}

type EmbyItems struct {
	Name              string          `json:"Name"`
	ServerID          string          `json:"ServerId"`
	ID                string          `json:"Id"`
	Path			  string		  `json:"Path"`
	Album             string          `json:"Album,omitempty"`
	AlbumArtist       string          `json:"AlbumArtist,omitempty"`
	Artists           []string  	  `json:"Artists"`
}

type EmbyPlaylist struct {
	ID string `json:"Id"`
}

type Emby struct {
	LibraryID string
	HttpClient *util.HttpClient
	Cfg config.ClientConfig
}

func NewEmby(cfg config.ClientConfig, httpClient *util.HttpClient) *Emby {
	return &Emby{Cfg: cfg,
	HttpClient: httpClient}
}

func (c *Emby) AddHeader() error {
	if c.Cfg.Creds.Headers == nil {
		c.Cfg.Creds.Headers = make(map[string]string)
		c.Cfg.Creds.Headers["X-Emby-Client"] = c.Cfg.ClientID
	}

	if c.Cfg.Creds.APIKey != "" {
		c.Cfg.Creds.Headers["X-Emby-Token"] = c.Cfg.Creds.APIKey
		return nil
	}
	return fmt.Errorf("API_KEY not set")
}

func (c *Emby) GetAuth() error {
	return nil
}

func (c *Emby) GetLibrary() error {
	reqParam := "/emby/Library/VirtualFolders"

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}

	var paths EmbyPaths
	if err = util.ParseResp(body, &paths); err != nil {
		return err
	}
	for _, path := range paths {
		if path.Name == c.Cfg.LibraryName {
			c.LibraryID = path.ItemID
			return nil
		}
	}

	return fmt.Errorf("failed to find library named %s", c.Cfg.LibraryName)
}

func (c *Emby) AddLibrary() error {
	reqParam := "/emby/Library/VirtualFolders"

	payload := fmt.Appendf(nil, `{
		"Name": "%s",
		"CollectionType": "Music",
		"RefreshLibrary": true,
		"Paths": "%s"
		"LibraryOptions": {
		  "Enabled": true,
		  "EnableRealtimeMonitor": true,
		  "EnableLUFSScan": false
		}
	  }`, c.Cfg.LibraryName, c.Cfg.DownloadDir)

	if _, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParam, bytes.NewReader(payload), c.Cfg.Creds.Headers); err != nil {
		log.Fatalf("failed to add library to Emby using the download path, please define a library name using LIBRARY_NAME in .env: %s", err.Error())
	}
	return nil
}

func (c *Emby) RefreshLibrary() error {
	reqParam := fmt.Sprintf("/emby/Items/%s/Refresh?Recursive=True&MetadataRefreshMode=FullRefresh", c.LibraryID)

	if _, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers); err != nil {
		return err
	}
	return nil
}

func (c *Emby) SearchSongs(tracks []*models.Track) error {
	for _, track := range tracks {
		reqParam := fmt.Sprintf("/Items?IncludeMediaTypes=Audio&SearchTerm=%s&Recursive=true&Fields=Path", url.QueryEscape(track.CleanTitle))

		body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers)
		if err != nil {
			return err
		}

		var results EmbyItemSearch
		if err = util.ParseResp(body, &results); err != nil {
			return err
		}

		for _, item := range results.Items {
			if strings.EqualFold(track.MainArtist, item.AlbumArtist) && (strings.EqualFold(item.Name, track.CleanTitle) || strings.Contains(strings.ToLower(item.Path), strings.ToLower(track.File))) {
				track.ID = item.ID
				track.Present = true
				break
			}

			if len(item.Artists) > 0 &&
				strings.Contains(strings.ToLower(item.Artists[0]), strings.ToLower(track.MainArtist)) &&
				strings.Contains(strings.ToLower(item.Path), strings.ToLower(track.File)) {
				track.ID = item.ID
				track.Present = true
				break
			}
		}

		if !track.Present {
			debug.Debug(fmt.Sprintf("[emby] failed to find '%s' by '%s' in album '%s'", track.Title, track.Artist, track.Album))
		}
	}
	return nil
}

func (c *Emby) SearchPlaylist() error {
	params := fmt.Sprintf("/emby/Items?SearchTerm=%s&Recursive=true&IncludeItemTypes=Playlist", c.Cfg.PlaylistName)

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}

	var results EmbyItemSearch
	if err = util.ParseResp(body, &results); err != nil {
		return err
	}

	if len(results.Items) != 0 {
		c.Cfg.PlaylistID = results.Items[0].ID
		return nil
	} else {
		return fmt.Errorf("no results found for %s", c.Cfg.PlaylistName)
	}
}

func (c *Emby) CreatePlaylist(tracks []*models.Track) error {
	songIDs := formatEmbySongs(tracks)

	reqParam := fmt.Sprintf("/emby/Playlists?Name=%s&Ids=%s&MediaType=Music", c.Cfg.PlaylistName, songIDs)


	body, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}
	var playlist EmbyPlaylist
	if err = util.ParseResp(body, &playlist); err != nil {
		return err
	}
	c.Cfg.PlaylistID = playlist.ID
	return nil
}

func (c *Emby) UpdatePlaylist(overview string) error {
	time.Sleep(5 * time.Second) // small buffer between playlist creation and updating, Emby doesn't update playlist otherwise
	reqParam := fmt.Sprintf("/emby/Items/%s", c.Cfg.PlaylistID)

	payload := fmt.Appendf(nil, `
		{
		"Id": "%s",
		"Name": "%s",
		"Overview": "%s",
		"ProviderIds": {}
		}`, c.Cfg.PlaylistID, c.Cfg.PlaylistName, overview) // the additional field has to be added, otherwise Emby returns code 500

	if _, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParam, bytes.NewBuffer(payload), c.Cfg.Creds.Headers); err != nil {
		return err
	}
	return nil
}

func (c *Emby) DeletePlaylist() error { // Doesn't currently work due to a bug in Emby
	/* reqParam := fmt.Sprintf("/emby/Items/Delete?Ids=%s", c.Cfg.PlaylistID)

	if _, err := util.MakeRequest("POST", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers); err != nil {
		return err
	} */
	return nil
}

func formatEmbySongs(tracks []*models.Track) string {
	songIDs := make([]string, 0, len(tracks))
	for _, track := range tracks {
		if track.Present {
			songIDs = append(songIDs,track.ID)
		}
	}
	songs := strings.Join(songIDs, ",")

	return songs
}