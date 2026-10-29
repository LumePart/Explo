package run

import (
	"net/http"
	"errors"
	"encoding/json"
	"log/slog"
	"fmt"

	"explo/src/web/backend/users"
)

// handleRun starts an explo run in the background. Clients follow output via /api/ui/run/events.
func (mr *ManualRun) HandleRun(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "bad form data", http.StatusBadRequest)
		return
	}

	replacePlaylist := r.FormValue("replace_playlist") != "false"
	user := r.FormValue("user")
	if user != "" && user != "all" && !users.IDRe.MatchString(user) {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}
	args := buildArgs(r.FormValue("playlist"), r.FormValue("download_mode"), mr.cfg.WebEnvPath, replacePlaylist, user)

	if err := mr.startRun(args); err != nil {
		if errors.Is(err, errRunAlreadyStarted) {
			http.Error(w, "a run is already in progress", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(mr.currentRunStatus()); err != nil {
		slog.Warn("failed to encode current run status", "msg", err.Error())
	}
}

func (mr *ManualRun) HandleStopRun(w http.ResponseWriter, r *http.Request) {
	mr.state.mu.Lock()
	cancel := mr.state.cancel
	running := mr.state.running
	mr.state.mu.Unlock()

	if !running || cancel == nil {
		http.Error(w, "no run is currently in progress", http.StatusConflict)
		return
	}

	cancel()
	w.WriteHeader(http.StatusAccepted)
}

func (mr *ManualRun) HandleRunStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(mr.currentRunStatus()); err != nil {
		slog.Warn("failed encoding current run status to response")
	}
}

// handleRunEvents streams the current in-memory run log, then follows new lines
// until the active run exits. Safe to reconnect after a browser refresh.
func (mr *ManualRun) HandleRunEvents(w http.ResponseWriter, r *http.Request) {
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
	mr.state.mu.Lock()
	lines := append([]string(nil), mr.state.logs...)
	running := mr.state.running
	var exitCode *int
	if mr.state.exitCode != nil {
		code := *mr.state.exitCode
		exitCode = &code
	}
	if running {
		mr.state.subscribers[ch] = struct{}{}
	}
	mr.state.mu.Unlock()

	for _, line := range lines {
		sendEvent("", line)
	}
	if !running {
		if exitCode != nil {
			sendEvent("done", fmt.Sprintf("%d", *exitCode))
		}
		return
	}

	defer mr.unsubscribeRun(ch)
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