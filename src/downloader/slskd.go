package downloader

import (
	"bytes" // Could be moved to util for all clients
	"encoding/json"
	"errors"
	"explo/src/config"
	"explo/src/logging"
	"explo/src/models"
	"explo/src/util"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type Search struct {
	EndedAt         time.Time `json:"endedAt"`
	FileCount       int       `json:"fileCount"`
	ID              string    `json:"id"`
	IsComplete      bool      `json:"isComplete"`
	LockedFileCount int       `json:"lockedFileCount"`
	ResponseCount   int       `json:"responseCount"`
	SearchText      string    `json:"searchText"`
	StartedAt       time.Time `json:"startedAt"`
	State           string    `json:"state"`
	Token           int       `json:"token"`
}

type SearchResults []struct {
	FileCount         int    `json:"fileCount"`
	Files             []File `json:"files"`
	HasFreeUploadSlot bool   `json:"hasFreeUploadSlot"`
	LockedFileCount   int    `json:"lockedFileCount"`
	LockedFiles       []any  `json:"lockedFiles"`
	QueueLength       int    `json:"queueLength"`
	Token             int    `json:"token"`
	UploadSpeed       int    `json:"uploadSpeed"`
	Username          string `json:"username"`
}
type File struct {
	BitRate   int    `json:"bitRate"`
	BitDepth  int    `json:"bitDepth"`
	Code      int    `json:"code"`
	Extension string `json:"extension"`
	Name      string `json:"filename"`
	Length    int    `json:"length"`
	Size      int    `json:"size"`
	IsLocked  bool   `json:"isLocked"`
	Username  string // Save user from SearchResults to here during collection
}

type DownloadPayload struct {
	Filename string `json:"filename"`
	Size     int    `json:"size"`
}

type DownloadStatus []struct {
	Username    string        `json:"username"`
	Directories []Directories `json:"directories"`
}
type DownloadFiles struct {
	ID               string          `json:"id"`
	Username         string          `json:"username"`
	Direction        string          `json:"direction"`
	Name             string 		 `json:"filename"`
	Size             int             `json:"size"`
	StartOffset      int             `json:"startOffset"`
	State            string          `json:"state"`
	RequestedAt      string          `json:"requestedAt"`
	EnqueuedAt       string          `json:"enqueuedAt"`
	StartedAt        time.Time       `json:"startedAt"`
	EndedAt          time.Time       `json:"endedAt"`
	BytesTransferred int             `json:"bytesTransferred"`
	AverageSpeed     float64         `json:"averageSpeed"`
	BytesRemaining   int             `json:"bytesRemaining"`
	ElapsedTime      string          `json:"elapsedTime"`
	PercentComplete  float64         `json:"percentComplete"`
	RemainingTime    string          `json:"remainingTime"`
}
type Directories struct {
	Directory string          `json:"directory"`
	FileCount int             `json:"fileCount"`
	Files     []DownloadFiles `json:"files"`
}

type DownloadMonitor struct {
	LastBytesTransferred int
	Counter              int
	PlaceInQueue         int
	Skipped              bool
	LastUpdated          time.Time
}

type Slskd struct {
	Headers     map[string]string
	HttpClient  *util.HttpClient
	DownloadDir string
	Cfg         config.Slskd
	retry       *retryState
}

type retryState struct {
	mu        sync.Mutex
	remaining map[string][]File // trackID -> untried ranked candidates
}

type SearchPayload struct {
	SearchText string `json:"searchText"`
}

func NewSlskd(cfg config.Slskd, downloadDir string) *Slskd {
	return &Slskd{Cfg: cfg,
		HttpClient:  util.NewHttp(util.HttpClientConfig{Timeout: cfg.Timeout}),
		DownloadDir: downloadDir,
		retry:       &retryState{remaining: make(map[string][]File)}}
}

func (c *Slskd) AddHeader() {
	if c.Headers == nil {
		c.Headers = make(map[string]string)
	}
	c.Headers["X-API-Key"] = c.Cfg.APIKey

}

func (c *Slskd) GetConf() (MonitorConfig, error) {
	return  MonitorConfig{
		CheckInterval: time.Duration(c.Cfg.MonitorConfig.Interval) * time.Minute,
		MonitorDuration: time.Duration(c.Cfg.MonitorConfig.Duration) * time.Minute,
		MigrateDownload: c.Cfg.MigrateDL,
		ToDir: c.DownloadDir,
		FromDir: c.Cfg.SlskdDir,
		Service: "slskd",
	}, nil
}

var errNoRes = errors.New("no results found for query")

func (c *Slskd) QueryTrack(track *models.Track) error {
	queries := c.searchQueries(track)

	var lastErr error
	for _, q := range queries {
		ID, err := c.searchTrack(q)
		if err != nil {
			return err
		}
		slog.Info("initiating search", "track", q)

		completed, err := c.searchStatus(ID, q)
		if err == nil && completed {
			_, err = c.fetchCollectableFiles(track, ID)
			if err == nil {
				track.ID = ID
				return nil
			}
		}
		lastErr = err

		if delErr := c.deleteSearch(ID); delErr != nil {
			slog.Warn("failed to delete search", "context", delErr.Error())
		}
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("no downloadable results for %s - %s", track.CleanTitle, track.Artist)
}

func (c *Slskd) searchQueries(track *models.Track) []string {
	var queries []string
	seen := make(map[string]bool)
	add := func(q string) {
		q = strings.TrimSpace(q)
		if q == "" || seen[strings.ToLower(q)] {
			return
		}
		seen[strings.ToLower(q)] = true
		queries = append(queries, q)
	}

	add(fmt.Sprintf("%s - %s", track.CleanTitle, track.Artist))
	var names []string
	for _, n := range splitArtists(track.MainArtist) {
		if len([]rune(n)) >= 2 {
			names = append(names, n)
		}
	}
	if len(names) > 2 {
		names = names[:2]
	}
	for _, name := range names {
		add(fmt.Sprintf("%s %s", track.CleanTitle, name))
	}
	add(fmt.Sprintf("%s - %s", track.CleanTitle, wildcardArtist(track.Artist)))
	return queries
}

func splitArtists(artist string) []string {
	norm := artist
	for _, sep := range []string{" featuring ", " feat.", " feat ", " ft.", " ft ", " with ", " & ", " x ", ",", "/", "+", "&"} {
		norm = strings.ReplaceAll(norm, sep, "|")
	}
	var parts []string
	for _, part := range strings.Split(norm, "|") {
		if p := strings.TrimSpace(part); p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func (c *Slskd) GetTrack(track *models.Track) error {
	files, err := c.fetchCollectableFiles(track, track.ID)
	if err != nil {
		return err
	}
	if err := c.queueDownload(files, track); err != nil {
		return err
	}
	return nil
}

func (c *Slskd) fetchCollectableFiles(track *models.Track, searchID string) ([]File, error) {
	var results SearchResults
	for attempt := 0; attempt < 4; attempt++ {
		r, err := c.searchResults(searchID)
		if err != nil {
			return nil, err
		}
		results = r
		total := 0
		for _, res := range results {
			total += len(res.Files)
		}
		if total > 0 {
			break
		}
		time.Sleep(3 * time.Second)
	}
	return c.CollectFiles(*track, results)
}

func (c Slskd) searchTrack(trackDetails string) (string, error) {
	reqParams := "/api/v0/searches"

	payloadStr := SearchPayload{
		SearchText: trackDetails,
	}
	payload, err := json.Marshal(payloadStr)
	if err != nil {
		return "", fmt.Errorf("failed to marshal search payload %w", err)
	}

	body, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParams, bytes.NewReader(payload), c.Headers)
	if err != nil {
		return "", err
	}
	var queryResult Search
	if err := util.ParseResp(body, &queryResult); err != nil {
		return "", err
	}
	return queryResult.ID, nil
}

func (c Slskd) searchStatus(ID, trackDetails string) (bool, error) {
	reqParams := fmt.Sprintf("/api/v0/searches/%s", ID)

	const pollInterval = 3 * time.Second

	maxWait := time.Duration(c.Cfg.Retry) * 15 * time.Second
	if maxWait < 90*time.Second {
		maxWait = 90 * time.Second
	}
	deadline := time.Now().Add(maxWait)

	for {
		body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParams, nil, c.Headers)
		if err != nil {
			return false, err
		}
		var queryResult Search
		if err := util.ParseResp(body, &queryResult); err != nil {
			return false, err
		}

		downloadable := queryResult.FileCount - queryResult.LockedFileCount

		if queryResult.IsComplete {
			if downloadable > 0 {
				return true, nil
			}
			if queryResult.FileCount == 0 {
				return false, errNoRes
			}
			return false, fmt.Errorf("search complete, did not find any downloadable files for %s", trackDetails)
		}

		if time.Now().After(deadline) {
			if downloadable > 0 {
				return true, nil
			}
			return false, fmt.Errorf("search wasn't completed within %s, skipping %s", maxWait, trackDetails)
		}

		time.Sleep(pollInterval)
	}
}

func (c Slskd) searchResults(ID string) (SearchResults, error) {
	reqParams := fmt.Sprintf("/api/v0/searches/%s/responses", ID)

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParams, nil, c.Headers)
	if err != nil {
		return nil, err
	}
	var results SearchResults
	if err = util.ParseResp(body, &results); err != nil {
		return nil, err
	}

	return results, nil
}

func (c Slskd) deleteSearch(ID string) error {
	reqParams := fmt.Sprintf("/api/v0/searches/%s", ID)

	_, err := c.HttpClient.MakeRequest("DELETE", c.Cfg.URL+reqParams, nil, c.Headers)
	if err != nil {
		return err
	}
	return nil
}

func (c Slskd) CollectFiles(track models.Track, searchResults SearchResults) ([]File, error) {
	sanitizedTitle := util.AlnumOnly(track.CleanTitle)
	artistTokens := artistMatchTokens(track.MainArtist)

	sanitizedAlbum := util.AlnumOnly(track.Album)
	if sanitizedAlbum == sanitizedTitle || len([]rune(sanitizedAlbum)) < 4 {
		sanitizedAlbum = ""
	}

	extRank := make(map[string]int, len(c.Cfg.Filters.Extensions))
	for i, ext := range c.Cfg.Filters.Extensions {
		extRank[ext] = i
	}

	type candidate struct {
		file     File
		freeSlot bool
		rank     int
	}
	var candidates []candidate

	for _, result := range searchResults {
		if result.FileCount == 0 {
			continue
		}
		for _, file := range result.Files {
			nameExt := util.AlnumOnly(strings.TrimPrefix(strings.ToLower(filepath.Ext(string(file.Name))), "."))
			reportedExt := strings.TrimPrefix(strings.ToLower(file.Extension), ".")
			if nameExt != "" {
				file.Extension = nameExt
			} else {
				file.Extension = reportedExt
			}

			rank, ok := extRank[file.Extension]
			if !ok || ContainsKeyword(track, file.Name, c.Cfg.Filters.FilterList) {
				continue
			}

			if track.Duration > 0 && util.Abs(track.Duration/1000-file.Length) > 10 { // skip song if track lengths have a 10s+ difference
				continue
			}

			if file.BitRate > 0 && file.BitRate < c.Cfg.Filters.MinBitRate {
				continue
			}
			if file.BitDepth > 0 && file.BitDepth < c.Cfg.Filters.MinBitDepth {
				continue
			}

			if !matchesTrack(file, sanitizedTitle, sanitizedAlbum, artistTokens) {
				continue
			}

			file.Username = result.Username
			candidates = append(candidates, candidate{file: file, freeSlot: result.HasFreeUploadSlot, rank: rank})
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no tracks passed collection for %s - %s", track.MainArtist, track.CleanTitle)
	}

	slices.SortStableFunc(candidates, func(a, b candidate) int {
		if a.freeSlot != b.freeSlot {
			if a.freeSlot {
				return -1
			}
			return 1
		}
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		return b.file.BitRate - a.file.BitRate
	})

	files := make([]File, 0, len(candidates))
	for _, cand := range candidates {
		files = append(files, cand.file)
		if len(files) >= c.Cfg.DownloadAttempts {
			break
		}
	}
	return files, nil
}

func matchesTrack(file File, sanitizedTitle, sanitizedAlbum string, artistTokens []string) bool {
	base := filepath.Base(strings.ReplaceAll(string(file.Name), `\`, `/`))
	if !containsLower(util.AlnumOnly(base), sanitizedTitle) {
		return false
	}
	sanitizedFilename := util.AlnumOnly(string(file.Name))
	if sanitizedAlbum != "" && containsLower(sanitizedFilename, sanitizedAlbum) {
		return true
	}
	for _, tok := range artistTokens {
		if containsLower(sanitizedFilename, tok) {
			return true
		}
	}
	return false
}

func artistMatchTokens(artist string) []string {
	seen := make(map[string]struct{})
	var tokens []string
	for _, part := range splitArtists(strings.ToLower(artist)) {
		tok := util.AlnumOnly(part)
		if len(tok) < 3 {
			continue
		}
		if _, dup := seen[tok]; dup {
			continue
		}
		seen[tok] = struct{}{}
		tokens = append(tokens, tok)
	}
	if len(tokens) == 0 {
		if tok := util.AlnumOnly(artist); tok != "" {
			tokens = append(tokens, tok)
		}
	}
	return tokens
}

func (c Slskd) queueDownload(files []File, track *models.Track) error {
	for i, file := range files {
		if err := c.queueFile(file, track); err != nil {
			slog.Warn(fmt.Sprintf("[%d/%d] failed to queue download for '%s - %s': %s", i+1, len(files), track.CleanTitle, track.Artist, err.Error()))
			continue
		}
		c.retry.mu.Lock()
		c.retry.remaining[track.ID] = files[i+1:]
		c.retry.mu.Unlock()
		return nil
	}
	if err := c.deleteSearch(track.ID); err != nil {
		slog.Debug("failed to delete search", logging.RuntimeAttr(err.Error()))
	}
	return fmt.Errorf("couldn't download track: %s - %s", track.CleanTitle, track.Artist)
}

// queueFile asks slskd to download one file and records it on the track.
func (c Slskd) queueFile(file File, track *models.Track) error {
	payload, err := json.Marshal([]DownloadPayload{{Filename: file.Name, Size: file.Size}})
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %s", err.Error())
	}
	reqParams := fmt.Sprintf("/api/v0/transfers/downloads/%s", file.Username)
	if _, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParams, bytes.NewBuffer(payload), c.Headers); err != nil {
		return err
	}
	track.MainArtistID = file.Username
	track.Size = file.Size
	track.File = file.Name
	return nil
}

// RetryDownload queues the next untried candidate, or returns false if none remain.
func (c *Slskd) RetryDownload(track *models.Track) (bool, error) {
	c.retry.mu.Lock()
	files := c.retry.remaining[track.ID]
	c.retry.mu.Unlock()

	for i, file := range files {
		if err := c.queueFile(file, track); err != nil {
			continue
		}
		c.retry.mu.Lock()
		c.retry.remaining[track.ID] = files[i+1:]
		c.retry.mu.Unlock()
		slog.Info("[slskd] retrying with next source", "track", track.CleanTitle, "file", file.Name)
		return true, nil
	}
	c.retry.mu.Lock()
	delete(c.retry.remaining, track.ID)
	c.retry.mu.Unlock()
	return false, nil
}

func (c *Slskd) GetDownloadStatus(tracks []*models.Track) (map[string]FileStatus, error) {
	reqParams := "/api/v0/transfers/downloads"
	fileStatuses := make(map[string]FileStatus, len(tracks))
	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParams, nil, c.Headers)
	if err != nil {
		return nil, err
	}

	var statuses DownloadStatus
	if err := util.ParseResp(body, &statuses); err != nil {
		return nil, err
	}
	for _, status := range statuses {
		for _, track := range tracks {
			if status.Username != track.MainArtistID {
				continue
			}

			for _, dir := range status.Directories {
				for _, file := range dir.Files {
					if string(file.Name) == track.File {
						fileStatuses[track.File] = FileStatus{
							ID:               file.ID,
							Size:             file.Size,
							State:            normalize(file.State),
							BytesTransferred: file.BytesTransferred,
							BytesRemaining:   file.BytesRemaining,
							PercentComplete:  file.PercentComplete,
						}
					}
				}
			}
		}
	}
	if len(fileStatuses) != 0 {
		return fileStatuses, nil
	}
	return nil, fmt.Errorf("no files found to monitor")
}

func (c Slskd) deleteDownload(user, ID string) error {
	reqParams := fmt.Sprintf("/api/v0/transfers/downloads/%s/%s", user, ID)

	// cancel download
	if _, err := c.HttpClient.MakeRequest("DELETE", c.Cfg.URL+reqParams+"?remove=false", nil, c.Headers); err != nil {
		return fmt.Errorf("soft delete failed: %s", err.Error())
	}
	time.Sleep(1 * time.Second) // Small buffer between soft and hard delete
	// delete download
	if _, err := c.HttpClient.MakeRequest("DELETE", c.Cfg.URL+reqParams+"?remove=true", nil, c.Headers); err != nil {
		return fmt.Errorf("hard delete failed: %s", err.Error())
	}

	return nil
}

func (c *Slskd) Cleanup(track models.Track, fileID string) error {
	if err := c.deleteSearch(track.ID); err != nil {
		slog.Debug("failed to delete search request", logging.RuntimeAttr(err.Error()))
	}
	if err := c.deleteDownload(track.MainArtistID, fileID); err != nil {
		slog.Debug("failed to delete download", logging.RuntimeAttr(err.Error()))
	}
	return nil
}

func parsePath(p string) (string, string) { // parse filepath to downloaded format, return filename and parent dir
	p = strings.ReplaceAll(p, `\`, `/`)
	return filepath.Base(p), filepath.Base(filepath.Dir(p))

}

func wildcardArtist(artist string) string {
	prefix := ""
	if len(artist) >= 4 && strings.EqualFold(artist[:4], "the ") {
		prefix = artist[:4]
		artist = strings.TrimSpace(artist[4:])
	}
	r := []rune(strings.TrimSpace(artist))

	if len(r) < 3 {
		return artist
	}

	r[0] = '*'
	return prefix + string(r)
}

// different failure states slskd has (format is "Completed,Rejected", "Errored,Cancelled" etc..)
var failureStates = map[string]struct{}{
	"Aborted":   {},
	"TimedOut":  {},
	"Rejected":  {},
	"Errored":   {},
	"Cancelled": {},
}

// return a single error state for failed downloads
func normalize(state string) string {
	parts := strings.SplitSeq(state, ",")

	for p := range parts {
		p = strings.TrimSpace(p)
		if _, ok := failureStates[p]; ok {
			slog.Debug("[slskd] download failed", "status", state)
			return "Errored"
		}
	}
	return state
}
