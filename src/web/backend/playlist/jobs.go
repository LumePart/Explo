package playlist

import (
	"log/slog"
	"os"
	"time"
	"explo/src/util"
	"explo/src/web/backend/jobs"

	"github.com/go-co-op/gocron/v2"
)

// dueForRefresh reports whether a playlist whose data was last fetched
// at lastFetched is due again under a refreshDays cadence. A zero or
// negative cadence refreshes on every tick; the cron schedule itself
// sets the tick rate, so a weekly list is refreshed by its daily job
// exactly when seven days have passed since the last fetch.
func dueForRefresh(lastFetched time.Time, refreshDays int, now time.Time) bool {
	if refreshDays <= 0 {
		return true
	}
	return !now.Before(lastFetched.Add(time.Duration(refreshDays) * 24 * time.Hour))
}

// RegisterCustomPlaylistRefresh registers a cache-refresh job for each custom playlist
// using its stored schedule. Falls back to daily at 4 AM if no schedule is set.
func (p *Playlist) RegisterCustomPlaylistRefresh(j *jobs.Jobs) error {
	playlists := loadCustomPlaylists(p.cfg.WebDataDir)
	if len(playlists) == 0 {
		return nil
	}

	var envValues map[string]string
	if data, err := os.ReadFile(p.cfg.WebEnvPath); err == nil {
		envValues = p.settings.ParseEnvText(string(data))
	} else {
		envValues = map[string]string{}
	}

	for _, plist := range playlists {
		plist := plist
		prefix := util.CustomEnvPrefix(plist.Name)
		flags := envValues[prefix+"_FLAGS"]
		if flags == "" {
			continue // disabled
		}
		schedule := envValues[prefix+"_SCHEDULE"]
		if plist.RefreshDays <= 0 && schedule == "" {
			continue
		}
		if schedule == "" {
			schedule = "0 4 * * *"
		}
		_, err := j.Scheduler.NewJob(
			gocron.CronJob(schedule, false),
			gocron.NewTask(func() {
				// Reload the current state before gating: plist is the
				// copy captured at registration, so gating on its
				// LastFetched refreshed every run once the playlist was
				// past its interval (daily refresh of a weekly list).
				lastFetched := plist.LastFetched
				if fresh := GetCustomPlaylist(p.cfg.WebDataDir, plist.ID); fresh != nil {
					lastFetched = fresh.LastFetched
				}
				if !dueForRefresh(lastFetched, plist.RefreshDays, time.Now()) {
					return
				}
				slog.Info("custom-playlists: refreshing", "id", plist.ID, "name", plist.Name, "source", plist.Source)
				result, err := fetchCustomPlaylistTracks(plist)
				if err != nil {
					slog.Warn("custom-playlists: refresh fetch failed", "id", plist.ID, "err", err)
					return
				}
				writePrefetchCache(p.cfg.WebDataDir, plist.ID, result.Tracks)
				if err := updateCustomPlaylists(p.cfg.WebDataDir, func(playlists []CustomPlaylist) ([]CustomPlaylist, error) {
					for i, pl := range playlists {
						if pl.ID == plist.ID {
							playlists[i].LastFetched = time.Now().UTC()
							break
						}
					}
					return playlists, nil
				}); err != nil {
					slog.Error("custom-playlists: failed to save after refresh", "err", err)
				}
			}),
		)
		if err != nil {
			slog.Warn("custom-playlists: failed to register refresh job", "id", plist.ID, "err", err)
		}
	}
	return nil
}