package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"explo/src/config"
	"explo/src/util"
)

type Lidarr struct {
	Cfg        config.LidarrConfig
	HttpClient *util.HttpClient
	Headers    map[string]string
}

type LidarrSystemStatus struct {
	Version    string `json:"version"`
	AppName    string `json:"appName"`
	InstanceID string `json:"instanceName"`
}

type LidarrAddOptions struct {
	Monitor                string `json:"monitor"`
	SearchForMissingAlbums bool   `json:"searchForMissingAlbums"`
}

type LidarrArtist struct {
	ID                int              `json:"id,omitempty"`
	ForeignArtistID   string           `json:"foreignArtistId"`
	ArtistName        string           `json:"artistName"`
	Monitored         bool             `json:"monitored"`
	MonitorNewItems   string           `json:"monitorNewItems,omitempty"`
	QualityProfileID  int              `json:"qualityProfileId,omitempty"`
	MetadataProfileID int              `json:"metadataProfileId,omitempty"`
	RootFolderPath    string           `json:"rootFolderPath,omitempty"`
	AddOptions        *LidarrAddOptions `json:"addOptions,omitempty"`
}

type LidarrAlbum struct {
	ID             int    `json:"id"`
	ForeignAlbumID string `json:"foreignAlbumId"`
	Title          string `json:"title"`
	ArtistID       int    `json:"artistId"`
	Monitored      bool   `json:"monitored"`
}

type LidarrCommand struct {
	Name     string `json:"name"`
	AlbumIDs []int  `json:"albumIds,omitempty"`
	ArtistID int    `json:"artistId,omitempty"`
}

type LidarrRootFolder struct {
	ID         int    `json:"id"`
	Path       string `json:"path"`
	Accessible bool   `json:"accessible"`
}

type LidarrQualityProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type LidarrMetadataProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func NewLidarr(cfg config.LidarrConfig, httpClient *util.HttpClient) *Lidarr {
	return &Lidarr{
		Cfg:        cfg,
		HttpClient: httpClient,
		Headers: map[string]string{
			"X-Api-Key": cfg.APIKey,
		},
	}
}

func (c *Lidarr) endpoint(path string) string {
	return strings.TrimRight(c.Cfg.URL, "/") + path
}

func (c *Lidarr) TestConnection() (string, error) {
	body, err := c.HttpClient.MakeRequest("GET", c.endpoint("/api/v1/system/status"), nil, c.Headers)
	if err != nil {
		return "", err
	}
	var status LidarrSystemStatus
	if err := util.ParseResp(body, &status); err != nil {
		return "", err
	}
	return status.Version, nil
}

func (c *Lidarr) LookupArtist(mbid string) ([]LidarrArtist, error) {
	if mbid == "" {
		return nil, fmt.Errorf("empty MBID")
	}
	params := "/api/v1/artist/lookup?term=" + url.QueryEscape("lidarr:"+mbid)
	body, err := c.HttpClient.MakeRequest("GET", c.endpoint(params), nil, c.Headers)
	if err != nil {
		return nil, err
	}
	var results []LidarrArtist
	if err := util.ParseResp(body, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *Lidarr) LookupArtistByName(name string) ([]LidarrArtist, error) {
	params := "/api/v1/artist/lookup?term=" + url.QueryEscape(name)
	body, err := c.HttpClient.MakeRequest("GET", c.endpoint(params), nil, c.Headers)
	if err != nil {
		return nil, err
	}
	var results []LidarrArtist
	if err := util.ParseResp(body, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *Lidarr) GetArtists() ([]LidarrArtist, error) {
	body, err := c.HttpClient.MakeRequest("GET", c.endpoint("/api/v1/artist"), nil, c.Headers)
	if err != nil {
		return nil, err
	}
	var results []LidarrArtist
	if err := util.ParseResp(body, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *Lidarr) AddArtist(artist LidarrArtist) (*LidarrArtist, error) {
	payload, err := json.Marshal(artist)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal artist: %s", err.Error())
	}
	body, err := c.HttpClient.MakeRequest("POST", c.endpoint("/api/v1/artist"), bytes.NewBuffer(payload), c.Headers)
	if err != nil {
		return nil, err
	}
	var created LidarrArtist
	if err := util.ParseResp(body, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

func (c *Lidarr) RefreshArtist(artistID int) error {
	cmd := LidarrCommand{Name: "RefreshArtist", ArtistID: artistID}
	payload, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %s", err.Error())
	}
	_, err = c.HttpClient.MakeRequest("POST", c.endpoint("/api/v1/command"), bytes.NewBuffer(payload), c.Headers)
	return err
}

func (c *Lidarr) GetAlbumsByArtist(artistID int) ([]LidarrAlbum, error) {
	params := fmt.Sprintf("/api/v1/album?artistId=%d", artistID)
	body, err := c.HttpClient.MakeRequest("GET", c.endpoint(params), nil, c.Headers)
	if err != nil {
		return nil, err
	}
	var albums []LidarrAlbum
	if err := util.ParseResp(body, &albums); err != nil {
		return nil, err
	}
	return albums, nil
}

func (c *Lidarr) MonitorAlbum(album LidarrAlbum) error {
	album.Monitored = true
	payload, err := json.Marshal(album)
	if err != nil {
		return fmt.Errorf("failed to marshal album: %s", err.Error())
	}
	_, err = c.HttpClient.MakeRequest("PUT", c.endpoint(fmt.Sprintf("/api/v1/album/%d", album.ID)), bytes.NewBuffer(payload), c.Headers)
	return err
}

func (c *Lidarr) SearchAlbum(albumID int) error {
	cmd := LidarrCommand{Name: "AlbumSearch", AlbumIDs: []int{albumID}}
	payload, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %s", err.Error())
	}
	_, err = c.HttpClient.MakeRequest("POST", c.endpoint("/api/v1/command"), bytes.NewBuffer(payload), c.Headers)
	return err
}

func (c *Lidarr) GetRootFolders() ([]LidarrRootFolder, error) {
	body, err := c.HttpClient.MakeRequest("GET", c.endpoint("/api/v1/rootfolder"), nil, c.Headers)
	if err != nil {
		return nil, err
	}
	var folders []LidarrRootFolder
	if err := util.ParseResp(body, &folders); err != nil {
		return nil, err
	}
	return folders, nil
}

func (c *Lidarr) GetQualityProfiles() ([]LidarrQualityProfile, error) {
	body, err := c.HttpClient.MakeRequest("GET", c.endpoint("/api/v1/qualityprofile"), nil, c.Headers)
	if err != nil {
		return nil, err
	}
	var profiles []LidarrQualityProfile
	if err := util.ParseResp(body, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (c *Lidarr) GetMetadataProfiles() ([]LidarrMetadataProfile, error) {
	body, err := c.HttpClient.MakeRequest("GET", c.endpoint("/api/v1/metadataprofile"), nil, c.Headers)
	if err != nil {
		return nil, err
	}
	var profiles []LidarrMetadataProfile
	if err := util.ParseResp(body, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}
