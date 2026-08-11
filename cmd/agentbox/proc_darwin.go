//go:build darwin

package main

import (
	"strings"

	"golang.org/x/sys/unix"
)

// The process-tree reads behind sessionKey and agentName, macOS edition. Same
// contract as proc_linux.go, and it is met exactly: one sysctl answers the name,
// the parent and the start time together, where procfs needed two file reads.
//
// kern.proc.pid is the supported way to ask about a process you may not own, which
// matters for the same reason EPERM counts as alive in pidAlive - an agent started
// under a different login is still an agent.
func procParent(pid int) (comm string, ppid int, err error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return "", 0, err
	}
	return commOf(kp), int(kp.Eproc.Ppid), nil
}

// procStartTime is what makes a pid safe to name a session after. P_starttime is
// wall-clock, so it is folded to microseconds and used only as an opaque stamp -
// the number is compared against the same pid seen again and never read as a date,
// which is the whole of what the contract asks for.
func procStartTime(pid int) (int64, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, err
	}
	return kp.Proc.P_starttime.Sec*1_000_000 + int64(kp.Proc.P_starttime.Usec), nil
}

// commOf reads the fixed-width comm field, which is NUL-padded and truncated to 16
// characters by the kernel - the same 16 that procfs gives, so PlaceholderAgent's
// list of shells and wrappers matches without a second set of names.
func commOf(kp *unix.KinfoProc) string {
	raw := kp.Proc.P_comm[:]
	if i := strings.IndexByte(string(raw), 0); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(string(raw))
}
