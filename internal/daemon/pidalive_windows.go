//go:build windows

package daemon

import "golang.org/x/sys/windows"

// pidAlive on Windows. Same contract as the unix build (pidalive_unix.go): a
// process we cannot touch counts as ALIVE, because an orphaned lock freed under a
// process that is still working is worse than one that waits.
//
// There is no signal 0 here, so the probe is "can this pid be opened at all".
// SYNCHRONIZE is deliberately the weakest right that answers the question - asking
// for PROCESS_QUERY_INFORMATION would fail on a process running at a higher
// integrity level and report a live daemon as dead, which is the one wrong answer
// this function must not give.
//
// A pid that opens but has already exited is caught by the wait: a signalled
// process object means the process is gone and the handle is only outliving it.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// ERROR_ACCESS_DENIED is a process that exists and is not ours; anything
		// else (ERROR_INVALID_PARAMETER for a pid with no process) is death.
		return err == windows.ERROR_ACCESS_DENIED
	}
	defer windows.CloseHandle(h)
	state, err := windows.WaitForSingleObject(h, 0)
	return err != nil || state != windows.WAIT_OBJECT_0
}
