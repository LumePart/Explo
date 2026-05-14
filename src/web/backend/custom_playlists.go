package backend

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CustomPlaylist holds the metadata for a user-imported ListenBrainz playlist.
type CustomPlaylist struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	LBMBID      string    `json:"lb_mbid"`
	RefreshDays int       `json:"refresh_days"`
	ColorIndex  int       `json:"color_index"`
	LastFetched time.Time `json:"last_fetched"`
}

var lbMBIDRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// extractLBMBID pulls the playlist UUID out of a ListenBrainz playlist URL or bare MBID string.
func extractLBMBID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	m := lbMBIDRe.FindString(raw)
	if m == "" {
		return "", fmt.Errorf("no ListenBrainz playlist UUID found in %q", raw)
	}
	return m, nil
}

func customPlaylistsPath(cfgDir string) string {
	return filepath.Join(cfgDir, "custom-playlists.json")
}

func loadCustomPlaylists(cfgDir string) []CustomPlaylist {
	data, err := os.ReadFile(customPlaylistsPath(cfgDir))
	if err != nil {
		return nil
	}
	var out []CustomPlaylist
	if err := json.Unmarshal(data, &out); err != nil {
		slog.Warn("custom-playlists: failed to parse metadata", "err", err)
		return nil
	}
	return out
}

