package webui

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// webkitReapGrace is how long a WebKit process gets to exit on its own after
// its window is closed before this package assumes Wails v3's GTK4 teardown
// has stalled and kills it directly.
//
// Confirmed live 2026-09-18: closing a card destroys the GtkWindow (the
// default WindowClosing handler wails installs unconditionally calls
// gtk_window_destroy) but the sandboxed WebKitWebProcess, its bwrap wrapper
// and the shared WebKitNetworkProcess's xdg-dbus-proxy can outlive it
// indefinitely - github.com/wailsapp/wails#2565, still present byte-for-byte
// in the pinned v3.0.0-alpha2.117 and in v3.0.0-beta.19's identical
// linux_cgo.go destroy() path, so bumping the dependency alone will not fix
// it. A toast's own default dismiss is 6-8s; 3s of extra grace on top of
// that is generous room for a slow-but-genuine teardown before this treats
// the process as stuck.
const webkitReapGrace = 3 * time.Second

// webkitFamily names the process tree WebKitGTK spawns per view: one shared
// network process, one sandboxed web process, and the two bwrap/
// xdg-dbus-proxy wrappers the sandbox puts around the web process.
func webkitFamily(comm string) bool {
	switch comm {
	case "WebKitNetworkProcess", "WebKitWebProcess", "bwrap", "xdg-dbus-proxy":
		return true
	}
	return false
}

// webkitChildren returns the pids of this process's direct children that are
// part of a WebKitGTK view. Call it once before a window is created and once
// after (diffWebkitChildren the two) to name exactly that window's own
// processes - never a sibling window's, which is what makes
// reapStrayWebkitChildren safe to act on unconditionally.
func webkitChildren() map[int]bool {
	out := map[int]bool{}
	self := os.Getpid()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		ppid, comm := procParent(pid)
		if ppid == self && webkitFamily(comm) {
			out[pid] = true
		}
	}
	return out
}

// diffWebkitChildren returns the pids in after that were not in before.
func diffWebkitChildren(before, after map[int]bool) map[int]bool {
	out := map[int]bool{}
	for pid := range after {
		if !before[pid] {
			out[pid] = true
		}
	}
	return out
}

// procParent reads a pid's parent pid and command name out of
// /proc/<pid>/stat. Both are zero/empty once the pid is gone or reused by an
// unrelated process, which reapStrayWebkitChildren relies on to tell "exited
// cleanly" from "still stuck".
func procParent(pid int) (ppid int, comm string) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, ""
	}
	// Format is "pid (comm) state ppid ...". comm can itself contain spaces
	// or parens, so split on the LAST ')' rather than on fields.
	s := string(data)
	end := strings.LastIndexByte(s, ')')
	open := strings.IndexByte(s, '(')
	if end < 0 || open < 0 || open >= end {
		return 0, ""
	}
	comm = s[open+1 : end]
	fields := strings.Fields(s[end+2:])
	if len(fields) < 2 {
		return 0, comm
	}
	ppid, _ = strconv.Atoi(fields[1])
	return ppid, comm
}

// reapStrayWebkitChildren waits webkitReapGrace for the pids in before (a
// window's own webkitChildren diff, captured at creation) to exit on their
// own, then SIGTERMs whichever are still alive and still WebKit-family. It
// only ever acts on pids a specific window's own creation produced, so a
// still-open sibling window's process is never a candidate - there is no
// sweep of the daemon's whole process tree here.
func reapStrayWebkitChildren(before map[int]bool, log *slog.Logger) {
	if len(before) == 0 {
		return
	}
	go func() {
		time.Sleep(webkitReapGrace)
		survivors := 0
		for pid := range before {
			ppid, comm := procParent(pid)
			if ppid == 0 || !webkitFamily(comm) {
				continue // exited on its own, or the pid was reused by something else
			}
			survivors++
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
		if survivors > 0 && log != nil {
			log.Warn("webui.webkit_process_reaped", "component", "webui",
				"count", survivors, "grace_s", int(webkitReapGrace.Seconds()))
		}
	}()
}
