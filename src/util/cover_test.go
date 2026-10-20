package util

import (
	"testing"
)

// BUG-25: DownloadCover returned the destination path even when the
// fetch failed, so callers embedded a cover from a nonexistent file
// and the track save failed with the artwork.
func TestDownloadCover_FailedFetchReturnsEmptyPaths(t *testing.T) {
	api, path := DownloadCover("http://127.0.0.1:1/cover.jpg", t.TempDir())
	if api != "" || path != "" {
		t.Errorf("DownloadCover(failed) = %q, %q; want empty paths so callers proceed without cover art", api, path)
	}

	api, path = DownloadCover("", t.TempDir())
	if api != "" || path != "" {
		t.Errorf("DownloadCover(empty url) = %q, %q; want empty paths", api, path)
	}
}
