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
	User string `env:"LISTENBRAINZ_USER"`
}

func readEnv() Config {
	var cfg Config



	err := cleanenv.ReadConfig("./local.env", &cfg)
	if err != nil {
		log.Fatalf("Failed to read env file: %s", err)
	}
	return cfg
}

func cleanUp(cfg Youtube, songs []string) {

	for _, song := range songs {
		path := fmt.Sprintf("%s%s.webm", cfg.DownloadDir,song)
		
		err := os.Remove(path)
		if err != nil {
			log.Fatalf("Failed to remove %s, got %s", song, err)
		}
	}

}


func main() {
	cfg := readEnv()
	cfg.Subsonic = genToken(cfg.Subsonic)

	mbids := getReccs(cfg.Listenbrainz)
	tracks := getTracks(mbids)
	var files []string
	var songs []string
	
	for _, v := range tracks {
		song, file := downloadAndFormat(v.Recording.Name, v.Release.AlbumArtistName, v.Release.Name, cfg.Youtube)
		files = append(files, file)
		songs = append(songs, song)
	}

	cleanUp(cfg.Youtube, files)
	scan(cfg.Subsonic)
	log.Printf("\nSleeping for %v minutes, to allow scan to complete..\n", cfg.Sleep)
	time.Sleep(time.Duration(cfg.Sleep) * time.Minute)
	createPlaylist(cfg.Subsonic, songs)

}