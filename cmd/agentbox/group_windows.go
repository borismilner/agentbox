//go:build windows

package main

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

// ownGroup and signalGroup on Windows. The GOAL is the one that matters and it is
// met: a wrapper that is being torn down does not leave the command tree running
// and the lock held. How it gets there is blunter, and the difference is written
// down rather than papered over.
//
// CREATE_NEW_PROCESS_GROUP is the Setpgid half and it works the same way - the
// command and its children form a group of their own.
//
// The signalling half does not. Windows has no SIGTERM to pass on: the closest
// thing is a console control event, which only reaches processes that share a
// console, and a wrapper started by an agent may have none. So signalGroup kills
// the top process instead. A child that outlives its parent can survive that,
// which is the honest gap; the lock is still released either way, because the
// wrapper's release runs on both branches of the wait. Closing the gap properly
// means a job object with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, which is a real
// improvement and a separate piece of work.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func signalGroup(p *os.Process, sig os.Signal) error {
	return p.Kill()
}
