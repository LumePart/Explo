package downloader

import (
	"explo/src/logging"
	"explo/src/models"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type Monitor interface {
	GetDownloadStatus([]*models.Track) (map[string]FileStatus, error)
	GetConf() (MonitorConfig, error)
	MoveDownload(string, string, string, *models.Track) error
	Cleanup(models.Track, string) error
}

type MonitorConfig struct {
	CheckInterval   time.Duration
	StallDuration time.Duration
	MaxDuration time.Duration
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
	QueueID 		 string    `json:"queueID"`
}

func (c *DownloadClient) MonitorDownloads(tracks []*models.Track, m Monitor) error {
	var successDownloads int

	progressMap := make(map[string]*DownloadMonitor)
	monCfg, err := m.GetConf()
	if err != nil {
		return err
	}

	ticker := time.NewTicker(monCfg.CheckInterval)

	defer ticker.Stop()

	for range ticker.C {
		statuses, err := m.GetDownloadStatus(tracks)
		if err != nil {
			return fmt.Errorf("[%s/monitor] error fetching download status: %s", monCfg.Service, err.Error())
		}
		slog.Debug("fetched download queue", "size", len(statuses))

		currentTime := time.Now().Local()

		for _, track := range tracks {

			key := fmt.Sprintf("%s|%s", track.ID, track.CleanTitle)

			if track.Present || track.ID == "" || (progressMap[key] != nil && progressMap[key].Skipped) {
				continue
			}

			// Initialize tracker if not present
			if _, exists := progressMap[key]; !exists {
				progressMap[key] = &DownloadMonitor{
					LastBytesTransferred: 0,
					Counter:              0,
					LastUpdated:          currentTime,
					StartedAt:            currentTime, 
				}
			}
			fileStatus, exists := statuses[track.ID]
			tracker := progressMap[key]
			if !exists {
				tracker.Counter++
				if tracker.Counter >= 2 {
					slog.Info("[monitor] track not found in queue after retries, skipping", "service", monCfg.Service,"track title", track.CleanTitle, "track artist", track.MainArtist)
					tracker.Skipped = true
				}
				continue
			} 
			tracker.Counter = 0

			stallTime := currentTime.Sub(tracker.LastUpdated)
			monitoredTime := currentTime.Sub(tracker.StartedAt)

			if (fileStatus.BytesRemaining == 0 && fileStatus.BytesTransferred != 0) || fileStatus.PercentComplete == 100 || strings.Contains(fileStatus.State, "Succeeded") {		
				track.File = fileStatus.Filename
				track.Present = true
				slog.Info("[monitor] file downloaded successfully", "service", monCfg.Service, "file", track.File)
				var filePath string
				track.File, filePath = parsePath(track.File)
				if monCfg.MigrateDownload {
					if err = m.MoveDownload(monCfg.FromDir, monCfg.ToDir, filePath, track); err != nil {
						slog.Error("error while moving file", "err", err)
					} else {
						slog.Info("track moved successfully", "service", monCfg.Service)
					}
				}
				delete(progressMap, key)
				successDownloads += 1
				if err = m.Cleanup(*track, fileStatus.QueueID); err != nil {
					slog.Debug("cleanup failed", logging.RuntimeAttr(err.Error()))
				}
				continue

			} else if fileStatus.BytesTransferred > tracker.LastBytesTransferred {
				tracker.LastBytesTransferred = fileStatus.BytesTransferred
				tracker.LastUpdated = currentTime
				slog.Info("[monitor] progress updated", "service", monCfg.Service, "title", track.CleanTitle, "bytes transferred", fileStatus.BytesTransferred)
				continue

			} else if fileStatus.State == "Errored" ||
				stallTime > monCfg.StallDuration ||
				monitoredTime > monCfg.MaxDuration {

				switch {
				case fileStatus.State == "Errored":
					slog.Info("[monitor] download errored",
						"service", monCfg.Service,
						"title", track.CleanTitle,
					)
				case stallTime > monCfg.StallDuration:
					slog.Info("[monitor] download stalled",
						"service", monCfg.Service,
						"title", track.CleanTitle,
						"duration", stallTime,
					)
				default:
					slog.Info("[monitor] maximum monitor time exceeded",
						"service", monCfg.Service,
						"title", track.CleanTitle,
						"duration", monitoredTime,
					)
				}

				tracker.Skipped = true
				if err = m.Cleanup(*track, fileStatus.QueueID); err != nil {
					slog.Debug("cleanup failed", logging.RuntimeAttr(err.Error()))
				}
				continue
			}
		}
			// Exit condition: all tracks have been processed or skipped
		if tracksProcessed(tracks, progressMap) {
			slog.Info("[monitor] Finished", "service", monCfg.Service, "downloaded files", successDownloads, "total tracks", len(tracks))
			return nil
		}
	}
	return nil
}

// Checks if all tracks are processed (either downloaded or skipped)
func tracksProcessed(tracks []*models.Track, progressMap map[string]*DownloadMonitor) bool {
	for _, track := range tracks {
		key := fmt.Sprintf("%s|%s", track.ID, track.CleanTitle)
		tracker, exists := progressMap[key]
		if !track.Present && exists && !tracker.Skipped {
			slog.Info("[monitor] track download still in progress", "title", track.CleanTitle, "artist", track.MainArtist)
			return false
		}
	}
	return true
}