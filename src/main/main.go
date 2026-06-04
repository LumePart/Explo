package main

import (
	"explo/src/logging"
	"explo/src/models"
	"explo/src/web/backend"
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
func runSearchTest(cfg *config.Config, httpClient *util.HttpClient) {
	lb := discovery.NewListenBrainz(cfg.DiscoveryCfg, httpClient)
	track, err := lb.LookupRecording(cfg.Flags.SearchMBID)
	if err != nil {
		log.Fatalf("failed to resolve MBID %s from ListenBrainz: %s", cfg.Flags.SearchMBID, err)
	}
	slog.Info("resolved recording", "title", track.CleanTitle, "artist", track.MainArtist, "album", track.Album, "duration_ms", track.Duration)

	c, err := client.NewClient(cfg)
	if err != nil {
		log.Fatalf("failed to init client: %s", err)
	}
	tracks := []*models.Track{track}
	if err := c.CheckTracks(tracks); err != nil {
		slog.Warn("CheckTracks error", "err", err)
	}

	if track.Present {
		slog.Info("FOUND in library", "system", cfg.System, "key", track.ID)
	} else {
		slog.Info("NOT FOUND in library", "system", cfg.System)
	}
}
func main() {
	var cfg config.Config
	if err := cfg.GetFlags(); err != nil {
		log.Fatal(err)
	}
	cfg.ReadEnv()
	cfg.MergeFlags()
	setup(&cfg)

	httpClient := initHttpClient()

	if cfg.Flags.SearchMBID != "" {
		runSearchTest(&cfg, httpClient)
		return
	}

	slog.Info("Starting Explo...")

	if err := os.MkdirAll(cfg.DownloadCfg.Youtube.CoversDir, 0755); err != nil {
		slog.Error("failed making directory", "msg", err.Error())
	}

	if cfg.ServerCfg.Enabled {

		exploPath, err := os.Executable()
		if err != nil {
			log.Fatal("could not determine executable path: ", err)
		}

		cfg.ServerCfg.ExploPath = exploPath
		srv := backend.NewServer(cfg.ServerCfg)
		log.Fatal(srv.Start())
	}

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

	if cfg.ServerCfg.Enabled {
		added := make(map[string]bool)
		for _, t := range tracks {
			added[t.CleanTitle+"|"+t.Artist] = true
		}
		backend.WritePlaylistCache(cfg.Flags.CfgPath, cfg.Flags.Playlist, allTracks, added)
	}

	if err := client.CreatePlaylist(tracks); err != nil {
		slog.Warn(err.Error())
	} else {
		slog.Info("playlist created successfully", "system", cfg.System, "playlistName", cfg.ClientCfg.PlaylistName, "notify", true)
	}
}
