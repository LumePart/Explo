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

	"explo/src/discovery"
	"explo/src/util"
	"explo/src/web"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// CustomPlaylist holds the metadata for a user-imported playlist.
type CustomPlaylist struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Source          string    `json:"source"`                     // "listenbrainz" | "apple_music" | "spotify"
	SourceURL       string    `json:"source_url,omitempty"`       // original URL for dedup + refresh
	LBMBID          string    `json:"lb_mbid,omitempty"`          // ListenBrainz MBID (backward compat)
	ArtworkURL      string    `json:"artwork_url,omitempty"`      // playlist cover image (Apple Music)
	ArtworkUploaded bool      `json:"artwork_uploaded,omitempty"` // true after artwork has been pushed to the music app
	RefreshDays     int       `json:"refresh_days"`
	ColorIndex      int       `json:"color_index"`
	LastFetched     time.Time `json:"last_fetched"`
}

// CustomPlaylistArtworkPath returns the local file path where a playlist's
// artwork is cached (regardless of whether the file exists).
func CustomPlaylistArtworkPath(cfgDir, id string) string {
	return filepath.Join(cfgDir, "cache", "playlist_artwork", id+".jpg")
}

// GetCustomPlaylist looks up a custom playlist by ID. Returns nil if not found.
func GetCustomPlaylist(cfgDir, id string) *CustomPlaylist {
	for _, p := range loadCustomPlaylists(cfgDir) {
		if p.ID == id {
			cp := p
			return &cp
		}
	}
	return nil
}

// MarkCustomPlaylistArtworkUploaded sets ArtworkUploaded=true and persists.
func MarkCustomPlaylistArtworkUploaded(cfgDir, id string) error {
	playlists := loadCustomPlaylists(cfgDir)
	for i := range playlists {
		if playlists[i].ID == id {
			if playlists[i].ArtworkUploaded {
				return nil
			}
			playlists[i].ArtworkUploaded = true
			return saveCustomPlaylists(cfgDir, playlists)
		}
	}
	return nil
}

var lbMBIDRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

var appleMusicURLRe = regexp.MustCompile(
	`^https?://music\.apple\.com/[a-z]{2}/playlist/[^/]+/(pl\.[a-zA-Z0-9-]+)`,
)

// extractAppleMusicID pulls the playlist ID (pl.xxx) from an Apple Music URL.
func extractAppleMusicID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	m := appleMusicURLRe.FindStringSubmatch(raw)
	if len(m) < 2 {
		return "", fmt.Errorf("not a valid Apple Music playlist URL")
	}
	return m[1], nil
}

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

// customEnvPrefix converts a playlist name like "Today's Hits"
// to an env-var prefix like "CUSTOM_TODAYS_HITS".
// Non-alphanumeric characters are collapsed into underscores.
func customEnvPrefix(name string) string {
	var b strings.Builder
	prevUnderscore := true // start true so leading separators are skipped
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevUnderscore = false
		} else if !prevUnderscore {
			b.WriteRune('_')
			prevUnderscore = true
		}
	}
	return "CUSTOM_" + strings.TrimRight(b.String(), "_")
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

// FetchResult is the uniform return type for fetching playlist data from any source.
type FetchResult struct {
	Name       string
	ArtworkURL string
	Tracks     []PlaylistTrack
}

// fetchCustomPlaylistTracks dispatches to the appropriate source fetcher.
// This is the single point where source-specific logic lives for fetching.
func fetchCustomPlaylistTracks(p CustomPlaylist) (FetchResult, error) {
	switch p.Source {
	case "apple_music":
		name, art, tracks, err := fetchAppleMusicPlaylist(p.SourceURL)
		return FetchResult{name, art, tracks}, err
	case "spotify":
		name, art, tracks, err := fetchSpotifyPlaylist(p.SourceURL)
		return FetchResult{name, art, tracks}, err
	default: // "listenbrainz" or legacy empty
		mbid := p.LBMBID
		if mbid == "" && p.SourceURL != "" {
			var err error
			mbid, err = extractLBMBID(p.SourceURL)
			if err != nil {
				return FetchResult{}, err
			}
		}
		if mbid == "" {
			return FetchResult{}, fmt.Errorf("no source data for playlist %s", p.ID)
		}
		httpClient := util.NewHttp(util.HttpClientConfig{Timeout: 30})
		name, modelTracks, err := discovery.FetchPlaylistByMBID(httpClient, mbid)
		if err != nil {
			return FetchResult{}, err
		}
		tracks := modelTracksToPlaylistTracks(modelTracks)
		return FetchResult{Name: name, Tracks: tracks}, nil
	}
}

