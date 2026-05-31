package util

import (
	"regexp"
	"strings"
)

var (
	filenameRe    = regexp.MustCompile(`[^\p{L}\d._,\-]+`)
	alnumRe       = regexp.MustCompile(`[^\p{L}\d]+`)
	featTailRe    = regexp.MustCompile(`(?i)\s*[\(\[\{]\s*(feat\.?|featuring|ft\.?|with)\s[^\)\]\}]*[\)\]\}]\s*$`)
	remasterTailRe = regexp.MustCompile(`(?i)\s*[-–—]\s*\d{4}\s*remaster(ed)?\s*$`)
)

// CleanSearchTitle strips trailing (feat. …) and "- 2011 Remaster" suffixes
// but keeps the title human-readable for use in search API queries.
func CleanSearchTitle(s string) string {
	s = featTailRe.ReplaceAllString(s, "")
	s = remasterTailRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// NormalizeTitle strips trailing (feat. …) annotations, lowercases,
// and reduces to alphanumeric-only for fuzzy title comparison.
func NormalizeTitle(s string) string {
	s = featTailRe.ReplaceAllString(s, "")
	s = remasterTailRe.ReplaceAllString(s, "")
	return AlnumOnly(strings.ToLower(s))
}

// FilenameSafe replaces characters unsafe for filenames with '_'
func FilenameSafe(s string) string {
	return filenameRe.ReplaceAllString(s, "_")
}

// AlnumOnly removes everything except letters and digits
func AlnumOnly(s string) string {
	return alnumRe.ReplaceAllString(s, "")
}