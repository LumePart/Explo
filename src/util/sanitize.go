package util

import (
	"regexp"
	"strings"
)

var (
	filenameRe     = regexp.MustCompile(`[^\p{L}\d._,\-]+`)
	alnumRe        = regexp.MustCompile(`[^\p{L}\d]+`)
	featTailRe     = regexp.MustCompile(`(?i)\s*[\(\[\{]\s*(feat\.?|featuring|ft\.?|with)\s[^\)\]\}]*[\)\]\}]\s*$`)
	featBareRe     = regexp.MustCompile(`(?i)\s+(feat\.?|featuring|ft\.?)\s+.*$`)
	remasterTailRe = regexp.MustCompile(`(?i)\s*[-–—]\s*\d{4}\s*remaster(ed)?\s*$`)
)

// StripFeat removes a trailing "feat./ft./featuring …" clause, whether
// parenthesized "(feat. X)" or bare " feat. X". Works for titles and artists.
func StripFeat(s string) string {
	s = featTailRe.ReplaceAllString(s, "")
	s = featBareRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// NormalizeTitle strips feat annotations and "- YYYY Remaster" suffixes,
// lowercases, and reduces to alphanumeric-only for fuzzy comparison.
func NormalizeTitle(s string) string {
	s = StripFeat(s)
	s = remasterTailRe.ReplaceAllString(s, "")
	return AlnumOnly(strings.ToLower(s))
}

// NormalizeArtist strips a trailing feat clause and reduces to
// alphanumeric-only, so "Rihanna feat. Eminem" and "Rihanna" compare equal.
func NormalizeArtist(s string) string {
	return AlnumOnly(strings.ToLower(StripFeat(s)))
}

// CleanSearchTitle strips trailing (feat. …) and "- 2011 Remaster" suffixes
// but keeps the title human-readable for use in search API queries.
func CleanSearchTitle(s string) string {
	s = featTailRe.ReplaceAllString(s, "")
	s = remasterTailRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// FilenameSafe replaces characters unsafe for filenames with '_'
func FilenameSafe(s string) string {
	return filenameRe.ReplaceAllString(s, "_")
}

// AlnumOnly removes everything except letters and digits
func AlnumOnly(s string) string {
	return alnumRe.ReplaceAllString(s, "")
}

// Case insensitive check if substring is present in s
func ContainsFold(s, substr string) bool {
    return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
