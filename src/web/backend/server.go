package backend

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"explo/src/config"
	"explo/src/web"
)

// Option is a value/label pair for select-type fields.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Condition expresses a dependency on another field's value.
// All non-zero properties are ANDed together.
type Condition struct {
	Field    string   `json:"field"`
	Eq       string   `json:"eq,omitempty"`       // field === value
	In       []string `json:"in,omitempty"`       // field is one of values
	Contains string   `json:"contains,omitempty"` // value appears in field's comma-separated list
}

// ConfigResponse is returned by GET /api/config.
type ConfigResponse struct {
	Values  map[string]string `json:"values"`
	Sources map[string]string `json:"sources"` // "env" | "file"
}

// runEvent is an SSE event sent to connected browser clients.
type runEvent struct {
	typ  string
	data string
}

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

func newManualRunState() manualRunState {
	return manualRunState{subscribers: make(map[chan runEvent]struct{})}
}

type Server struct {
	cfg			   config.ServerConfig
	mux            *http.ServeMux
	server         *http.Server
	authStore      *AuthStore
	cronJobs	   *Jobs
	sessionManager *SessionManager
	manualRun      manualRunState
}

func NewServer(cfg config.ServerConfig) *Server {
	sessionManager := NewSessionManager(
		NewInMemorySessionStore(),
		1*time.Hour,
		7*(24*time.Hour),
		"session",
	)

	authStore := NewAuthStore(
	cfg.Username,
	cfg.Password,
	sessionManager,
)

	cronJobs := NewJobs()

	mux := http.NewServeMux()
	s := &Server{
		cfg: cfg,
		mux:        mux,
		server: &http.Server{
			Addr:    cfg.Port,
			Handler: sessionManager.Handle(mux),
		},
		authStore: authStore,
		cronJobs: cronJobs,
		sessionManager: sessionManager,
		manualRun: newManualRunState(),
	}

	s.registerRoutes()
	return s
}

func (s *Server) Start() error {
	s.initServerLog()
	s.startJobs()
	coversDir := filepath.Join(s.cfg.WebDataDir, "cache", "covers")
	if _, err := os.Stat(coversDir); os.IsNotExist(err) {
		s.PrefetchCovers()
	}
	slog.Info("Explo web UI started", "addr", s.server.Addr)
	go checkForUpdate()
	return s.server.ListenAndServe()
}

func checkForUpdate() {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/LumePart/Explo/releases/latest")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return
	}
	l := parseVer(release.TagName)
	c := parseVer(config.Version)
	newer := false
	for i := range 3 {
		if l[i] > c[i] { newer = true; break }
		if l[i] < c[i] { break }
	}
	if newer {
		slog.Info("new version available!", "latest", release.TagName, "current", config.Version)
	}
}

func parseVer(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		fmt.Sscanf(p, "%d", &out[i])
	}
	return out
}

// Jobs to register on startup
func (s *Server) startJobs() {

	coversDir := filepath.Join(s.cfg.WebDataDir, "cache", "covers")
	if err := s.cronJobs.RegisterCoverCleanup(
		"0 3 * * *", coversDir, s.cfg.CacheSizeMB<<20); err != nil {
			slog.Warn("failed to register cover cleanup job", "err", err.Error())
		}


	s.cronJobs.Start()
}

func(s *Server) PrefetchCovers() {

	coversDir := filepath.Join(s.cfg.WebDataDir, "cache", "covers")

	url := randomLocalCoverHiRes(coversDir)
	if url == "" {
		fetchSitewideCovers(coversDir)
	}
}

