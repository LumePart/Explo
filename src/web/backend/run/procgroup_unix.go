//go:build unix

package run

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the run in its own process group and makes
// cancellation kill the whole group, so `--user all` grandchildren stop too.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
