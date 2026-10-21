package downloader

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfg "explo/src/config"
	"explo/src/models"
)

func albumClient(t *testing.T, handler http.HandlerFunc) *Slskd {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := NewSlskd(cfg.Slskd{
		URL:       server.URL,
		APIKey:    "test-key",
		Timeout:   5,
		AlbumMode: true,
		Filters: cfg.Filters{
			Extensions:  []string{"flac", "mp3"},
			MinBitRate:  256,
			MinBitDepth: 8,
			FilterList:  []string{"live", "remix"},
		},
	}, "/data/")
	client.AddHeader()

	return client
}

// peer describes one peer sharing one directory.
type peer struct {
	user  string
	dir   string
	names []string
}

// albumResults builds search results without naming SearchResults' anonymous
// element type, so new fields on it cannot break these tests.
func albumResults(peers ...peer) SearchResults {
	results := make(SearchResults, len(peers))
	for i, p := range peers {
		results[i].Username = p.user
		results[i].HasFreeUploadSlot = true
		results[i].FileCount = len(p.names)
		for _, name := range p.names {
			results[i].Files = append(results[i].Files, File{
				Name:      p.dir + `\` + name,
				Extension: "flac",
				BitRate:   900,
				BitDepth:  16,
				Size:      1000,
				Length:    200,
			})
		}
	}
	return results
}

func albumTrack() models.Track {
	return models.Track{
		CleanTitle: "Bad Guy",
		Title:      "Bad Guy",
		Artist:     "Billie Eilish",
		MainArtist: "Billie Eilish",
		Album:      "When We All Fall Asleep",
		Duration:   200_000,
	}
}

// The whole point of album mode: every track in the release is returned, not
// just the one that was recommended.
func TestCollectAlbumFiles_ReturnsWholeRelease(t *testing.T) {
	client := albumClient(t, nil)

	results := albumResults(peer{"peer1", `@@x\Billie Eilish\When We All Fall Asleep`, []string{"01 Bury A Friend.flac", "02 Bad Guy.flac", "03 Xanny.flac"}})

	files, err := client.CollectAlbumFiles(albumTrack(), results)
	if err != nil {
		t.Fatalf("CollectAlbumFiles() = %v, want nil", err)
	}
	if len(files) != 3 {
		t.Fatalf("got %d files, want the whole 3-track release", len(files))
	}
	if !strings.Contains(string(files[0].Name), "Bad Guy") {
		t.Errorf("files[0] = %q, want the recommended track first", files[0].Name)
	}
}

// A directory that happens to contain the track but is not the album must lose
// to the one that actually names the album.
func TestCollectAlbumFiles_PrefersTheMatchingAlbum(t *testing.T) {
	client := albumClient(t, nil)

	results := albumResults(
		peer{"peer1", `@@x\Various\Party Mix 2019`,
			[]string{"04 Bad Guy.flac", "05 Something Else.flac", "06 Another.flac", "07 More.flac"}},
		peer{"peer2", `@@y\Billie Eilish\When We All Fall Asleep`,
			[]string{"01 Bury A Friend.flac", "02 Bad Guy.flac"}},
	)

	files, err := client.CollectAlbumFiles(albumTrack(), results)
	if err != nil {
		t.Fatalf("CollectAlbumFiles() = %v, want nil", err)
	}
	if files[0].Username != "peer2" {
		t.Errorf("selected peer %q, want peer2 -- the compilation has more files but is the wrong release", files[0].Username)
	}
}

// A release with no copy of the recommended track is useless here, however
// well it otherwise matches.
func TestCollectAlbumFiles_RequiresTheRecommendedTrack(t *testing.T) {
	client := albumClient(t, nil)

	results := albumResults(peer{"peer1", `@@x\Billie Eilish\When We All Fall Asleep`,
		[]string{"01 Bury A Friend.flac", "03 Xanny.flac"}})

	_, err := client.CollectAlbumFiles(albumTrack(), results)
	if err == nil {
		t.Fatal("CollectAlbumFiles() = nil, want an error when the recommended track is absent")
	}
	if !strings.Contains(err.Error(), "no release found") {
		t.Errorf("error = %q, want it to say no release was found", err)
	}
}

// FILTER_LIST applies to the directory in album mode, so a live pressing is
// rejected as a whole rather than track by track.
func TestCollectAlbumFiles_RejectsFilteredRelease(t *testing.T) {
	client := albumClient(t, nil)

	results := albumResults(peer{"peer1", `@@x\Billie Eilish\When We All Fall Asleep (Live)`, []string{"01 Bury A Friend.flac", "02 Bad Guy.flac"}})

	if _, err := client.CollectAlbumFiles(albumTrack(), results); err == nil {
		t.Fatal("CollectAlbumFiles() = nil, want the live release to be filtered out")
	}
}

// Sibling tracks have their own durations, so the recommended track's runtime
// must not be used to exclude them.
func TestCollectAlbumFiles_DoesNotApplyTrackDurationToSiblings(t *testing.T) {
	client := albumClient(t, nil)

	results := albumResults(peer{"peer1", `@@x\Billie Eilish\When We All Fall Asleep`, []string{"01 Bury A Friend.flac", "02 Bad Guy.flac"}})

	// A 6-minute sibling against a 200s recommended track.
	results[0].Files[0].Length = 360

	files, err := client.CollectAlbumFiles(albumTrack(), results)
	if err != nil {
		t.Fatalf("CollectAlbumFiles() = %v, want nil", err)
	}
	if len(files) != 2 {
		t.Errorf("got %d files, want 2 -- the longer sibling was wrongly excluded", len(files))
	}
}

// selectionClient builds the same album-mode client as albumClient but
// without the httptest server: release selection never touches the network,
// so these tests also run where binding a local port is forbidden.
func selectionClient() *Slskd {
	client := NewSlskd(cfg.Slskd{
		URL:       "http://localhost:5030",
		APIKey:    "test-key",
		Timeout:   5,
		AlbumMode: true,
		Filters: cfg.Filters{
			Extensions:  []string{"flac", "mp3"},
			MinBitRate:  256,
			MinBitDepth: 8,
			FilterList:  []string{"live", "remix"},
		},
	}, "/data/")
	client.AddHeader()

	return client
}

// A lone file in an album-named folder is a stray, not the album: the fuller
// release must win even when the stray scores higher on name match alone.
func TestCollectAlbumFiles_PrefersFullReleaseOverSingleFileStray(t *testing.T) {
	client := selectionClient()

	// The stray outscores the real thing on name match alone (artist and
	// album in the folder vs album only), so only the full-release
	// preference can save the correct pick.
	results := albumResults(
		peer{"peer1", `@@x\Billie Eilish\When We All Fall Asleep`,
			[]string{"02 Bad Guy.flac"}},
		peer{"peer2", `@@y\Music\When We All Fall Asleep`,
			[]string{"01 Bury A Friend.flac", "02 Bad Guy.flac", "03 Xanny.flac"}},
	)

	files, err := client.CollectAlbumFiles(albumTrack(), results)
	if err != nil {
		t.Fatalf("CollectAlbumFiles() = %v, want nil", err)
	}
	if files[0].Username != "peer2" {
		t.Errorf("selected peer %q, want peer2 -- the single-file stray won over the full release", files[0].Username)
	}
	if len(files) != 3 {
		t.Errorf("got %d files, want the whole 3-track release", len(files))
	}
}

// A 64-file folder outscores the real 10-track album on the fuller-release
// size term, so the cap has to exclude it from the strict pass. The track
// carries the enriched count, the way a real recommendation does.
func TestCollectAlbumFiles_SkipsAnOversizedDump(t *testing.T) {
	client := selectionClient()
	track := albumTrack()
	track.TrackTotal = 10

	dump := []string{"01 Bad Guy.flac"}
	for i := 2; i <= 64; i++ {
		dump = append(dump, "Dump Track.flac")
	}
	correct := []string{"01 Bury A Friend.flac", "02 Bad Guy.flac", "03 Xanny.flac",
		"04 Everything I Wanted.flac", "05 Ocean Eyes.flac", "06 Lovely.flac",
		"07 You Should See Me.flac", "08 All The Good Girls.flac",
		"09 Wish You Were Gay.flac", "10 Ilomilo.flac"}

	results := albumResults(
		peer{"peer1", `@@x\Billie Eilish\When We All Fall Asleep`, dump},
		peer{"peer2", `@@y\Billie Eilish\When We All Fall Asleep`, correct},
	)

	files, err := client.CollectAlbumFiles(track, results)
	if err != nil {
		t.Fatalf("CollectAlbumFiles() = %v, want nil", err)
	}
	if files[0].Username != "peer2" {
		t.Errorf("selected peer %q, want peer2 -- the 64-file dump won over the 10-track album", files[0].Username)
	}
	if len(files) != 10 {
		t.Errorf("got %d files, want the 10-track album", len(files))
	}
}

// The dump stays eligible too: with nothing right-sized available, the big
// folder still beats no music.
func TestCollectAlbumFiles_FallsBackToADumpWhenNothingElse(t *testing.T) {
	client := selectionClient()
	track := albumTrack()
	track.TrackTotal = 10

	dump := []string{"01 Bad Guy.flac"}
	for i := 2; i <= 64; i++ {
		dump = append(dump, "Dump Track.flac")
	}

	results := albumResults(peer{"peer1", `@@x\Billie Eilish\When We All Fall Asleep`, dump})

	files, err := client.CollectAlbumFiles(track, results)
	if err != nil {
		t.Fatalf("CollectAlbumFiles() = %v, want nil", err)
	}
	if len(files) != 64 {
		t.Errorf("got %d files, want the 64-file dump as a last resort", len(files))
	}
}

// The stray stays eligible: with nothing fuller available, one song still
// beats no song.
func TestCollectAlbumFiles_FallsBackToSingleFileWhenNothingElse(t *testing.T) {
	client := selectionClient()

	results := albumResults(peer{"peer1", `@@x\Billie Eilish\When We All Fall Asleep`,
		[]string{"02 Bad Guy.flac"}})

	files, err := client.CollectAlbumFiles(albumTrack(), results)
	if err != nil {
		t.Fatalf("CollectAlbumFiles() = %v, want nil", err)
	}
	if len(files) != 1 {
		t.Errorf("got %d files, want the lone stray as a last resort", len(files))
	}
}

// One request per release, carrying every file, and the track left pointing at
// the recommended one so the monitor and the playlist still work.
func TestQueueAlbumDownload_QueuesEveryFileInOneRequest(t *testing.T) {
	var gotPayload []DownloadPayload
	var gotPath string
	var requests int

	client := albumClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		if err := json.Unmarshal(body, &gotPayload); err != nil {
			t.Errorf("unmarshaling request body: %v", err)
		}
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Errorf("fake slskd failed to write response: %v", err)
		}
	})

	results := albumResults(peer{"peer1", `@@x\Billie Eilish\When We All Fall Asleep`, []string{"01 Bury A Friend.flac", "02 Bad Guy.flac", "03 Xanny.flac"}})

	files, err := client.CollectAlbumFiles(albumTrack(), results)
	if err != nil {
		t.Fatalf("CollectAlbumFiles() = %v, want nil", err)
	}

	track := albumTrack()
	if err := client.queueAlbumDownload(files, &track); err != nil {
		t.Fatalf("queueAlbumDownload() = %v, want nil", err)
	}

	if requests != 1 {
		t.Errorf("made %d requests, want 1 for the whole release", requests)
	}
	if gotPath != "/api/v0/transfers/downloads/peer1" {
		t.Errorf("posted to %q, want the peer's download endpoint", gotPath)
	}
	if len(gotPayload) != 3 {
		t.Errorf("queued %d files, want 3", len(gotPayload))
	}
	if !strings.Contains(track.File, "Bad Guy") {
		t.Errorf("track.File = %q, want the recommended track -- the monitor follows this", track.File)
	}
	if len(track.AlbumFiles) != 2 {
		t.Errorf("AlbumFiles has %d entries, want the 2 siblings", len(track.AlbumFiles))
	}
	for _, sibling := range track.AlbumFiles {
		if sibling == track.File {
			t.Error("the recommended track is also listed as a sibling; it would be migrated twice")
		}
	}
}

// Album mode searches for the release. Without an album name there is nothing
// to search for, so it must fall back to the track.
func TestAlbumSearchTerm(t *testing.T) {
	track := albumTrack()
	if got, want := albumSearchTerm(track), "Billie Eilish - When We All Fall Asleep"; got != want {
		t.Errorf("albumSearchTerm() = %q, want %q", got, want)
	}

	track.Album = "  "
	if got, want := albumSearchTerm(track), "Bad Guy - Billie Eilish"; got != want {
		t.Errorf("with no album, albumSearchTerm() = %q, want the track fallback %q", got, want)
	}
}

// Peers report Windows paths; grouping by directory has to survive that or
// every file looks like it lives in its own release.
func TestNormalizePeerPathGrouping(t *testing.T) {
	client := albumClient(t, nil)

	results := albumResults(peer{"peer1", `@@x\Billie Eilish\When We All Fall Asleep`, []string{"01 Bury A Friend.flac", "02 Bad Guy.flac"}})

	dirs := client.groupByDirectory(results)
	if len(dirs) != 1 {
		t.Fatalf("grouped into %d directories, want 1 -- backslash paths were not normalised", len(dirs))
	}
	if len(dirs[0].files) != 2 {
		t.Errorf("directory holds %d files, want 2", len(dirs[0].files))
	}
}

// downloadsResponse fakes GET /api/v0/transfers/downloads: one peer, one
// directory, and whatever per-file transfer states the test needs.
func downloadsResponse(t *testing.T, user string, states map[string]string) string {
	t.Helper()

	files := make([]map[string]any, 0, len(states))
	for name, state := range states {
		files = append(files, map[string]any{"filename": name, "state": state})
	}

	body, err := json.Marshal([]map[string]any{{
		"username":    user,
		"directories": []map[string]any{{"files": files}},
	}})
	if err != nil {
		t.Fatalf("building fake downloads response: %v", err)
	}
	return string(body)
}

// A sibling slskd has finished belongs in the library; one still transferring
// must be left alone, because copying it would put a truncated track there.
func TestMoveAlbumSiblings_SkipsUnfinishedTransfers(t *testing.T) {
	const (
		peerDirectory = `@@x\Billie Eilish\When We All Fall Asleep`
		done          = peerDirectory + `\01 Bury A Friend.flac`
		pending       = peerDirectory + `\03 Xanny.flac`
	)

	client := albumClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(downloadsResponse(t, "peer1", map[string]string{
			done:    "Completed, Succeeded",
			pending: "InProgress",
		}))); err != nil {
			t.Errorf("fake slskd failed to write response: %v", err)
		}
	})

	srcDir := t.TempDir()
	destDir := t.TempDir()
	for _, name := range []string{"01 Bury A Friend.flac", "03 Xanny.flac"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}

	track := albumTrack()
	track.MainArtistID = "peer1" // queueAlbumDownload stashes the peer here
	track.AlbumFiles = []string{done, pending}

	client.MoveAlbumSiblings(srcDir, destDir, &track)

	if _, err := os.Stat(filepath.Join(destDir, "01 Bury A Friend.flac")); err != nil {
		t.Errorf("finished sibling was not migrated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(srcDir, "01 Bury A Friend.flac")); !os.IsNotExist(err) {
		t.Error("finished sibling was copied but the original was left behind")
	}
	if _, err := os.Stat(filepath.Join(destDir, "03 Xanny.flac")); !os.IsNotExist(err) {
		t.Error("a still-transferring sibling was migrated; the library would get a truncated file")
	}
	if _, err := os.Stat(filepath.Join(srcDir, "03 Xanny.flac")); err != nil {
		t.Errorf("still-transferring sibling should be left in place: %v", err)
	}
}

// Album mode is opt-in, so the migration must be inert when it is off even if
// a track somehow carries siblings.
func TestMoveAlbumSiblings_NoopWhenAlbumModeIsOff(t *testing.T) {
	client := albumClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("slskd was queried with album mode off")
	})
	client.Cfg.AlbumMode = false

	srcDir := t.TempDir()
	destDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "01 Bury A Friend.flac"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seeding sibling: %v", err)
	}

	track := albumTrack()
	track.MainArtistID = "peer1"
	track.AlbumFiles = []string{`@@x\dir\01 Bury A Friend.flac`}

	client.MoveAlbumSiblings(srcDir, destDir, &track)

	if _, err := os.Stat(filepath.Join(srcDir, "01 Bury A Friend.flac")); err != nil {
		t.Errorf("sibling was migrated with album mode off: %v", err)
	}
}

// *Slskd must satisfy the optional interface DownloadClient looks for, or the
// monitor's release migration silently never fires.
func TestSlskdImplementsAlbumMigrator(t *testing.T) {
	var _ albumMigrator = &Slskd{}
}

// The bug this guards: the recommended track is queued first and normally
// finishes first, so the siblings are still transferring at the moment the
// release is first migrated. They have to be picked up on a later pass instead
// of being skipped once and abandoned.
func TestMoveAlbumSiblings_PicksUpSiblingsThatFinishLater(t *testing.T) {
	const (
		peerDirectory = `@@x\Billie Eilish\When We All Fall Asleep`
		early         = peerDirectory + `\01 Bury A Friend.flac`
		late          = peerDirectory + `\03 Xanny.flac`
	)

	// The state the slow sibling reports, flipped between attempts.
	lateState := "InProgress"

	client := albumClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(downloadsResponse(t, "peer1", map[string]string{
			early: "Completed, Succeeded",
			late:  lateState,
		}))); err != nil {
			t.Errorf("fake slskd failed to write response: %v", err)
		}
	})

	srcDir := t.TempDir()
	destDir := t.TempDir()
	for _, name := range []string{"01 Bury A Friend.flac", "03 Xanny.flac"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}

	track := albumTrack()
	track.MainArtistID = "peer1"
	track.AlbumFiles = []string{early, late}

	if remaining := client.MoveAlbumSiblings(srcDir, destDir, &track); remaining != 1 {
		t.Fatalf("first pass left %d files pending, want 1", remaining)
	}
	if _, err := os.Stat(filepath.Join(destDir, "03 Xanny.flac")); !os.IsNotExist(err) {
		t.Fatal("the unfinished sibling was migrated on the first pass")
	}

	lateState = "Completed, Succeeded"

	if remaining := client.MoveAlbumSiblings(srcDir, destDir, &track); remaining != 0 {
		t.Errorf("second pass left %d files pending, want 0", remaining)
	}
	if _, err := os.Stat(filepath.Join(destDir, "03 Xanny.flac")); err != nil {
		t.Errorf("the sibling that finished later was never migrated: %v", err)
	}
}

// Already-migrated files must not be retried: the source is gone, so a second
// attempt would fail forever and hold the release open until the deadline.
func TestMoveAlbumSiblings_ForgetsMigratedFiles(t *testing.T) {
	const done = `@@x\dir\01 Bury A Friend.flac`

	client := albumClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(downloadsResponse(t, "peer1", map[string]string{
			done: "Completed, Succeeded",
		}))); err != nil {
			t.Errorf("fake slskd failed to write response: %v", err)
		}
	})

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "01 Bury A Friend.flac"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seeding sibling: %v", err)
	}

	track := albumTrack()
	track.MainArtistID = "peer1"
	track.AlbumFiles = []string{done}

	client.MoveAlbumSiblings(srcDir, t.TempDir(), &track)

	if len(track.AlbumFiles) != 0 {
		t.Errorf("AlbumFiles still holds %v after migration", track.AlbumFiles)
	}
}

// A sibling slskd has given up on must not keep the release open, or the
// monitor waits out the full deadline for a file that is never coming.
func TestMoveAlbumSiblings_DropsFailedSiblings(t *testing.T) {
	const failed = `@@x\dir\03 Xanny.flac`

	client := albumClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(downloadsResponse(t, "peer1", map[string]string{
			failed: "Completed, Errored",
		}))); err != nil {
			t.Errorf("fake slskd failed to write response: %v", err)
		}
	})

	track := albumTrack()
	track.MainArtistID = "peer1"
	track.AlbumFiles = []string{failed}

	if remaining := client.MoveAlbumSiblings(t.TempDir(), t.TempDir(), &track); remaining != 0 {
		t.Errorf("a failed sibling left %d files pending, want 0", remaining)
	}
}
