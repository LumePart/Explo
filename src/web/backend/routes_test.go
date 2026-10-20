package backend

import (
	"net/http"
	"testing"

	"explo/src/config"
)

// BUG-28: the pattern lacked its leading slash, so ServeMux parsed
// "api" as a host pattern and requests fell through to the SPA
// fallback instead of the presets handler.
func TestRoute_PathTemplatesRegistered(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, err := http.NewRequest(method, "/api/ui/path-templates", nil)
		if err != nil {
			t.Fatal(err)
		}
		s := NewServer(config.ServerConfig{})
		_, pattern := s.mux.Handler(req)
		if pattern != "/api/ui/path-templates" {
			t.Errorf("%s matched pattern %q, want /api/ui/path-templates", method, pattern)
		}
	}
}
