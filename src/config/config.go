package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var Version = "dev"

type Config struct {
	DownloadCfg  DownloadConfig
	DiscoveryCfg DiscoveryConfig
	ClientCfg    ClientConfig
	NotifyCfg    NotifyConfig
	ServerCfg    ServerConfig
	Flags        Flags
	PersistENV   bool `env:"PERSIST" env-default:"true"`
	Persist      bool
	System       string `env:"EXPLO_SYSTEM"`
	Debug        bool   `env:"DEBUG" env-default:"false"`
	LogLevel     string `env:"LOG_LEVEL" env-default:"INFO"`
}

type Flags struct {
	CfgPath      string
	CfgSet       bool
	Playlist     string
	DownloadMode string
	ExcludeLocal bool
	Persist      bool
	PersistSet   bool
}

type ServerConfig struct {
	Enabled     bool   `env:"WEB_UI" env-default:"false"`
	Port        string `env:"WEB_ADDR" env-default:":7288"`
	Username    string `env:"UI_USERNAME"`
	Password    string `env:"UI_PASSWORD"`
	WebDataDir  string `env:"WEB_DATA_PATH" env-default:"/opt/explo/config/"`
	WebEnvPath  string `env:"WEB_ENV_PATH" env-default:"/opt/explo/.env"`
	CacheSizeMB int64  `env:"WEB_CACHE_MB" env-default:"500"`
	ExploPath   string
}

type ClientConfig struct {
	ClientID        string `env:"CLIENT_ID" env-default:"explo"`
	LibraryName     string `env:"LIBRARY_NAME" env-default:"Explo"`
	URL             string `env:"SYSTEM_URL"`
	DownloadDir     string `env:"DOWNLOAD_DIR" env-default:"/data/"`
	PlaylistDir     string `env:"PLAYLIST_DIR"`
	PlaylistName    string
	PlaylistNFormat string `env:"PLAYLISTNAME_FORMAT" env-default:"week"`
	PlaylistDescr   string
	PlaylistID      string
	Sleep           int `env:"SLEEP" env-default:"2"`
	HTTPTimeout     int `env:"CLIENT_HTTP_TIMEOUT" env-default:"10"`
	Creds           Credentials
	AdminCreds      AdminCredentials
	Subsonic        SubsonicConfig
}

type Credentials struct {
	APIKey   string `env:"API_KEY"`
	User     string `env:"SYSTEM_USERNAME"`
	Password string `env:"SYSTEM_PASSWORD"`
	Headers  map[string]string
	Token    string
	Salt     string
}

type AdminCredentials struct {
	User     string `env:"ADMIN_SYSTEM_USERNAME"`
	Password string `env:"ADMIN_SYSTEM_PASSWORD"`
}

type SubsonicConfig struct {
	Version        string `env:"SUBSONIC_VERSION" env-default:"1.16.1"`
	ID             string `env:"CLIENT" env-default:"explo"`
	PublicPlaylist bool   `env:"PUBLIC_PLAYLIST" env-default:"false"`
}

