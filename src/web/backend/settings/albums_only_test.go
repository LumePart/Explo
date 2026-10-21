package settings

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// The toggle round-trips through the .env file: on writes true, off writes
// false, and anything else is rejected before anything is written.
func TestHandleSaveAlbumsOnly(t *testing.T) {
	s, envPath := newSettings(t)

	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/ui/config/albums-only", bytes.NewReader([]byte(body)))
		rec := httptest.NewRecorder()
		s.HandleSaveAlbumsOnly(rec, req)
		return rec
	}

	if rec := post(`{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want 200", rec.Code)
	}
	written, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading written env: %v", err)
	}
	if !bytes.Contains(written, []byte("ALBUMS_ONLY=true")) {
		t.Errorf("written env lacks ALBUMS_ONLY=true:\n%s", written)
	}

	if rec := post(`{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want 200", rec.Code)
	}
	written, err = os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading written env: %v", err)
	}
	if !bytes.Contains(written, []byte("ALBUMS_ONLY=false")) {
		t.Errorf("written env lacks ALBUMS_ONLY=false:\n%s", written)
	}

	if rec := post(`{bad json`); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed status = %d, want 400", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/ui/config/albums-only", nil)
	rec := httptest.NewRecorder()
	s.HandleSaveAlbumsOnly(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", rec.Code)
	}
}
