package client

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

type LoginPayload struct {
	Login                string `json:"login"`
	Password             string `json:"password"`
	PlexClientIdentifier string `json:"X-Plex-Client-Identifier"`
}

type LoginResponse struct {
	AuthToken string `json:"authToken"`
}

type PlexSharedServers struct {
	XMLName       xml.Name         `xml:"MediaContainer"`
	SharedServers []PlexSharedUser `xml:"SharedServer"`
}
type PlexSharedUser struct {
	ID          string `xml:"id,attr"`
	UserID      string `xml:"userID,attr"`
	Username    string `xml:"username,attr"`
	Email       string `xml:"email,attr"`
	Name        string `xml:"name,attr"`
	AccessToken string `xml:"accessToken,attr"`
}

type Libraries struct {
	MediaContainer struct {
		Size      int    `json:"size"`
		AllowSync bool   `json:"allowSync"`
		Title1    string `json:"title1"`
		Library   []struct {
			Title    string `json:"title"`
			Key      string `json:"key"`
			Location []struct {
				ID   int    `json:"id"`
				Path string `json:"path"`
			} `json:"Location"`
		} `json:"Directory"`
	} `json:"MediaContainer"`
}

type PlexHubSearch struct {
	MediaContainer struct {
		Size int             `json:"size"`
		Hub  []SongHubSearch `json:"Hub"`
	} `json:"MediaContainer"`
}

type SongHubSearch struct {
	Type     string         `json:"type"`
	Metadata []SongMetadata `json:"Metadata"`
}
type SongMetadata struct {
	LibrarySectionTitle string  `json:"librarySectionTitle"`
	RatingKey           string  `json:"ratingKey"`
	Key                 string  `json:"key"`
	Type                string  `json:"type"`
	Title               string  `json:"title"`            // Track
	GrandparentTitle    string  `json:"grandparentTitle"` // Artist
	ParentTitle         string  `json:"parentTitle"`      // Album
	OriginalTitle       string  `json:"originalTitle"`
	Summary             string  `json:"summary"`
	Duration            int     `json:"duration"`
	AddedAt             int     `json:"addedAt"`
	UpdatedAt           int     `json:"updatedAt"`
	Media               []Media `json:"Media"`
}
type SongSearch struct {
	Metadata SongMetadata `json:"Metadata"`
}

type Media struct {
	ID       int `json:"id"`
	Duration int `json:"duration"`
	Part     []struct {
		ID       int    `json:"id"`
		Key      string `json:"key"`
		Duration int    `json:"duration"`
		File     string `json:"file"`
		Size     int    `json:"size"`
	} `json:"Part"`
	AudioChannels int    `json:"audioChannels"`
	AudioCodec    string `json:"audioCodec"`
	Container     string `json:"container"`
}

