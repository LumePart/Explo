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
	"github.com/joho/godotenv"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var cleanenvReadEnv = cleanenv.ReadEnv

type Config struct {
	DownloadCfg DownloadConfig
	DiscoveryCfg DiscoveryConfig
	ClientCfg ClientConfig
	NotifyCfg NotifyConfig
	Flags Flags 
	PersistENV bool `env:"PERSIST" env-default:"true"`
	Persist bool
	System string `env:"EXPLO_SYSTEM"`
	Debug bool `env:"DEBUG" env-default:"false"`
	LogLevel string `env:"LOG_LEVEL" env-default:"INFO"`
}

type Flags struct {
	CfgPath string
	Playlist string
	DownloadMode string
	ExcludeLocal bool
	Persist bool
	PersistSet bool
}

type ClientConfig struct {
	ClientID string `env:"CLIENT_ID" env-default:"explo"`
	LibraryName string `env:"LIBRARY_NAME" env-default:"Explo"`
	URL string `env:"SYSTEM_URL"`
	DownloadDir string `env:"DOWNLOAD_DIR" env-default:"/data/"`
	PlaylistDir string `env:"PLAYLIST_DIR"`
	PlaylistName string
	PlaylistNFormat string `env:"PLAYLISTNAME_FORMAT" env-default:"week"`
	PlaylistDescr string
	PlaylistID string
	Sleep int `env:"SLEEP" env-default:"2"`
	HTTPTimeout int `env:"CLIENT_HTTP_TIMEOUT" env-default:"10"`
	Creds Credentials
	AdminCreds AdminCredentials
	Subsonic SubsonicConfig
}

type Credentials struct {
	APIKey string `env:"API_KEY"`
	User string `env:"SYSTEM_USERNAME"`
	Password string `env:"SYSTEM_PASSWORD"`
	Listenbrainz string `env:"LISTENBRAINZ_TOKEN"`
	Headers map[string]string
	Token string
	Salt string
}

type AdminCredentials struct {
	User string `env:"ADMIN_SYSTEM_USERNAME"`
	Password string `env:"ADMIN_SYSTEM_PASSWORD"`
}


type SubsonicConfig struct {
	Version	string `env:"SUBSONIC_VERSION" env-default:"1.16.1"`
	ID string `env:"CLIENT" env-default:"explo"`
	PublicPlaylist bool `env:"PUBLIC_PLAYLIST" env-default:"false"`
}

