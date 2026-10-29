package client

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"explo/src/models"
	"explo/src/util"
)

type recordedReq struct {
	Method, Path, Title string
}

func playlistServer(t *testing.T, reqs *[]recordedReq) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*reqs = append(*reqs, recordedReq{r.Method, r.URL.Path, r.URL.Query().Get("title")})
		mu.Unlock()
		body := `{}`
		if r.Method == http.MethodPost && r.URL.Path == "/playlists" {
			body = `{"MediaContainer":{"Metadata":[{"ratingKey":"99"}]}}`
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("fake server failed to write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPlexCreatePlaylist_PostsTitleAndAddsTracks(t *testing.T) {
	var reqs []recordedReq
	server := playlistServer(t, &reqs)

	plex := NewPlex(testConfig(server.URL, ""), util.NewHttp(util.HttpClientConfig{Timeout: 5}))
	tracks := []*models.Track{{ID: "/library/metadata/1"}, {ID: "/library/metadata/2"}}

	if err := plex.CreatePlaylist(tracks); err != nil {
		t.Fatalf("CreatePlaylist() = %v", err)
	}

	var posts, puts int
	for _, r := range reqs {
		switch {
		case r.Method == "POST" && r.Path == "/playlists":
			posts++
			if r.Title != "Weekly Exploration" {
				t.Errorf("title = %q, want Weekly Exploration", r.Title)
			}
		case r.Method == "PUT" && r.Path == "/playlists/99/items":
			puts++
		}
	}
	if posts != 1 || puts != 2 || len(reqs) != 3 {
		t.Errorf("requests = %+v, want 1 POST /playlists and 2 PUT /playlists/99/items", reqs)
	}
	if plex.Cfg.PlaylistID != "99" {
		t.Errorf("PlaylistID = %q, want 99", plex.Cfg.PlaylistID)
	}
}
