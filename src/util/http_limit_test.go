package util

import (
	"strings"
	"testing"
)

// BUG-20: remote responses were read with unbounded io.ReadAll on
// clients with no timeout, so one stalled or huge response could hang
// or OOM the app.
func TestReadLimited(t *testing.T) {
	data, err := readLimited(strings.NewReader("hello"), 10)
	if err != nil || string(data) != "hello" {
		t.Errorf("readLimited small = %q, %v; want hello, nil", data, err)
	}

	if _, err := readLimited(strings.NewReader(strings.Repeat("x", 100)), 10); err == nil {
		t.Error("readLimited over-limit = nil error, want rejection instead of truncation")
	}

	data, err = readLimited(strings.NewReader(strings.Repeat("x", 10)), 10)
	if err != nil || len(data) != 10 {
		t.Errorf("readLimited at-limit = %d bytes, %v; want 10, nil", len(data), err)
	}
}

func TestRemoteHTTPClientHasTimeout(t *testing.T) {
	if remoteHTTPClient.Timeout <= 0 {
		t.Error("remoteHTTPClient has no timeout; a stalled server would hang forever")
	}
}
