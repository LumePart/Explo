package downloader

import (
	"sync"
	"testing"

	"explo/src/models"
)

// BUG-02: AlbumCache was read without CacheMu while cacheAlbum wrote it
// from concurrent goroutines, and a missing entry was dereferenced. Run
// with -race.
func TestLidarrAlbumCache_ConcurrentAccess(t *testing.T) {
	c := &Lidarr{AlbumCache: make(map[string]*AlbumMetadata)}
	newTrack := func() *models.Track {
		return &models.Track{MainArtist: "Artist", Album: "Album", AlbumID: "1", MainArtistID: "2"}
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine works on its own track, as concurrent album
			// fetches do; they share only the cache map under test.
			track := newTrack()
			for j := 0; j < 200; j++ {
				c.cacheAlbum(track)
				if _, ok := c.cachedAlbum(c.cacheKey(track)); !ok {
					t.Error("cached album disappeared")
					return
				}
				c.populateFromCache(track)
			}
		}()
	}
	wg.Wait()

	if _, ok := c.cachedAlbum(c.cacheKey(newTrack())); !ok {
		t.Fatal("album not cached after concurrent writes")
	}
	if _, ok := c.cachedAlbum("missing|key"); ok {
		t.Error("missing key reported as cached")
	}
}
