package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"io"
	"github.com/ilyakaznacheev/cleanenv"
	"net/http"
)

type Config struct {
	Subsonic Subsonic
	Jellyfin Jellyfin
	Youtube Youtube
	Listenbrainz Listenbrainz
	Creds Credentials
	URL string `env:"SERVER_URL"`
	Sleep int `env:"SLEEP" env-default:"1"`
	PlaylistDir string `env:"PLAYLIST_DIR"`
	Persist bool `env:"PERSIST" env-default:"true"`
	Client string `env:"CLIENT" env-default:"explo"`
	System string `env:"SYSTEM"`
	PlaylistName string
}

type Credentials struct {
	APIKey string `env:"API_KEY"`
	User string `env:"USER"`
	Password string `env:"PASSWORD"`
	Token string
	Salt string
}


type Jellyfin struct {
	Creds Credentials
	Source string `env:"JELLYFIN_SOURCE"`
}

type Subsonic struct {
	Version	string `env:"SUBSONIC_VERSION" env-default:"1.16.1"`
	ID string `env:"CLIENT" env-default:"explo"`
	URL	string `env:"SUBSONIC_URL" env-default:"http://127.0.0.1:4533"`
	Creds Credentials
	User string `env:"SUBSONIC_USER"`
	Password string `env:"SUBSONIC_PASSWORD"`
}

type Youtube struct {
	APIKey string `env:"YOUTUBE_API_KEY"`
	DownloadDir string `env:"DOWNLOAD_DIR"`
	Separator string `env:"FILENAME_SEPARATOR" env-default:" "`
	FfmpegPath string `env:"FFMPEG_PATH"`
}
type Listenbrainz struct {
	Discovery string `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	User string `env:"LISTENBRAINZ_USER"`
}

func (cfg *Config) handleDeprecation() { // assign deprecared env vars to new ones
	// Deprecated since v0.6.0
	if cfg.Subsonic.User != "" && cfg.Subsonic.Creds.User == "" {
		log.Println("Warning: 'SUBSONIC_USER' is deprecated. Please use 'USER' instead.")
		cfg.Subsonic.Creds.User = cfg.Subsonic.User
	}
	if cfg.Subsonic.Password != "" && cfg.Subsonic.Creds.Password == "" {
		log.Println("Warning: 'SUBSONIC_PASSWORD' is deprecated. Please use 'PASSWORD' instead.")
		cfg.Subsonic.Creds.Password = cfg.Subsonic.Password
	}
	if cfg.Subsonic.URL != "" && cfg.URL == "" {
		log.Println("Warning: 'SUBSONIC_URL' is deprecated. Please use 'URL' instead.")
		cfg.URL = cfg.Subsonic.URL
	}
}

func readEnv() Config {
	var cfg Config

	err := cleanenv.ReadConfig("./local.env", &cfg)
	if err != nil {
		panic(err)
	}
	return cfg
}

func (cfg *Config) verifyDir(system string) { // verify if dir variables have suffix

	if system == "mpd" {
		cfg.PlaylistDir = fixDir(cfg.PlaylistDir)
	}
	
	cfg.Youtube.DownloadDir = fixDir(cfg.Youtube.DownloadDir)
}

func fixDir(dir string) string {
	if !strings.HasSuffix(dir, "/") {
		return dir + "/"
	}
	return dir
}

func cleanUp(cfg Config, songs []string) { // Remove downloaded webms

	for _, song := range songs {
		path := fmt.Sprintf("%s%s.webm", cfg.Youtube.DownloadDir,song)
		
		err := os.Remove(path)
		if err != nil {
			log.Printf("failed to remove file: %v", err)
		}
	}

}

func deleteSongs(cfg Youtube) { // Deletes all files if persist equals false
	entries, err := os.ReadDir(cfg.DownloadDir)
	if err != nil {
		log.Printf("failed to read directory: %v", err)
	}
	for _, entry := range entries {
		if !(entry.IsDir()) {
			err = os.Remove(path.Join(cfg.DownloadDir, entry.Name()))
			if err != nil {
				log.Printf("failed to remove file: %v", err)
			}
		}
	}
}


func (cfg *Config) detectSystem() {
	if cfg.System == "" {
		log.Printf("Warning: no SYSTEM variable set, trying to detect automatically..")
		if cfg.Subsonic.User != "" && cfg.Subsonic.Password != "" {
			log.Println("using Subsonic")
			cfg.System = "subsonic"
			return

		} else if cfg.Creds.APIKey != "" {
			log.Println("using Jellyfin")
			cfg.System = "jellyfin"
			return

		} else if cfg.PlaylistDir != "" {
			log.Println("using Music Player Daemon")
			cfg.System = "mpd"
			return

		}
		log.Fatal("unable to detect system, check if SUBSONIC_USER, JELLYFIN_API or PLAYLIST_DIR fields exist")
}
log.Printf("using %s", cfg.System)
}

func verifyVars(cfg Config) {

	switch cfg.System {

	case "subsonic":
		if (cfg.Creds.User == "" && cfg.Creds.Password == "") {
			log.Fatal("USER and/or PASSWORD variable not set, exiting")
		}
	case "jellyfin":

		if cfg.Creds.APIKey == "" {
			log.Fatal("API_KEY variable not set")
		}
	case "mpd":

		if cfg.PlaylistDir == "" {
			log.Fatal("PLAYLIST_DIR variable not set")
		}
	default:
		log.Fatalf("system: %s not known, please use a supported system", cfg.System)
}
}

func makeRequest(method, url string, payload io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize request: %v", err)
	}

	for key, value := range headers {
		req.Header.Add(key,value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return body, nil
}


func main() {
	cfg := readEnv()
	cfg.detectSystem()
	cfg.verifyDir(cfg.System)
	verifyVars(cfg)
	cfg.handleDeprecation()
	cfg.getPlaylistName(cfg.Persist)

	var tracks Track

	if cfg.Listenbrainz.Discovery == "playlist" {
		id, err := getWeeklyExploration(cfg.Listenbrainz)
		if err != nil {
			log.Fatal(err.Error())
		}
		tracks = parseWeeklyExploration(id)
	} else {
		mbids := getReccs(cfg.Listenbrainz)
		tracks = getTracks(mbids)
	}

	if !cfg.Persist { // delete songs and playlist before downloading new ones
		if err := handlePlaylistDeletion(cfg); err != nil {
			log.Printf("failed to delete playlist: %s", err.Error())
		}
	}

	var files []string
	var songs []string
	var m3usongs []string
	
	for _, track := range tracks {
		song, file := gatherVideo(cfg.Youtube, track.Title, track.Artist, track.Album)
		files = append(files, file) // used for deleting .webms
		if song != "" { // used for creating playlists
			m3usongs = append(m3usongs, file)
			songs = append(songs, song)
		}
	}

	cleanUp(cfg, files)
	
	err := createPlaylist(cfg, songs, m3usongs)
	if err != nil {
		log.Fatal(err.Error())
	}
}