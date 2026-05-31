package main

import (
	"encoding/json"
	"explo/src/logging"
	"explo/src/models"
	"explo/src/web/backend"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

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

// loadCustomTracks reads a custom playlist's track cache and returns them as
// models.Track slices, bypassing the LB discovery step entirely.
func loadCustomTracks(dataDir, playlistID string) ([]*models.Track, string, error) {
	type cachedTrack struct {
		Title      string `json:"title"`
		Artist     string `json:"artist"`
		MainArtist string `json:"mainArtist"`
		Release    string `json:"release"`
		CoverURL   string `json:"coverUrl"`
	}
	type cacheFile struct {
		Tracks []cachedTrack `json:"tracks"`
	}
	type customPlaylist struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	data, err := os.ReadFile(filepath.Join(dataDir, "cache", playlistID+".json"))
	if err != nil {
		return nil, "", fmt.Errorf("custom playlist %q not found in cache: %w", playlistID, err)
	}
	var c cacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, "", fmt.Errorf("failed to parse custom playlist cache: %w", err)
	}

	// Look up the human-readable name from metadata
	name := playlistID
	if meta, err := os.ReadFile(filepath.Join(dataDir, "custom-playlists.json")); err == nil {
		var all []customPlaylist
		if json.Unmarshal(meta, &all) == nil {
			for _, p := range all {
				if p.ID == playlistID {
					name = p.Name
					break
				}
			}
		}
	}

	tracks := make([]*models.Track, len(c.Tracks))
	for i, t := range c.Tracks {
		mainArtist := t.MainArtist
		if mainArtist == "" {
			mainArtist = t.Artist
		}
		tracks[i] = &models.Track{
			CleanTitle: t.Title,
			Title:      t.Title,
			Artist:     t.Artist,
			MainArtist: mainArtist,
			Album:      t.Release,
			CoverURL:   t.CoverURL,
		}
	}
	return tracks, name, nil
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
	var cfg config.Config
	if err := cfg.GetFlags(); err != nil {
		log.Fatal(err)
	}
	cfg.ReadEnv()
	cfg.MergeFlags()
	setup(&cfg)
	slog.Info("Starting Explo...")

	if cfg.ServerCfg.Enabled {
		
		exploPath, err := os.Executable()
		if err != nil {
			log.Fatal("could not determine executable path: ", err)
		}

		cfg.ServerCfg.ExploPath = exploPath
		srv := backend.NewServer(cfg.ServerCfg)
		log.Fatal(srv.Start())
	}

	if cfg.Flags.RefreshOnly {
		if err := client.TriggerRefresh(&cfg); err != nil {
			slog.Error("refresh-only failed", "err", err.Error())
			os.Exit(1)
		}
		slog.Info("library refresh triggered")
		return
	}

	httpClient := initHttpClient()

	var tracks []*models.Track
	var err error
	if strings.HasPrefix(cfg.Flags.Playlist, "custom-") {
		var playlistName string
		tracks, playlistName, err = loadCustomTracks(cfg.ServerCfg.WebDataDir, cfg.Flags.Playlist)
		if err == nil {
			cfg.ClientCfg.PlaylistName = playlistName
		}
	} else {
		disc := discovery.NewDiscoverer(cfg.DiscoveryCfg, httpClient)
		tracks, err = disc.Discover()
	}
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
		uploadCustomPlaylistArtwork(&cfg, client)
	}
}

// uploadCustomPlaylistArtwork pushes a custom playlist's cached artwork to the music app
// after first successful creation. No-op for non-custom playlists, playlists without
// artwork, or clients that don't support artwork upload (Subsonic, MPD).
func uploadCustomPlaylistArtwork(cfg *config.Config, c *client.Client) {
	if !strings.HasPrefix(cfg.Flags.Playlist, "custom-") {
		return
	}
	cp := backend.GetCustomPlaylist(cfg.ServerCfg.WebDataDir, cfg.Flags.Playlist)
	if cp == nil || cp.ArtworkURL == "" || cp.ArtworkUploaded {
		return
	}
	uploader, ok := c.API.(client.ArtworkUploader)
	if !ok {
		return
	}
	path := backend.CustomPlaylistArtworkPath(cfg.ServerCfg.WebDataDir, cp.ID)
	if _, err := os.Stat(path); err != nil {
		slog.Warn("custom-playlists: artwork not cached locally, skipping upload", "id", cp.ID, "path", path)
		return
	}
	if err := uploader.SetPlaylistArtwork(path); err != nil {
		slog.Warn("custom-playlists: failed to upload playlist artwork", "id", cp.ID, "err", err.Error())
		return
	}
	if err := backend.MarkCustomPlaylistArtworkUploaded(cfg.ServerCfg.WebDataDir, cp.ID); err != nil {
		slog.Warn("custom-playlists: artwork upload succeeded but flag not persisted", "id", cp.ID, "err", err.Error())
		return
	}
	slog.Info("custom-playlists: playlist artwork uploaded", "id", cp.ID, "system", cfg.System)
}
