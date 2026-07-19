package util

import (
	"regexp"
	"strings"
)

var (
	filenameRe     = regexp.MustCompile(`[^\p{L}\d._,\-]+`)
	alnumRe        = regexp.MustCompile(`[^\p{L}\d]+`)
	
	// Captures the parts before and after a featuring clause
	featSplitRe    = regexp.MustCompile(`(?i)\s+(?:feat\.?|featuring|ft\.?|with)\s+`)
	
	// Existing regexes kept for compatibility
	featTailRe     = regexp.MustCompile(`(?i)\s*[\(\[\{]\s*(feat\.?|featuring|ft\.?|with)\s[^\)\]\}]*[\)\]\}]\s*$`)
	featBareRe     = regexp.MustCompile(`(?i)\s+(feat\.?|featuring|ft\.?)\s+.*$`)
	remasterTailRe = regexp.MustCompile(`(?i)\s*[-–—]\s*\d{4}\s*remaster(ed)?\s*$`)
	
	// Clean up internal brackets inside featuring clauses if parenthesized: "(feat. Rahzel)" -> "Rahzel"
	bracketTrimRe  = regexp.MustCompile(`[()\[\]{}]`)
)

// ConvertFeatToSemicolon rewrites "Artist A feat. Artist B" to "Artist A; Artist B"
// This aligns string formatting directly with Jellyfin's multi-artist tracking schema.
func ConvertFeatToSemicolon(s string) string {
	if !featSplitRe.MatchString(s) && !featTailRe.MatchString(s) {
		return strings.TrimSpace(s)
	}
	
	// Handle parenthesized features like: Bring Me the Horizon (feat. Rahzel)
	if featTailRe.MatchString(s) {
		cleaned := bracketTrimRe.ReplaceAllString(s, "")
		parts := featSplitRe.Split(cleaned, 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[0]) + "; " + strings.TrimSpace(parts[1])
		}
	}

	// Handle bare features like: Bring Me the Horizon feat. Rahzel
	parts := featSplitRe.Split(s, 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]) + "; " + strings.TrimSpace(parts[1])
	}
	
	return strings.TrimSpace(s)
}

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

// NormalizeArtist normalizes featuring clauses to semicolons instead of dropping them,
// ensuring accurate database alignment for tracks with multi-artist records.
func NormalizeArtist(s string) string {
	s = ConvertFeatToSemicolon(s)
	return strings.ToLower(s)
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
