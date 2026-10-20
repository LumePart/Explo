package client

import (
	"os"
	"path/filepath"
	"testing"

	"explo/src/models"
)

// BUG-16: the .m3u was opened with O_APPEND and the handle never
// closed, so every re-run doubled the playlist.
func TestMPDCreatePlaylist_ReRunDoesNotDuplicate(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig("", "")
	cfg.PlaylistDir = dir + string(os.PathSeparator)
	cfg.PlaylistName = "weekly"
	mpd := NewMPD(cfg)

	tracks := []*models.Track{
		{File: "a.flac", Present: true},
		{File: "b.flac", Present: true},
		{File: "missing.flac", Present: false},
	}

	for i := 0; i < 2; i++ {
		if err := mpd.CreatePlaylist(tracks); err != nil {
			t.Fatalf("CreatePlaylist run %d: %v", i, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(dir, "weekly.m3u"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a.flac\nb.flac\n" {
		t.Errorf("playlist = %q, want each present track exactly once", data)
	}
}
