package run

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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
