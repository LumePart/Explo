package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"log/slog"

	"explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

type Paths []struct {
	Name           string   `json:"Name"`
	Locations      []string `json:"Locations"`
	CollectionType string   `json:"CollectionType"`
	ItemID         string   `json:"ItemId"`
	RefreshStatus  string   `json:"RefreshStatus"`
}

type Search struct {
	SearchHints      []SearchHints `json:"SearchHints"`
	TotalRecordCount int           `json:"TotalRecordCount"`
}
type SearchHints struct {
	ItemID      string `json:"ItemId"`
	ID          string `json:"Id"`
	Name        string `json:"Name"`
	Album       string `json:"Album"`
	AlbumID     string `json:"AlbumId"`
	AlbumArtist string `json:"AlbumArtist"`
}

type Audios struct {
	Items            []Items `json:"Items"`
	TotalRecordCount int     `json:"TotalRecordCount"`
	StartIndex       int     `json:"StartIndex"`
}

type Items struct {
	Name        string   `json:"Name"`
	ServerID    string   `json:"ServerId"`
	ID          string   `json:"Id"`
	Path        string   `json:"Path"`
	Album       string   `json:"Album,omitempty"`
	AlbumArtist string   `json:"AlbumArtist,omitempty"`
	Artists     []string `json:"Artists"`

}

type JFPlaylist struct {
	ID string `json:"Id"`
}

type JFUser struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}

type Jellyfin struct {
	LibraryID  string
	HttpClient *util.HttpClient
	Cfg        config.ClientConfig
}

func NewJellyfin(cfg config.ClientConfig, httpClient *util.HttpClient) *Jellyfin {
	return &Jellyfin{Cfg: cfg,
		HttpClient: httpClient}
}

func (c *Jellyfin) AddHeader() error {
	if c.Cfg.Creds.Headers == nil {
		c.Cfg.Creds.Headers = make(map[string]string)
	}

	if c.Cfg.Creds.APIKey != "" {
		c.Cfg.Creds.Headers["Authorization"] = fmt.Sprintf("MediaBrowser Token=%s, Client=%s", c.Cfg.Creds.APIKey, c.Cfg.ClientID)
		return nil
	}
	return fmt.Errorf("API_KEY not set")
}

func (c *Jellyfin) GetAuth() error {
	return nil
}

func (c *Jellyfin) GetLibrary() error {
	reqParam := "/Library/VirtualFolders"

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}

	var paths Paths
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

func (c *Jellyfin) AddLibrary() error {
	cleanPath := url.PathEscape(c.Cfg.DownloadDir)
	reqParam := fmt.Sprintf("/Library/VirtualFolders?name=%s&paths=%s&collectionType=music&refreshLibrary=true", c.Cfg.LibraryName, cleanPath)
	payload := []byte(`{
		"LibraryOptions": {
		  "Enabled": true,
		  "EnableRealtimeMonitor": true,
		  "EnableLUFSScan": false
		}
	  }`)

	if _, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParam, bytes.NewReader(payload), c.Cfg.Creds.Headers); err != nil {
		return fmt.Errorf("failed to add library to Jellyfin using the download path, please define a library name using LIBRARY_NAME in .env: %s", err.Error())
	}
	return nil
}

func (c *Jellyfin) RefreshLibrary() error {
	reqParam := fmt.Sprintf("/Items/%s/Refresh?metadataRefreshMode=FullRefresh&Recursive=true", c.LibraryID)

	if _, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers); err != nil {
		return err
	}
	return nil
}

func (c *Jellyfin) CheckRefreshState() bool {
	return false
}

func (c *Jellyfin) SearchSongs(tracks []*models.Track) error {
	for _, track := range tracks {
		reqParam := fmt.Sprintf("/Items?IncludeMediaTypes=Audio&SearchTerm=%s&Recursive=true&Fields=Path", url.QueryEscape(track.CleanTitle))

		body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers)
		if err != nil {
			return err
		}

		var results Audios
		if err = util.ParseResp(body, &results); err != nil {
			return err
		}

		for _, item := range results.Items {
			if strings.EqualFold(track.MainArtist, item.AlbumArtist) && (util.NormalizeTitle(item.Name) == util.NormalizeTitle(track.Title) || (track.File != "" && strings.Contains(strings.ToLower(item.Path), strings.ToLower(track.File)))) {
				track.ID = item.ID
				track.Present = true
				break
			}

			if track.File != "" && len(item.Artists) > 0 &&
				strings.Contains(strings.ToLower(item.Artists[0]), strings.ToLower(track.MainArtist)) &&
				strings.Contains(strings.ToLower(item.Path), strings.ToLower(track.File)) {
				track.ID = item.ID
				track.Present = true
				break
			}
		}

		if !track.Present {
			slog.Debug(fmt.Sprintf("[jellyfin] failed to find '%s' by '%s' in album '%s'", track.Title, track.Artist, track.Album))
		}
	}
	return nil
}

