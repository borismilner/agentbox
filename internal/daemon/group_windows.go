//go:build windows

package daemon

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

// ownGroup and killGroup on Windows, with the same contract as the unix build
// (group_unix.go) and the same honest gap the CLI's pair records
// (cmd/agentbox/group_windows.go): CREATE_NEW_PROCESS_GROUP is the Setpgid half
// and does the same job, but there is no signal that reaches a group here, so a
// command that is out of time loses its top process and a child that outlived
// its parent can survive. Closing that properly means a job object with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, which is a separate piece of work.
//
// The action path is unix-shaped anyway - execAction runs `sh -c` - so this side
// exists to keep the daemon compiling and to state where it stops being true.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func killGroup(p *os.Process) error {
	if p == nil {
		return nil
	}
	return p.Kill()
}
