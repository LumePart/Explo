package playlist

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"explo/src/web/backend/app"
	"explo/src/web/backend/settings"
	"explo/src/web/backend/users"
)

const validLBURL = "https://listenbrainz.org/playlist/12345678-1234-1234-1234-123456789abc"

func writeUsersFile(t *testing.T, dir string, ids ...string) []users.User {
	t.Helper()
	var list []users.User
	for _, id := range ids {
		list = append(list, users.User{ID: id, Name: id, Enabled: true})
	}
	raw, _ := json.Marshal(list)
	if err := os.WriteFile(filepath.Join(dir, "users.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return list
}

func newTestPlaylist(t *testing.T) (*Playlist, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := app.Config{WebDataDir: dir, WebEnvPath: filepath.Join(dir, ".env")}
	old := fetchTracks
	fetchTracks = func(CustomPlaylist) (FetchResult, error) { return FetchResult{Name: "Mix"}, nil }
	t.Cleanup(func() { fetchTracks = old })
	return NewPlaylist(cfg, settings.NewSettings(cfg)), dir
}

func doJSON(h http.HandlerFunc, path, id string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(body)
	req := httptest.NewRequest("POST", path, &buf)
	if id != "" {
		req.SetPathValue("id", id)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestImport_RequiresUserWhenUsersExist(t *testing.T) {
	p, dir := newTestPlaylist(t)
	writeUsersFile(t, dir, "alice", "bob")

	if rec := doJSON(p.HandleImportCustomPlaylist, "/x", "", map[string]any{"url": validLBURL}); rec.Code != 400 {
		t.Errorf("missing user_id: %d", rec.Code)
	}
	if rec := doJSON(p.HandleImportCustomPlaylist, "/x", "", map[string]any{"url": validLBURL, "user_id": "nobody"}); rec.Code != 400 {
		t.Errorf("unknown user_id: %d", rec.Code)
	}
	rec := doJSON(p.HandleImportCustomPlaylist, "/x", "", map[string]any{"url": validLBURL, "user_id": "bob"})
	if rec.Code != 200 {
		t.Fatalf("valid user_id: %d %s", rec.Code, rec.Body)
	}
	got := loadCustomPlaylists(dir)
	if len(got) != 1 || got[0].UserID != "bob" {
		t.Errorf("stored: %+v", got)
	}
}

func TestImport_NoUsersFileStillWorksWithoutUser(t *testing.T) {
	p, dir := newTestPlaylist(t)
	rec := doJSON(p.HandleImportCustomPlaylist, "/x", "", map[string]any{"url": validLBURL})
	if rec.Code != 200 {
		t.Fatalf("code %d %s", rec.Code, rec.Body)
	}
	if got := loadCustomPlaylists(dir); len(got) != 1 || got[0].UserID != "" {
		t.Errorf("stored: %+v", got)
	}
}

func TestReassign_ChangesOwner(t *testing.T) {
	p, dir := newTestPlaylist(t)
	writeUsersFile(t, dir, "alice", "bob")
	_ = saveCustomPlaylists(dir, []CustomPlaylist{{ID: "custom-1", UserID: "alice"}})

	rec := doJSON(p.HandleReassignCustomPlaylist, "/x", "custom-1", map[string]any{"user_id": "bob"})
	if rec.Code != 200 {
		t.Fatalf("code %d %s", rec.Code, rec.Body)
	}
	if got := OwnerOf(dir, "custom-1"); got != "bob" {
		t.Errorf("owner %q", got)
	}
	for name, c := range map[string]struct {
		id   string
		body map[string]any
		want int
	}{
		"unknown user":    {"custom-1", map[string]any{"user_id": "zed"}, 400},
		"bad user id":     {"custom-1", map[string]any{"user_id": "../x"}, 400},
		"bad playlist id": {"nope", map[string]any{"user_id": "bob"}, 400},
		"missing list":    {"custom-999", map[string]any{"user_id": "bob"}, 404},
	} {
		if rec := doJSON(p.HandleReassignCustomPlaylist, "/x", c.id, c.body); rec.Code != c.want {
			t.Errorf("%s: %d want %d", name, rec.Code, c.want)
		}
	}
}

func TestOwnerOf_LegacyEntryBelongsToFirstUser(t *testing.T) {
	dir := t.TempDir()
	writeUsersFile(t, dir, "alice", "bob")
	_ = saveCustomPlaylists(dir, []CustomPlaylist{{ID: "custom-1"}, {ID: "custom-2", UserID: "bob"}})
	if got := OwnerOf(dir, "custom-1"); got != "alice" {
		t.Errorf("legacy owner %q", got)
	}
	if got := OwnerOf(dir, "custom-2"); got != "bob" {
		t.Errorf("owner %q", got)
	}
	if got := OwnerOf(t.TempDir(), "custom-1"); got != "" {
		t.Errorf("no users: %q", got)
	}
}

func TestDeleteUser_ReassignsOrphanedPlaylistsToFirstRemainingUser(t *testing.T) {
	dir := t.TempDir()
	rem := writeUsersFile(t, dir, "bob", "carol")
	_ = saveCustomPlaylists(dir, []CustomPlaylist{{ID: "custom-1", UserID: "alice"}, {ID: "custom-2", UserID: "carol"}})
	if err := OnUserDeleted(dir, "alice", rem); err != nil {
		t.Fatal(err)
	}
	if got := OwnerOf(dir, "custom-1"); got != "bob" {
		t.Errorf("custom-1 owner %q", got)
	}
	if got := OwnerOf(dir, "custom-2"); got != "carol" {
		t.Errorf("custom-2 owner %q", got)
	}
}

func TestDeleteLastUser_ClearsOwnerAndKeepsPlaylists(t *testing.T) {
	dir := t.TempDir()
	_ = saveCustomPlaylists(dir, []CustomPlaylist{{ID: "custom-1", UserID: "alice"}})
	if err := OnUserDeleted(dir, "alice", nil); err != nil {
		t.Fatal(err)
	}
	got := loadCustomPlaylists(dir)
	if len(got) != 1 || got[0].UserID != "" {
		t.Errorf("stored: %+v", got)
	}
}
