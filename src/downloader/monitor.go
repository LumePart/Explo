package downloader

import (
	"explo/src/debug"
	"explo/src/models"
	"fmt"
	"log"
	"strings"
	"time"
)

type DownloadStatusFetcher func() (DownloadStatus, error)
type CleanupFunc func(track *models.Track, fileID string)
type MoveFunc func(from, to, path, file string) error

type MonitorConfig struct {
	CheckInterval   time.Duration
	MonitorDuration time.Duration
	MigrateDownload bool
	FromDir         string
	ToDir           string
}

func Monitor(tracks []*models.Track,
	fetchStatus DownloadStatusFetcher,
	cleanup CleanupFunc,
	move MoveFunc,
	cfg MonitorConfig) error {
	const checkInterval = 1 * time.Minute
	const monitorDuration = 15 * time.Minute
	var successDownloads int

	progressMap := make(map[string]*DownloadMonitor)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			status, err := fetchStatus()
			if err != nil {
				log.Printf("Error fetching download status: %s", err.Error())
				continue
			}

			currentTime := time.Now().Local()

			for _, track := range tracks {

				key := fmt.Sprintf("%s|%s", track.MainArtistID, track.File)

				if track.Present || track.MainArtistID == "" || (progressMap[key] != nil && progressMap[key].Skipped) {
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

				// Find the corresponding file in the download status
				fileStatus := findFile(status, *track)

				tracker := progressMap[key]
				if fileStatus.Size == 0 {
					tracker.Counter++

					if tracker.Counter >= 2 {
						log.Printf("[monitor] %s by %s not found in queue after retries, skipping track", track.CleanTitle, track.MainArtist)
						tracker.Skipped = true
					}
					continue
				}

				if fileStatus.BytesRemaining == 0 || fileStatus.PercentComplete == 100 || strings.Contains(fileStatus.State, "Succeeded") {
					track.Present = true
					log.Printf("[monitor] %s downloaded successfully", track.File)
					file, path := parsePath(track.File)
					if cfg.MigrateDownload {
						if err = moveDownload(cfg.FromDir, cfg.ToDir, path, file); err != nil {
							debug.Debug(err.Error())
						} else {
							debug.Debug("track moved successfully")
						}
					}
					delete(progressMap, key)
					track.File = file
					successDownloads += 1
					cleanup(track, fileStatus.ID)
					continue

				} else if fileStatus.BytesTransferred > tracker.LastBytesTransferred {
					tracker.LastBytesTransferred = fileStatus.BytesTransferred
					tracker.LastUpdated = currentTime
					log.Printf("[monitor] progress updated for %s: %d bytes transferred", track.File, fileStatus.BytesTransferred)
					continue

				} else if currentTime.Sub(tracker.LastUpdated) > monitorDuration || strings.Contains(fileStatus.State, "Errored") || strings.Contains(fileStatus.State, "Cancelled") {
					log.Printf("[monitor] no progress on %s in %v, skipping track", track.File, monitorDuration)
					tracker.Skipped = true
					cleanup(track, fileStatus.ID)
					continue
				}
			}

			// Exit condition: all tracks have been processed or skipped
			if tracksProcessed(tracks, progressMap) {
				log.Printf("[monitor] %d out of %d tracks have been downloaded", successDownloads, len(tracks))
				return nil
			}
		default:
			continue
		}
	}
}

func findFile(status DownloadStatus, track models.Track) DownloadFiles {
	for _, userStatus := range status {
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
}

func tracksProcessed(tracks []*models.Track, progressMap map[string]*DownloadMonitor) bool { // Checks if all tracks are processed (either downloaded or skipped)
	for _, track := range tracks {
		key := fmt.Sprintf("%s|%s", track.MainArtistID, track.File)
		tracker, exists := progressMap[key]
		if !track.Present && exists && !tracker.Skipped {
			log.Printf("%s still present", track.File)
			return false
		}
	}
	return true
}
