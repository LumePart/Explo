package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"explo/src/config"
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
		Library   []struct {
			Title    string `json:"title"`
			Key      string `json:"key"`
			Type     string `json:"type"`
			Location []struct {
				ID   int    `json:"id"`
				Path string `json:"path"`
			} `json:"Location"`
		} `json:"Directory"`
	} `json:"MediaContainer"`
}

type PlexGuid struct {
	ID string `json:"id"`
}

// PlexTrackMetadata describes a track entity returned by Plex.
// Used by both /library/search results and /library/sections/{id}/all listings.
type PlexTrackMetadata struct {
	LibrarySectionTitle  string     `json:"librarySectionTitle"`
	RatingKey            string     `json:"ratingKey"`
	Key                  string     `json:"key"`
	Type                 string     `json:"type"`
	Title                string     `json:"title"`            // Track
	GrandparentTitle     string     `json:"grandparentTitle"` // Artist
	GrandparentRatingKey string     `json:"grandparentRatingKey"`
	GrandparentGUID      string     `json:"grandparentGuid"`
	ParentTitle          string     `json:"parentTitle"` // Album
	ParentRatingKey      string     `json:"parentRatingKey"`
	ParentGUID           string     `json:"parentGuid"`
	OriginalTitle        string     `json:"originalTitle"`
	Summary              string     `json:"summary"`
	Duration             int        `json:"duration"`
	UserRating           float64    `json:"userRating"`
	AddedAt              int        `json:"addedAt"`
	UpdatedAt            int        `json:"updatedAt"`
	LastRatedAt          int        `json:"lastRatedAt"`
	GUID                 string     `json:"guid"`
	Guid                 []PlexGuid `json:"Guid"`
	Media                []struct {
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
	} `json:"Media"`
}

// PlexLibraryItems is the response shape for /library/sections/{id}/all.
type PlexLibraryItems struct {
	MediaContainer struct {
		Size     int                 `json:"size"`
		Metadata []PlexTrackMetadata `json:"Metadata"`
	} `json:"MediaContainer"`
}

