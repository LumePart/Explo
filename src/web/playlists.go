package web

import (
	"encoding/json"
	"fmt"
	"io"
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
// Falls back to fetching the most recent playlist from ListenBrainz if no cache exists.
func (s *Server) handleGetPlaylist(w http.ResponseWriter, r *http.Request) {
	playlistType := r.URL.Query().Get("type")
	if !validPlaylistTypes[playlistType] {
		http.Error(w, "unknown playlist type", http.StatusBadRequest)
		return
	}

	cachePath := filepath.Join(filepath.Dir(s.configPath), "cache", playlistType+".json")
	if raw, err := os.ReadFile(cachePath); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
		return
	}

	// No cache yet — fall back to the most recent LB playlist (any date).
	username := os.Getenv("LISTENBRAINZ_USER")
	if username == "" {
		if data, err := os.ReadFile(s.configPath); err == nil {
			username = parseEnvText(string(data))["LISTENBRAINZ_USER"]
		}
	}
	if username == "" {
		http.Error(w, "LISTENBRAINZ_USER not configured", http.StatusBadRequest)
		return
	}

	var tracks [][4]string
	var generatedAt time.Time
	var err error

	if playlistType == "on-repeat" {
		tracks, err = fetchTopRecordingsLB(username)
	} else {
		tracks, generatedAt, err = fetchMostRecentLBPlaylist(username, playlistType)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	type cachedTrack struct {
		Rank     int    `json:"rank"`
		Title    string `json:"title"`
		Artist   string `json:"artist"`
		Release  string `json:"release"`
		CoverURL string `json:"coverUrl,omitempty"`
	}
	type response struct {
		Tracks      []cachedTrack `json:"tracks"`
		GeneratedAt *time.Time    `json:"generatedAt,omitempty"`
	}

	ct := make([]cachedTrack, len(tracks))
	for i, t := range tracks {
		ct[i] = cachedTrack{Rank: i + 1, Title: t[0], Artist: t[1], Release: t[2], CoverURL: t[3]}
	}

	var gen *time.Time
	if !generatedAt.IsZero() {
		gen = &generatedAt
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response{Tracks: ct, GeneratedAt: gen})
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
			Title   string `json:"title"`
			Creator string `json:"creator"`
			Album   string `json:"album"`
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

type lbStatsResp struct {
	Payload struct {
		Recordings []struct {
			ArtistName  string `json:"artist_name"`
			ReleaseMbid string `json:"release_mbid"`
			ReleaseName string `json:"release_name"`
			TrackName   string `json:"track_name"`
		} `json:"recordings"`
	} `json:"payload"`
}

func fetchTopRecordingsLB(username string) ([][4]string, error) {
	url := fmt.Sprintf("%s/stats/user/%s/recordings?count=30&range=month", lbAPIBase, username)
	body, err := lbGet(url)
	if err != nil {
		return nil, fmt.Errorf("stats fetch: %w", err)
	}
	var resp lbStatsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("stats parse: %w", err)
	}
	out := make([][4]string, 0, len(resp.Payload.Recordings))
	for _, r := range resp.Payload.Recordings {
		var cover string
		if r.ReleaseMbid != "" {
			cover = fmt.Sprintf("https://coverartarchive.org/release/%s/front-250", r.ReleaseMbid)
		}
		out = append(out, [4]string{r.TrackName, r.ArtistName, r.ReleaseName, cover})
	}
	return out, nil
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

func lbGet(url string) ([]byte, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LB returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
