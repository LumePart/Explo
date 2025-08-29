package downloader

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"

	cfg "explo/src/config"
	"explo/src/debug"
	"explo/src/models"
	"explo/src/util"

	ffmpeg "github.com/u2takey/ffmpeg-go"
	"github.com/wader/goutubedl"
)

type Videos struct {
	Items []Item `json:"items"`
}

type ID struct {
	VideoID string `json:"videoId"`
}

type Snippet struct {
	Title        string `json:"title"`
	ChannelTitle string `json:"channelTitle"`
}

type Item struct {
	ID      ID      `json:"id"`
	Snippet Snippet `json:"snippet"`
}

type Youtube struct {
	DownloadDir string
	HttpClient  *util.HttpClient
	Cfg         cfg.Youtube
}

func NewYoutube(cfg cfg.Youtube, discovery, downloadDir string, httpClient *util.HttpClient) *Youtube { // init downloader cfg for youtube
	return &Youtube{
		DownloadDir: downloadDir,
		Cfg:         cfg,
		HttpClient:  httpClient}
}

func (c *Youtube) GetConf() (MonitorConfig, error) {
	return MonitorConfig{}, fmt.Errorf("[youtube] no monitoring required")
}

func (c *Youtube) QueryTrack(track *models.Track) error { // Queries youtube for the song

	escQuery := url.PathEscape(fmt.Sprintf("%s - %s", track.Title, track.Artist))
	queryURL := fmt.Sprintf("https://youtube.googleapis.com/youtube/v3/search?part=snippet&q=%s&type=video&videoCategoryId=10&key=%s", escQuery, c.Cfg.APIKey)

	body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, nil)
	if err != nil {
		return err
	}
	var videos Videos
	if err = util.ParseResp(body, &videos); err != nil {
		return fmt.Errorf("failed to unmarshal queryYT body: %s", err.Error())
	}

	id := gatherVideo(c.Cfg, videos, *track)
	if id == "" {
		return fmt.Errorf("no YouTube video found for track: %s - %s", track.Title, track.Artist)
	}
	track.ID = id

	return nil
}

func (c *Youtube) GetTrack(track *models.Track) error {
	ctx := context.Background() // ctx for yt-dlp
	
	track.File = getFilename(track.Title, track.Artist)+".opus"
	track.Present = fetchAndSaveVideo(ctx, *c, *track)

	if track.Present {
		log.Printf("[youtube] Download finished: %s - %s", track.Artist, track.Title)
		return nil
	}
	return fmt.Errorf("failed to download track: %s - %s", track.Title, track.Artist)
}

func (c *Youtube) MonitorDownloads(track []*models.Track) error { // No need to monitor yt-dlp downloads, there is no queue for them
	log.Println("[youtube] No further monitoring required")
	return nil
 }

func getTopic(cfg cfg.Youtube, videos Videos, track models.Track) string { // gets song under artist topic or personal channel

	for _, v := range videos.Items {
		if (strings.Contains(v.Snippet.ChannelTitle, "- Topic") || v.Snippet.ChannelTitle == track.MainArtist) && filter(track, v.Snippet.Title, cfg.Filters.FilterList) {
			return v.ID.VideoID
		}
	}
	return ""
}

func getVideo(ctx context.Context, c Youtube, videoID string) (*goutubedl.DownloadResult, error) { // gets video stream using yt-dlp

	if c.Cfg.YtdlpPath != "" {
		goutubedl.Path = c.Cfg.YtdlpPath
	}

	result, err := goutubedl.New(ctx, videoID, goutubedl.Options{})
	if err != nil {
		return nil, fmt.Errorf("could not create URL for video download (ID: %s): %s", videoID, err.Error())
	}

	downloadResult, err := result.Download(ctx, "bestaudio")
	if err != nil {
		return nil, fmt.Errorf("could not download video: %s", err.Error())
	}

	return downloadResult, nil

}

func saveVideo(c Youtube, track models.Track, stream *goutubedl.DownloadResult) bool {

	defer func() {
		if err := stream.Close(); err != nil {
			log.Printf("warning: stream close failed: %v", err)
		}
	}()

	input := fmt.Sprintf("%s%s.tmp", c.DownloadDir, track.File)
	file, err := os.Create(input)
	if err != nil {
		log.Fatalf("failed to create song file: %s", err.Error())
	}

	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("warning: file close failed: %v", err)
		}
	}()

	if _, err = io.Copy(file, stream); err != nil {
		log.Printf("failed to copy stream to file: %s", err.Error())
		if err = os.Remove(input); err != nil {
			debug.Debug(fmt.Sprintf("failed to remove %s: %s", input, err.Error()))
		}
		return false
	}

	cmd := ffmpeg.Input(input).Output(fmt.Sprintf("%s%s", c.DownloadDir, track.File), ffmpeg.KwArgs{
		"map":      "0:a",
		"metadata": []string{"artist=" + track.Artist, "title=" + track.Title, "album=" + track.Album},
		"loglevel": "error",
	}).OverWriteOutput().ErrorToStdOut()

	if c.Cfg.FfmpegPath != "" {
		cmd.SetFfmpegPath(c.Cfg.FfmpegPath)
	}

	if err = cmd.Run(); err != nil {
		log.Printf("failed to convert audio: %s", err.Error())
		if err = os.Remove(input); err != nil {
			debug.Debug(fmt.Sprintf("failed to remove %s: %s", input, err.Error()))
		}
		return false
	}
	if err = os.Remove(input); err != nil {
		debug.Debug(fmt.Sprintf("failed to remove %s: %s", input, err.Error()))
	}
	return true
}

func gatherVideo(cfg cfg.Youtube, videos Videos, track models.Track) string { // filter out video ID

	// Try to get the video from the official or topic channel
	if id := getTopic(cfg, videos, track); id != "" {
		return id

	}
	// If official video isn't found, try the first suitable channel
	for _, video := range videos.Items {
		if filter(track, video.Snippet.Title, cfg.Filters.FilterList) {
			return video.ID.VideoID
		}
	}

	return ""
}

func fetchAndSaveVideo(ctx context.Context, cfg Youtube, track models.Track) bool {
	stream, err := getVideo(ctx, cfg, track.ID)
	if err != nil {
		log.Printf("failed getting stream for video ID %s: %s", track.ID, err.Error())
		return false
	}

	if stream != nil {
		return saveVideo(cfg, track, stream)
	}

	log.Printf("stream was nil for video ID %s", track.ID)
	return false
}

func filter(track models.Track, videoTitle string, filterList []string) bool { // ignore titles that have a specific keyword (defined in .env)

	for _, keyword := range filterList {
		if !containsLower(track.Title, keyword) && !containsLower(track.Artist, keyword) && containsLower(videoTitle, keyword) {
			return false
		}
	}
	return true
}

func (c *Youtube) GetDownloadStatus(tracks []*models.Track) (map[string]FileStatus, error) {
	return nil, fmt.Errorf("no monitoring required")
}

func (c *Youtube) Cleanup(track models.Track, ID string) error {
	return nil
}