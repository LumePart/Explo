package auth

import (
	"net/http"
	"log/slog"
	"encoding/json"
)

func (a *AuthStore) HandleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if a.Disabled {
		writeAuthStatus(w, true, true)
		return
	}

	sess := a.sessionManager.GetSession(r)
	auth, _ := sess.Get("authenticated").(bool)
	if !auth {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeAuthStatus(w, true, false)
}

func writeAuthStatus(w http.ResponseWriter, authenticated, disabled bool) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]bool{
		"authenticated": authenticated,
		"auth_disabled": disabled,
	}); err != nil {
		slog.Error("failed encoding auth status to http", "msg", err.Error())
	}
}

func (a *AuthStore) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		err := http.StatusMethodNotAllowed
		http.Error(w, "Invalid request method", err)
		return
	}

	// Nothing to authenticate against. Accept, so a cached frontend that still
	// renders the login form does not get a confusing 401 from the empty hash.
	if a.Disabled {
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if !a.CompareCreds(username, password) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	sess := a.sessionManager.GetSession(r)
	sess.Put("authenticated", true)
	sess.Put("username", username)
	
	if err := a.sessionManager.Migrate(sess); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	slog.Info("successful login", "user", username)
}

func (a *AuthStore) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if a.Disabled {
		w.WriteHeader(http.StatusOK)
		return
	}

	sess := a.sessionManager.GetSession(r)
	sess.Delete("authenticated")
	sess.Delete("username")
	w.WriteHeader(http.StatusOK)
}

func (a *AuthStore) HandleCSRF(w http.ResponseWriter, r *http.Request) {
	session := a.sessionManager.GetSession(r)

	token, _ := session.Get("csrf_token").(string)

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]string{
		"csrf_token": token,
	}); err != nil {
		slog.Error("failed encoding token to http", "msg", err.Error())
	}
}
