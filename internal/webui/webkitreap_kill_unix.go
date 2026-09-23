//go:build !windows

package webui

import "syscall"

// killProcess is the reaper's one platform call. SIGKILL and nothing softer:
// the pid may be a pid namespace's init, which drops every other signal sent
// from outside its namespace (reapStrayWebkitChildren has the citation).
func killProcess(pid int) error { return syscall.Kill(pid, syscall.SIGKILL) }
