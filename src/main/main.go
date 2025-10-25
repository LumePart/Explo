package main

import (
	"explo/src/debug"
	"log"
	"os"
	"log/slog"

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
	cfg.HandleDeprecation()
	debug.Init(cfg.LogLevel)
	cfg.GetPlaylistName()
}

func main() {
	var cfg config.Config
	if err := cfg.GetFlags(); err != nil {
		log.Fatal(err)
	}
	cfg.ReadEnv()
	setup(&cfg)
	slog.Info("Starting Explo...")

	httpClient := initHttpClient()
	client, err := client.NewClient(&cfg)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	discovery := discovery.NewDiscoverer(cfg.DiscoveryCfg, httpClient)
	downloader, err := downloader.NewDownloader(&cfg.DownloadCfg, httpClient, cfg.Flags.ExcludeLocal)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	tracks, err := discovery.Discover()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	if !cfg.Persist {
		err := client.DeletePlaylist()
		if err != nil {
			slog.Warn(err.Error())
		}
		downloader.DeleteSongs()
	}
	if cfg.Flags.DownloadMode != "force" {
		if err := client.CheckTracks(tracks); err != nil { // Check if tracks exist on system before downloading
			slog.Warn(err.Error())
		}
	}

	if cfg.Flags.DownloadMode != "skip" {
		downloader.StartDownload(&tracks)
		if len(tracks) == 0 {
			slog.Error("couldn't download any tracks")
			os.Exit(1)
		}
	}

	if err := client.CreatePlaylist(tracks); err != nil {
		slog.Warn(err.Error())
	} else {
		slog.Info("playlist created successfully", "system", cfg.System, "name", cfg.ClientCfg.PlaylistName)
	}
}