// extractSourceID validates a URL and returns the canonical ID for the given source.
func extractSourceID(source, url string) (string, error) {
	switch source {
	case "apple_music":
		return extractAppleMusicID(url)
	case "spotify":
		return extractSpotifyID(url)
	default:
		return extractLBMBID(url)
	}
}

// isDuplicate checks whether a playlist with the same source and source ID already exists.
func isDuplicate(source, sourceID string, existing []CustomPlaylist) (string, bool) {
	for _, p := range existing {
		if p.Source != source && p.Source != "" {
			continue
		}
		existID, _ := extractSourceID(p.Source, p.SourceURL)
		if existID == "" && p.LBMBID != "" {
			existID = p.LBMBID
		}
		if existID == "" {
			continue
		}
		if existID == sourceID {
			return p.ID, true
		}
	}
	return "", false
}

// handleGetCustomPlaylists returns all saved custom playlists with a track_count
// derived from their cache file (if present) and the current sync schedule from .env.
func (s *Server) handleGetCustomPlaylists(w http.ResponseWriter, r *http.Request) {
	playlists := loadCustomPlaylists(s.cfg.WebDataDir)

	// Read .env to look up schedule state for each custom playlist.
	var envValues map[string]string
	if data, err := os.ReadFile(s.cfg.WebEnvPath); err == nil {
		envValues = parseEnvText(string(data))
	} else {
		envValues = map[string]string{}
	}

	type respItem struct {
		CustomPlaylist
		TrackCount int    `json:"track_count"`
		Schedule   string `json:"schedule"`
		Flags      string `json:"flags"`
	}
	items := make([]respItem, 0, len(playlists))
	for _, p := range playlists {
		count := customPlaylistTrackCount(s.cfg.WebDataDir, p.ID)
		prefix := customEnvPrefix(p.Name)
		sched := envValues[prefix+"_SCHEDULE"]
		flags := envValues[prefix+"_FLAGS"]
		items = append(items, respItem{CustomPlaylist: p, TrackCount: count, Schedule: sched, Flags: flags})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		slog.Error("custom-playlists: failed to write response", "err", err)
	}
}

