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
	Cfg *config.ClientConfig
	API APIClient
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
func NewClient(cfg *config.Config, httpClient *util.HttpClient) *Client {
	c := &Client{
		System: cfg.System,
		Cfg: &cfg.ClientCfg,
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

	c.systemSetup() // Run setup automatically
	return c
}

// systemSetup checks needed credentials and initializes the selected system
func (c *Client) systemSetup() {
	switch c.System {
	case "subsonic":
		if c.Cfg.Creds.User == "" || c.Cfg.Creds.Password == "" {
			log.Fatal("Subsonic USER and PASSWORD are required")
		}
		c.API.GetAuth()

	case "jellyfin":
		if c.Cfg.Creds.APIKey == "" {
			log.Fatal("Jellyfin API_KEY is required")
		}
		c.API.AddHeader()
		c.API.GetLibrary()

	case "mpd":
		if c.Cfg.PlaylistDir == "" {
			log.Fatal("MPD PLAYLIST_DIR is required")
		}

	case "plex":
		if (c.Cfg.Creds.User == "" || c.Cfg.Creds.Password == "") && c.Cfg.Creds.APIKey == "" {
			log.Fatal("Plex USER/PASSWORD or API_KEY is required")
		}
		c.API.AddHeader()
		if c.Cfg.Creds.APIKey == "" {
			if err := c.API.GetAuth(); err != nil {
				log.Fatal(err)
			}

		}
		c.API.AddHeader()
		c.API.GetLibrary()

	case "emby":
		if c.Cfg.Creds.APIKey == "" {
			log.Fatal("Emby API_KEY is required")
		}
		c.API.AddHeader()
		c.API.GetLibrary()

	default:
		log.Fatalf("Unknown system: %s. Use a supported system (emby, jellyfin, mpd, plex, or subsonic).", c.System)
	}
}

func (c *Client) CheckTracks(tracks []*models.Track) {
	c.API.SearchSongs(tracks)
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
	c.API.SearchSongs(tracks) // search newly added songs
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
	c.API.SearchPlaylist()
	err := c.API.DeletePlaylist()
	if err != nil {
		return fmt.Errorf("[%s] failed to delete playlist: %s", c.System, err.Error())
	}
	return nil
}