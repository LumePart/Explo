package run

import (
	"time"
	"fmt"
	"errors"
	"log/slog"
	"os"
	"os/exec"
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
}

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
	cmd := exec.CommandContext(ctx, mr.cfg.ExploPath, args...)
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
	subscribers := make([]chan runEvent, 0, len(mr.state.subscribers))
	for ch := range mr.state.subscribers {
		subscribers = append(subscribers, ch)
		delete(mr.state.subscribers, ch)
	}
	mr.state.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- done:
		default:
		}
		close(ch)
	}
}

// helper to build flag arguments
func buildArgs(playlist, downloadMode, WebEnvPath string, replacePlaylist bool) []string {
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
	return args
}
// runContext bounds a run, or leaves it unbounded when the timeout is zero.
func runContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), timeout)
}
