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
	"sync"
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
	AlbumCache	map[string]*AlbumMetadata
	CacheMu   	sync.RWMutex
	RootFolder 	RootFolder
}

type AlbumMetadata struct {
	AlbumID 		 string
	ArtistID 		 string
	ReleaseGroupMBID string
	ArtistMBID 		 string

	Tracks  func()   []LidarrTrack
}

type Album struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	ArtistID       int    `json:"artistId"`
	ForeignAlbumID string `json:"foreignAlbumId"`
	Artist         struct {
		ForeignArtistID string `json:"foreignArtistId"`
	} `json:"artist"`
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
	Name                     string `json:"name"`
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

func (c *Lidarr) GetTrack(track *models.Track) error {
	slog.Info("downloading track",
		"title", track.Title,
		"artist", track.Artist,
		"album", track.Album,
	)
	if track.Present {
		return nil
	}

	if c.populateFromCache(track) {
    	return c.findTrackID(track)
	}

	if track.AlbumID != "" {
		albumID, err := strconv.Atoi(track.AlbumID)
		if err != nil {
			return fmt.Errorf("invalid lidarr album ID: %w", err)
		}
		slog.Info("album in library but track missing, triggering search", "album", track.Album, "id", albumID)
		
		if err = c.triggerAlbumSearch(albumID); err != nil {
			return fmt.Errorf("failed to trigger album search: %w", err)
		}

		c.cacheAlbum(track)

		if err := c.searchStatus(albumID, 1); err != nil {
			return fmt.Errorf("album search failed: %w", err)
		}

		track.File = track.CleanTitle
		return nil
	}

	if err := c.addNewAlbum(track); err != nil {
		return fmt.Errorf("failed to add album: %w", err)
	}

	if c.populateFromCache(track) {
    	return c.findTrackID(track)
	}
	track.File = track.CleanTitle

	slog.Info("download started", "AlbumID", track.AlbumID)
	return nil
}

func (c *Lidarr) addNewAlbum(track *models.Track) error {

	payload := map[string]any{
		"foreignAlbumId": track.MusicBrainzReleaseGroupID,
		"monitored":      true,
		"anyReleaseOk":   true,
		"artist": map[string]any{
			"qualityProfileId":  c.RootFolder.DefaultQualityProfileId,
			"metadataProfileId": c.RootFolder.DefaultMetadataProfileId,
			"foreignArtistId":   track.MusicBrainzArtistID,
			"rootFolderPath":    c.RootFolder.Path,
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
		if strings.Contains(err.Error(), "got 409") || strings.Contains(string(body), "has already been added") {
			if _, ok := c.AlbumCache[c.cacheKey(track)]; ok {
				slog.Debug("album already in Lidarr", "album", track.MusicBrainzReleaseGroupID)
				return nil
			}
			return fmt.Errorf("album present in lidarr but explo couldn't find it")
		}
		return err
	}
	var album Album
	if err = util.ParseResp(body, &album); err != nil {
		return fmt.Errorf("failed to unmarshal lidarr album: %w", err)
	}
	track.AlbumID = strconv.Itoa(album.ID)
	track.MainArtistID = strconv.Itoa(album.ArtistID)
	c.cacheAlbum(track)
	if err := c.searchStatus(album.ID, 1); err != nil {
			return fmt.Errorf("album search failed: %w", err)
	}
	return nil
}

func (c *Lidarr) triggerAlbumSearch(albumID int) error {
	payload := map[string]any{
		"name":     "AlbumSearch",
		"albumIds": []int{albumID},
		}
	payloadBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}
	_, err = c.HttpClient.MakeRequest("POST", fmt.Sprintf("%s/api/v1/command", c.Cfg.URL), bytes.NewReader(payloadBody), c.Headers)
	if err != nil {
		return err
	}
	return nil
}

func (c *Lidarr) getAlbumTracks(albumID string, artistID string) []LidarrTrack {
	queryURL := fmt.Sprintf("%s/api/v1/track?albumId=%s&artistId=%s", c.Cfg.URL, albumID, artistID)
	for attempt := 1; attempt <= 3; attempt++ {
		body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, c.Headers)
		if err == nil {
			var tracks []LidarrTrack
			if err := util.ParseResp(body, &tracks); err == nil {
				return tracks
			}
		}
		slog.Warn("failed loading album tracks", "attempt", attempt, "err", err)

		if attempt < 3 {
			time.Sleep(time.Second)
		}
	}
	return nil
}