type DownloadConfig struct {
	DownloadDir string `env:"DOWNLOAD_DIR" env-default:"/data/"`
	Youtube Youtube
	YoutubeMusic YoutubeMusic
	Slskd Slskd
	ExcludeLocal bool
	KeepPermissions bool `env:"KEEP_PERMISSIONS" env-default:"true"` // keep original file permissions when migrating download
	RenameTrack bool `env:"RENAME_TRACK" env-default:"false"` // Rename track in {title}-{artist} format
	UseSubDir bool `env:"USE_SUBDIRECTORY" env-default:"true"`
	Discovery string `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	Services []string `env:"DOWNLOAD_SERVICES" env-default:"youtube"`
}

type Filters struct {
	Extensions []string `env:"EXTENSIONS" env-default:"flac,mp3"`
	MinBitDepth int `env:"MIN_BIT_DEPTH" env-default:"8"`
	MinBitRate int `env:"MIN_BITRATE" env-default:"256"`
	FilterList []string `env:"FILTER_LIST" env-default:"live,remix,instrumental,extended,clean,acapella"`
}

type Youtube struct {
	APIKey string `env:"YOUTUBE_API_KEY"`
	FfmpegPath string `env:"FFMPEG_PATH"`
	YtdlpPath string `env:"YTDLP_PATH"`
	FileExtension string `env:"TRACK_EXTENSION" env-default:"opus"`
	CookiesPath string `env:"COOKIES_PATH" env-default:"./cookies.txt"`
	Filters Filters
}

type YoutubeMusic struct {
	FfmpegPath string `env:"FFMPEG_PATH"`
	YtdlpPath string `env:"YTDLP_PATH"`
	Filters Filters
}

type Slskd struct {
	APIKey string `env:"SLSKD_API_KEY"`
	URL string `env:"SLSKD_URL"`
	Retry int `env:"SLSKD_RETRY" env-default:"5"` // Number of times to check search status before skipping the track
	DownloadAttempts int `env:"SLSKD_DL_ATTEMPTS" env-default:"3"` // Max number of files to attempt downloading per track
	SlskdDir string `env:"SLSKD_DIR" env-default:"/slskd/"`
	MigrateDL bool `env:"MIGRATE_DOWNLOADS" env-default:"false"` // Move downloads from SlskdDir to DownloadDir
	Timeout int `env:"SLSKD_TIMEOUT" env-default:"20"`
	Filters Filters
	MonitorConfig SlskdMon
}

type SlskdMon struct {
	Interval time.Duration `env:"SLSKD_MONITOR_INTERVAL" env-default:"1m"`
	Duration time.Duration `env:"SLSKD_MONITOR_DURATION" env-default:"15m"`
}

type DiscoveryConfig struct {
	Discovery string `env:"DISCOVERY_SERVICE" env-default:"listenbrainz"`
	Listenbrainz Listenbrainz
}
type Listenbrainz struct {
	Discovery string `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	User string `env:"LISTENBRAINZ_USER"`
	ImportPlaylist string
	SingleArtist bool `env:"SINGLE_ARTIST" env-default:"true"`
}

type NotifyConfig struct {
	Matrix MatrixNotif
	Discord DiscordNotif
	Http HttpNotif
}

type MatrixNotif struct {
	UserID string `env:"MATRIX_USERID"`
	RoomID string `env:"MATRIX_ROOMID"`
	HomeServer string `env:"MATRIX_HOMESERVER_URL"`
	AccessToken string `env:"MATRIX_ACCESSTOKEN"`
}

type DiscordNotif struct {
	BotToken string `env:"DISCORD_BOT_TOKEN"`
	ChannelIDs []string `env:"DISCORD_CHANNEL_ID"`
}

type HttpNotif struct {
	ReceiverURLs []string `env:"HTTP_RECEIVER"`
}
func (cfg *Config) ReadEnv() {
	if cfg == nil {
		slog.Error("config is nil")
		os.Exit(1)
	}

	if cfg.Flags.CfgPath == "" {
		cfg.Flags.CfgPath = ".env"
	}

	// Load .env into the process environment if present.
	err := godotenv.Load(cfg.Flags.CfgPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Error("failed to load config file", "path", cfg.Flags.CfgPath, "context", err.Error())
		os.Exit(1)
	}
	if errors.Is(err, os.ErrNotExist) {
		slog.Debug("config file not found, using process environment only", "path", cfg.Flags.CfgPath)
	}

	// Read from process env so Docker/container variables are always considered.
	if err := cleanenvReadEnv(cfg); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "wrong type ptr") {
			slog.Warn("cleanenv pointer type failure, applying manual env fallback", "context", err.Error())
			cfg.applyManualEnvFallback()
		} else {
			slog.Error("failed to load config from env vars", "context", err.Error())
			os.Exit(1)
		}
	}

	cfg.CommonFixes()
}

