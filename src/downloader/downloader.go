package downloader

import (
	"os"
	"path"
	"log"
	"sync"

	cfg "explo/src/config"
	"explo/src/debug"
	"explo/src/models"
	"explo/src/util"
)

type DownloadClient struct {
	Cfg *cfg.DownloadConfig
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
		}
	}

	return &DownloadClient{
		Cfg: cfg,
		Downloaders: downloader}
}

	func (c *DownloadClient) StartDownload(tracks *[]*models.Track) {
		for _, d := range c.Downloaders {
			var wg sync.WaitGroup
	
			for _, track := range *tracks {
				if track.Present {
					continue
				}
	
				wg.Add(1)
				go func(track *models.Track) {
					defer wg.Done()
	
					if err := d.QueryTrack(track); err != nil {
						debug.Debug(err.Error())
						return
					}
					if err := d.GetTrack(track); err != nil {
						debug.Debug(err.Error())
						return
					}
				}(track)
			}
			wg.Wait()
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