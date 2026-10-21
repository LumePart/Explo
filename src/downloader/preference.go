package downloader

import (
	"cmp"

	"explo/src/models"
	"slices"
	"strings"
)

// Size preferences. Ranking, not filtering: everything inside the configured
// quality range stays eligible, the preference only decides what is reached
// for first.
const (
	// PreferNone leaves ordering to the order of EXTENSIONS, which is how
	// Explo has always chosen, so an upgrade changes nothing.
	PreferNone = "none"
	// PreferSmaller takes the best lossy file available and only falls back to
	// lossless when there is no lossy candidate at all. Note that this is not
	// "smallest": within the lossy band it still wants the highest bitrate,
	// which is the whole point -- a 320 beats a 256 beats a FLAC.
	PreferSmaller = "smaller"
	// PreferLarger is the reverse, and reduces to plain highest-bitrate-first.
	PreferLarger = "larger"
)

// normalisePreference tolerates casing and blank values so a hand-edited .env
// cannot break a run over a capital letter.
func normalisePreference(pref string) string {
	switch strings.ToLower(strings.TrimSpace(pref)) {
	case PreferSmaller:
		return PreferSmaller
	case PreferLarger:
		return PreferLarger
	default:
		return PreferNone
	}
}

// bandRank groups a file for the preference. Lower sorts first.
//
// The two settings are not mirror images. "Larger" is satisfied by sorting on
// bitrate alone, while "smaller" needs the band, because wanting a small file
// does not mean wanting a bad one.
func bandRank(file File, pref string) int {
	switch pref {
	case PreferSmaller:
		if isLossless(file) {
			return 1
		}
		return 0
	case PreferLarger:
		if isLossless(file) {
			return 0
		}
		return 1
	default:
		return 0
	}
}

// compareFiles orders two candidates under a preference. Returns a negative
// number when a should be tried before b.
func (c Slskd) compareFiles(a, b File, pref string) int {
	if pref == PreferNone {
		// Preserve the historic behaviour exactly: the position of a file's
		// extension in EXTENSIONS is the preference, and nothing else is.
		return cmp.Compare(c.extensionRank(a), c.extensionRank(b))
	}

	if r := cmp.Compare(bandRank(a, pref), bandRank(b, pref)); r != 0 {
		return r
	}

	// Within a band, better is always better -- for both settings.
	rateA, rateB := effectiveBitRate(a), effectiveBitRate(b)

	// A file nobody described is eligible but never preferred: we cannot say
	// it is the best of its band without knowing what it is.
	switch {
	case rateA == 0 && rateB == 0:
		return 0
	case rateA == 0:
		return 1
	case rateB == 0:
		return -1
	}

	return cmp.Compare(rateB, rateA)
}

// extensionRank is a file's position in the configured extension list.
// Anything unlisted sorts last, though the caller has already filtered those
// out.
func (c Slskd) extensionRank(file File) int {
	if i := slices.Index(c.Cfg.Filters.Extensions, file.Extension); i >= 0 {
		return i
	}
	return len(c.Cfg.Filters.Extensions)
}

// sortByPreference orders candidates in place. The sort is stable, so files
// that rank equally keep the order the search returned them in.
func (c Slskd) sortByPreference(files []File) {
	pref := normalisePreference(c.Cfg.SizePreference)
	slices.SortStableFunc(files, func(a, b File) int {
		return c.compareFiles(a, b, pref)
	})
}

// releaseBitRate summarises a release's quality as the median of its files'
// bitrates. The median rather than the mean, so that one 128 kbps bonus track
// cannot drag down an otherwise clean release. Files of unknown bitrate are
// left out; a release where nothing is known returns 0.
func releaseBitRate(files []File) int {
	rates := make([]int, 0, len(files))
	for _, file := range files {
		if rate := effectiveBitRate(file); rate > 0 {
			rates = append(rates, rate)
		}
	}

	if len(rates) == 0 {
		return 0
	}

	slices.Sort(rates)
	return rates[len(rates)/2]
}

// preferenceScore is what a release's quality contributes to its ranking.
//
// Deliberately smaller than either name-matching term in scoreDir: quality
// breaks a tie between releases that match the recommendation equally well and
// is never a reason to take a worse match. A wrong-album FLAC must still lose
// to a right-album MP3.
const preferenceScore = 20

