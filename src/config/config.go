package config

import (
	"log"
	"fmt"
	"time"
	"strings"
	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	DownloadCfg DownloadConfig
	DiscoveryCfg DiscoveryConfig
	ClientCfg ClientConfig
	Persist bool `env:"PERSIST" env-default:"true"`
	System string `env:"EXPLO_SYSTEM"`
	Debug bool `env:"DEBUG" env-default:"false"`
}

type ClientConfig struct {
	ClientID string `env:"CLIENT_ID" env-default:"explo"`
	LibraryName string `env:"LIBRARY_NAME" env-default:"Explo"`
	URL string `env:"SYSTEM_URL"`
	DownloadDir string `env:"DOWNLOAD_DIR" env-default:"/data/"`
	SlskdDir string `env:"SLSKD_DIR"`
	PlaylistDir string `env:"PLAYLIST_DIR"`
	PlaylistName string
	PlaylistID string
	Sleep int `env:"SLEEP" env-default:"2"`
	Creds Credentials
	Subsonic SubsonicConfig
}

type Credentials struct {
	APIKey string `env:"API_KEY"`
	User string `env:"SYSTEM_USERNAME"`
	Password string `env:"SYSTEM_PASSWORD"`
	Headers map[string]string
	Token string
	Salt string
}


type SubsonicConfig struct {
	Version	string `env:"SUBSONIC_VERSION" env-default:"1.16.1"`
	ID string `env:"CLIENT" env-default:"explo"`
	URL	string `env:"SUBSONIC_URL" env-default:"http://127.0.0.1:4533"`
	User string `env:"SUBSONIC_USER"`
	Password string `env:"SUBSONIC_PASSWORD"`
}

type DownloadConfig struct {
	DownloadDir string `env:"DOWNLOAD_DIR"`
	Youtube Youtube
	Slskd Slskd
	Discovery string `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	Services []string `env:"DOWNLOAD_SERVICES" env-default:"youtube"`
}

type Filters struct {
	Extensions []string `env:"EXTENSIONS" env-default:"flac,mp3"`
	MinBitDepth int `env:"MIN_BIT_DEPTH" env-default:"8"`
	MinBitRate int `env:"MIN_BITRATE" env-default:"256"`
	FilterList []string `env:"FILTER_LIST" env-default:"live,remix,instrumental,extended"`
}

type Youtube struct {
	APIKey string `env:"YOUTUBE_API_KEY"`
	Separator string `env:"FILENAME_SEPARATOR" env-default:" "`
	FfmpegPath string `env:"FFMPEG_PATH"`
	YtdlpPath string `env:"YTDLP_PATH"`
	Filters Filters
}

type Slskd struct {
	APIKey string `env:"SLSKD_API_KEY"`
	URL string `env:"SLSKD_URL"`
	Retry int `env:"SLSKD_RETRY" env-default:"5"` // Number of times to check search status before skipping the track
	DownloadAttempts int `env:"SLSKD_DL_ATTEMPTS" env-default:"2"` // Max number of files to attempt downloading per track
	Timeout time.Duration `env:"SLSKD_TIMEOUT" env-default:"20s"`
	Filters Filters
}

type DiscoveryConfig struct {
	Discovery string `env:"DISCOVERY_SERVICE" env-default:"listenbrainz"`
	Separator string `env:"FILENAME_SEPARATOR" env-default:" "`
	Listenbrainz Listenbrainz
}
type Listenbrainz struct {
	Discovery string `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	User string `env:"LISTENBRAINZ_USER"`
	SingleArtist bool `env:"SINGLE_ARTIST" env-default:"true"`
}

func ReadEnv() Config {
	var cfg Config
	if err := cleanenv.ReadConfig("./.env", &cfg); err != nil {
		log.Fatalf("Failed to load config: %s", err)
	}
	cfg.VerifyDir()
	return cfg
}

func (cfg *Config) VerifyDir() {
	if cfg.System == "mpd" {
		cfg.ClientCfg.PlaylistDir = fixDir(cfg.ClientCfg.PlaylistDir)
	}
	cfg.DownloadCfg.DownloadDir = fixDir(cfg.DownloadCfg.DownloadDir)
}

func fixDir(dir string) string {
	if !strings.HasSuffix(dir, "/") && dir != "" {
		return dir + "/"
	}
	return dir
}

/* func (cfg *Config) HandleDeprecation() { // no deprecations at the moment (keeping this for reference)
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