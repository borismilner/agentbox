//go:build unix

package server

import (
	"os"
	"syscall"
)

// The single-instance lock (NFR12, ADR-0003). flock is the right primitive on
// every unix: the kernel drops it when the holder's last descriptor closes, so a
// daemon that was killed rather than stopped does not leave a lock nobody can
// clear. That property is the whole reason the socket cleanup is allowed to
// happen after the lock and not before.
//
// ErrAlreadyRunning rather than the raw errno, so Listen does not have to know
// that EWOULDBLOCK is what a taken flock looks like - and so the Windows half can
// answer the same question with a completely different call.
func lockFile(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if err == syscall.EWOULDBLOCK {
			return ErrAlreadyRunning
		}
		return err
	}
	return nil
}

func unlockFile(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
