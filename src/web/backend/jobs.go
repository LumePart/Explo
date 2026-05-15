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

// RegisterCustomPlaylistRefresh registers a daily job that re-fetches custom playlists
// whose refresh interval has elapsed.
func (j *Jobs) RegisterCustomPlaylistRefresh(cfgDir string) error {
	_, err := j.scheduler.NewJob(
		gocron.CronJob("0 4 * * *", false),
		gocron.NewTask(func() {
			playlists := loadCustomPlaylists(cfgDir)
			updated := false
			for i, p := range playlists {
				if p.RefreshDays <= 0 {
					continue
				}
				if time.Since(p.LastFetched) < time.Duration(p.RefreshDays)*24*time.Hour {
					continue
				}
				slog.Info("custom-playlists: refreshing", "id", p.ID, "name", p.Name, "source", p.Source)
				var tracks [][4]string
				var err error
				switch p.Source {
				case "apple_music":
					_, _, tracks, err = fetchAppleMusicPlaylist(p.SourceURL)
				default:
					_, tracks, err = fetchLBPlaylistByMBID(p.LBMBID)
				}
				if err != nil {
					slog.Warn("custom-playlists: refresh fetch failed", "id", p.ID, "err", err)
					continue
				}
				writePrefetchCache(cfgDir, p.ID, tracks)
				playlists[i].LastFetched = time.Now().UTC()
				updated = true
			}
			if updated {
				if err := saveCustomPlaylists(cfgDir, playlists); err != nil {
					slog.Error("custom-playlists: failed to save after refresh", "err", err)
				}
			}
		}),
	)
	return err
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