type DownloadConfig struct {
	DownloadDir     string `env:"DOWNLOAD_DIR" env-default:"/data/"`
	Youtube         Youtube
	YoutubeMusic    YoutubeMusic
	Slskd           Slskd
	ExcludeLocal    bool
	KeepPermissions bool     `env:"KEEP_PERMISSIONS" env-default:"true"` // keep original file permissions when migrating download
	RenameTrack     bool     `env:"RENAME_TRACK" env-default:"false"`    // Rename track in {title}-{artist} format
	UseSubDir       bool     `env:"USE_SUBDIRECTORY" env-default:"true"`
	Discovery       string   `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	Services        []string `env:"DOWNLOAD_SERVICES" env-default:"youtube"`
}

type Filters struct {
	Extensions  []string `env:"EXTENSIONS" env-default:"flac,mp3"` // slskd
	MinBitDepth int      `env:"MIN_BIT_DEPTH" env-default:"8"`
	MinBitRate  int      `env:"MIN_BITRATE" env-default:"256"`
	FilterList  []string `env:"FILTER_LIST" env-default:"live,remix,instrumental,extended,clean,acapella"`
}

type Youtube struct {
	APIKey        string `env:"YOUTUBE_API_KEY"`
	FfmpegPath    string `env:"FFMPEG_PATH"`
	YtdlpPath     string `env:"YTDLP_PATH"`
	FileExtension string `env:"TRACK_EXTENSION" env-default:"mp3"` // yt-dlp
	EmbedCoverArt bool   `env:"EMBED_COVER_ART" env-default:"false"`
	CookiesPath   string `env:"COOKIES_PATH" env-default:"./cookies.txt"`
	Filters       Filters
	CoversDir     string
}

type YoutubeMusic struct {
	FfmpegPath string `env:"FFMPEG_PATH"`
	YtdlpPath  string `env:"YTDLP_PATH"`
	Filters    Filters
}

type Slskd struct {
	APIKey           string `env:"SLSKD_API_KEY"`
	URL              string `env:"SLSKD_URL"`
	Retry            int    `env:"SLSKD_RETRY" env-default:"5"`       // Number of times to check search status before skipping the track
	DownloadAttempts int    `env:"SLSKD_DL_ATTEMPTS" env-default:"3"` // Max number of files to attempt downloading per track
	SlskdDir         string `env:"SLSKD_DIR" env-default:"/slskd/"`
	MigrateDL        bool   `env:"MIGRATE_DOWNLOADS" env-default:"false"` // Move downloads from SlskdDir to DownloadDir
	Timeout          int    `env:"SLSKD_TIMEOUT" env-default:"20"`
	Filters          Filters
	MonitorConfig    SlskdMon
}

type SlskdMon struct {
	Interval int `env:"SLSKD_MONITOR_INTERVAL" env-default:"1"`
	Duration int `env:"SLSKD_MONITOR_DURATION" env-default:"15"`
}

type DiscoveryConfig struct {
	Discovery    string `env:"DISCOVERY_SERVICE" env-default:"listenbrainz"`
	Listenbrainz Listenbrainz
}
type Listenbrainz struct {
	Discovery              string `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	User                   string `env:"LISTENBRAINZ_USER"`
	ImportPlaylist         string
	SingleArtist           bool   `env:"SINGLE_ARTIST" env-default:"true"`
	CoverArtSize           string `env:"COVER_ART_SIZE" env-default:"250"`
	EnrichTrackMetadata	   bool   `env:"ENRICH_TRACK_METADATA" env-default:"false"`
}

type NotifyConfig struct {
	Matrix  MatrixNotif
	Discord DiscordNotif
	Http    HttpNotif
}

type MatrixNotif struct {
	UserID      string `env:"MATRIX_USERID"`
	RoomID      string `env:"MATRIX_ROOMID"`
	HomeServer  string `env:"MATRIX_HOMESERVER_URL"`
	AccessToken string `env:"MATRIX_ACCESSTOKEN"`
}

type DiscordNotif struct {
	BotToken   string   `env:"DISCORD_BOT_TOKEN"`
	ChannelIDs []string `env:"DISCORD_CHANNEL_ID"`
}

type HttpNotif struct {
	ReceiverURLs []string `env:"HTTP_RECEIVER"`
}

