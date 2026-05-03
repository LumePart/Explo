package backend

import (
	"encoding/json"
	"explo/src/models"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const lbAPIBase = "https://api.listenbrainz.org/1"

// validPlaylistTypes is derived from playlistDefs — no manual sync needed.
var validPlaylistTypes = func() map[string]bool {
	m := make(map[string]bool, len(playlistDefs))
	for k := range playlistDefs {
		m[k] = true
	}
	return m
}()

// handleGetPlaylist serves the tracklist cache written by explo during its last run.
// Returns an empty track list if no cache exists yet.
func (s *Server) handleGetPlaylist(w http.ResponseWriter, r *http.Request) {
	playlistType := r.URL.Query().Get("type")
	if !validPlaylistTypes[playlistType] {
		http.Error(w, "unknown playlist type", http.StatusBadRequest)
		return
	}

	cachePath := filepath.Join(filepath.Dir(s.configPath), "cache", playlistType+".json")
	if raw, err := os.ReadFile(cachePath); err == nil {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(raw); err != nil {
			slog.Error("failed to write playlist response", "msg", err.Error())
		}
		return
	}

	// No cache yet — return an empty response. Run explo or use the prefetch
	// endpoint to populate the cache.
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(`{"tracks":[]}`)); err != nil {
		slog.Error("failed to write empty playlist response", "msg", err.Error())
	}
}

// ── LB fallback ──────────────────────────────────────────────────────────────

type lbCreatedForResp struct {
	Count         int `json:"count"`
	Offset        int `json:"offset"`
	PlaylistCount int `json:"playlist_count"`
	Playlists     []struct {
		Playlist struct {
			Date       time.Time `json:"date"`
			Identifier string    `json:"identifier"`
			Extension  struct {
				JspfPlaylist struct {
					AdditionalMetadata struct {
						AlgorithmMetadata struct {
							SourcePatch string `json:"source_patch"`
						} `json:"algorithm_metadata"`
					} `json:"additional_metadata"`
				} `json:"https://musicbrainz.org/doc/jspf#playlist"`
			} `json:"extension"`
		} `json:"playlist"`
	} `json:"playlists"`
}

type lbPlaylistResp struct {
	Playlist struct {
		Track []struct {
			Title     string `json:"title"`
			Creator   string `json:"creator"`
			Album     string `json:"album"`
			Extension struct {
				JspfTrack struct {
					AdditionalMetadata struct {
						CaaID          int64  `json:"caa_id"`
						CaaReleaseMbid string `json:"caa_release_mbid"`
					} `json:"additional_metadata"`
				} `json:"https://musicbrainz.org/doc/jspf#track"`
			} `json:"extension"`
		} `json:"track"`
	} `json:"playlist"`
}

func fetchMostRecentLBPlaylist(username, playlistType string) ([][4]string, time.Time, error) {
	var offset int
	var bestDate time.Time
	var bestID string

	for {
		url := fmt.Sprintf("%s/user/%s/playlists/createdfor?offset=%d", lbAPIBase, username, offset)
		body, err := lbGet(url)
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("createdfor fetch: %w", err)
		}
		var resp lbCreatedForResp
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, time.Time{}, fmt.Errorf("createdfor parse: %w", err)
		}
		for _, p := range resp.Playlists {
			patch := p.Playlist.Extension.JspfPlaylist.AdditionalMetadata.AlgorithmMetadata.SourcePatch
			if patch != playlistType {
				continue
			}
			if bestID == "" || p.Playlist.Date.After(bestDate) {
				bestDate = p.Playlist.Date
				parts := strings.Split(p.Playlist.Identifier, "/")
				bestID = parts[len(parts)-1]
			}
		}
		fetched := resp.Count + resp.Offset
		if fetched >= resp.PlaylistCount || resp.Count == 0 {
			break
		}
		offset += resp.Count
	}

	if bestID == "" {
		return nil, time.Time{}, nil
	}

	body, err := lbGet(fmt.Sprintf("%s/playlist/%s", lbAPIBase, bestID))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("playlist fetch: %w", err)
	}
	var resp lbPlaylistResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, time.Time{}, fmt.Errorf("playlist parse: %w", err)
	}

	out := make([][4]string, 0, len(resp.Playlist.Track))
	for _, t := range resp.Playlist.Track {
		meta := t.Extension.JspfTrack.AdditionalMetadata
		var cover string
		if meta.CaaReleaseMbid != "" && meta.CaaID != 0 {
			cover = fmt.Sprintf("https://coverartarchive.org/release/%s/%d-250.jpg",
				meta.CaaReleaseMbid, meta.CaaID)
		}
		out = append(out, [4]string{t.Title, t.Creator, t.Album, cover})
	}
	return out, bestDate, nil
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
		InLibrary *bool  `json:"inLibrary,omitempty"`
	}
	type cache struct {
		Tracks []cachedTrack `json:"tracks"`
	}

	cfgDir := filepath.Dir(cfgPath)
	coversDir := filepath.Join(cfgDir, "cache", "covers")
	if err := os.MkdirAll(coversDir, 0755); err != nil {
		slog.Error("failed making directory", "msg", err.Error())
	}

	ct := make([]cachedTrack, len(tracks))
	for i, t := range tracks {
		localCover := downloadCover(t.CoverURL, coversDir)
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
			CoverURL:  localCover,
			InLibrary: inLibrary,
		}
	}

	raw, err := json.Marshal(cache{Tracks: ct})
	if err != nil {
		return
	}
	cacheDir := filepath.Join(cfgDir, "cache")
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

