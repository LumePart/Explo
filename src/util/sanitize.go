package util

import (
	"regexp"
	"strings"
)

var (
	filenameRe = regexp.MustCompile(`[^\p{L}\d._,\-]+`)
	alnumRe    = regexp.MustCompile(`[^\p{L}\d]+`)
	featTailRe = regexp.MustCompile(`(?i)\s*[\(\[\{]\s*(feat\.?|featuring|ft\.?|with)\s[^\)\]\}]*[\)\]\}]\s*$`)
)

// NormalizeTitle strips trailing (feat. …) annotations, lowercases,
// and reduces to alphanumeric-only for fuzzy title comparison.
func NormalizeTitle(s string) string {
	s = featTailRe.ReplaceAllString(s, "")
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