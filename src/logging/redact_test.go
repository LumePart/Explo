package logging

import (
	"strings"
	"testing"
)

// The whole point of redaction: a shared log must never contain the
// secret, in any format the codebase emits.
func TestRedactLine(t *testing.T) {
	secret := "hunter2-SECRET"
	tests := []struct {
		name  string
		input string
	}{
		{"bare kv", `api_key=` + secret},
		{"no-underscore key", `ADMIN_SYSTEM_APIKEY=` + secret},
		{"quoted value", `password="` + secret + `"`},
		{"single-quoted value", `token='` + secret + `'`},
		{"query param", `https://slskd.example/download?api_key=` + secret + `&x=1`},
		{"amp param", `url="https://x/?a=1&token=` + secret + `"`},
		{"credential", `credential=` + secret},
		{"header style", `X-Plex-Token=` + secret},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactLine(tt.input)
			if strings.Contains(got, secret) {
				t.Errorf("secret survived redaction: %q", got)
			}
		})
	}

	if got := RedactLine(`monkey=banana`); got != `monkey=banana` {
		t.Errorf("ordinary key masked: %q", got)
	}
}

func TestRedactLine_KeepsOrdinaryContent(t *testing.T) {
	in := `time=2026-10-20 msg="[monitor] download errored" service=slskd title="Hot Blooded" state="Completed, Errored"`
	if got := RedactLine(in); got != in {
		t.Errorf("ordinary line changed:\n got: %q\nwant: %q", got, in)
	}
}

func TestRedactWriter_FragmentsAndLines(t *testing.T) {
	var out strings.Builder
	w := NewRedactWriter(&out)
	// Fragments must not leak: nothing is emitted before the newline,
	// and the secret is masked once the line completes.
	if _, err := w.Write([]byte(`token=abc`)); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("emitted %q before newline", out.String())
	}
	if _, err := w.Write([]byte("123\nnext line\n")); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "abc123") || !strings.Contains(got, "token=••••••••") || !strings.Contains(got, "next line\n") {
		t.Errorf("writer output = %q", got)
	}
}