// downloadCover downloads coverURL into coversDir and returns "/api/covers/<mbid>.jpg".
// Returns "" if url is empty.
func downloadCover(url, coversDir string) string {
	if url == "" {
		return ""
	}
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	mbid := parts[len(parts)-2]
	destPath := filepath.Join(coversDir, mbid+".jpg")
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		resp, err := http.Get(url) //nolint:noctx
		if err == nil {
			func() {
				defer func() {
					if cerr := resp.Body.Close(); cerr != nil {
						slog.Error("failed to close cover response", "err", cerr.Error())
					}
				}()
				if resp.StatusCode == http.StatusOK {
					if data, err := io.ReadAll(resp.Body); err == nil {
						if err := os.WriteFile(destPath, data, 0644); err != nil {
							slog.Error("failed writing cover", "path", destPath, "err", err.Error())
						}
					}
				}
			}()
		}
	}
	return "/api/covers/" + mbid + ".jpg"
}

// handlePrefetchCovers fetches the most recent LB playlists for the given user,
// writes a preliminary JSON cache for the web UI, then downloads cover art.
// Runs in the background — returns 202 immediately.
func (s *Server) handlePrefetchCovers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User      string   `json:"user"`
		Playlists []string `json:"playlists"`
		Source    string   `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.User == "" || len(body.Playlists) == 0 {
		http.Error(w, "user and playlists are required", http.StatusBadRequest)
		return
	}

	cfgDir := filepath.Dir(s.configPath)
	forceRefresh := body.Source == "wizard"
	w.WriteHeader(http.StatusAccepted)

	slog.Info("prefetch: starting", "user", body.User, "playlists", body.Playlists, "source", body.Source, "force_refresh", forceRefresh)
	go func() {
		for _, pt := range body.Playlists {
			if !validPlaylistTypes[pt] {
				slog.Warn("prefetch: unknown playlist type", "type", pt)
				continue
			}
			// Normal prefetch keeps an existing cache intact; wizard prefetch refreshes it
			// after the user updates discovery settings.
			cachePath := filepath.Join(cfgDir, "cache", pt+".json")
			if _, err := os.Stat(cachePath); err == nil && !forceRefresh {
				slog.Info("prefetch: cache already exists, skipping", "playlist", pt)
				continue
			}
			tracks, _, err := fetchMostRecentLBPlaylist(body.User, pt)
			if err != nil {
				slog.Warn("prefetch: failed to fetch LB playlist", "type", pt, "err", err)
				continue
			}
			slog.Info("prefetch: fetched tracks", "playlist", pt, "count", len(tracks))
			writePrefetchCache(cfgDir, pt, tracks)
		}
	}()
}

type cachedPrefetchTrack struct {
	Rank     int    `json:"rank"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Release  string `json:"release"`
	CoverURL string `json:"coverUrl,omitempty"`
}

func writePrefetchCache(cfgDir, playlistType string, tracks [][4]string) {
	ct := make([]cachedPrefetchTrack, len(tracks))
	for i, t := range tracks {
		ct[i] = cachedPrefetchTrack{
			Rank:     i + 1,
			Title:    t[0],
			Artist:   t[1],
			Release:  t[2],
			CoverURL: t[3],
		}
	}

	if !writeTrackCache(cfgDir, playlistType, ct) {
		return
	}
	slog.Info("prefetch: cache written", "playlist", playlistType, "covers", "remote")

	coversDir := filepath.Join(cfgDir, "cache", "covers")
	if err := os.MkdirAll(coversDir, 0755); err != nil {
		slog.Error("prefetch: failed to create covers dir", "err", err.Error())
		return
	}

	for i, t := range tracks {
		ct[i].CoverURL = downloadCover(t[3], coversDir)
	}
	if writeTrackCache(cfgDir, playlistType, ct) {
		slog.Info("prefetch: cache updated", "playlist", playlistType, "covers", "local")
	}
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
