package playlist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"explo/src/models"
)

func TestCacheDir_EmptyUserIsLegacyPath(t *testing.T) {
	got, err := CacheDir("/data", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/data", "cache"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCacheDir_UserPath(t *testing.T) {
	got, err := CacheDir("/data", "kelly-w")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/data", "cache", "users", "kelly-w"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCacheDir_InvalidIDErrors(t *testing.T) {
	for _, id := range []string{"../../etc", "a/b", "..", "A", strings.Repeat("a", 33)} {
		got, err := CacheDir("/data", id)
		if err == nil || got != "" {
			t.Errorf("CacheDir(%q) = %q, %v; want empty path and error", id, got, err)
		}
	}
}

func TestWritePlaylistCache_WritesUnderUserDir(t *testing.T) {
	dir := t.TempDir()
	tracks := []*models.Track{{CleanTitle: "T", Artist: "A", Album: "R", CoverPath: "x"}}
	WritePlaylistCache(dir, "kelly-w", "weekly-exploration", tracks, nil)
	if _, err := os.Stat(filepath.Join(dir, "cache", "users", "kelly-w", "weekly-exploration.json")); err != nil {
		t.Errorf("user cache not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache", "weekly-exploration.json")); err == nil {
		t.Error("legacy cache must not be written for a user")
	}
	WritePlaylistCache(dir, "", "weekly-exploration", tracks, nil)
	if _, err := os.Stat(filepath.Join(dir, "cache", "weekly-exploration.json")); err != nil {
		t.Errorf("legacy cache not written: %v", err)
	}
	WritePlaylistCache(dir, "../bad", "p", tracks, nil)
	if _, err := os.Stat(filepath.Join(dir, "cache", "p.json")); err == nil {
		t.Error("invalid id must write nothing")
	}
}

func TestRemoveUserCache_RemovesOnlyThatUser(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"users/a/x.json", "users/b/x.json", "covers/c.jpg"} {
		full := filepath.Join(dir, "cache", p)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveUserCache(dir, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache", "users", "a")); !os.IsNotExist(err) {
		t.Error("user a should be removed")
	}
	for _, p := range []string{"users/b/x.json", "covers/c.jpg"} {
		if _, err := os.Stat(filepath.Join(dir, "cache", p)); err != nil {
			t.Errorf("%s should survive: %v", p, err)
		}
	}
	if err := RemoveUserCache(dir, "a"); err != nil {
		t.Errorf("missing dir should not error: %v", err)
	}
	if err := RemoveUserCache(dir, ".."); err == nil {
		t.Error("invalid id should error")
	}
	if err := RemoveUserCache(dir, ""); err == nil {
		t.Error("empty id must error (would remove nothing/legacy)")
	}
}
