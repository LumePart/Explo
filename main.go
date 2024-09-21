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
	Sleep int `env:"SLEEP" env-default:"1"`
	PlaylistDir string `env:"PLAYLIST_DIR"`
	Persist bool `env:"PERSIST" env-default:"true"`
	PlaylistName string
}

type Jellyfin struct {
	Source string `env:"JELLYFIN_SOURCE"`
	URL string `env:"JELLYFIN_URL" env-default:"http://127.0.0.1:8096"`
	APIKey string `env:"JELLYFIN_API_KEY"`
	Client string `env:"CLIENT" env-default:"explo"`
}

type Subsonic struct {
	Version	string `env:"SUBSONIC_VERSION" env-default:"1.16.1"`
	ID string `env:"CLIENT" env-default:"explo"`
	URL	string `env:"SUBSONIC_URL" env-default:"http://127.0.0.1:4533"`
	User string `env:"SUBSONIC_USER"`
	Password string `env:"SUBSONIC_PASSWORD"`
	Token string
	Salt string
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

func readEnv() Config {
	var cfg Config

	err := cleanenv.ReadConfig("./local.env",&cfg)
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

func detectSystem(cfg Config) string {
	if cfg.Subsonic.User != "" && cfg.Subsonic.Password != "" {
		log.Println("using Subsonic")
		return "subsonic"

	} else if cfg.Jellyfin.APIKey != "" {
		log.Println("using Jellyfin")
		return "jellyfin"

	} else if cfg.PlaylistDir != "" {
		log.Println("using Music Player Daemon")
		return "mpd"

	}
	log.Fatal("unable to detect system, check if SUBSONIC_USER, JELLYFIN_API or PLAYLIST_DIR fields exist")
	return ""
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
	system := detectSystem(cfg)
	cfg.verifyDir(system)
	cfg.Subsonic.genToken()
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
		if err := handlePlaylistDeletion(cfg, system); err != nil {
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
	
	err := createPlaylist(cfg, songs, m3usongs, system)
	if err != nil {
		log.Fatal(err.Error())
	}
}