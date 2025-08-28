package downloader

import (
	"explo/src/debug"
	"explo/src/models"
	"fmt"
	"log"
	"strings"
	"time"
)

type Monitor interface {
	GetDownloadStatus([]*models.Track) (map[string]FileStatus, error)
	GetConf() MonitorConfig
	Cleanup(models.Track, string) error
}

/* type DownloadStatusFetcher func() (DownloadStatus, error)
type CleanupFunc func(track *models.Track, fileID string)
type MoveFunc func(from, to, path, file string) error
 */
type MonitorConfig struct {
	CheckInterval   time.Duration
	MonitorDuration time.Duration
	MigrateDownload bool
	FromDir         string
	ToDir           string
	Service			string
}

type FileStatus struct {
	ID               string    `json:"id"`
	Filename         string    `json:"filename"`
	Size             int       `json:"size"`
	State            string    `json:"state"`
	BytesTransferred int       `json:"bytesTransferred"`
	BytesRemaining   int       `json:"bytesRemaining"`
	PercentComplete  float64   `json:"percentComplete"`
}

func (c *DownloadClient) MonitorDownloads(tracks []*models.Track, m Monitor) error {
	var successDownloads int

	progressMap := make(map[string]*DownloadMonitor)
	monCfg := m.GetConf()

	ticker := time.NewTicker(monCfg.CheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		statuses, err := m.GetDownloadStatus(tracks)
		if err != nil {
			log.Printf("Error fetching download status: %s", err.Error())
			continue
		}

		currentTime := time.Now().Local()

		for _, track := range tracks {

			key := fmt.Sprintf("%s|%s", track.ID, track.File)

			if track.Present || track.ID == "" || (progressMap[key] != nil && progressMap[key].Skipped) {
				continue
			}

			// Initialize tracker if not present
			if _, exists := progressMap[key]; !exists {
				progressMap[key] = &DownloadMonitor{
					LastBytesTransferred: 0,
					Counter:              0,
					LastUpdated:          currentTime,
				}
			}
			fileStatus, exists := statuses[track.File]
			tracker := progressMap[key]
			if !exists {
				tracker.Counter++

				if tracker.Counter >= 2 {
					log.Printf("[%s/monitor] %s by %s not found in queue after retries, skipping track", monCfg.Service, track.CleanTitle, track.MainArtist)
					tracker.Skipped = true
				}
				continue
			}

			if fileStatus.BytesRemaining == 0 || fileStatus.PercentComplete == 100 || strings.Contains(fileStatus.State, "Succeeded") {					
				track.Present = true
				log.Printf("[%s/monitor] %s downloaded successfully",monCfg.Service, track.File)
				file, path := parsePath(track.File)
				if monCfg.MigrateDownload {
					if err = moveDownload(monCfg.FromDir, monCfg.ToDir, path, file); err != nil {
						debug.Debug(err.Error())
					} else {
						debug.Debug(fmt.Sprintf("[%s] track moved successfully", monCfg.Service))
					}
				}
				delete(progressMap, key)
				track.File = file
				successDownloads += 1
				if err = m.Cleanup(*track, fileStatus.ID); err != nil {
					debug.Debug(err.Error())
				}
				continue

			} else if fileStatus.BytesTransferred > tracker.LastBytesTransferred {
				tracker.LastBytesTransferred = fileStatus.BytesTransferred
				tracker.LastUpdated = currentTime
				log.Printf("[%s/monitor] progress updated for %s: %d bytes transferred", monCfg.Service, track.File, fileStatus.BytesTransferred)
				continue

			} else if currentTime.Sub(tracker.LastUpdated) > monCfg.MonitorDuration || strings.Contains(fileStatus.State, "Errored") || strings.Contains(fileStatus.State, "Cancelled") {
				log.Printf("[%s/monitor] no progress on %s in %v, skipping track", monCfg.Service, track.File, monCfg.MonitorDuration)
				tracker.Skipped = true
				if err = m.Cleanup(*track, fileStatus.ID); err != nil {
					debug.Debug(err.Error())
				}
				continue
			}
		}
			// Exit condition: all tracks have been processed or skipped
		if tracksProcessed(tracks, progressMap) {
			log.Printf("[%s/monitor] %d out of %d tracks have been downloaded", monCfg.Service, successDownloads, len(tracks))
			return nil
		}
	}
	return nil
}

/* func findFile(statuses []FileStatus, track models.Track) DownloadFiles {
	for _, status := range statuses {
		if userStatus.Username != track.MainArtistID {
			continue
		}
		for _, dir := range userStatus.Directories {
			for _, file := range dir.Files {
				if string(file.Filename) == track.File {
					return file
				}
			}
		}
	}
	return DownloadFiles{}
} */

func tracksProcessed(tracks []*models.Track, progressMap map[string]*DownloadMonitor) bool { // Checks if all tracks are processed (either downloaded or skipped)
	for _, track := range tracks {
		key := fmt.Sprintf("%s|%s", track.ID, track.File)
		tracker, exists := progressMap[key]
		if !track.Present && exists && !tracker.Skipped {
			log.Printf("%s still present", track.File)
			return false
		}
	}
	return true
}