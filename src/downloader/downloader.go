package downloader

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"

	ffmpeg "github.com/u2takey/ffmpeg-go"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
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
			downloader = append(downloader, NewYoutube(cfg.Youtube, cfg.Discovery, cfg.DownloadDir, httpClient))
		case "slskd":
			slskdClient := NewSlskd(cfg.Slskd, cfg.DownloadDir)
			slskdClient.AddHeader()
			downloader = append(downloader, slskdClient)
		case "lidarr":
			lidarrClient := NewLidarr(cfg.Lidarr, cfg.DownloadDir)
			lidarrClient.AddHeader()
			if err := lidarrClient.getRootDirectory(); err != nil {
				return nil, err
			}
			downloader = append(downloader, lidarrClient)
		default:
			return nil, fmt.Errorf("downloader '%s' not supported", service)
		}
	}

	return &DownloadClient{
		Cfg:         cfg,
		Downloaders: downloader}, nil
}

func (c *DownloadClient) StartDownload(tracks *[]*models.Track) {
	if c.Cfg.ExcludeLocal { // remove locally found tracks, so they can't be added to playlist
		filterLocalTracks(tracks, true)
	}

	if c.needsDownloadDir() {
		if err := os.MkdirAll(c.Cfg.DownloadDir, 0755); err != nil {
			slog.Error(err.Error())
			return
		}
	}

	for _, d := range c.Downloaders {
		var g errgroup.Group
		g.SetLimit(3)

		limiter := rate.NewLimiter(rate.Every(time.Second), c.Cfg.DownloadLimiter)

		for _, track := range *tracks {
			if track.Present {
				continue
			}

			track := track

			g.Go(func() error {
				ctx := context.Background()

				if err := limiter.Wait(ctx); err != nil {
					return err
				}

				if err := d.QueryTrack(track); err != nil {
					slog.Warn(err.Error())
					return nil
				}

				if err := limiter.Wait(ctx); err != nil {
					return err
				}

				if err := d.GetTrack(track); err != nil {
					slog.Warn(err.Error())
					return nil
				}

				return nil
			})
		}

		if err := g.Wait(); err != nil {
			slog.Warn(err.Error())
			return
		}

		if m, ok := d.(Monitor); ok {
			if err := c.MonitorDownloads(*tracks, m); err != nil {
				slog.Warn(err.Error())
			}
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
	if c.Cfg.Lidarr.MigrateDL {
		return c.Cfg.Lidarr.MigrateDL
	}
	return c.Cfg.Slskd.MigrateDL
}

func (c *DownloadClient) DeleteSongs() {
	entries, err := os.ReadDir(c.Cfg.DownloadDir)
	if err != nil {
		slog.Error("failed to read directory", "context", err.Error())
	}
	for _, entry := range entries {
		if !(entry.IsDir()) {
			err = os.Remove(path.Join(c.Cfg.DownloadDir, entry.Name()))

			if err != nil {
				slog.Error("failed to remove file", "context", err.Error())
			}
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

func sanitize(s string) string {
	s = strings.TrimSpace(s)

    replacer := strings.NewReplacer(
        "/", "-",
        "\\", "-",
        ":", "-",
        "*", "",
        "?", "",
        "\"", "",
        "<", "",
        ">", "",
        "|", "",
    )

    s = replacer.Replace(s)

    switch s {
    case ".", "..", ". .", "":
        return "_"
    }

    return s
}

func buildTrackPath(template string, track *models.Track) (string, error) {
	replacements := map[string]string{
		"Artist":		sanitize(track.MainArtist),
		"Album":		sanitize(track.Album),
		"AlbumName":	sanitize(track.Album),
		"TrackName":	sanitize(track.CleanTitle),
		"TrackNumber":	fmt.Sprintf("%02d", track.TrackNumber),
		"DiscNumber":	fmt.Sprintf("%02d", track.DiscNumber),
		"Year":			strconv.Itoa(track.OriginalYear),
		"File":			sanitize(track.File),
		"ext":			strings.TrimPrefix(filepath.Ext(track.File), "."),
	}

	result := template

	for key, value := range replacements {
		result = strings.ReplaceAll(
			result,
			"{{"+key+"}}",
			value,
		)
	}
	cleanPath := filepath.Clean(result)

	if !filepath.IsLocal(cleanPath) {
        return "", fmt.Errorf("path template resolves to a non-local path: %s", cleanPath)
    }

	file := filepath.Base(cleanPath)
	if ext := filepath.Ext(file); ext == "" {
		slog.Warn("path template does not have file extension ( {{ext}} ) appended, adding it automatically")
		file += filepath.Ext(track.File)
		cleanPath = filepath.Join(filepath.Dir(cleanPath), file)
	}

	track.File = file
	return cleanPath, nil
}

func overwriteMetadata(metadata []string, srcFile string) error {
	opts := ffmpeg.KwArgs{
			"c": "copy",
			"metadata": metadata,
			"loglevel": "error",
		}
		streams := []*ffmpeg.Stream{
    		ffmpeg.Input(srcFile),
		}

		tmpFile := tempAudioFile(srcFile)

		if err := util.WriteMetadata(streams, "", tmpFile, opts); err != nil {
			return fmt.Errorf("failed to overwrite metadata: %w", err)
		} else {
			if err := os.Rename(tmpFile, srcFile); err != nil {
				return fmt.Errorf("failed to rename tmp file: %w", err)
			}
		}
		return nil
}

func tempAudioFile(path string) string {
    ext := filepath.Ext(path)
    return strings.TrimSuffix(path, ext) + ".tmp" + ext
}


func moveTrack(srcFile, destDir string, track *models.Track, pathTemplate string, keepPerms bool) error {
	var dstFile string
    if pathTemplate != "" {
        relativePath, err := buildTrackPath(pathTemplate, track)
		if err != nil {
			return err
		}

        if track.File == "." || track.File == string(filepath.Separator) {
            track.File = getFilename(track.CleanTitle, track.MainArtist) + filepath.Ext(srcFile)
            relativePath = filepath.Join(filepath.Dir(relativePath), track.File)
            slog.Warn("invalid path template result",
                "track", track.Title,
                "artist", track.Artist,
                "filename", track.File)
        }

        dstFile = filepath.Join(destDir, relativePath)
    } else {
        if err := os.MkdirAll(destDir, os.ModePerm); err != nil {
            return err
        }
        dstFile = filepath.Join(destDir, track.File)
    }

    if err := moveFile(srcFile, dstFile, keepPerms); err != nil {
        return err
    }

    srcDir := filepath.Dir(srcFile)
    isEmpty, err := isDirEmpty(srcDir)
	if err != nil {
		return fmt.Errorf("couldn't check if directory is empty: %s", err.Error())
	} else if isEmpty {
		if err = os.Remove(srcDir); err != nil {
			return fmt.Errorf("failed to remove empty directory: %s", err.Error())
		}
	}

    return nil
}

func moveFile(srcFile, dstFile string, keepPermissions bool) error {
    in, err := os.Open(srcFile)
    if err != nil {
        return fmt.Errorf("couldn't open source file: %w", err)
    }
    defer func() {
    	if err := in.Close(); err != nil {
        	slog.Warn("failed to close source file", "err", err)
    	}
	}()
	if err := os.MkdirAll(filepath.Dir(dstFile), 0755); err != nil {
		return fmt.Errorf("couldn't create directory for file: %w", err)
	}
    out, err := os.Create(dstFile)
    if err != nil {
        return fmt.Errorf("couldn't create destination file: %w", err)
    }

    if _, err := io.Copy(out, in); err != nil {
        if closeErr := out.Close(); closeErr != nil {
        	slog.Warn("failed to close destination file", "err", closeErr)
    	}
        return fmt.Errorf("copy failed: %w", err)
    }

    if err := out.Sync(); err != nil {
        if closeErr := out.Close(); closeErr != nil {
        	slog.Warn("failed to close destination file", "err", closeErr)
    	}
        return fmt.Errorf("sync failed: %w", err)
    }

    if err := out.Close(); err != nil {
        return fmt.Errorf("failed to close destination file: %w", err)
    }

    if keepPermissions {
        info, err := os.Stat(srcFile)
        if err != nil {
            return fmt.Errorf("stat error: %w", err)
        }

        if err := os.Chmod(dstFile, info.Mode()); err != nil {
            return fmt.Errorf("chmod failed: %w", err)
        }
    }

    return os.Remove(srcFile)
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