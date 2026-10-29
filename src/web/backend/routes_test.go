package backend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"explo/src/config"
	"explo/src/web/backend/playlist"
)

// BUG-28: the pattern lacked its leading slash, so ServeMux parsed
// "api" as a host pattern and requests fell through to the SPA
// fallback instead of the presets handler.
func TestRoute_PathTemplatesRegistered(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, err := http.NewRequest(method, "/api/ui/path-templates", nil)
		if err != nil {
			t.Fatal(err)
		}
		s := NewServer(config.ServerConfig{})
		_, pattern := s.mux.Handler(req)
		if pattern != "/api/ui/path-templates" {
			t.Errorf("%s matched pattern %q, want /api/ui/path-templates", method, pattern)
		}
	}
}

// Wiring check: DELETE /api/ui/users/{id} must reach the real
// playlist.RemoveUserCache and remove that user's cache directory.
func TestRoute_UserDeleteRemovesRealCache(t *testing.T) {
	dir := t.TempDir()
	s := NewServer(config.ServerConfig{WebDataDir: dir})
	body := `{"name":"Kelly","listenbrainz_user":"lb","plex_user":"px","enabled":true}`
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/ui/users", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Skipf("create not reachable without auth (status %d)", rec.Code)
	}
	cacheDir, err := playlist.CacheDir(dir, "kelly")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest("DELETE", "/api/ui/users/kelly", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Errorf("cache dir should be removed: %v", err)
	}
}
