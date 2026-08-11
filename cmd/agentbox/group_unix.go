//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// ownGroup and signalGroup are the pair that makes `agentbox sync lock NAME -- CMD`
// release its hold on the way out.
//
// An agent's foreground shell call is SIGTERMed at 120s, so a wrapper that only
// signalled its top child would leave the command tree running and the hold behind
// it - on every command longer than two minutes. Setpgid puts the command in a
// group of its own and the negative pid signals that whole group, so a make that
// spawned a compiler that spawned a linker all get the signal the wrapper got.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalGroup(p *os.Process, sig os.Signal) error {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return p.Kill()
	}
	return syscall.Kill(-p.Pid, s)
}
