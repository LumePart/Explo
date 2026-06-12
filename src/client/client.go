package client

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

// uploadPlaylistArtwork POSTs raw image bytes to a music app's artwork endpoint.
// Plex, Jellyfin, and Emby all accept the same format — POST + Content-Type: image/jpeg + raw body.
// The only per-client difference is the URL path, which each caller builds before invoking.
func uploadPlaylistArtwork(hc *util.HttpClient, endpoint, localPath string, headers map[string]string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read artwork: %w", err)
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "image/jpeg")
	req.ContentLength = int64(len(data))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := hc.Client.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Warn("artwork upload: response body close failed", "err", cerr.Error())
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload artwork: status %d, body: %s", resp.StatusCode, string(body))
	}
	return nil
}

// Client manages interactions with the selected music system
type Client struct {
	System string
	Cfg    *config.ClientConfig
	API    APIClient
}

type APIClient interface {
	GetLibrary() error
	GetAuth() error
	AddHeader() error
	AddLibrary() error
	SearchSongs([]*models.Track) error
	RefreshLibrary() error
	CheckRefreshState() bool
	CreatePlaylist([]*models.Track) error
	SearchPlaylist() error
	UpdatePlaylist() error
	DeletePlaylist() error
}

// ArtworkUploader is an optional capability for clients that support setting
// playlist artwork. Use a type assertion: if u, ok := c.API.(client.ArtworkUploader); ok {...}.
type ArtworkUploader interface {
	SetPlaylistArtwork(localPath string) error
}

// NewClient initializes a client and sets up authentication
func NewClient(cfg *config.Config) (*Client, error) {
	c := &Client{
		System: cfg.System,
		Cfg:    &cfg.ClientCfg,
	}
	// Create http client with timeout
	httpClient := util.NewHttp(util.HttpClientConfig{
		Timeout: cfg.ClientCfg.HTTPTimeout,
	})

	switch c.System {

	case "emby":
		c.API = NewEmby(cfg.ClientCfg, httpClient)

	case "jellyfin":
		c.API = NewJellyfin(cfg.ClientCfg, httpClient)

	case "mpd":
		c.API = NewMPD(cfg.ClientCfg)

	case "plex":
		c.API = NewPlex(cfg.ClientCfg, httpClient)

	case "subsonic":
		c.API = NewSubsonic(cfg.ClientCfg, httpClient)

	default:
		return nil, fmt.Errorf("unknown system: %s. Use a supported system (emby, jellyfin, mpd, plex, or subsonic)", c.System)
	}

	if err := c.systemSetup(); err != nil { // Run setup automatically
		return nil, fmt.Errorf("setup failed: %w", err)
	}

	return c, nil
}

// TriggerRefresh Runs a trigger to refresh the users app music library
// Useful for one-shot operations
func TriggerRefresh(cfg *config.Config) error {
	c, err := NewClient(cfg)
	if err != nil {
		return fmt.Errorf("client setup: %w", err)
	}
	return c.API.RefreshLibrary()
}

// systemSetup checks needed credentials and initializes the selected system
func (c *Client) systemSetup() error {
	switch c.System {
	case "subsonic":
		if c.Cfg.Creds.User == "" || c.Cfg.Creds.Password == "" {
			return fmt.Errorf("Subsonic USER and PASSWORD are required")
		}
		return c.API.GetAuth()

	case "jellyfin":
		if c.Cfg.Creds.APIKey == "" && c.Cfg.AdminCreds.APIKey == "" {
			return fmt.Errorf("Jellyfin API_KEY or ADMIN_API_KEY is required")
		}
		if c.Cfg.Creds.User == "" {
			slog.Warn("It is recommended to set SYSTEM_USERNAME for Jellyfin")
		}
		if err := c.API.AddHeader(); err != nil {
			return err
		}
		return c.API.GetLibrary()

	case "mpd":
		if c.Cfg.PlaylistDir == "" {
			return fmt.Errorf("MPD PLAYLIST_DIR is required")
		}
		return nil

	case "plex":
		if (c.Cfg.Creds.User == "" || c.Cfg.Creds.Password == "") && c.Cfg.Creds.APIKey == "" {
			return fmt.Errorf("Plex USER/PASSWORD or API_KEY is required")
		}
		if err := c.API.AddHeader(); err != nil {
			return err
		}

		if c.Cfg.Creds.APIKey == "" {
			if err := c.API.GetAuth(); err != nil {
				return err
			}
		}

		if err := c.API.AddHeader(); err != nil {
			return err
		}
		return c.API.GetLibrary()

	case "emby":
		if c.Cfg.Creds.APIKey == "" {
			return fmt.Errorf("Emby API_KEY is required")
		}
		if err := c.API.AddHeader(); err != nil {
			return err
		}
		return c.API.GetLibrary()

	default:
		return fmt.Errorf("unknown system: %s. Use a supported system (emby, jellyfin, mpd, plex, or subsonic)", c.System)
	}
}

func (c *Client) CheckTracks(tracks []*models.Track) error {
	if err := c.API.SearchSongs(tracks); err != nil {
		return fmt.Errorf("SearchSongs failed: %s", err.Error())
	}
	return nil
}

func (c *Client) CreatePlaylist(tracks []*models.Track) error {
	if c.System == "" {
		return fmt.Errorf("could not get music system")
	}

	if err := c.API.RefreshLibrary(); err != nil {
		return fmt.Errorf("[%s] failed to schedule a library scan: %s", c.System, err.Error())
	}
	slog.Info("Refreshing library...", "system", c.System)
	if !c.API.CheckRefreshState() {
		slog.Debug("could not check library refresh state, either the client doesn't support it or threw an error")
		slog.Debug("falling back on SLEEP env variable")
		time.Sleep(time.Duration(c.Cfg.Sleep) * time.Minute)
	}

	if err := c.API.SearchSongs(tracks); err != nil { // search newly added songs
		slog.Warn("SearchSongs failed", "context", err)
	}
	if err := c.API.CreatePlaylist(tracks); err != nil {
		return fmt.Errorf("[%s] failed to create playlist: %s", c.System, err.Error())
	}

	if err := c.API.UpdatePlaylist(); err != nil {
		return fmt.Errorf("[%s] failed to update playlist: %s", c.System, err.Error())
	}
	return nil
}

func (c *Client) DeletePlaylist() error {
	if err := c.API.SearchPlaylist(); err != nil {
		return fmt.Errorf("SearchPlaylist failed: %v", err)
	}
	if err := c.API.DeletePlaylist(); err != nil {
		return fmt.Errorf("[%s] failed to delete playlist: %s", c.System, err.Error())
	}
	return nil
}
