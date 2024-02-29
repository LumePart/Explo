package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/kkdai/youtube/v2"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

type Videos struct {
	Items      	[]struct {
		ID      struct {
			VideoID string `json:"videoId"`
		}      `json:"id"`
		Snippet struct {
			Title string `json:"title"`
		} `json:"snippet"`
	}  `json:"items"`
}




func queryYT(song string, artist string, cfg Youtube) Videos { // Queries youtube for the song

	client := http.Client{}
	
	escQuery := url.PathEscape(fmt.Sprintf("%s - %s", song, artist))
	query := fmt.Sprintf("https://youtube.googleapis.com/youtube/v3/search?part=snippet&q=%s&type=video&videoCategoryId=10&key=%s", escQuery, cfg.APIKey)
	req, err := http.NewRequest(http.MethodGet, query, nil)
	if err != nil {
		log.Fatalf("Failed to initialize request: %v", err)
	}

	var videos Videos
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to make request: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}
	err = json.Unmarshal(body, &videos)
	if err != nil {
		log.Fatalf("Failed to unmarshal body: %v", err)
	}

	return videos

}

func downloadAndFormat(song string, artist string, name string, cfg Youtube) (string, string) {

	var format youtube.Format

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
			if formats == nil {
				log.Println("video format is empty, getting next one...")
				continue
			}

			switch len(formats) {
			case 0:
				log.Println("format list is empty, getting next video...")
				continue
			case 1, 2:
				format = formats[0]
			default: // if video has audio only format use that (to save space)
				format = formats[2]
			}

			stream, _, err := yt_client.GetStream(video, &format)
			if err != nil {
				log.Printf("Failed to get video stream: %v", err)
				break
			}
			defer stream.Close()

			input := fmt.Sprintf("%s%s - %s.webm", cfg.DownloadDir,s, a)
			file, err := os.Create(input)
			if err != nil {
				log.Fatalf("Failed to create song file: %v", err)
			}
			defer file.Close()

			_, err = io.Copy(file, stream)
			if err != nil {
				log.Printf("Failed to copy stream to file: %v", err)
				break // If the download fails (downloads a few bytes) then it will get triggered here: "tls: bad record MAC"
			}

			err = ffmpeg.Input(input).Output(fmt.Sprintf("%s%s - %s.mp3", cfg.DownloadDir,s, a), ffmpeg.KwArgs{
				"q:a": 0,
				"map": "a",
				"metadata": []string{"artist="+artist,"title="+song,"album="+name},
				"loglevel": "error",
			}).OverWriteOutput().ErrorToStdOut().Run()

			if err != nil {
				log.Fatalf("Failed to convert file via ffmpeg: %v", err)
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