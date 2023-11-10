package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"log"

	"github.com/kkdai/youtube/v2"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

type Videos struct {
	Items         []Items  `json:"items"`
}

type ID struct {
	VideoID string `json:"videoId"`
}

type Snippet struct {
	Title string `json:"title"`
}

type Items struct {
	ID      ID      `json:"id"`
	Snippet Snippet `json:"snippet"`
}

func queryYT(song string, artist string, cfg Youtube) Videos { // Queries youtube for the song

	client := http.Client{}
	
	escQuery := url.PathEscape(fmt.Sprintf("%s - %s", song, artist))
	query := fmt.Sprintf("https://youtube.googleapis.com/youtube/v3/search?part=snippet&q=%s&type=video&videoCategoryId=10&key=%s", escQuery, cfg.APIKey)
	req, _ := http.NewRequest(http.MethodGet, query, nil)

	var videos Videos
	resp, _ := client.Do(req)
	body, _ := io.ReadAll(resp.Body)
	err := json.Unmarshal(body, &videos)
	if err != nil {
		log.Fatalf("Error unmarshaling response: %s", err)
	}

	return videos

}

func downloadAndFormat(song string, artist string, name string, cfg Youtube) (string, string) {

	videos := queryYT(song, artist, cfg)

	
	yt_client := youtube.Client{}

	for _, v := range videos.Items {
		if (!filter(song,"live") && !filter(artist,"live") && filter(v.Snippet.Title, "live")) || // ignore artist lives or song remixes
		(!filter(song,"remix") && !filter(artist,"remix") && filter(v.Snippet.Title, "remix")) {
			continue
		} else {

			// Remove illegal characters for file naming
			re := regexp.MustCompile("[^a-zA-Z0-9._]+")
			s := re.ReplaceAllString(song, " ")
			a := re.ReplaceAllString(artist, " ")

			video, _ := yt_client.GetVideo(v.ID.VideoID)
			formats := video.Formats.WithAudioChannels() // Get video with audio

			stream, _, err := yt_client.GetStream(video, &formats[0])
			if err != nil {
				log.Fatalf("Error getting video stream: %s", err)
			}
			defer stream.Close()

			input := fmt.Sprintf("%s%s - %s.webm", cfg.DownloadDir,s, a)
			file, err := os.Create(input)
			if err != nil {
				log.Fatalf("Error creating file: %s", err)
			}
			defer file.Close()

			_, err = io.Copy(file, stream)
			if err != nil {
				log.Fatalf("Error copying the stream to file")
				break // If the download fails (downloads a few bytes) then it will get triggered here: "tls: bad record MAC"
			}

			err = ffmpeg.Input(input).Output(fmt.Sprintf("%s%s - %s.mp3", cfg.DownloadDir,s, a), ffmpeg.KwArgs{
				"q:a": 0,
				"map": "a",
				"metadata": []string{"artist="+artist,"title="+song,"album="+name},
				"loglevel": "error",
			}).OverWriteOutput().ErrorToStdOut().Run()

			if err != nil {
				log.Fatalf("Error while running ffmpeg: %s", err)
			}

			return fmt.Sprintf("%s %s %s", song, artist, name), fmt.Sprintf("%s - %s", s, a)
		}
	}

	return "", ""
}

func filter(str string, substr string) bool {

	return strings.Contains(
        strings.ToLower(str),
        substr,
    )
}