// spaFS returns the filesystem to serve the frontend from.
// When WEB_DEV=true, serves directly from src/web/dist on disk so that
// running "npm run build" reflects changes without recompiling the binary.
func spaFS() (fs.FS, []byte) {
	if os.Getenv("WEB_DEV") == "true" {
		diskFS := os.DirFS("src/web/dist")
		index, _ := fs.ReadFile(diskFS, "index.html")
		return diskFS, index
	}
	embedded, _ := fs.Sub(web.DistFiles, "dist")
	index, _ := fs.ReadFile(embedded, "index.html")
	return embedded, index
}

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
	s.mux.Handle("GET /api/ui/config", s.authStore.RequireAuth(http.HandlerFunc(s.handleGetConfig)))
	s.mux.Handle("GET /api/ui/config/raw", s.authStore.RequireAuth(http.HandlerFunc(s.handleGetConfigRaw)))
	s.mux.Handle("POST /api/ui/config", s.authStore.RequireAuth(http.HandlerFunc(s.handleSaveConfig)))
	s.mux.Handle("POST /api/ui/config/reset", s.authStore.RequireAuth(http.HandlerFunc(s.handleResetConfig)))
	s.mux.Handle("POST /api/ui/config/schedules", s.authStore.RequireAuth(http.HandlerFunc(s.handleSaveSchedule)))
	s.mux.Handle("POST /api/ui/wizard/step1", s.authStore.RequireAuth(http.HandlerFunc(s.handleWizardStep1)))
	s.mux.Handle("POST /api/ui/wizard/step2", s.authStore.RequireAuth(http.HandlerFunc(s.handleWizardStep2)))
	s.mux.Handle("POST /api/ui/wizard/step3", s.authStore.RequireAuth(http.HandlerFunc(s.handleWizardStep3)))
	s.mux.Handle("GET /api/ui/browse", s.authStore.RequireAuth(http.HandlerFunc(s.handleBrowse)))
	s.mux.Handle("POST /api/ui/run", s.authStore.RequireAuth(http.HandlerFunc(s.handleRun)))
	s.mux.Handle("GET /api/ui/run/events", s.authStore.RequireAuth(http.HandlerFunc(s.handleRunEvents)))
	s.mux.Handle("POST /api/ui/run/stop", s.authStore.RequireAuth(http.HandlerFunc(s.handleStopRun)))
	s.mux.Handle("GET /api/ui/run/status", s.authStore.RequireAuth(http.HandlerFunc(s.handleRunStatus)))
	s.mux.Handle("GET /api/ui/logs", s.authStore.RequireAuth(http.HandlerFunc(s.handleGetLog)))
	s.mux.Handle("GET /api/ui/playlists", s.authStore.RequireAuth(http.HandlerFunc(s.handleGetPlaylist)))
	s.mux.Handle("POST /api/ui/playlists/prefetch", s.authStore.RequireAuth(http.HandlerFunc(s.handlePrefetchCovers)))
	s.mux.Handle("POST /api/ui/logout", s.authStore.RequireAuth(http.HandlerFunc(s.handleLogout)))
	s.mux.HandleFunc("GET /api/ui/csrf", s.csrfHandler)
	s.mux.HandleFunc("POST /api/ui/login", s.handleLogin)
	s.mux.HandleFunc("GET /api/ui/auth/status", s.handleAuthStatus)
	s.mux.HandleFunc("GET /api/ui/background-art", s.handleBackgroundArt)
	s.mux.HandleFunc("GET /api/ui/setup-status", s.handleSetupStatus)

	coversDir := filepath.Join(s.cfg.WebDataDir, "cache", "covers")
	s.mux.Handle("/api/covers/", http.StripPrefix("/api/covers/", http.FileServer(http.Dir(coversDir))))
}

// ── Logging ────────────────────────────────────────────────────────────────

// logPath returns the path to the single rolling log file.
func (s *Server) logPath() string {
	return filepath.Join(s.cfg.WebDataDir, "logs", "explo.log")
}

// initServerLog redirects the default slog handler so all server log output
// goes to both stderr and the rolling log file.
func (s *Server) initServerLog() {
	lf, err := s.openRunLog()
	if err != nil {
		return
	}
	w := io.MultiWriter(os.Stderr, lf)
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))
}

// openRunLog opens the single rolling log file in append mode.
func (s *Server) openRunLog() (*os.File, error) {
	p := s.logPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return nil, err
	}
	return os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
}

