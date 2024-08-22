package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Subsonic Subsonic
	Youtube Youtube
	Listenbrainz Listenbrainz
	Sleep int `env:"SLEEP" env-default:"1"`
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

func cleanUp(cfg Config, songs []string) { // Remove downloaded webms

	for _, song := range songs {
		path := fmt.Sprintf("%s%s.webm", cfg.Youtube.DownloadDir,song)
		
		err := os.Remove(path)
		if err != nil {
			log.Printf("failed to remove file: %v", err)
		}
	}

}

func deleteSongs(cfg Youtube) { // Deletes all songs if persist equals false
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


func main() {
	cfg := readEnv()
	cfg.Subsonic = genToken(cfg.Subsonic)

	if !(cfg.Persist) {

		deleteSongs(cfg.Youtube)
		playlists, err := getDiscoveryPlaylist(cfg.Subsonic)
		if err != nil {
			log.Fatal(err.Error())
		}
		err = delPlaylists(playlists, cfg.Subsonic)
		if err != nil {
			log.Fatal(err.Error())
		}
	}

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
	err := scan(cfg.Subsonic)
	if err != nil {
		log.Fatal(err.Error())
	}
	
	log.Printf("sleeping for %v minutes, to allow scan to complete..", cfg.Sleep)
	time.Sleep(time.Duration(cfg.Sleep) * time.Minute)

	err = createPlaylist(cfg.Subsonic, songs, cfg.Persist)
	if err != nil {
		log.Fatal(err.Error())
	}
}