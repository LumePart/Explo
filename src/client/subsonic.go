package client

import (
	"fmt"
	"strings"
	"time"

	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"net/url"

	"explo/src/config"
	"explo/src/debug"
	"explo/src/models"
	"explo/src/util"
)

type FailedResp struct {
	SubsonicResponse struct {
		Status        string `json:"status"`
		Error         struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"subsonic-response"`
}

type SubResponse struct {
	SubsonicResponse struct {
		Status        string        `json:"status"`
		Version       string        `json:"version"`
		Type          string        `json:"type"`
		ServerVersion string        `json:"serverVersion"`
		SearchResult3 struct {
			Song []struct {
				ID          string    `json:"id"`
				Title       string    `json:"title"`
				Artist        string    `json:"artist"`
			} `json:"song"`
		} `json:"searchResult3,omitempty"`
		Playlists     struct {
			Playlist []Playlist `json:"playlist,omitempty"`
		} `json:"playlists,omitempty"`
		Playlist      Playlist `json:"playlist,omitempty"`
	} `json:"subsonic-response"`
}

type Playlist struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Comment   string    `json:"comment,omitempty"`
	SongCount int       `json:"songCount"`
	Duration  int       `json:"duration"`
	Public    bool      `json:"public"`
	Owner     string    `json:"owner"`
	Created   time.Time `json:"created"`
	Changed   time.Time `json:"changed"`
	CoverArt  string    `json:"coverArt"`
}

type Subsonic struct {
	Token string
	Salt string
	HttpClient *util.HttpClient
	Cfg config.ClientConfig
}

func NewSubsonic(cfg config.ClientConfig, httpClient *util.HttpClient) *Subsonic {
	return &Subsonic{Cfg: cfg,
		HttpClient: httpClient}
}

func (c *Subsonic) AddHeader() error {
	return nil
}

func (c *Subsonic) GetAuth() error { // Generate salt and token
	var salt = make([]byte, 6)


	_, err := rand.Read(salt)
	if err != nil {
		return fmt.Errorf("failed to read salt: %s", err.Error())
	}

	saltStr := base64.RawURLEncoding.EncodeToString(salt)
	passSalt := fmt.Sprintf("%s%s", c.Cfg.Creds.Password, saltStr)

	token := fmt.Sprintf("%x", md5.Sum([]byte(passSalt)))

	c.Token = url.PathEscape(token)
	c.Salt = url.PathEscape(saltStr)
	return nil
}

func (c *Subsonic) GetLibrary() error {
	return nil
}

func (c *Subsonic) AddLibrary() error {
	return nil
}

func (c *Subsonic) SearchSongs(tracks []*models.Track) error {
	for _, track := range tracks {
		searchQuery := fmt.Sprintf("%s %s", track.Title, track.MainArtist)

		reqParam := fmt.Sprintf("search3?query=%s&f=json", url.QueryEscape(searchQuery))

		body, err := c.subsonicRequest(reqParam)
		if err != nil {
			return err
		}

		var resp SubResponse
    	if err := util.ParseResp(body, &resp); err != nil {
        return err
		}

		switch len(resp.SubsonicResponse.SearchResult3.Song) {
		case 0:
			debug.Debug(fmt.Sprintf("no results found for %s", searchQuery))
		case 1:
			track.ID = resp.SubsonicResponse.SearchResult3.Song[0].ID
			track.Present = true
		default:
			for i, song := range resp.SubsonicResponse.SearchResult3.Song {
				if strings.Contains(song.Artist, track.MainArtist) && (song.Title == track.Title || song.Title == track.CleanTitle) {
					track.ID = song.ID
					track.Present = true
					break
				} else if i == len(resp.SubsonicResponse.SearchResult3.Song) -1 {
					debug.Debug(fmt.Sprintf("multiple songs found for: %s, but titles do not match with the actual track", searchQuery))
				}
			}
		}
	}
	return nil
}

func (c *Subsonic) RefreshLibrary() error {
	reqParam := "startScan?f=json"
	
	if _, err := c.subsonicRequest(reqParam); err != nil {
		return err
	}
	return nil
}

func (c *Subsonic) CreatePlaylist(tracks []*models.Track) error {
	var trackIDs strings.Builder
	for _, track := range tracks { // build songID parameters
		fmt.Fprintf(&trackIDs, "&songId=%s", track.ID)
	}

	reqParam := fmt.Sprintf("createPlaylist?name=%s%s&f=json", c.Cfg.PlaylistName, trackIDs.String())

	body, err := c.subsonicRequest(reqParam)
	if err != nil {
		return err
	}

	var resp SubResponse
	if err := util.ParseResp(body, &resp); err != nil {
        return err
    }
	
	c.Cfg.PlaylistID = resp.SubsonicResponse.Playlist.ID
	return nil
}

func (c *Subsonic) SearchPlaylist() error {
	reqParam := "getPlaylists?f=json"

	body, err := c.subsonicRequest(reqParam)
	if err != nil {
		return err
	}

	var resp SubResponse
	if err := util.ParseResp(body, &resp); err != nil {
        return err
    }

	for _, playlist := range resp.SubsonicResponse.Playlists.Playlist {
		if playlist.Name == c.Cfg.PlaylistName {
			c.Cfg.PlaylistID = playlist.ID
			return nil

		}
	}
	return nil
}

func (c *Subsonic) UpdatePlaylist(comment string) error {
	reqParam := fmt.Sprintf("updatePlaylist?playlistId=%s&comment=%s&f=json",c.Cfg.PlaylistID, url.QueryEscape(comment))

	if _, err := c.subsonicRequest(reqParam); err != nil {
		return err
	}
	return nil
}

func (c *Subsonic) DeletePlaylist() error {
	reqParam := fmt.Sprintf("deletePlaylist?id=%s&f=json", c.Cfg.PlaylistID)

	if _, err := c.subsonicRequest(reqParam); err != nil {
		return err
	}
	return nil
}

func (c *Subsonic) subsonicRequest(reqParams string) ([]byte, error) {

	reqURL := fmt.Sprintf("%s/rest/%s&u=%s&t=%s&s=%s&v=%s&c=%s",c.Cfg.URL, reqParams, c.Cfg.Creds.User, c.Token, c.Salt, c.Cfg.Subsonic.Version, c.Cfg.ClientID)
	body, err := c.HttpClient.MakeRequest("GET", reqURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to make request %s", err.Error())
	}

	var checkResp FailedResp
	if err = util.ParseResp(body, &checkResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request %s", err.Error())
	} else if checkResp.SubsonicResponse.Status == "failed" {
		return nil, fmt.Errorf("%s", checkResp.SubsonicResponse.Error.Message)
	}
	return body, nil
}