//go:build unix

package client

import (
	"os/exec"
	"syscall"
)

// detach puts the spawned daemon in its own session, which is what makes
// auto-spawn (ADR-0003) safe: the first agent to call a tool starts the daemon, and
// that daemon must not die when the shell or the tool call that happened to be
// first goes away. Setsid also drops the controlling terminal, so a Ctrl-C in the
// spawning shell is not delivered to it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
