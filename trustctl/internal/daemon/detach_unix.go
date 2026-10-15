//go:build !windows

package daemon

import (
	"os/exec"
	"syscall"
)

// detach starts the process in its own session, so closing the terminal does not kill it.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
