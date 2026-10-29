//go:build !unix

package run

import "os/exec"

// setProcessGroup is a no-op where process groups are unavailable; the
// default exec.CommandContext cancellation (kill the child) applies.
func setProcessGroup(*exec.Cmd) {}
