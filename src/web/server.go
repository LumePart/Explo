package web

import (
	"bufio"
	"context"
	"embed"
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
)

//go:embed dist
var distFiles embed.FS

//go:embed sample.env
var sampleEnv []byte

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

// FieldDef describes a single configurable env var.
// Injected into the page as window.__FIELDS__ for the settings UI to consume.
type FieldDef struct {
	Key          string     `json:"key"`
	Label        string     `json:"label"`
	Type         string     `json:"type"`    // text | password | url | select
	Section      string     `json:"section"` // discovery | system | downloader
	Placeholder  string     `json:"placeholder,omitempty"`
	Hint         string     `json:"hint,omitempty"`
	Required     bool       `json:"required,omitempty"`
	Options      []Option   `json:"options,omitempty"`      // for type=select
	VisibleWhen  *Condition `json:"visibleWhen,omitempty"`  // hide field when condition is false
	RequiredWhen *Condition `json:"requiredWhen,omitempty"` // conditionally required
}

var netSystems = []string{"jellyfin", "emby", "plex", "subsonic"}
var apiKeySystems = []string{"jellyfin", "emby", "plex"}

// allConfigKeys is the complete set of env keys the web UI reads and writes.
var allConfigKeys = []string{
	"LISTENBRAINZ_USER", "LISTENBRAINZ_DISCOVERY",
	"WEEKLY_EXPLORATION_SCHEDULE", "WEEKLY_EXPLORATION_FLAGS",
	"WEEKLY_JAMS_SCHEDULE", "WEEKLY_JAMS_FLAGS",
	"DAILY_JAMS_SCHEDULE", "DAILY_JAMS_FLAGS",
	"EXPLO_SYSTEM", "SYSTEM_URL", "API_KEY", "LIBRARY_NAME",
	"SYSTEM_USERNAME", "SYSTEM_PASSWORD", "PLAYLIST_DIR", "SLEEP", "PUBLIC_PLAYLIST",
	"DOWNLOAD_DIR", "USE_SUBDIRECTORY",
	"DOWNLOAD_SERVICES", "YOUTUBE_API_KEY", "TRACK_EXTENSION", "FILTER_LIST",
	"SLSKD_URL", "SLSKD_API_KEY",
}

// ConfigResponse is returned by GET /api/config.
type ConfigResponse struct {
	Values  map[string]string `json:"values"`
	Sources map[string]string `json:"sources"` // "env" | "file"
}

