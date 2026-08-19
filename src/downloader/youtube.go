package downloader

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // registers the JPEG decoder used to size cover art
	_ "image/png"  // registers the PNG decoder used to size cover art
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cfg "explo/src/config"
	"explo/src/logging"
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
	gouTubeOpts goutubedl.Options
	Sleep 		int
}

func NewYoutube(cfg cfg.Youtube, discovery, downloadDir string, httpClient *util.HttpClient) *Youtube { // init downloader cfg for youtube
	// check for custom ytdlp options
	if cfg.YtdlpPath != "" {
		goutubedl.Path = cfg.YtdlpPath
	}

	var opts goutubedl.Options
	if _, err := os.Stat(cfg.CookiesPath); err == nil {
		opts.Cookies = cfg.CookiesPath
	}

	return &Youtube{
		DownloadDir: downloadDir,
		Cfg:         cfg,
		HttpClient:  httpClient,
		gouTubeOpts: opts}
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

	id := c.gatherVideo(c.Cfg, videos, *track)
	if id == "" {
		return fmt.Errorf("no YouTube video found for track: %s - %s", track.Title, track.Artist)
	}
	track.ID = id

	return nil
}

func queryYTMusic(track *models.Track, query string) error {

	slog.Debug(fmt.Sprintf("Querying YTMusic for track %s", query))

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

	track.File = fmt.Sprintf("%s.%s", getFilename(track.Title, track.Artist), c.Cfg.FileExtension)
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

// gets song under artist topic or personal channel
func getTopic(cfg cfg.Youtube, videos Videos, track models.Track) string {

	for _, v := range videos.Items {
		if (strings.Contains(v.Snippet.ChannelTitle, "- Topic") || v.Snippet.ChannelTitle == track.MainArtist) && !ContainsKeyword(track, v.Snippet.Title, cfg.Filters.FilterList) {
			return v.ID.VideoID
		}
	}
	return ""
}

// gets video stream using yt-dlp
func getVideo(ctx context.Context, c Youtube, videoID string) (*goutubedl.DownloadResult, error) {

	result, err := goutubedl.New(ctx, videoID, c.gouTubeOpts)
	if err != nil {
		return nil, fmt.Errorf("could not create URL for video download (ID: %s): %s", videoID, err.Error())
	}

	downloadResult, err := result.Download(ctx, "bestaudio")
	if err != nil {
		return nil, fmt.Errorf("could not download video: %s", err.Error())
	}

	return downloadResult, nil

}

// How a container can carry cover art, if at all.
type coverMethod int

const (
	coverNone          coverMethod = iota
	coverStream                    // an attached picture stream (ID3, MP4, FLAC)
	coverVorbisComment             // a base64 METADATA_BLOCK_PICTURE tag (Ogg family, WavPack)
)

// Verified per container by writing a file and reading the artwork back with a
// tag library, on ffmpeg 7.0.2, 8.1.2 and 9.0.1. Anything missing here gets no
// artwork on purpose: passing an image to muxers that cannot hold one (wav,
// aac, ac3) makes ffmpeg fail, and the track is lost with it. aiff, mka and
// webm are left out because the file ffmpeg produces is accepted but the
// artwork is not readable afterwards.
var coverSupport = map[string]coverMethod{
	".mp3":  coverStream,
	".flac": coverStream,
	".m4a":  coverStream,
	".m4b":  coverStream,
	".mp4":  coverStream,
	".ogg":  coverVorbisComment,
	".oga":  coverVorbisComment,
	".opus": coverVorbisComment,
	".spx":  coverVorbisComment,
	".wv":   coverVorbisComment,
}

// Linux caps a single argument at 128 KiB (MAX_ARG_STRLEN), and the base64 tag
// is passed on the command line. The default 250px cover art is far below this;
// a bigger one is skipped rather than risking the whole command.
const maxCoverTagLen = 100 * 1024

// coverFor returns the cover to embed and how the container wants it. The path
// is empty when there is nothing usable to embed.
func coverFor(track models.Track, coversDir, ext string) (string, coverMethod) {
	how := coverSupport[strings.ToLower(ext)]
	if how == coverNone {
		slog.Debug("container cannot carry cover art, skipping it",
			"extension", ext, "track", track.Title)
		return "", coverNone
	}

	// The cover may already be on disk: custom playlists cache it when the
	// playlist is created, so CoverPath is set before the download starts and
	// there is nothing to fetch.
	if track.CoverPath == "" {
		_, track.CoverPath = util.DownloadCover(track.CoverURL, coversDir)
	}

	// DownloadCover returns its destination path even when the fetch failed, so
	// check the file is really there and not empty.
	if st, err := os.Stat(track.CoverPath); err != nil || st.Size() == 0 {
		slog.Warn("cover not usable, writing track without artwork",
			"path", track.CoverPath, "track", track.Title)
		return "", coverNone
	}

	return track.CoverPath, how
}

// vorbisPictureTag encodes an image as a METADATA_BLOCK_PICTURE value, which is
// how the Ogg family and WavPack store cover art. ffmpeg cannot build this from
// an image input, but it does pass the tag through to the container.
func vorbisPictureTag(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("unsupported image: %w", err)
	}

	mime := "image/" + format
	put := binary.BigEndian.AppendUint32

	block := make([]byte, 0, len(data)+len(mime)+36)
	block = put(block, 3) // picture type: front cover
	block = put(block, uint32(len(mime)))
	block = append(block, mime...)
	block = put(block, 0) // no description
	block = put(block, uint32(cfg.Width))
	block = put(block, uint32(cfg.Height))
	block = put(block, 24) // colour depth in bits per pixel
	block = put(block, 0)  // colours used, 0 for non-indexed images
	block = put(block, uint32(len(data)))
	block = append(block, data...)

	tag := base64.StdEncoding.EncodeToString(block)
	if len(tag) > maxCoverTagLen {
		return "", fmt.Errorf("cover art too large to pass as a tag: %d bytes encoded", len(tag))
	}

	return tag, nil
}

