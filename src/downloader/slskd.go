package downloader

import (
	"bytes" // Could be moved to util for all clients
	"encoding/json"
	"explo/src/config"
	"explo/src/debug"
	"explo/src/models"
	"explo/src/util"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type Search struct {
	EndedAt 		time.Time `json:"endedAt"`
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
	FileCount         int     `json:"fileCount"`
	Files             []File `json:"files"`
	HasFreeUploadSlot bool    `json:"hasFreeUploadSlot"`
	LockedFileCount   int     `json:"lockedFileCount"`
	LockedFiles       []any   `json:"lockedFiles"`
	QueueLength       int     `json:"queueLength"`
	Token             int     `json:"token"`
	UploadSpeed       int     `json:"uploadSpeed"`
	Username          string  `json:"username"`
}
type File struct {
	BitRate   int    `json:"bitRate"`
	BitDepth  int    `json:"bitDepth"`
	Code      int    `json:"code"`
	Extension string `json:"extension"`
	Name  json.RawMessage `json:"filename"`
	Length    int    `json:"length"`
	Size      int    `json:"size"`
	IsLocked  bool   `json:"isLocked"`
	Username string // Save user from SearchResults to here during collection
}

type DownloadStatus []struct {
	Username    string        `json:"username"`
	Directories []Directories `json:"directories"`
}
type DownloadFiles struct {
	ID               string    `json:"id"`
	Username         string    `json:"username"`
	Direction        string    `json:"direction"`
	Filename         json.RawMessage    `json:"filename"`
	Size             int       `json:"size"`
	StartOffset      int       `json:"startOffset"`
	State            string    `json:"state"`
	RequestedAt      string    `json:"requestedAt"`
	EnqueuedAt       string    `json:"enqueuedAt"`
	StartedAt        time.Time `json:"startedAt"`
	EndedAt          time.Time `json:"endedAt"`
	BytesTransferred int       `json:"bytesTransferred"`
	AverageSpeed     float64   `json:"averageSpeed"`
	BytesRemaining   int       `json:"bytesRemaining"`
	ElapsedTime      string    `json:"elapsedTime"`
	PercentComplete  float64   `json:"percentComplete"`
	RemainingTime    string    `json:"remainingTime"`
}
type Directories struct {
	Directory string  `json:"directory"`
	FileCount int     `json:"fileCount"`
	Files     []DownloadFiles `json:"files"`
}

type DownloadMonitor struct {
    LastBytesTransferred int
	Counter int
	PlaceInQueue int
	Skipped bool
    LastUpdated time.Time
}

type Slskd struct {
	Headers map[string]string
	HttpClient *util.HttpClient
	Cfg config.Slskd
}

func NewSlskd(cfg config.Slskd) *Slskd {
	return &Slskd{Cfg: cfg,
	HttpClient: util.NewHttp(util.HttpClientConfig{Timeout: cfg.Timeout})}
}

func (c *Slskd) AddHeader() {
	if c.Headers == nil {
		c.Headers = make(map[string]string)
	}
	c.Headers["X-API-Key"] = c.Cfg.APIKey

}

func (c *Slskd) QueryTrack(track *models.Track) error {
	ID, err := c.searchTrack(track)
	if err != nil {
		return err
	}
	trackDetails := fmt.Sprintf("%s - %s", track.CleanTitle, track.Artist)
	log.Printf("initiating search for %s", trackDetails)
	completed, err := c.searchStatus(ID, trackDetails, 0)
	if err != nil {
		if delErr := c.deleteSearch(ID); delErr != nil {
			debug.Debug(delErr.Error())
		}
		return err
	}
	if !completed {
		return fmt.Errorf("search not completed for %s, skipping track", trackDetails)
	}

	track.ID = ID
	results, err := c.searchResults(ID)
	if err != nil {
		return err
	}
	files, err := c.CollectFiles(*track, results)
	if err != nil {
		return err
	}
	file, err := c.filterFiles(files)
	if err != nil {
		return err
	}

	track.File = string(file.Name)
	track.Size = file.Size
	track.MainArtistID = file.Username

	return nil
}

func (c *Slskd) GetTrack(track *models.Track) error {
	if err := c.queueDownload(*track); err != nil {
		return err
	}
	return nil
}

func (c Slskd) searchTrack(track *models.Track) (string, error)  {
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
						file.Extension = strings.TrimPrefix(strings.ToLower(filepath.Ext(string(file.Name))), ".")
					}

					if !slices.Contains(c.Cfg.Filters.Extensions, file.Extension) {
						continue
					}

					if track.Duration > 0 && abs(track.Duration / 1000 - file.Length) > 10 { // skip song if track lengths have a 10s+ difference
						continue
					}

					sanitizedFilename := sanitizeName(string(file.Name))
					if ((containsLower(sanitizedFilename, sanitizedArtist) || containsLower(sanitizedFilename, sanitizedAlbum)) && containsLower(sanitizedFilename, sanitizedTitle)) {
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
		return nil, fmt.Errorf("no tracks passed collection")
	}
}

