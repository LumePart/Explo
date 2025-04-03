package main

import (
	"explo/src/debug"
	"log"

	"explo/src/client"
	"explo/src/config"
	"explo/src/discovery"
	"explo/src/downloader"
)

type Song struct {
	Title string
	Artist string
	Album string
}

func setup(cfg *config.Config) { // Inits debug, gets playlist name, if needed, handles deprecation
	debug.Init(cfg.Debug)
	cfg.GetPlaylistName()
}

func main() {

	cfg := config.ReadEnv()
	setup(&cfg)
	client := client.NewClient(&cfg)
	// VerifyDir?
	discovery := discovery.NewDiscoverer(cfg.DiscoveryCfg)
	downloader := downloader.NewDownloader(&cfg.DownloadCfg)

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
	if err := client.CreatePlaylist(tracks); err != nil {
		log.Println(err)
	}
}