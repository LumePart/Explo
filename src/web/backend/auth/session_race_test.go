package auth

import (
	"sync"
	"testing"
	"time"
)

// BUG-01: concurrent requests sharing one session (multiple tabs, fast
// clicking) raced on Session.data and Session.id, causing a fatal
// "concurrent map writes" panic. Run with -race.
func TestSession_ConcurrentAccess(t *testing.T) {
	manager := NewSessionManager(NewInMemorySessionStore(), time.Hour, 7*24*time.Hour, "explo_session_test")
	session := newSession()
	if err := manager.store.write(session); err != nil {
		t.Fatalf("writing session: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				session.Put("key", n)
				_ = session.Get("key")
				session.Delete("key")
				_ = session.Get("csrf_token")
			}
		}(i)
	}
	wg.Wait()

	// Session must still be usable after concurrent access.
	session.Put("authenticated", true)
	if got, _ := session.Get("authenticated").(bool); !got {
		t.Error("session data lost after concurrent access")
	}
	if session.ID() == "" {
		t.Error("session ID empty after concurrent access")
	}
}
