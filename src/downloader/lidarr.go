package downloader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

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
	ID             int    `json:"id"`
	Title          string `json:"title"`
	ArtistID       int    `json:"artistId"`
	ForeignAlbumID string `json:"foreignAlbumId"`
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
	ID                  int    `json:"id"`
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
	ArtistID                int                 `json:"artistId"`
	AlbumID                 int                 `json:"albumId"`
	Size                    int64               `json:"size"`
	Title                   string              `json:"title"`
	SizeLeft                int64               `json:"sizeleft"`
	TimeLeft                string              `json:"timeleft"` // duration string like "00:00:00"
	EstimatedCompletionTime time.Time           `json:"estimatedCompletionTime"`
	Added                   time.Time           `json:"added"`
	Status                  string              `json:"status"`
	ID                      int                 `json:"id"`
	Artist                  []LidarrQueueArtist `json:"artist"`
}

type LidarrHistory struct {
	Page          int            `json:"page"`
	PageSize      int            `json:"pageSize"`
	SortKey       string         `json:"sortKey"`
	SortDirection string         `json:"sortDirection"`
	TotalRecords  int            `json:"totalRecords"`
	Records       []LidarrRecord `json:"records"`
}

type LidarrRecord struct {
	AlbumID             int         `json:"albumId"`
	ArtistID            int         `json:"artistId"`
	TrackID             int         `json:"trackId"`
	SourceTitle         string      `json:"sourceTitle"`
	QualityCutoffNotMet bool        `json:"qualityCutoffNotMet"`
	Data                Data        `json:"data"`
	Track               LidarrTrack `json:"track"`
	ID                  int         `json:"id"`
}

type Data struct {
	FileID         string `json:"fileId"`
	DroppedPath    string `json:"droppedPath"`
	ImportedPath   string `json:"importedPath"`
	DownloadClient string `json:"downloadClient"`
	ReleaseGroup   any    `json:"releaseGroup"`
	Size           string `json:"size"`
	IndexerFlags   string `json:"indexerFlags"`
}

type RootFolder struct {
	Path                     string `json:"path"`
	DefaultMetadataProfileId int    `json:"defaultMetadataProfileId"`
	DefaultQualityProfileId  int    `json:"defaultQualityProfileId"`
}

type AddOptions struct {
	SearchForNewAlbum bool `json:"searchForNewAlbum"`
}

type LidarrCommands []struct {
	Name                string       `json:"name"`
	CommandName         string       `json:"commandName"`
	Message             string       `json:"message,omitempty"`
	Body                CommandsBody `json:"body,omitempty"`
	Priority            string       `json:"priority"`
	Status              string       `json:"status"`
	Result              string       `json:"result"`
	Queued              time.Time    `json:"queued"`
	Started             time.Time    `json:"started,omitempty"`
	Trigger             string       `json:"trigger"`
	StateChangeTime     time.Time    `json:"stateChangeTime,omitempty"`
	SendUpdatesToClient bool         `json:"sendUpdatesToClient"`
	UpdateScheduledTask bool         `json:"updateScheduledTask"`
	ID                  int          `json:"id"`
	Ended               time.Time    `json:"ended,omitempty"`
	Duration            string       `json:"duration,omitempty"`
	LastExecutionTime   time.Time    `json:"lastExecutionTime,omitempty"`
}

type CommandsBody struct {
	AlbumIds            []int  `json:"albumIds"`
	SendUpdatesToClient bool   `json:"sendUpdatesToClient"`
	UpdateScheduledTask bool   `json:"updateScheduledTask"`
	RequiresDiskAccess  bool   `json:"requiresDiskAccess"`
	IsExclusive         bool   `json:"isExclusive"`
	IsTypeExclusive     bool   `json:"isTypeExclusive"`
	IsLongRunning       bool   `json:"isLongRunning"`
	Name                string `json:"name"`
	Trigger             string `json:"trigger"`
	SuppressMessages    bool   `json:"suppressMessages"`
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
		CheckInterval:   time.Duration(c.Cfg.MonitorConfig.Interval) * time.Minute,
		MonitorDuration: time.Duration(c.Cfg.MonitorConfig.Duration) * time.Minute,
		MigrateDownload: c.Cfg.MigrateDL,
		ToDir:           c.DownloadDir,
		FromDir:         c.Cfg.LidarrDir,
		Service:         "Lidarr",
	}, nil
}

