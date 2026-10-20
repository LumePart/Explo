package client

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"explo/src/config"
	"explo/src/util"
)

// recordingServer reports every request path it receives, so a test can assert
// that no request was made at all.
func recordingServer(t *testing.T, paths *[]string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Errorf("fake server failed to write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func testConfig(url, playlistID string) config.ClientConfig {
	return config.ClientConfig{
		URL:          url,
		PlaylistID:   playlistID,
		PlaylistName: "Weekly Exploration",
		Creds:        config.Credentials{Headers: map[string]string{}},
	}
}

// SearchPlaylist leaves PlaylistID empty when it finds nothing, and the URL
// that used to produce was "/playlists/" -- a delete aimed at the collection
// rather than at one playlist. Plex answered 403, which surfaced as a warning
// with a notification attached for what is really a no-op.
func TestPlexDeletePlaylist_SendsNothingWithoutAnID(t *testing.T) {
	var paths []string
	server := recordingServer(t, &paths)

	plex := NewPlex(testConfig(server.URL, ""), util.NewHttp(util.HttpClientConfig{Timeout: 5}))

	if err := plex.DeletePlaylist(); err != nil {
		t.Errorf("DeletePlaylist() = %v, want nil -- nothing to delete is not a failure", err)
	}
	if len(paths) != 0 {
		t.Errorf("requested %v, want no request at all", paths)
	}
}

// With an ID it must still delete that one playlist.
func TestPlexDeletePlaylist_DeletesTheFoundPlaylist(t *testing.T) {
	var paths []string
	server := recordingServer(t, &paths)

	plex := NewPlex(testConfig(server.URL, "12345"), util.NewHttp(util.HttpClientConfig{Timeout: 5}))

	if err := plex.DeletePlaylist(); err != nil {
		t.Fatalf("DeletePlaylist() = %v, want nil", err)
	}
	if len(paths) != 1 || paths[0] != "/playlists/12345" {
		t.Errorf("requested %v, want a single DELETE of /playlists/12345", paths)
	}
}

// Jellyfin has the same hole, and its bare URL is "/Items" -- every item on the
// server rather than one playlist.
func TestJellyfinDeletePlaylist_SendsNothingWithoutAnID(t *testing.T) {
	var paths []string
	server := recordingServer(t, &paths)

	jf := NewJellyfin(testConfig(server.URL, ""), util.NewHttp(util.HttpClientConfig{Timeout: 5}))

	if err := jf.DeletePlaylist(); err != nil {
		t.Errorf("DeletePlaylist() = %v, want nil", err)
	}
	if len(paths) != 0 {
		t.Errorf("requested %v, want no request at all", paths)
	}
}

func TestJellyfinDeletePlaylist_DeletesTheFoundPlaylist(t *testing.T) {
	var paths []string
	server := recordingServer(t, &paths)

	jf := NewJellyfin(testConfig(server.URL, "abc"), util.NewHttp(util.HttpClientConfig{Timeout: 5}))

	if err := jf.DeletePlaylist(); err != nil {
		t.Fatalf("DeletePlaylist() = %v, want nil", err)
	}
	if len(paths) != 1 || paths[0] != "/Items/abc" {
		t.Errorf("requested %v, want a single DELETE of /Items/abc", paths)
	}
}

// Emby used to delete nothing and report success, so "replace playlist"
// left the old playlist in place and created a duplicate.
func TestEmbyDeletePlaylist_DeletesTheFoundPlaylist(t *testing.T) {
	var paths []string
	server := recordingServer(t, &paths)

	emby := NewEmby(testConfig(server.URL, "abc"), util.NewHttp(util.HttpClientConfig{Timeout: 5}))

	if err := emby.DeletePlaylist(); err != nil {
		t.Fatalf("DeletePlaylist() = %v, want nil", err)
	}
	if len(paths) != 1 || paths[0] != "/emby/Items/abc" {
		t.Errorf("requested %v, want a single DELETE of /emby/Items/abc", paths)
	}
}

func TestEmbyDeletePlaylist_SendsNothingWithoutAnID(t *testing.T) {
	var paths []string
	server := recordingServer(t, &paths)

	emby := NewEmby(testConfig(server.URL, ""), util.NewHttp(util.HttpClientConfig{Timeout: 5}))

	if err := emby.DeletePlaylist(); err != nil {
		t.Errorf("DeletePlaylist() = %v, want nil when there is no playlist", err)
	}
	if len(paths) != 0 {
		t.Errorf("requested %v, want no request at all", paths)
	}
}

// Subsonic would send deletePlaylist with an empty id.
func TestSubsonicDeletePlaylist_SendsNothingWithoutAnID(t *testing.T) {
	var paths []string
	server := recordingServer(t, &paths)

	sub := NewSubsonic(testConfig(server.URL, ""), util.NewHttp(util.HttpClientConfig{Timeout: 5}))

	if err := sub.DeletePlaylist(); err != nil {
		t.Errorf("DeletePlaylist() = %v, want nil", err)
	}
	if len(paths) != 0 {
		t.Errorf("requested %v, want no request at all", paths)
	}
}

// MPD already skipped the delete, but reported it as an error, which reached
// the user as the same spurious warning.
func TestMPDDeletePlaylist_MissingPlaylistIsNotAnError(t *testing.T) {
	mpd := NewMPD(testConfig("", ""))

	if err := mpd.DeletePlaylist(); err != nil {
		t.Errorf("DeletePlaylist() = %v, want nil when there is no playlist", err)
	}
}

// A playlist that is there still gets removed.
func TestMPDDeletePlaylist_RemovesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weekly.m3u")
	if err := os.WriteFile(path, []byte("#EXTM3U\n"), 0o644); err != nil {
		t.Fatalf("seeding playlist: %v", err)
	}

	mpd := NewMPD(testConfig("", path))

	if err := mpd.DeletePlaylist(); err != nil {
		t.Fatalf("DeletePlaylist() = %v, want nil", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("playlist file still present: %v", err)
	}
}
