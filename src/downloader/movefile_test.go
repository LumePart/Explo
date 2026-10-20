package downloader

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-07: moveFile truncated the destination with os.Create before
// copying, so src == dst zeroed the only copy and a name collision
// silently erased an existing library song.

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMoveFile_SameSourceAndDestinationKeepsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.flac")
	writeTestFile(t, path, "audio")

	if err := moveFile(path, path, false); err != nil {
		t.Fatalf("moveFile(same, same) = %v, want nil (already in place)", err)
	}
	if got := readTestFile(t, path); got != "audio" {
		t.Errorf("file content = %q, want %q — the file was truncated", got, "audio")
	}
}

func TestMoveFile_ExistingDestinationIsNotErased(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "downloads", "song.flac")
	dst := filepath.Join(dir, "library", "song.flac")
	writeTestFile(t, src, "new download")
	writeTestFile(t, dst, "existing library song")

	if err := moveFile(src, dst, false); err == nil {
		t.Fatal("moveFile onto an existing destination = nil, want refusal")
	}
	if got := readTestFile(t, dst); got != "existing library song" {
		t.Errorf("destination = %q, want the existing song untouched", got)
	}
	if got := readTestFile(t, src); got != "new download" {
		t.Errorf("source = %q, want the download kept after refusal", got)
	}
}

func TestMoveFile_NormalMoveStillWorks(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "downloads", "song.flac")
	dst := filepath.Join(dir, "library", "song.flac")
	writeTestFile(t, src, "audio")

	if err := moveFile(src, dst, false); err != nil {
		t.Fatalf("moveFile normal: %v", err)
	}
	if got := readTestFile(t, dst); got != "audio" {
		t.Errorf("destination = %q, want audio", got)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source still present after move: %v", err)
	}
}