// handleSetupStatus returns {"wizard_complete": bool} for first time setups. Public — no auth required.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	wizardComplete := false
	if data, err := os.ReadFile(s.cfg.WebEnvPath); err == nil {
		wizardComplete = parseEnvText(string(data))["WIZARD_COMPLETE"] == "true"
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]bool{"wizard_complete": wizardComplete}); err != nil {
		slog.Error("failed encoding setup status", "err", err.Error())
	}
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionManager.GetSession(r)
	auth, _ := sess.Get("authenticated").(bool)
	if !auth {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		err := http.StatusMethodNotAllowed
		http.Error(w, "Invalid request method", err)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if !s.authStore.CompareCreds(username, password) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	sess := s.sessionManager.GetSession(r)
	sess.Put("authenticated", true)
	sess.Put("username", username)
	//s.sessionManager.Migrate(sess)
	slog.Info("successful login", "user", username)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionManager.GetSession(r)
	sess.Delete("authenticated")
	sess.Delete("username")
	w.WriteHeader(http.StatusOK)
}

// handleGetLog returns the contents of the rolling log file.
func (s *Server) handleGetLog(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.logPath())
	if err != nil && !os.IsNotExist(err) {
		http.Error(w, "failed to read log", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := w.Write(data); err != nil {
		slog.Error("failed writing http response", "msg", err.Error())
	}
}

func (s *Server) csrfHandler(w http.ResponseWriter, r *http.Request) {
	session := s.sessionManager.GetSession(r)

	token, _ := session.Get("csrf_token").(string)

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]string{
		"csrf_token": token,
	}); err != nil {
		slog.Error("failed encoding token to http", "msg", err.Error())
	}
}

// ── Config ─────────────────────────────────────────────────────────────────

