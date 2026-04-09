package web

import (
	"bufio"
	"embed"
	"encoding/json"
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
)

//go:embed static
var staticFiles embed.FS

//go:embed sample.env
var sampleEnv []byte

type Server struct {
	configPath string
	exploPath  string
	mux        *http.ServeMux
	runMu      sync.Mutex
}

func NewServer(configPath, exploPath string) *Server {
	s := &Server{
		configPath: configPath,
		exploPath:  exploPath,
		mux:        http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	staticFS, _ := fs.Sub(staticFiles, "static")
	s.mux.Handle("/", http.FileServer(http.FS(staticFS)))
	s.mux.HandleFunc("GET /api/config", s.handleGetConfig)
	s.mux.HandleFunc("POST /api/config", s.handleSaveConfig)
	s.mux.HandleFunc("POST /api/wizard/step1", s.handleWizardStep1)
	s.mux.HandleFunc("POST /api/wizard/step2", s.handleWizardStep2)
	s.mux.HandleFunc("GET /api/browse", s.handleBrowse)
	s.mux.HandleFunc("POST /api/run", s.handleRun)
	s.mux.HandleFunc("GET /api/run/status", s.handleRunStatus)
}

func (s *Server) Start(addr string) error {
	slog.Info("Explo web UI started", "addr", addr)
	return http.ListenAndServe(addr, s.mux)
}

// handleGetConfig returns the raw .env file as plain text.
// Falls back to the embedded sample.env template if no config file exists yet.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.configPath)
	if os.IsNotExist(err) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(sampleEnv)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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

// handleRun starts an explo run and streams log output via SSE.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if !s.runMu.TryLock() {
		http.Error(w, "a run is already in progress", http.StatusConflict)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.runMu.Unlock()
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		s.runMu.Unlock()
		http.Error(w, "bad form data", http.StatusBadRequest)
		return
	}

	args := buildArgs(r.FormValue("playlist"), r.FormValue("download_mode"),
		r.FormValue("persist") == "false", r.FormValue("exclude_local") == "true",
		s.configPath)

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

	cmd := exec.CommandContext(r.Context(), s.exploPath, args...)
	// Strip WEB_UI from env so the child process runs normally, not as web server
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "WEB_UI=") {
			env = append(env, e)
		}
	}
	cmd.Env = env

	pr, pw, err := os.Pipe()
	if err != nil {
		s.runMu.Unlock()
		sendEvent("error", "failed to create pipe: "+err.Error())
		return
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		s.runMu.Unlock()
		sendEvent("error", "failed to start explo: "+err.Error())
		return
	}

	// Close write end in parent so reader gets EOF when child exits
	pw.Close()

	exitCh := make(chan int, 1)
	go func() {
		defer s.runMu.Unlock()
		cmd.Wait()
		code := 0
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		exitCh <- code
	}()

	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		sendEvent("", scanner.Text())
	}
	pr.Close()

	sendEvent("done", fmt.Sprintf("%d", <-exitCh))
}

// handleRunStatus returns whether a run is currently in progress.
func (s *Server) handleRunStatus(w http.ResponseWriter, r *http.Request) {
	locked := !s.runMu.TryLock()
	if !locked {
		s.runMu.Unlock()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"running": locked})
}

// handleWizardStep1 saves discovery settings (username + enabled playlists with default schedules).
func (s *Server) handleWizardStep1(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User      string   `json:"user"`
		Playlists []string `json:"playlists"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.User == "" {
		http.Error(w, "user is required", http.StatusBadRequest)
		return
	}

	// Default schedules per playlist type
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

	updates := map[string]string{"LISTENBRAINZ_USER": body.User}
	for playlist, prefix := range envPrefixes {
		if enabled[playlist] {
			d := defaults[playlist]
			updates[prefix+"_SCHEDULE"] = d.schedule
			updates[prefix+"_FLAGS"] = d.flags
		} else {
			// Clear so start.sh won't register a cron job for it
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
		idx := strings.IndexByte(trimmed, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
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

// handleWizardStep2 saves media system configuration.
func (s *Server) handleWizardStep2(w http.ResponseWriter, r *http.Request) {
	var body struct {
		System      string `json:"system"`
		URL         string `json:"url"`
		APIKey      string `json:"api_key"`
		LibraryName string `json:"library_name"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		PlaylistDir string `json:"playlist_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.System == "" {
		http.Error(w, "system is required", http.StatusBadRequest)
		return
	}

	updates := map[string]string{
		"EXPLO_SYSTEM":    body.System,
		"SYSTEM_URL":      body.URL,
		"API_KEY":         body.APIKey,
		"LIBRARY_NAME":    body.LibraryName,
		"SYSTEM_USERNAME": body.Username,
		"SYSTEM_PASSWORD": body.Password,
		"PLAYLIST_DIR":    body.PlaylistDir,
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
