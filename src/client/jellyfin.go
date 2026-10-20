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

type ProviderIds struct {
	MusicBrainzTrack     string `json:"MusicBrainzTrack"`
	MusicBrainzRecording string `json:"MusicBrainzRecording"`
}

type Items struct {
	Name         string      `json:"Name"`
	ServerID     string      `json:"ServerId"`
	ID           string      `json:"Id"`
	ProviderIds  ProviderIds `json:"ProviderIds"`
	Path         string      `json:"Path"`
	RunTimeTicks int         `json:"RunTimeTicks"`
	Album        string      `json:"Album,omitempty"`
	AlbumArtist  string      `json:"AlbumArtist,omitempty"`
	Artists      []string    `json:"Artists"`

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

	apiKey := c.resolveAPIKey()

	if apiKey == "" {
		return fmt.Errorf("API_KEY or ADMIN_API_KEY not set")
	}

	c.Cfg.Creds.Headers["Authorization"] = fmt.Sprintf("MediaBrowser Token=%s, Client=%s", apiKey, c.Cfg.ClientID)

	return nil
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
		searchQuery := util.CleanSearchTitle(track.CleanTitle)
		reqParam := fmt.Sprintf("/Items?IncludeMediaTypes=Audio&SearchTerm=%s&Recursive=true&Fields=Path,ProviderIDs", url.QueryEscape(searchQuery))

		body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers)
		if err != nil {
			return err
		}

		var results Audios
		if err = util.ParseResp(body, &results); err != nil {
			return err
		}
		if len(results.Items) == 0 {
			slog.Debug("no results returned for query", "query", searchQuery, "trackArtist", track.MainArtist, "trackAlbum", track.Album)
			continue
		}

		searchData := make([]SearchResult, 0, len(results.Items))
		for _, item := range results.Items {
			mbID := item.ProviderIds.MusicBrainzRecording
			if mbID == "" {
				mbID = item.ProviderIds.MusicBrainzTrack
			}
			searchData = append(searchData, SearchResult{
				ID: item.ID,
				Title: item.Name,
				Album: item.Album,
				Artist: item.AlbumArtist,
				Artists: item.Artists,
				Path: item.Path,
				Duration: (item.RunTimeTicks / 10000000),
				MBID: mbID,
			})
		}
		trackMatch, ok := BestMatch(track, searchData, c.Cfg.MatchScore)
		if !ok {
			slog.Debug("failed to find match, no results returned above threshold score", "threshold", c.Cfg.MatchScore, "searchQuery", searchQuery)
			continue
		}
		track.ID = trackMatch.ID
		track.Present = true
		slog.Debug("matched track", "searchQuery", searchQuery, "matchScore", trackMatch.Score, "trackArtist", trackMatch.Artist, "trackAlbum", trackMatch.Album)
	}
	return nil
}

func (c *Jellyfin) SearchPlaylist() error {
	queryParams := fmt.Sprintf("/Search/Hints?IncludeItemTypes=Playlist&SearchTerm=%s", url.QueryEscape(c.Cfg.PlaylistName))
	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+queryParams, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}

	var results Search
	if err = util.ParseResp(body, &results); err != nil {
		return err
	}

	for _, playlist := range results.SearchHints {
		if playlist.Name == c.Cfg.PlaylistName {
			c.Cfg.PlaylistID = playlist.ID
			return nil
		}
	}
	return fmt.Errorf("no results found for playlist: %s", c.Cfg.PlaylistName)
}

func (c *Jellyfin) CreatePlaylist(tracks []*models.Track) error {

	songs, err := formatJFSongs(tracks)
	if err != nil {
		return fmt.Errorf("failed to marshal track IDs: %s", err.Error())
	}
	var userID string
	isPublic := c.Cfg.PublicPlaylist

	if c.Cfg.Creds.User != "" {
		userID, err = c.ResolveUserID()
		if err != nil {
			return err
		}
	} else {
		userID = c.resolveAPIKey()
		isPublic = true
	}


	queryParams := "/Playlists"
	payload, err := jfCreatePlaylistBody(c.Cfg.PlaylistName, songs, userID, isPublic)
	if err != nil {
		return fmt.Errorf("failed to marshal playlist body: %s", err.Error())
	}

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

// Playlist names and descriptions are user input: quoting a quote,
// backslash or newline into fmt-built JSON produced invalid bodies and
// the server answered 400 (BUG-15). Marshal instead.
func jfCreatePlaylistBody(name string, songs json.RawMessage, userID string, isPublic bool) ([]byte, error) {
	return json.Marshal(struct {
		Name      string          `json:"Name"`
		IDs       json.RawMessage `json:"Ids"`
		MediaType string          `json:"MediaType"`
		UserID    string          `json:"UserId"`
		IsPublic  bool            `json:"IsPublic"`
	}{name, songs, "Audio", userID, isPublic})
}

func jfUpdatePlaylistBody(id, name, overview string, isPublic bool) ([]byte, error) {
	return json.Marshal(struct {
		ID          string            `json:"Id"`
		Name        string            `json:"Name"`
		Overview    string            `json:"Overview"`
		IsPublic    bool              `json:"IsPublic"`
		Genres      []string          `json:"Genres"`
		Tags        []string          `json:"Tags"`
		ProviderIDs map[string]string `json:"ProviderIds"`
	}{id, name, overview, isPublic, []string{}, []string{}, map[string]string{}})
}

func (c *Jellyfin) UpdatePlaylist() error {
	isPublic := c.Cfg.PublicPlaylist
	if c.Cfg.Creds.User == "" {
		isPublic = true
	}
	queryParams := fmt.Sprintf("/Items/%s", c.Cfg.PlaylistID)
	// the additional fields have to be added, otherwise JF returns code 400
	payload, err := jfUpdatePlaylistBody(c.Cfg.PlaylistID, c.Cfg.PlaylistName, c.Cfg.PlaylistDescr, isPublic)
	if err != nil {
		return fmt.Errorf("failed to marshal playlist body: %s", err.Error())
	}

	if _, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+queryParams, bytes.NewBuffer(payload), c.Cfg.Creds.Headers); err != nil {
		return err
	}
	return nil
}

func (c *Jellyfin) DeletePlaylist() error {
	// Without an ID this would DELETE /Items, which is every item rather than
	// one playlist.
	if c.Cfg.PlaylistID == "" {
		slog.Debug("no jellyfin playlist to delete", "playlist", c.Cfg.PlaylistName)
		return nil
	}

	queryParams := fmt.Sprintf("/Items/%s", c.Cfg.PlaylistID)

	if _, err := c.HttpClient.MakeRequest("DELETE", c.Cfg.URL+queryParams, nil, c.Cfg.Creds.Headers); err != nil {
		return fmt.Errorf("deleyeJfPlaylist(): %s", err.Error())
	}
	return nil
}

// SetPlaylistArtwork uploads a JPEG as the playlist's primary image.
func (c *Jellyfin) SetPlaylistArtwork(localPath string) error {
	if c.Cfg.PlaylistID == "" {
		return fmt.Errorf("jellyfin: no PlaylistID set")
	}
	return uploadPlaylistArtwork(c.HttpClient, c.Cfg.URL+"/Items/"+c.Cfg.PlaylistID+"/Images/Primary", localPath, c.Cfg.Creds.Headers)
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

// Check which API Key variable is used
func (c *Jellyfin) resolveAPIKey() string {
	if c.Cfg.AdminCreds.APIKey != "" {
		return c.Cfg.AdminCreds.APIKey
	}
	return c.Cfg.Creds.APIKey
}
