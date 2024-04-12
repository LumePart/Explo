package main

import (
	"fmt"
	"log"
	"os"
	"time"
	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Subsonic Subsonic
	Youtube Youtube
	Listenbrainz Listenbrainz
	Sleep int `env:"SLEEP" env-default:"1"`
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

func cleanUp(cfg Youtube, songs []string) { // Remove downloaded webms

	for _, song := range songs {
		path := fmt.Sprintf("%s%s.webm", cfg.DownloadDir,song)
		
		err := os.Remove(path)
		if err != nil {
			log.Printf("Failed to remove file: %v", err.Error())
		}
	}

}


func main() {
	cfg := readEnv()
	cfg.Subsonic = genToken(cfg.Subsonic)

	var tracks Track

	if cfg.Listenbrainz.Discovery == "playlist" {
		id, err := getWeeklyExploration(cfg.Listenbrainz)
		if err != nil {
			log.Fatal(err.Error())
		}
		tracks = parseWeeklyExploration(id)
	} else { // use reccommendations from API
		mbids := getReccs(cfg.Listenbrainz)
		tracks = getTracks(mbids)
	}
	var files []string
	var songs []string
	
	for _, track := range tracks {
		song, file := downloadAndFormat(track.Title, track.Artist, track.Album, cfg.Youtube)
		files = append(files, file)
		songs = append(songs, song)
	}

	cleanUp(cfg.Youtube, files)
	scan(cfg.Subsonic)
	log.Printf("Sleeping for %v minutes, to allow scan to complete..", cfg.Sleep)
	time.Sleep(time.Duration(cfg.Sleep) * time.Minute)
	err := createPlaylist(cfg.Subsonic, songs)

	if err != nil {
		log.Fatalf("failed to create playlist: %s", err.Error())
	}
}