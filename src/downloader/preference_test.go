package downloader

import (
	"testing"

	cfg "explo/src/config"
	"explo/src/models"
)

func prefClient(pref string) Slskd {
	return Slskd{Cfg: cfg.Slskd{
		DownloadAttempts: 10,
		SizePreference:   pref,
		Filters: cfg.Filters{
			Extensions:  []string{"flac", "mp3"},
			MinBitDepth: 8,
		},
	}}
}

func names(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Name
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The candidates every ordering test starts from, deliberately not in any
// meaningful order.
func candidates() []File {
	return []File{
		{Name: "mp3-256", Extension: "mp3", BitRate: 256},
		{Name: "flac-cd", Extension: "flac", BitRate: 1000},
		{Name: "mp3-320", Extension: "mp3", BitRate: 320},
		{Name: "flac-hi", Extension: "flac", BitRate: 4608},
	}
}

func TestSortByPreference(t *testing.T) {
	tests := []struct {
		name string
		pref string
		want []string
	}{
		{
			// The point of "smaller": the best lossy file, not the smallest
			// file. 320 beats 256, and both beat any FLAC.
			name: "smaller takes the best lossy first",
			pref: PreferSmaller,
			want: []string{"mp3-320", "mp3-256", "flac-hi", "flac-cd"},
		},
		{
			name: "larger takes the best lossless first",
			pref: PreferLarger,
			want: []string{"flac-hi", "flac-cd", "mp3-320", "mp3-256"},
		},
		{
			// Unchanged behaviour: EXTENSIONS is flac,mp3, so FLAC first, and
			// within a format the order the search returned them in.
			name: "none follows the extension order",
			pref: PreferNone,
			want: []string{"flac-cd", "flac-hi", "mp3-256", "mp3-320"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := prefClient(tt.pref)
			files := candidates()
			client.sortByPreference(files)

			if got := names(files); !equal(got, tt.want) {
				t.Errorf("order = %v, want %v", got, tt.want)
			}
		})
	}
}

// The preference ranks, it does not filter: when the preferred band is empty
// the other one must still be reached rather than the track being lost.
func TestSortByPreference_FallsBackWhenTheBandIsEmpty(t *testing.T) {
	client := prefClient(PreferSmaller)
	files := []File{
		{Name: "flac-cd", Extension: "flac", BitRate: 1000},
		{Name: "flac-hi", Extension: "flac", BitRate: 4608},
	}

	client.sortByPreference(files)

	if len(files) != 2 {
		t.Fatalf("kept %d files, want both -- preference must not drop candidates", len(files))
	}
	if files[0].Name != "flac-hi" {
		t.Errorf("first = %s, want the best of the only band available", files[0].Name)
	}
}

// A file nobody described is still eligible, but it cannot be called the best
// of its band.
func TestSortByPreference_UnknownBitrateRanksLastInItsBand(t *testing.T) {
	client := prefClient(PreferSmaller)
	files := []File{
		{Name: "mp3-unknown", Extension: "mp3"},
		{Name: "mp3-256", Extension: "mp3", BitRate: 256},
		{Name: "flac-cd", Extension: "flac", BitRate: 1000},
	}

	client.sortByPreference(files)

	want := []string{"mp3-256", "mp3-unknown", "flac-cd"}
	if got := names(files); !equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// filterFiles caps at DownloadAttempts, so the ordering decides which
// candidates survive, not just what order they are tried in.
func TestFilterFiles_PreferenceDecidesWhichSurviveTheCap(t *testing.T) {
	client := prefClient(PreferSmaller)
	client.Cfg.DownloadAttempts = 1

	got, err := client.filterFiles(candidates())
	if err != nil {
		t.Fatalf("filterFiles() = %v, want nil", err)
	}
	if len(got) != 1 || got[0].Name != "mp3-320" {
		t.Errorf("kept %v, want just mp3-320", names(got))
	}
}

func TestReleaseBitRate_MedianIgnoresAnOutlier(t *testing.T) {
	files := []File{
		{BitRate: 320}, {BitRate: 320}, {BitRate: 320}, {BitRate: 320},
		{BitRate: 96}, // a bonus track nobody cares about
	}

	if got := releaseBitRate(files); got != 320 {
		t.Errorf("releaseBitRate() = %d, want 320 -- one bad track must not sink the release", got)
	}
}

func TestReleaseBitRate_UnknownWhenNothingIsDescribed(t *testing.T) {
	if got := releaseBitRate([]File{{}, {}}); got != 0 {
		t.Errorf("releaseBitRate() = %d, want 0", got)
	}
}

func albumDir(ext string, rate int, names ...string) peerDir {
	dir := peerDir{dir: "Billie Eilish - When We All Fall Asleep"}
	for _, n := range names {
		dir.files = append(dir.files, File{Name: n, Extension: ext, BitRate: rate})
	}
	return dir
}

// Quality is a tiebreak between equally good matches. It must never be a
// reason to take a release that matches the recommendation worse.
func TestScoreDirWithPreference_QualityNeverBeatsAMatch(t *testing.T) {
	track := models.Track{
		CleanTitle: "Bad Guy",
		MainArtist: "Billie Eilish",
		Album:      "When We All Fall Asleep",
	}

	client := prefClient(PreferLarger)

	right := albumDir("mp3", 320, "01 Bad Guy.mp3")
	right.dir = "Billie Eilish - When We All Fall Asleep"

	wrong := albumDir("flac", 1000, "01 Bad Guy.flac")
	wrong.dir = "Some Other Compilation"

	rightScore := client.scoreDirWithPreference(right, track)
	wrongScore := client.scoreDirWithPreference(wrong, track)

	if wrongScore >= rightScore {
		t.Errorf("wrong-album FLAC scored %d against right-album MP3 %d; quality outranked the match",
			wrongScore, rightScore)
	}
}

// Between two releases that match equally well, the preference decides.
func TestScoreDirWithPreference_BreaksATieBetweenEqualMatches(t *testing.T) {
	track := models.Track{
		CleanTitle: "Bad Guy",
		MainArtist: "Billie Eilish",
		Album:      "When We All Fall Asleep",
	}

	lossy := albumDir("mp3", 320, "01 Bad Guy.mp3")
	lossless := albumDir("flac", 1000, "01 Bad Guy.flac")

	smaller := prefClient(PreferSmaller)
	if smaller.scoreDirWithPreference(lossy, track) <= smaller.scoreDirWithPreference(lossless, track) {
		t.Error("prefer smaller did not favour the lossy release")
	}

	larger := prefClient(PreferLarger)
	if larger.scoreDirWithPreference(lossless, track) <= larger.scoreDirWithPreference(lossy, track) {
		t.Error("prefer larger did not favour the lossless release")
	}
}

// The default has to leave release scoring exactly as it was.
func TestScoreDirWithPreference_NoneChangesNothing(t *testing.T) {
	track := models.Track{
		CleanTitle: "Bad Guy",
		MainArtist: "Billie Eilish",
		Album:      "When We All Fall Asleep",
	}

	client := prefClient(PreferNone)
	for _, dir := range []peerDir{
		albumDir("mp3", 320, "01 Bad Guy.mp3"),
		albumDir("flac", 1000, "01 Bad Guy.flac"),
	} {
		if got, want := client.scoreDirWithPreference(dir, track), scoreDir(dir, track); got != want {
			t.Errorf("with no preference score = %d, want the unmodified %d", got, want)
		}
	}
}

func TestNormalisePreference(t *testing.T) {
	cases := map[string]string{
		"smaller": PreferSmaller,
		"LARGER":  PreferLarger,
		" none ":  PreferNone,
		"":        PreferNone,
		"nonsense": PreferNone,
	}
	for in, want := range cases {
		if got := normalisePreference(in); got != want {
			t.Errorf("normalisePreference(%q) = %q, want %q", in, got, want)
		}
	}
}

// releaseDir builds a candidate release of n files whose directory names both
// the artist and the album, so only the size term varies between cases.
func releaseDir(n int) peerDir {
	dir := peerDir{dir: "Billie Eilish - When We All Fall Asleep"}
	for i := 0; i < n; i++ {
		dir.files = append(dir.files, File{
			Name:      "track.mp3",
			Extension: "mp3",
			BitRate:   320,
		})
	}
	return dir
}

func albumTrackOf(total, discs int) models.Track {
	return models.Track{
		CleanTitle: "Bad Guy",
		MainArtist: "Billie Eilish",
		Album:      "When We All Fall Asleep",
		TrackTotal: total,
		DiscTotal:  discs,
	}
}

func TestReleaseSizeScore_PrefersTheRealAlbumLength(t *testing.T) {
	track := albumTrackOf(11, 1)

	exact := releaseSizeScore(releaseDir(11), track, PreferSmallerRelease)
	deluxe := releaseSizeScore(releaseDir(21), track, PreferSmallerRelease)

	if exact <= deluxe {
		t.Errorf("11-track release scored %d against a 21-track deluxe %d; the deluxe would win",
			exact, deluxe)
	}
	if exact != maxReleaseSizeScore {
		t.Errorf("an exact match scored %d, want the full %d", exact, maxReleaseSizeScore)
	}
}

// The hole this design exists to close: under a naive "fewest tracks wins",
// a directory holding only the recommended track beats every real album and
// album mode stops fetching albums.
func TestReleaseSizeScore_ASingleFileLosesToTheRealAlbum(t *testing.T) {
	track := albumTrackOf(12, 1)

	lone := releaseSizeScore(releaseDir(1), track, PreferSmallerRelease)
	album := releaseSizeScore(releaseDir(12), track, PreferSmallerRelease)

	if lone >= album {
		t.Errorf("a 1-file directory scored %d against the 12-track album %d; album mode would "+
			"quietly stop downloading albums", lone, album)
	}
}

// A real single is not a degenerate case: when the release genuinely has one
// track, one file is the right answer.
func TestReleaseSizeScore_ASingleFileWinsForARealSingle(t *testing.T) {
	track := albumTrackOf(1, 1)

	lone := releaseSizeScore(releaseDir(1), track, PreferSmallerRelease)
	padded := releaseSizeScore(releaseDir(9), track, PreferSmallerRelease)

	if lone <= padded {
		t.Errorf("one file scored %d for a one-track single, against %d for nine", lone, padded)
	}
}

func TestReleaseSizeScore_FallsBackWithoutACanonicalCount(t *testing.T) {
	cases := map[string]models.Track{
		// Enrichment is off or the MusicBrainz lookup failed.
		"no track total": albumTrackOf(0, 1),
		// TrackTotal counts the first medium only, so a two-disc release
		// legitimately holds more files than it. Scoring closeness would
		// punish the complete release for being complete.
		"multi disc": albumTrackOf(11, 2),
	}

	for name, track := range cases {
		t.Run(name, func(t *testing.T) {
			dir := releaseDir(21)
			got := releaseSizeScore(dir, track, PreferSmallerRelease)
			want := fullerReleaseScore(dir)

			if got != want {
				t.Errorf("score = %d, want the fuller-release fallback %d", got, want)
			}
		})
	}
}

// The default must leave release scoring exactly as it was.
func TestReleaseSizeScore_FullerIsUnchanged(t *testing.T) {
	track := albumTrackOf(11, 1)

	for _, n := range []int{1, 11, 21, 60} {
		dir := releaseDir(n)
		if got, want := releaseSizeScore(dir, track, PreferFullerRelease), min(n, 40); got != want {
			t.Errorf("fuller score for %d files = %d, want %d", n, got, want)
		}
	}
}

// A 64-file folder is not a 10-track album, however well the folder is
// named. Anything at or under twice the expected count stays eligible --
// deluxe padding included.
func TestOversizedDir_FlagsADump(t *testing.T) {
	track := albumTrackOf(10, 1)

	if !oversizedDir(releaseDir(64), track) {
		t.Error("64 files for a 10-track album is not flagged oversized")
	}
	if oversizedDir(releaseDir(10), track) {
		t.Error("the exact 10-track release is flagged oversized")
	}
	if oversizedDir(releaseDir(20), track) {
		t.Error("a 20-track deluxe for a 10-track album is flagged oversized; the cap must leave deluxe padding alone")
	}
}

// Same unknown-count cases as the scoring fallback: no expected count, no
// verdict.
func TestOversizedDir_NeverFiresWithoutAnExpectedCount(t *testing.T) {
	cases := map[string]models.Track{
		"no track total": albumTrackOf(0, 1),
		"multi disc":     albumTrackOf(11, 2),
	}

	for name, track := range cases {
		t.Run(name, func(t *testing.T) {
			if oversizedDir(releaseDir(64), track) {
				t.Errorf("64 files flagged oversized with no usable expected count (%s)", name)
			}
		})
	}
}

// The cap is what stops a right-sized wrong album beating a wrong-sized right
// one. Without it the size term could outrank the album name.
func TestReleaseSizeScore_NeverExceedsTheCap(t *testing.T) {
	track := albumTrackOf(11, 1)

	for _, n := range []int{0, 1, 11, 21, 200} {
		if got := releaseSizeScore(releaseDir(n), track, PreferSmallerRelease); got > maxReleaseSizeScore {
			t.Errorf("score for %d files = %d, above the cap of %d", n, got, maxReleaseSizeScore)
		}
	}
}

// End to end through the scorer the album flow actually calls.
func TestScoreDirWithPreference_RightSizedWrongAlbumStillLoses(t *testing.T) {
	track := albumTrackOf(11, 1)

	client := prefClient(PreferNone)
	client.Cfg.ReleasePreference = PreferSmallerRelease

	right := releaseDir(21) // the deluxe, but the correct album

	wrong := releaseDir(11) // exactly the right length, wrong album entirely
	wrong.dir = "Some Other Compilation"

	if client.scoreDirWithPreference(wrong, track) >= client.scoreDirWithPreference(right, track) {
		t.Error("a right-sized wrong album beat a wrong-sized right album")
	}
}

func TestScoreDirWithPreference_SmallerPicksTheStandardEdition(t *testing.T) {
	track := albumTrackOf(11, 1)

	client := prefClient(PreferNone)
	client.Cfg.ReleasePreference = PreferSmallerRelease

	standard := releaseDir(11)
	deluxe := releaseDir(21)

	if client.scoreDirWithPreference(standard, track) <= client.scoreDirWithPreference(deluxe, track) {
		t.Error("the deluxe edition beat the standard album")
	}

	// And the default still does the opposite.
	client.Cfg.ReleasePreference = PreferFullerRelease
	if client.scoreDirWithPreference(deluxe, track) <= client.scoreDirWithPreference(standard, track) {
		t.Error("with the default preference the fuller release did not win")
	}
}

func TestNormaliseReleasePreference(t *testing.T) {
	cases := map[string]string{
		"smaller":  PreferSmallerRelease,
		"SMALLER":  PreferSmallerRelease,
		" fuller ": PreferFullerRelease,
		"":         PreferFullerRelease,
		"nonsense": PreferFullerRelease,
	}
	for in, want := range cases {
		if got := normaliseReleasePreference(in); got != want {
			t.Errorf("normaliseReleasePreference(%q) = %q, want %q", in, got, want)
		}
	}
}

// The scorer has the same deluxe-match problem as the trim: bailing on the
// matched release's DiscTotal hands a padded directory the fuller-release score.
func TestExpectedTrackCount_ConsensusBeatsADeluxeMatch(t *testing.T) {
	expected, ok := expectedTrackCount(models.Track{
		TrackTotal: 23, DiscTotal: 2,
		CanonicalTrackTotal: 16, CanonicalDiscTotal: 1,
	})

	if !ok {
		t.Fatal("expectedTrackCount() gave up despite a single-disc consensus")
	}
	if expected != 16 {
		t.Errorf("expected = %d, want the consensus 16", expected)
	}
}

// A genuine double album must still bail: its first-medium count cannot measure
// a complete two-disc directory.
func TestExpectedTrackCount_StillBailsOnARealDoubleAlbum(t *testing.T) {
	if _, ok := expectedTrackCount(models.Track{
		TrackTotal: 13, DiscTotal: 2,
		CanonicalTrackTotal: 13, CanonicalDiscTotal: 2,
	}); ok {
		t.Error("expectedTrackCount() answered for a 2-disc consensus; the complete release would be punished")
	}
}
