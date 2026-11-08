package run

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"explo/src/logging"
)


func (mr *ManualRun) appendRunLog(line string) {
	event := runEvent{data: line}

	mr.state.mu.Lock()
	mr.state.logs = append(mr.state.logs, line)
	subscribers := make([]chan runEvent, 0, len(mr.state.subscribers))
	for ch := range mr.state.subscribers {
		subscribers = append(subscribers, ch)
	}
	mr.state.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (mr *ManualRun) collectRunOutput(cmd *exec.Cmd, pr *os.File, lf *os.File) {
	defer func() {
		if cerr := pr.Close(); cerr != nil {
			slog.Error("failed to close source file", "err", cerr.Error())
		}
	}()

	if lf != nil {
		defer func() {
			if cerr := lf.Close(); cerr != nil {
				slog.Error("failed to close source file", "err", cerr.Error())
			}
		}()
	}

	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		line := scanner.Text()
		// Echo to stdout so runs show up in docker logs.
		_, _ = fmt.Fprintln(os.Stdout, line)
		if lf != nil {
			if _, err := fmt.Fprintln(lf, line); err != nil {
				mr.appendRunLog("failed to write run output: " + err.Error())
			}
		}
		mr.appendRunLog(line)
	}
	if err := scanner.Err(); err != nil {
		mr.appendRunLog("failed to read run output: " + err.Error())
	}

	code := 0
	if err := cmd.Wait(); err != nil && cmd.ProcessState == nil {
		code = 1
	}
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	mr.finishRun(code)
}

func (mr *ManualRun) unsubscribeRun(ch chan runEvent) {
	mr.state.mu.Lock()
	delete(mr.state.subscribers, ch)
	mr.state.mu.Unlock()
}

// logPath returns the path to the single rolling log file.
func (mr *ManualRun) LogPath() string {
	return filepath.Join(mr.cfg.WebDataDir, "logs", "explo.log")
}

// initServerLog redirects the default slog handler so all server log output
// goes to both stderr and the rolling log file.
func (mr *ManualRun) InitServerLog() {
	lf, err := mr.openRunLog()
	if err != nil {
		return
	}
	w := io.MultiWriter(os.Stderr, lf)
	slog.SetDefault(slog.New(logging.NewTextHandler(w, nil)))
}

// openRunLog opens the single rolling log file in append mode.
func (mr *ManualRun) openRunLog() (*os.File, error) {
	p := mr.LogPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return nil, err
	}
	return os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
}
