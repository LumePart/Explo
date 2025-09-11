package main

import (
	"explo/src/debug"
	"log"

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

func initHttpClient() *util.HttpClient {
	return util.NewHttp(util.HttpClientConfig{
		Timeout: 10,
	})
}

func setup(cfg *config.Config) { // Inits debug, gets playlist name, if needed, handles deprecation
	debug.Init(cfg.Debug)
	cfg.GetPlaylistName()
}

func main() {
	var cfg config.Config
	if err := cfg.GetFlags(); err != nil {
		log.Fatal(err)
	}
	cfg.ReadEnv()
	setup(&cfg)

	httpClient := initHttpClient()
	client, err := client.NewClient(&cfg, httpClient)
	if err != nil {
		log.Fatal(err)
	}
	discovery := discovery.NewDiscoverer(cfg.DiscoveryCfg, httpClient)
	downloader := downloader.NewDownloader(&cfg.DownloadCfg, httpClient, cfg.Flags.ExcludeLocal)

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
	if cfg.Flags.DownloadMode != "force" {
		client.CheckTracks(tracks) // Check if tracks exist on system before downloading
	}

	if cfg.Flags.DownloadMode != "skip" {
		downloader.StartDownload(&tracks)
		if len(tracks) == 0 {
			log.Fatal("couldn't download any tracks")
		}
	}

	if err := client.CreatePlaylist(tracks); err != nil {
		log.Println(err)
	} else {
		log.Printf("[%s] %s playlist created successfully", cfg.System, cfg.ClientCfg.PlaylistName)
	}
}
