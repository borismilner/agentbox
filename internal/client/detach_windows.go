//go:build windows

package client

import (
	"os/exec"

	"golang.org/x/sys/windows"
)

// detach is Setsid's Windows equivalent, and it takes two flags because Windows
// splits what setsid does into two ideas:
//
//   - DETACHED_PROCESS gives the daemon no console, so it neither inherits the
//     spawning shell's console nor pops one of its own. Without it a
//     console-spawned daemon dies with that console's window.
//   - CREATE_NEW_PROCESS_GROUP takes it out of the group Ctrl-C is delivered to,
//     which is the other half of losing the controlling terminal.
//
// Same guarantee as the unix build: the daemon outlives whichever tool call
// happened to be the first to need it (ADR-0003).
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}
