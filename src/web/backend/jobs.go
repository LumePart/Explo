package backend

// Jobs running on a schedule go here i.e cache cleanups (and playlist imports in the future)

import (
	"path/filepath"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/go-co-op/gocron/v2"
)


type Jobs struct {
	scheduler gocron.Scheduler
}

type fileInfo struct {
		path    string
		size    int64
		modTime time.Time
}

func NewJobs() (*Jobs) {
	scheduler, err := gocron.NewScheduler()
	if err != nil {
		slog.Error("failed creating cron scheduler")
	}

	return &Jobs{ scheduler: scheduler}
}

func (j *Jobs) Start() {
	j.scheduler.Start()
}

func (j *Jobs) RegisterCoverCleanup(schedule, coversDir string, maxBytes int64) error {
	_, err := j.scheduler.NewJob(
		gocron.CronJob(schedule, false),
		gocron.NewTask(func() {
			slog.Info("running cache cleanup")

			trimCacheDir(coversDir, maxBytes)
		}),
	)

	return err
}

// RegisterCustomPlaylistRefresh registers a cache-refresh job for each custom playlist
// using its stored schedule. Falls back to daily at 4 AM if no schedule is set.
func (j *Jobs) RegisterCustomPlaylistRefresh(cfgDir, envPath string) error {
	playlists := loadCustomPlaylists(cfgDir)
	if len(playlists) == 0 {
		return nil
	}

	var envValues map[string]string
	if data, err := os.ReadFile(envPath); err == nil {
		envValues = parseEnvText(string(data))
	} else {
		envValues = map[string]string{}
	}

	for _, p := range playlists {
		p := p
		prefix := customEnvPrefix(p.ID)
		flags := envValues[prefix+"_FLAGS"]
		if flags == "" {
			continue // disabled
		}
		schedule := envValues[prefix+"_SCHEDULE"]
		if p.RefreshDays <= 0 && schedule == "" {
			continue
		}
		if schedule == "" {
			schedule = "0 4 * * *"
		}
		_, err := j.scheduler.NewJob(
			gocron.CronJob(schedule, false),
			gocron.NewTask(func() {
				if time.Since(p.LastFetched) < time.Duration(p.RefreshDays)*24*time.Hour {
					return
				}
				slog.Info("custom-playlists: refreshing", "id", p.ID, "name", p.Name, "source", p.Source)
				result, err := fetchCustomPlaylistTracks(p)
				if err != nil {
					slog.Warn("custom-playlists: refresh fetch failed", "id", p.ID, "err", err)
					return
				}
				writePrefetchCache(cfgDir, p.ID, result.Tracks)
				playlists := loadCustomPlaylists(cfgDir)
				for i, pl := range playlists {
					if pl.ID == p.ID {
						playlists[i].LastFetched = time.Now().UTC()
						break
					}
				}
				if err := saveCustomPlaylists(cfgDir, playlists); err != nil {
					slog.Error("custom-playlists: failed to save after refresh", "err", err)
				}
			}),
		)
		if err != nil {
			slog.Warn("custom-playlists: failed to register refresh job", "id", p.ID, "err", err)
		}
	}
	return nil
}

func trimCacheDir(dataDir string, maxBytes int64) {

	var files []fileInfo
	var total int64

	err := filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		files = append(files, fileInfo{
			path:    path,
			size:    info.Size(),
			modTime: info.ModTime(),
		})

		total += info.Size()
		return nil
	})

	if err != nil || total <= maxBytes {
		return
	}

	slices.SortFunc(files, func(a, b fileInfo) int {
		return a.modTime.Compare(b.modTime)
	})

	for _, f := range files {
		if total <= maxBytes {
			break
		}

		if err := os.Remove(f.path); err == nil {
			total -= f.size
		}
	}
}