package users

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeHooks struct {
	removed []string
}

func newAPI(t *testing.T) (*http.ServeMux, string, *fakeHooks) {
	t.Helper()
	dir := t.TempDir()
	fh := &fakeHooks{}
	h := NewHandlers(dir, Hooks{RemoveCache: func(dataDir, id string) error {
		fh.removed = append(fh.removed, id)
		return os.RemoveAll(filepath.Join(dataDir, "cache", "users", id))
	}})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ui/users", h.HandleList)
	mux.HandleFunc("POST /api/ui/users", h.HandleCreate)
	mux.HandleFunc("PUT /api/ui/users/{id}", h.HandleUpdate)
	mux.HandleFunc("DELETE /api/ui/users/{id}", h.HandleDelete)
	return mux, dir, fh
}

func do(mux http.Handler, method, url string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, url, &buf)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func m(name string) map[string]any {
	return map[string]any{"name": name, "listenbrainz_user": "lb", "listenbrainz_token": "secret123", "plex_user": "px", "enabled": true}
}

func list(t *testing.T, mux http.Handler) []map[string]any {
	t.Helper()
	rec := do(mux, "GET", "/api/ui/users", nil)
	if rec.Code != 200 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUsersAPI_CreateListUpdateDelete(t *testing.T) {
	mux, _, _ := newAPI(t)
	rec := do(mux, "POST", "/api/ui/users", m("Kelly W."))
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created["id"] != "kelly-w" || created["token_set"] != true || created["enabled"] != true {
		t.Errorf("created = %v", created)
	}
	if got := list(t, mux); len(got) != 1 || got[0]["id"] != "kelly-w" {
		t.Errorf("list = %v", got)
	}
	upd := m("Kelly Renamed")
	upd["plex_user"] = "px2"
	upd["enabled"] = false
	if rec := do(mux, "PUT", "/api/ui/users/kelly-w", upd); rec.Code != 200 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	got := list(t, mux)
	if got[0]["name"] != "Kelly Renamed" || got[0]["plex_user"] != "px2" || got[0]["enabled"] != false || got[0]["id"] != "kelly-w" {
		t.Errorf("after update = %v", got)
	}
	if rec := do(mux, "PUT", "/api/ui/users/nope", upd); rec.Code != 404 {
		t.Errorf("update missing: %d", rec.Code)
	}
	if rec := do(mux, "DELETE", "/api/ui/users/kelly-w", nil); rec.Code != 200 && rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := do(mux, "DELETE", "/api/ui/users/kelly-w", nil); rec.Code != 404 {
		t.Errorf("delete missing: %d", rec.Code)
	}
}

func TestUsersAPI_TokenNeverReturned(t *testing.T) {
	mux, _, _ := newAPI(t)
	bodies := []string{
		do(mux, "POST", "/api/ui/users", m("Kelly")).Body.String(),
		do(mux, "GET", "/api/ui/users", nil).Body.String(),
		do(mux, "PUT", "/api/ui/users/kelly", m("Kelly")).Body.String(),
	}
	for i, b := range bodies {
		if strings.Contains(b, "secret123") {
			t.Errorf("body %d leaks token: %s", i, b)
		}
		if !strings.Contains(b, "token_set") {
			t.Errorf("body %d lacks token_set: %s", i, b)
		}
	}
}

func TestUsersAPI_PutEmptyTokenKeepsToken(t *testing.T) {
	mux, dir, _ := newAPI(t)
	do(mux, "POST", "/api/ui/users", m("Kelly"))
	upd := m("Kelly")
	upd["listenbrainz_token"] = ""
	if rec := do(mux, "PUT", "/api/ui/users/kelly", upd); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	us, _ := Load(dir)
	if len(us) != 1 || us[0].ListenBrainzToken != "secret123" {
		t.Errorf("token not kept: %+v", us)
	}
	upd["listenbrainz_token"] = "newtok"
	do(mux, "PUT", "/api/ui/users/kelly", upd)
	us, _ = Load(dir)
	if us[0].ListenBrainzToken != "newtok" {
		t.Errorf("token not replaced: %+v", us)
	}
}

func TestUsersAPI_PutIgnoresBodyID(t *testing.T) {
	mux, dir, _ := newAPI(t)
	do(mux, "POST", "/api/ui/users", m("Kelly"))
	upd := m("Kelly")
	upd["id"] = "evil"
	if rec := do(mux, "PUT", "/api/ui/users/kelly", upd); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	us, _ := Load(dir)
	if len(us) != 1 || us[0].ID != "kelly" {
		t.Errorf("id changed: %+v", us)
	}
}

func TestUsersAPI_ValidationErrors(t *testing.T) {
	mux, _, _ := newAPI(t)
	for name, mut := range map[string]func(map[string]any){
		"empty name":   func(b map[string]any) { b["name"] = "" },
		"blank name":   func(b map[string]any) { b["name"] = "   " },
		"no lb user":   func(b map[string]any) { b["listenbrainz_user"] = "" },
		"no plex user": func(b map[string]any) { b["plex_user"] = "" },
		"unslugable":   func(b map[string]any) { b["name"] = "!!!" },
	} {
		b := m("Kelly")
		mut(b)
		if rec := do(mux, "POST", "/api/ui/users", b); rec.Code != 400 {
			t.Errorf("%s: got %d", name, rec.Code)
		}
	}
	b := m("Kelly")
	b["listenbrainz_token"] = ""
	if rec := do(mux, "POST", "/api/ui/users", b); rec.Code != 200 && rec.Code != 201 {
		t.Errorf("empty token should be accepted: %d %s", rec.Code, rec.Body)
	}
	rec := do(mux, "POST", "/api/ui/users", m("kelly!"))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "a user with that id already exists") {
		t.Errorf("duplicate: %d %s", rec.Code, rec.Body)
	}
	if rec := do(mux, "PUT", "/api/ui/users/kelly", func() map[string]any { b := m("Kelly"); b["name"] = ""; return b }()); rec.Code != 400 {
		t.Errorf("put empty name: %d", rec.Code)
	}
	if rec := httptest.NewRecorder(); true {
		req := httptest.NewRequest("POST", "/api/ui/users", strings.NewReader("{bad"))
		mux.ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Errorf("bad json: %d", rec.Code)
		}
	}
}

