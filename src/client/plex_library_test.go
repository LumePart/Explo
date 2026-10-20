package client

import (
	"encoding/json"
	"testing"
)

// BUG-05: Plex returning an empty MediaContainer after library creation
// panicked on Library[0].
func TestFirstLibraryKey_EmptyContainerErrors(t *testing.T) {
	var libs Libraries
	if err := json.Unmarshal([]byte(`{"MediaContainer":{"size":0}}`), &libs); err != nil {
		t.Fatal(err)
	}
	if _, err := firstLibraryKey(libs); err == nil {
		t.Fatal("firstLibraryKey(empty) = nil error, want a descriptive failure instead of a panic")
	}
}

func TestFirstLibraryKey_ReturnsKey(t *testing.T) {
	var libs Libraries
	if err := json.Unmarshal([]byte(`{"MediaContainer":{"Directory":[{"title":"Music","key":"7"}]}}`), &libs); err != nil {
		t.Fatal(err)
	}
	key, err := firstLibraryKey(libs)
	if err != nil {
		t.Fatalf("firstLibraryKey: %v", err)
	}
	if key != "7" {
		t.Errorf("key = %q, want 7", key)
	}
}
