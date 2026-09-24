package webui

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
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

// webkitFamily names the processes WebKitGTK spawns: one network process
// shared by every view, and per view one sandboxed web process, the
// xdg-dbus-proxy beside it, and the bwrap wrappers the sandbox puts around
// each of those two. The walk crosses all of them; webkitShared says which
// one it then refuses to name.
func webkitFamily(comm string) bool {
	switch comm {
	case "WebKitNetworkProcess", "WebKitWebProcess", "bwrap", "xdg-dbus-proxy":
		return true
	}
	return false
}

// webkitShared is the one WebKit process that is per-daemon, not per-view.
// WebKitGTK launches a single network process for the whole application on
// its first view and keeps it for the life of the process, so it belongs to
// no window and is never a reap candidate: it would land in the set of
// whichever window happened to be the daemon's first, be killed when that
// window closed, and take down whatever a still-open sibling was loading -
// for WebKit to relaunch it on the next request anyway. It holds ~45 MB
// once; the leak this file exists for is the per-view sandbox.
func webkitShared(comm string) bool { return comm == "WebKitNetworkProcess" }

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
// The shared network process is crossed but left out of the result
// (webkitShared).
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
	for pid := range out {
		if webkitShared(procs[pid].comm) {
			delete(out, pid)
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

// webkitSnapshot and webkitReaper are what a tracker calls; tests swap them.
var (
	webkitSnapshot = webkitChildren
	webkitReaper   = reapStrayWebkitChildren
)

// webkitTracker pins the WebKit-family processes ONE window's creation
// produced and reaps them when that window closes. There is one per window,
// made by UI.trackWindow on the goroutine about to call NewWithOptions, and
// its reap is registered in that window's WindowClosing hook - which is the
// path every close takes, the WM's and Close()'s alike (wails emits the same
// event for both), so no window kind is left out. Before 2026-09-24 only the
// card did this; the viewer, the board, the progress bar, the strip, the
// mark, the app and the panel all closed through the same wails#2565 destroy
// and left the same ~170-300 MB sandbox behind, unreaped.
//
// Ownership is by arrival: whatever WebKit-family process appears under the
// daemon between the snapshot and pin is this window's. Two rules keep that
// honest. Ready (bridge.go) pins the moment the window's bundle mounts, which
// is when its own web process certainly exists and before anything else is
// likely to have spawned. And UI.trackWindow pins every older, still-unpinned
// tracker BEFORE it snapshots for a new window, so an older window whose
// surface never mounted cannot claim a newer window's processes. A window
// closed before either happened is diffed at reap, so nothing escapes for
// being quick. The gap that remains: two windows created within WebKit's
// spawn latency of each other, where the older's pin can run before its own
// processes exist; that window then leaks rather than a sibling being killed.
type webkitTracker struct {
	mu     sync.Mutex
	before map[int]bool
	pids   map[int]bool // the window's own set; nil until pinned
	done   bool
	snap   func() map[int]bool
	kill   func(map[int]bool, *slog.Logger)
	log    *slog.Logger
}

func newWebkitTracker(log *slog.Logger) *webkitTracker {
	return &webkitTracker{before: webkitSnapshot(), snap: webkitSnapshot, kill: webkitReaper, log: log}
}

// pin names the window's own processes: whatever WebKit-family process has
// appeared since the snapshot. The first call wins and later ones are no-ops,
// so a late Ready cannot widen a set a sibling's creation already closed.
func (t *webkitTracker) pin() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pids != nil || t.done {
		return
	}
	t.pids = diffWebkitChildren(t.before, t.snap())
}

// reap hands the window's processes to the reaper, once. A window closed
// before it was pinned - its bundle never mounted - is diffed now.
func (t *webkitTracker) reap() {
	t.mu.Lock()
	if t.done {
		t.mu.Unlock()
		return
	}
	t.done = true
	pids := t.pids
	if pids == nil {
		pids = diffWebkitChildren(t.before, t.snap())
	}
	t.mu.Unlock()
	t.kill(pids, t.log)
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
// window's own set, pinned by its webkitTracker) to exit on their own,
// SIGKILLs whichever are still alive and still WebKit-family, then looks
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
