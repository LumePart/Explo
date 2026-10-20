package downloader

import (
	"explo/src/logging"
	"explo/src/models"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
)

// maxPollFailures is how many consecutive status polls may fail before the
// monitor gives up. Anything less than the whole run is worth surviving: the
// downloads themselves keep going, and only the monitor loses sight of them.
const maxPollFailures = 3

type Monitor interface {
	GetDownloadStatus([]*models.Track) (map[string]FileStatus, error)
	GetConf() (MonitorConfig, error)
	// MoveDownload migrates a finished track into the library and returns the
	// directory it landed in, so album siblings can follow it there.
	MoveDownload(string, string, string, *models.Track) (string, error)
	Cleanup(models.Track, string) error
}

type MonitorConfig struct {
	CheckInterval   time.Duration
	StallDuration time.Duration
	MaxDuration time.Duration
	// FallbackAfter bounds the whole monitor phase for one provider. Once it
	// elapses, any track still unresolved is abandoned so the next provider in
	// DOWNLOAD_SERVICES can attempt it. Zero means wait indefinitely, which is
	// the historical behaviour.
	FallbackAfter   time.Duration
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
	// RawState is the provider's original status string. slskd's
	// normalize collapses every failure into "Errored", so without
	// this the log can never say how a download failed.
	RawState string `json:"rawState,omitempty"`
}

// rawState prefers the provider's original status for log lines.
func rawState(fs FileStatus) string {
	if fs.RawState != "" {
		return fs.RawState
	}
	return fs.State
}

// pendingAlbum is a release whose recommended track has already been migrated
// but whose remaining files are still downloading. The monitor revisits these
// each tick, because the recommended track is queued first and so normally
// finishes well ahead of its siblings.
type pendingAlbum struct {
	track    *models.Track
	trackDir string
	destDir  string
	// remaining is the count the migrator last reported. Progress is measured
	// against this rather than against the migrator's own bookkeeping, so the
	// deadline depends only on what the interface returns.
	remaining  int
	startedAt  time.Time
	lastChange time.Time
}

