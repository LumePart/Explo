package downloader

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"regexp"
	"strings"

	cfg "explo/src/config"
	"explo/src/debug"
	"explo/src/models"
	"explo/src/util"
)

var opus_re *regexp.Regexp = regexp.MustCompile(`"(.*\.opus)"`)

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
	track.Present = fetchAndSaveAudioTrack(*c, *track)

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

func downloadAudioTrack(videoID string) (string, error) { // gets video stream using yt-dlp

	args := []string{
		"-x", // only audio
		"--no-warnings",
		"--embed-metadata",
		fmt.Sprintf("https://music.youtube.com/watch?v=%s", videoID),
	}

	out, err := util.ExecUtility("yt-dlp", args...)

	if err != nil {
		return "", err
	}

	m := opus_re.FindSubmatch(out)
	if len(m) != 2 {
		return "", errors.New("could not match yt-dlp result for filename")
	}

	return string(m[1]), nil

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

func fetchAndSaveAudioTrack(cfg Youtube, track models.Track) bool {
	var err error

	track.File, err = downloadAudioTrack(track.ID)
	if err != nil {
		log.Printf("failed downloading track for ID %s: %s", track.ID, err.Error())
		return false
	}

	args := []string{
		"-f", // force cp
		track.File,
		fmt.Sprintf("%s/%s", cfg.DownloadDir, track.File),
	}
	_, err = util.ExecUtility("mv", args...)

	if err != nil {
		log.Printf("failed copying %s to download dir", track.File)
		return false
	}

	return true
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
