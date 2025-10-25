package config

import (
	"slices"
	"fmt"
	"strings"
	flag "github.com/spf13/pflag"
)

var (
	validPlaylists    = []string{"weekly-exploration", "weekly-jams", "daily-jams"}
	validDownloadMode = []string{"normal", "skip", "force"}
)

func (cfg *Config) GetFlags() error {
	var configPath string
	var playlist string
	var downloadMode string
	var excludeLocal bool
	var persist bool
	// Long flags
	flag.StringVarP(&configPath, "config", "c", ".env", "Path of the configuration file")
	flag.StringVarP(&playlist, "playlist", "p", "weekly-exploration", "Playlist where to get tracks. Supported: weekly-exploration, weekly-jams, daily-jams")
	flag.StringVarP(&downloadMode, "download-mode", "d", "normal", "Download mode: 'normal' (download only when track is not found locally), 'skip' (skip downloading, only use tracks already found locally), 'force' (always download, don't check for local tracks)")
	flag.BoolVarP(&excludeLocal, "exclude-local", "e",  false, "Exclude locally found tracks from the imported playlist")
	flag.BoolVar(&persist, "persist", true, "Keep playlists between generations")
	persistSet := flag.Lookup("persist").Changed

	flag.Parse()

	// Validation for playlist
	if !contains(validPlaylists, playlist) {
		return fmt.Errorf("flag validation error: invalid playlist %s (must be one of: %s)",
			playlist, strings.Join(validPlaylists, ", "))
	}

	// Validation for download mode
	if !contains(validDownloadMode, downloadMode) {
		return fmt.Errorf("flag validation error: invalid download mode %s (must be one of: %s)",
			downloadMode, strings.Join(validDownloadMode, ", "))
	}

	cfg.Flags.CfgPath = configPath
	cfg.Flags.Playlist = playlist
	cfg.Flags.DownloadMode = downloadMode
	cfg.Flags.ExcludeLocal = excludeLocal
	cfg.Flags.Persist = persist

	// for deprecation purposes (can be removed at a later date)
	cfg.Flags.PersistSet = persistSet

	cfg.mergeFlags()
	return nil
}

func (cfg *Config) mergeFlags() {
	cfg.DiscoveryCfg.Listenbrainz.ImportPlaylist = cfg.Flags.Playlist
	cfg.DownloadCfg.ExcludeLocal = cfg.Flags.ExcludeLocal
	
	if cfg.Flags.PersistSet {
		cfg.Persist = cfg.Flags.Persist
	} else {
		cfg.Persist = cfg.PersistENV
	}
}

func contains(valid []string, val string) bool {
	return slices.Contains(valid, val)
}