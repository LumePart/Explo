package run

import (
	"time"
	"fmt"
	"errors"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"context"
	"sync"

	"explo/src/web/backend/app"
)

// RunStatus is returned by GET /api/run/status.
type RunStatus struct {
	Running  bool `json:"running"`
	ExitCode *int `json:"exit_code,omitempty"`
}

type manualRunState struct {
	mu          sync.Mutex
	running     bool
	cancel      context.CancelFunc
	exitCode    *int
	logs        []string
	subscribers map[chan runEvent]struct{}
	runPlaylist string
	runStarted  time.Time
}

// RunRecord is one finished run for the diagnostics history: enough to
// answer "what ran, did it work, and what failed first" without
// re-reading the whole log.
type RunRecord struct {
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Playlist  string    `json:"playlist,omitempty"`
	ExitCode  int       `json:"exit_code"`
	Errors    []string  `json:"errors,omitempty"`
}

// maxRunRecords caps the diagnostics history file.
const maxRunRecords = 500

// maxRecordErrors caps stored error lines per run.
const maxRecordErrors = 10

// runEvent is an SSE event sent to connected browser clients.
type runEvent struct {
	typ  string
	data string
}

type ManualRun struct {
	cfg app.Config
	state manualRunState
}

var errRunAlreadyStarted = errors.New("run already in progress")

func NewManualRun(cfg app.Config) *ManualRun {
	return &ManualRun{
		cfg: cfg,
		state: newManualRunState(),
	}
}

func newManualRunState() manualRunState {
	return manualRunState{subscribers: make(map[chan runEvent]struct{})}
}

func (mr *ManualRun) startRun(args []string) error {
	// A run that wedges holds the "already running" guard below, so every later
	// run is refused until someone stops it by hand. The timeout kills the
	// child, which lets cmd.Wait return and clears the guard on its own.
	ctx, cancel := runContext(mr.cfg.RunTimeout)
	cmd := newRunCommand(ctx, mr.cfg.ExploPath, args...)
	// Strip WEB_UI from env so the child process runs normally, not as web server.
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "WEB_UI=") {
			env = append(env, e)
		}
	}
	cmd.Env = env

	pr, pw, err := os.Pipe()
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create pipe: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	lf, err := mr.openRunLog()
	if err != nil {
		slog.Warn("failed to open run log", "err", err.Error())
	}

	mr.state.mu.Lock()
	if mr.state.running {
		mr.state.mu.Unlock()
		cancel()
		if err := pr.Close(); err != nil {
			slog.Warn("failed to close file reader", "err", err.Error())
		}

		if err := pw.Close(); err != nil {
			slog.Warn("failed to close file writer", "err", err.Error())
		}
		if lf != nil {
			if err := lf.Close(); err != nil {
				slog.Warn("failed to close run log", "err", err.Error())
			}
		}
		return errRunAlreadyStarted
	}
	mr.state.running = true
	mr.state.cancel = cancel
	mr.state.exitCode = nil
	mr.state.logs = nil
	mr.state.runPlaylist = playlistArg(args)
	mr.state.runStarted = time.Now()
	mr.state.mu.Unlock()

	if err := cmd.Start(); err != nil {
		mr.finishRun(1)
		cancel()
		if err := pr.Close(); err != nil {
			slog.Warn("failed to close file reader", "err", err.Error())
		}

		if err := pw.Close(); err != nil {
			slog.Warn("failed to close file writer", "err", err.Error())
		}
		if lf != nil {
			if err := lf.Close(); err != nil {
				slog.Warn("failed to close run log", "err", err.Error())
			}
		}
		return fmt.Errorf("failed to start explo: %w", err)
	}

	// Close write end in parent so reader gets EOF when child exits.
	if err := pw.Close(); err != nil {
		slog.Warn("failed to close file writer", "err", err.Error())
	}

	go mr.collectRunOutput(cmd, pr, lf)
	return nil
}

func (mr *ManualRun) currentRunStatus() RunStatus {
	return mr.CurrentStatus()
}

