package playlist

import (
	"explo/src/util"
	"explo/src/web/backend/jobs"
	"log/slog"
	"os"
	"time"

	"github.com/go-co-op/gocron/v2"
)

// RegisterCustomPlaylistRefresh registers a cache-refresh job for each custom playlist
// using its stored schedule. Falls back to daily at 4 AM if no schedule is set.
func (p *Playlist) RegisterCustomPlaylistRefresh(j *jobs.Jobs) error {
	playlists := loadCustomPlaylists(p.cfg.WebDataDir)
	if len(playlists) == 0 {
		return nil
	}

	var envValues map[string]string
	var AMSuffixRe = false
	if data, err := os.ReadFile(p.cfg.WebEnvPath); err == nil {
		envValues = p.settings.ParseEnvText(string(data))
	} else {
		envValues = map[string]string{}
	}
	if envValues["SUFFIX_REMOVAL"] == "true" {
		AMSuffixRe = true
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
				if time.Since(plist.LastFetched) < time.Duration(plist.RefreshDays)*24*time.Hour {
					return
				}
				slog.Info("custom-playlists: refreshing", "id", plist.ID, "name", plist.Name, "source", plist.Source)
				result, err := fetchCustomPlaylistTracks(plist, AMSuffixRe)
				if err != nil {
					slog.Warn("custom-playlists: refresh fetch failed", "id", plist.ID, "err", err)
					return
				}
				writePrefetchCache(p.cfg.WebDataDir, plist.ID, result.Tracks)
				playlists := loadCustomPlaylists(p.cfg.WebDataDir)
				for i, pl := range playlists {
					if pl.ID == plist.ID {
						playlists[i].LastFetched = time.Now().UTC()
						break
					}
				}
				if err := saveCustomPlaylists(p.cfg.WebDataDir, playlists); err != nil {
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
