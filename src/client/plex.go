package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"

	"explo/src/config"
	"explo/src/debug"
	"explo/src/models"
	"explo/src/util"
)

type LoginPayload struct {
	User LoginUser `json:"user"`
}

type LoginUser struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type LoginResponse struct {
	User struct {
		AuthToken string `json:"authToken"`
	} `json:"user"`
}

type Libraries struct {
	MediaContainer struct {
		Size      int    `json:"size"`
		AllowSync bool   `json:"allowSync"`
		Title1    string `json:"title1"`
		Library []struct {
			Title 			 string `json:"title"`
			Key              string `json:"key"`
			Location         []struct {
				ID   int    `json:"id"`
				Path string `json:"path"`
			} `json:"Location"`
		} `json:"Directory"`
	} `json:"MediaContainer"`
}

type PlexSearch struct {
	MediaContainer struct {
		Size         int `json:"size"`
		SearchResult []struct {
			Score    float64 `json:"score"`
			Metadata struct {
				LibrarySectionTitle  string `json:"librarySectionTitle"`
				Key                  string `json:"key"`
				Type                 string `json:"type"`
				Title                string `json:"title"` // Track
				GrandparentTitle     string `json:"grandparentTitle"` // Artist
				ParentTitle          string `json:"parentTitle"` // Album
				OriginalTitle        string `json:"originalTitle"`
				Summary              string `json:"summary"`
				Duration             int    `json:"duration"`
				AddedAt              int    `json:"addedAt"`
				UpdatedAt            int    `json:"updatedAt"`
				Media                []struct {
					ID            int    `json:"id"`
					Duration      int    `json:"duration"`
					AudioChannels int    `json:"audioChannels"`
					AudioCodec    string `json:"audioCodec"`
					Container     string `json:"container"`
				} `json:"Media"`
			} `json:"Metadata"`
		} `json:"SearchResult"`
	} `json:"MediaContainer"`
}

	
type PlexServer struct {
	MediaContainer struct {
		Size              int    `json:"size"`
		APIVersion        string `json:"apiVersion"`
		Claimed           bool   `json:"claimed"`
		MachineIdentifier string `json:"machineIdentifier"`
		Version           string `json:"version"`
	} `json:"MediaContainer"`
}

type PlexPlaylist struct {
	MediaContainer struct {
		Size     int `json:"size"`
		Metadata []struct {
			RatingKey    string `json:"ratingKey"`
			Key          string `json:"key"`
			GUID         string `json:"guid"`
			Type         string `json:"type"`
			Title        string `json:"title"`
			Summary      string `json:"summary"`
			Smart        bool   `json:"smart"`
			PlaylistType string `json:"playlistType"`
			AddedAt      int    `json:"addedAt"`
			UpdatedAt    int    `json:"updatedAt"`
			Duration     int    `json:"duration,omitempty"`
		} `json:"Metadata"`
	} `json:"MediaContainer"`
}

type Plex struct {
	machineID string
	LibraryID string
	HttpClient *util.HttpClient
	Cfg config.ClientConfig
}

func NewPlex(cfg config.ClientConfig, httpClient *util.HttpClient) *Plex {
	return &Plex{
		Cfg: cfg,
		HttpClient: httpClient}
}

func (c *Plex) AddHeader() error {
	if c.Cfg.Creds.Headers == nil {
		c.Cfg.Creds.Headers = make(map[string]string)
		c.Cfg.Creds.Headers["X-Plex-Client-Identifier"] = c.Cfg.ClientID
		return nil
	}

	if c.Cfg.Creds.APIKey != "" {
		c.Cfg.Creds.Headers["X-Plex-Token"] = c.Cfg.Creds.APIKey
		return nil
	}
	return fmt.Errorf("couldn't get API key")
}

func (c *Plex) GetAuth() error { // Get user token and server ID from plex
	payload := LoginPayload{
		User: LoginUser{
			Login:    c.Cfg.Creds.User,
			Password: c.Cfg.Creds.Password,
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %s", err.Error())
	}


	body, err := c.HttpClient.MakeRequest("POST", "https://plex.tv/users/sign_in.json", bytes.NewBuffer(payloadBytes), c.Cfg.Creds.Headers)
	if err != nil {
		return fmt.Errorf("%s", err.Error())
	}

	var auth LoginResponse
	err = util.ParseResp(body, &auth)
	if err != nil {
		return fmt.Errorf("%s", err.Error())
	}

	c.Cfg.Creds.APIKey = auth.User.AuthToken

	err = c.getServer()
	if err != nil {
		return fmt.Errorf("%s", err.Error())
	}
	return nil
}

func (c *Plex) GetLibrary() error {
	params := "/library/sections/"

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return fmt.Errorf("failed to make request to plex: %s", err.Error())
	}

	var libraries Libraries
	err = util.ParseResp(body, &libraries)
	if err != nil {
		return fmt.Errorf("failed to parse libraries: %s", err.Error())
	}

	for _, library := range libraries.MediaContainer.Library {
		if c.Cfg.LibraryName == library.Title {
			c.LibraryID = library.Key
			return nil
		}
	}
	if err = c.AddLibrary(); err != nil {
		debug.Debug(err.Error())
		log.Fatalf("library named %s not found and cannot be added, please create it manually and ensure 'Prefer local metadata' is checked", c.Cfg.LibraryName)
	}
	return fmt.Errorf("library '%s' not found", c.Cfg.LibraryName)
}

