package client

import (
	"fmt"
	"log"
	"time"

	"explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

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
	CreatePlaylist([]*models.Track) error
	SearchPlaylist() error
	UpdatePlaylist(string) error
	DeletePlaylist() error
}

// NewClient initializes a client and sets up authentication
func NewClient(cfg *config.Config, httpClient *util.HttpClient) (*Client, error) {
	c := &Client{
		System: cfg.System,
		Cfg:    &cfg.ClientCfg,
	}
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
		log.Fatalf("unknown system: %s. Use a supported system (emby, jellyfin, mpd, plex, or subsonic).", c.System)
	}

	if err := c.systemSetup(); err != nil { // Run setup automatically
		return nil, fmt.Errorf("setup failed: %w", err)
	}

	return c, nil
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
		if c.Cfg.Creds.APIKey == "" {
			return fmt.Errorf("Jellyfin API_KEY is required")
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

func (c *Client) CheckTracks(tracks []*models.Track) {
	if err := c.API.SearchSongs(tracks); err != nil {
		log.Printf("warning: SearchSongs failed: %v", err)
	}
}

func (c *Client) CreatePlaylist(tracks []*models.Track) error {
	if c.System == "" {
		log.Fatal("could not get music system")
	}

	if err := c.API.RefreshLibrary(); err != nil {
		return fmt.Errorf("[%s] failed to schedule a library scan: %s", c.System, err.Error())
	}

	log.Printf("[%s] Refreshing library...", c.System)
	time.Sleep(time.Duration(c.Cfg.Sleep) * time.Minute)
	if err := c.API.SearchSongs(tracks); err != nil { // search newly added songs
		log.Printf("warning: SearchSongs failed: %v", err)
	}
	if err := c.API.CreatePlaylist(tracks); err != nil {
		return fmt.Errorf("[%s] failed to create playlist: %s", c.System, err.Error())
	}

	description := "Created by Explo using recommendations from ListenBrainz"
	if err := c.API.UpdatePlaylist(description); err != nil {
		return fmt.Errorf("[%s] failed to update playlist: %s", c.System, err.Error())
	}
	return nil
}

func (c *Client) DeletePlaylist() error {
	if err := c.API.SearchPlaylist(); err != nil {
		return fmt.Errorf("warning: SearchSongs failed: %v", err)
	}
	if err := c.API.DeletePlaylist(); err != nil {
		return fmt.Errorf("[%s] failed to delete playlist: %s", c.System, err.Error())
	}
	return nil
}