func readEnvTrimmed(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func (cfg *Config) applyManualEnvFallback() {
	if cfg.System == "" {
		cfg.System = readEnvTrimmed("MUSIC_SYSTEM_TYPE")
	}
	if cfg.ClientCfg.URL == "" {
		cfg.ClientCfg.URL = readEnvTrimmed("MUSIC_SYSTEM_URL")
	}
	if cfg.ClientCfg.Creds.APIKey == "" {
		cfg.ClientCfg.Creds.APIKey = readEnvTrimmed("MUSIC_SYSTEM_TOKEN")
	}
	if cfg.DiscoveryCfg.Listenbrainz.User == "" {
		cfg.DiscoveryCfg.Listenbrainz.User = readEnvTrimmed("LISTENBRAINZ_USER")
	}
	if cfg.ClientCfg.Creds.Listenbrainz == "" {
		cfg.ClientCfg.Creds.Listenbrainz = readEnvTrimmed("LISTENBRAINZ_TOKEN")
	}
	if len(cfg.DownloadCfg.Services) == 0 {
		downloadType := readEnvTrimmed("DOWNLOAD_TYPE")
		if downloadType != "" {
			cfg.DownloadCfg.Services = []string{downloadType}
		}
	}
	if cfg.DownloadCfg.DownloadDir == "" {
		cfg.DownloadCfg.DownloadDir = readEnvTrimmed("DOWNLOAD_DIR")
	}
}

func (cfg *Config) CommonFixes() {
	cfg.TrimEnvValues()
	cfg.ResolveSystemEnv()
	cfg.DownloadCfg.Youtube.FileExtension = strings.TrimPrefix(cfg.DownloadCfg.Youtube.FileExtension, ".")
	cfg.ClientCfg.URL = strings.TrimSuffix(cfg.ClientCfg.URL, "/")
	cfg.NormalizeDir()
}

func (cfg *Config) ResolveSystemEnv() {
	if cfg.System == "" {
		cfg.System = os.Getenv("EXPLO_SYSTEM")
	}
	if cfg.System == "" {
		cfg.System = os.Getenv("MUSIC_SYSTEM_TYPE")
	}
	cfg.System = strings.ToLower(strings.TrimSpace(cfg.System))
}

func (cfg *Config) TrimEnvValues() {
	cfg.System = strings.TrimSpace(cfg.System)
	cfg.LogLevel = strings.TrimSpace(cfg.LogLevel)

	cfg.ClientCfg.ClientID = strings.TrimSpace(cfg.ClientCfg.ClientID)
	cfg.ClientCfg.LibraryName = strings.TrimSpace(cfg.ClientCfg.LibraryName)
	cfg.ClientCfg.URL = strings.TrimSpace(cfg.ClientCfg.URL)
	cfg.ClientCfg.DownloadDir = strings.TrimSpace(cfg.ClientCfg.DownloadDir)
	cfg.ClientCfg.PlaylistDir = strings.TrimSpace(cfg.ClientCfg.PlaylistDir)
	cfg.ClientCfg.PlaylistNFormat = strings.TrimSpace(cfg.ClientCfg.PlaylistNFormat)

	cfg.ClientCfg.Creds.APIKey = strings.TrimSpace(cfg.ClientCfg.Creds.APIKey)
	cfg.ClientCfg.Creds.User = strings.TrimSpace(cfg.ClientCfg.Creds.User)
	cfg.ClientCfg.Creds.Password = strings.TrimSpace(cfg.ClientCfg.Creds.Password)
	cfg.ClientCfg.Creds.Listenbrainz = strings.TrimSpace(cfg.ClientCfg.Creds.Listenbrainz)
	cfg.ClientCfg.AdminCreds.User = strings.TrimSpace(cfg.ClientCfg.AdminCreds.User)
	cfg.ClientCfg.AdminCreds.Password = strings.TrimSpace(cfg.ClientCfg.AdminCreds.Password)

	cfg.ClientCfg.Subsonic.Version = strings.TrimSpace(cfg.ClientCfg.Subsonic.Version)
	cfg.ClientCfg.Subsonic.ID = strings.TrimSpace(cfg.ClientCfg.Subsonic.ID)

	cfg.DownloadCfg.DownloadDir = strings.TrimSpace(cfg.DownloadCfg.DownloadDir)
	cfg.DownloadCfg.Discovery = strings.TrimSpace(cfg.DownloadCfg.Discovery)
	cfg.DownloadCfg.Services = trimStrings(cfg.DownloadCfg.Services)

	cfg.DownloadCfg.Youtube.APIKey = strings.TrimSpace(cfg.DownloadCfg.Youtube.APIKey)
	cfg.DownloadCfg.Youtube.FfmpegPath = strings.TrimSpace(cfg.DownloadCfg.Youtube.FfmpegPath)
	cfg.DownloadCfg.Youtube.YtdlpPath = strings.TrimSpace(cfg.DownloadCfg.Youtube.YtdlpPath)
	cfg.DownloadCfg.Youtube.FileExtension = strings.TrimSpace(cfg.DownloadCfg.Youtube.FileExtension)
	cfg.DownloadCfg.Youtube.CookiesPath = strings.TrimSpace(cfg.DownloadCfg.Youtube.CookiesPath)
	cfg.DownloadCfg.Youtube.Filters.Extensions = trimStrings(cfg.DownloadCfg.Youtube.Filters.Extensions)
	cfg.DownloadCfg.Youtube.Filters.FilterList = trimStrings(cfg.DownloadCfg.Youtube.Filters.FilterList)

	cfg.DownloadCfg.YoutubeMusic.FfmpegPath = strings.TrimSpace(cfg.DownloadCfg.YoutubeMusic.FfmpegPath)
	cfg.DownloadCfg.YoutubeMusic.YtdlpPath = strings.TrimSpace(cfg.DownloadCfg.YoutubeMusic.YtdlpPath)
	cfg.DownloadCfg.YoutubeMusic.Filters.Extensions = trimStrings(cfg.DownloadCfg.YoutubeMusic.Filters.Extensions)
	cfg.DownloadCfg.YoutubeMusic.Filters.FilterList = trimStrings(cfg.DownloadCfg.YoutubeMusic.Filters.FilterList)

	cfg.DownloadCfg.Slskd.APIKey = strings.TrimSpace(cfg.DownloadCfg.Slskd.APIKey)
	cfg.DownloadCfg.Slskd.URL = strings.TrimSpace(cfg.DownloadCfg.Slskd.URL)
	cfg.DownloadCfg.Slskd.SlskdDir = strings.TrimSpace(cfg.DownloadCfg.Slskd.SlskdDir)
	cfg.DownloadCfg.Slskd.Filters.Extensions = trimStrings(cfg.DownloadCfg.Slskd.Filters.Extensions)
	cfg.DownloadCfg.Slskd.Filters.FilterList = trimStrings(cfg.DownloadCfg.Slskd.Filters.FilterList)

	cfg.DiscoveryCfg.Discovery = strings.TrimSpace(cfg.DiscoveryCfg.Discovery)
	cfg.DiscoveryCfg.Listenbrainz.Discovery = strings.TrimSpace(cfg.DiscoveryCfg.Listenbrainz.Discovery)
	cfg.DiscoveryCfg.Listenbrainz.User = strings.TrimSpace(cfg.DiscoveryCfg.Listenbrainz.User)

	cfg.NotifyCfg.Matrix.UserID = strings.TrimSpace(cfg.NotifyCfg.Matrix.UserID)
	cfg.NotifyCfg.Matrix.RoomID = strings.TrimSpace(cfg.NotifyCfg.Matrix.RoomID)
	cfg.NotifyCfg.Matrix.HomeServer = strings.TrimSpace(cfg.NotifyCfg.Matrix.HomeServer)
	cfg.NotifyCfg.Matrix.AccessToken = strings.TrimSpace(cfg.NotifyCfg.Matrix.AccessToken)
	cfg.NotifyCfg.Discord.BotToken = strings.TrimSpace(cfg.NotifyCfg.Discord.BotToken)
	cfg.NotifyCfg.Discord.ChannelIDs = trimStrings(cfg.NotifyCfg.Discord.ChannelIDs)
	cfg.NotifyCfg.Http.ReceiverURLs = trimStrings(cfg.NotifyCfg.Http.ReceiverURLs)
}

func trimStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}

	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		trimmed = append(trimmed, value)
	}

	return trimmed
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