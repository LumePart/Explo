package config

import (
	"slices"
	"fmt"
	"strings"
	flag "github.com/spf13/pflag"
)
// Custom usage message to add short and long flags together
/* const usage = `Usage of explo:
  -c, --config Path of the configuration file
  -p, --playlist Playlist where to get tracks. Supported: weekly-exploration, weekly-jams, daily-jams
  --download-mode Download mode: 'normal' (download only when track is not found locally), 'skip' (skip downloading, only use tracks already found locally), 'force' (always download, don't check for local tracks)
  --filter-local Filter out locally found tracks from playlist
  -h, --help prints help information 
` */

var (
	validPlaylists    = []string{"weekly-exploration", "weekly-jams", "daily-jams"}
	validDownloadMode = []string{"normal", "skip", "force"}
)

func (cfg *Config) GetFlags() error {
	var configPath string
	var playlist string
	var downloadMode string
	var filterLocal bool
	// Long flags
	flag.StringVarP(&configPath, "config", "c", ".env", "Path of the configuration file")
	flag.StringVarP(&playlist, "playlist", "p", "weekly-exploration", "Playlist where to get tracks. Supported: weekly-exploration, weekly-jams, daily-jams")
	flag.StringVarP(&downloadMode, "download-mode", "d", "normal", "Download mode: 'normal' (download only when track is not found locally), 'skip' (skip downloading, only use tracks already found locally), 'force' (always download, don't check for local tracks)")
	flag.BoolVar(&filterLocal, "filter-local",  false, "Filter out locally found tracks from playlist")

	//flag.Usage = func() { fmt.Print(usage) }
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
	cfg.Flags.FilterLocal = filterLocal
	cfg.mergeFlags()
	return nil
}

func (cfg *Config) mergeFlags() {
	cfg.DiscoveryCfg.Listenbrainz.ImportPlaylist = cfg.Flags.Playlist
	cfg.DownloadCfg.FilterLocal = cfg.Flags.FilterLocal
}

func contains(valid []string, val string) bool {
	return slices.Contains(valid, val)
}