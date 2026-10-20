package run

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"explo/src/web/backend/app"
)

// A finished run must leave a record answering what ran, whether it
// worked, and what failed first — without re-reading the whole log.
func TestFinishRun_WritesRecord(t *testing.T) {
	mr := NewManualRun(app.Config{WebDataDir: t.TempDir()})

	mr.state.mu.Lock()
	mr.state.running = true
	mr.state.runPlaylist = "weekly"
	mr.state.runStarted = time.Now().Add(-time.Minute)
	mr.state.logs = []string{
		`time=x level=INFO msg="starting"`,
		`time=x level=ERROR msg="first failure" service=slskd`,
		`time=x level=INFO msg="progress"`,
		`time=x level=ERROR msg="second failure" service=lidarr`,
	}
	mr.state.mu.Unlock()

	mr.finishRun(1)

	data, err := os.ReadFile(filepath.Join(mr.cfg.WebDataDir, "logs", "runs.jsonl"))
	if err != nil {
		t.Fatalf("reading records: %v", err)
	}
	records := mr.ReadRunRecords()
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1\n%s", len(records), data)
	}
	r := records[0]
	if r.Playlist != "weekly" || r.ExitCode != 1 {
		t.Errorf("record = %+v, want weekly/1", r)
	}
	if len(r.Errors) != 2 {
		t.Errorf("record errors = %d, want the 2 error lines", len(r.Errors))
	}
	if r.StartedAt.IsZero() || r.EndedAt.Before(r.StartedAt) {
		t.Errorf("record times = %v..%v, want a valid interval", r.StartedAt, r.EndedAt)
	}
}

func TestFirstErrorLines_CapsAndIgnoresInfo(t *testing.T) {
	logs := []string{`level=INFO x`, `level=ERROR a`, `level=Error b`}
	got := firstErrorLines(logs)
	if len(got) != 2 {
		t.Errorf("errors = %v, want the 2 error lines", got)
	}
	if got := playlistArg([]string{"--config", "x", "--playlist", "daily"}); got != "daily" {
		t.Errorf("playlistArg = %q, want daily", got)
	}
	if got := playlistArg([]string{"--config", "x"}); got != "" {
		t.Errorf("playlistArg = %q, want empty", got)
	}
}