// scorePreference rewards a release for sitting in the preferred band. It
// returns 0 for PreferNone, leaving release scoring exactly as it was.
func scorePreference(files []File, pref string) int {
	if pref == PreferNone {
		return 0
	}

	rate := releaseBitRate(files)
	if rate == 0 {
		return 0
	}

	// One file is enough to characterise the release: a directory of FLACs is
	// lossless whatever its median bitrate works out to.
	lossless := false
	for _, file := range files {
		if isLossless(file) {
			lossless = true
			break
		}
	}

	wantLossless := pref == PreferLarger
	if lossless == wantLossless {
		return preferenceScore
	}
	return 0
}

// Release preferences. Where SizePreference is about how good each file is,
// this is about how much of the release to take.
const (
	// PreferFullerRelease takes the most complete directory: the historic
	// default, still the fallback whenever the real track count is unknown.
	PreferFullerRelease = "fuller"
	// PreferSmallerRelease reaches for the release closest to the album's real
	// track count, which is what keeps a 21-track deluxe edition from being
	// downloaded in place of the 11-track album.
	//
	// Deliberately "closest" and not "fewest": a directory holding only the
	// recommended track would win every time under fewest, and album mode
	// would quietly stop fetching albums at all.
	PreferSmallerRelease = "smaller"
)

// maxReleaseSizeScore caps every size term. Below the album (100) and artist
// (50) terms in scoreDir, so being the right size can decide between releases
// that match equally well but can never win against a better match.
const maxReleaseSizeScore = 40

// releaseSizeMissPenalty is how much each track of difference from the
// expected count costs, so a release ten tracks out scores nothing.
const releaseSizeMissPenalty = 4

func normaliseReleasePreference(pref string) string {
	if strings.EqualFold(strings.TrimSpace(pref), PreferSmallerRelease) {
		return PreferSmallerRelease
	}
	return PreferFullerRelease
}

// expectedTrackCount reports how many files a correct release should hold, and
// whether that is knowable at all.
//
// The count comes from MusicBrainz during enrichment and covers the first
// medium only. A multi-disc release therefore legitimately holds more files
// than that, so a closeness score would punish the complete release for being
// complete -- better to admit we cannot tell.
//
// Which disc count decides that is the subtle part. The matched release may be
// a two-disc deluxe of a single-disc album, and bailing on its DiscTotal would
// hand the padded directory the fuller-release score it should have lost. The
// release group's consensus is asked first and the matched release only when
// there is none, so a genuine double album still bails while a deluxe-edition
// mismatch does not.
func expectedTrackCount(track models.Track) (int, bool) {
	expected := expectedAlbumLength(track)
	if expected <= 0 {
		return 0, false
	}
	if discs := track.CanonicalDiscTotal; discs > 0 {
		if discs > 1 {
			return 0, false
		}
		return expected, true
	}
	if track.DiscTotal > 1 {
		return 0, false
	}
	return expected, true
}

// oversizedReleaseCap bounds the strict selection pass: a candidate holding
// more than this multiple of the expected track count is a compilation dump,
// not the album (BUG-35: a 64-file folder won a 10-track album). Deluxe
// padding stays well under it, and anything bigger still wins when nothing
// better exists -- the cap only decides between candidates, never eligibility
// outright.
const oversizedReleaseCap = 2

// oversizedDir reports whether dir holds far more files than the album should
// have. Without a usable expected count it never fires: same unknown-count
// cases as releaseSizeScore, same refusal to guess.
func oversizedDir(dir peerDir, track models.Track) bool {
	expected, ok := expectedTrackCount(track)
	if !ok {
		return false
	}
	return len(dir.files) > oversizedReleaseCap*expected
}

// releaseSizeScore rewards a release for being the size it ought to be.
//
// Without a usable expected count -- enrichment off, the lookup failed, or a
// multi-disc release -- it falls back to preferring the fuller release rather
// than guessing. Guessing is what would reopen the single-track hole: only
// when the real track count is known can a lone file be recognised as either a
// genuine single or a fragment of an album.
func releaseSizeScore(dir peerDir, track models.Track, pref string) int {
	if pref != PreferSmallerRelease {
		return fullerReleaseScore(dir)
	}

	expected, ok := expectedTrackCount(track)
	if !ok {
		return fullerReleaseScore(dir)
	}

	diff := len(dir.files) - expected
	if diff < 0 {
		diff = -diff
	}

	return max(0, maxReleaseSizeScore-diff*releaseSizeMissPenalty)
}
