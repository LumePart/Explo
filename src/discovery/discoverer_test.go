package discovery

import (
	"strings"
	"testing"

	cfg "explo/src/config"
)

// BUG-04: a typo in DISCOVERY_SERVICE made NewDiscoverer return nil and
// the caller panicked dereferencing it, with no hint of the bad value.
func TestNewDiscoverer_UnknownServiceErrorsWithValue(t *testing.T) {
	c, err := NewDiscoverer(cfg.DiscoveryConfig{Discovery: "listenbranz"}, nil)
	if err == nil {
		t.Fatal("NewDiscoverer with a typo = nil error, want a clear failure")
	}
	if c != nil {
		t.Errorf("client = %v, want nil on error", c)
	}
	if !strings.Contains(err.Error(), "listenbranz") {
		t.Errorf("error %q does not name the bad value", err)
	}
}

func TestNewDiscoverer_ListenBrainzOK(t *testing.T) {
	c, err := NewDiscoverer(cfg.DiscoveryConfig{Discovery: "listenbrainz"}, nil)
	if err != nil {
		t.Fatalf("NewDiscoverer(listenbrainz): %v", err)
	}
	if c == nil || c.Discovery == nil {
		t.Error("NewDiscoverer(listenbrainz) returned an incomplete client")
	}
}