// configFields is the single source of truth for the settings this web UI
// currently owns. VisibleWhen / RequiredWhen drive the settings UI; the wizard
// uses bespoke HTML but references the same logical rules.
var configFields = []FieldDef{
	// ── Discovery ──────────────────────────────────────────────────
	{
		Key: "LISTENBRAINZ_USER", Label: "ListenBrainz Username",
		Type: "text", Section: "discovery",
		Placeholder: "e.g. musiclover42", Required: true,
	},

	// ── Media System ───────────────────────────────────────────────
	{
		Key: "EXPLO_SYSTEM", Label: "Media System",
		Type: "select", Section: "system", Required: true,
		Options: []Option{
			{Value: "jellyfin", Label: "Jellyfin"},
			{Value: "emby", Label: "Emby"},
			{Value: "plex", Label: "Plex"},
			{Value: "subsonic", Label: "Subsonic"},
			{Value: "mpd", Label: "MPD"},
		},
	},
	{
		Key: "SYSTEM_URL", Label: "Server URL",
		Type: "url", Section: "system",
		Placeholder:  "e.g. http://192.168.1.100:8096",
		VisibleWhen:  &Condition{Field: "EXPLO_SYSTEM", In: netSystems},
		RequiredWhen: &Condition{Field: "EXPLO_SYSTEM", In: netSystems},
	},
	{
		Key: "API_KEY", Label: "API Key",
		Type: "text", Section: "system",
		VisibleWhen:  &Condition{Field: "EXPLO_SYSTEM", In: apiKeySystems},
		RequiredWhen: &Condition{Field: "EXPLO_SYSTEM", In: apiKeySystems},
	},
	{
		Key: "LIBRARY_NAME", Label: "Library Name",
		Type: "text", Section: "system",
		Placeholder: "e.g. Music",
		VisibleWhen: &Condition{Field: "EXPLO_SYSTEM", In: apiKeySystems},
	},
	{
		Key: "SYSTEM_USERNAME", Label: "Username",
		Type: "text", Section: "system",
		VisibleWhen:  &Condition{Field: "EXPLO_SYSTEM", Eq: "subsonic"},
		RequiredWhen: &Condition{Field: "EXPLO_SYSTEM", Eq: "subsonic"},
	},
	{
		Key: "SYSTEM_PASSWORD", Label: "Password",
		Type: "password", Section: "system",
		VisibleWhen:  &Condition{Field: "EXPLO_SYSTEM", Eq: "subsonic"},
		RequiredWhen: &Condition{Field: "EXPLO_SYSTEM", Eq: "subsonic"},
	},
	{
		Key: "PLAYLIST_DIR", Label: "Playlist Directory",
		Type: "text", Section: "system",
		Hint:         "Explo writes .m3u files here — MPD reads them as playlists.",
		VisibleWhen:  &Condition{Field: "EXPLO_SYSTEM", Eq: "mpd"},
		RequiredWhen: &Condition{Field: "EXPLO_SYSTEM", Eq: "mpd"},
	},
	{
		Key: "SLEEP", Label: "Library Scan Wait (minutes)",
		Type: "text", Section: "system",
		Placeholder: "2",
		Hint:        "How long to wait after triggering a library scan before creating playlists.",
		VisibleWhen: &Condition{Field: "EXPLO_SYSTEM", In: netSystems},
	},
	{
		Key: "PUBLIC_PLAYLIST", Label: "Public Playlists",
		Type: "text", Section: "system",
		Hint:        "Set to true to make playlists visible to all users (Subsonic).",
		VisibleWhen: &Condition{Field: "EXPLO_SYSTEM", Eq: "subsonic"},
	},

	// ── Downloader ─────────────────────────────────────────────────
	{
		Key: "DOWNLOAD_DIR", Label: "Download directory",
		Type: "text", Section: "downloader",
		Placeholder: "e.g. /data/ or ./downloads/",
		Required:    true,
	},
	{
		Key: "USE_SUBDIRECTORY", Label: "Use playlist subfolders",
		Type: "text", Section: "downloader",
		Hint: "When enabled, Explo creates a subfolder per playlist inside the download directory.",
	},
	{
		Key: "YOUTUBE_API_KEY", Label: "YouTube API Key",
		Type: "text", Section: "downloader",
		Placeholder:  "AIza…",
		Hint:         "Required when using YouTube. Enable the YouTube Data API v3.",
		VisibleWhen:  &Condition{Field: "DOWNLOAD_SERVICES", Contains: "youtube"},
		RequiredWhen: &Condition{Field: "DOWNLOAD_SERVICES", Contains: "youtube"},
	},
	{
		Key: "SLSKD_URL", Label: "Slskd URL",
		Type: "url", Section: "downloader",
		Placeholder:  "e.g. http://192.168.1.100:5030",
		VisibleWhen:  &Condition{Field: "DOWNLOAD_SERVICES", Contains: "slskd"},
		RequiredWhen: &Condition{Field: "DOWNLOAD_SERVICES", Contains: "slskd"},
	},
	{
		Key: "SLSKD_API_KEY", Label: "Slskd API Key",
		Type: "text", Section: "downloader",
		VisibleWhen:  &Condition{Field: "DOWNLOAD_SERVICES", Contains: "slskd"},
		RequiredWhen: &Condition{Field: "DOWNLOAD_SERVICES", Contains: "slskd"},
	},
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
	configPath string
	exploPath  string
	mux        *http.ServeMux
	manualRun  manualRunState
}

