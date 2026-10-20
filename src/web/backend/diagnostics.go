package backend

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"explo/src/config"
	"explo/src/logging"
)

// maxLogTail caps lines per logs request; maxLogLineBytes caps memory per
// line served.
const (
	defaultLogTail = 200
	maxLogTail     = 2000
)

// diagnosticsAuth accepts a valid read-only API token or falls back to
// the UI session. Token management endpoints must NOT use this: they
// stay session-only so a token can never mint its own replacement.
func (s *Server) diagnosticsAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token, ok := bearerToken(r); ok {
			if s.apiTokens != nil && s.apiTokens.Verify(token) {
				next.ServeHTTP(w, r)
				return
			}
			slog.Warn("diagnostics: rejected request with invalid token")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.auth(next).ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	token, found := strings.CutPrefix(header, "Bearer ")
	if !found || token == "" {
		return "", false
	}
	return token, true
}

type diagnosticsStatus struct {
	Version    string         `json:"version"`
	StartedAt  time.Time      `json:"started_at"`
	UptimeS    int64          `json:"uptime_s"`
	Run        any            `json:"run"`
	RecentRuns []runSummary   `json:"recent_runs"`
}

type runSummary struct {
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Playlist  string    `json:"playlist,omitempty"`
	ExitCode  int       `json:"exit_code"`
	Errors    []string  `json:"errors,omitempty"`
}

// handleDiagnosticsStatus is the one-glance health snapshot: run state
// plus recent history, with no secret values anywhere in it.
func (s *Server) handleDiagnosticsStatus(w http.ResponseWriter, r *http.Request) {
	records := s.manualRun.ReadRunRecords()
	recent := make([]runSummary, 0, 5)
	for i := len(records) - 1; i >= 0 && len(recent) < 5; i-- {
		rec := records[i]
		recent = append([]runSummary{{
			StartedAt: rec.StartedAt,
			EndedAt:   rec.EndedAt,
			Playlist:  rec.Playlist,
			ExitCode:  rec.ExitCode,
			Errors:    rec.Errors,
		}}, recent...)
	}

	writeDiagnosticsJSON(w, diagnosticsStatus{
		Version:    config.Version,
		StartedAt:  s.startedAt,
		UptimeS:    int64(time.Since(s.startedAt).Seconds()),
		Run:        s.manualRun.CurrentStatus(),
		RecentRuns: recent,
	})
}

// handleDiagnosticsRuns returns the full finished-run history plus the
// live Manual Run state.
func (s *Server) handleDiagnosticsRuns(w http.ResponseWriter, r *http.Request) {
	writeDiagnosticsJSON(w, map[string]any{
		"live":    s.manualRun.CurrentStatus(),
		"history": s.manualRun.ReadRunRecords(),
	})
}

// handleDiagnosticsLogs serves tail-filtered lines from the rolling log
// without loading the whole file: ?tail=200&level=error&service=slskd&since=1h.
// Lines are redacted again on the way out: child-process output reaches
// the file without passing the server-side writer redactor.
func (s *Server) handleDiagnosticsLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	tail := defaultLogTail
	if raw := q.Get("tail"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			http.Error(w, "invalid tail", http.StatusBadRequest)
			return
		}
		tail = min(n, maxLogTail)
	}

	var since time.Time
	if raw := q.Get("since"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 {
			http.Error(w, "invalid since", http.StatusBadRequest)
			return
		}
		since = time.Now().Add(-d)
	}

	level := strings.ToLower(q.Get("level"))
	service := strings.ToLower(q.Get("service"))

	lines, err := tailLogFile(s.manualRun.LogPath(), tail, func(line string) bool {
		lower := strings.ToLower(line)
		switch level {
		case "", "all":
		case "error":
			if !strings.Contains(lower, "level=error") {
				return false
			}
		case "warn", "warning":
			if !strings.Contains(lower, "level=warn") && !strings.Contains(lower, "level=error") {
				return false
			}
		case "info":
			if !strings.Contains(lower, "level=info") && !strings.Contains(lower, "level=warn") && !strings.Contains(lower, "level=error") {
				return false
			}
		default:
			return false
		}
		if service != "" && !strings.Contains(lower, "service="+service) {
			return false
		}
		if !since.IsZero() && !logLineAfter(line, since) {
			return false
		}
		return true
	})
	if err != nil {
		http.Error(w, "failed to read log", http.StatusInternalServerError)
		return
	}

	for i, line := range lines {
		lines[i] = logging.RedactLine(line)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"lines": lines}); err != nil {
		slog.Error("failed encoding log lines", "msg", err.Error())
	}
}

// tailLogFile returns up to the last maxLines of path, oldest first,
// keeping only lines accepted by keep. It reads from the end in chunks
// so a large log file is never loaded whole.
func tailLogFile(path string, maxLines int, keep func(string) bool) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	const chunk = 64 * 1024
	var data []byte
	lineCount := 0
	for offset := info.Size(); offset > 0 && lineCount <= maxLines; {
		size := int64(chunk)
		if offset < size {
			size = offset
		}
		offset -= size
		buf := make([]byte, size)
		if _, err := f.ReadAt(buf, offset); err != nil {
			return nil, err
		}
		data = append(buf, data...)
		lineCount = 0
		for _, b := range data {
			if b == '\n' {
				lineCount++
			}
		}
	}

	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		if keep(line) {
			out = append(out, line)
		}
	}
	if len(out) > maxLines {
		out = out[len(out)-maxLines:]
	}
	return out, nil
}

// logLineAfter reports whether a slog text line is at or after t. Lines
// without a parseable time= field (raw child-process output) are kept:
// their age is unknown and dropping them would hide failures.
func logLineAfter(line string, t time.Time) bool {
	idx := strings.Index(line, "time=")
	if idx < 0 {
		return true
	}
	rest := line[idx+len("time="):]
	end := strings.IndexByte(rest, ' ')
	if end >= 0 {
		rest = rest[:end]
	}
	ts, err := time.Parse(time.RFC3339, strings.Trim(rest, `"`))
	if err != nil {
		return true
	}
	return !ts.Before(t)
}

// handleDiagnosticsTokenCreate mints a fresh read-only token, revoking
// any prior one. Session-only. The full token is returned exactly once.
func (s *Server) handleDiagnosticsTokenCreate(w http.ResponseWriter, r *http.Request) {
	token, err := s.apiTokens.Generate()
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}
	slog.Warn("diagnostics: API token generated; prior token revoked")
	writeDiagnosticsJSON(w, map[string]any{"token": token})
}

// handleDiagnosticsTokenRevoke invalidates the current token. Session-only.
func (s *Server) handleDiagnosticsTokenRevoke(w http.ResponseWriter, r *http.Request) {
	if err := s.apiTokens.Revoke(); err != nil {
		http.Error(w, "failed to revoke token", http.StatusInternalServerError)
		return
	}
	slog.Warn("diagnostics: API token revoked")
	w.WriteHeader(http.StatusNoContent)
}

func writeDiagnosticsJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed encoding diagnostics response", "msg", err.Error())
	}
}
