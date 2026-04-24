package downloader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"strconv"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

type Lidarr struct {
	Headers     map[string]string
	DownloadDir string
	HttpClient  *util.HttpClient
	Cfg         cfg.Lidarr
}

type Album struct {
	ID             int       `json:"id"`
	Title          string    `json:"title"`
	Disambiguation string    `json:"disambiguation"`
	Overview       string    `json:"overview"`
	ArtistID       int       `json:"artistId"`
	ForeignAlbumID string    `json:"foreignAlbumId"`
	Monitored      bool      `json:"monitored"`
	AnyReleaseOK   bool      `json:"anyReleaseOk"`
	ProfileID      int       `json:"profileId"`
	Duration       int       `json:"duration"`
	AlbumType      string    `json:"albumType"`
	SecondaryTypes []string  `json:"secondaryTypes"`
	MediumCount    int       `json:"mediumCount"`
	Ratings        Ratings   `json:"ratings"`
	ReleaseDate    string    `json:"releaseDate"`
	Releases       []Release `json:"releases"`
	Genres         []string  `json:"genres"`
	Media          []Media   `json:"media"`
	Artist         Artist    `json:"artist"`
}

type Ratings struct {
	Votes int     `json:"votes"`
	Value float64 `json:"value"`
}

type Release struct {
	ID               int      `json:"id"`
	AlbumID          int      `json:"albumId"`
	ForeignReleaseID string   `json:"foreignReleaseId"`
	Title            string   `json:"title"`
	Status           string   `json:"status"`
	Duration         int      `json:"duration"`
	TrackCount       int      `json:"trackCount"`
	Media            []Media  `json:"media"`
	MediumCount      int      `json:"mediumCount"`
	Disambiguation   string   `json:"disambiguation"`
	Country          []string `json:"country"`
	Label            []string `json:"label"`
	Format           string   `json:"format"`
	Monitored        bool     `json:"monitored"`
}

type Media struct {
	MediumNumber int    `json:"mediumNumber"`
	MediumName   string `json:"mediumName"`
	MediumFormat string `json:"mediumFormat"`
}

type Artist struct {
	Status            string `json:"status"`
	Ended             bool   `json:"ended"`
	ArtistName        string `json:"artistName"`
	ForeignArtistID   string `json:"foreignArtistId"`
	ArtistType        string `json:"artistType"`
	Disambiguation    string `json:"disambiguation"`
	QualityProfileID  int
	MetadataProfileID int
	RootFolderPath    string
}

type LidarrTrack struct {
	ArtistID            int    `json:"artistId"`
	ForeignTrackID      string `json:"foreignTrackId"`
	ForeignRecordingID  string `json:"foreignRecordingId"`
	TrackFileID         int    `json:"trackFileId"`
	AlbumID             int    `json:"albumId"`
	Explicit            bool   `json:"explicit"`
	AbsoluteTrackNumber int    `json:"absoluteTrackNumber"`
	TrackNumber         string `json:"trackNumber"`
	Title               string `json:"title"`
	Duration            int    `json:"duration"` // In milliseconds
	MediumNumber        int    `json:"mediumNumber"`
	HasFile             bool   `json:"hasFile"`
	Ratings             struct {
		Votes int     `json:"votes"`
		Value float64 `json:"value"`
	} `json:"ratings"`
	ID int `json:"id"`
}

type LidarrQueue struct {
	TotalRecords int               `json:"totalRecords"`
	Records      []LidarrQueueItem `json:"records"`
}

type LidarrQueueArtist struct {
	ForeignArtistID string           `json:"foreignArtistId"`
	Album           LidarrQueueAlbum `json:"album"`
}

type LidarrQueueAlbum struct {
	ForeignAlbumID string `json:"foreignAlbumId"`
}

type LidarrQueueItem struct {
	ArtistID                            int                 `json:"artistId"`
	AlbumID                             int                 `json:"albumId"`
	Size                                int64               `json:"size"`
	Title                               string              `json:"title"`
	SizeLeft                            int64               `json:"sizeleft"`
	TimeLeft                            string              `json:"timeleft"` // duration string like "00:00:00"
	EstimatedCompletionTime             time.Time           `json:"estimatedCompletionTime"`
	Added                               time.Time           `json:"added"`
	Status                              string              `json:"status"`
	TrackedDownloadStatus               string              `json:"trackedDownloadStatus"`
	TrackedDownloadState                string              `json:"trackedDownloadState"`
	StatusMessages                      []string            `json:"statusMessages"`
	DownloadID                          string              `json:"downloadId"`
	Protocol                            string              `json:"protocol"`
	DownloadClient                      string              `json:"downloadClient"`
	DownloadClientHasPostImportCategory bool                `json:"downloadClientHasPostImportCategory"`
	Indexer                             string              `json:"indexer"`
	TrackFileCount                      int                 `json:"trackFileCount"`
	TrackHasFileCount                   int                 `json:"trackHasFileCount"`
	DownloadForced                      bool                `json:"downloadForced"`
	ID                                  int64               `json:"id"`
	Artist                              []LidarrQueueArtist `json:"artist"`
}