func TestUsersAPI_CorruptFileIs500NotEmpty(t *testing.T) {
	mux, dir, _ := newAPI(t)
	if err := os.WriteFile(filepath.Join(dir, "users.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ method, url string }{
		{"GET", "/api/ui/users"}, {"POST", "/api/ui/users"}, {"PUT", "/api/ui/users/kelly"}, {"DELETE", "/api/ui/users/kelly"},
	} {
		var body any
		if c.method == "POST" || c.method == "PUT" {
			body = m("Kelly")
		}
		if rec := do(mux, c.method, c.url, body); rec.Code != 500 {
			t.Errorf("%s: got %d, want 500", c.method, rec.Code)
		}
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "users.json"))
	if string(raw) != "{not json" {
		t.Errorf("corrupt file overwritten: %s", raw)
	}
}

func TestUsersAPI_DeleteRemovesCacheDirNotOthers(t *testing.T) {
	mux, dir, fh := newAPI(t)
	do(mux, "POST", "/api/ui/users", m("Kelly"))
	do(mux, "POST", "/api/ui/users", m("Bob"))
	for _, id := range []string{"kelly", "bob"} {
		if err := os.MkdirAll(filepath.Join(dir, "cache", "users", id), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if rec := do(mux, "DELETE", "/api/ui/users/kelly", nil); rec.Code != 200 && rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache", "users", "kelly")); !os.IsNotExist(err) {
		t.Error("kelly cache should be gone")
	}
	if _, err := os.Stat(filepath.Join(dir, "cache", "users", "bob")); err != nil {
		t.Error("bob cache should remain")
	}
	if len(fh.removed) != 1 || fh.removed[0] != "kelly" {
		t.Errorf("hook calls = %v", fh.removed)
	}
	if got := list(t, mux); len(got) != 1 || got[0]["id"] != "bob" {
		t.Errorf("list = %v", got)
	}
}

func TestUsersAPI_DeleteLastUser(t *testing.T) {
	mux, _, _ := newAPI(t)
	do(mux, "POST", "/api/ui/users", m("Kelly"))
	do(mux, "DELETE", "/api/ui/users/kelly", nil)
	rec := do(mux, "GET", "/api/ui/users", nil)
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("body = %q, want []", rec.Body)
	}
}

func TestUsersAPI_OnUserDeletedHook(t *testing.T) {
	dir := t.TempDir()
	var gotID string
	var gotRemaining []User
	h := NewHandlers(dir, Hooks{OnUserDeleted: func(_, id string, rem []User) error {
		gotID, gotRemaining = id, rem
		return nil
	}})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /u", h.HandleCreate)
	mux.HandleFunc("DELETE /u/{id}", h.HandleDelete)
	do(mux, "POST", "/u", m("Kelly"))
	do(mux, "POST", "/u", m("Bob"))
	do(mux, "DELETE", "/u/kelly", nil)
	if gotID != "kelly" || len(gotRemaining) != 1 || gotRemaining[0].ID != "bob" {
		t.Errorf("hook got %q %+v", gotID, gotRemaining)
	}
}
