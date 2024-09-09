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
	Items   []Item `json:"items"`
}

type ID struct {
	VideoID string `json:"videoId"`
}

type Snippet struct {
	Title                string     `json:"title"`
	ChannelTitle         string     `json:"channelTitle"`
}

type Item struct {
	ID      ID      `json:"id"`
	Snippet Snippet `json:"snippet"`
}




func queryYT(cfg Youtube, song, artist string) Videos { // Queries youtube for the song

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

func getTopic(videos Videos, song, artist string) string { // gets song under artist topic or personal channel
	
	for _, v := range videos.Items {
		if (strings.Contains(v.Snippet.ChannelTitle, "- Topic") || v.Snippet.ChannelTitle == artist) && filter(song, artist, v.Snippet.Title) {
			return v.ID.VideoID
		} else {
			continue
		}
	}
	return ""
}

func getVideo(videoID string) (io.ReadCloser, error) { // gets video stream using kddai's youtube package

	yt_client := youtube.Client{}
	var format youtube.Format
	
	video, err := yt_client.GetVideo(videoID)
	if err != nil {
		log.Println("could not get video, trying next one")
	}
	formats := video.Formats.WithAudioChannels() // Get video with audio

	switch len(formats) {
	case 0:
		log.Println("format list is empty, getting next video...")
		return nil, err
	case 1, 2:
		format = formats[0]
	default: // if video has audio only format use that (to save temp space)
		format = formats[2]
	}

	stream, _, err := yt_client.GetStream(video, &format)

	if err != nil {
		log.Printf("Failed to get video stream: %s", err.Error())
		return nil, err
	}
	return stream, nil
			
}

func saveVideo(cfg Youtube, song, artist, album string, stream io.ReadCloser) (string, string) {

	defer stream.Close()
	// Remove illegal characters for file naming
	re := regexp.MustCompile("[^a-zA-Z0-9._]+")
	s := re.ReplaceAllString(song, cfg.Separator)
	a := re.ReplaceAllString(artist, cfg.Separator)

	input := fmt.Sprintf("%s%s-%s.webm", cfg.DownloadDir,s, a)
	file, err := os.Create(input)
	if err != nil {
		log.Fatalf("Failed to create song file: %s", err.Error())
	}
	defer file.Close()

	_, err = io.Copy(file, stream)
	if err != nil {
		log.Printf("Failed to copy stream to file: %s", err.Error())
		return "", fmt.Sprintf("%s-%s", s, a) // If the download fails (downloads a few bytes) then it will get triggered here: "tls: bad record MAC"
	}

	cmd := ffmpeg.Input(input).Output(fmt.Sprintf("%s%s-%s.mp3", cfg.DownloadDir,s, a), ffmpeg.KwArgs{
			"map": "0:a",
			"metadata": []string{"artist="+artist,"title="+song,"album="+album},
			"loglevel": "error",
		}).OverWriteOutput().ErrorToStdOut()

	if cfg.FfmpegPath != "" {
		cmd.SetFfmpegPath(cfg.FfmpegPath)
	}

	err = cmd.Run()
	if err != nil {
		log.Printf("Failed to convert audio: %s", err.Error())
		return "", fmt.Sprintf("%s-%s", s, a)
	}
		
	return fmt.Sprintf("%s %s %s", song, artist, album), fmt.Sprintf("%s-%s", s, a)
	
}

func gatherVideo(cfg Youtube, song, artist, album string) (string, string) {

	videos := queryYT(cfg, song, artist)
	id := getTopic(videos, song, artist)

	if id != "" {
		stream, err := getVideo(id)
		if stream != nil && err == nil {
			song, file := saveVideo(cfg, song, artist, album, stream)
			return song, file
		} else {
			log.Printf("failed getting stream: %s", err.Error())
		}
	}
	// if getting song from official channel fails, try getting from first available channel
	for _, video := range videos.Items {
		if filter(song, artist, video.Snippet.Title) {
		stream, err := getVideo(video.ID.VideoID)
		if stream != nil && err == nil {
			song, file := saveVideo(cfg, song, artist, album, stream)
			return song, file
		} else {
			log.Printf("failed getting stream: %s", err.Error())
			continue
		}
	}
}
	return "", ""

}

func filter(song, artist, videoTitle string) bool { // ignore artist lives or song remixes

	if (!contains(song,"live") && !contains(artist,"live") && contains(videoTitle, "live")) {
		return false
	}

	if (!contains(song,"remix") && !contains(artist,"remix") && contains(videoTitle, "remix")) {
			return false
	}

	return true
}

func contains(str string, substr string) bool {

	return strings.Contains(
        strings.ToLower(str),
        substr,
    )
}