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
	"time"
	"os"
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
    StartedAt            time.Time
}

type Slskd struct {
	Headers     map[string]string
	HttpClient  *util.HttpClient
	DownloadDir string
	Cfg         config.Slskd
}

type SearchPayload struct {
	SearchText string `json:"searchText"`
}

func NewSlskd(cfg config.Slskd, downloadDir string) *Slskd {
	return &Slskd{Cfg: cfg,
		HttpClient:  util.NewHttp(util.HttpClientConfig{Timeout: cfg.Timeout}),
		DownloadDir: downloadDir}
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
		MaxDuration: time.Duration(c.Cfg.MonitorConfig.MaxDuration) * time.Minute,
		MigrateDownload: c.Cfg.MigrateDL,
		ToDir: c.DownloadDir,
		FromDir: c.Cfg.SlskdDir,
		Service: "slskd",
	}, nil
}

var errNoRes = errors.New("no results found for query")

func (c *Slskd) QueryTrack(track *models.Track) error {

	wildcardSearch := false
	trackDetails := fmt.Sprintf("%s - %s", track.CleanTitle, track.Artist)

	retry:
		ID, err := c.searchTrack(trackDetails)
		if err != nil {
			return err
		}
		slog.Info("initiating search", "track", trackDetails)

		cleanup := func() {
    		if err := c.deleteSearch(ID); err != nil {
        		slog.Warn("failed to delete search", "context", err.Error())
    		}
		}

		completed, err := c.searchStatus(ID, trackDetails, 0)
		if errors.Is(err, errNoRes) && !wildcardSearch {
			cleanup()
			wildcardSearch = true
			trackDetails = fmt.Sprintf("%s - %s", track.CleanTitle, wildcardArtist(track.Artist))
			slog.Debug("no result found with artist full name, trying with wildcard", "query", trackDetails)
			goto retry
		}

		if err != nil {
			cleanup()
   	 		return err
		}

		if !completed {
			cleanup()
			return fmt.Errorf("search not completed for %s, skipping track", trackDetails)
		}


		track.ID = ID
		return nil
}

func (c *Slskd) GetTrack(track *models.Track) error {
	results, err := c.searchResults(track.ID)
	if err != nil {
		return err
	}
	files, err := c.CollectFiles(*track, results)
	if err != nil {
		return err
	}
	filterFiles, err := c.filterFiles(files)
	if err != nil {
		return err
	}
	if err := c.queueDownload(filterFiles, track); err != nil {
		return err
	}
	return nil
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

func (c Slskd) searchStatus(ID, trackDetails string, count int) (bool, error) { // Recursive func to see if search for track is finished
	reqParams := fmt.Sprintf("/api/v0/searches/%s", ID)

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParams, nil, c.Headers)
	if err != nil {
		return false, err
	}
	var queryResult Search
	if err := util.ParseResp(body, &queryResult); err != nil {
		return false, err
	}
	if queryResult.IsComplete && queryResult.FileCount > 0 {
		return true, nil
	} else if queryResult.IsComplete && queryResult.FileCount == 0 {
		return false, errNoRes
	} else if queryResult.IsComplete && queryResult.FileCount == queryResult.LockedFileCount {
		return false, fmt.Errorf("search complete, did not find any downloadable files for %s", trackDetails)
	} else if count >= c.Cfg.Retry {
		slog.Debug(fmt.Sprintf("failed to remove %s", ID), logging.RuntimeAttr(""))
		return false, fmt.Errorf("search wasn't completed after %d retries, skipping %s", count, trackDetails)
	}

	slog.Debug(fmt.Sprintf("[%s] (%d/%d) Searching for %s", "slskd", count, c.Cfg.Retry, trackDetails))
	time.Sleep(15 * time.Second)
	return c.searchStatus(ID, trackDetails, count+1)
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

// Collect all files in response that match criteria
func (c Slskd) CollectFiles(track models.Track, searchResults SearchResults) ([]File, error) {
	sanitizedArtist := util.AlnumOnly(track.MainArtist)
	sanitizedAlbum := util.AlnumOnly(track.Album)
	sanitizedTitle := util.AlnumOnly(track.CleanTitle)

	files := slices.Collect(func(yield func(File) bool) {
		for _, result := range searchResults {
			if result.FileCount == 0 || !result.HasFreeUploadSlot {
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

				if !slices.Contains(c.Cfg.Filters.Extensions, file.Extension) || ContainsKeyword(track, file.Name, c.Cfg.Filters.FilterList) {
					continue
				}

				if track.Duration > 0 && util.Abs(track.Duration/1000-file.Length) > 10 { // skip song if track lengths have a 10s+ difference
					continue
				}

				sanitizedFilename := util.AlnumOnly(string(file.Name))
				matchesArtist := containsLower(sanitizedFilename, sanitizedArtist)
				matchesAlbum := containsLower(sanitizedFilename, sanitizedAlbum)
				matchesTitle := containsLower(sanitizedFilename, sanitizedTitle)
				if (matchesArtist || matchesAlbum) && matchesTitle {
					file.Username = result.Username
					if !yield(file) {
							return
					}
				}
			}
		}
	})
	if len(files) == 0 {
		return nil, fmt.Errorf("no tracks passed collection for %s - %s", track.MainArtist, track.CleanTitle)
	} 
	return files, nil
}

func (c Slskd) filterFiles(files []File) ([]File, error) {
	var filtered []File

	for _, ext := range c.Cfg.Filters.Extensions {
		for _, file := range files {
			if file.Extension != ext {
				continue
			}

			if file.BitRate > 0 && file.BitRate < c.Cfg.Filters.MinBitRate {
				continue
			}

			if file.BitDepth > 0 && file.BitDepth < c.Cfg.Filters.MinBitDepth {
				continue
			}

			filtered = append(filtered, file)
			if len(filtered) >= c.Cfg.DownloadAttempts {
				return filtered, nil
			}
		}
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no files found that match filters")
	}
	return filtered, nil
}

func (c Slskd) queueDownload(files []File, track *models.Track) error {
	for i, file := range files {
		reqParams := fmt.Sprintf("/api/v0/transfers/downloads/%s", file.Username)
		payload := []DownloadPayload{
			{
				Filename: file.Name,
				Size:     file.Size,
			},
		}

		DLpayload, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %s", err.Error())
		}

		_, err = c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParams, bytes.NewBuffer(DLpayload), c.Headers)
		if err == nil {
			track.MainArtistID = file.Username
			track.Size = file.Size
			track.File = file.Name
			return nil
		}

		slog.Warn(fmt.Sprintf("[%d/%d] failed to queue download for '%s - %s': %s", i+1, len(files), track.CleanTitle, track.Artist, err.Error()))
		continue
	}
	if err := c.deleteSearch(track.ID); err != nil {
		slog.Debug("failed to delete search", logging.RuntimeAttr(err.Error()))
	}
	return fmt.Errorf("couldn't download track: %s - %s", track.CleanTitle, track.Artist)
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
							ID: file.ID,
							Size: file.Size,
							State: normalize(file.State),
							BytesTransferred: file.BytesTransferred,
							BytesRemaining: file.BytesRemaining,
							PercentComplete: file.PercentComplete,
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
var failureStates = map[string]struct{} {
	"Aborted": {},
	"TimedOut": {},
	"Rejected": {},
	"Errored":  {},
	"Cancelled": {},
}

