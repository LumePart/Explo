package playlist

import (
	"bytes"
	"encoding/json"
	cfg "explo/src/config"
	"explo/src/discovery"
	"explo/src/models"
	"explo/src/util"
	"explo/src/web/backend/app"
	"explo/src/web/backend/defs"
	"explo/src/web/backend/settings"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const lbAPIBase = "https://api.listenbrainz.org/1"

// PlaylistTrack represents a single track fetched from any playlist source.
// Replaces the raw [][4]string{title, artist, album, coverURL} pattern
// with named fields and adds MainArtist for accurate matching in music servers.
type PlaylistTrack struct {
	Title      string
	Artist     string
	MainArtist string
	Album      string
	CoverURL   string
}


type Playlist struct {
	settings *settings.Settings
	cfg app.Config
}

// validPlaylistTypes is derived from playlistDefs — no manual sync needed.
var validPlaylistTypes = func() map[string]bool {
	m := make(map[string]bool, len(defs.PlaylistDefs))
	for k := range defs.PlaylistDefs {
		m[k] = true
	}
	return m
}()

func NewPlaylist(cfg app.Config, settings *settings.Settings) *Playlist {
	return &Playlist{
		cfg: cfg,
		settings: settings}
}

// isValidPlaylistID accepts built-in playlist types and custom-* IDs (blocks path traversal).
func isValidPlaylistID(t string) bool {
	return validPlaylistTypes[t] || defs.CustomIDRe.MatchString(t)
}

// ── LB fallback ──────────────────────────────────────────────────────────────

func fetchOnRepeatTracks(username string) ([]PlaylistTrack, error) {
	tracks, err := discovery.FetchTopRecordings(util.NewHttp(util.HttpClientConfig{Timeout: 30}), username)
	if err != nil {
		return nil, err
	}
	return modelTracksToPlaylistTracks(tracks), nil
}

func (p *Playlist) fetchFreshReleaseTracks(username string) ([]PlaylistTrack, error) {
	config := cfg.Listenbrainz{
		User:              username,
		CoverArtSize:      "250",
		FreshReleaseDays:  30,
		FreshReleaseLimit: 8,
	}
	values := map[string]string{}
	if data, err := os.ReadFile(p.cfg.WebEnvPath); err == nil {
		values = p.settings.ParseEnvText(string(data))
	}
	for _, key := range []string{"COVER_ART_SIZE", "FRESH_RELEASES_DAYS", "FRESH_RELEASES_MAX_RELEASES"} {
		if value, ok := os.LookupEnv(key); ok {
			values[key] = value
		}
	}
	if value := values["COVER_ART_SIZE"]; value != "" {
		config.CoverArtSize = value
	}
	if value, err := strconv.Atoi(values["FRESH_RELEASES_DAYS"]); err == nil {
		config.FreshReleaseDays = value
	}
	if value, err := strconv.Atoi(values["FRESH_RELEASES_MAX_RELEASES"]); err == nil {
		config.FreshReleaseLimit = value
	}
	tracks, err := discovery.FetchFreshReleaseTracks(util.NewHttp(util.HttpClientConfig{Timeout: 60}), config)
	if err != nil {
		return nil, err
	}
	return modelTracksToPlaylistTracks(tracks), nil
}

func fetchMostRecentLBPlaylist(username, playlistType string) ([]PlaylistTrack, error) {
	tracks, err := discovery.FetchMostRecentPlaylistByType(util.NewHttp(util.HttpClientConfig{Timeout: 30}), username, playlistType)
	if err != nil {
		return nil, err
	}
	return modelTracksToPlaylistTracks(tracks), nil
}

func modelTracksToPlaylistTracks(tracks []*models.Track) []PlaylistTrack {
	out := make([]PlaylistTrack, len(tracks))
	for i, t := range tracks {
		out[i] = PlaylistTrack{
			Title:      t.CleanTitle,
			Artist:     t.Artist,
			MainArtist: t.MainArtist,
			Album:      t.Album,
			CoverURL:   t.CoverURL,
		}
	}
	return out
}

// writePlaylistCache downloads cover art and writes a tracklist JSON for the web UI.
// added maps "CleanTitle|Artist" → true for tracks that made it into the playlist; nil means status unknown.
func WritePlaylistCache(cfgPath, playlist string, tracks []*models.Track, added map[string]bool) {
	type cachedTrack struct {
		Rank      int    `json:"rank"`
		Title     string `json:"title"`
		Artist    string `json:"artist"`
		Release   string `json:"release"`
		CoverURL  string `json:"coverUrl,omitempty"`
		CoverPath string `json:"coverPath,omitempty"`
		InLibrary *bool  `json:"inLibrary,omitempty"`
	}
	type cache struct {
		Tracks []cachedTrack `json:"tracks"`
	}

	coversDir := filepath.Join(cfgPath, "cache", "covers")
	if err := os.MkdirAll(coversDir, 0755); err != nil {
		slog.Error("failed making directory", "msg", err.Error())
	}

	ct := make([]cachedTrack, len(tracks))
	for i, t := range tracks {
		apiPath, coverPath := util.DownloadCover(t.CoverURL, coversDir)
		var inLibrary *bool
		if added != nil {
			v := added[t.CleanTitle+"|"+t.Artist]
			inLibrary = &v
		}
		ct[i] = cachedTrack{
			Rank:      i + 1,
			Title:     t.CleanTitle,
			Artist:    t.Artist,
			Release:   t.Album,
			CoverURL:  apiPath,
			CoverPath: coverPath,
			InLibrary: inLibrary,
		}
	}

	raw, err := json.Marshal(cache{Tracks: ct})
	if err != nil {
		return
	}
	cacheDir := filepath.Join(cfgPath, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		slog.Error("failed creating cache dir", "msg", err.Error())
	}
	if err := os.WriteFile(filepath.Join(cacheDir, playlist+".json"), raw, 0644); err != nil {
		slog.Error("failed writing json file", "msg", err.Error())
	}
}

func lbGet(url string) ([]byte, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Error("failed to close response", "msg", err.Error())
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LB returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

type cachedPrefetchTrack struct {
	Rank       int    `json:"rank"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	MainArtist string `json:"mainArtist,omitempty"`
	Release    string `json:"release"`
	CoverURL   string `json:"coverUrl,omitempty"`
	CoverPath  string `json:"coverPath,omitempty"`
}

// writePreliminaryCache writes the track cache with remote cover URLs immediately.
// Returns false if the write fails.
func writePreliminaryCache(cfgDir, playlistType string, tracks []PlaylistTrack) bool {
	ct := make([]cachedPrefetchTrack, len(tracks))
	for i, t := range tracks {
		ct[i] = cachedPrefetchTrack{Rank: i + 1, Title: t.Title, Artist: t.Artist, MainArtist: t.MainArtist, Release: t.Album, CoverURL: t.CoverURL}
	}
	if !writeTrackCache(cfgDir, playlistType, ct) {
		return false
	}
	slog.Info("prefetch: cache written", "playlist", playlistType, "covers", "remote")
	return true
}

// downloadAndCacheCovers downloads cover art and rewrites the cache with local URLs.
// Safe to call in a goroutine.
func downloadAndCacheCovers(cfgDir, playlistType string, tracks []PlaylistTrack) {
	coversDir := filepath.Join(cfgDir, "cache", "covers")
	if err := os.MkdirAll(coversDir, 0755); err != nil {
		slog.Error("prefetch: failed to create covers dir", "err", err.Error())
		return
	}
	ct := make([]cachedPrefetchTrack, len(tracks))
	for i, t := range tracks {
		APIPath, coverPath := util.DownloadCover(t.CoverURL, coversDir)
		ct[i] = cachedPrefetchTrack{Rank: i + 1, Title: t.Title, Artist: t.Artist, MainArtist: t.MainArtist, Release: t.Album, CoverURL: APIPath, CoverPath: coverPath}
	}
	if writeTrackCache(cfgDir, playlistType, ct) {
		slog.Info("prefetch: cache updated", "playlist", playlistType, "covers", "local")
	}
}

func writePrefetchCache(cfgDir, playlistType string, tracks []PlaylistTrack) {
	if !writePreliminaryCache(cfgDir, playlistType, tracks) {
		return
	}
	downloadAndCacheCovers(cfgDir, playlistType, tracks)
}

// ── Background art ───────────────────────────────────────────────────────────

type sitewideReleasesResp struct {
	Payload struct {
		Releases []struct {
			ReleaseMbid string `json:"release_mbid"`
		} `json:"releases"`
	} `json:"payload"`
}

// randomLocalCoverHiRes picks a random cover from the existing library, ensures a
// 1200px background version is cached (as {mbid}-bg.jpg), and returns its API URL.
// Playlist thumbnails are stored at 250px; this fetches full-res on demand from CAA.
func randomLocalCoverHiRes(coversDir string) string {
	entries, err := os.ReadDir(coversDir)
	if err != nil {
		return ""
	}
	var mbids []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".jpg") && !strings.HasSuffix(name, "-bg.jpg") {
			mbids = append(mbids, strings.TrimSuffix(name, ".jpg"))
		}
	}
	if len(mbids) == 0 {
		return ""
	}
	rand.Shuffle(len(mbids), func(i, j int) { mbids[i], mbids[j] = mbids[j], mbids[i] })
	for _, mbid := range mbids[:min(3, len(mbids))] {
		bgFile := mbid + "-bg.jpg"
		bgPath := filepath.Join(coversDir, bgFile)
		if _, err := os.Stat(bgPath); err == nil {
			return "/api/covers/" + bgFile
		}
		// Download hi-res from Cover Art Archive using the release MBID
		resp, err := http.Get("https://coverartarchive.org/release/" + mbid + "/front-1200") //nolint:noctx
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close() // nolint:errcheck
			}
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close() // nolint:errcheck
		if err != nil {
			continue
		}
		if err := os.WriteFile(bgPath, data, 0644); err != nil {
			slog.Error("background-art: failed to write hi-res cover", "err", err.Error())
			continue
		}
		return "/api/covers/" + bgFile
	}
	return ""
}

// fetchSitewideCovers downloads cover art for the top global LB albums and
// returns a "/api/covers/<mbid>.jpg" URL for the first one that meets the
// minimum resolution requirement (1000px).
func fetchSitewideCovers(coversDir string) string {
	if err := os.MkdirAll(coversDir, 0755); err != nil {
		return ""
	}
	body, err := lbGet(lbAPIBase + "/stats/sitewide/releases?count=10&range=week")
	if err != nil {
		slog.Warn("background-art: LB sitewide fetch failed", "err", err)
		return ""
	}
	var resp sitewideReleasesResp
	if err := json.Unmarshal(body, &resp); err != nil {
		slog.Warn("background-art: LB sitewide parse failed", "err", err)
		return ""
	}
	for _, rel := range resp.Payload.Releases {
		if rel.ReleaseMbid == "" {
			continue
		}
		url := "https://coverartarchive.org/release/" + rel.ReleaseMbid + "/front-1200"

		dlResp, err := http.Get(url) //nolint:noctx
		if err != nil || dlResp.StatusCode != http.StatusOK {
			if dlResp != nil {
				dlResp.Body.Close() // nolint:errcheck
			}
			continue
		}
		data, err := io.ReadAll(dlResp.Body)
		dlResp.Body.Close() //nolint:errcheck
		if err != nil {
			continue
		}

		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width < 1000 || cfg.Height < 1000 {
			continue
		}

		destPath := filepath.Join(coversDir, rel.ReleaseMbid+".jpg")
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			slog.Error("background-art: failed to write sitewide cover", "err", err.Error())
			continue
		}
		return "/api/covers/" + rel.ReleaseMbid + ".jpg"
	}
	return ""
}

func writeTrackCache(cfgDir, playlistType string, tracks []cachedPrefetchTrack) bool {
	type cache struct {
		Tracks []cachedPrefetchTrack `json:"tracks"`
	}
	raw, err := json.Marshal(cache{Tracks: tracks})
	if err != nil {
		slog.Error("prefetch: failed to marshal cache", "err", err.Error())
		return false
	}
	cacheDir := filepath.Join(cfgDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		slog.Error("prefetch: failed to create cache dir", "err", err.Error())
		return false
	}
	if err := os.WriteFile(filepath.Join(cacheDir, playlistType+".json"), raw, 0644); err != nil {
		slog.Error("prefetch: failed to write cache", "err", err.Error())
		return false
	}
	return true
}
