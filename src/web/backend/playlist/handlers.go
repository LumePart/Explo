package playlist

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"explo/src/util"
	"explo/src/web"
	"explo/src/web/backend/defs"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// handleGetCustomPlaylists returns all saved custom playlists with a track_count
// derived from their cache file (if present) and the current sync schedule from .env.
func (p *Playlist) HandleGetCustomPlaylists(w http.ResponseWriter, r *http.Request) {
	playlists := loadCustomPlaylists(p.cfg.WebDataDir)

	// Read .env to look up schedule state for each custom playlist.
	var envValues map[string]string
	if data, err := os.ReadFile(p.cfg.WebEnvPath); err == nil {
		envValues = p.settings.ParseEnvText(string(data))
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
	for _, plist := range playlists {
		count := customPlaylistTrackCount(p.cfg.WebDataDir, plist.ID)
		prefix := util.CustomEnvPrefix(plist.Name)
		sched := envValues[prefix+"_SCHEDULE"]
		flags := envValues[prefix+"_FLAGS"]
		items = append(items, respItem{CustomPlaylist: plist, TrackCount: count, Schedule: sched, Flags: flags})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		slog.Error("custom-playlists: failed to write response", "err", err)
	}
}

// handleImportCustomPlaylist imports a playlist by URL (ListenBrainz or Apple Music),
// writes a cache, and returns the playlist name/tracks to the frontend for the import animation.
func (p *Playlist) HandleImportCustomPlaylist(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL         string `json:"url"`
		Source      string `json:"source"` // "listenbrainz" | "apple_music"
		RefreshDays int    `json:"refresh_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	existing := loadCustomPlaylists(p.cfg.WebDataDir)

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

	// Read .env
	var envValues map[string]string
	var enabled = false
	if data, err := os.ReadFile(p.cfg.WebEnvPath); err == nil {
		envValues = p.settings.ParseEnvText(string(data))
	} else {
		envValues = map[string]string{}
	}
	if envValues["SUFFIX_REMOVAL"] == "true" {
		enabled = true
	}

	result, err := fetchCustomPlaylistTracks(CustomPlaylist{Source: body.Source, SourceURL: body.URL}, enabled)
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
	if err := os.MkdirAll(filepath.Join(p.cfg.WebDataDir, "cache"), 0755); err != nil {
		slog.Error("custom-playlists: failed to create data dir", "err", err)
		http.Error(w, "server data directory unavailable: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Generate a short unique ID
	id := fmt.Sprintf("custom-%x", rand.Uint32())

	// Write cache with remote cover URLs synchronously so the response is fast,
	// then download local copies of cover art in the background.
	slog.Info("custom-playlists: writing cache", "id", id)
	if !writePreliminaryCache(p.cfg.WebDataDir, id, tracks) {
		http.Error(w, "failed to write playlist cache", http.StatusInternalServerError)
		return
	}
	go downloadAndCacheCovers(p.cfg.WebDataDir, id, tracks)

	// Cache the playlist's own artwork locally so we can later push it to the
	// music app on first playlist creation. Apple Music imports have artwork;
	// ListenBrainz don't.
	if artworkURL != "" {
		go func() {
			if _, err := util.DownloadFile(artworkURL, CustomPlaylistArtworkPath(p.cfg.WebDataDir, id)); err != nil {
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
	if err := saveCustomPlaylists(p.cfg.WebDataDir, existing); err != nil {
		slog.Error("custom-playlists: failed to save metadata", "err", err)
		http.Error(w, "failed to save playlist metadata: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Mark the playlist as active by writing FLAGS. For non-Never cadence, also write
	// a daily poll SCHEDULE — RefreshDays in the JSON gates the actual refresh interval
	// inside the cron task body. "Never" imports get FLAGS only so the card is usable
	// for manual runs while the schedule editor pre-selects "Never".
	prefix := util.CustomEnvPrefix(name)
	envUpdates := map[string]string{
		prefix + "_FLAGS": "--playlist " + id,
	}
	if body.RefreshDays > 0 {
		envUpdates[prefix+"_SCHEDULE"] = "0 4 * * *"
	}
	_ = p.settings.UpdateEnvKeys(envUpdates, web.SampleEnv)

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
func (p *Playlist) HandleRefreshCustomPlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !defs.CustomIDRe.MatchString(id) {
		http.Error(w, "invalid playlist id", http.StatusBadRequest)
		return
	}

	playlists := loadCustomPlaylists(p.cfg.WebDataDir)
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

	var envValues map[string]string
	var AMSuffixRe = false
	if data, err := os.ReadFile(p.cfg.WebEnvPath); err == nil {
		envValues = p.settings.ParseEnvText(string(data))
	} else {
		envValues = map[string]string{}
	}
	if envValues["SUFFIX_REMOVAL"] == "true" {
		AMSuffixRe = true
	}

	plist := playlists[idx]
	slog.Info("custom-playlists: manual refresh", "id", id, "source", plist.Source)

	result, err := fetchCustomPlaylistTracks(plist, AMSuffixRe)
	if err != nil {
		slog.Error("custom-playlists: refresh fetch failed", "id", id, "err", err)
		http.Error(w, "failed to fetch playlist: "+err.Error(), http.StatusBadGateway)
		return
	}
	tracks := result.Tracks

	if !writePreliminaryCache(p.cfg.WebDataDir, id, tracks) {
		http.Error(w, "failed to write playlist cache", http.StatusInternalServerError)
		return
	}
	go downloadAndCacheCovers(p.cfg.WebDataDir, id, tracks)

	playlists[idx].LastFetched = time.Now().UTC()
	if err := saveCustomPlaylists(p.cfg.WebDataDir, playlists); err != nil {
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
func (p *Playlist) HandleDeleteCustomPlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !defs.CustomIDRe.MatchString(id) {
		slog.Warn("custom-playlists: invalid id in delete request", "id", id)
		http.Error(w, "invalid playlist id", http.StatusBadRequest)
		return
	}
	deleteTracks := r.URL.Query().Get("delete_tracks") == "true"
	slog.Info("custom-playlists: delete request", "id", id, "delete_tracks", deleteTracks)

	existing := loadCustomPlaylists(p.cfg.WebDataDir)
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

	if err := saveCustomPlaylists(p.cfg.WebDataDir, filtered); err != nil {
		http.Error(w, "failed to save: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove the cache file; ignore error if already gone
	cachePath := filepath.Join(p.cfg.WebDataDir, "cache", id+".json")
	_ = os.Remove(cachePath)

	// Remove schedule env vars from .env
	prefix := util.CustomEnvPrefix(deletedName)
	_ = p.settings.UpdateEnvKeys(map[string]string{
		prefix + "_SCHEDULE": "",
		prefix + "_FLAGS":    "",
	}, web.SampleEnv)

	if deleteTracks {
		if data, err := os.ReadFile(p.cfg.WebEnvPath); err == nil {
			env := p.settings.ParseEnvText(string(data))
			if env["USE_SUBDIRECTORY"] == "true" && env["DOWNLOAD_DIR"] != "" {
				prefix := cases.Title(language.Und).String(id) // "custom-1234" -> "Custom-1234"
				removed, err := util.RemoveDirsByPrefix(env["DOWNLOAD_DIR"], prefix)
				if err != nil {
					slog.Warn("custom-playlists: track cleanup failed", "id", id, "err", err)
				} else {
					slog.Info("custom-playlists: removed download dirs", "id", id, "count", removed)
					if removed > 0 {
						util.TriggerLibraryRefresh(p.cfg)
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

// handleGetPlaylist serves the tracklist cache written by explo during its last run.
// Returns an empty track list if no cache exists yet.
func (p *Playlist) HandleGetPlaylist(w http.ResponseWriter, r *http.Request) {
	playlistType := r.URL.Query().Get("type")
	if !isValidPlaylistID(playlistType) {
		http.Error(w, "unknown playlist type", http.StatusBadRequest)
		return
	}

	cachePath := filepath.Join(p.cfg.WebDataDir, "cache", playlistType+".json")
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

// handlePrefetchCovers fetches the most recent LB playlists for the given user,
// writes a preliminary JSON cache for the web UI, then downloads cover art.
// Runs in the background — returns 202 immediately.
func (p *Playlist) HandlePrefetchCovers(w http.ResponseWriter, r *http.Request) {
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
			cachePath := filepath.Join(p.cfg.WebDataDir, "cache", pt+".json")
			if _, err := os.Stat(cachePath); err == nil && !forceRefresh {
				slog.Info("prefetch: cache already exists, skipping", "playlist", pt)
				continue
			}
			var tracks []PlaylistTrack
			var err error
			if pt == "on-repeat" {
				tracks, err = fetchOnRepeatTracks(body.User)
			} else {
				tracks, err = fetchMostRecentLBPlaylist(body.User, pt)
			}
			if err != nil {
				slog.Warn("prefetch: failed to fetch LB playlist", "type", pt, "err", err)
				continue
			}
			slog.Info("prefetch: fetched tracks", "playlist", pt, "count", len(tracks))
			writePrefetchCache(p.cfg.WebDataDir, pt, tracks)
		}
	}()
}

// handleBackgroundArt returns a single cover art URL for use as a login page backdrop.
// It picks a random local cover if any exist; otherwise it fetches the top global
// albums from ListenBrainz and downloads cover art for the first available one.
func (p *Playlist) HandleBackgroundArt(w http.ResponseWriter, r *http.Request) {
	coversDir := filepath.Join(p.cfg.WebDataDir, "cache", "covers")

	url := randomLocalCoverHiRes(coversDir)
	if url == "" {
		url = fetchSitewideCovers(coversDir)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"url": url}); err != nil {
		slog.Error("background-art: failed to write response", "err", err.Error())
	}
}