// PlexMetadataResponse is the response shape for /library/metadata/{ratingKey}.
type PlexMetadataResponse struct {
	MediaContainer struct {
		Size     int `json:"size"`
		Metadata []struct {
			RatingKey string     `json:"ratingKey"`
			Key       string     `json:"key"`
			Type      string     `json:"type"`
			Title     string     `json:"title"`
			GUID      string     `json:"guid"`
			Guid      []PlexGuid `json:"Guid"`
		} `json:"Metadata"`
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
	machineID       string
	LibraryID       string
	musicSectionIDs []string // all music-type library sections, for cross-library track search
	HttpClient      *util.HttpClient
	Cfg             config.ClientConfig
}

func NewPlex(cfg config.ClientConfig, httpClient *util.HttpClient) *Plex {
	return &Plex{
		Cfg:        cfg,
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
		if err := c.getServer(); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("couldn't get API key")
}

func (c *Plex) GetAuth() error { // Get user token from plex
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
		if library.Type == "artist" {
			c.musicSectionIDs = append(c.musicSectionIDs, library.Key)
		}
		if c.Cfg.LibraryName == library.Title {
			c.LibraryID = library.Key
		}
	}
	if c.LibraryID != "" {
		return nil
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
		key, err := c.findTrackAcrossSections(track)
		if err != nil {
			slog.Debug(err.Error())
			continue
		}
		if key != "" {
			track.ID = key
			track.Present = true
		}
	}
	return nil
}

// findTrackAcrossSections searches every music library section for the given track,
// returning the Plex key of the first match. Using per-section /all?type=10 avoids
// the global search endpoint, which ignores the type filter and floods results with
// TV/movie episodes that happen to share the track title.
func (c *Plex) findTrackAcrossSections(track *models.Track) (string, error) {
	for _, sectionID := range c.musicSectionIDs {
		params := fmt.Sprintf("/library/sections/%s/all?type=10&title=%s", sectionID, url.QueryEscape(track.CleanTitle))
		body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
		if err != nil {
			slog.Warn("search request failed", "section", sectionID, "track", track.Title, "err", err.Error())
			continue
		}
		var items PlexLibraryItems
		if err = util.ParseResp(body, &items); err != nil {
			slog.Warn("failed to parse search response", "section", sectionID, "track", track.Title, "err", err.Error())
			continue
		}
		if key := getPlexSong(track, items.MediaContainer.Metadata); key != "" {
			return key, nil
		}
	}
	return "", fmt.Errorf("failed to find '%s' by '%s' in '%s'", track.Title, track.Artist, track.Album)
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

func getPlexSong(track *models.Track, candidates []PlexTrackMetadata) string {
	loweredArtist := strings.ToLower(track.MainArtist)

	for _, md := range candidates {
		titleMatch := strings.EqualFold(md.Title, track.Title) || strings.EqualFold(md.Title, track.CleanTitle)
		albumMatch := strings.EqualFold(md.ParentTitle, track.Album)
		artistMatch := strings.Contains(strings.ToLower(md.OriginalTitle), loweredArtist) || strings.Contains(strings.ToLower(md.GrandparentTitle), loweredArtist)

		if titleMatch && (albumMatch || artistMatch) {
			slog.Debug(fmt.Sprintf("matched track via metadata: %s by %s", track.Title, track.Artist))
			return md.Key
		}

		if track.File == "" || len(md.Media) == 0 || len(md.Media[0].Part) == 0 {
			continue
		}

		media := md.Media[0]
		pathMatch := strings.Contains(strings.ToLower(media.Part[0].File), strings.ToLower(track.File))
		durationMatch := util.Abs(media.Duration-track.Duration) < 10000 // duration within 10s

		if durationMatch && pathMatch {
			slog.Debug(fmt.Sprintf("matched track via path: %s by %s", track.Title, track.Artist))
			return md.Key
		}
	}

	return ""
}

// GetRatedTracks returns all tracks in the configured library that have a userRating > 0.
// Plex's filter operator syntax is finicky across versions, so this fetches all tracks
// in the library section and filters in Go. The Explo library is small with persist off (typically <200 tracks)
// so the cost is trivial.
func (c *Plex) GetRatedTracks() ([]PlexTrackMetadata, error) {
	if c.LibraryID == "" {
		return nil, fmt.Errorf("library ID not set; call GetLibrary first")
	}
	params := fmt.Sprintf("/library/sections/%s/all?type=10&includeGuids=1", c.LibraryID)

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch library tracks: %s", err.Error())
	}

	var items PlexLibraryItems
	if err = util.ParseResp(body, &items); err != nil {
		return nil, fmt.Errorf("failed to parse library tracks: %s", err.Error())
	}

	rated := make([]PlexTrackMetadata, 0)
	for _, t := range items.MediaContainer.Metadata {
		if t.UserRating > 0 {
			rated = append(rated, t)
		}
	}
	return rated, nil
}

// GetArtistMetadata fetches a single metadata entry by ratingKey, used to resolve
// the artist-level MBID (Plex track Guid[] only contains the recording MBID).
func (c *Plex) GetArtistMetadata(ratingKey string) (*PlexMetadataResponse, error) {
	params := fmt.Sprintf("/library/metadata/%s?includeGuids=1", url.PathEscape(ratingKey))

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metadata for %s: %s", ratingKey, err.Error())
	}

	var resp PlexMetadataResponse
	if err = util.ParseResp(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse metadata for %s: %s", ratingKey, err.Error())
	}
	return &resp, nil
}

func (c *Plex) addtoPlaylist(tracks []*models.Track) {
	for _, track := range tracks {
		if track.ID != "" {
			params := fmt.Sprintf("/playlists/%s/items?uri=server://%s/com.plexapp.plugins.library%s", c.Cfg.PlaylistID, c.machineID, track.ID)

			if _, err := c.HttpClient.MakeRequest("PUT", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers); err != nil {
				slog.Warn("failed to add %s to playlist: %s", track.Title, err.Error())
			}
		}
	}
}
