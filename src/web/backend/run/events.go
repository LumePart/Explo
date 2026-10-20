package run

import (
	"fmt"
	"log/slog"
	"os"
	"io"
	"path/filepath"
	"bufio"
	"os/exec"
	"strings"

	"explo/src/logging"
)


// maxRunLineBytes caps how much of one output line is kept. Lines beyond
// it are drained and truncated rather than stopping the reader.
const maxRunLineBytes = 1 << 20

// scanRunLines emits each line from r until EOF. Unlike bufio.Scanner
// with its 64KB limit, a single very long line cannot stop draining:
// before this, an over-long line wedged the child on a full pipe,
// cmd.Wait never returned, and every later run was refused as already
// running.
func scanRunLines(r io.Reader, emit func(string)) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	for {
		var buf []byte
		truncated := false
		for {
			frag, err := reader.ReadSlice('\n')
			if len(buf)+len(frag) <= maxRunLineBytes {
				buf = append(buf, frag...)
			} else {
				if len(buf) < maxRunLineBytes {
					buf = append(buf, frag[:maxRunLineBytes-len(buf)]...)
				}
				truncated = true
			}
			if err == bufio.ErrBufferFull {
				continue
			}
			if len(frag) > 0 || len(buf) > 0 {
				line := strings.TrimRight(string(buf), "\r\n")
				if truncated {
					line += "...[truncated]"
				}
				emit(line)
			}
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			break
		}
	}
}

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

	if err := scanRunLines(pr, func(line string) {
		// Echo to stdout so runs show up in docker logs.
		_, _ = fmt.Fprintln(os.Stdout, line)
		if lf != nil {
			if _, err := fmt.Fprintln(lf, line); err != nil {
				mr.appendRunLog("failed to write run output: " + err.Error())
			}
		}
		mr.appendRunLog(line)
	}); err != nil {
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
// goes to both stderr and the rolling log file. Both pass through the
// secret redactor: shared logs are a debugging artifact, not a vault.
func (mr *ManualRun) InitServerLog() {
	lf, err := mr.openRunLog()
	if err != nil {
		return
	}
	w := logging.NewRedactWriter(io.MultiWriter(os.Stderr, lf))
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))
}

// openRunLog opens the single rolling log file in append mode.
func (mr *ManualRun) openRunLog() (*os.File, error) {
	p := mr.LogPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return nil, err
	}
	return os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
}