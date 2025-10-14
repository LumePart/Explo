package downloader

import (
	"bytes" // Could be moved to util for all clients
	"encoding/json"
	"explo/src/config"
	"explo/src/debug"
	"explo/src/models"
	"explo/src/util"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
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
		CheckInterval: c.Cfg.MonitorConfig.Interval,
		MonitorDuration: c.Cfg.MonitorConfig.Duration,
		MigrateDownload: c.Cfg.MigrateDL,
		ToDir: c.DownloadDir,
		FromDir: c.Cfg.SlskdDir,
		Service: "slskd",
	}, nil
}

func (c *Slskd) QueryTrack(track *models.Track) error {
	ID, err := c.searchTrack(track)
	if err != nil {
		return err
	}
	trackDetails := fmt.Sprintf("%s - %s", track.CleanTitle, track.Artist)
	slog.Info("initiating search", "track", trackDetails)

	defer func() { // Delete search if ID is empty
		if track.ID == "" {
			if delErr := c.deleteSearch(ID); delErr != nil {
				slog.Warn("failed to delete search", "service", "slskd", "context", delErr.Error())
			}
		}
	}()

	completed, err := c.searchStatus(ID, trackDetails, 0)
	if err != nil {
		return err
	}
	if !completed {
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

func (c Slskd) searchTrack(track *models.Track) (string, error) {
	reqParams := "/api/v0/searches"

	payload := fmt.Appendf(nil, `{"searchText": "%s - %s"}`, track.CleanTitle, track.Artist)

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
	} else if queryResult.IsComplete && (queryResult.FileCount == 0 || queryResult.FileCount == queryResult.LockedFileCount) {
		return false, fmt.Errorf("search complete, did not find any available files for %s", trackDetails)
	} else if count >= c.Cfg.Retry {
		debug.Debug(fmt.Sprintf("search not completed for ID: %s", ID))
		return false, fmt.Errorf("search wasn't completed after %d retries, skipping %s", count, trackDetails)
	}

	debug.Debug(fmt.Sprintf("[%d/%d] Searching for %s", count, c.Cfg.Retry, trackDetails))
	time.Sleep(20 * time.Second)
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

func (c Slskd) CollectFiles(track models.Track, searchResults SearchResults) ([]File, error) { // Collect all files in response that match criteria
	sanitizedArtist := sanitizeName(track.MainArtist)
	sanitizedAlbum := sanitizeName(track.Album)
	sanitizedTitle := sanitizeName(track.CleanTitle)

	files := slices.Collect(func(yield func(File) bool) {
		for _, result := range searchResults {
			if result.FileCount > 0 && result.HasFreeUploadSlot {
				for _, file := range result.Files {
					file.Extension = strings.TrimPrefix(strings.ToLower(file.Extension), ".")
					if file.Extension == "" {
						extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(string(file.Name))), ".")
						file.Extension = sanitizeName(extension) // sanitize extension incase of bad chars
					}

					if !slices.Contains(c.Cfg.Filters.Extensions, file.Extension) {
						continue
					}

					if track.Duration > 0 && util.Abs(track.Duration/1000-file.Length) > 10 { // skip song if track lengths have a 10s+ difference
						continue
					}

					sanitizedFilename := sanitizeName(string(file.Name))
					if (containsLower(sanitizedFilename, sanitizedArtist) || containsLower(sanitizedFilename, sanitizedAlbum)) && containsLower(sanitizedFilename, sanitizedTitle) {
						file.Username = result.Username
						if !yield(file) {
							return
						}
					}
				}
			}
		}
	})
	if len(files) != 0 {
		return files, nil
	} else {
		return nil, fmt.Errorf("no tracks passed collection for %s - %s", track.MainArtist, track.CleanTitle)
	}
}

func (c Slskd) filterFiles(files []File) ([]File, error) {
	var filtered []File

	for _, ext := range c.Cfg.Filters.Extensions {
		for _, file := range files {
			if file.Extension != ext {
				continue
			}

			if file.BitRate > 0 && file.BitRate <= c.Cfg.Filters.MinBitRate {
				continue
			}

			if file.BitDepth > 0 && file.BitDepth <= c.Cfg.Filters.MinBitDepth {
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
		debug.Debug(fmt.Sprintf("failed to delete search: %s", err.Error()))
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
							State: file.State,
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
		debug.Debug(fmt.Sprintf("failed to delete search request: %v", err))
	}
	if err := c.deleteDownload(track.MainArtistID, fileID); err != nil {
		debug.Debug(fmt.Sprintf("failed to delete download: %v", err))
	}
	return nil
}

func parsePath(p string) (string, string) { // parse filepath to downloaded format, return filename and parent dir
	p = strings.ReplaceAll(p, `\`, `/`)
	return filepath.Base(p), filepath.Base(filepath.Dir(p))

}