func (c *DownloadClient) MonitorDownloads(tracks []*models.Track, m Monitor) error {
	var successDownloads int

	progressMap := make(map[string]*DownloadMonitor)
	pendingAlbums := make(map[string]*pendingAlbum)
	monCfg, err := m.GetConf()
	if err != nil {
		return err
	}

	ticker := time.NewTicker(monCfg.CheckInterval)

	defer ticker.Stop()

	var pollFailures int

	phaseStart := time.Now()

	for range ticker.C {
		statuses, err := m.GetDownloadStatus(tracks)
		if err != nil {
			// A failed poll used to end monitoring for every track at once, so
			// one blip -- a restarted service, a dropped connection -- cost the
			// whole playlist, since tracks that never reach Present are dropped
			// before it is built. Ride out a few and try again on the next tick.
			pollFailures++
			if pollFailures >= maxPollFailures {
				return fmt.Errorf("[%s/monitor] error fetching download status %d times in a row, giving up: %s",
					monCfg.Service, pollFailures, err.Error())
			}
			slog.Warn("[monitor] couldn't fetch download status, retrying",
				"service", monCfg.Service, "attempt", pollFailures, "context", err.Error())
			continue
		}
		pollFailures = 0
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
			expired := monCfg.MaxDuration > 0 && monitoredTime > monCfg.MaxDuration

			if (fileStatus.BytesRemaining == 0 && fileStatus.BytesTransferred != 0) || fileStatus.PercentComplete == 100 || strings.Contains(fileStatus.State, "Succeeded") {
				track.File = fileStatus.Filename
				slog.Info("[monitor] file downloaded successfully", "service", monCfg.Service, "file", track.File)
				var filePath string
				track.File, filePath = parsePath(track.File)
				if monCfg.MigrateDownload {
					destDir, err := m.MoveDownload(monCfg.FromDir, monCfg.ToDir, filePath, track)
					if err != nil {
						// The song never landed in the library, so it must
						// not be marked Present or counted: before this,
						// the playlist kept a track that never arrived.
						slog.Error("error while moving file, skipping track", "err", err)
						tracker.Skipped = true
						if err = m.Cleanup(*track, fileStatus.QueueID); err != nil {
							slog.Debug("cleanup failed", logging.RuntimeAttr(err.Error()))
						}
						continue
					}
					slog.Info("track moved successfully", "service", monCfg.Service)
					c.trackAlbum(pendingAlbums, key, &pendingAlbum{
						track:      track,
						trackDir:   filepath.Join(monCfg.FromDir, filePath),
						destDir:    destDir,
						startedAt:  currentTime,
						lastChange: currentTime,
					}, monCfg)
				}
				track.Present = true
				delete(progressMap, key)
				successDownloads += 1
				if err = m.Cleanup(*track, fileStatus.QueueID); err != nil {
					slog.Debug("cleanup failed", logging.RuntimeAttr(err.Error()))
				}
				continue

			} else if fileStatus.BytesTransferred > tracker.LastBytesTransferred && !expired {
				tracker.LastBytesTransferred = fileStatus.BytesTransferred
				tracker.LastUpdated = currentTime
				slog.Info("[monitor] progress updated", "service", monCfg.Service, "title", track.CleanTitle, "bytes transferred", fileStatus.BytesTransferred)
				continue

			} else if fileStatus.State == "Errored" ||
				stallTime > monCfg.StallDuration ||
				expired {

				switch {
				case fileStatus.State == "Errored":
					slog.Info("[monitor] download errored",
						"service", monCfg.Service,
						"title", track.CleanTitle,
						"state", rawState(fileStatus),
					)
				case stallTime > monCfg.StallDuration:
					slog.Info("[monitor] download stalled",
						"service", monCfg.Service,
						"title", track.CleanTitle,
						"state", fileStatus.State,
						"duration", stallTime,
					)
				default:
					slog.Info("[monitor] maximum monitor time exceeded",
						"service", monCfg.Service,
						"title", track.CleanTitle,
						"state", fileStatus.State,
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
		c.migratePendingAlbums(pendingAlbums, monCfg, time.Now().Local())

			// Exit condition: all tracks have been processed or skipped, and
			// every release has finished being migrated
		if tracksProcessed(tracks, progressMap) && len(pendingAlbums) == 0 {
			slog.Info("[monitor] Finished", "service", monCfg.Service, "downloaded files", successDownloads, "total tracks", len(tracks))
			return nil
		}

		// Budget for this provider exhausted: abandon whatever is still in
		// flight so the next service in DOWNLOAD_SERVICES gets a turn.
		if monCfg.FallbackAfter > 0 && time.Since(phaseStart) > monCfg.FallbackAfter {
			abandoned := abandonUnresolved(tracks, statuses, progressMap, m)
			slog.Info("[monitor] fallback threshold reached, moving on to the next download service",
				"service", monCfg.Service, "threshold", monCfg.FallbackAfter,
				"downloaded files", successDownloads, "unresolved tracks", abandoned)
			return nil
		}
	}
	return nil
}

// abandonUnresolved cancels any still-running download for tracks this provider
// did not finish, so it does not keep downloading in the background after we
// have moved on. Returns how many tracks were left unresolved.
func abandonUnresolved(tracks []*models.Track, statuses map[string]FileStatus, progressMap map[string]*DownloadMonitor, m Monitor) int {
	var unresolved int
	for _, track := range tracks {
		if track.Present {
			continue
		}
		key := fmt.Sprintf("%s|%s", track.ID, track.CleanTitle)
		if tracker, exists := progressMap[key]; exists && tracker.Skipped {
			continue
		}
		unresolved++

		fileStatus, exists := statuses[track.ID]
		if !exists {
			continue
		}
		if err := m.Cleanup(*track, fileStatus.QueueID); err != nil {
			slog.Debug("cleanup failed", logging.RuntimeAttr(err.Error()))
		}
	}
	return unresolved
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
// trackAlbum records a release for later migration, migrating whatever has
// already finished. Releases with nothing left over are never recorded, so a
// single-track download costs nothing.
func (c *DownloadClient) trackAlbum(pending map[string]*pendingAlbum, key string, album *pendingAlbum, monCfg MonitorConfig) {
	if remaining := c.migrateAlbum(album); remaining > 0 {
		album.remaining = remaining
		pending[key] = album
		slog.Info("[monitor] waiting for the rest of the release", "service", monCfg.Service,
			"album", album.track.Album, "files", remaining)
	}
}

// migrateAlbum moves whatever of one release has finished, returning how many
// of its files are still in flight.
func (c *DownloadClient) migrateAlbum(album *pendingAlbum) int {
	var remaining int
	for _, m := range c.albumMigrators() {
		remaining += m.MoveAlbumSiblings(album.trackDir, album.destDir, album.track)
	}
	return remaining
}

// migratePendingAlbums revisits every release still waiting on files. A release
// that stops making progress is abandoned on the same deadline a stalled track
// is, and one that keeps trickling in is abandoned at the same maximum a track
// is, so the monitor cannot be held open by a peer that has gone away or slowed
// to a crawl.
func (c *DownloadClient) migratePendingAlbums(pending map[string]*pendingAlbum, monCfg MonitorConfig, now time.Time) {
	for key, album := range pending {
		before := album.remaining
		remaining := c.migrateAlbum(album)
		album.remaining = remaining

		if remaining == 0 {
			delete(pending, key)
			if err := removeDirIfEmpty(album.trackDir); err != nil {
				slog.Debug("couldn't clean up release directory", "context", err.Error())
			}
			slog.Info("[monitor] release fully migrated", "service", monCfg.Service, "album", album.track.Album)
			continue
		}

		if remaining < before {
			album.lastChange = now
		}

		stalled := now.Sub(album.lastChange) > monCfg.StallDuration
		overdue := monCfg.MaxDuration > 0 && now.Sub(album.startedAt) > monCfg.MaxDuration
		if stalled || overdue {
			delete(pending, key)
			slog.Warn("[monitor] giving up on the rest of the release", "service", monCfg.Service,
				"album", album.track.Album, "files left in place", remaining, "stalled", stalled)
		}
	}
}
