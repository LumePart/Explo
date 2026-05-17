package downloader

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

type DownloadClient struct {
	Cfg         *cfg.DownloadConfig
	Downloaders []Downloader
}

type Downloader interface {
	QueryTrack(*models.Track) error
	GetTrack(*models.Track) error
	Monitor
}

// get download services from config and append them to DownloadClient
func NewDownloader(cfg *cfg.DownloadConfig, httpClient *util.HttpClient, filterLocal bool) (*DownloadClient, error) {
	var downloader []Downloader
	for _, service := range cfg.Services {
		switch service {
		case "youtube":
			downloader = append(downloader, NewYoutube(cfg.Youtube, cfg, httpClient))
		case "slskd":
			slskdClient := NewSlskd(cfg.Slskd, cfg.DownloadDir)
			slskdClient.AddHeader()
			downloader = append(downloader, slskdClient)
		default:
			return nil, fmt.Errorf("downloader '%s' not supported", service)
		}
	}

	return &DownloadClient{
		Cfg:         cfg,
		Downloaders: downloader}, nil
}

func (c *DownloadClient) StartDownload(tracks *[]*models.Track) {
	var filesBeforeDownload map[string]struct{}

	if c.Cfg.ExcludeLocal { // remove locally found tracks, so they can't be added to playlist
		filterLocalTracks(tracks, true)
	}
	if c.needsDownloadDir() {
		if err := os.MkdirAll(c.Cfg.DownloadDir, 0755); err != nil {
			slog.Error(err.Error())
			return
		}
	}
	if c.needsDownloadDir() && playlistManifestCleanupRequired(c.Cfg) {
		var err error
		filesBeforeDownload, err = snapshotDownloadFiles(c.Cfg.DownloadDir)
		if err != nil {
			slog.Warn("failed to snapshot download directory before run", "context", err.Error())
		}
	}

	for _, d := range c.Downloaders {
		var g errgroup.Group
		g.SetLimit(1)

		for _, track := range *tracks {
			if track.Present {
				continue
			}

			g.Go(func() error {

				if err := d.QueryTrack(track); err != nil {
					slog.Warn(err.Error())
					return nil
				}
				if err := d.GetTrack(track); err != nil {
					slog.Warn(err.Error())
					return nil
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return
		}

		if m, ok := d.(Monitor); ok {
			err := c.MonitorDownloads(*tracks, m)
			if err != nil {
				slog.Warn(err.Error())
			}
		}
	}

	if c.needsDownloadDir() && playlistManifestCleanupRequired(c.Cfg) {
		if err := c.writePlaylistManifest(*tracks, filesBeforeDownload); err != nil {
			slog.Warn("failed to write playlist manifest", "context", err.Error())
		}
	}
	filterLocalTracks(tracks, false)
}

func (c *DownloadClient) needsDownloadDir() bool {
	for _, svc := range c.Cfg.Services {
		if svc == "youtube" || svc == "youtube-music" {
			return true
		}
	}
	return c.Cfg.Slskd.MigrateDL
}

func (c *DownloadClient) DeleteSongs() {
	if playlistManifestCleanupRequired(c.Cfg) {
		if err := c.deletePlaylistManifestFiles(c.Cfg); err != nil {
			slog.Warn("failed to clean playlist manifest downloads", "context", err.Error())
		}
		result, err := cleanupOrphanDownloads(c.Cfg)
		if err != nil {
			slog.Warn("failed to clean orphan downloads", "context", err.Error())
		} else {
			slog.Info("orphan cleanup finished", "scanned", result.Scanned, "removed", result.Removed, "referenced", result.Referenced, "skipped", result.Skipped)
		}
		return
	}

	downloadDir := cleanupDownloadDir(c.Cfg)
	entries, err := os.ReadDir(downloadDir)
	if err != nil {
		slog.Error("failed to read directory", "context", err.Error())
	}
	for _, entry := range entries {
		entryPath := filepath.Join(downloadDir, entry.Name())
		if entry.IsDir() {
			err = os.RemoveAll(entryPath)
		} else {
			err = os.Remove(entryPath)
		}

		if err != nil {
			slog.Error("failed to remove downloaded track", "context", err.Error())
		}
	}
}

func filterLocalTracks(tracks *[]*models.Track, preDownload bool) { // filter local tracks
	filteredTracks := (*tracks)[:0]

	for _, t := range *tracks {
		switch {
		case preDownload && !t.Present:
			// keep only unavailable tracks if c.FilterLocal is true
			filteredTracks = append(filteredTracks, t)

		case !preDownload && t.Present:
			// keep only tracks already present locally
			t.Present = false // reset so music system can reuse the field
			filteredTracks = append(filteredTracks, t)
		}
	}

	*tracks = filteredTracks
}

func getFilename(title, artist string) string {
	const maxBytes = 240

	// Remove illegal characters for file naming
	t := util.FilenameSafe(title)
	a := util.FilenameSafe(artist)

	// truncate long filename
	runes := []rune(fmt.Sprintf("%s-%s", t, a))
	for len(runes) > 0 && len(string(runes)) > maxBytes {
		runes = runes[:len(runes)-1]
	}

	return string(runes)
}

// ignore titles that have a specific keyword (defined in .env)
func ContainsKeyword(track models.Track, contentTitle string, filterList []string) bool {
	title := strings.ToLower(track.Title)
	artist := strings.ToLower(track.Artist)
	content := strings.ToLower(contentTitle)

	for _, keyword := range filterList {
		keyword = strings.ToLower(keyword)
		if strings.Contains(title, keyword) || strings.Contains(artist, keyword) {
			continue
		}
		if strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

func containsLower(str string, substr string) bool {

	return strings.Contains(
		strings.ToLower(str),
		strings.ToLower(substr),
	)
}

// Move download from the source dir to the dest dir (download dir)
func (c *DownloadClient) MoveDownload(srcDir, destDir, trackPath string, track *models.Track) error {
	trackDir := filepath.Join(srcDir, trackPath)
	srcFile := filepath.Join(trackDir, track.File)

	if c.Cfg.RenameTrack { // Rename file to {title}-{artist} format
		track.File = getFilename(track.CleanTitle, track.MainArtist) + filepath.Ext(track.File)
	}

	in, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("couldn't open source file: %s", err.Error())
	}

	defer func() {
		if cerr := in.Close(); cerr != nil {
			slog.Error(fmt.Sprintf("failed to close source file: %s", err.Error()))
		}
	}()

	moveCfg := *c.Cfg
	moveCfg.DownloadDir = destDir
	targetDir := trackDownloadDir(&moveCfg, track)
	if err = os.MkdirAll(targetDir, os.ModePerm); err != nil {
		return fmt.Errorf("couldn't make download directory: %s", err.Error())
	}

	dstFile := filepath.Join(targetDir, track.File)
	out, err := os.Create(dstFile)
	if err != nil {
		return fmt.Errorf("couldn't create destination file: %s", err.Error())
	}

	defer func() {
		if err = out.Close(); err != nil {
			slog.Error(fmt.Sprintf("failed to close destination file: %s", err.Error()))
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("copy failed: %s", err.Error())
	}

	if err = out.Sync(); err != nil {
		return fmt.Errorf("sync failed: %s", err.Error())
	}

	// Keep permissions, unless specified otherwise in .env (some systems don't support chmod)
	if c.Cfg.KeepPermissions {
		info, err := os.Stat(srcFile)
		if err != nil {
			return fmt.Errorf("stat error: %s", err.Error())
		}
		if err = os.Chmod(dstFile, info.Mode()); err != nil {
			return fmt.Errorf("chmod failed: %s", err.Error())
		}
	}

	// Remove only the moved file, not the directory
	if err = os.Remove(srcFile); err != nil {
		return fmt.Errorf("failed to delete original file: %s", err.Error())
	}

	// to avoid removing additional downloads check if directory is empty before removing
	isEmpty, err := isDirEmpty(trackDir)
	if err != nil {
		return fmt.Errorf("couldn't check if directory is empty: %s", err.Error())
	} else if isEmpty {
		if err = os.Remove(trackDir); err != nil {
			return fmt.Errorf("failed to remove empty directory: %s", err.Error())
		}
	}
	return nil
}

func isDirEmpty(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() {
		if err = f.Close(); err != nil {
			slog.Error(fmt.Sprintf("failed to close directory path: %s", err.Error()))
		}
	}()

	// If we get something other than an err, it's not empty
	_, err = f.Readdir(1)
	if err == io.EOF {
		return true, nil // no entries
	}
	return false, err
}