func (c *Plex) AddLibrary() error {
	params := fmt.Sprintf("/library/sections?name=%s&type=artist&scanner=Plex+Music&agent=tv.plex.agents.music&language=en-US&location=%s&prefs[respectTags]=1", c.Cfg.LibraryName, c.Cfg.DownloadDir)

	body, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}

	var libraries Libraries
	if err = util.ParseResp(body, &libraries); err != nil {
		return err
	}
	c.LibraryID = libraries.MediaContainer.Library[0].Key
	return nil
}

func (c *Plex) RefreshLibrary() error {
	params := fmt.Sprintf("/library/sections/%s/refresh", c.LibraryID)

	if _, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers); err != nil {
		return fmt.Errorf("refreshPlexLibrary(): %s", err.Error())
	}
	return nil
}

func (c *Plex) SearchSongs(tracks []*models.Track) error {
	for _, track := range tracks {
		params := fmt.Sprintf("/library/search?query=%s", url.QueryEscape(track.Title))

		body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
		if err != nil {
			log.Printf("search request failed request for '%s': %s", track.Title, err.Error())
			continue
		}
		
		var searchResults PlexSearch
		if err = util.ParseResp(body, &searchResults); err != nil {
			log.Printf("failed to parse response for '%s': %s", track.Title, err.Error())
			continue
		}
		key, err := getPlexSong(track, searchResults)
		if err != nil {
			debug.Debug(err.Error())
			continue
		}
		if key != "" {
			track.ID = key
			track.Present = true
		}
	}
	return nil
}

func (c *Plex) SearchPlaylist() error {
	params := "/playlists"

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}

	var playlists PlexPlaylist
	if err = util.ParseResp(body, &playlists); err != nil {
		return err
	}

	for _, playlist := range playlists.MediaContainer.Metadata {
		if playlist.Title == c.Cfg.PlaylistName {
			c.Cfg.PlaylistID = playlist.RatingKey
			return nil
		}
	}
	return nil
}


func (c *Plex) CreatePlaylist(tracks []*models.Track) error {
	params := fmt.Sprintf("/playlists?title=%s&type=audio&smart=0&uri=server://%s/com.plexapp.plugins.library/%s", c.Cfg.PlaylistName, c.machineID, c.LibraryID)

	body, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}

	var playlist PlexPlaylist

	if err = util.ParseResp(body, &playlist); err != nil {
		return err
	}

	c.Cfg.PlaylistID = playlist.MediaContainer.Metadata[0].RatingKey

	c.addtoPlaylist(tracks)

	return nil
}

func (c *Plex) UpdatePlaylist(summary string) error {
	params := fmt.Sprintf("/playlists/%s?summary=%s", c.Cfg.PlaylistID, url.QueryEscape(summary))

	if _, err := c.HttpClient.MakeRequest("PUT",c.Cfg.URL+params, nil, c.Cfg.Creds.Headers); err != nil {
		return err
	}
	return nil
}

func (c *Plex) DeletePlaylist() error {
	params := fmt.Sprintf("/playlists/%s", c.Cfg.PlaylistID)

	if _, err := c.HttpClient.MakeRequest("DELETE", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers); err != nil {
		return err
	}
	return nil
}

func (c *Plex) getServer() error {
	params := "/identity"

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return fmt.Errorf("failed to get server ID: %s", err.Error())
	}

	var server PlexServer

	if err = util.ParseResp(body, &server); err != nil {
		return fmt.Errorf("failed to get server ID: %s", err.Error())
	}
	c.machineID = server.MediaContainer.MachineIdentifier
	return nil
}

func getPlexSong(track *models.Track, searchResults PlexSearch) (string, error) { // match track with Plex search result

	for _, result := range searchResults.MediaContainer.SearchResult {
		if result.Metadata.Type == "track" && (result.Metadata.Title == track.Title || result.Metadata.Title  ==  track.CleanTitle) && (result.Metadata.ParentTitle == track.Album || 
		(strings.Contains(result.Metadata.OriginalTitle, track.MainArtist) || strings.Contains(result.Metadata.GrandparentTitle, track.MainArtist)))  {
			return result.Metadata.Key, nil
		}
	}
	debug.Debug(fmt.Sprintf("full search result: %v", searchResults.MediaContainer.SearchResult))
	return "", fmt.Errorf("failed to find '%s' by '%s' in %s album", track.Title, track.Artist, track.Album)
}

func (c *Plex) addtoPlaylist(tracks []*models.Track) {

	for _, track := range tracks {
		if track.ID != "" {
			params := fmt.Sprintf("/playlists/%s/items?uri=server://%s/com.plexapp.plugins.library%s", c.Cfg.PlaylistID, c.machineID, track.ID)

			if _, err := c.HttpClient.MakeRequest("PUT", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers); err != nil {
				log.Printf("failed to add %s to playlist: %s", track.Title, err.Error())
			}
		}
	}
}