func (c Slskd) filterFiles(files []File) (File, error) { // return first file that passes checks
	for _, extension := range c.Cfg.Filters.Extensions { // looping the Extensions list allows for priority based filtering (i.e flac before mp3 etc...)
		for _, file := range files {
			if file.Extension != extension { // if extension not matched, skip file
				continue
			}

			if file.BitRate > 0 && file.BitRate < c.Cfg.Filters.MinBitRate {
				continue
			}

			if file.BitDepth > 0 && file.BitDepth < c.Cfg.Filters.MinBitDepth {
				continue
			}

			return file, nil
		}
	}
	return File{}, fmt.Errorf("no files found that match filters")
}

func (c Slskd) queueDownload(track models.Track) error {
	reqParams := fmt.Sprintf("/api/v0/transfers/downloads/%s", track.MainArtistID)

	payload := fmt.Appendf(nil, `[{"filename": %s, "size": %d}]`, track.File, track.Size)

	_, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParams, bytes.NewBuffer(payload), c.Headers)
	if err != nil {
		return fmt.Errorf("failed to queue download: %w", err)
	}
	return nil
}

func (c Slskd) getDownloadStatus() (DownloadStatus, error) {
	reqParams := "/api/v0/transfers/downloads"

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParams, nil, c.Headers)
	if err != nil {
		return nil, err
	}

	var status DownloadStatus
	if err := util.ParseResp(body, &status); err != nil {
		return nil, err
	}
	return status, nil
}

func (c *Slskd) MonitorDownloads(tracks []*models.Track) error {
	const checkInterval = 1 * time.Minute
	const monitorDuration = 15 * time.Minute
	var successDownloads int

	progressMap := make(map[string]*DownloadMonitor)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			status, err := c.getDownloadStatus()
			if err != nil {
				log.Printf("Error fetching download status: %s", err.Error())
				continue
			}

			currentTime := time.Now().Local()

			for _, track := range tracks {


                key := fmt.Sprintf("%s|%s", track.MainArtistID, track.File)

                // Initialize tracker if not present
                if _, exists := progressMap[key]; !exists {
                    progressMap[key] = &DownloadMonitor{
                        LastBytesTransferred: 0,
						Counter: 0,
                        LastUpdated: currentTime,
                    }
                }

				if track.Present || progressMap[key].Skipped {
					continue
				}

                // Find the corresponding file in the status
                var fileStatus DownloadFiles
				Found:
                for _, userStatus := range status {
                    if userStatus.Username != track.MainArtistID {
                        continue
                    }
                    for _, dir := range userStatus.Directories {
                        for _, file := range dir.Files {
                            if string(file.Filename) == track.File {
                                fileStatus = file
                                break Found
                            }
                        }
                    }
                }

				tracker := progressMap[key]
                if fileStatus.Size == 0 {
					tracker.Counter++
				
					if tracker.Counter >= 2 {
						debug.Debug(fmt.Sprintf("[slskd] %s by %s not found in queue after retries, skipping track", track.CleanTitle, track.MainArtist))
						tracker.Skipped = true
					}
					continue
				}

				if fileStatus.BytesRemaining == 0 || fileStatus.PercentComplete == 100 || strings.Contains(fileStatus.State, "Succeeded") {
					track.Present = true
					log.Printf("[slskd] %s downloaded successfully", track.File)
					delete(progressMap, key)
					successDownloads += 1
					continue

				} else if fileStatus.BytesTransferred > tracker.LastBytesTransferred {
                    tracker.LastBytesTransferred = fileStatus.BytesTransferred
                    tracker.LastUpdated = currentTime
                	debug.Debug(fmt.Sprintf("[slskd] progress updated for %s: %d bytes transferred", track.File, fileStatus.BytesTransferred))
					continue

                } else if currentTime.Sub(tracker.LastUpdated) > monitorDuration || strings.Contains(fileStatus.State, "Errored") {
                    log.Printf("[slskd] %s failed to download, skipping track", track.File)
					tracker.Skipped = true
					if err = c.deleteSearch(track.ID); err != nil {
						debug.Debug(fmt.Sprintf("failed to delete search request: %s", err.Error()))
					}

					if err = c.deleteDownload(track.MainArtistID, track.ID); err != nil {
						debug.Debug(fmt.Sprintf("failed to delete download: %s", err.Error()))
					}
					continue
                }
            }

			// Check if all tracks have been processed
			allDone := true
			for _, track := range tracks {
				key := fmt.Sprintf("%s|%s", track.MainArtistID, track.File)
				tracker, exists := progressMap[key]
				if !track.Present && (!exists || !tracker.Skipped) {
					allDone = false
					break
				}
			}
            // Exit condition: all tracks have been processed or skipped
            if allDone {
				log.Printf("[slskd] %d out of %d tracks have been downloaded", successDownloads, len(tracks))
				return nil
			}
        }
    }
}

func (c Slskd) deleteDownload(user, ID string) error {
	reqParams := fmt.Sprintf("/api/v0/transfers/downloads/%s/%s?remove=true", user, ID)

	_, err := c.HttpClient.MakeRequest("DELETE", c.Cfg.URL+reqParams, nil, c.Headers)
	if err != nil {
		return err
	}
	return nil
}

func abs(x int) int { // Helper track to return absolute difference between tracks
	if x < 0 {
		return -x
	}
	return x
}



