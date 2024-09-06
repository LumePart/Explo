package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Subsonic Subsonic
	Youtube Youtube
	Listenbrainz Listenbrainz
	Sleep int `env:"SLEEP" env-default:"1"`
	PlaylistDir string `env:"PLAYLIST_DIR"`
	Persist bool `env:"PERSIST" env-default:"true"`
}

type Subsonic struct {
	Version	string `env:"SUBSONIC_VERSION" env-default:"1.16.1"`
	ID string `env:"SUBSONIC_ID" env-default:"explo"`
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
	
	cfg.Youtube.DownloadDir = fixDir(cfg.PlaylistDir)
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
			os.Remove(path.Join(cfg.DownloadDir, entry.Name()))
		}
	}
}

func detectSystem(cfg Config) string { // if more systems are added, then API detection would be good
	fmt.Println(cfg.Subsonic.User)
	if cfg.Subsonic.User != "" && cfg.Subsonic.Password != "" {
		log.Println("using Subsonic")
		return "subsonic"

	} else if cfg.PlaylistDir != "" {
		log.Println("using Music Player Daemon")
		return "mpd"

	}
	log.Fatal("unable to detect system, check if PLAYLIST_DIR or SUBSONIC_USER fields exist")
	return ""
}


func main() {
	cfg := readEnv()
	system := detectSystem(cfg)
	cfg.verifyDir(system)
	cfg.Subsonic.genToken()

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
	var files []string
	var songs []string
	
	for _, track := range tracks {
		song, file := gatherVideo(cfg.Youtube, track.Title, track.Artist, track.Album)
		files = append(files, file)
		if song != "" {
			songs = append(songs, song)
		}
	}

	cleanUp(cfg, files)
	
	err := createPlaylist(cfg, songs, files, system)
	if err != nil {
		log.Fatal(err.Error())
	}
}