// CurrentStatus reports the live run state for diagnostics consumers.
func (mr *ManualRun) CurrentStatus() RunStatus {
	mr.state.mu.Lock()
	defer mr.state.mu.Unlock()

	var exitCode *int
	if mr.state.exitCode != nil {
		code := *mr.state.exitCode
		exitCode = &code
	}
	return RunStatus{Running: mr.state.running, ExitCode: exitCode}
}

func (mr *ManualRun) finishRun(code int) {
	done := runEvent{typ: "done", data: fmt.Sprintf("%d", code)}

	mr.state.mu.Lock()
	mr.state.running = false
	mr.state.cancel = nil
	mr.state.exitCode = &code
	record := RunRecord{
		StartedAt: mr.state.runStarted,
		EndedAt:   time.Now(),
		Playlist:  mr.state.runPlaylist,
		ExitCode:  code,
		Errors:    firstErrorLines(mr.state.logs),
	}
	mr.state.runPlaylist = ""
	subscribers := make([]chan runEvent, 0, len(mr.state.subscribers))
	for ch := range mr.state.subscribers {
		subscribers = append(subscribers, ch)
		delete(mr.state.subscribers, ch)
	}
	mr.state.mu.Unlock()

	if err := mr.appendRunRecord(record); err != nil {
		slog.Warn("failed to record finished run", "err", err.Error())
	}

	for _, ch := range subscribers {
		select {
		case ch <- done:
		default:
		}
		close(ch)
	}
}

// playlistArg pulls the --playlist value out of child args for records.
func playlistArg(args []string) string {
	for i, a := range args {
		if a == "--playlist" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// firstErrorLines keeps the first few error-level lines of a run.
func firstErrorLines(logs []string) []string {
	var out []string
	for _, line := range logs {
		if strings.Contains(strings.ToLower(line), "level=error") {
			out = append(out, line)
			if len(out) >= maxRecordErrors {
				break
			}
		}
	}
	return out
}

// runRecordsPath is the append-only diagnostics history of finished runs.
func (mr *ManualRun) runRecordsPath() string {
	return filepath.Join(mr.cfg.WebDataDir, "logs", "runs.jsonl")
}

// appendRunRecord stores one record, pruning the file to the newest
// maxRunRecords entries so the history cannot grow without bound.
func (mr *ManualRun) appendRunRecord(record RunRecord) error {
	if record.StartedAt.IsZero() {
		return nil
	}
	p := mr.runRecordsPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	lines := readRecordLines(p)
	lines = append(lines, string(raw)+"\n")
	if len(lines) > maxRunRecords {
		lines = lines[len(lines)-maxRunRecords:]
	}
	return writeFileAtomic(p, []byte(strings.Join(lines, "")))
}

// ReadRunRecords returns the stored history, newest last.
func (mr *ManualRun) ReadRunRecords() []RunRecord {
	var out []RunRecord
	for _, line := range readRecordLines(mr.runRecordsPath()) {
		var r RunRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}

func readRecordLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line+"\n")
		}
	}
	return lines
}

// writeFileAtomic replaces path via a temp file plus rename.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".runs-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// helper to build flag arguments
func buildArgs(playlist, downloadMode, WebEnvPath string, replacePlaylist bool, user string) []string {
	args := []string{"--config", WebEnvPath}
	if playlist != "" {
		args = append(args, "--playlist", playlist)
	}
	if downloadMode != "" {
		args = append(args, "--download-mode", downloadMode)
	}
	if !replacePlaylist {
		args = append(args, "--replace-playlist=false")
	}
	if user != "" {
		args = append(args, "--user", user)
	}
	return args
}
// runContext bounds a run, or leaves it unbounded when the timeout is zero.
func runContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), timeout)
}

// newRunCommand builds the explo child for a manual run. Cancellation (Stop or
// the run timeout) kills the whole process tree where supported.
func newRunCommand(ctx context.Context, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	// Bound how long Wait blocks on pipes held by stray descendants after kill.
	cmd.WaitDelay = 5 * time.Second
	setProcessGroup(cmd)
	return cmd
}
