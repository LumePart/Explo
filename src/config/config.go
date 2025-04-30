package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type Config struct {
	DownloadCfg  DownloadConfig
	DiscoveryCfg DiscoveryConfig
	ClientCfg    ClientConfig
	Flags        Flags
	Persist      bool   `env:"PERSIST" env-default:"true"`
	System       string `env:"EXPLO_SYSTEM"`
	Debug        bool   `env:"DEBUG" env-default:"false"`
	Timeout      int    `env:"TIMEOUT" env-default:"10"`
}

type Flags struct {
	CfgPath      string
	Playlist     string
	DownloadMode string
	ExcludeLocal bool
	Timeout      int `env:"TIMEOUT" env-default:"10"`
}

type ClientConfig struct {
	ClientID      string `env:"CLIENT_ID" env-default:"explo"`
	LibraryName   string `env:"LIBRARY_NAME" env-default:"Explo"`
	URL           string `env:"SYSTEM_URL"`
	DownloadDir   string `env:"DOWNLOAD_DIR" env-default:"/data/"`
	PlaylistDir   string `env:"PLAYLIST_DIR"`
	PlaylistName  string
	PlaylistDescr string
	PlaylistID    string
	Sleep         int `env:"SLEEP" env-default:"2"`
	Creds         Credentials
	Subsonic      SubsonicConfig
}

type Credentials struct {
	APIKey   string `env:"API_KEY"`
	User     string `env:"SYSTEM_USERNAME"`
	Password string `env:"SYSTEM_PASSWORD"`
	Headers  map[string]string
	Token    string
	Salt     string
}

type SubsonicConfig struct {
	Version  string `env:"SUBSONIC_VERSION" env-default:"1.16.1"`
	ID       string `env:"CLIENT" env-default:"explo"`
	URL      string `env:"SUBSONIC_URL" env-default:"http://127.0.0.1:4533"`
	User     string `env:"SUBSONIC_USER"`
	Password string `env:"SUBSONIC_PASSWORD"`
}

type DownloadConfig struct {
	DownloadDir  string `env:"DOWNLOAD_DIR" env-default:"/data/"`
	Youtube      Youtube
	YoutubeMusic YoutubeMusic
	Slskd        Slskd
	Lidarr       Lidarr
	ExcludeLocal bool
	Discovery    string   `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	Services     []string `env:"DOWNLOAD_SERVICES" env-default:"youtube"`
}

type Filters struct {
	Extensions  []string `env:"EXTENSIONS" env-default:"flac,mp3"`
	MinBitDepth int      `env:"MIN_BIT_DEPTH" env-default:"8"`
	MinBitRate  int      `env:"MIN_BITRATE" env-default:"256"`
	FilterList  []string `env:"FILTER_LIST" env-default:"live,remix,instrumental,extended"`
}

type Youtube struct {
	APIKey     string `env:"YOUTUBE_API_KEY"`
	FfmpegPath string `env:"FFMPEG_PATH"`
	YtdlpPath  string `env:"YTDLP_PATH"`
	Filters    Filters
}

type YoutubeMusic struct {
	FfmpegPath string `env:"FFMPEG_PATH"`
	YtdlpPath  string `env:"YTDLP_PATH"`
	Filters    Filters
}

type Lidarr struct {
	APIKey     string   `env:"LIDARR_API_KEY"`
	Separator  string   `env:"FILENAME_SEPARATOR" env-default:" "`
	FilterList []string `env:"FILTER_LIST" env-default:"live,remix,instrumental,extended"`
	Scheme     string   `env:"LIDARR_SCHEME" env-default:"http"`
	URL        string   `env:"LIDARR_URL"`
}

type Slskd struct {
	APIKey           string        `env:"SLSKD_API_KEY"`
	URL              string        `env:"SLSKD_URL"`
	Retry            int           `env:"SLSKD_RETRY" env-default:"5"`       // Number of times to check search status before skipping the track
	DownloadAttempts int           `env:"SLSKD_DL_ATTEMPTS" env-default:"3"` // Max number of files to attempt downloading per track
	SlskdDir         string        `env:"SLSKD_DIR" env-default:"/slskd/"`
	MigrateDL        bool          `env:"MIGRATE_DOWNLOADS" env-default:"false"` // Move downloads from SlskdDir to DownloadDir
	Timeout          time.Duration `env:"SLSKD_TIMEOUT" env-default:"20s"`
	Filters          Filters
	MonitorConfig    SlskdMon
}

type SlskdMon struct {
	Interval time.Duration `env:"SLSKD_MONITOR_INTERVAL" env-default:"1m"`
	Duration time.Duration `env:"SLSKD_MONITOR_DURATION" env-default:"15m"`
}

type DiscoveryConfig struct {
	Discovery    string `env:"DISCOVERY_SERVICE" env-default:"listenbrainz"`
	Listenbrainz Listenbrainz
}
type Listenbrainz struct {
	Discovery      string `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	User           string `env:"LISTENBRAINZ_USER"`
	ImportPlaylist string
	SingleArtist   bool `env:"SINGLE_ARTIST" env-default:"true"`
}