func (c *Jellyfin) SearchPlaylist() error {
	queryParams := fmt.Sprintf("/Search/Hints?IncludeItemTypes=Playlist&SearchTerm=%s", c.Cfg.PlaylistName)
	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+queryParams, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}

	var results Search
	if err = util.ParseResp(body, &results); err != nil {
		return err
	}

	if len(results.SearchHints) != 0 {
		c.Cfg.PlaylistID = results.SearchHints[0].ID
		return nil
	} else {
		return fmt.Errorf("no results found for playlist: %s", c.Cfg.PlaylistName)
	}
}

func (c *Jellyfin) CreatePlaylist(tracks []*models.Track) error {

	songs, err := formatJFSongs(tracks)
	if err != nil {
		return fmt.Errorf("failed to marshal track IDs: %s", err.Error())
	}
	userID, err := c.ResolveUserID()
	if err != nil {
		return err
	}
	isPublic := c.Cfg.Subsonic.PublicPlaylist

	queryParams := "/Playlists"
	payload := fmt.Appendf(nil, `
		{
		"Name": "%s",
		"Ids": %s,
		"MediaType": "Audio",
		"UserId": "%s",
		"IsPublic": %t
		}`, c.Cfg.PlaylistName, songs, userID, isPublic)

	body, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+queryParams, bytes.NewReader(payload), c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}
	var playlist JFPlaylist
	if err = util.ParseResp(body, &playlist); err != nil {
		return err
	}
	c.Cfg.PlaylistID = playlist.ID
	return nil
}

func (c *Jellyfin) UpdatePlaylist() error {
	isPublic := c.Cfg.Subsonic.PublicPlaylist
	queryParams := fmt.Sprintf("/Items/%s", c.Cfg.PlaylistID)
	payload := fmt.Appendf(nil, `
		{
		"Id":"%s",
		"Name":"%s",
		"Overview":"%s",
		"IsPublic":%t,
		"Genres":[],
		"Tags":[],
		"ProviderIds":{}
		}`, c.Cfg.PlaylistID, c.Cfg.PlaylistName, c.Cfg.PlaylistDescr, isPublic) // the additional fields have to be added, otherwise JF returns code 400

	if _, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+queryParams, bytes.NewBuffer(payload), c.Cfg.Creds.Headers); err != nil {
		return err
	}
	return nil
}

func (c *Jellyfin) DeletePlaylist() error {
	queryParams := fmt.Sprintf("/Items/%s", c.Cfg.PlaylistID)

	if _, err := c.HttpClient.MakeRequest("DELETE", c.Cfg.URL+queryParams, nil, c.Cfg.Creds.Headers); err != nil {
		return fmt.Errorf("deleyeJfPlaylist(): %s", err.Error())
	}
	return nil
}

func formatJFSongs(tracks []*models.Track) ([]byte, error) { // marshal track IDs
	songIDs := make([]string, 0, len(tracks))
	for _, track := range tracks {
		if track.Present {
			songIDs = append(songIDs, track.ID)
		}
	}
	songs, err := json.Marshal(songIDs)
	if err != nil {
		return nil, err
	}
	return songs, nil
}

func (c *Jellyfin) ResolveUserID() (string, error) {
	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+"/Users", nil, c.Cfg.Creds.Headers)
	if err != nil {
		return "", err
	}
	var users []JFUser
	if err = util.ParseResp(body, &users); err != nil {
		return "", err
	}
	for _, user := range users {
		if strings.EqualFold(user.Name, c.Cfg.Creds.User) {
			return user.ID, nil
		}
	}

	return "", fmt.Errorf("failed to find Jellyfin user %q", c.Cfg.Creds.User)
}