func (c *Lidarr) QueryTrack(track *models.Track) error {
	trackDetails := fmt.Sprintf("%s - %s", track.Title, track.Artist)
	slog.Info("initiating search", "track", trackDetails)

	if err := c.getReleaseGroupId(track); err != nil {
		return fmt.Errorf("failed to get release group id for %s - %s: %w", track.Title, track.Artist, err)
	}

	queryURL := fmt.Sprintf("%s/api/v1/album?foreignAlbumId=%s", c.Cfg.URL, track.MusicBrainzReleaseGroupID)
	body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, c.Headers)
	if err != nil {
		return fmt.Errorf("failed to check library for album: %w", err)
	}

	var libraryAlbums []Album
	if err = util.ParseResp(body, &libraryAlbums); err != nil {
		return fmt.Errorf("failed to unmarshal library albums: %w", err)
	}

	if len(libraryAlbums) == 0 {
		slog.Info("album not found in Lidarr library")
		return nil
	}

	libraryAlbum := libraryAlbums[0]
	slog.Info("album found in Lidarr library", "album", libraryAlbum.Title, "id", libraryAlbum.ID)
	track.ID = strconv.Itoa(libraryAlbum.ID)

	queryURL = fmt.Sprintf("%s/api/v1/track?artistId=%v&albumId=%v", c.Cfg.URL, libraryAlbum.ArtistID, libraryAlbum.ID)
	body, err = c.HttpClient.MakeRequest("GET", queryURL, nil, c.Headers)
	if err != nil {
		return fmt.Errorf("failed to check existing tracks: %w", err)
	}

	var lidarrTracks []LidarrTrack
	if err = util.ParseResp(body, &lidarrTracks); err != nil {
		return fmt.Errorf("failed to unmarshal lidarr tracks: %w", err)
	}

	for _, t := range lidarrTracks {
		if util.ContainsFold(t.Title, track.Title) && t.HasFile {
			track.Present = true
			slog.Info("track already present in Lidarr", "track", trackDetails)
			return nil
		}
	}

	return nil
}