// return a single error state for failed downloads
func normalize(state string) string{
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


func (c *Slskd) MoveDownload(srcDir, destDir, trackPath string, track *models.Track) error {
	trackDir := filepath.Join(srcDir, trackPath)
	srcFile := filepath.Join(trackDir, track.File)

	if c.Cfg.RenameTrack { // Rename file to {title}-{artist} format
		track.File = getFilename(track.CleanTitle, track.MainArtist) + filepath.Ext(track.File)
	}
	if c.Cfg.OverwriteMetadata {
		metadata := util.BuildffmpegMetadata(*track)
		if err := overwriteMetadata(metadata, srcFile); err != nil {
			slog.Warn("problem overwriting metadata", "msg", err.Error())
		}
	}

	in, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("couldn't open source file: %s", err.Error())
	}

	defer func() {
		if cerr := in.Close(); cerr != nil {
			slog.Error(fmt.Sprintf("failed to close source file: %s", cerr.Error()))
		}
	}()

	var dstFile string
	
	if c.Cfg.PathTemplate != "" {
		relativePath := buildTrackPath(c.Cfg.PathTemplate, track)
		track.File = filepath.Base(relativePath)
		if track.File == "." || track.File == string(filepath.Separator) {
			track.File = getFilename(track.CleanTitle, track.MainArtist) + filepath.Ext(track.File)
			relativePath = filepath.Dir(relativePath) + string(filepath.Separator) + track.File
			slog.Warn(fmt.Sprintf("invalid path template result for track '%s' by '%s', using filename '%s' instead", track.Title, track.Artist, track.File))
		}
		dstFile = filepath.Join(destDir, relativePath)
	} else {
		if err = os.MkdirAll(destDir, os.ModePerm); err != nil {
			return fmt.Errorf("couldn't make download directory: %s", err.Error())
		}

		dstFile = filepath.Join(destDir, track.File)
	}
	if err := moveFile(srcFile, dstFile, c.Cfg.KeepPermissions); err != nil {
			return fmt.Errorf("file move failed: %w", err)
		}

	isEmpty, err := isDirEmpty(trackDir)
	if err != nil {
		return fmt.Errorf("couldn't check if directory is empty: %s", err.Error())
	} else if isEmpty {
		if err = os.Remove(trackDir); err != nil {
			return fmt.Errorf("failed to remove empty directory: %s", err.Error())
		}
	}

	return nil
}