func (cfg *Config) ReadEnv() {
	// fallback in case the user adds path/.env as a directory
	if info, err := os.Stat(cfg.Flags.CfgPath); err == nil && info.IsDir() {
		cfg.Flags.CfgPath = filepath.Join(cfg.Flags.CfgPath, ".env")
		slog.Warn("config path is a directory, using .env inside it", "path", cfg.Flags.CfgPath)
	}

	// Try to read from .env file first
	err := cleanenv.ReadConfig(cfg.Flags.CfgPath, cfg)
	if err != nil {
		// If the error is because the file doesn't exist, fallback to env vars
		if errors.Is(err, os.ErrNotExist) {
			slog.Warn("no config file found, creating empty one", "path", cfg.Flags.CfgPath)
			if f, err := os.Create(cfg.Flags.CfgPath); err != nil {
				slog.Warn("could not create config file", "path", cfg.Flags.CfgPath, "context", err.Error())
			} else if err := f.Close(); err != nil {
				slog.Warn("could not close config file", "path", cfg.Flags.CfgPath, "context", err.Error())
			}
			if err := cleanenv.ReadConfig(cfg.Flags.CfgPath, cfg); err != nil {
				slog.Error("failed to load config file", "path", cfg.Flags.CfgPath, "context", err.Error())
				os.Exit(1)
			}
		} else {
			slog.Error("failed to load config file", "path", cfg.Flags.CfgPath, "context", err.Error())
			os.Exit(1)
		}
	}

	cfg.CommonFixes()
}

func (cfg *Config) CommonFixes() {
	cfg.DownloadCfg.Youtube.FileExtension = strings.TrimPrefix(cfg.DownloadCfg.Youtube.FileExtension, ".")
	cfg.DownloadCfg.Youtube.CoversDir = filepath.Join(filepath.Dir(cfg.Flags.CfgPath), "cache", "covers")
	cfg.ClientCfg.URL = fixBaseURL(cfg.ClientCfg.URL)
	cfg.DownloadCfg.Slskd.URL = fixBaseURL(cfg.DownloadCfg.Slskd.URL)
	cfg.NormalizeDir()
}

func (cfg *Config) NormalizeDir() {
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

func fixBaseURL(rawURL string) string {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return ""
	}
	if !strings.Contains(u, "://") {
		u = "http://" + u
	}
	return strings.TrimRight(u, "/")
}

func (cfg *Config) HandleDeprecation() { //
	if cfg.Debug {
		slog.Warn("'DEBUG' variable is deprecated, please use LOG_LEVEL=DEBUG instead")
		cfg.LogLevel = "DEBUG"
	}
	if !cfg.PersistENV {
		slog.Warn("'PERSIST' variable is deprecated, use --persist flag instead")
	}

	if !cfg.Persist && !cfg.DownloadCfg.UseSubDir {
		slog.Warn("Deleting tracks requires 'USE_SUBDIRECTORY' to be true")
	}
}

func (cfg *Config) GenPlaylistName() { // Generate playlist name and description

	cfg.ClientCfg.PlaylistName = getPlaylistName(cfg.Flags.Playlist, cfg.ClientCfg.PlaylistNFormat, cfg.Persist)
	cfg.ClientCfg.PlaylistDescr = fmt.Sprintf(
		"Created for %s by Explo, using ListenBrainz recommendations.",
		cfg.DiscoveryCfg.Listenbrainz.User)

	if cfg.DownloadCfg.UseSubDir {
		// add playlist name to downloadDir so all songs get downloaded to a single sub directory.
		cfg.DownloadCfg.DownloadDir = filepath.Join(
			cfg.DownloadCfg.DownloadDir,
			cfg.ClientCfg.PlaylistName)
	}
}

func getPlaylistName(playlistType, format string, persist bool) string {
	now := time.Now()

	toTitle := cases.Title(language.Und)
	base := toTitle.String(playlistType)

	// Non-persistent playlists always use base name
	if !persist {
		return base
	}

	// Explicit date-based naming
	if format == "date" {
		return fmt.Sprintf(
			"%s-%s",
			base,
			now.Format("2006-01-02"),
		)
	}

	// Persistent, non-date naming
	if playlistType == "daily-jams" {
		return fmt.Sprintf(
			"%s-%d-Day%d",
			base,
			now.Year(),
			now.YearDay(),
		)
	}

	year, week := now.ISOWeek()
	return fmt.Sprintf(
		"%s-%d-Week%d",
		base,
		year,
		week,
	)
}
