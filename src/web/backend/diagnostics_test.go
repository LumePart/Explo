package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"explo/src/config"
)

func testDiagnosticsServer(t *testing.T) *Server {
	t.Helper()
	return NewServer(config.ServerConfig{
		Username:   "admin",
		Password:   "password123",
		WebDataDir: t.TempDir(),
	})
}

func doDiagnostics(t *testing.T, s *Server, method, target, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	// Serve through the full chain (session middleware + mux), as in
	// production: auth handlers require the session context.
	s.server.Handler.ServeHTTP(rec, req)
	return rec
}

// No token and no session must be refused: diagnostics are never public.
func TestDiagnostics_UnauthenticatedRefused(t *testing.T) {
	s := testDiagnosticsServer(t)
	for _, target := range []string{
		"/api/ui/diagnostics/status",
		"/api/ui/diagnostics/logs",
		"/api/ui/diagnostics/runs",
	} {
		rec := doDiagnostics(t, s, "GET", target, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without credentials = %d, want 401", target, rec.Code)
		}
	}
}

func TestDiagnostics_InvalidTokenRefused(t *testing.T) {
	s := testDiagnosticsServer(t)
	rec := doDiagnostics(t, s, "GET", "/api/ui/diagnostics/status", "explo_bogus")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("bad token = %d, want 401", rec.Code)
	}
}

func TestDiagnostics_StatusWithToken(t *testing.T) {
	s := testDiagnosticsServer(t)
	token, err := s.apiTokens.Generate()
	if err != nil {
		t.Fatal(err)
	}

	rec := doDiagnostics(t, s, "GET", "/api/ui/diagnostics/status", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("status body is not JSON: %v", err)
	}
	for _, key := range []string{"version", "started_at", "uptime_s", "run", "recent_runs"} {
		if _, ok := body[key]; !ok {
			t.Errorf("status body lacks %q: %v", key, body)
		}
	}
	// The snapshot must carry no secret-shaped values.
	lowered := strings.ToLower(rec.Body.String())
	for _, needle := range []string{"password123", "hunter2"} {
		if strings.Contains(lowered, needle) {
			t.Errorf("status body leaks %q", needle)
		}
	}
}

func TestDiagnostics_LogsFiltering(t *testing.T) {
	s := testDiagnosticsServer(t)
	token, err := s.apiTokens.Generate()
	if err != nil {
		t.Fatal(err)
	}

	logDir := filepath.Join(s.cfg.WebDataDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join([]string{
		`time=2026-10-20T10:00:00Z level=INFO msg="starting" service=slskd`,
		`time=2026-10-20T10:01:00Z level=ERROR msg="first failure" service=slskd api_key=hunter2`,
		`time=2026-10-20T10:02:00Z level=INFO msg="progress" service=lidarr`,
		`time=2026-10-20T10:03:00Z level=ERROR msg="second failure" service=lidarr`,
		``,
	}, "\n")
	if err := os.WriteFile(filepath.Join(logDir, "explo.log"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	decode := func(rec *httptest.ResponseRecorder) []string {
		t.Helper()
		var body struct {
			Lines []string `json:"lines"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("logs body is not JSON: %v\n%s", err, rec.Body.String())
		}
		return body.Lines
	}

	rec := doDiagnostics(t, s, "GET", "/api/ui/diagnostics/logs?level=error", token)
	lines := decode(rec)
	if len(lines) != 2 {
		t.Errorf("level=error lines = %d, want 2: %v", len(lines), lines)
	}
	for _, line := range lines {
		if strings.Contains(line, "hunter2") {
			t.Errorf("secret survived log serving: %q", line)
		}
	}

	rec = doDiagnostics(t, s, "GET", "/api/ui/diagnostics/logs?level=error&service=lidarr", token)
	if lines := decode(rec); len(lines) != 1 {
		t.Errorf("service=lidarr error lines = %d, want 1: %v", len(lines), lines)
	}

	rec = doDiagnostics(t, s, "GET", "/api/ui/diagnostics/logs?tail=1", token)
	if lines := decode(rec); len(lines) != 1 {
		t.Errorf("tail=1 lines = %d, want 1: %v", len(lines), lines)
	}

	rec = doDiagnostics(t, s, "GET", "/api/ui/diagnostics/logs?since=2m", token)
	_ = decode(rec)

	rec = doDiagnostics(t, s, "GET", "/api/ui/diagnostics/logs?tail=nope", token)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad tail = %d, want 400", rec.Code)
	}
}

func TestDiagnostics_RoutesRegistered(t *testing.T) {
	s := testDiagnosticsServer(t)
	for target, want := range map[string]string{
		"/api/ui/diagnostics/status": "/api/ui/diagnostics/status",
		"/api/ui/diagnostics/logs":   "/api/ui/diagnostics/logs",
		"/api/ui/diagnostics/runs":   "/api/ui/diagnostics/runs",
	} {
		req := httptest.NewRequest("GET", target, nil)
		_, pattern := s.mux.Handler(req)
		if pattern != "GET "+want {
			t.Errorf("GET %s matched %q, want %q", target, pattern, "GET "+want)
		}
	}
}
