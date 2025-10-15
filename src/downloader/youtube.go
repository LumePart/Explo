package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
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

type YTMusicSearchResult struct {
	VideoID string `json:"videoId"`
	Title   string `json:"title"`
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

	query := fmt.Sprintf("%s - %s", track.Title, track.Artist)
	if c.Cfg.APIKey == "" { // if no API key set, use Python YT Music module
		err := queryYTMusic(track, query)
		return err
	}

	escQuery := url.PathEscape(query)
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

func queryYTMusic(track *models.Track, query string) error {

	debug.Debug(fmt.Sprintf("Querying YTMusic for track %s", query))

	cmd := exec.Command("python3", "search_ytmusic.py", query, "1")

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ytmusicapi subprocess failed: %w", err)
	}

	var results []YTMusicSearchResult
	if err := json.Unmarshal(out, &results); err != nil {
		return fmt.Errorf("failed to parse ytmusicapi JSON: %w", err)
	}

	if len(results) == 0 {
		return fmt.Errorf("no YouTube Music track found for: %s", query)
	}

	track.ID = results[0].VideoID
	//log.Printf("Matched track %s => videoId %s", query, track.ID) keeping this until I improve logging (good trace)

	return nil
}

func (c *Youtube) GetTrack(track *models.Track) error {
	ctx := context.Background() // ctx for yt-dlp

	track.File = getFilename(track.Title, track.Artist) + ".opus"
	track.Present = fetchAndSaveVideo(ctx, *c, *track)

	if track.Present {
		slog.Info("download finished", "service", "youtube", "track", track.File)
		return nil
	}
	return fmt.Errorf("failed to download track: %s - %s", track.Title, track.Artist)
}

func (c *Youtube) MonitorDownloads(track []*models.Track) error { // No need to monitor yt-dlp downloads, there is no queue for them
	slog.Info("no further monitoring required", "service", "youtube")
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
			slog.Warn("closing stream failed", "context", err.Error())
		}
	}()

	input := fmt.Sprintf("%s%s.tmp", c.DownloadDir, track.File)
	file, err := os.Create(input)
	if err != nil {
		slog.Error("failed to create song file", "context",err.Error())
		return false
	}

	defer func() {
		if err := file.Close(); err != nil {
			slog.Warn("file close failed", "context", err.Error())
		}
	}()

	if _, err = io.Copy(file, stream); err != nil {
		slog.Error("failed to copy stream to file", "context", err.Error())
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
		slog.Error("failed to convert audio", "context", err.Error())
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
		slog.Error("failed getting stream for video", "trackID",track.ID, "context", err.Error())
		return false
	}

	if stream != nil {
		return saveVideo(cfg, track, stream)
	}

	slog.Error("stream was empty for video", "trackID", track.ID)
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
