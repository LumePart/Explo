package playlist

import (
	"fmt"
	"sync"
	"testing"
)

// BUG-06: unsynchronized load-modify-save of custom-playlists.json let two
// writers drop each other's playlists, and a non-atomic write could be
// parsed as empty mid-truncate, wiping the file on the next save.
func TestUpdateCustomPlaylists_ConcurrentAppendsAllSurvive(t *testing.T) {
	dir := t.TempDir()
	const writers = 16

	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("custom-%d", n)
			err := updateCustomPlaylists(dir, func(cur []CustomPlaylist) ([]CustomPlaylist, error) {
				return append(cur, CustomPlaylist{ID: id, Name: id}), nil
			})
			if err != nil {
				t.Errorf("update %s: %v", id, err)
			}
			// Readers must never observe a torn/empty file mid-write.
			_ = loadCustomPlaylists(dir)
		}(i)
	}
	wg.Wait()

	got := loadCustomPlaylists(dir)
	if len(got) != writers {
		t.Fatalf("saved %d playlists, want %d — a concurrent writer's append was lost", len(got), writers)
	}
	seen := make(map[string]bool, writers)
	for _, p := range got {
		seen[p.ID] = true
	}
	for i := 0; i < writers; i++ {
		if !seen[fmt.Sprintf("custom-%d", i)] {
			t.Errorf("playlist custom-%d missing after concurrent saves", i)
		}
	}
}

func TestUpdateCustomPlaylists_ErrorSkipsSave(t *testing.T) {
	dir := t.TempDir()
	if err := saveCustomPlaylists(dir, []CustomPlaylist{{ID: "keep", Name: "keep"}}); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	if err := updateCustomPlaylists(dir, func(cur []CustomPlaylist) ([]CustomPlaylist, error) {
		return nil, errPlaylistNotFound
	}); err != errPlaylistNotFound {
		t.Fatalf("update error = %v, want errPlaylistNotFound", err)
	}
	got := loadCustomPlaylists(dir)
	if len(got) != 1 || got[0].ID != "keep" {
		t.Errorf("list = %v, want the seeded playlist untouched after a failed update", got)
	}
}
