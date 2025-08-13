package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	DownloadCfg  DownloadConfig
	DiscoveryCfg DiscoveryConfig
	ClientCfg    ClientConfig
	Persist      bool   `env:"PERSIST" env-default:"true"`
	System       string `env:"EXPLO_SYSTEM"`
	Debug        bool   `env:"DEBUG" env-default:"false"`
}

type ClientConfig struct {
	ClientID     string `env:"CLIENT_ID" env-default:"explo"`
	LibraryName  string `env:"LIBRARY_NAME" env-default:"Explo"`
	URL          string `env:"SYSTEM_URL"`
	DownloadDir  string `env:"DOWNLOAD_DIR" env-default:"/data/"`
	PlaylistDir  string `env:"PLAYLIST_DIR"`
	PlaylistName string
	PlaylistID   string
	Sleep        int `env:"SLEEP" env-default:"2"`
	Creds        Credentials
	Subsonic     SubsonicConfig
}

type Credentials struct {
	APIKey   string `env:"API_KEY"`
	User     string `env:"SYSTEM_USERNAME"`
	Password string `env:"SYSTEM_PASSWORD"`
	Headers  map[string]string
	Token    string
	Salt     string
}

type DiscoveryConfig struct {
	Discovery    string `env:"DISCOVERY_SERVICE" env-default:"listenbrainz"`
	Separator    string `env:"FILENAME_SEPARATOR" env-default:" "`
	Listenbrainz Listenbrainz
}

type DownloadConfig struct {
	DownloadDir string `env:"DOWNLOAD_DIR"  env-default:"/data/"`
	Lidarr      Lidarr
	Slskd       Slskd
	Youtube     Youtube
	Discovery   string   `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	Services    []string `env:"DOWNLOAD_SERVICES" env-default:"youtube"`
}

type Filters struct {
	Extensions  []string `env:"EXTENSIONS" env-default:"flac,mp3"`
	MinBitDepth int      `env:"MIN_BIT_DEPTH" env-default:"8"`
	MinBitRate  int      `env:"MIN_BITRATE" env-default:"256"`
	FilterList  []string `env:"FILTER_LIST" env-default:"live,remix,instrumental,extended"`
}

type Listenbrainz struct {
	Discovery    string `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	User         string `env:"LISTENBRAINZ_USER"`
	SingleArtist bool   `env:"SINGLE_ARTIST" env-default:"true"`
}

type Lidarr struct {
	APIKey           string        `env:"LIDARR_API_KEY"`
	Retry            int           `env:"LIDARR_RETRY" env-default:"5"`       // Number of times to check search status before skipping the track
	DownloadAttempts int           `env:"LIDARR_DL_ATTEMPTS" env-default:"3"` // Max number of files to attempt downloading per track
	LidarrDir        string        `env:"LIDARR_DIR" env-default:"/lidarr/"`
	MigrateDL        bool          `env:"MIGRATE_DOWNLOADS" env-default:"false"` // Move downloads from SlskdDir to DownloadDir
	Timeout          time.Duration `env:"LIDARR_TIMEOUT" env-default:"20s"`
	Scheme           string        `env:"LIDARR_SCHEME" env-default:"http"`
	URL              string        `env:"LIDARR_URL"`
	Filters          Filters
}

type SubsonicConfig struct {
	Version  string `env:"SUBSONIC_VERSION" env-default:"1.16.1"`
	ID       string `env:"CLIENT" env-default:"explo"`
	URL      string `env:"SUBSONIC_URL" env-default:"http://127.0.0.1:4533"`
	User     string `env:"SUBSONIC_USER"`
	Password string `env:"SUBSONIC_PASSWORD"`
}

type Youtube struct {
	APIKey     string `env:"YOUTUBE_API_KEY"`
	FfmpegPath string `env:"FFMPEG_PATH"`
	YtdlpPath  string `env:"YTDLP_PATH"`
	Filters    Filters
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
}

func ReadEnv() Config {
	var cfg Config

	// Try to read from .env file first
	err := cleanenv.ReadConfig(".env", &cfg)
	if err != nil {
		// If the error is because the file doesn't exist, fallback to env vars
		if errors.Is(err, os.ErrNotExist) {
			if err := cleanenv.ReadEnv(&cfg); err != nil {
				log.Fatalf("Failed to load config from env vars: %s", err)
			}
		} else {
			log.Fatalf("Failed to load config: %s", err)
		}
	}

	cfg.VerifyDir()
	return cfg
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
func (cfg *Config) GetPlaylistName() { // Generate playlist name depending if user wants to keep it or not
	playlistName := "Discover-Weekly"
	if cfg.Persist {
		year, week := time.Now().ISOWeek()
		playlistName = fmt.Sprintf("%s-%d-Week%d", playlistName, year, week)
	}
	cfg.ClientCfg.PlaylistName = playlistName
}