// parseEnvText parses key=value lines, ignoring comments and blanks.
func parseEnvText(text string) map[string]string {
	out := map[string]string{}
	for line := range strings.SplitSeq(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		k, v, ok := strings.Cut(t, "=")
		if !ok {
			continue
		}
		if k = strings.TrimSpace(k); k != "" {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

// handleGetConfig returns resolved config as JSON: { values, sources }.
// File keys are checked first because cleanenv sets them as OS env vars on startup,
// so checking os.LookupEnv first would misclassify all file keys as "env".
// Only keys present in the OS environment but absent from the file are marked "env".
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.cfg.WebEnvPath)
	var fileValues map[string]string
	if err == nil {
		fileValues = parseEnvText(string(data))
	} else {
		fileValues = parseEnvText(string(web.SampleEnv))
	}

	values := make(map[string]string, len(allConfigKeys))
	sources := make(map[string]string, len(allConfigKeys))
	for _, key := range allConfigKeys {
		if v, ok := fileValues[key]; ok && v != "" {
			values[key] = v
			sources[key] = "file"
		} else if v, ok := os.LookupEnv(key); ok && v != "" {
			values[key] = v
			sources[key] = "env"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ConfigResponse{Values: values, Sources: sources}); err != nil {
		slog.Error("failed encoding config to http", "msg", err.Error())
	}
}

// handleGetConfigRaw returns the raw .env file contents as plain text.
func (s *Server) handleGetConfigRaw(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.cfg.WebEnvPath)
	if err != nil {
		data = web.SampleEnv
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := w.Write(data); err != nil {
		slog.Error("failed writing http response", "msg", err.Error())
	}
}

// handleSaveConfig writes the posted plain-text body directly to the .env file.
func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.WriteFile(s.cfg.WebEnvPath, data, 0600); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleResetConfig resets all settings and restarts the container.
func (s *Server) handleResetConfig(w http.ResponseWriter, r *http.Request) {
	if err := os.WriteFile(s.cfg.WebEnvPath, web.SampleEnv, 0600); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	go func() {
		time.Sleep(300 * time.Millisecond)
		if err := syscall.Kill(1, syscall.SIGTERM); err != nil {
			slog.Warn("failed to kill process", "msg", err.Error())
		}

	}()
}

// handleSaveSchedule updates a single playlist's schedule in the .env file.
func (s *Server) handleSaveSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Day     int    `json:"day"` // 0=Sun…6=Sat, -1=every day
		Hour    int    `json:"hour"`
		Minute  int    `json:"minute"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	def, ok := playlistDefs[body.Name]
	if !ok {
		http.Error(w, "unknown playlist name", http.StatusBadRequest)
		return
	}

	updates := map[string]string{}
	if body.Enabled {
		dom := "*"
		dow := "*"
		if body.Day == 100 {
			dom = "1"
		} else if body.Day >= 0 {
			dow = fmt.Sprintf("%d", body.Day)
		}
		updates[def.EnvPrefix+"_SCHEDULE"] = fmt.Sprintf("%d %d %s * %s", body.Minute, body.Hour, dom, dow)
		updates[def.EnvPrefix+"_FLAGS"] = def.DefaultFlags
	} else {
		updates[def.EnvPrefix+"_SCHEDULE"] = ""
		updates[def.EnvPrefix+"_FLAGS"] = ""
	}

	if err := updateEnvKeys(s.cfg.WebEnvPath, updates, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// updateEnvKeys reads the env file (falling back to fallback if missing), updates the
// given key=value pairs in-place preserving comments, and writes the result back.
func updateEnvKeys(path string, updates map[string]string, fallback []byte) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data = fallback
	} else if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	touched := make(map[string]bool)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if val, ok := updates[key]; ok {
			if val == "" {
				lines[i] = "" // remove by blanking
			} else {
				lines[i] = key + "=" + val
			}
			touched[key] = true
		}
	}

	// Append any keys that weren't already in the file
	for k, v := range updates {
		if !touched[k] && v != "" {
			lines = append(lines, k+"="+v)
		}
	}

	// Filter out consecutive blank lines left by removals
	out := make([]string, 0, len(lines))
	prevBlank := false
	for _, l := range lines {
		blank := strings.TrimSpace(l) == ""
		if blank && prevBlank {
			continue
		}
		out = append(out, l)
		prevBlank = blank
	}

	return os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0600)
}

// ── Wizard ─────────────────────────────────────────────────────────────────

// handleWizardStep1 saves discovery settings (username + enabled playlists with default schedules).
func (s *Server) handleWizardStep1(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User          string   `json:"user"`
		Playlists     []string `json:"playlists"`
		DiscoveryMode string   `json:"discovery_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.User == "" {
		http.Error(w, "user is required", http.StatusBadRequest)
		return
	}

	enabled := make(map[string]bool, len(body.Playlists))
	for _, p := range body.Playlists {
		enabled[p] = true
	}

	updates := map[string]string{
		"LISTENBRAINZ_USER":      body.User,
		"LISTENBRAINZ_DISCOVERY": body.DiscoveryMode,
	}
	for name, def := range playlistDefs {
		if enabled[name] {
			updates[def.EnvPrefix+"_SCHEDULE"] = def.DefaultSchedule
			updates[def.EnvPrefix+"_FLAGS"] = def.DefaultFlags
		} else {
			updates[def.EnvPrefix+"_SCHEDULE"] = ""
			updates[def.EnvPrefix+"_FLAGS"] = ""
		}
	}

	if err := updateEnvKeys(s.cfg.WebEnvPath, updates, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleWizardStep2 saves media system configuration.
func (s *Server) handleWizardStep2(w http.ResponseWriter, r *http.Request) {
	var body struct {
		System         string `json:"system"`
		URL            string `json:"url"`
		APIKey         string `json:"api_key"`
		LibraryName    string `json:"library_name"`
		Username       string `json:"username"`
		Password       string `json:"password"`
		PlaylistDir    string `json:"playlist_dir"`
		Sleep          string `json:"sleep"`
		PublicPlaylist bool   `json:"public_playlist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.System == "" {
		http.Error(w, "system is required", http.StatusBadRequest)
		return
	}

	publicPlaylist := ""
	if body.PublicPlaylist {
		publicPlaylist = "true"
	}
	updates := map[string]string{
		"EXPLO_SYSTEM":    body.System,
		"SYSTEM_URL":      body.URL,
		"API_KEY":         body.APIKey,
		"LIBRARY_NAME":    body.LibraryName,
		"SYSTEM_USERNAME": body.Username,
		"SYSTEM_PASSWORD": body.Password,
		"PLAYLIST_DIR":    body.PlaylistDir,
		"SLEEP":           body.Sleep,
		"PUBLIC_PLAYLIST": publicPlaylist,
	}

	if err := updateEnvKeys(s.cfg.WebEnvPath, updates, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleWizardStep3 saves downloader configuration.
func (s *Server) handleWizardStep3(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DownloadDir      string   `json:"download_dir"`
		UseSubdirectory  bool     `json:"use_subdirectory"`
		MigrateDownloads bool     `json:"migrate_downloads"`
		DownloadServices []string `json:"download_services"`
		YoutubeAPIKey    string   `json:"youtube_api_key"`
		TrackExtension   string   `json:"track_extension"` // yt-dlp
		FilterList       string   `json:"filter_list"`
		SlskdURL         string   `json:"slskd_url"`
		SlskdAPIKey      string   `json:"slskd_api_key"`
		Extensions       string   `json:"extensions"`      // slskd
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.DownloadServices) == 0 {
		http.Error(w, "at least one download service is required", http.StatusBadRequest)
		return
	}
	joined := strings.Join(body.DownloadServices, ",")

	useSubdir := "false"
	if body.UseSubdirectory {
		useSubdir = "true"
	}
	migrateDL := "false"
	if body.MigrateDownloads {
		migrateDL = "true"
	}
	updates := map[string]string{
		"DOWNLOAD_DIR":      body.DownloadDir,
		"USE_SUBDIRECTORY":  useSubdir,
		"MIGRATE_DOWNLOADS": migrateDL,
		"DOWNLOAD_SERVICES": joined,
		"YOUTUBE_API_KEY":   body.YoutubeAPIKey,
		"TRACK_EXTENSION":   body.TrackExtension, // yt-dlp
		"FILTER_LIST":       body.FilterList,
		"SLSKD_URL":         body.SlskdURL,
		"SLSKD_API_KEY":     body.SlskdAPIKey,
		"EXTENSIONS":        body.Extensions,        // slskd
		"WIZARD_COMPLETE":	 "true",
	}

	if err := updateEnvKeys(s.cfg.WebEnvPath, updates, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleBrowse returns subdirectories of the requested path for filesystem autocomplete.
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	path := filepath.Clean(r.URL.Query().Get("path"))
	if path == "" || path == "." {
		path = "/"
	}
	if !filepath.IsAbs(path) {
		http.Error(w, "path must be absolute", http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode([]string{}); err != nil {
			slog.Error("failed to encode empty slice", "msg", err.Error())
		}
		return
	}

	dirs := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, filepath.Join(path, e.Name()))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dirs); err != nil {
		slog.Warn("failed to encode directories to response", "err", err.Error())
	}
}

// ── Manual run ─────────────────────────────────────────────────────────────

var errRunAlreadyStarted = errors.New("run already in progress")

// handleRun starts an explo run in the background. Clients follow output via /api/ui/run/events.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "bad form data", http.StatusBadRequest)
		return
	}

	args := buildArgs(r.FormValue("playlist"), r.FormValue("download_mode"),
		r.FormValue("persist") == "false", r.FormValue("exclude_local") == "true",
		s.cfg.WebEnvPath)

	if err := s.startRun(args); err != nil {
		if errors.Is(err, errRunAlreadyStarted) {
			http.Error(w, "a run is already in progress", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(s.currentRunStatus()); err != nil {
		slog.Warn("failed to encode current run status", "msg", err.Error())
	}
}

func (s *Server) startRun(args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, s.cfg.ExploPath, args...)
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

	lf, err := s.openRunLog()
	if err != nil {
		slog.Warn("failed to open run log", "err", err.Error())
	}

	s.manualRun.mu.Lock()
	if s.manualRun.running {
		s.manualRun.mu.Unlock()
		cancel()
		if err := pr.Close(); err != nil {
			slog.Warn("failed to close file reader", "err", err.Error())
		}

		if err := pw.Close(); err != nil {
			slog.Warn("failed to close file writer", "err", err.Error())
		}
		if lf != nil {
			if err := pw.Close(); err != nil {
				slog.Warn("failed to close file writer", "err", err.Error())
			}
		}
		return errRunAlreadyStarted
	}
	s.manualRun.running = true
	s.manualRun.cancel = cancel
	s.manualRun.exitCode = nil
	s.manualRun.logs = nil
	s.manualRun.mu.Unlock()

	if err := cmd.Start(); err != nil {
		s.finishRun(1)
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

	go s.collectRunOutput(cmd, pr, lf)
	return nil
}

func (s *Server) collectRunOutput(cmd *exec.Cmd, pr *os.File, lf *os.File) {
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
		if lf != nil {
			if _, err := fmt.Fprintln(lf, line); err != nil {
				s.appendRunLog("failed to write run output: " + err.Error())
			}
		}
		s.appendRunLog(line)
	}
	if err := scanner.Err(); err != nil {
		s.appendRunLog("failed to read run output: " + err.Error())
	}

	code := 0
	if err := cmd.Wait(); err != nil && cmd.ProcessState == nil {
		code = 1
	}
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	s.finishRun(code)
}

func (s *Server) handleStopRun(w http.ResponseWriter, r *http.Request) {
	s.manualRun.mu.Lock()
	cancel := s.manualRun.cancel
	running := s.manualRun.running
	s.manualRun.mu.Unlock()

	if !running || cancel == nil {
		http.Error(w, "no run is currently in progress", http.StatusConflict)
		return
	}

	cancel()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) currentRunStatus() RunStatus {
	s.manualRun.mu.Lock()
	defer s.manualRun.mu.Unlock()

	var exitCode *int
	if s.manualRun.exitCode != nil {
		code := *s.manualRun.exitCode
		exitCode = &code
	}
	return RunStatus{Running: s.manualRun.running, ExitCode: exitCode}
}

func (s *Server) handleRunStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.currentRunStatus()); err != nil {
		slog.Warn("failed encoding current run status to response")
	}
}

// ── SSE event stream ───────────────────────────────────────────────────────

func (s *Server) appendRunLog(line string) {
	event := runEvent{data: line}

	s.manualRun.mu.Lock()
	s.manualRun.logs = append(s.manualRun.logs, line)
	subscribers := make([]chan runEvent, 0, len(s.manualRun.subscribers))
	for ch := range s.manualRun.subscribers {
		subscribers = append(subscribers, ch)
	}
	s.manualRun.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *Server) finishRun(code int) {
	done := runEvent{typ: "done", data: fmt.Sprintf("%d", code)}

	s.manualRun.mu.Lock()
	s.manualRun.running = false
	s.manualRun.cancel = nil
	s.manualRun.exitCode = &code
	subscribers := make([]chan runEvent, 0, len(s.manualRun.subscribers))
	for ch := range s.manualRun.subscribers {
		subscribers = append(subscribers, ch)
		delete(s.manualRun.subscribers, ch)
	}
	s.manualRun.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- done:
		default:
		}
		close(ch)
	}
}

// handleRunEvents streams the current in-memory run log, then follows new lines
// until the active run exits. Safe to reconnect after a browser refresh.
func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sendEvent := func(typ, data string) {
		if typ != "" {
			if _, err := fmt.Fprintf(w, "event: %s\n", typ); err != nil {
				slog.Warn("failed handling run event", "err", err.Error())
			}
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			slog.Warn("failed handling run event", "err", err.Error())
		}
		flusher.Flush()
	}

	ch := make(chan runEvent, 256)
	s.manualRun.mu.Lock()
	lines := append([]string(nil), s.manualRun.logs...)
	running := s.manualRun.running
	var exitCode *int
	if s.manualRun.exitCode != nil {
		code := *s.manualRun.exitCode
		exitCode = &code
	}
	if running {
		s.manualRun.subscribers[ch] = struct{}{}
	}
	s.manualRun.mu.Unlock()

	for _, line := range lines {
		sendEvent("", line)
	}
	if !running {
		if exitCode != nil {
			sendEvent("done", fmt.Sprintf("%d", *exitCode))
		}
		return
	}

	defer s.unsubscribeRun(ch)
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			sendEvent(ev.typ, ev.data)
			if ev.typ == "done" {
				return
			}
		}
	}
}

func (s *Server) unsubscribeRun(ch chan runEvent) {
	s.manualRun.mu.Lock()
	delete(s.manualRun.subscribers, ch)
	s.manualRun.mu.Unlock()
}

// ── Helpers ────────────────────────────────────────────────────────────────

func buildArgs(playlist, downloadMode string, noPersist, excludeLocal bool, WebEnvPath string) []string {
	args := []string{"--config", WebEnvPath}
	if playlist != "" {
		args = append(args, "--playlist", playlist)
	}
	if downloadMode != "" {
		args = append(args, "--download-mode", downloadMode)
	}
	if noPersist {
		args = append(args, "--persist=false")
	}
	if excludeLocal {
		args = append(args, "--exclude-local")
	}
	return args
}
