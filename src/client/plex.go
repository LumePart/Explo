package client

import (
	"bytes"
	"errors"
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
	Part     []Part `json:"Part"`
	AudioChannels int    `json:"audioChannels"`
	AudioCodec    string `json:"audioCodec"`
	Container     string `json:"container"`
}

type Part     struct {
		ID       int    `json:"id"`
		Key      string `json:"key"`
		Duration int    `json:"duration"`
		File     string `json:"file"`
		Size     int    `json:"size"`
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

type MetadataResponse struct {
	MediaContainer struct {
		Metadata []struct {
			PlexGUID string `json:"guid"`
			GUID []struct {
				ID string `json:"id"`
			} `json:"Guid"`
		} `json:"Metadata"`
	} `json:"MediaContainer"`
}

type Plex struct {
	machineID   string
	LibraryID   string
	HttpClient  *util.HttpClient
	AdminClient *Plex
	Cfg         config.ClientConfig
	// userSwitcher, when set, replaces AdminClient.SwitchUser (test seam).
	userSwitcher func(username string) (*Plex, error)
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

// isOwnerAccount reports whether username identifies the account that owns the
// client's token. The server owner is never in the shared-servers list, so
// SwitchUser has to recognise it separately.
func (c *Plex) isOwnerAccount(username string) bool {
	headers := c.cloneHeaders()
	headers["Accept"] = "application/json"

	body, err := c.HttpClient.MakeRequest("GET", "https://plex.tv/api/v2/user", nil, headers)
	if err != nil {
		return false
	}

	var me struct {
		Username string `json:"username"`
		Title    string `json:"title"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		return false
	}

	for _, id := range []string{me.Username, me.Title, me.Email} {
		if id != "" && strings.EqualFold(id, username) {
			return true
		}
	}
	return false
}

func (c *Plex) SwitchUser(username string) (*Plex, error) {
	user, err := c.findSharedUser(username)
	if err != nil {
		if c.isOwnerAccount(username) {
			ownerClient := *c
			return &ownerClient, nil
		}
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
	c.AdminClient.machineID = c.machineID
	return c.AdminClient.SwitchUser(c.Cfg.Creds.User)
}

// playlistHeaders returns the request headers of the account that owns the
// playlist. Playlists are created under the switched user, so finding,
// updating, deleting and posting artwork must use that user's token too. A
// failed switch is an error rather than a fallback to the instance token:
// that token may belong to a different person, whose playlists a delete would
// then hit.
func (c *Plex) playlistHeaders() (map[string]string, error) {
	userClient, err := c.ensureUserClient()
	if err != nil {
		return nil, fmt.Errorf("failed to switch user: %w", err)
	}
	return userClient.Cfg.Creds.Headers, nil
}
func (c *Plex) AddHeader() error {
	if c.Cfg.Creds.Headers == nil {
		c.Cfg.Creds.Headers = make(map[string]string)
		c.Cfg.Creds.Headers["X-Plex-Client-Identifier"] = c.Cfg.ClientID

		// Only bail out here when there is no key to install yet. The plex
		// setup path calls AddHeader() again after GetAuth() fills in the
		// token, but the admin client built in GetLibrary() gets a fresh
		// Credentials{APIKey: ...} with a nil Headers map and is only called
		// once -- returning early there left it with no X-Plex-Token, so
		// every admin request 401'd.
		if c.Cfg.Creds.APIKey == "" {
			return nil
		}
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
	if (c.Cfg.AdminCreds.User != "" && c.Cfg.AdminCreds.Password != "")  {
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
	} else if (c.Cfg.AdminCreds.APIKey != "") {
		adminCfg := c.Cfg
		adminCfg.Creds = config.Credentials{
			APIKey: c.Cfg.AdminCreds.APIKey,
		}

		c.AdminClient = NewPlex(adminCfg, c.HttpClient)
		if err := c.AdminClient.AddHeader(); err != nil {
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
	key, err := firstLibraryKey(libraries)
	if err != nil {
		return err
	}
	c.LibraryID = key
	return nil
}

// firstLibraryKey returns the created library's key. Plex can answer a
// library creation with an empty container (a hiccup, a race with another
// client); indexing [0] without a check panicked the run.
func firstLibraryKey(libraries Libraries) (string, error) {
	if len(libraries.MediaContainer.Library) == 0 {
		return "", fmt.Errorf("plex returned no library after creating one; check the Plex server and the configured library name and download directory")
	}
	return libraries.MediaContainer.Library[0].Key, nil
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
		searchQuery := util.CleanSearchTitle(track.CleanTitle)
		params := fmt.Sprintf(
			"/hubs/search?query=%s&limit=20", url.QueryEscape(searchQuery),
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
			slog.Warn("search request failed", "searchQuery", searchQuery, "err", err)
			continue
		}

		var hubResults PlexHubSearch
		if err := util.ParseResp(body, &hubResults); err != nil {
			slog.Warn("failed to parse hub response", "searchQuery", searchQuery, "err", err)
			continue
		}

		var all []SongMetadata

		for _, hub := range hubResults.MediaContainer.Hub {
			if hub.Type == "track" {
				all = append(all, hub.Metadata...)
			}
		}
		if len(all) == 0 {
			slog.Debug("no results returned for query", "query", searchQuery, "trackArtist", track.MainArtist, "trackAlbum", track.Album)
			continue
		}

		trackMatch, err := c.getPlexSong(track, all)
		if err != nil {
			slog.Debug("failed to find match, no results returned above threshold score", "threshold", c.Cfg.MatchScore, "searchQuery", searchQuery)
			continue
		}
		track.ID = trackMatch.ID
		track.Present = true
		slog.Debug("matched track", "searchQuery", searchQuery, "matchScore", trackMatch.Score, "trackArtist", trackMatch.Artist, "trackAlbum", trackMatch.Album)
	}

	return nil
}
func (c *Plex) SearchPlaylist() error {
	params := "/playlists"

	headers, err := c.playlistHeaders()
	if err != nil {
		return err
	}

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, headers)
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

	id, err := c.createPlaylistAs(userClient, userClient.Cfg.PlaylistName, tracks)
	if err != nil {
		return err
	}
	c.Cfg.PlaylistID = id

	return nil
}

// createPlaylistAs creates a playlist titled title as userClient, adds tracks
// with userClient's token, and returns the new rating key.
func (c *Plex) createPlaylistAs(userClient *Plex, title string, tracks []*models.Track) (string, error) {
	if _, err := c.postPlaylistAs(userClient, title); err != nil {
		return "", err
	}

	userClient.addtoPlaylist(tracks)

	return userClient.Cfg.PlaylistID, nil
}

// postPlaylistAs creates an empty playlist titled title as userClient, records
// its rating key in userClient.Cfg.PlaylistID and returns it.
func (c *Plex) postPlaylistAs(userClient *Plex, title string) (string, error) {
	metadataURI := fmt.Sprintf(
		"server://%s/com.plexapp.plugins.library/%s",
		userClient.machineID,
		c.LibraryID,
	)

	params := fmt.Sprintf(
		"/playlists?title=%s&type=audio&smart=0&uri=%s",
		url.QueryEscape(title),
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
		return "", fmt.Errorf("playlist create failed: %w", err)
	}

	var playlist PlexPlaylist
	if err := util.ParseResp(body, &playlist); err != nil {
		return "", fmt.Errorf("failed parsing playlist response: %w", err)
	}

	if len(playlist.MediaContainer.Metadata) == 0 {
		return "", fmt.Errorf("playlist created but no metadata returned")
	}

	userClient.Cfg.PlaylistID = playlist.MediaContainer.Metadata[0].RatingKey

	return userClient.Cfg.PlaylistID, nil
}

func (c *Plex) UpdatePlaylist() error {
	params := fmt.Sprintf("/playlists/%s?summary=%s", c.Cfg.PlaylistID, url.QueryEscape(c.Cfg.PlaylistDescr))

	headers, err := c.playlistHeaders()
	if err != nil {
		return err
	}

	if _, err := c.HttpClient.MakeRequest("PUT", c.Cfg.URL+params, nil, headers); err != nil {
		return err
	}
	return nil
}

func (c *Plex) DeletePlaylist() error {
	// Without an ID this would DELETE the /playlists collection itself. Plex
	// answers 403, but a delete aimed at every playlist is not a request worth
	// making and finding nothing to delete is not a failure.
	if c.Cfg.PlaylistID == "" {
		slog.Debug("no plex playlist to delete", "playlist", c.Cfg.PlaylistName)
		return nil
	}

	params := fmt.Sprintf("/playlists/%s", c.Cfg.PlaylistID)

	headers, err := c.playlistHeaders()
	if err != nil {
		return err
	}

	if _, err := c.HttpClient.MakeRequest("DELETE", c.Cfg.URL+params, nil, headers); err != nil {
		return err
	}
	return nil
}

// SetPlaylistArtwork uploads an image as the playlist's poster.
func (c *Plex) SetPlaylistArtwork(localPath string) error {
	if c.Cfg.PlaylistID == "" {
		return fmt.Errorf("plex: no PlaylistID set")
	}
	headers, err := c.playlistHeaders()
	if err != nil {
		return err
	}
	return uploadPlaylistArtwork(c.HttpClient, c.Cfg.URL+"/library/metadata/"+c.Cfg.PlaylistID+"/posters", localPath, headers)
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

func (c *Plex) getPlexSong(track *models.Track, metadata []SongMetadata) (SearchResult, error) {
	searchResults := make([]SearchResult, 0, len(metadata))
	for _, md := range metadata {
		if md.Type != "track" {
			continue
		}
		var mbid string
        if c.AdminClient != nil {
            mbid = c.AdminClient.getPlexMBID(md.RatingKey)
        } else {
            mbid = c.getPlexMBID(md.RatingKey)
        }
		var media Media
		if len(md.Media) > 0 {
			media = md.Media[0]
		}
		if len(media.Part) == 0 {
			media.Part = []Part{{File: "", Duration: 0}}
		}
		searchResults = append(searchResults, SearchResult{
			ID: md.Key,
			Title: md.Title,
			Album: md.ParentTitle,
			Artist: md.OriginalTitle,
			Artists: []string{md.GrandparentTitle},
			Path: media.Part[0].File,
			Duration: (media.Part[0].Duration / 1000),
			MBID: mbid,
		})
	}

	trackMatch, ok := BestMatch(track, searchResults, c.Cfg.MatchScore)

	if !ok {
		slog.Debug("full search result", "results", metadata)
		slog.Debug("failed to find match, no results returned above threshold score", "threshold", c.Cfg.MatchScore)
		return SearchResult{}, fmt.Errorf("failed to find match, no results returned above threshold score, threshold %d", c.Cfg.MatchScore)
	}
	return trackMatch, nil
}

func (c *Plex) getPlexMBID(ratingKey string) string {
	params := fmt.Sprintf("/library/metadata/%s", ratingKey)


	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return ""
	}
	var meta MetadataResponse
	err = json.Unmarshal(body, &meta)
	if err != nil {
		return ""
	}
	prefix := "mbid://"
	for _, metadata := range meta.MediaContainer.Metadata {
		for _, guid := range metadata.GUID {
			if strings.HasPrefix(guid.ID, prefix) {
				mbid := strings.TrimPrefix(guid.ID, prefix)
				return mbid
			}
	}
}
	return ""
}

func (c *Plex) addtoPlaylist(tracks []*models.Track) {
	c.addTracksCounted(tracks)
}

// addTracksCounted adds tracks to c.Cfg.PlaylistID, logging failures, and
// returns how many were added.
func (c *Plex) addTracksCounted(tracks []*models.Track) int {
	added := 0
	for _, track := range tracks {
		if track.ID != "" {
			params := fmt.Sprintf("/playlists/%s/items?uri=server://%s/com.plexapp.plugins.library%s", c.Cfg.PlaylistID, c.machineID, track.ID)

			if _, err := c.HttpClient.MakeRequest("PUT", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers); err != nil {
				slog.Warn("failed to add to playlist", "title", track.Title, "err", err)
				continue
			}
			added++
		}
	}
	return added
}

// switchTo returns a client scoped to username's Plex account.
func (c *Plex) switchTo(username string) (*Plex, error) {
	if c.userSwitcher != nil {
		return c.userSwitcher(username)
	}
	c.AdminClient.machineID = c.machineID
	return c.AdminClient.SwitchUser(username)
}

// SharePlaylist puts a copy of the playlist, titled after its owner, into each
// user's account, replacing the user's previous copy (exact title match
// only), which is deleted only after the new copy has at least one track. Every request
// for a user is sent with that user's own token. A failure for one user is
// logged and returned but does not stop the others.
func (c *Plex) SharePlaylist(tracks []*models.Track, owner string, users []string) error {
	if strings.TrimSpace(owner) == "" {
		// Without an owner the copy title is the bare playlist name, which is
		// the recipient's own Explo playlist; replacing it would delete it.
		slog.Warn("playlist sharing needs PLAYLIST_OWNER_NAME; skipping")
		return nil
	}
	if c.AdminClient == nil && c.userSwitcher == nil {
		slog.Warn("plex playlist sharing needs an admin client; skipping")
		return nil
	}

	var present []*models.Track
	for _, t := range tracks {
		if t.ID != "" {
			present = append(present, t)
		}
	}
	if len(present) == 0 {
		slog.Info("no matched tracks to share")
		return nil
	}

	title := ShareTitle(owner, c.Cfg.PlaylistName)
	var errs []error
	for _, user := range users {
		if err := c.shareWith(user, title, present); err != nil {
			slog.Warn("failed to share playlist", "user", user, "err", err)
			errs = append(errs, fmt.Errorf("share with %s: %w", user, err))
		}
	}
	return errors.Join(errs...)
}

func (c *Plex) shareWith(user, title string, tracks []*models.Track) error {
	recipient, err := c.switchTo(user)
	if err != nil {
		return fmt.Errorf("switch user: %w", err)
	}
	headers := recipient.cloneHeaders()

	var oldKey string
	body, err := recipient.HttpClient.MakeRequest("GET", recipient.Cfg.URL+"/playlists", nil, headers)
	if err != nil {
		return fmt.Errorf("list playlists: %w", err)
	}
	var playlists PlexPlaylist
	if err := util.ParseResp(body, &playlists); err != nil {
		return fmt.Errorf("parse playlists: %w", err)
	}
	for _, p := range playlists.MediaContainer.Metadata {
		if p.Title == title {
			oldKey = p.RatingKey
			break
		}
	}

	newKey, err := c.postPlaylistAs(recipient, title)
	if err != nil {
		return err
	}
	if newKey == "" {
		return fmt.Errorf("playlist create returned no rating key")
	}
	if recipient.addTracksCounted(tracks) == 0 {
		return fmt.Errorf("no tracks added to new copy %s; old copy kept", newKey)
	}

	summary := fmt.Sprintf("/playlists/%s?summary=%s", newKey, url.QueryEscape(c.Cfg.PlaylistDescr))
	if _, err := recipient.HttpClient.MakeRequest("PUT", recipient.Cfg.URL+summary, nil, headers); err != nil {
		slog.Warn("failed to set shared playlist summary", "user", user, "err", err)
	}

	if oldKey == "" || oldKey == newKey {
		return nil
	}
	if _, err := recipient.HttpClient.MakeRequest("DELETE", recipient.Cfg.URL+"/playlists/"+oldKey, nil, headers); err != nil {
		return fmt.Errorf("delete old copy %s: %w", oldKey, err)
	}
	return nil
}
