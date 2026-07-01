package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cfg "explo/src/config"
	"explo/src/models"
)

// spotiflacMinBytes is the smallest file the CLI path treats as a successful
// download. A real lossless (or even lossy fallback) track is multiple megabytes;
// a partial/aborted download is far smaller, so this guards against half-written
// files being reported as "present".
const spotiflacMinBytes = 64 * 1024

// Spotiflac downloads lossless (FLAC) tracks via SpotiFLAC
// (https://github.com/ShuShuzinhuu/SpotiFLAC-Module-Version), with two paths:
//
//   - Tracks that carry a streaming URL (e.g. Spotify-imported playlists) are
//     downloaded with the official `spotiflac` CLI, which matches the exact track.
//   - Tracks matched only by metadata (e.g. ListenBrainz discovery: ISRC or
//     title/artist) are downloaded with the bundled module helper
//     (spotiflac_dl.py), which searches each FLAC provider by ISRC with a
//     title/artist text-search fallback — mirroring how the youtube service shells
//     out to ytmusicapi.
//
// Either way SpotiFLAC tries the configured sources (deezer, tidal, qobuz, amazon, netease, joox)
// in priority order. Tracks with neither a URL nor enough metadata are skipped so
// the other configured services (slskd/youtube) can take over.
type Spotiflac struct {
	DownloadDir string
	Cfg         cfg.Spotiflac
}

func NewSpotiflac(cfg cfg.Spotiflac, downloadDir string) *Spotiflac {
	return &Spotiflac{
		DownloadDir: downloadDir,
		Cfg:         cfg,
	}
}

// spotiflacPayload is the JSON contract passed to the module helper.
type spotiflacPayload struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Artists     string   `json:"artists"`
	Album       string   `json:"album"`
	AlbumArtist string   `json:"album_artist"`
	ISRC        string   `json:"isrc"`
	TrackNumber int      `json:"track_number"`
	DurationMs  int      `json:"duration_ms"`
	CoverURL    string   `json:"cover_url"`
	OutputDir   string   `json:"output_dir"`
	Sources     []string `json:"sources"`
	TimeoutS    int      `json:"timeout_s,omitempty"`
}

// spotiflacResult is the JSON the module helper prints on stdout.
type spotiflacResult struct {
	Success  bool   `json:"success"`
	File     string `json:"file"`
	Path     string `json:"path"`
	Provider string `json:"provider"`
	Format   string `json:"format"`
	Error    string `json:"error"`
}

func (c *Spotiflac) QueryTrack(track *models.Track) error {
	// SpotiFLAC resolves and downloads in a single step (GetTrack). Accept the
	// track if we have any usable handle on it; otherwise return an error so
	// StartDownload skips it and the next configured service takes over.
	if track.SourceURL != "" || firstISRC(track) != "" {
		return nil
	}
	if (track.CleanTitle != "" || track.Title != "") && track.Artist != "" {
		return nil
	}
	return fmt.Errorf("[spotiflac] insufficient metadata (need a streaming URL, ISRC, or title+artist) for '%s - %s'", track.Title, track.Artist)
}

func (c *Spotiflac) GetTrack(track *models.Track) error {
	if track.SourceURL != "" {
		return c.getViaCLI(track)
	}
	return c.getViaHelper(track)
}

