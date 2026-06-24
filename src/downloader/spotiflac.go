package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	cfg "explo/src/config"
	"explo/src/models"
)

// Spotiflac downloads lossless (FLAC) tracks by shelling out to the bundled
// SpotiFLAC python wrapper (src/downloader/spotiflac/spotiflac_dl.py), mirroring
// how the youtube service uses the ytmusicapi helper. SpotiFLAC resolves a track
// by ISRC (falling back to a title/artist search) and downloads it from one of
// several FLAC sources (deezer, tidal, qobuz, amazon) in priority order.
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

// spotiflacPayload is the JSON contract passed to the python wrapper.
type spotiflacPayload struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Artists        string   `json:"artists"`
	Album          string   `json:"album"`
	AlbumArtist    string   `json:"album_artist"`
	ISRC           string   `json:"isrc"`
	TrackNumber    int      `json:"track_number"`
	DurationMs     int      `json:"duration_ms"`
	CoverURL       string   `json:"cover_url"`
	OutputDir      string   `json:"output_dir"`
	Sources        []string `json:"sources"`
	Quality        string   `json:"quality,omitempty"`
	FilenameFormat string   `json:"filename_format,omitempty"`
	QobuzToken     string   `json:"qobuz_token,omitempty"`
	TimeoutS       int      `json:"timeout_s,omitempty"`
}

// spotiflacResult is the JSON the wrapper prints on stdout.
type spotiflacResult struct {
	Success  bool   `json:"success"`
	File     string `json:"file"`
	Path     string `json:"path"`
	Provider string `json:"provider"`
	Format   string `json:"format"`
	Error    string `json:"error"`
}

func (c *Spotiflac) QueryTrack(track *models.Track) error {
	// SpotiFLAC resolves and downloads in a single step (GetTrack); here we only
	// ensure the track carries enough to match on.
	if firstISRC(track) == "" && (track.CleanTitle == "" || track.Artist == "") {
		return fmt.Errorf("[spotiflac] insufficient metadata (need ISRC or title+artist) for '%s - %s'", track.Title, track.Artist)
	}
	return nil
}

func (c *Spotiflac) GetTrack(track *models.Track) error {
	albumArtist := track.AlbumArtist
	if albumArtist == "" {
		albumArtist = track.MainArtist
	}

	payload := spotiflacPayload{
		ID:             track.MusicBrainzTrackID,
		Title:          track.CleanTitle,
		Artists:        track.Artist,
		Album:          track.Album,
		AlbumArtist:    albumArtist,
		ISRC:           firstISRC(track),
		TrackNumber:    track.TrackNumber,
		DurationMs:     track.Duration,
		CoverURL:       track.CoverURL,
		OutputDir:      c.DownloadDir,
		Sources:        c.Cfg.Sources,
		Quality:        c.Cfg.Quality,
		FilenameFormat: c.Cfg.FilenameFormat,
		QobuzToken:     c.Cfg.QobuzToken,
		TimeoutS:       c.Cfg.Timeout,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal spotiflac payload: %w", err)
	}

	ctx := context.Background()
	if c.Cfg.Timeout > 0 {
		// Give the subprocess some slack over its own per-track timeout so it can
		// report a clean failure before the context kills it.
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(c.Cfg.Timeout+30)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, c.Cfg.PythonPath, c.Cfg.ScriptPath, string(body))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, runErr := cmd.Output() // wrapper exits non-zero on failure but still prints JSON

	result, parseErr := parseSpotiflacResult(stdout)
	if parseErr != nil {
		detail := lastLine(stderr.String())
		if detail == "" && runErr != nil {
			detail = runErr.Error()
		}
		return fmt.Errorf("[spotiflac] wrapper failed for '%s - %s': %s", track.CleanTitle, track.Artist, detail)
	}

	if !result.Success {
		return fmt.Errorf("[spotiflac] no download for '%s - %s': %s", track.CleanTitle, track.Artist, result.Error)
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

// parseSpotiflacResult reads the last JSON object printed on the wrapper's stdout.
func parseSpotiflacResult(stdout []byte) (*spotiflacResult, error) {
	line := lastJSONLine(string(stdout))
	if line == "" {
		return nil, fmt.Errorf("no JSON output")
	}
	var res spotiflacResult
	if err := json.Unmarshal([]byte(line), &res); err != nil {
		return nil, fmt.Errorf("failed to parse wrapper output: %w", err)
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

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}
