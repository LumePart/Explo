package backend

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type Session struct {
	createdAt      time.Time
	mu             sync.Mutex
	id             string
	data           map[string]any
}

/* type SessionStore interface {
	read(id string) (*Session, error)
	write(session *Session) error
	destroy(id string) error
	gc(absoluteExpiration time.Duration) error
} */

type SessionManager struct {
	store              *InMemorySessionStore
	//idleExpiration     time.Duration // Could be used if project grows (session expiry based on user activity)
	absoluteExpiration time.Duration
	cookieName         string
}

type sessionContextKey struct {}

func newSession() *Session {
	return &Session{
		id:             generateToken(),
		data:           map[string]any{"csrf_token": generateToken()},
		createdAt:      time.Now(),
	}
}

func NewSessionManager(
	store *InMemorySessionStore,
	gcInterval,
	absoluteExpiration time.Duration,
	cookieName string) *SessionManager {

	m := &SessionManager{
		store:              store,
		absoluteExpiration: absoluteExpiration,
		cookieName:         cookieName,
	}

	go m.gc(gcInterval)

	return m
}

func (m *SessionManager) gc(d time.Duration) {
	ticker := time.NewTicker(d)

	for range ticker.C {
		if err := m.store.gc(m.absoluteExpiration); err != nil {
			panic("garbage collection failed")
		}
	}
}

func (m *SessionManager) validate(session *Session) bool {
	if time.Since(session.createdAt) > m.absoluteExpiration {
        
        // Delete the session from the store
		err := m.store.destroy(session.id)
		if err != nil {
			panic(err)
		}

		return false
	}

	return true
}

func (m *SessionManager) start(r *http.Request) (*Session, *http.Request) {
	var session *Session

    // Read From Cookie
	cookie, err := r.Cookie(m.cookieName)
	if err == nil {
		session, err = m.store.read(cookie.Value)
		if err != nil {
			slog.Error("Failed to read session from store", "msg", err.Error())
		}
	}

    // Generate a new session if no session exists or it expired
	if session == nil || !m.validate(session) {
		session = newSession()
		if err := m.store.write(session); err != nil {
			panic("failed writing session")
		}
	}

    // Attach session to context
	ctx := context.WithValue(r.Context(), sessionContextKey{}, session)
	r = r.WithContext(ctx)

	return session, r
}

func (m *SessionManager) Migrate(session *Session) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	err := m.store.destroy(session.id)
	if err != nil {
		return err
	}

	session.id = generateToken()

	return m.store.write(session)
}


// Used to generate session and CSRF tokens
func generateToken() string {
	id := make([]byte, 32)

	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		panic("failed to generate token")
	}

	return base64.RawURLEncoding.EncodeToString(id)
}

func (s *Session) Get(key string) any {
	return s.data[key]
}

func (s *Session) Put(key string, value any) {
	s.data[key] = value
}

func (s *Session) Delete(key string) {
	delete(s.data, key)
}

func (m *SessionManager) GetSession(r *http.Request) *Session {
	session, ok := r.Context().Value(sessionContextKey{}).(*Session)
	if !ok {
		panic("session not found in request context")
	}

	return session
}

func (m *SessionManager) verifyCSRFToken(r *http.Request, session *Session) bool {
	sToken, ok := session.Get("csrf_token").(string)
	if !ok {
		return false
	}

	token := r.FormValue("csrf_token")

	if token == "" {
		token = r.Header.Get("X-CSRF-Token")
	}
	return token == sToken
}
