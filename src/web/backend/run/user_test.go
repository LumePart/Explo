package run

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"explo/src/web/backend/app"
)

func TestRunArgs_NoUserUnchanged(t *testing.T) {
	got := buildArgs("weekly", "normal", "/e/.env", false, "")
	want := []string{"--config", "/e/.env", "--playlist", "weekly", "--download-mode", "normal", "--replace-playlist=false"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildArgs = %v, want %v", got, want)
	}
}

func TestRunArgs_UserAndAll(t *testing.T) {
	if got := buildArgs("", "", "/e", true, "all"); !reflect.DeepEqual(got, []string{"--config", "/e", "--user", "all"}) {
		t.Errorf("all: %v", got)
	}
	if got := buildArgs("", "", "/e", true, "alice"); !reflect.DeepEqual(got, []string{"--config", "/e", "--user", "alice"}) {
		t.Errorf("alice: %v", got)
	}
}

func TestRun_InvalidUserRejected(t *testing.T) {
	mr := NewManualRun(app.Config{ExploPath: "/bin/sleep"})
	form := url.Values{"user": {"../x"}}
	req := httptest.NewRequest(http.MethodPost, "/api/ui/run", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mr.HandleRun(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if mr.currentRunStatus().Running {
		t.Error("a process was started")
	}
}

func seedRunUsers(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, "users.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestResolveRunUser(t *testing.T) {
	t.Run("empty with users becomes all", func(t *testing.T) {
		mr := NewManualRun(app.Config{WebDataDir: seedRunUsers(t, `[{"id":"a","enabled":true}]`)})
		got, err := mr.resolveRunUser("")
		if err != nil || got != "all" {
			t.Errorf("got %q err %v", got, err)
		}
	})
	t.Run("explicit user unchanged", func(t *testing.T) {
		mr := NewManualRun(app.Config{WebDataDir: seedRunUsers(t, `[{"id":"a","enabled":true}]`)})
		if got, _ := mr.resolveRunUser("a"); got != "a" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("no users stays empty and args byte-identical", func(t *testing.T) {
		mr := NewManualRun(app.Config{WebDataDir: seedRunUsers(t, "")})
		got, err := mr.resolveRunUser("")
		if err != nil || got != "" {
			t.Fatalf("got %q err %v", got, err)
		}
		args := buildArgs("weekly", "normal", "/e/.env", false, got)
		want := []string{"--config", "/e/.env", "--playlist", "weekly", "--download-mode", "normal", "--replace-playlist=false"}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("args = %v", args)
		}
	})
	t.Run("corrupt users.json errors", func(t *testing.T) {
		mr := NewManualRun(app.Config{WebDataDir: seedRunUsers(t, "{bad")})
		if _, err := mr.resolveRunUser(""); err == nil {
			t.Error("want error")
		}
	})
}

func TestRun_CorruptUsersIs500AndNothingStarted(t *testing.T) {
	mr := NewManualRun(app.Config{ExploPath: "/bin/sleep", WebDataDir: seedRunUsers(t, "{bad")})
	req := httptest.NewRequest(http.MethodPost, "/api/ui/run", strings.NewReader(url.Values{"playlist": {"weekly"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mr.HandleRun(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if mr.currentRunStatus().Running {
		t.Error("a process was started")
	}
}
