package util

import (
	"log/slog"
	"os"
	"path/filepath"
)

// RemoveDirsByPrefix deletes every immediate subdirectory of parentDir whose
// name starts with prefix. Returns the number of directories successfully removed.
// Non-directory matches and missing parents are ignored; per-entry removal errors
// are logged but do not abort the operation.
func RemoveDirsByPrefix(parentDir, prefix string) (int, error) {
	matches, err := filepath.Glob(filepath.Join(parentDir, prefix+"*"))
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || !info.IsDir() {
			continue
		}
		if err := os.RemoveAll(m); err != nil {
			slog.Warn("failed to remove directory", "path", m, "err", err.Error())
			continue
		}
		removed++
	}
	return removed, nil
}