type Image struct {
	// can leave empty for now
}

type AddOptions struct {
	SearchForNewAlbum bool `json:"searchForNewAlbum"`
}

type MinimalArtist struct {
	ForeignArtistID   string `json:"foreignArtistId"`
	QualityProfileID  int    `json:"qualityProfileId"`
	MetadataProfileID int    `json:"metadataProfileId"`
	Monitored         bool   `json:"monitored"`
	RootFolderPath    string `json:"rootFolderPath"`
}

type AddAlbumRequest struct {
	ForeignAlbumID string        `json:"foreignAlbumId"`
	Images         []Image       `json:"images"`
	Monitored      bool          `json:"monitored"`
	AnyReleaseOk   bool          `json:"anyReleaseOk"`
	Artist         MinimalArtist `json:"artist"`
	AddOptions     AddOptions    `json:"addOptions"`
	Releases       []Release     `json:"releases"`
}

type RootFolder struct {
	Path                     string `json:"path"`
	DefaultMetadataProfileId int    `json:"defaultMetadataProfileId"`
	DefaultQualityProfileId  int    `json:"defaultQualityProfileId"`
}

func NewLidarr(cfg cfg.Lidarr, downloadDir string) *Lidarr { // init downloader cfg for lidarr
	return &Lidarr{
		Cfg:         cfg,
		HttpClient:  util.NewHttp(util.HttpClientConfig{Timeout: cfg.Timeout}),
		DownloadDir: downloadDir,
	}
}

func (c *Lidarr) AddHeader() {
	if c.Headers == nil {
		c.Headers = make(map[string]string)
	}
	c.Headers["X-API-Key"] = c.Cfg.APIKey
}

func (c *Lidarr) GetConf() (MonitorConfig, error) {
	return MonitorConfig{
		CheckInterval:   c.Cfg.MonitorConfig.Interval,
		MonitorDuration: c.Cfg.MonitorConfig.Duration,
		MigrateDownload: c.Cfg.MigrateDL,
		ToDir:           c.DownloadDir,
		FromDir:         c.Cfg.LidarrDir,
		Service:         "Lidarr",
	}, nil
}

func (c *Lidarr) QueryTrack(track *models.Track) error {

	slog.Debug("querying track", 
	    "title", track.Title,
	    "artist", track.Artist,
	    "album", track.Album,
	)
	slog.Debug(fmt.Sprintf("looking for track %s by %s on album %s", track.Title, track.Artist, track.Album))

	album, err := c.findBestAlbumMatch(track)
	if err != nil {
		return err
	}

	queryURL := fmt.Sprintf("%s/api/v1/track?apiKey=%s&artistId=%v&albumId=%v", c.Cfg.URL, c.Cfg.APIKey, album.ArtistID, album.Releases[0].AlbumID)
	body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to check existing tracks: %w", err)
	}

	var lidarrTracks []LidarrTrack
	if err = util.ParseResp(body, &lidarrTracks); err != nil {
		return fmt.Errorf("failed to unmarshal query lidarr tracks body: %w", err)
	}

	for _, t := range lidarrTracks {
		if strings.Contains(t.Title, track.Title) {
			if t.HasFile {
				track.Present = true
			}
		}
	}

	return nil
}