type PlexSearch struct {
	MediaContainer struct {
		Size         int          `json:"size"`
		SearchResult []SongSearch `json:"SearchResult"`
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

type GUID struct {
	ID string `json:"id"`
}
type Metadata struct {
	GUID []GUID `json:"Guid"`
}

type Plex struct {
	machineID   string
	LibraryID   string
	HttpClient  *util.HttpClient
	AdminClient *Plex
	Cfg         config.ClientConfig
}

func NewPlex(cfg config.ClientConfig, httpClient *util.HttpClient) *Plex {
	return &Plex{
		Cfg:        cfg,
		HttpClient: httpClient}
}

func (c *Plex) cloneHeaders() map[string]string {
	h := make(map[string]string, len(c.Cfg.Creds.Headers))
	for k, v := range c.Cfg.Creds.Headers {
		h[k] = v
	}
	return h
}

func (c *Plex) getSharedServers() ([]PlexSharedUser, error) {
	url := fmt.Sprintf(
		"https://plex.tv/api/servers/%s/shared_servers",
		c.machineID,
	)

	body, err := c.HttpClient.MakeRequest("GET", url, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch shared servers: %w", err)
	}

	var resp PlexSharedServers
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse shared_servers XML: %w", err)
	}

	return resp.SharedServers, nil
}
func (c *Plex) findSharedUser(username string) (*PlexSharedUser, error) {
	users, err := c.getSharedServers()
	if err != nil {
		return nil, err
	}

	username = strings.ToLower(username)

	for _, u := range users {

		if strings.ToLower(u.Username) == username {
			return &u, nil
		}

		if strings.ToLower(u.Email) == username {
			return &u, nil
		}

		if strings.ToLower(u.Name) == username {
			return &u, nil
		}

		if u.UserID == username || u.ID == username {
			return &u, nil
		}
	}

	return nil, fmt.Errorf("unable to find shared user: %s", username)
}

func (c *Plex) SwitchUser(username string) (*Plex, error) {
	user, err := c.findSharedUser(username)
	if err != nil {
		return nil, err
	}

	if user.AccessToken == "" {
		return nil, fmt.Errorf("shared user has no access token: %s", username)
	}

	newClient := *c
	newHeaders := newClient.cloneHeaders()
	newHeaders["X-Plex-Token"] = user.AccessToken

	newClient.Cfg.Creds.Headers = newHeaders
	newClient.Cfg.Creds.APIKey = user.AccessToken

	return &newClient, nil
}
func (c *Plex) ensureUserClient() (*Plex, error) {
	// If no admin client, assume already user-scoped
	if c.AdminClient == nil {
		return c, nil
	}

	// Switch using admin client (correct source of truth)
	return c.AdminClient.SwitchUser(c.Cfg.Creds.User)
}
func (c *Plex) AddHeader() error {
	if c.Cfg.Creds.Headers == nil {
		c.Cfg.Creds.Headers = make(map[string]string)
		c.Cfg.Creds.Headers["X-Plex-Client-Identifier"] = c.Cfg.ClientID

		return nil
	}
	if c.Cfg.Creds.APIKey != "" {
		c.Cfg.Creds.Headers["X-Plex-Token"] = c.Cfg.Creds.APIKey
		if err := c.getServer(); err != nil {
			println(err)
			return err
		}
		return nil
	}
	return fmt.Errorf("couldn't get API key")
}

func (c *Plex) GetAuth() error { // Get user token from plex
	payload := LoginPayload{
		Login:    c.Cfg.Creds.User,
		Password: c.Cfg.Creds.Password,
	}
	url := "https://plex.tv/api/v2/users/signin.json"
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %s", err.Error())
	}
	body, err := c.HttpClient.MakeRequest("POST", url, bytes.NewBuffer(payloadBytes), c.Cfg.Creds.Headers)

	if err != nil {
		return fmt.Errorf("%s", err.Error())
	}

	var auth LoginResponse
	err = util.ParseResp(body, &auth)
	if err != nil {
		return fmt.Errorf("%s", err.Error())
	}
	c.Cfg.Creds.APIKey = auth.AuthToken
	c.Cfg.Creds.Headers["X-Plex-Token"] = auth.AuthToken
	return nil
}
func (c *Plex) GetLibrary() error {
	if c.Cfg.AdminCreds.User != "" && c.Cfg.AdminCreds.Password != "" {
		adminCfg := c.Cfg
		adminCfg.Creds = config.Credentials{
			User:     c.Cfg.AdminCreds.User,
			Password: c.Cfg.AdminCreds.Password,
		}

		c.AdminClient = NewPlex(adminCfg, c.HttpClient)
		if err := c.AdminClient.AddHeader(); err != nil {
			return err
		}
		if err := c.AdminClient.GetAuth(); err != nil {
			return err
		}

		err := c.AdminClient.getLibraryRequest()
		if err != nil {
			return err
		}
		c.LibraryID = c.AdminClient.LibraryID

		return err
	}
	return c.getLibraryRequest()
}