// handleImportCustomPlaylist imports a playlist by URL (ListenBrainz or Apple Music),
// writes a cache, and returns the playlist name/tracks to the frontend for the import animation.
func (s *Server) handleImportCustomPlaylist(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL         string `json:"url"`
		Source      string `json:"source"` // "listenbrainz" | "apple_music"
		RefreshDays int    `json:"refresh_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	existing := loadCustomPlaylists(s.cfg.WebDataDir)

	if body.Source == "" {
		body.Source = "listenbrainz"
	}

	sourceID, err := extractSourceID(body.Source, body.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slog.Info("custom-playlists: import request", "source", body.Source, "source_id", sourceID, "refresh_days", body.RefreshDays)

	if existingID, dup := isDuplicate(body.Source, sourceID, existing); dup {
		slog.Warn("custom-playlists: duplicate import rejected", "source_id", sourceID, "existing_id", existingID)
		http.Error(w, "playlist already imported", http.StatusConflict)
		return
	}

	result, err := fetchCustomPlaylistTracks(CustomPlaylist{Source: body.Source, SourceURL: body.URL})
	if err != nil {
		slog.Error("custom-playlists: fetch failed", "source", body.Source, "err", err)
		http.Error(w, "failed to fetch playlist: "+err.Error(), http.StatusBadGateway)
		return
	}

	name := result.Name
	tracks := result.Tracks
	artworkURL := result.ArtworkURL
	if name == "" {
		name = "Imported Playlist"
	}
	slog.Info("custom-playlists: fetched", "source", body.Source, "name", name, "tracks", len(tracks))

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

	// Cache the playlist's own artwork locally so we can later push it to the
	// music app on first playlist creation. Apple Music imports have artwork;
	// ListenBrainz don't.
	if artworkURL != "" {
		go func() {
			if _, err := util.DownloadFile(artworkURL, CustomPlaylistArtworkPath(s.cfg.WebDataDir, id)); err != nil {
				slog.Warn("custom-playlists: artwork download failed", "id", id, "err", err.Error())
			}
		}()
	}

	// Save metadata
	// Derive LBMBID for backward compatibility (LB playlists only)
	var lbMBID string
	if body.Source != "apple_music" && body.Source != "spotify" {
		lbMBID = sourceID
	}

	cp := CustomPlaylist{
		ID:          id,
		Name:        name,
		Source:      body.Source,
		SourceURL:   body.URL,
		LBMBID:      lbMBID,     // empty for apple_music
		ArtworkURL:  artworkURL, // empty for listenbrainz
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

	// Mark the playlist as active by writing FLAGS. For non-Never cadence, also write
	// a daily poll SCHEDULE — RefreshDays in the JSON gates the actual refresh interval
	// inside the cron task body. "Never" imports get FLAGS only so the card is usable
	// for manual runs while the schedule editor pre-selects "Never".
	prefix := customEnvPrefix(name)
	envUpdates := map[string]string{
		prefix + "_FLAGS": "--playlist " + id,
	}
	if body.RefreshDays > 0 {
		envUpdates[prefix+"_SCHEDULE"] = "0 4 * * *"
	}
	_ = updateEnvKeys(s.cfg.WebEnvPath, envUpdates, web.SampleEnv)

	slog.Info("custom-playlists: import complete", "id", id, "name", name)

	// Collect up to 6 unique remote cover URLs for the import animation
	seen := make(map[string]bool)
	covers := make([]string, 0, 6)
	for _, t := range tracks {
		if t.CoverURL != "" && !seen[t.CoverURL] {
			seen[t.CoverURL] = true
			covers = append(covers, t.CoverURL)
		}
		if len(covers) >= 6 {
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":          id,
		"name":        name,
		"track_count": len(tracks),
		"cover_urls":  covers,
		"color_index": cp.ColorIndex,
		"artwork_url": artworkURL,
	}); err != nil {
		slog.Error("custom-playlists: failed to write import response", "err", err)
	}
}

// handleRefreshCustomPlaylist re-fetches a custom playlist and updates the cache.
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
	slog.Info("custom-playlists: manual refresh", "id", id, "source", p.Source)

	result, err := fetchCustomPlaylistTracks(p)
	if err != nil {
		slog.Error("custom-playlists: refresh fetch failed", "id", id, "err", err)
		http.Error(w, "failed to fetch playlist: "+err.Error(), http.StatusBadGateway)
		return
	}
	tracks := result.Tracks

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
// If ?delete_tracks=true is set and USE_SUBDIRECTORY is on, also removes the
// playlist's download subdirectories from DOWNLOAD_DIR.
func (s *Server) handleDeleteCustomPlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !customIDRe.MatchString(id) {
		slog.Warn("custom-playlists: invalid id in delete request", "id", id)
		http.Error(w, "invalid playlist id", http.StatusBadRequest)
		return
	}
	deleteTracks := r.URL.Query().Get("delete_tracks") == "true"
	slog.Info("custom-playlists: delete request", "id", id, "delete_tracks", deleteTracks)

	existing := loadCustomPlaylists(s.cfg.WebDataDir)
	filtered := existing[:0]
	found := false
	var deletedName string
	for _, p := range existing {
		if p.ID == id {
			found = true
			deletedName = p.Name
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

	// Remove schedule env vars from .env
	prefix := customEnvPrefix(deletedName)
	_ = updateEnvKeys(s.cfg.WebEnvPath, map[string]string{
		prefix + "_SCHEDULE": "",
		prefix + "_FLAGS":    "",
	}, web.SampleEnv)

	if deleteTracks {
		if data, err := os.ReadFile(s.cfg.WebEnvPath); err == nil {
			env := parseEnvText(string(data))
			if env["USE_SUBDIRECTORY"] == "true" && env["DOWNLOAD_DIR"] != "" {
				prefix := cases.Title(language.Und).String(id) // "custom-1234" -> "Custom-1234"
				removed, err := util.RemoveDirsByPrefix(env["DOWNLOAD_DIR"], prefix)
				if err != nil {
					slog.Warn("custom-playlists: track cleanup failed", "id", id, "err", err)
				} else {
					slog.Info("custom-playlists: removed download dirs", "id", id, "count", removed)
					if removed > 0 {
						s.triggerLibraryRefresh()
					}
				}
			} else {
				slog.Info("custom-playlists: skipping track cleanup", "id", id,
					"use_subdir", env["USE_SUBDIRECTORY"], "download_dir", env["DOWNLOAD_DIR"])
			}
		}
	}

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
