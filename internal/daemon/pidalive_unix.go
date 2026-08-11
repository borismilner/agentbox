//go:build unix

package daemon

import (
	"errors"
	"syscall"
)

// Signal 0 is the portable "does this pid exist" probe on every unix: the kernel
// runs the permission check and the deliverability check and sends nothing.
//
// EPERM counts as alive, and that is the decision rather than an accident - see
// pidAlive's contract in locks.go. It happens for a pid owned by another user,
// which on this machine means a daemon somebody else's session started.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
