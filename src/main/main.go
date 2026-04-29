package main

import (
	"explo/src/logging"
	"explo/src/models"
	"explo/src/web"
	"log"
	"log/slog"
	"os"
	
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

// Inits debug, gets playlist name, if needed, handles deprecation
func setup(cfg *config.Config) {
	cfg.HandleDeprecation()
	notifyClient := logging.InitNotify(cfg.NotifyCfg)
	logging.Init(cfg.LogLevel, notifyClient)
	cfg.GenPlaylistName()
}

func main() {
	if os.Getenv("WEB_UI") == "true" {
		cfgPath := os.Getenv("WEB_CFG_PATH")
		if cfgPath == "" {
			cfgPath = ".env"
		}
		exploPath, err := os.Executable()
		if err != nil {
			log.Fatal("could not determine executable path: ", err)
		}
		addr := os.Getenv("WEB_ADDR")
		if addr == "" {
			addr = ":7288"
		}
		srv := web.NewServer(cfgPath, exploPath)
		log.Fatal(srv.Start(addr))
	}

	var cfg config.Config
	if err := cfg.GetFlags(); err != nil {
		log.Fatal(err)
	}
	cfg.ReadEnv()
	cfg.MergeFlags()
	setup(&cfg)
	slog.Info("Starting Explo...")

	httpClient := initHttpClient()
	discovery := discovery.NewDiscoverer(cfg.DiscoveryCfg, httpClient)
	tracks, err := discovery.Discover()
	if err != nil {
		slog.Error(err.Error(), "notify", true)
		os.Exit(1)
	}
	allTracks := append([]*models.Track(nil), tracks...)

	client, err := client.NewClient(&cfg)
	if err != nil {
		slog.Error(err.Error(), "notify", true)
		os.Exit(1)
	}
	downloader, err := downloader.NewDownloader(&cfg.DownloadCfg, httpClient, cfg.Flags.ExcludeLocal)
	if err != nil {
		slog.Error(err.Error(), "notify", true)
		os.Exit(1)
	}
	if !cfg.Persist {
		err := client.DeletePlaylist()
		if err != nil {
			slog.Warn(err.Error(), "notify", true)
		}
		if cfg.DownloadCfg.UseSubDir {
			downloader.DeleteSongs()
		}
	}
	if cfg.Flags.DownloadMode != "force" {
		if err := client.CheckTracks(tracks); err != nil { // Check if tracks exist on system before downloading
			slog.Warn(err.Error(), "notify", true)
		}
	}

	if cfg.Flags.DownloadMode != "skip" {
		downloader.StartDownload(&tracks)
		if len(tracks) == 0 {
			slog.Error("couldn't download any tracks", "notify", true)
			os.Exit(1)
		}
	}

	added := make(map[string]bool)
	for _, t := range tracks {
		added[t.CleanTitle+"|"+t.Artist] = true
	}
	writePlaylistCache(cfg.Flags.CfgPath, cfg.Flags.Playlist, allTracks, added)

	if err := client.CreatePlaylist(tracks); err != nil {
		slog.Warn(err.Error())
	} else {
		slog.Info("playlist created successfully", "system", cfg.System, "playlistName", cfg.ClientCfg.PlaylistName, "notify", true)
	}
}
