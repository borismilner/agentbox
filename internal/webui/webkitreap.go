package webui

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// webkitReapGrace is how long a WebKit process gets to exit on its own after
// its window is closed before this package assumes Wails v3's GTK4 teardown
// has stalled and kills it directly.
//
// Confirmed live 2026-09-18: closing a card destroys the GtkWindow (the
// default WindowClosing handler wails installs unconditionally calls
// gtk_window_destroy) but the sandboxed WebKitWebProcess, its bwrap wrappers
// and the xdg-dbus-proxy can outlive it indefinitely - github.com/wailsapp/
// wails#2565, still present byte-for-byte in the pinned v3.0.0-alpha2.117 and
// in v3.0.0-beta.19's identical linux_cgo.go destroy() path, so bumping the
// dependency alone will not fix it. A toast's own default dismiss is 6-8s; 3s
// of extra grace on top of that is generous room for a slow-but-genuine
// teardown before this treats the process as stuck.
const webkitReapGrace = 3 * time.Second

// webkitReapVerify is how long after the kill the reaper looks again before
// logging. The 2026-09-18 version logged "reaped" the moment it had sent a
// signal, and for six days that line covered eight renderers that never died.
const webkitReapVerify = 500 * time.Millisecond

// webkitFamily names the processes WebKitGTK spawns per view: one shared
// network process, one sandboxed web process, the xdg-dbus-proxy beside it,
// and the bwrap wrappers the sandbox puts around each of those two.
func webkitFamily(comm string) bool {
	switch comm {
	case "WebKitNetworkProcess", "WebKitWebProcess", "bwrap", "xdg-dbus-proxy":
		return true
	}
	return false
}

// procInfo is one process's parent and command name as /proc reports them.
type procInfo struct {
	ppid int
	comm string
}

// procTable reads every live process's parent and comm out of /proc in one
// pass. Empty where there is no /proc, which makes the reaper a no-op there.
func procTable() map[int]procInfo {
	out := map[int]procInfo{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		ppid, comm, _ := procStat(pid)
		if ppid == 0 && comm == "" {
			continue
		}
		out[pid] = procInfo{ppid: ppid, comm: comm}
	}
	return out
}

// webkitChildren returns the pids of every WebKit-family process below this
// one - not only its direct children. Call it once before a window is created
// and once after (diffWebkitChildren the two) to name exactly that window's
// own processes - never a sibling window's, which is what makes
// reapStrayWebkitChildren safe to act on unconditionally.
//
// The whole tree matters because of its shape. Measured on a live view,
// 2026-09-24:
//
//	daemon
//	├─ WebKitNetworkProcess
//	├─ bwrap                      outer monitor, in the host's pid namespace
//	│  └─ bwrap                   pid 1 of the sandbox's own pid namespace
//	│     └─ xdg-dbus-proxy
//	└─ bwrap                      outer monitor
//	   └─ bwrap                   pid 1 of the sandbox
//	      └─ WebKitWebProcess     ~250-310 MB
//
// Only the two outer monitors are direct children, so a direct-children scan
// names two of six processes and misses everything that holds the memory.
func webkitChildren() map[int]bool {
	return webkitDescendants(os.Getpid(), procTable())
}

// webkitDescendants walks procs downward from root, crossing WebKit-family
// processes only. A non-family child (a speech engine, a shell) is a wall,
// not a step: whatever sits under it is somebody else's and is never named.
func webkitDescendants(root int, procs map[int]procInfo) map[int]bool {
	children := map[int][]int{}
	for pid, p := range procs {
		if webkitFamily(p.comm) {
			children[p.ppid] = append(children[p.ppid], pid)
		}
	}
	out := map[int]bool{}
	stack := []int{root}
	for len(stack) > 0 {
		pid := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, c := range children[pid] {
			if !out[c] {
				out[c] = true
				stack = append(stack, c)
			}
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

// procStat reads a pid's parent pid, command name and state letter out of
// /proc/<pid>/stat. ppid is zero and comm empty once the pid is gone or reused
// by an unrelated process, which stillWebkit relies on to tell "exited" from
// "still stuck".
func procStat(pid int) (ppid int, comm string, state byte) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, "", 0
	}
	// Format is "pid (comm) state ppid ...". comm can itself contain spaces
	// or parens, so split on the LAST ')' rather than on fields.
	s := string(data)
	end := strings.LastIndexByte(s, ')')
	open := strings.IndexByte(s, '(')
	if end < 0 || open < 0 || open >= end {
		return 0, "", 0
	}
	comm = s[open+1 : end]
	fields := strings.Fields(s[end+1:])
	if len(fields) < 2 {
		return 0, comm, 0
	}
	if len(fields[0]) > 0 {
		state = fields[0][0]
	}
	ppid, _ = strconv.Atoi(fields[1])
	return ppid, comm, state
}

// procParent is procStat without the state, for callers that only need the
// tree.
func procParent(pid int) (ppid int, comm string) {
	ppid, comm, _ = procStat(pid)
	return ppid, comm
}

// stillWebkit reports whether pid is alive, not yet a zombie, and still a
// WebKit-family process - so a pid that exited, or was reused by something
// unrelated, is never signalled.
func stillWebkit(pid int) bool {
	ppid, comm, state := procStat(pid)
	return ppid != 0 && state != 'Z' && webkitFamily(comm)
}

// reapStrayWebkitChildren waits webkitReapGrace for the pids in family (a
// window's own webkitChildren diff, captured at creation) to exit on their
// own, SIGKILLs whichever are still alive and still WebKit-family, then looks
// again and logs what that left. It only ever acts on pids a specific window's
// own creation produced, so a still-open sibling window's process is never a
// candidate - there is no sweep of the daemon's whole process tree here.
//
// SIGKILL, not SIGTERM. The inner bwrap is pid 1 of the sandbox's pid
// namespace, and pid_namespaces(7) is explicit: a namespace's init receives a
// signal from an ancestor namespace only if it has a handler for it, or the
// signal is SIGKILL or SIGSTOP. bwrap installs no handler, so a SIGTERM to it
// is dropped without a trace - confirmed live 2026-09-24, still running two
// seconds later. SIGKILL to that init takes the whole namespace with it. The
// window is already destroyed by the time this runs, so there is nothing left
// for a graceful exit to save.
//
// The 2026-09-18 version SIGTERMed the daemon's direct children, which are
// the outer monitors. Those did die - and the inner init and web process under
// each were reparented to the user's systemd and lived on. Eight of them, ~300
// MB apiece, were found on 2026-09-24 under a daemon that had logged a "reaped"
// line for every one.
func reapStrayWebkitChildren(family map[int]bool, log *slog.Logger) {
	if len(family) == 0 {
		return
	}
	go func() {
		time.Sleep(webkitReapGrace)
		killed := 0
		for pid := range family {
			if !stillWebkit(pid) {
				continue // exited on its own, or the pid was reused by something else
			}
			killed++
			_ = killProcess(pid)
		}
		if killed == 0 {
			return
		}
		time.Sleep(webkitReapVerify)
		left := 0
		for pid := range family {
			if stillWebkit(pid) {
				left++
			}
		}
		if log != nil {
			log.Warn("webui.webkit_process_reaped", "component", "webui",
				"count", killed, "left", left, "grace_s", int(webkitReapGrace.Seconds()))
		}
	}()
}
