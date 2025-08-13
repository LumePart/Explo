package downloader

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
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
	MonitorDownloads([]*models.Track) error
}

func NewDownloader(cfg *cfg.DownloadConfig, httpClient *util.HttpClient) *DownloadClient { // get download services from config and append them to DownloadClient
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
			downloader = append(downloader, NewLidarr(cfg.Lidarr, cfg.Discovery, cfg.DownloadDir, httpClient))
		default:
			log.Fatalf("downloader '%s' not supported", service)
		}
	}
	return &DownloadClient{
		Cfg:         cfg,
		Downloaders: downloader}
}

func (c *DownloadClient) StartDownload(tracks *[]*models.Track) {
	for _, d := range c.Downloaders {
		var g errgroup.Group
		g.SetLimit(5)

		for _, track := range *tracks {
			if track.Present {
				continue
			}

			g.Go(func() error {

				if err := d.QueryTrack(track); err != nil {
					log.Println(err.Error())
					return nil
				}
				if err := d.GetTrack(track); err != nil {
					log.Println(err.Error())
					return nil
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return
		}

		if err := d.MonitorDownloads(*tracks); err != nil {
			log.Printf("track monitoring failed: %s", err.Error())
		}
	}
	filterTracks(tracks)
}

func (c *DownloadClient) DeleteSongs() {
	entries, err := os.ReadDir(c.Cfg.DownloadDir)
	if err != nil {
		log.Printf("failed to read directory: %s", err.Error())
	}
	for _, entry := range entries {
		if !(entry.IsDir()) {
			err = os.Remove(path.Join(c.Cfg.DownloadDir, entry.Name()))

			if err != nil {
				log.Printf("failed to remove file: %s", err.Error())
			}
		}
	}
}

func filterTracks(tracks *[]*models.Track) { // only keep tracks that were downloaded or were found by music system
	filteredTracks := (*tracks)[:0]
	for _, track := range *tracks {
		if track.Present {
			track.Present = false // clear present status so music system can use the same field
			filteredTracks = append(filteredTracks, track)
		}
	}
	*tracks = filteredTracks
}

func containsLower(str string, substr string) bool {

	return strings.Contains(
		strings.ToLower(str),
		strings.ToLower(substr),
	)
}

func sanitizeName(s string) string { // return string with only letters and digits
	var sanitizer = regexp.MustCompile(`[^\p{L}\d]+`)
	return sanitizer.ReplaceAllString(s, "")
}

func getFilename(title, artist string) string {

	// Remove illegal characters for file naming
	re := regexp.MustCompile(`[^\p{L}\d._,\-]+`)
	t := re.ReplaceAllString(title, "_")
	a := re.ReplaceAllString(artist, "_")

	return fmt.Sprintf("%s-%s", t, a)
}

func moveDownload(srcDir, destDir, trackPath, file string) error { // Move download from the source dir to the dest dir (download dir)
	trackDir := filepath.Join(srcDir, trackPath)
	srcFile := filepath.Join(trackDir, file)

	info, err := os.Stat(srcFile)
	if err != nil {
		return fmt.Errorf("stat error: %s", err.Error())
	}

	in, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("couldn't open source file: %s", err.Error())
	}

	defer func() {
		if cerr := in.Close(); cerr != nil {
			log.Printf("warning: failed to close source file: %s", cerr)
		}
	}()

	dstDir := filepath.Join(destDir, trackPath)
	if err = os.MkdirAll(dstDir, os.ModePerm); err != nil {
		return fmt.Errorf("couldn't make download directory: %s", err.Error())
	}

	dstFile := filepath.Join(dstDir, file)
	out, err := os.Create(dstFile)
	if err != nil {
		return fmt.Errorf("couldn't create destination file: %s", err.Error())
	}

	defer func() {
		if err = out.Close(); err != nil {
			log.Printf("failed to close destination file: %s", err.Error())
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("copy failed: %s", err.Error())
	}

	if err = out.Sync(); err != nil {
		return fmt.Errorf("sync failed: %s", err.Error())
	}

	if err = os.Chmod(dstFile, info.Mode()); err != nil {
		return fmt.Errorf("chmod failed: %s", err.Error())
	}

	if err = os.Remove(srcFile); err != nil {
		return fmt.Errorf("failed to delete original file: %s", err.Error())
	}

	return nil
}
