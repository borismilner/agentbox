//go:build windows

package server

import (
	"os"

	"golang.org/x/sys/windows"
)

// LockFileEx is Windows' flock, and it has the property that matters: a mandatory
// lock on a file handle, released by the kernel when the process dies. So a daemon
// that crashed does not leave a lock the next one has to guess about, which is the
// same guarantee the unix side relies on to let socket cleanup follow the lock.
//
// LOCKFILE_FAIL_IMMEDIATELY is the LOCK_NB half. Without it the second daemon
// would block instead of losing, and NFR12 wants the loser to exit cleanly.
func lockFile(f *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &overlapped)
	if err != nil {
		// ERROR_LOCK_VIOLATION is a lock somebody holds; anything else is a
		// genuine failure to ask.
		if err == windows.ERROR_LOCK_VIOLATION || err == windows.ERROR_IO_PENDING {
			return ErrAlreadyRunning
		}
		return err
	}
	return nil
}

func unlockFile(f *os.File) {
	var overlapped windows.Overlapped
	windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped)
}
