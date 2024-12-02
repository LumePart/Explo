package main

import (
	"context"
	"encoding/json"
	"explo/debug"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"

	ffmpeg "github.com/u2takey/ffmpeg-go"
	"github.com/wader/goutubedl"
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

func getVideo(ctx context.Context, cfg Youtube, videoID string) (*goutubedl.DownloadResult, error) { // gets video stream using kddai's youtube package

	if cfg.YtdlpPath != "" {
		goutubedl.Path = cfg.YtdlpPath
	}

	result, err := goutubedl.New(ctx, videoID, goutubedl.Options{})
	if err != nil {
		return nil, fmt.Errorf("could not create URL for video download: %s", err.Error())
	}

	downloadResult, err := result.Download(ctx, "bestaudio")
	if err != nil {
		return nil, fmt.Errorf("could not download video: %s", err.Error())
	}

	return downloadResult, nil
			
}

func saveVideo(cfg Youtube, track Track, stream *goutubedl.DownloadResult) bool {

	defer stream.Close()

	input := fmt.Sprintf("%s%s_TEMP.mp3", cfg.DownloadDir, track.File)
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

	cmd := ffmpeg.Input(input, ffmpeg.KwArgs{
			"c": "copy",
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
	ctx := context.Background()

	for i := range tracks {
		if !tracks[i].Present {
			downloaded := gatherVideo(ctx, cfg.Youtube, tracks[i])

				// If "test" discovery mode is enabled, download just one song and break
				if cfg.Listenbrainz.Discovery == "test" && downloaded {
					log.Println("Using 'test' discovery method. Downloaded 1 song.")
					break
			}
		}
	}
}

func gatherVideo(ctx context.Context, cfg Youtube, track Track) bool {
	// Query YouTube for videos matching the track
	videos := queryYT(cfg, track)
	
	// Try to get the video from the official or topic channel
	if id := getTopic(videos, track); id != "" {
		return fetchAndSaveVideo(ctx, cfg, track, id)
			
	}

	// If official video isn't found, try the first suitable channel
	for _, video := range videos.Items {
		if filter(track, video.Snippet.Title) {
			return fetchAndSaveVideo(ctx, cfg, track, video.ID.VideoID)
		}
	}

	return false
}

func fetchAndSaveVideo(ctx context.Context, cfg Youtube, track Track, videoID string) bool {
	stream, err := getVideo(ctx, cfg, videoID)
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