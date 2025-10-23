package downloader

import (
	"explo/src/debug"
	"explo/src/models"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type Monitor interface {
	GetDownloadStatus([]*models.Track) (map[string]FileStatus, error)
	GetConf() (MonitorConfig, error)
	Cleanup(models.Track, string) error
}

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
					slog.Info("[monitor] track not found in queue after retries, skipping", "service", monCfg.Service,"track title", track.CleanTitle, "track artist", track.MainArtist)
					tracker.Skipped = true
				}
				continue
			}

			if fileStatus.BytesRemaining == 0 || fileStatus.PercentComplete == 100 || strings.Contains(fileStatus.State, "Succeeded") {					
				track.Present = true
				slog.Info("[monitor] file downloaded successfully", "service", monCfg.Service, "file", track.File)
				file, path := parsePath(track.File)
				if monCfg.MigrateDownload {
					if err = moveDownload(monCfg.FromDir, monCfg.ToDir, path, file); err != nil {
						slog.Debug("error while moving file", debug.RuntimeAttr(err.Error()))
					} else {
						slog.Info("track moved successfully", "service", monCfg.Service)
					}
				}
				delete(progressMap, key)
				track.File = file
				successDownloads += 1
				if err = m.Cleanup(*track, fileStatus.ID); err != nil {
					slog.Debug("cleanup failed", debug.RuntimeAttr(err.Error()))
				}
				continue

			} else if fileStatus.BytesTransferred > tracker.LastBytesTransferred {
				tracker.LastBytesTransferred = fileStatus.BytesTransferred
				tracker.LastUpdated = currentTime
				slog.Info("[monitor] progress updated", "service", monCfg.Service, "file", track.File, "bytes transferred", fileStatus.BytesTransferred)
				continue

			} else if currentTime.Sub(tracker.LastUpdated) > monCfg.MonitorDuration || strings.Contains(fileStatus.State, "Errored") || strings.Contains(fileStatus.State, "Cancelled") {
				slog.Info("[monitor] no download progress for file, skipping", "service", monCfg.Service, "file", track.File, "duration", monCfg.MonitorDuration)
				tracker.Skipped = true
				if err = m.Cleanup(*track, fileStatus.ID); err != nil {
					slog.Debug("cleanup failed", debug.RuntimeAttr(err.Error()))
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

func tracksProcessed(tracks []*models.Track, progressMap map[string]*DownloadMonitor) bool { // Checks if all tracks are processed (either downloaded or skipped)
	for _, track := range tracks {
		key := fmt.Sprintf("%s|%s", track.ID, track.File)
		tracker, exists := progressMap[key]
		if !track.Present && exists && !tracker.Skipped {
			slog.Info("file still present", "file", track.File)
			return false
		}
	}
	return true
}