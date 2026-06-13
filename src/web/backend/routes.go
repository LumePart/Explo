package backend

import (
	"log/slog"
	"net/http"
	"strings"
	"io/fs"
	"path/filepath"
)

func (s *Server) registerRoutes() {
	distFS, indexHTML := spaFS()
	fileServer := http.FileServer(http.FS(distFS))

	// SPA fallback: serve static assets when they exist, otherwise serve index.html.
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := fs.Stat(distFS, path); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(indexHTML); err != nil {
			slog.Error("failed writing to http", "msg", err.Error())
		}
	})

	s.registerAuthRoutes()
	s.registerConfigRoutes()
	s.registerWizardRoutes()
	s.registerPlaylistRoutes()
	s.registerRunRoutes()
	s.registerMiscRoutes()

}

func (s *Server) registerAuthRoutes() {
	s.mux.Handle("POST /api/ui/logout", s.auth(s.handleLogout))

	// Public routes
	s.mux.HandleFunc("GET /api/ui/csrf", s.csrfHandler)
	s.mux.HandleFunc("POST /api/ui/login", s.handleLogin)
	s.mux.HandleFunc("GET /api/ui/auth/status", s.handleAuthStatus)
}

func (s *Server) registerConfigRoutes() {
	s.mux.Handle("GET /api/ui/config", s.auth(s.handleGetConfig))
	s.mux.Handle("POST /api/ui/config", s.auth(s.handleSaveConfig))

	s.mux.Handle("GET /api/ui/config/raw", s.auth(s.handleGetConfigRaw))
	s.mux.Handle("POST /api/ui/config/reset", s.auth(s.handleResetConfig))
	s.mux.Handle("POST /api/ui/config/schedules", s.auth(s.handleSaveSchedule))
	s.mux.Handle("POST /api/ui/config/path-template", s.auth(s.handleSavePathTemplate))
	s.mux.Handle("POST /api/ui/config/enrich-metadata", s.auth(s.handleSaveEnrichMetadata))

	// Path template presets: GET list, POST add; DELETE per name under prefix
	s.mux.Handle("api/ui/path-templates", s.auth(s.handlePathTemplates))
	s.mux.Handle("DELETE /api/ui/path-templates/", s.auth(s.handleDeletePathTemplate))

}

func (s *Server) registerWizardRoutes() {
	// Wizard steps (POST) — require auth
	s.mux.Handle("POST /api/ui/wizard/step1", s.auth(s.handleWizardStep1))
	s.mux.Handle("POST /api/ui/wizard/step2", s.auth(s.handleWizardStep2))
	s.mux.Handle("POST /api/ui/wizard/step3", s.auth(s.handleWizardStep3))

	// Public
	s.mux.HandleFunc("GET /api/ui/setup-status", s.handleSetupStatus)
}

func (s *Server) registerPlaylistRoutes() {
	s.mux.Handle("GET /api/ui/playlists", s.auth(s.handleGetPlaylist))
	s.mux.Handle("POST /api/ui/playlists/prefetch", s.auth(s.handlePrefetchCovers))

	// custom playlists: GET list, POST import (same path); per-ID actions under prefix
	s.mux.Handle("GET /api/ui/custom-playlists", s.auth(s.handleGetCustomPlaylists))
	s.mux.Handle("POST /api/ui/custom-playlists", s.auth(s.handleImportCustomPlaylist))

	// ID-specific routes: DELETE /api/ui/custom-playlists/{id} and POST .../{id}/refresh
	s.mux.Handle("POST /api/ui/custom-playlists/{id}/refresh", s.auth(s.handleRefreshCustomPlaylist))
	s.mux.Handle("DELETE /api/ui/custom-playlists/{id}", s.auth(s.handleDeleteCustomPlaylist))
}

func (s *Server) registerRunRoutes() {
	s.mux.Handle("POST /api/ui/run", s.auth(s.handleRun))
	s.mux.Handle("GET /api/ui/run/events", s.auth(s.handleRunEvents))
	s.mux.Handle("POST /api/ui/run/stop", s.auth(s.handleStopRun))
	s.mux.Handle("GET /api/ui/run/status", s.auth(s.handleRunStatus))
}

func (s *Server) registerMiscRoutes() {
	s.mux.Handle("GET /api/ui/logs", s.auth(s.handleGetLog))
	s.mux.Handle("GET /api/ui/browse", s.auth(s.handleBrowse))
	s.mux.HandleFunc("GET /api/ui/background-art", s.handleBackgroundArt)

	coversDir := filepath.Join(s.cfg.WebDataDir, "cache", "covers")
	s.mux.Handle("GET /api/covers/", http.StripPrefix("/api/covers/", http.FileServer(http.Dir(coversDir))))
}

// small helper func for auth routing
func (s *Server) auth(h http.HandlerFunc) http.Handler {
	return s.authStore.RequireAuth(h)
}