func NewServer(configPath, exploPath string) *Server {
	s := &Server{
		configPath: configPath,
		exploPath:  exploPath,
		mux:        http.NewServeMux(),
		manualRun:  newManualRunState(),
	}
	s.registerRoutes()
	return s
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
	embedded, _ := fs.Sub(distFiles, "dist")
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
		w.Write(indexHTML)
	})
	s.mux.HandleFunc("GET /api/config", s.handleGetConfig)
	s.mux.HandleFunc("GET /api/config/raw", s.handleGetConfigRaw)
	s.mux.HandleFunc("POST /api/config", s.handleSaveConfig)
	s.mux.HandleFunc("POST /api/config/reset", s.handleResetConfig)
	s.mux.HandleFunc("POST /api/config/schedules", s.handleSaveSchedule)
	s.mux.HandleFunc("POST /api/wizard/step1", s.handleWizardStep1)
	s.mux.HandleFunc("POST /api/wizard/step2", s.handleWizardStep2)
	s.mux.HandleFunc("POST /api/wizard/step3", s.handleWizardStep3)
	s.mux.HandleFunc("GET /api/browse", s.handleBrowse)
	s.mux.HandleFunc("POST /api/run", s.handleRun)
	s.mux.HandleFunc("GET /api/run/events", s.handleRunEvents)
	s.mux.HandleFunc("POST /api/run/stop", s.handleStopRun)
	s.mux.HandleFunc("GET /api/run/status", s.handleRunStatus)
	s.mux.HandleFunc("GET /api/logs", s.handleGetLog)
}

func (s *Server) Start(addr string) error {
	s.initServerLog()
	slog.Info("Explo web UI started", "addr", addr)
	return http.ListenAndServe(addr, s.mux)
}

// ── Logging ────────────────────────────────────────────────────────────────

