package users

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// Hooks are injected by the server so this package stays stdlib-only
// (playlist imports users, so users cannot import playlist).
type Hooks struct {
	// RemoveCache deletes one user's cache directory.
	RemoveCache func(dataDir, userID string) error
	// OnUserDeleted runs after a user is removed, with the remaining users.
	OnUserDeleted func(dataDir, deletedID string, remaining []User) error
}

// Handlers serves the users API.
type Handlers struct {
	dataDir string
	store   *Store
	hooks   Hooks
}

// NewHandlers builds the users API handlers.
func NewHandlers(dataDir string, hooks Hooks) *Handlers {
	return &Handlers{dataDir: dataDir, store: NewStore(dataDir), hooks: hooks}
}

// userResponse never carries the ListenBrainz token.
type userResponse struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ListenBrainzUser string `json:"listenbrainz_user"`
	TokenSet         bool   `json:"token_set"`
	PlexUser         string `json:"plex_user"`
	Enabled          bool   `json:"enabled"`
}

func toResponse(u User) userResponse {
	return userResponse{
		ID: u.ID, Name: u.Name, ListenBrainzUser: u.ListenBrainzUser,
		TokenSet: u.ListenBrainzToken != "", PlexUser: u.PlexUser, Enabled: u.Enabled,
	}
}

type userRequest struct {
	Name              string `json:"name"`
	ListenBrainzUser  string `json:"listenbrainz_user"`
	ListenBrainzToken string `json:"listenbrainz_token"`
	PlexUser          string `json:"plex_user"`
	Enabled           bool   `json:"enabled"`
}

// clientError marks errors that map to a 4xx response.
type clientError struct {
	status int
	msg    string
}

func (e *clientError) Error() string { return e.msg }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("users: failed to write response", "err", err)
	}
}

func fail(w http.ResponseWriter, err error) {
	var ce *clientError
	if errors.As(err, &ce) {
		http.Error(w, ce.msg, ce.status)
		return
	}
	slog.Error("users: request failed", "err", err)
	http.Error(w, "internal error: "+err.Error(), http.StatusInternalServerError)
}

// HandleList returns all users without tokens.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	list, err := Load(h.dataDir)
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]userResponse, 0, len(list))
	for _, u := range list {
		out = append(out, toResponse(u))
	}
	writeJSON(w, http.StatusOK, out)
}

// HandleCreate adds a user; the ID is derived from the name.
func (h *Handlers) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	u := User{
		ID: Slug(req.Name), Name: req.Name, ListenBrainzUser: req.ListenBrainzUser,
		ListenBrainzToken: req.ListenBrainzToken, PlexUser: req.PlexUser, Enabled: req.Enabled,
	}
	err := h.store.Update(func(list []User) ([]User, error) {
		for _, e := range list {
			if e.ID == u.ID {
				return nil, &clientError{http.StatusBadRequest, "a user with that id already exists"}
			}
		}
		if err := Validate(u, list, false); err != nil {
			return nil, &clientError{http.StatusBadRequest, err.Error()}
		}
		return append(list, u), nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(u))
}

// HandleUpdate edits a user. The path ID is authoritative; an empty token keeps the stored one.
func (h *Handlers) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req userRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	var updated User
	err := h.store.Update(func(list []User) ([]User, error) {
		for i := range list {
			if list[i].ID != id {
				continue
			}
			u := User{
				ID: id, Name: req.Name, ListenBrainzUser: req.ListenBrainzUser,
				ListenBrainzToken: req.ListenBrainzToken, PlexUser: req.PlexUser, Enabled: req.Enabled,
			}
			if u.ListenBrainzToken == "" {
				u.ListenBrainzToken = list[i].ListenBrainzToken
			}
			if err := Validate(u, list, true); err != nil {
				return nil, &clientError{http.StatusBadRequest, err.Error()}
			}
			list[i] = u
			updated = u
			return list, nil
		}
		return nil, &clientError{http.StatusNotFound, "user not found"}
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(updated))
}

// HandleDelete removes a user record and that user's cache directory. It never touches Plex.
func (h *Handlers) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var remaining []User
	err := h.store.Update(func(list []User) ([]User, error) {
		out := make([]User, 0, len(list))
		found := false
		for _, u := range list {
			if u.ID == id {
				found = true
				continue
			}
			out = append(out, u)
		}
		if !found {
			return nil, &clientError{http.StatusNotFound, "user not found"}
		}
		remaining = out
		return out, nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	if h.hooks.RemoveCache != nil {
		if err := h.hooks.RemoveCache(h.dataDir, id); err != nil {
			slog.Error("users: failed to remove user cache", "user", id, "err", err)
			http.Error(w, "user removed but cache cleanup failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if h.hooks.OnUserDeleted != nil {
		if err := h.hooks.OnUserDeleted(h.dataDir, id, remaining); err != nil {
			slog.Error("users: post-delete hook failed", "user", id, "err", err)
			http.Error(w, "user removed but follow-up cleanup failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