// getViaCLI downloads a track that carries a streaming URL using the official
// `spotiflac` CLI. The CLI emits no machine-readable result, so success is
// determined by the deterministic -o output file, not the exit code.
func (c *Spotiflac) getViaCLI(track *models.Track) error {
	bin := c.Cfg.BinPath
	if bin == "" {
		bin = "spotiflac"
	}

	filename := getFilename(track.CleanTitle, track.MainArtist) + ".flac"
	outPath := filepath.Join(c.DownloadDir, filename)

	// Clear any stale file at the target path so the post-run existence check
	// reliably reflects this download.
	_ = os.Remove(outPath)

	quality := c.Cfg.Quality
	if quality == "" {
		quality = "LOSSLESS"
	}

	args := []string{
		track.SourceURL,
		c.DownloadDir,
		"-o", outPath,
		"-q", quality,
		"--retries", strconv.Itoa(c.Cfg.Retries),
		// Skip lyrics + metadata enrichment: keeps batch downloads fast and
		// deterministic. The music system handles richer metadata itself.
		"--no-lyrics", "--no-enrich",
	}
	if c.Cfg.Timeout > 0 {
		args = append(args, "--timeout", strconv.Itoa(c.Cfg.Timeout))
	}
	// --service takes a space-separated list (nargs='+'), so it must come last
	// to avoid argparse swallowing the positional url/output_dir arguments.
	if len(c.Cfg.Sources) > 0 {
		args = append(args, "-s")
		args = append(args, c.Cfg.Sources...)
	}

	ctx := context.Background()
	if c.Cfg.Timeout > 0 {
		// Generous outer ceiling so a wedged process can't hang a run: the
		// per-track --timeout bounds each attempt; retries cycle all providers
		// with backoff, so budget for (retries+1) attempts plus IO/metadata slack.
		budget := c.Cfg.Timeout*(c.Cfg.Retries+1) + 120
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(budget)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	info, statErr := os.Stat(outPath)
	if statErr != nil || info.Size() < spotiflacMinBytes {
		detail := lastLine(stderr.String())
		if detail == "" && runErr != nil {
			detail = runErr.Error()
		}
		if detail == "" {
			detail = "no file produced"
		}
		return fmt.Errorf("[spotiflac] no download for '%s - %s': %s", track.CleanTitle, track.Artist, detail)
	}

	track.File = filename
	track.Present = true
	slog.Info("download finished", "service", "spotiflac", "track", filename, "source", track.SourceURL)
	return nil
}

// getViaHelper downloads a track matched only by metadata (ISRC or title/artist)
// using the bundled SpotiFLAC module helper, which searches each provider.
func (c *Spotiflac) getViaHelper(track *models.Track) error {
	albumArtist := track.AlbumArtist
	if albumArtist == "" {
		albumArtist = track.MainArtist
	}
	title := track.CleanTitle
	if title == "" {
		title = track.Title
	}

	payload := spotiflacPayload{
		ID:          track.MusicBrainzTrackID,
		Title:       title,
		Artists:     track.Artist,
		Album:       track.Album,
		AlbumArtist: albumArtist,
		ISRC:        firstISRC(track),
		TrackNumber: track.TrackNumber,
		DurationMs:  track.Duration,
		CoverURL:    track.CoverURL,
		OutputDir:   c.DownloadDir,
		Sources:     c.Cfg.Sources,
		TimeoutS:    c.Cfg.Timeout,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal spotiflac payload: %w", err)
	}

	ctx := context.Background()
	if c.Cfg.Timeout > 0 {
		// Give the subprocess slack over its own per-track timeout so it can
		// report a clean failure before the context kills it.
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(c.Cfg.Timeout+30)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, c.Cfg.PythonPath, c.Cfg.ScriptPath, string(body))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, runErr := cmd.Output() // helper exits non-zero on failure but still prints JSON

	result, parseErr := parseSpotiflacResult(stdout)
	if parseErr != nil {
		detail := lastLine(stderr.String())
		if detail == "" && runErr != nil {
			detail = runErr.Error()
		}
		return fmt.Errorf("[spotiflac] helper failed for '%s - %s': %s", title, track.Artist, detail)
	}

	if !result.Success {
		return fmt.Errorf("[spotiflac] no download for '%s - %s': %s", title, track.Artist, result.Error)
	}

	track.File = result.File
	track.Present = true
	slog.Info("download finished", "service", "spotiflac", "track", result.File, "source", result.Provider)
	return nil
}

// Monitor interface — SpotiFLAC downloads synchronously, there is no queue to poll.
func (c *Spotiflac) GetConf() (MonitorConfig, error) {
	return MonitorConfig{}, fmt.Errorf("[spotiflac] no monitoring required")
}

func (c *Spotiflac) GetDownloadStatus(tracks []*models.Track) (map[string]FileStatus, error) {
	return nil, fmt.Errorf("[spotiflac] no monitoring required")
}

func (c *Spotiflac) Cleanup(track models.Track, ID string) error {
	return nil
}

func firstISRC(track *models.Track) string {
	if len(track.ISRCs) > 0 {
		return track.ISRCs[0]
	}
	return ""
}

// parseSpotiflacResult reads the last JSON object printed on the helper's stdout.
func parseSpotiflacResult(stdout []byte) (*spotiflacResult, error) {
	line := lastJSONLine(string(stdout))
	if line == "" {
		return nil, fmt.Errorf("no JSON output")
	}
	var res spotiflacResult
	if err := json.Unmarshal([]byte(line), &res); err != nil {
		return nil, fmt.Errorf("failed to parse helper output: %w", err)
	}
	return &res, nil
}

func lastJSONLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); strings.HasPrefix(l, "{") {
			return l
		}
	}
	return ""
}

// lastLine returns the last non-empty line of s, used to surface a concise
// failure reason from a subprocess's stderr.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}