func (c *Plex) getLibraryRequest() error {
	params := "/library/sections/all"
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
		slog.Debug(err.Error())
		return fmt.Errorf("library named %s not found and cannot be added, please create it manually and ensure 'Prefer local metadata' is checked", c.Cfg.LibraryName)
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
	if c.AdminClient != nil {
		return c.AdminClient.refreshLibraryRequest()
	}

	return c.refreshLibraryRequest()
}

func (c *Plex) refreshLibraryRequest() error {
	params := fmt.Sprintf("/library/sections/%s/refresh", c.LibraryID)

	if _, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers); err != nil {
		return fmt.Errorf("refreshPlexLibrary(): %s", err.Error())
	}
	return nil
}

func (c *Plex) CheckRefreshState() bool {
	return false
}
func (c *Plex) SearchSongs(tracks []*models.Track) error {
	for _, track := range tracks {
		params := fmt.Sprintf(
			"/hubs/search?query=%s&limit=10",
			url.QueryEscape(util.CleanSearchTitle(track.CleanTitle)),
		)

		var body []byte
		var err error

		if c.AdminClient != nil {
			body, err = c.HttpClient.MakeRequest(
				"GET",
				c.Cfg.URL+params,
				nil,
				c.AdminClient.Cfg.Creds.Headers,
			)
		} else {
			body, err = c.HttpClient.MakeRequest(
				"GET",
				c.Cfg.URL+params,
				nil,
				c.Cfg.Creds.Headers,
			)
		}

		if err != nil {
			slog.Warn("search request failed", "title", track.Title, "err", err)
			continue
		}

		var hubResults PlexHubSearch
		if err := util.ParseResp(body, &hubResults); err != nil {
			slog.Warn("failed to parse hub response", "title", track.Title, "err", err)
			continue
		}

		var matched bool

		var all []SongMetadata

		for _, hub := range hubResults.MediaContainer.Hub {
			if hub.Type == "track" {
				all = append(all, hub.Metadata...)
			}
		}

		key, err := c.getPlexSong(track, all)
		if err != nil {
			slog.Warn("failed to find match", "title", track.Title, "err", err)
			continue
		}
		if key != "" {
			track.ID = key
			track.Present = true
			matched = true
		}
		if !matched {
			slog.Debug("no match found for track", "title", track.Title)
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
	if len(tracks) == 0 {
		return fmt.Errorf("no tracks provided")
	}
	var userClient *Plex
	var err error
	if c.AdminClient != nil {
		c.AdminClient.machineID = c.machineID
		userClient, err = c.ensureUserClient()
		if err != nil {
			return fmt.Errorf("failed to switch user: %w", err)
		}
	} else {
		userClient = c
	}

	metadataURI := fmt.Sprintf(
		"server://%s/com.plexapp.plugins.library/%s",
		userClient.machineID,
		c.LibraryID,
	)

	params := fmt.Sprintf(
		"/playlists?title=%s&type=audio&smart=0&uri=%s",
		url.QueryEscape(userClient.Cfg.PlaylistName),
		url.QueryEscape(metadataURI),
	)

	headers := userClient.cloneHeaders()

	body, err := userClient.HttpClient.MakeRequest(
		"POST",
		userClient.Cfg.URL+params,
		nil,
		headers,
	)
	if err != nil {
		return fmt.Errorf("playlist create failed: %w", err)
	}

	var playlist PlexPlaylist
	if err := util.ParseResp(body, &playlist); err != nil {
		return fmt.Errorf("failed parsing playlist response: %w", err)
	}

	if len(playlist.MediaContainer.Metadata) == 0 {
		return fmt.Errorf("playlist created but no metadata returned")
	}

	userClient.Cfg.PlaylistID = playlist.MediaContainer.Metadata[0].RatingKey

	userClient.addtoPlaylist(tracks)

	c.Cfg.PlaylistID = userClient.Cfg.PlaylistID

	return nil
}
func (c *Plex) UpdatePlaylist() error {
	params := fmt.Sprintf("/playlists/%s?summary=%s", c.Cfg.PlaylistID, url.QueryEscape(c.Cfg.PlaylistDescr))

	if _, err := c.HttpClient.MakeRequest("PUT", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers); err != nil {
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

// SetPlaylistArtwork uploads an image as the playlist's poster.
func (c *Plex) SetPlaylistArtwork(localPath string) error {
	if c.Cfg.PlaylistID == "" {
		return fmt.Errorf("plex: no PlaylistID set")
	}
	return uploadPlaylistArtwork(c.HttpClient, c.Cfg.URL+"/library/metadata/"+c.Cfg.PlaylistID+"/posters", localPath, c.Cfg.Creds.Headers)
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

func (c *Plex) getPlexSong(track *models.Track, metadata []SongMetadata) (string, error) {
	normArtist := util.AlnumOnly(track.MainArtist)
	normalizedTrackTitle := util.NormalizeTitle(track.Title)
	normalizedCleanTitle := util.NormalizeTitle(track.CleanTitle)
	normalizedAlbum := util.AlnumOnly(strings.ToLower(track.Album))

	for _, md := range metadata {
		if md.Type != "track" {
			continue
		}

                var mbid string;
                if c.AdminClient != nil {
                    mbid = c.AdminClient.getPlexMBID(md.RatingKey)
                } else {
                    mbid = c.getPlexMBID(md.RatingKey)
                }

		normalizedSongTitle := util.NormalizeTitle(md.Title)
		musicBrainzMatch := mbid != "" && track.MusicBrainzReleaseTrackID == mbid
		titleMatch := normalizedSongTitle == normalizedTrackTitle || normalizedSongTitle == normalizedCleanTitle
		albumMatch := util.AlnumOnly(strings.ToLower(md.ParentTitle)) == normalizedAlbum
		artistMatch := util.ContainsFold(util.AlnumOnly(md.OriginalTitle), normArtist) || util.ContainsFold(util.AlnumOnly(md.GrandparentTitle), normArtist)

		if musicBrainzMatch || (titleMatch && (albumMatch || artistMatch)) {
			slog.Debug("matched track via metadata", "title", track.Title, "artist", track.Artist)
			return md.Key, nil
		}

		if track.File == "" || len(md.Media) == 0 || len(md.Media[0].Part) == 0 {
			continue
		}

		media := md.Media[0]
		pathMatch := util.ContainsFold(media.Part[0].File, track.File)
		durationMatch := util.Abs(media.Duration-track.Duration) < 10000 // duration within 10s

		if durationMatch && pathMatch {
			slog.Debug("matched track via path", "title", track.Title, "artist", track.Artist)
			return md.Key, nil
		}
	}

	slog.Debug(fmt.Sprintf("full search result: %v", metadata))
	return "", fmt.Errorf("failed to find '%s' by '%s' in '%s'", track.Title, track.Artist, track.Album)
}

func (c *Plex) getPlexMBID(ratingKey string) string {
	params := fmt.Sprintf("/library/metadata/%s", ratingKey)


	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return ""
	}

	var metadata Metadata
	if err = util.ParseResp(body, &metadata); err != nil {
		return ""
	}
	prefix := "mbid://"
	for _, guid := range metadata.GUID {
		if strings.HasPrefix(guid.ID, prefix) {
			return strings.TrimPrefix(guid.ID, prefix)
		}
	}
	return ""
}

func (c *Plex) addtoPlaylist(tracks []*models.Track) {
	for _, track := range tracks {
		if track.ID != "" {
			params := fmt.Sprintf("/playlists/%s/items?uri=server://%s/com.plexapp.plugins.library%s", c.Cfg.PlaylistID, c.machineID, track.ID)

			if _, err := c.HttpClient.MakeRequest("PUT", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers); err != nil {
				slog.Warn("failed to add to playlist", "title", track.Title, "err", err)
			}
		}
	}
}