// logPath returns the path to the single rolling log file.
func (s *Server) logPath() string {
	return filepath.Join(filepath.Dir(s.configPath), "logs", "explo.log")
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

// handleGetLog returns the contents of the rolling log file.
func (s *Server) handleGetLog(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.logPath())
	if err != nil && !os.IsNotExist(err) {
		http.Error(w, "failed to read log", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
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
// Sources are "env" when set via os.Environ (takes precedence), "file" otherwise.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.configPath)
	var fileValues map[string]string
	if err == nil {
		fileValues = parseEnvText(string(data))
	} else {
		fileValues = parseEnvText(string(sampleEnv))
	}

	values := make(map[string]string, len(allConfigKeys))
	sources := make(map[string]string, len(allConfigKeys))
	for _, key := range allConfigKeys {
		if v, ok := os.LookupEnv(key); ok {
			values[key] = v
			sources[key] = "env"
		} else if v := fileValues[key]; v != "" {
			values[key] = v
			sources[key] = "file"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ConfigResponse{Values: values, Sources: sources})
}

// handleGetConfigRaw returns the raw .env file contents as plain text.
func (s *Server) handleGetConfigRaw(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		data = sampleEnv
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

// handleSaveConfig writes the posted plain-text body directly to the .env file.
func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.WriteFile(s.configPath, data, 0600); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleResetConfig resets all settings and restarts the container.
func (s *Server) handleResetConfig(w http.ResponseWriter, r *http.Request) {
	if err := os.WriteFile(s.configPath, sampleEnv, 0600); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	go func() {
		time.Sleep(300 * time.Millisecond)
		syscall.Kill(1, syscall.SIGTERM)
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

	envPrefixes := map[string]string{
		"weekly-exploration": "WEEKLY_EXPLORATION",
		"weekly-jams":        "WEEKLY_JAMS",
		"daily-jams":         "DAILY_JAMS",
	}
	flagDefaults := map[string]string{
		"weekly-exploration": "--playlist weekly-exploration",
		"weekly-jams":        "--playlist weekly-jams",
		"daily-jams":         "--playlist daily-jams",
	}

	prefix, ok := envPrefixes[body.Name]
	if !ok {
		http.Error(w, "unknown playlist name", http.StatusBadRequest)
		return
	}

	updates := map[string]string{}
	if body.Enabled {
		dow := "*"
		if body.Day >= 0 {
			dow = fmt.Sprintf("%d", body.Day)
		}
		updates[prefix+"_SCHEDULE"] = fmt.Sprintf("%d %d * * %s", body.Minute, body.Hour, dow)
		updates[prefix+"_FLAGS"] = flagDefaults[body.Name]
	} else {
		updates[prefix+"_SCHEDULE"] = ""
		updates[prefix+"_FLAGS"] = ""
	}

	if err := updateEnvKeys(s.configPath, updates, sampleEnv); err != nil {
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

	type schedDef struct{ schedule, flags string }
	defaults := map[string]schedDef{
		"weekly-exploration": {"15 00 * * 2", "--playlist weekly-exploration"},
		"weekly-jams":        {"30 00 * * 1", "--playlist weekly-jams"},
		"daily-jams":         {"15 01 * * *", "--playlist daily-jams"},
	}
	envPrefixes := map[string]string{
		"weekly-exploration": "WEEKLY_EXPLORATION",
		"weekly-jams":        "WEEKLY_JAMS",
		"daily-jams":         "DAILY_JAMS",
	}

	enabled := make(map[string]bool, len(body.Playlists))
	for _, p := range body.Playlists {
		enabled[p] = true
	}

	updates := map[string]string{
		"LISTENBRAINZ_USER":      body.User,
		"LISTENBRAINZ_DISCOVERY": body.DiscoveryMode,
	}
	for playlist, prefix := range envPrefixes {
		if enabled[playlist] {
			d := defaults[playlist]
			updates[prefix+"_SCHEDULE"] = d.schedule
			updates[prefix+"_FLAGS"] = d.flags
		} else {
			updates[prefix+"_SCHEDULE"] = ""
			updates[prefix+"_FLAGS"] = ""
		}
	}

	if err := updateEnvKeys(s.configPath, updates, sampleEnv); err != nil {
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

	if err := updateEnvKeys(s.configPath, updates, sampleEnv); err != nil {
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
		TrackExtension   string   `json:"track_extension"`
		FilterList       string   `json:"filter_list"`
		SlskdURL         string   `json:"slskd_url"`
		SlskdAPIKey      string   `json:"slskd_api_key"`
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
	hasYoutube := strings.Contains(joined, "youtube")
	hasSlskd := strings.Contains(joined, "slskd")
	if (hasYoutube || (hasSlskd && body.MigrateDownloads)) && body.DownloadDir == "" {
		http.Error(w, "download_dir is required", http.StatusBadRequest)
		return
	}

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
		"TRACK_EXTENSION":   body.TrackExtension,
		"FILTER_LIST":       body.FilterList,
		"SLSKD_URL":         body.SlskdURL,
		"SLSKD_API_KEY":     body.SlskdAPIKey,
	}

	if err := updateEnvKeys(s.configPath, updates, sampleEnv); err != nil {
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
		json.NewEncoder(w).Encode([]string{})
		return
	}

	dirs := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, filepath.Join(path, e.Name()))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dirs)
}

// ── Manual run ─────────────────────────────────────────────────────────────

var errRunAlreadyStarted = errors.New("run already in progress")

// handleRun starts an explo run in the background. Clients follow output via /api/run/events.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form data", http.StatusBadRequest)
		return
	}

	args := buildArgs(r.FormValue("playlist"), r.FormValue("download_mode"),
		r.FormValue("persist") == "false", r.FormValue("exclude_local") == "true",
		s.configPath)

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
	json.NewEncoder(w).Encode(s.currentRunStatus())
}

func (s *Server) startRun(args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, s.exploPath, args...)
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
		slog.Warn("failed to open run log", "err", err)
	}

	s.manualRun.mu.Lock()
	if s.manualRun.running {
		s.manualRun.mu.Unlock()
		cancel()
		pr.Close()
		pw.Close()
		if lf != nil {
			lf.Close()
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
		pr.Close()
		pw.Close()
		if lf != nil {
			lf.Close()
		}
		return fmt.Errorf("failed to start explo: %w", err)
	}

	// Close write end in parent so reader gets EOF when child exits.
	pw.Close()

	go s.collectRunOutput(cmd, pr, lf)
	return nil
}

func (s *Server) collectRunOutput(cmd *exec.Cmd, pr *os.File, lf *os.File) {
	defer pr.Close()
	if lf != nil {
		defer lf.Close()
	}

	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		line := scanner.Text()
		if lf != nil {
			fmt.Fprintln(lf, line)
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
	json.NewEncoder(w).Encode(s.currentRunStatus())
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
			fmt.Fprintf(w, "event: %s\n", typ)
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
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

func buildArgs(playlist, downloadMode string, noPersist, excludeLocal bool, cfgPath string) []string {
	args := []string{"--config", cfgPath}
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
