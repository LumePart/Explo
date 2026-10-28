package config

import (
	"fmt"
	"os"

	"slices"
	"strings"

	flag "github.com/spf13/pflag"
)

var (
	validPlaylists    = []string{"weekly-exploration", "weekly-jams", "daily-jams", "on-repeat"}
	validDownloadMode = []string{"normal", "skip", "force"}
)

func (cfg *Config) GetFlags() error {
	var configPath string
	var playlist string
	var downloadMode string
	var excludeLocal bool
	var persist bool
	var replace bool
	var showVersion bool
	var searchMBID string
	var refreshOnly bool
	var cleanDownloads bool
	// Long flags
	flag.StringVarP(&configPath, "config", "c", ".env", "Path of the configuration file")
	flag.StringVarP(&playlist, "playlist", "p", "weekly-exploration", "Playlist where to get tracks. Supported: weekly-exploration, weekly-jams, daily-jams, on-repeat")
	flag.StringVarP(&downloadMode, "download-mode", "d", "normal", "Download mode: 'normal' (download only when track is not found locally), 'skip' (skip downloading, only use tracks already found locally), 'force' (always download, don't check for local tracks)")
	flag.BoolVar(&persist, "persist", true, "Keep playlists between generations (DEPRECATED - use --replace-playlist instead)")
	flag.BoolVarP(&excludeLocal, "exclude-local", "e", false, "Exclude locally found tracks from the imported playlist")
	flag.BoolVar(&replace, "replace-playlist", true, "Replace existing playlist with the same name")
	flag.BoolVarP(&showVersion, "version", "v", false, "Print version and exit")
	flag.StringVar(&searchMBID, "search-mbid", "", "Test Plex search for a single recording MBID (resolves via ListenBrainz, then searches your library)")
	flag.BoolVar(&refreshOnly, "refresh-only", false, "Trigger alibrary rescan and exit; skips discovery and downloads")
	flag.BoolVar(&cleanDownloads, "clean-downloads", false, "Delete previously downloaded tracks before downloading new ones (requires USE_SUBDIRECTORY)")

	flag.Parse()

	if showVersion {
		fmt.Println(Version)
		os.Exit(0)
	}
	persistSet := flag.Lookup("persist").Changed
	cfgSet := flag.Lookup("config").Changed
	playlistSet := flag.Lookup("playlist").Changed

	
	
	if searchMBID == "" {
		if !contains(validPlaylists, playlist) && !strings.HasPrefix(playlist, "custom-") {
		  return fmt.Errorf("flag validation error: invalid playlist %s (must be one of: %s, or a custom-* id)",
			playlist, strings.Join(validPlaylists, ", "))
		}
		if !contains(validDownloadMode, downloadMode) {
			return fmt.Errorf("flag validation error: invalid download mode %s (must be one of: %s)",
				downloadMode, strings.Join(validDownloadMode, ", "))
		}

	}
	cfg.Flags.CfgPath = configPath
	cfg.Flags.CfgSet = cfgSet
	cfg.Flags.Playlist = playlist
	cfg.Flags.PlaylistSet = playlistSet
	cfg.Flags.DownloadMode = downloadMode
	cfg.Flags.ExcludeLocal = excludeLocal
	cfg.Flags.ReplacePlaylist = replace
	cfg.Flags.SearchMBID = searchMBID
	cfg.Flags.RefreshOnly = refreshOnly
	cfg.Flags.CleanDownloads = cleanDownloads

	// for deprecation purposes (can be removed at a later date)
	cfg.Flags.PersistSet = persistSet

	return nil
}

func (cfg *Config) MergeFlags() {
	cfg.DiscoveryCfg.Listenbrainz.ImportPlaylist = cfg.Flags.Playlist
	cfg.DownloadCfg.ExcludeLocal = cfg.Flags.ExcludeLocal

	if cfg.Flags.CfgSet {
		cfg.ServerCfg.WebEnvPath = cfg.Flags.CfgPath
	}

	cfg.ReplacePlaylist = cfg.Flags.ReplacePlaylist
}

func contains(valid []string, val string) bool {
	return slices.Contains(valid, val)
}
