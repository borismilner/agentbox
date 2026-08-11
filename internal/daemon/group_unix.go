//go:build unix

package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

// ownGroup and killGroup are what make an action command's deadline reach the
// whole tree it started. The contract is stated once at the call site
// (execAction): a command that is out of time must leave nothing behind, and
// signalling the shell alone leaves its children running with the deadline
// already spent on them.
//
// Setpgid puts the command in a group of its own; the negative pid signals that
// whole group, so `sh -c 'sleep 600 &'` dies with the sleep rather than beside
// it.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killGroup(p *os.Process) error {
	if p == nil {
		return nil
	}
	// The group leader's pid is the process's own, because Setpgid made it one.
	// A failure here is the group already being gone, which is the outcome asked
	// for, so it falls back to the single process rather than being reported.
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return p.Kill()
}