func (cfg *Config) ReadEnv() {

	// Try to read from .env file first
	err := cleanenv.ReadConfig(cfg.Flags.CfgPath, cfg)
	if err != nil {
		// If the error is because the file doesn't exist, fallback to env vars
		if errors.Is(err, os.ErrNotExist) {
			if err := cleanenv.ReadEnv(&cfg); err != nil {
				log.Fatalf("failed to load config from env vars: %s", err)
			}
		} else {
			log.Fatalf("failed to load config file %s: %s", cfg.Flags.CfgPath, err)
		}
	}

	cfg.VerifyDir()
}

func (cfg *Config) VerifyDir() {
	if cfg.System == "mpd" {
		cfg.ClientCfg.PlaylistDir = fixDir(cfg.ClientCfg.PlaylistDir)
	}
	cfg.DownloadCfg.Slskd.SlskdDir = fixDir(cfg.DownloadCfg.Slskd.SlskdDir)
	cfg.DownloadCfg.DownloadDir = fixDir(cfg.DownloadCfg.DownloadDir)
}

func fixDir(dir string) string {
	if !strings.HasSuffix(dir, "/") && dir != "" {
		return dir + "/"
	}
	return dir
}

/*
	 func (cfg *Config) HandleDeprecation() { // no deprecations at the moment (keeping this for reference)
		switch cfg.System {
		case "subsonic":
			if cfg.Subsonic.User != "" && cfg.Creds.User == "" {
				log.Println("Warning: 'SUBSONIC_USER' is deprecated. Please use 'SYSTEM_USERNAME'.")
				cfg.Creds.User = cfg.Subsonic.User
			}
			if cfg.Subsonic.Password != "" && cfg.Creds.Password == "" {
				log.Println("Warning: 'SUBSONIC_PASSWORD' is deprecated. Please use 'SYSTEM_PASSWORD'.")
				cfg.Creds.Password = cfg.Subsonic.Password
			}
			if cfg.Subsonic.URL != "" && cfg.URL == "" {
				log.Println("Warning: 'SUBSONIC_URL' is deprecated. Please use 'SYSTEM_URL'.")
				cfg.URL = cfg.Subsonic.URL
			}
		}
	}
*/
func (cfg *Config) GetPlaylistName() { // Generate playlist name and description

	toTitle := cases.Title(language.Und)

	playlistName := toTitle.String(cfg.Flags.Playlist)
	if cfg.Persist {
		year, week := time.Now().ISOWeek()
		if cfg.Flags.Playlist != "daily-jams" {
			playlistName = fmt.Sprintf("%s-%d-Week%d", playlistName, year, week)
		} else {
			day := time.Now().YearDay()
			playlistName = fmt.Sprintf("%s-%d-Day%d", playlistName, year, day)
		}
	}
	cfg.ClientCfg.PlaylistDescr = fmt.Sprintf("Created for %s by Explo, using ListenBrainz recommendations.", cfg.DiscoveryCfg.Listenbrainz.User)
	cfg.ClientCfg.PlaylistName = playlistName

	// add playlist name to downloadDir so all songs get downloaded into a single place
	cfg.DownloadCfg.DownloadDir = fmt.Sprintf("%s%s/", cfg.DownloadCfg.DownloadDir, playlistName)
}
