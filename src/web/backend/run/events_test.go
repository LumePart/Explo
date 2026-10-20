package run

import (
	"strings"
	"testing"
)

// BUG-19: bufio.Scanner's 64KB line limit stopped draining run output,
// so one long line blocked the child on a full pipe, cmd.Wait never
// returned, and every later run was refused as already running.
func collectLines(t *testing.T, input string) []string {
	t.Helper()
	var lines []string
	if err := scanRunLines(strings.NewReader(input), func(line string) {
		lines = append(lines, line)
	}); err != nil {
		t.Fatalf("scanRunLines: %v", err)
	}
	return lines
}

func TestScanRunLines_LongLineDoesNotStopDraining(t *testing.T) {
	long := strings.Repeat("x", 200*1024) // 3x the old 64KB scanner limit
	lines := collectLines(t, "first\n"+long+"\nafter\n")

	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 — draining stopped at the long line", len(lines))
	}
	if lines[0] != "first" || lines[2] != "after" {
		t.Errorf("lines around the long one = %q, %q", lines[0], lines[2])
	}
	if lines[1] != long {
		t.Errorf("long line kept %d bytes, want it intact under the cap", len(lines[1]))
	}
}

func TestScanRunLines_HugeLineIsTruncatedButDrainingContinues(t *testing.T) {
	huge := strings.Repeat("y", maxRunLineBytes+5000)
	lines := collectLines(t, huge+"\nnext")

	if len(lines) != 2 || lines[1] != "next" {
		t.Fatalf("lines = %d entries, want the truncated line plus 'next'", len(lines))
	}
	if !strings.HasSuffix(lines[0], "...[truncated]") {
		t.Error("huge line was not marked truncated")
	}
}

func TestScanRunLines_PreservesEmptyLinesAndPartialTail(t *testing.T) {
	lines := collectLines(t, "a\n\nb")
	if len(lines) != 3 || lines[0] != "a" || lines[1] != "" || lines[2] != "b" {
		t.Errorf("lines = %q, want [a '' b]", lines)
	}
}