func (c Lidarr) GetTrack(track *models.Track) error {

	slog.Info("downloading track",
		"title", track.Title,
		"artist", track.Artist,
		"album", track.Album,
	)
	if track.Present {
		return nil
	}

	if track.ID != "" {
		albumID, err := strconv.Atoi(track.ID)
		if err != nil {
			return fmt.Errorf("invalid lidarr album ID: %w", err)
		}
		slog.Info("album in library but track missing, triggering search", "album", track.Album, "id", albumID)
		payload := map[string]any{
			"name":     "AlbumSearch",
			"albumIds": []int{albumID},
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal error: %w", err)
		}
		_, err = c.HttpClient.MakeRequest("POST", fmt.Sprintf("%s/api/v1/command", c.Cfg.URL), bytes.NewReader(body), c.Headers)
		if err != nil {
			return fmt.Errorf("failed to trigger album search: %w", err)
		}

		if err := c.searchStatus(albumID, 1); err != nil {
			return fmt.Errorf("album search failed: %w", err)
		}
		track.File = track.Title
		return nil
	}

	rootFolder, err := c.getRootDirectory()
	if err != nil {
		return fmt.Errorf("could not look up root directory: %w", err)
	}

	payload := map[string]any{
		"foreignAlbumId": track.MusicBrainzReleaseGroupID,
		"monitored":      true,
		"anyReleaseOk":   true,
		"artist": map[string]any{
			"qualityProfileId":  rootFolder.DefaultQualityProfileId,
			"metadataProfileId": rootFolder.DefaultMetadataProfileId,
			"foreignArtistId":   track.MusicBrainzArtistID,
			"rootFolderPath":    rootFolder.Path,
		},
		"addOptions": AddOptions{
			SearchForNewAlbum: true,
		},
	}

	payloadBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}
	queryURL := fmt.Sprintf("%s/api/v1/album", c.Cfg.URL)

	body, err := c.HttpClient.MakeRequest("POST", queryURL, bytes.NewReader(payloadBody), c.Headers)
	if err != nil {
		if strings.Contains(err.Error(), "got 409") {
			slog.Debug("album already in Lidarr, skipping", "album", track.MusicBrainzReleaseGroupID)
			return nil
		}
		return fmt.Errorf("failed to add album: %w", err)
	}
	var album Album
	if err = util.ParseResp(body, &album); err != nil {
		return fmt.Errorf("failed to unmarshal lidarr album: %w", err)
	}

	if err := c.searchStatus(album.ID, 1); err != nil {
			return fmt.Errorf("album search failed: %w", err)
	}

	track.ID = strconv.Itoa(album.ID)
	track.File = track.CleanTitle
	slog.Info("download started", "AlbumID", track.ID)
	return nil
}
// Check whether album search is completed
func (c *Lidarr) searchStatus(AlbumID, count int) error {
	reqParams := "/api/v1/command"
		body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParams, nil, c.Headers)
	if err != nil {
		return err
	}

	var commands LidarrCommands
	if err := util.ParseResp(body, &commands); err != nil {
		return err
	}
	for _, command := range commands {
		if command.Name != "AlbumSearch" ||
        	!slices.Contains(command.Body.AlbumIds, AlbumID) {
        		continue
    		}
		switch command.Status {
		case "completed":
			return nil
		case "failed", "aborted", "cancelled", "orphaned":
			return fmt.Errorf("index search %s, skipping album", command.Status)
		}
	}
	if count >= c.Cfg.Retry {
		return fmt.Errorf("search wasn't completed after %d retries, skipping album %d", count, AlbumID)
	}
	slog.Debug("waiting for Lidarr album search", "albumID", AlbumID, "attempt", count, "maxAttempts", c.Cfg.Retry)
	time.Sleep(15 * time.Second)
	return c.searchStatus(AlbumID, count+1)
}

func (c *Lidarr) GetDownloadStatus(tracks []*models.Track) (map[string]FileStatus, error) {
	
	statuses := make(map[string]FileStatus)
	requested := make(map[string]struct{})
	// check for completed downloads and build map of tracks that might be in queue
	for _, track := range tracks {
		if track.ID == "" || track.Present {
			continue
		}
		
		requested[track.ID] = struct{}{}
		
		file, err := c.checkHistory(*track)
		if err != nil {
			slog.Warn("failed to check download history", "err", err)
		}

		if file != "" {
			statuses[track.ID] = FileStatus{
			ID:               track.ID,
			Filename:         file,
			State:            "Succeeded",
			BytesTransferred: 1,
			BytesRemaining:   0,
			PercentComplete:  100,
			QueueID:          "",
			}
		}
	
	}
	var totalPages = 1
	pageSize := 50
	var queues []LidarrQueue
	for page := 1; page <= totalPages; page++ {
		req := fmt.Sprintf("/api/v1/queue?pageSize=%d&page=%d", pageSize, page)

		body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+req, nil, c.Headers)
		if err != nil {
			return nil, err
		}

		var queue LidarrQueue
		if err := util.ParseResp(body, &queue); err != nil {
			return nil, err
		}
		queues = append(queues, queue)
		if page == 1 {
        totalPages = (queue.TotalRecords + pageSize - 1) / pageSize
    }
	}
	for _, queue := range queues {
		for _, record := range queue.Records {
			ID := strconv.Itoa(record.AlbumID)
			if _, ok := requested[ID]; !ok {
				continue
			}
			if _, e := statuses[ID]; e {
				continue
			}
			
			statuses[ID] = FileStatus{
				ID:               ID,
				State:            record.Status,
				BytesRemaining:   int(record.SizeLeft),
				BytesTransferred: int(record.Size - record.SizeLeft),
				PercentComplete:  percent(record.Size, record.SizeLeft),
				QueueID:          strconv.Itoa(record.ID),
			}
		}
	}

	return statuses, nil
}

