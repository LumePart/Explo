package playlist

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"explo/src/web/backend/users"
)

func TestGetPlaylist_UserCacheAndInvalidUser(t *testing.T) {
	p, dir := newTestPlaylist(t)
	ud := filepath.Join(dir, "cache", "users", "alice")
	if err := os.MkdirAll(ud, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(ud, "weekly-exploration.json"), []byte(`{"tracks":[{"title":"mine"}]}`), 0o644)

	get := func(q string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		p.HandleGetPlaylist(rec, httptest.NewRequest(http.MethodGet, "/api/ui/playlists?type=weekly-exploration"+q, nil))
		return rec
	}
	if rec := get("&user=alice"); !bytes.Contains(rec.Body.Bytes(), []byte("mine")) {
		t.Errorf("user cache not served: %s", rec.Body.String())
	}
	if rec := get("&user=..%2Fx"); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid user status = %d", rec.Code)
	}
	if rec := get(""); rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte(`{"tracks":[]}`)) {
		t.Errorf("legacy path changed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPrefetch_UsesThatUsersListenBrainzName(t *testing.T) {
	p, dir := newTestPlaylist(t)
	raw, _ := json.Marshal([]users.User{{ID: "alice", Name: "Alice", ListenBrainzUser: "alice-lb", Enabled: true}})
	_ = os.WriteFile(filepath.Join(dir, "users.json"), raw, 0o600)

	got := make(chan string, 1)
	old := prefetchLBPlaylist
	prefetchLBPlaylist = func(username, pt string) ([]PlaylistTrack, error) {
		got <- username
		return []PlaylistTrack{{Title: "t"}}, nil
	}
	t.Cleanup(func() { prefetchLBPlaylist = old })

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		p.HandlePrefetchCovers(rec, httptest.NewRequest(http.MethodPost, "/api/ui/playlists/prefetch", bytes.NewBufferString(body)))
		return rec
	}
	if rec := post(`{"user_id":"ghost","playlists":["weekly-exploration"]}`); rec.Code != 400 {
		t.Errorf("unknown user status = %d", rec.Code)
	}
	if rec := post(`{"user_id":"../x","playlists":["weekly-exploration"]}`); rec.Code != 400 {
		t.Errorf("invalid user status = %d", rec.Code)
	}
	if rec := post(`{"user_id":"alice","user":"env-name","playlists":["weekly-exploration"]}`); rec.Code != 202 {
		t.Fatalf("status = %d", rec.Code)
	}
	select {
	case name := <-got:
		if name != "alice-lb" {
			t.Errorf("fetched for %q, want alice-lb", name)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no fetch")
	}
	want := filepath.Join(dir, "cache", "users", "alice", "weekly-exploration.json")
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(want); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("cache not written at %s", want)
}