func (c Lidarr) GetTrack(track *models.Track) error {

	slog.Debug("downloading track", 
	    "title", track.Title,
	    "artist", track.Artist,
	    "album", track.Album,
	)
	if track.Present {
		return nil
	}

	// Get the defaults from the root dir
	queryURL := fmt.Sprintf("%s/api/v1/rootfolder?apiKey=%s", c.Cfg.URL, c.Cfg.APIKey)

	body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to lookup root folder: %w", err)
	}

	var rootFolders []RootFolder
	if err = util.ParseResp(body, &rootFolders); err != nil {
		return fmt.Errorf("failed to unmarshal query lidarr body: %w", err)
	}

	if len(rootFolders) == 0 {
		return fmt.Errorf("no root folders found in Lidarr")
	}
	rootFolder := rootFolders[0]

	album, err := c.findBestAlbumMatch(track)
	if err != nil {
		return err
	}

	payload := AddAlbumRequest{
		ForeignAlbumID: track.AlbumMBID,
		Images:         []Image{},
		Monitored:      true,
		AnyReleaseOk:   true,
		Artist: MinimalArtist{
			QualityProfileID:  rootFolder.DefaultQualityProfileId,
			MetadataProfileID: rootFolder.DefaultMetadataProfileId,
			Monitored:         false,
			ForeignArtistID:   track.ArtistMBID,
			RootFolderPath:    rootFolder.Path,
		},
		AddOptions: AddOptions{
			SearchForNewAlbum: true,
		},
		Releases: []Release{album.Releases[0]},
	}

	body, err = json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}
	queryURL = fmt.Sprintf("%s/api/v1/album?apiKey=%s", c.Cfg.URL, c.Cfg.APIKey)
	_, err = c.HttpClient.MakeRequest("POST", queryURL, bytes.NewReader(body), nil)
	if err != nil {
		return fmt.Errorf("failed to add album: %w", err)
	}
	return nil
}

func (c Lidarr) findBestAlbumMatch(track *models.Track) (*Album, error) {
	escQuery := url.PathEscape(fmt.Sprintf("%s - %s", track.Album, track.MainArtist))
	queryURL := fmt.Sprintf("%s/api/v1/album/lookup?apiKey=%s&term=%s", c.Cfg.URL, c.Cfg.APIKey, escQuery)

	body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup tracks: %w", err)
	}

	var albums []Album
	if err = util.ParseResp(body, &albums); err != nil {
		return nil, fmt.Errorf("failed to unmarshal query lidarr body: %w", err)
	}

	if len(albums) == 0 {
		return nil, fmt.Errorf("could not find album for track: %s - %s", track.Title, track.MainArtist)
	}
	topMatch := albums[0]
	if len(topMatch.Releases) == 0 {
		return nil, fmt.Errorf("could not find album releases for track: %s - %s", track.Title, track.MainArtist)
	}

	track.AlbumMBID = topMatch.ForeignAlbumID
	track.ArtistMBID = topMatch.Artist.ForeignArtistID

	if topMatch.Releases[0].ID == 0 || topMatch.ArtistID == 0 {
		return nil, fmt.Errorf("invalid album or artist ID for track: %s - %s", track.Title, track.MainArtist)
	}

	return &topMatch, nil
}


func (c *Lidarr) GetDownloadStatus(tracks []*models.Track) (map[string]FileStatus, error) {
	req := fmt.Sprintf("/api/v1/queue?apiKey=%s", c.Cfg.APIKey)

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+req, nil, nil)
	if err != nil {
		return nil, err
	}

	var queue LidarrQueue
	if err := util.ParseResp(body, &queue); err != nil {
		return nil, err
	}

	statuses := make(map[string]FileStatus)

	for _, record := range queue.Records {
		// MVP assumption: record.Title matches track.File closely enough
		statuses[record.Title] = FileStatus{
			ID:               strconv.FormatInt(record.ID, 10),
			State:            record.Status,
			BytesRemaining:   int(record.SizeLeft),
			BytesTransferred: int(record.Size - record.SizeLeft),
			PercentComplete:  percent(record.Size, record.SizeLeft),
		}
	}

	if len(statuses) == 0 {
		return nil, fmt.Errorf("no queue items found")
	}

	return statuses, nil
}

func percent(total, remaining int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(total-remaining) / float64(total) * 100
}

func (c Lidarr) deleteDownload(ID string) error {
	reqParams := fmt.Sprintf("/api/v1/queue/%s?apiKey=%s", ID, c.Cfg.APIKey)

	// cancel download
	if _, err := c.HttpClient.MakeRequest("DELETE", c.Cfg.URL+reqParams+"/removeFromClient=false", nil, nil); err != nil {
		return fmt.Errorf("soft delete failed: %w", err)
	}
	time.Sleep(1 * time.Second) // Small buffer between soft and hard delete
	// delete download
	if _, err := c.HttpClient.MakeRequest("DELETE", c.Cfg.URL+reqParams+"/removeFromClient=true", nil, nil); err != nil {
		return fmt.Errorf("hard delete failed: %w", err)
	}

	return nil
}

func (c *Lidarr) Cleanup(track models.Track, fileID string) error {
	if err := c.deleteDownload(fileID); err != nil {
		slog.Debug(fmt.Sprintf("[lidarr] failed to delete download: %v", err))
	}
	return nil
}
