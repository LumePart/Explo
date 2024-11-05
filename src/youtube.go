package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
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
	
	escQuery := url.PathEscape(fmt.Sprintf("%s - %s", track.Title, track.Artist))
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
		if (strings.Contains(v.Snippet.ChannelTitle, "- Topic") || v.Snippet.ChannelTitle == track.Artist) && filter(track, v.Snippet.Title) {
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

func saveVideo(cfg Youtube, track Track, stream io.ReadCloser) bool {

	defer stream.Close()

	input := fmt.Sprintf("%s%s.webm", cfg.DownloadDir, track.File)
	file, err := os.Create(input)
	if err != nil {
		log.Fatalf("Failed to create song file: %s", err.Error())
	}
	defer file.Close()

	_, err = io.Copy(file, stream)
	if err != nil {
		log.Printf("Failed to copy stream to file: %s", err.Error())
		os.Remove(input)
		return false // If the download fails (downloads a few bytes) then it will get triggered here: "tls: bad record MAC"
	}

	cmd := ffmpeg.Input(input).Output(fmt.Sprintf("%s%s.mp3", cfg.DownloadDir, track.File), ffmpeg.KwArgs{
			"map": "0:a",
			"metadata": []string{"artist="+track.Artist,"title="+track.Title,"album="+track.Album},
			"loglevel": "error",
		}).OverWriteOutput().ErrorToStdOut()

	if cfg.FfmpegPath != "" {
		cmd.SetFfmpegPath(cfg.FfmpegPath)
	}

	err = cmd.Run()
	if err != nil {
		log.Printf("Failed to convert audio: %s", err.Error())
		os.Remove(input)
		return false
	}
	os.Remove(input)
	return true
}

func gatherVideos(cfg Config, tracks []Track) {
	for i := range tracks {
		if !tracks[i].Present {
			downloaded := gatherVideo(cfg.Youtube, tracks[i])

				// If "test" discovery mode is enabled, download just one song and break
				if cfg.Listenbrainz.Discovery == "test" && downloaded {
					log.Println("Using 'test' discovery method. Downloaded 1 song.")
					break
			}
		}
	}
}

func gatherVideo(cfg Youtube, track Track) bool {
	// Query YouTube for videos matching the track
	videos := queryYT(cfg, track)
	
	// Try to get the video from the official or topic channel
	if id := getTopic(videos, track); id != "" {
		return fetchAndSaveVideo(cfg, track, id)
			
	}

	// If official video isn't found, try the first suitable channel
	for _, video := range videos.Items {
		if filter(track, video.Snippet.Title) {
			return fetchAndSaveVideo(cfg, track, video.ID.VideoID)
		}
	}

	return false
}

func fetchAndSaveVideo(cfg Youtube, track Track, videoID string) bool {
	stream, err := getVideo(videoID)
	if err != nil {
		log.Printf("failed getting stream for video ID %s: %s", videoID, err.Error())
		return false
	}
	
	if stream != nil {
		return saveVideo(cfg, track, stream)
	}
	
	log.Printf("stream was nil for video ID %s", videoID)
	return false
}

func filter(track Track, videoTitle string) bool { // ignore artist lives or song remixes

	if (!contains(track.Title,"live") && !contains(track.Artist,"live") && contains(videoTitle, "live")) {
		return false
	}

	if (!contains(track.Title,"remix") && !contains(track.Artist,"remix") && contains(videoTitle, "remix")) {
			return false
	}

	if (!contains(track.Title,"instrumental") && !contains(track.Artist,"instrumental") && contains(videoTitle, "instrumental")) {
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