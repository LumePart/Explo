package main

import (
	"explo/src/debug"
	"log"
	"time"

	"explo/src/client"
	"explo/src/config"
	"explo/src/discovery"
	"explo/src/downloader"
	"explo/src/util"
)

type Song struct {
	Title  string
	Artist string
	Album  string
}

func initHttpClient(cfg *config.Config) *util.HttpClient {
	return util.NewHttp(util.HttpClientConfig{
		Timeout: time.Duration(cfg.Timeout) * time.Second,
	})
}

func setup(cfg *config.Config) { // Inits debug, gets playlist name, if needed, handles deprecation
	debug.Init(cfg.Debug)
	cfg.GetPlaylistName()
}

func main() {

	cfg := config.ReadEnv()
	setup(&cfg)
<<<<<<< HEAD
	httpClient := initHttpClient()
	client, err := client.NewClient(&cfg, httpClient)
	if err != nil {
		log.Fatal(err)
	}
=======
	httpClient := initHttpClient(&cfg)
	client := client.NewClient(&cfg, httpClient)
>>>>>>> 3f855d8 (Implement initial support for Lidarr downloader)
	discovery := discovery.NewDiscoverer(cfg.DiscoveryCfg, httpClient)
	downloader := downloader.NewDownloader(&cfg.DownloadCfg, httpClient)

	tracks, err := discovery.Discover()
	if err != nil {
		log.Fatal(err)
	}
	if !cfg.Persist {
		err := client.DeletePlaylist()
		if err != nil {
			log.Println(err)
		}
		downloader.DeleteSongs()
	}
	client.CheckTracks(tracks) // Check if tracks exist on system before downloading
	downloader.StartDownload(&tracks)
	if len(tracks) == 0 {
		log.Fatal("couldn't download any tracks")
	}

	if err := client.CreatePlaylist(tracks); err != nil {
		log.Println(err)
	} else {
		log.Printf("[%s] %s playlist created successfully", cfg.System, cfg.ClientCfg.PlaylistName)
	}
}
