package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"explo/src/web/backend/app"
)

// BUG-08: concurrent UpdateEnvKeys calls each read the same starting
// .env and the later write silently undid the earlier update; a crash
// mid-write also left a truncated file.
func TestUpdateEnvKeys_ConcurrentUpdatesAllSurvive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("BASE=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Settings{cfg: app.Config{WebEnvPath: path}}

	const writers = 12
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("KEY_%d", n)
			if err := s.UpdateEnvKeys(map[string]string{key: fmt.Sprint(n)}, nil); err != nil {
				t.Errorf("UpdateEnvKeys(%s): %v", key, err)
			}
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := s.ParseEnvText(string(data))
	if got["BASE"] != "1" {
		t.Errorf("BASE = %q, want 1", got["BASE"])
	}
	for i := 0; i < writers; i++ {
		key := fmt.Sprintf("KEY_%d", i)
		if got[key] != fmt.Sprint(i) {
			t.Errorf("%s = %q, want %d — a concurrent update was lost", key, got[key], i)
		}
	}
	// Atomic rename must not litter temp files.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != ".env" {
		t.Errorf("dir entries = %v, want only .env", entries)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Errorf(".env mode = %v, err = %v; want 0600", info.Mode().Perm(), err)
	}
}