func saveCustomPlaylists(cfgDir string, playlists []CustomPlaylist) error {
	raw, err := json.MarshalIndent(playlists, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(customPlaylistsPath(cfgDir), raw, 0644)
}

// handleGetCustomPlaylists returns all saved custom playlists with a track_count
// derived from their cache file (if present).
func (s *Server) handleGetCustomPlaylists(w http.ResponseWriter, r *http.Request) {
	playlists := loadCustomPlaylists(s.cfg.WebDataDir)

	type respItem struct {
		CustomPlaylist
		TrackCount int `json:"track_count"`
	}
	items := make([]respItem, 0, len(playlists))
	for _, p := range playlists {
		count := customPlaylistTrackCount(s.cfg.WebDataDir, p.ID)
		items = append(items, respItem{CustomPlaylist: p, TrackCount: count})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		slog.Error("custom-playlists: failed to write response", "err", err)
	}
}

// handleImportCustomPlaylist imports a ListenBrainz playlist by URL, writes a cache,
// and returns the playlist name/tracks to the frontend for the import animation.
func (s *Server) handleImportCustomPlaylist(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LBURL       string `json:"lb_url"`
		RefreshDays int    `json:"refresh_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	mbid, err := extractLBMBID(body.LBURL)
	if err != nil {
		slog.Warn("custom-playlists: invalid URL", "url", body.LBURL, "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slog.Info("custom-playlists: import request", "mbid", mbid, "refresh_days", body.RefreshDays)

	// Reject duplicates
	existing := loadCustomPlaylists(s.cfg.WebDataDir)
	for _, p := range existing {
		if p.LBMBID == mbid {
			slog.Warn("custom-playlists: duplicate import rejected", "mbid", mbid, "existing_id", p.ID)
			http.Error(w, "playlist already imported", http.StatusConflict)
			return
		}
	}

	// Fetch playlist data from ListenBrainz
	slog.Info("custom-playlists: fetching from LB", "mbid", mbid)
	name, tracks, err := fetchLBPlaylistByMBID(mbid)
	if err != nil {
		slog.Error("custom-playlists: LB fetch failed", "mbid", mbid, "err", err)
		http.Error(w, "failed to fetch playlist: "+err.Error(), http.StatusBadGateway)
		return
	}
	if name == "" {
		name = "Imported Playlist"
	}
	slog.Info("custom-playlists: fetched", "name", name, "tracks", len(tracks))

	// Ensure data directories exist before writing anything
	if err := os.MkdirAll(filepath.Join(s.cfg.WebDataDir, "cache"), 0755); err != nil {
		slog.Error("custom-playlists: failed to create data dir", "err", err)
		http.Error(w, "server data directory unavailable: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Generate a short unique ID
	id := fmt.Sprintf("custom-%x", rand.Uint32())

	// Write cache with remote cover URLs synchronously so the response is fast,
	// then download local copies of cover art in the background.
	slog.Info("custom-playlists: writing cache", "id", id)
	if !writePreliminaryCache(s.cfg.WebDataDir, id, tracks) {
		http.Error(w, "failed to write playlist cache", http.StatusInternalServerError)
		return
	}
	go downloadAndCacheCovers(s.cfg.WebDataDir, id, tracks)

	// Save metadata
	cp := CustomPlaylist{
		ID:          id,
		Name:        name,
		LBMBID:      mbid,
		RefreshDays: body.RefreshDays,
		ColorIndex:  len(existing),
		LastFetched: time.Now().UTC(),
	}
	existing = append(existing, cp)
	if err := saveCustomPlaylists(s.cfg.WebDataDir, existing); err != nil {
		slog.Error("custom-playlists: failed to save metadata", "err", err)
		http.Error(w, "failed to save playlist metadata: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("custom-playlists: import complete", "id", id, "name", name)

	// Collect up to 6 remote cover URLs for the import animation
	covers := make([]string, 0, 6)
	for _, t := range tracks {
		if t[3] != "" {
			covers = append(covers, t[3])
		}
		if len(covers) >= 6 {
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":         id,
		"name":       name,
		"track_count": len(tracks),
		"cover_urls": covers,
		"color_index": cp.ColorIndex,
	}); err != nil {
		slog.Error("custom-playlists: failed to write import response", "err", err)
	}
}

// handleRefreshCustomPlaylist re-fetches a custom playlist from ListenBrainz and updates the cache.
// Equivalent to manually triggering the nightly refresh cron job for a single playlist.
func (s *Server) handleRefreshCustomPlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !customIDRe.MatchString(id) {
		http.Error(w, "invalid playlist id", http.StatusBadRequest)
		return
	}

	playlists := loadCustomPlaylists(s.cfg.WebDataDir)
	idx := -1
	for i, p := range playlists {
		if p.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		http.Error(w, "playlist not found", http.StatusNotFound)
		return
	}

	p := playlists[idx]
	slog.Info("custom-playlists: manual refresh", "id", id, "mbid", p.LBMBID)

	_, tracks, err := fetchLBPlaylistByMBID(p.LBMBID)
	if err != nil {
		slog.Error("custom-playlists: refresh fetch failed", "id", id, "err", err)
		http.Error(w, "failed to fetch playlist: "+err.Error(), http.StatusBadGateway)
		return
	}

	if !writePreliminaryCache(s.cfg.WebDataDir, id, tracks) {
		http.Error(w, "failed to write playlist cache", http.StatusInternalServerError)
		return
	}
	go downloadAndCacheCovers(s.cfg.WebDataDir, id, tracks)

	playlists[idx].LastFetched = time.Now().UTC()
	if err := saveCustomPlaylists(s.cfg.WebDataDir, playlists); err != nil {
		slog.Warn("custom-playlists: failed to update last_fetched after refresh", "err", err)
	}

	slog.Info("custom-playlists: refresh complete", "id", id, "tracks", len(tracks))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"track_count": len(tracks)}); err != nil {
		slog.Error("custom-playlists: failed to write refresh response", "err", err)
	}
}

// handleDeleteCustomPlaylist removes a custom playlist's metadata and cache file.
func (s *Server) handleDeleteCustomPlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !customIDRe.MatchString(id) {
		slog.Warn("custom-playlists: invalid id in delete request", "id", id)
		http.Error(w, "invalid playlist id", http.StatusBadRequest)
		return
	}
	slog.Info("custom-playlists: delete request", "id", id)

	existing := loadCustomPlaylists(s.cfg.WebDataDir)
	filtered := existing[:0]
	found := false
	for _, p := range existing {
		if p.ID == id {
			found = true
		} else {
			filtered = append(filtered, p)
		}
	}
	if !found {
		http.Error(w, "playlist not found", http.StatusNotFound)
		return
	}

	if err := saveCustomPlaylists(s.cfg.WebDataDir, filtered); err != nil {
		http.Error(w, "failed to save: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove the cache file; ignore error if already gone
	cachePath := filepath.Join(s.cfg.WebDataDir, "cache", id+".json")
	_ = os.Remove(cachePath)

	slog.Info("custom-playlists: deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// customPlaylistTrackCount reads the cached track count for a custom playlist without
// fully parsing the JSON.
func customPlaylistTrackCount(cfgDir, id string) int {
	type mini struct {
		Tracks []json.RawMessage `json:"tracks"`
	}
	data, err := os.ReadFile(filepath.Join(cfgDir, "cache", id+".json"))
	if err != nil {
		return 0
	}
	var m mini
	if err := json.Unmarshal(data, &m); err != nil {
		return 0
	}
	return len(m.Tracks)
}