// Checks Lidarr history to see if album is downloaded. Returns file path if found
func (c *Lidarr) checkHistory(track models.Track) (string, error) {
	req := fmt.Sprintf("/api/v1/history?albumId=%s&pageSize=1111", track.ID)
	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+req, nil, c.Headers)
	if err != nil {
		return "", err
	}
	var history LidarrHistory
	if err := util.ParseResp(body, &history); err != nil {
		return "", err
	}

	mbTrack := track.MusicBrainzTrackID
	mbReleaseTrack := track.MusicBrainzReleaseTrackID
	for _, r := range history.Records {

		mbIDMatch := (mbTrack != "" && mbTrack == r.Track.ForeignTrackID) || (mbReleaseTrack != "" && mbReleaseTrack == r.Track.ForeignTrackID)
		titleMatch := util.ContainsFold(r.SourceTitle, track.CleanTitle) || util.ContainsFold(r.Track.Title, track.CleanTitle)
		if (mbIDMatch || titleMatch) && r.Data.ImportedPath != "" {
			return r.Data.ImportedPath, nil
		}
	}
	return "", nil

}

func (c Lidarr) getRootDirectory() (*RootFolder, error) {
	// Get the defaults from the root dir
	queryURL := fmt.Sprintf("%s/api/v1/rootfolder", c.Cfg.URL)
	body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, c.Headers)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup root folder: %w", err)
	}

	var rootFolders []RootFolder
	if err = util.ParseResp(body, &rootFolders); err != nil {
		return nil, fmt.Errorf("failed to unmarshal query root folder: %w", err)
	}

	if len(rootFolders) == 0 {
		return nil, fmt.Errorf("no root folders found in Lidarr")
	}
	rootFolder := rootFolders[0]
	return &rootFolder, nil
}

func (c Lidarr) getReleaseGroupId(track *models.Track) error {
	if track.MusicBrainzReleaseGroupID != "" {
		return nil
	}

	escQuery := url.PathEscape(fmt.Sprintf("%s - %s", track.Album, track.MainArtist))
	queryURL := fmt.Sprintf("%s/api/v1/album/lookup?term=%s", c.Cfg.URL, escQuery)

	body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, c.Headers)
	if err != nil {
		return fmt.Errorf("failed to lookup album: %w", err)
	}

	var albums []Album
	if err = util.ParseResp(body, &albums); err != nil {
		return fmt.Errorf("failed to unmarshal lookup response: %w", err)
	}

	if len(albums) == 0 {
		return fmt.Errorf("could not find album for track: %s - %s", track.Title, track.MainArtist)
	}

	track.MusicBrainzReleaseGroupID = albums[0].ForeignAlbumID
	return nil
}

func percent(total, remaining int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(total-remaining) / float64(total) * 100
}

func (c Lidarr) deleteDownload(ID string) error {
	reqParams := fmt.Sprintf("/api/v1/queue/%s", ID)

	time.Sleep(1 * time.Second)
	if _, err := c.HttpClient.MakeRequest("DELETE", c.Cfg.URL+reqParams+"?removeFromClient=true", nil, c.Headers); err != nil {
		return fmt.Errorf("hard delete failed: %w", err)
	}
	return nil
}

func (c *Lidarr) Cleanup(track models.Track, queueID string) error {
	if track.Present { // dont clear downloaded files from download client
		return nil
	}
	if err := c.deleteDownload(queueID); err != nil {
		slog.Info(fmt.Sprintf("[lidarr] failed to delete download: %v", err))
	}
	return nil
}
