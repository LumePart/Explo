package auth

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

/* type Server struct {
	mux *http.ServeMux
} */

type AuthStore struct {
	Username string
	Hash string
	Disabled bool // true when no credentials are configured; all routes become public
	sessionManager *SessionManager
}

func NewAuthStore(user, password string, sessionManager *SessionManager) *AuthStore{
	// No credentials configured at all: run the UI unauthenticated instead of
	// showing a login screen that accepts empty input anyway.
	if user == "" && password == "" {
		return &AuthStore{
			Disabled: true,
			sessionManager: sessionManager,
		}
	}

	hashPass, err := hashPassword(password)
	if err != nil {
		panic("failed to hash password")
	}

	return &AuthStore{
		Username: user,
		Hash: hashPass,
		sessionManager: sessionManager,
	}
}


func (a *AuthStore) CompareCreds(formUser, formPass string) bool {
	if formUser != a.Username || bcrypt.CompareHashAndPassword([]byte(a.Hash), []byte(formPass)) != nil {
		return false
	}
	return true
}

func (a *AuthStore) RequireAuth(next http.Handler) http.Handler {
	if a.Disabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := a.sessionManager.GetSession(r)

		auth, _ := sess.Get("authenticated").(bool)
		if !auth {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	return string(bytes), err
	
}

