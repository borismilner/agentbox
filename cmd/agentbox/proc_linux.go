//go:build linux

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The process-tree reads behind sessionKey and agentName, Linux edition. procfs is
// the cheapest and most exact answer here: two small file reads, no handle to
// leak, and a start time that comes straight off the boot clock.
//
// The contract these two owe their callers - and it is the same on every platform:
//
//   - procParent answers this pid's own name and its parent's pid, so
//     agentProcessFrom can walk past the shells and wrappers to the agent.
//   - procStartTime answers something that changes when a pid is REUSED. Any
//     monotonic per-process stamp will do; the number itself is never compared
//     across machines, only against the same pid seen again.

// procParent is one step up the tree: this pid's name and its parent's pid.
func procParent(pid int) (comm string, ppid int, err error) {
	c, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "", 0, err
	}
	// The name is read from comm rather than from stat, whose own comm field is
	// parenthesised and may itself contain parentheses or spaces.
	p, err := procStatField(pid, 4) // ppid
	if err != nil {
		return "", 0, err
	}
	return strings.TrimSpace(string(c)), p, nil
}

// procStartTime is the boot-clock tick a process started on. It is what makes a
// pid safe to name a session after: pids are recycled, and without the start time
// a new process landing on a dead agent's number would inherit its locks and its
// claims.
func procStartTime(pid int) (int64, error) {
	v, err := procStatField(pid, 22) // starttime
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

// procStatField reads one numeric field of /proc/PID/stat by its documented
// 1-based number (proc(5)).
//
// Fields 1 and 2 are the pid and the parenthesised name, and that name may itself
// contain parentheses or spaces - so the line is cut at its LAST close paren and
// counted from there rather than split whole.
func procStatField(pid, field int) (int, error) {
	st, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	rest := string(st)
	if i := strings.LastIndex(rest, ")"); i >= 0 {
		rest = rest[i+1:]
	}
	f := strings.Fields(rest)
	i := field - 3 // f[0] is field 3, the state
	if i < 0 || i >= len(f) {
		return 0, fmt.Errorf("unreadable stat for %d: no field %d", pid, field)
	}
	return strconv.Atoi(f[i])
}
