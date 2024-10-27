package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"explo/debug"

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




func queryYT(cfg Youtube, track Track) Videos { // Queries youtube for the song
	
	escQuery := url.PathEscape(fmt.Sprintf("%s - %s", track.Title, track.SearchArtist))
	queryURL := fmt.Sprintf("https://youtube.googleapis.com/youtube/v3/search?part=snippet&q=%s&type=video&videoCategoryId=10&key=%s", escQuery, cfg.APIKey)

	body, err := makeRequest("GET", queryURL, nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	var videos Videos
	err = json.Unmarshal(body, &videos)
	if err != nil {
		debug.Debug(fmt.Sprintf("response: %s", body))
		log.Fatalf("Failed to unmarshal queryYT body: %s", err.Error())
	}

	return videos

}

func getTopic(videos Videos, track Track) string { // gets song under artist topic or personal channel
	
	for _, v := range videos.Items {
		if (strings.Contains(v.Snippet.ChannelTitle, "- Topic") || v.Snippet.ChannelTitle == track.MetadataArtist) && filter(track, v.Snippet.Title) {
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

func saveVideo(cfg Youtube, track Track, stream io.ReadCloser) string {

	defer stream.Close()
	// Remove illegal characters for file naming
	re := regexp.MustCompile("[^a-zA-Z0-9._]+")
	s := re.ReplaceAllString(track.Title, cfg.Separator)
	a := re.ReplaceAllString(track.SearchArtist, cfg.Separator)

	input := fmt.Sprintf("%s%s-%s.webm", cfg.DownloadDir,s, a)
	file, err := os.Create(input)
	if err != nil {
		log.Fatalf("Failed to create song file: %s", err.Error())
	}
	defer file.Close()

	_, err = io.Copy(file, stream)
	if err != nil {
		log.Printf("Failed to copy stream to file: %s", err.Error())
		return fmt.Sprintf("%s-%s", s, a) // If the download fails (downloads a few bytes) then it will get triggered here: "tls: bad record MAC"
	}

	cmd := ffmpeg.Input(input).Output(fmt.Sprintf("%s%s-%s.mp3", cfg.DownloadDir,s, a), ffmpeg.KwArgs{
			"map": "0:a",
			"metadata": []string{"artist="+track.MetadataArtist,"title="+track.Title,"album="+track.Album},
			"loglevel": "error",
		}).OverWriteOutput().ErrorToStdOut()

	if cfg.FfmpegPath != "" {
		cmd.SetFfmpegPath(cfg.FfmpegPath)
	}

	err = cmd.Run()
	if err != nil {
		log.Printf("Failed to convert audio: %s", err.Error())
		return fmt.Sprintf("%s-%s", s, a)
	}
	return fmt.Sprintf("%s-%s", s, a)
	
}

func gatherVideo(cfg Youtube, track Track) string {

	videos := queryYT(cfg, track)
	id := getTopic(videos, track)

	if id != "" {
		stream, err := getVideo(id)
		if stream != nil && err == nil {
			file := saveVideo(cfg, track, stream)
			return file
		} else {
			log.Printf("failed getting stream: %s", err.Error())
		}
	}
	// if getting song from official channel fails, try getting from first available channel
	for _, video := range videos.Items {
		if filter(track, video.Snippet.Title) {
		stream, err := getVideo(video.ID.VideoID)
		if stream != nil && err == nil {
			file := saveVideo(cfg, track, stream)
			return file
		} else {
			log.Printf("failed getting stream: %s", err.Error())
			continue
		}
	}
}
	return ""
}

func filter(track Track, videoTitle string) bool { // ignore artist lives or song remixes

	if (!contains(track.Title,"live") && !contains(track.MetadataArtist,"live") && contains(videoTitle, "live")) {
		return false
	}

	if (!contains(track.Title,"remix") && !contains(track.MetadataArtist,"remix") && contains(videoTitle, "remix")) {
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