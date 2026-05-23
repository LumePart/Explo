package backend

import (
	"net/http"
	"sync"
	"time"
	"fmt"
)

type sessionResponseWriter struct {
	http.ResponseWriter
	sessionManager *SessionManager
	request        *http.Request
	done           bool
}

type InMemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func (w *sessionResponseWriter) Write(b []byte) (int, error) {
	writeCookieIfNecessary(w)

	return w.ResponseWriter.Write(b)
}

func (w *sessionResponseWriter) WriteHeader(code int) {
	// write essential headers
	w.Header().Add("Vary", "Cookie")
 	w.Header().Add("Cache-Control", `no-cache="Set-Cookie"`)

	writeCookieIfNecessary(w)

	w.ResponseWriter.WriteHeader(code)
}

func (w *sessionResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Flush forwards to the wrapped ResponseWriter so SSE handlers can do
// w.(http.Flusher) — without this, the auth middleware breaks streaming.
func (w *sessionResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		writeCookieIfNecessary(w)
		f.Flush()
	}
}

func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string]*Session),
	}
}

func (s *InMemorySessionStore) read(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
	return nil, fmt.Errorf("session not found")
	}
	
	return session, nil	
}

func (s *InMemorySessionStore) write(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.id] = session

	return nil
}

func (s *InMemorySessionStore) destroy(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, id)

	return nil
}

func (s *InMemorySessionStore) gc(absoluteExpiration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, session := range s.sessions {
		if time.Since(session.createdAt) > absoluteExpiration {
			delete(s.sessions, id)
		}
	}

	return nil
}

func (m *SessionManager) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Start the session
		session, rws := m.start(r)

		// Create a new response writer
		sw := &sessionResponseWriter{
			ResponseWriter: w,
			sessionManager: m,
			request:        rws,
		}

		isMutating :=
			r.Method == http.MethodPost ||
			r.Method == http.MethodPut ||
			r.Method == http.MethodPatch ||
			r.Method == http.MethodDelete

		if isMutating && r.URL.Path != "/login" {
			if !m.verifyCSRFToken(rws, session) {
				http.Error(sw, "CSRF token mismatch", http.StatusForbidden)
				return
			}
		}
		

		// Call the next handler and pass the new response writer and new request
		next.ServeHTTP(sw, rws)

		// Write the session cookie to the response if not already written
		writeCookieIfNecessary(sw)
	})
}

func writeCookieIfNecessary(w *sessionResponseWriter) {
	if w.done {
		return
	}

	session, ok := w.request.Context().Value(sessionContextKey{}).(*Session)
	if !ok {
		panic("session not found in request context")
	}

	cookie := &http.Cookie{
		Name:     w.sessionManager.cookieName,
		Value:    session.id,
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(w.sessionManager.absoluteExpiration),
		MaxAge:   int(w.sessionManager.absoluteExpiration / time.Second),
	}

	http.SetCookie(w.ResponseWriter, cookie)

	w.done = true
}