func (c *Lidarr) findTrackID(track *models.Track) error {
	key := c.cacheKey(track)

	c.CacheMu.RLock()
	a, ok := c.AlbumCache[key]
	c.CacheMu.RUnlock()

	if !ok {
		return fmt.Errorf("album not cached")
	}

	alnumTrackTitle := util.AlnumOnly(track.CleanTitle)
	for _, t := range a.Tracks() {
		mbIDMatch := (track.MusicBrainzReleaseTrackID != "" && track.MusicBrainzReleaseTrackID == t.ForeignTrackID) ||
			(track.MusicBrainzTrackID != "" && track.MusicBrainzTrackID == t.ForeignRecordingID)
		alnumLidarrTitle := util.AlnumOnly(t.Title)
		fmt.Printf("comparing Lidarr title: %s\n track title: %s\n Lidarr TrackID: %s\nTrack MBID: %s", alnumLidarrTitle, alnumTrackTitle, t.ForeignRecordingID, track.MusicBrainzTrackID)
		titleMatch := util.ContainsFold(alnumLidarrTitle, alnumTrackTitle)

		if mbIDMatch || titleMatch {
			if t.HasFile {
				track.Present = true
				slog.Info("track already present in Lidarr", "track", track.CleanTitle, "album", track.Album, "artist", track.MainArtist)
			}
			track.ID = strconv.Itoa(t.ID)
			return nil
		}
	}
	return fmt.Errorf("could not find track '%s' from Album: %s", track.CleanTitle, track.Album)
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

	albums := make(map[string][]*models.Track)

	for _, track := range tracks {
		if track.ID == "" || track.AlbumID == "" || track.Present {
			continue
		}
    	albums[track.AlbumID] = append(albums[track.AlbumID], track)
	}
	statuses := make(map[string]FileStatus)
	queues, err := c.getQueue()
		if err != nil {
			slog.Warn("[lidarr] failed to check queue", "err", err)
		}

	queueMap := make(map[string]LidarrQueueItem)

	for _, queue := range queues {
    	for _, record := range queue.Records {
        	queueMap[strconv.Itoa(record.AlbumID)] = record
    	}
	}

	// check for completed downloads and build map of tracks that might be in queue
	for albumID, albumTracks := range albums {

		history ,err := c.getHistory(albumID)
		if err != nil {
			slog.Warn("[lidarr] failed to check download history", "err", err)
		}

		for _, r := range history.Records {
			for _, track := range albumTracks {
				if _, exists := statuses[track.ID]; exists {
         		   continue
        		}
				trackIDMatch := track.ID == strconv.Itoa(r.TrackID)
				mbTrack := track.MusicBrainzTrackID
				mbReleaseTrack := track.MusicBrainzReleaseTrackID
				mbIDMatch := (mbTrack != "" && mbTrack == r.Track.ForeignTrackID) || (mbReleaseTrack != "" && mbReleaseTrack == r.Track.ForeignTrackID)
				if (mbIDMatch || trackIDMatch) && r.Data.ImportedPath != "" {
					statuses[track.ID] = FileStatus{
					ID:               track.ID,
					Filename:         r.Data.ImportedPath,
					State:            "Succeeded",
					BytesTransferred: 1,
					BytesRemaining:   0,
					PercentComplete:  100,
					QueueID:          "",
					}
				}
			}
		}

		record, ok := queueMap[albumID]
		if !ok {
			continue
		}

		for _, track := range albumTracks {
			if _, e := statuses[track.ID]; e {
				continue
			}
			// assign all tracks in an album the same state
			statuses[track.ID] = FileStatus{
				ID:               track.ID,
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

func (c *Lidarr) getQueue() ([]LidarrQueue, error) {

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
	return queues, nil
}

// Queries Lidarr history for specified album
func (c *Lidarr) getHistory(albumID string) (LidarrHistory, error) {

	req := fmt.Sprintf("/api/v1/history?albumId=%s&pageSize=1111", albumID)
	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+req, nil, c.Headers)
	if err != nil {
		return LidarrHistory{}, err
	}
	var history LidarrHistory
	if err := util.ParseResp(body, &history); err != nil {
		return LidarrHistory{}, err
	}

	return history, nil
}

func (c *Lidarr) getRootDirectory() error {
	// Get the defaults from the root dir
	queryURL := fmt.Sprintf("%s/api/v1/rootfolder", c.Cfg.URL)
	body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, c.Headers)
	if err != nil {
		return fmt.Errorf("failed to lookup root folder: %w", err)
	}

	var rootFolders []RootFolder
	if err = util.ParseResp(body, &rootFolders); err != nil {
		return fmt.Errorf("failed to unmarshal query root folder: %w", err)
	}

	if len(rootFolders) == 0 {
		return fmt.Errorf("no root folders found in Lidarr")
	}
	for _, folder := range rootFolders {
		if folder.Name == c.Cfg.RootFolder {
			c.RootFolder = folder
			return nil
		}
	}
	return fmt.Errorf("no root folder named '%s' found, please create one in Lidarr or point explo to a correct folder", c.Cfg.RootFolder)
}

func (c Lidarr) getReleaseGroupId(track *models.Track) error {
	if track.MusicBrainzReleaseGroupID != "" && track.MusicBrainzArtistID != "" {
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
	track.MusicBrainzArtistID = albums[0].Artist.ForeignArtistID
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

func (c *Lidarr) cacheKey(track *models.Track) string {
    return track.MainArtist + "|" + track.Album
}

func (c *Lidarr) cacheAlbum(track *models.Track) {
	key := c.cacheKey(track)
	c.CacheMu.Lock()
	defer c.CacheMu.Unlock()
    c.AlbumCache[key] = &AlbumMetadata{
        ReleaseGroupMBID: track.MusicBrainzReleaseGroupID,
        ArtistMBID:       track.MusicBrainzArtistID,
        AlbumID:          track.AlbumID,
        ArtistID:         track.MainArtistID,

		Tracks: sync.OnceValue(func() []LidarrTrack {
    		return c.getAlbumTracks(track.AlbumID, track.MainArtistID)
		}),
	}
}
// Checks if album is cached. Populates track fields if it is
func (c *Lidarr) populateFromCache(track *models.Track) bool {
	c.CacheMu.RLock()
    a, ok := c.AlbumCache[c.cacheKey(track)]
	c.CacheMu.RUnlock()

    if !ok {
        return false
    }

    track.MusicBrainzReleaseGroupID = a.ReleaseGroupMBID
    track.MusicBrainzArtistID = a.ArtistMBID
    track.AlbumID = a.AlbumID
    track.MainArtistID = a.ArtistID
    return true
}