// writeTrackFile muxes the downloaded audio into outputPath, embedding cover art
// when the container can hold it. Artwork never costs a track: if ffmpeg refuses
// the image for any reason, the file is written again without it.
func writeTrackFile(c Youtube, track models.Track, input, outputPath string, metadata []string) error {
	// Audio() is what the explicit "map": "0:a" used to do: keep the audio and
	// drop anything else the downloaded file may carry.
	audio := ffmpeg.Input(input).Audio()
	streams := []*ffmpeg.Stream{audio}
	opts := ffmpeg.KwArgs{
		"metadata": metadata,
		"loglevel": "error",
	}
	withCover := false

	if c.Cfg.EmbedCoverArt && track.CoverURL != "" {
		cover, how := coverFor(track, c.Cfg.CoversDir, filepath.Ext(outputPath))
		switch how {
		case coverStream:
			// Video() makes this "-map 1:v". The image also has to be re-encoded
			// and flagged: left alone ffmpeg stores it as a regular video stream
			// (h264 in mp4, png in mp3), and copied as-is a WebP served under a
			// .jpg name makes the mp4 muxer fail.
			streams = append(streams, ffmpeg.Input(cover).Video())
			opts["c:v"] = "mjpeg"
			opts["disposition:v"] = "attached_pic"
			// Without these the picture type is 0 ("other") instead of 3
			// ("front cover").
			opts["metadata:s:v"] = []string{"title=Album cover", "comment=Cover (front)"}
			withCover = true
		case coverVorbisComment:
			if tag, err := vorbisPictureTag(cover); err != nil {
				slog.Warn(
					"could not build cover art tag, writing track without artwork",
					"track", track.Title,
					logging.RuntimeAttr(err.Error()),
				)
			} else {
				opts["metadata"] = append(metadata, "metadata_block_picture="+tag)
				withCover = true
			}
		}
	}

	if err := util.WriteMetadata(streams, c.Cfg.FfmpegPath, outputPath, opts); err != nil {
		if !withCover {
			return err
		}
		// A track is worth more than its artwork: whatever went wrong with the
		// cover, write the file again without it before giving up.
		slog.Warn(
			"writing with cover art failed, retrying without it",
			"track", track.Title,
			logging.RuntimeAttr(err.Error()),
		)
		plain := ffmpeg.KwArgs{"metadata": metadata, "loglevel": "error"}
		return util.WriteMetadata([]*ffmpeg.Stream{audio}, c.Cfg.FfmpegPath, outputPath, plain)
	}

	return nil
}

func saveVideo(c Youtube, track models.Track, stream *goutubedl.DownloadResult) bool {

	defer func() {
		if err := stream.Close(); err != nil {
			slog.Warn("closing stream failed", "context", err.Error())
		}
	}()

	input := filepath.Join(c.DownloadDir, track.File+".tmp")
	file, err := os.Create(input)
	if err != nil {
		slog.Error("failed to create song file", "context", err.Error())
		return false
	}

	defer func() {
		if err := file.Close(); err != nil {
			slog.Warn("file close failed", "context", err.Error())
		}
	}()

	defer func() {
    	if err := os.Remove(input); err != nil {
			slog.Debug(
				fmt.Sprintf("failed to remove %s", input),
				logging.RuntimeAttr(err.Error()),
			)
    	}
}()

	if _, err = io.Copy(file, stream); err != nil {
		slog.Error("failed to copy stream to file", "context", err.Error())
		return false
	}

	metadata := util.BuildffmpegMetadata(track)

	outputPath := filepath.Join(c.DownloadDir, track.File)

	if c.Cfg.PathTemplate != "" {
			outputPath = filepath.Join(
				c.DownloadDir,
				buildTrackPath(c.Cfg.PathTemplate, &track),
			)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			slog.Error("failed to create output directory", "context", err.Error())
			return false
	}

	if err := writeTrackFile(c, track, input, outputPath, metadata); err != nil {
		slog.Error("failed to write track", "track", track.Title, "context", err.Error())
		return false
	}

	return true
}

// filter out video ID
func (c *Youtube) gatherVideo(cfg cfg.Youtube, videos Videos, track models.Track) string {

	// Try to get the video from the official or topic channel
	if id := getTopic(cfg, videos, track); id != "" {
		return id

	}
	// If official video isn't found, try the first suitable channel
	for _, video := range videos.Items {
		if !ContainsKeyword(track, video.Snippet.Title, c.Cfg.Filters.FilterList) {
			return video.ID.VideoID
		}
	}

	return ""
}

func fetchAndSaveVideo(ctx context.Context, cfg Youtube, track models.Track) bool {
	stream, err := getVideo(ctx, cfg, track.ID)
	if err != nil {
		slog.Error("failed getting stream for video", "trackID", track.ID, "context", err.Error())
		return false
	}

	if stream != nil {
		return saveVideo(cfg, track, stream)
	}

	slog.Error("stream was empty for video", "trackID", track.ID)
	return false
}

func (c *Youtube) GetDownloadStatus(tracks []*models.Track) (map[string]FileStatus, error) {
	return nil, fmt.Errorf("no monitoring required")
}

func (c *Youtube) Cleanup(track models.Track, ID string) error {
	return nil
}
