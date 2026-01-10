package util

import "regexp"

var (
	filenameRe = regexp.MustCompile(`[^\p{L}\d._,\-]+`)
	alnumRe    = regexp.MustCompile(`[^\p{L}\d]+`)
)

// FilenameSafe replaces characters unsafe for filenames with '_'
func FilenameSafe(s string) string {
	return filenameRe.ReplaceAllString(s, "_")
}

// AlnumOnly removes everything except letters and digits
func AlnumOnly(s string) string {
	return alnumRe.ReplaceAllString(s, "")
}