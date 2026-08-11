//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The process-tree reads behind sessionKey and agentName, Windows edition. Same
// contract as proc_linux.go (read it there), met by two different calls because
// Windows keeps the two facts in two places.
//
// The name is the executable's, with .exe trimmed, so PlaceholderAgent recognises
// the same shells and wrappers it does everywhere: cmd, powershell, sh, bash.

// procParent walks a toolhelp snapshot to find one pid's entry. A snapshot is a
// copy taken at one instant, which is the same guarantee-free ground the procfs
// walk stands on - hence the bounded loop in agentProcessFrom.
func procParent(pid int) (comm string, ppid int, err error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", 0, err
	}
	defer windows.CloseHandle(snap)

	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if int(e.ProcessID) != pid {
			continue
		}
		name := windows.UTF16ToString(e.ExeFile[:])
		name = strings.TrimSuffix(filepath.Base(name), ".exe")
		return name, int(e.ParentProcessID), nil
	}
	if err != nil {
		return "", 0, err
	}
	return "", 0, fmt.Errorf("no process %d in snapshot", pid)
}

// procStartTime uses the creation time, which is exactly the property the contract
// wants: it is set once when the process starts and a recycled pid gets a new one.
// SYNCHRONIZE is not enough to read times, so this asks for the limited query
// right - the one that works across integrity levels.
func procStartTime(pid int) (int64, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0, err
	}
	return creation.Nanoseconds(), nil
}
