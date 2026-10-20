package logging

import (
	"io"
	"regexp"
)

// Secret values must never reach the log file or stderr: shared logs are
// a debugging artifact, and URLs, headers and client configs carrying
// tokens flow through slog call sites all over the codebase.
var (
	// key=value pairs, quoted or bare: password=hunter2, "api_key"="x",
	// ADMIN_SYSTEM_APIKEY=y. The separator class (not \b) matches after
	// underscores while still not matching inside words like "monkey".
	sensitiveValueRE = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9])(password|passwd|token|secret|credential|api[-_]?key|access[-_]?key|auth[-_]?key|private[-_]?key)\s*=\s*("[^"]*"|'[^']*'|\S+)`)
	// Query strings: ?api_key=hunter2&x=1 or &token=abc.
	secretQueryRE = regexp.MustCompile(`(?i)([?&][^=&\s]*?(?:token|key|secret|password|auth)[^=&\s]*?=)[^&\s]*`)
)

// RedactLine masks secret values in one log line, preserving its shape.
func RedactLine(line string) string {
	line = sensitiveValueRE.ReplaceAllString(line, `$1$2=••••••••`)
	return secretQueryRE.ReplaceAllString(line, `${1}••••••••`)
}

// redactWriter is an io.Writer that redacts complete lines before
// passing them through. Writes are buffered until a newline because a
// slog record may arrive in fragments.
type redactWriter struct {
	w   io.Writer
	buf []byte
}

// NewRedactWriter wraps w so every line written through it is redacted.
func NewRedactWriter(w io.Writer) io.Writer {
	return &redactWriter{w: w}
}

func (r *redactWriter) Write(p []byte) (int, error) {
	r.buf = append(r.buf, p...)
	for {
		i := -1
		for j, b := range r.buf {
			if b == '\n' {
				i = j
				break
			}
		}
		if i < 0 {
			break
		}
		line := RedactLine(string(r.buf[:i]))
		if _, err := io.WriteString(r.w, line+"\n"); err != nil {
			return 0, err
		}
		r.buf = append([]byte(nil), r.buf[i+1:]...)
	}
	return len(p), nil
}
