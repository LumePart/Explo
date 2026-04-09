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
	"strings"
	"sync"
)

//go:embed static
var staticFiles embed.FS

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
	s.mux.HandleFunc("POST /api/run", s.handleRun)
	s.mux.HandleFunc("GET /api/run/status", s.handleRunStatus)
}

func (s *Server) Start(addr string) error {
	slog.Info("Explo web UI started", "addr", addr)
	return http.ListenAndServe(addr, s.mux)
}

// handleGetConfig returns the raw .env file as plain text.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.configPath)
	if os.IsNotExist(err) {
		w.WriteHeader(http.StatusOK)
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
