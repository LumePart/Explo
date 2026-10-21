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
	s.registerSettingRoutes()
	s.registerPlaylistRoutes()
	s.registerRunRoutes()
	s.registerDiagnosticsRoutes()
	s.registerMiscRoutes()

}

func (s *Server) registerAuthRoutes() {
	s.mux.Handle("POST /api/ui/logout", s.auth(s.authStore.HandleLogout))

	// Public routes
	s.mux.HandleFunc("GET /api/ui/csrf", s.authStore.HandleCSRF)
	s.mux.HandleFunc("POST /api/ui/login", s.authStore.HandleLogin)
	s.mux.HandleFunc("GET /api/ui/auth/status", s.authStore.HandleAuthStatus)
}

func (s *Server) registerSettingRoutes() {
	s.mux.Handle("GET /api/ui/config", s.auth(s.settings.HandleGetConfig))
	s.mux.Handle("POST /api/ui/config", s.auth(s.settings.HandleSaveConfig))

	s.mux.Handle("GET /api/ui/config/raw", s.auth(s.settings.HandleGetConfigRaw))
	s.mux.Handle("POST /api/ui/config/reset", s.auth(s.settings.HandleResetConfig))
	s.mux.Handle("POST /api/ui/config/schedules", s.auth(s.settings.HandleSaveSchedule))
	s.mux.Handle("POST /api/ui/config/path-template", s.auth(s.settings.HandleSavePathTemplate))
	s.mux.Handle("POST /api/ui/config/enrich-metadata", s.auth(s.settings.HandleSaveEnrichMetadata))
	s.mux.Handle("POST /api/ui/config/albums-only", s.auth(s.settings.HandleSaveAlbumsOnly))
	s.mux.Handle("POST /api/ui/config/replace-playlist", s.auth(s.settings.HandleSaveReplacePlaylist))
	s.mux.Handle("POST /api/ui/config/local-only", s.auth(s.settings.HandleSaveLocalOnly))
	s.mux.Handle("POST /api/ui/config/clean-downloads", s.auth(s.settings.HandleSaveCleanDownloads))

	// Path template presets: GET list, POST add; DELETE per name under prefix.
	// The pattern needs its leading slash: without it ServeMux parses
	// "api" as a host pattern and normal requests never reach the
	// handler, so the settings screen could never load presets.
	s.mux.Handle("/api/ui/path-templates", s.auth(s.settings.HandlePathTemplates))
	s.mux.Handle("DELETE /api/ui/path-templates/", s.auth(s.settings.HandleDeletePathTemplate))

	// Wizard steps (POST) — require auth
	s.mux.Handle("POST /api/ui/wizard/step1", s.auth(s.settings.HandleWizardStep1))
	s.mux.Handle("POST /api/ui/wizard/step2", s.auth(s.settings.HandleWizardStep2))
	s.mux.Handle("POST /api/ui/wizard/step3", s.auth(s.settings.HandleWizardStep3))

	// Public
	s.mux.HandleFunc("GET /api/ui/setup-status", s.settings.HandleSetupStatus)

}

func (s *Server) registerPlaylistRoutes() {
	s.mux.Handle("GET /api/ui/playlists", s.auth(s.customPlaylist.HandleGetPlaylist))
	s.mux.Handle("POST /api/ui/playlists/prefetch", s.auth(s.customPlaylist.HandlePrefetchCovers))

	// custom playlists: GET list, POST import (same path); per-ID actions under prefix
	s.mux.Handle("GET /api/ui/custom-playlists", s.auth(s.customPlaylist.HandleGetCustomPlaylists))
	s.mux.Handle("POST /api/ui/custom-playlists", s.auth(s.customPlaylist.HandleImportCustomPlaylist))

	// ID-specific routes: DELETE /api/ui/custom-playlists/{id} and POST .../{id}/refresh
	s.mux.Handle("POST /api/ui/custom-playlists/{id}/refresh", s.auth(s.customPlaylist.HandleRefreshCustomPlaylist))
	s.mux.Handle("DELETE /api/ui/custom-playlists/{id}", s.auth(s.customPlaylist.HandleDeleteCustomPlaylist))

	s.mux.HandleFunc("GET /api/ui/background-art", s.customPlaylist.HandleBackgroundArt)
}

func (s *Server) registerRunRoutes() {
	s.mux.Handle("POST /api/ui/run", s.auth(s.manualRun.HandleRun))
	s.mux.Handle("GET /api/ui/run/events", s.auth(s.manualRun.HandleRunEvents))
	s.mux.Handle("POST /api/ui/run/stop", s.auth(s.manualRun.HandleStopRun))
	s.mux.Handle("GET /api/ui/run/status", s.auth(s.manualRun.HandleRunStatus))
}

func (s *Server) registerDiagnosticsRoutes() {
	s.mux.Handle("GET /api/ui/diagnostics/status", s.diagnosticsAuth(s.handleDiagnosticsStatus))
	s.mux.Handle("GET /api/ui/diagnostics/logs", s.diagnosticsAuth(s.handleDiagnosticsLogs))
	s.mux.Handle("GET /api/ui/diagnostics/runs", s.diagnosticsAuth(s.handleDiagnosticsRuns))
	// Token management stays session-only: a token must never mint or
	// kill its own replacement.
	s.mux.Handle("POST /api/ui/diagnostics/token", s.auth(s.handleDiagnosticsTokenCreate))
	s.mux.Handle("DELETE /api/ui/diagnostics/token", s.auth(s.handleDiagnosticsTokenRevoke))
}

func (s *Server) registerMiscRoutes() {
	s.mux.Handle("GET /api/ui/logs", s.auth(s.handleGetLog))
	s.mux.Handle("GET /api/ui/browse", s.auth(s.handleBrowse))

	coversDir := filepath.Join(s.cfg.WebDataDir, "cache", "covers")
	s.mux.Handle("GET /api/covers/", http.StripPrefix("/api/covers/", http.FileServer(http.Dir(coversDir))))
}

// small helper func for auth routing
func (s *Server) auth(h http.HandlerFunc) http.Handler {
	return s.authStore.RequireAuth(h)
}