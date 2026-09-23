//go:build linux

package webui

import (
	"os"
	"os/exec"
	"sort"
	"syscall"
	"testing"
	"time"
)

// The tree below is the one measured on a live view on 2026-09-24 (pids kept):
// each sandbox is an outer bwrap monitor in the host namespace, an inner bwrap
// that is pid 1 of the sandbox, and the real process under that. Only the two
// outer monitors are the daemon's direct children.
func liveViewTable(self int) map[int]procInfo {
	return map[int]procInfo{
		7321: {self, "WebKitNetworkProcess"},
		7337: {self, "bwrap"},
		7338: {7337, "bwrap"},
		7339: {7338, "xdg-dbus-proxy"},
		7342: {self, "bwrap"},
		7343: {7342, "bwrap"},
		7344: {7343, "WebKitWebProcess"},
	}
}

func sortedPIDs(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for pid := range m {
		out = append(out, pid)
	}
	sort.Ints(out)
	return out
}

func TestWebkitDescendantsNamesTheWholeSandboxTree(t *testing.T) {
	const self = 7123
	procs := liveViewTable(self)
	// Strangers: an unrelated child of the daemon, another UI process with a
	// sandbox of its own, and a web process reachable only through a shell -
	// none of them is ours to kill.
	procs[7125] = procInfo{self, "voxtype"}
	procs[7999] = procInfo{1, "rigwindow"}
	procs[8000] = procInfo{7999, "bwrap"}
	procs[8001] = procInfo{8000, "bwrap"}
	procs[8002] = procInfo{8001, "WebKitWebProcess"}
	procs[9000] = procInfo{self, "sh"}
	procs[9001] = procInfo{9000, "WebKitWebProcess"}

	got := sortedPIDs(webkitDescendants(self, procs))
	want := []int{7321, 7337, 7338, 7339, 7342, 7343, 7344}
	if len(got) != len(want) {
		t.Fatalf("descendants = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("descendants = %v, want %v", got, want)
		}
	}
}

func TestWebkitDescendantsDirectChildrenAloneMissTheMemory(t *testing.T) {
	// Documents the 2026-09-18 bug rather than the fix: a scan of direct
	// children sees the two monitors and nothing that holds memory.
	const self = 7123
	direct := 0
	for _, p := range liveViewTable(self) {
		if p.ppid == self && p.comm == "bwrap" {
			direct++
		}
	}
	if direct != 2 {
		t.Fatalf("fixture drifted: %d direct bwrap children, want 2", direct)
	}
	all := webkitDescendants(self, liveViewTable(self))
	if !all[7344] || !all[7343] {
		t.Fatalf("the web process and its sandbox init must be named: %v", sortedPIDs(all))
	}
}

func TestDiffWebkitChildrenKeepsOnlyTheNewOnes(t *testing.T) {
	before := map[int]bool{1: true, 2: true}
	after := map[int]bool{2: true, 3: true}
	got := diffWebkitChildren(before, after)
	if len(got) != 1 || !got[3] {
		t.Fatalf("diff = %v, want {3}", sortedPIDs(got))
	}
}

// waitFor polls cond for up to d.
func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// childOf returns the one live child of ppid, or 0.
func childOf(ppid int) int {
	for pid, p := range procTable() {
		if p.ppid == ppid {
			return pid
		}
	}
	return 0
}

// TestSandboxInitIgnoresSIGTERM is the kernel fact the reaper rests on, run
// against a real bwrap: SIGTERM from outside a pid namespace does not reach
// its init, SIGKILL does and takes the namespace with it. Skipped where bwrap
// is absent or unprivileged namespaces are off.
func TestSandboxInitIgnoresSIGTERM(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	cmd := exec.Command("bwrap", "--unshare-pid", "--dev-bind", "/", "/", "/bin/sleep", "30")
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Skipf("bwrap would not start: %v", err)
	}
	outer := cmd.Process.Pid
	defer func() {
		_ = syscall.Kill(outer, syscall.SIGKILL)
		_ = cmd.Wait()
	}()

	var inner, leaf int
	if !waitFor(3*time.Second, func() bool {
		inner = childOf(outer)
		if inner == 0 {
			return false
		}
		leaf = childOf(inner)
		return leaf != 0
	}) {
		// The outer may have exited with an error (user namespaces disabled).
		if !stillWebkit(outer) {
			t.Skip("bwrap exited before building a sandbox; unprivileged namespaces are probably off")
		}
		t.Fatalf("no sandbox tree under bwrap %d", outer)
	}
	if _, comm, _ := procStat(inner); comm != "bwrap" {
		t.Fatalf("inner process is %q, want bwrap (the sandbox init)", comm)
	}

	// SIGTERM to the sandbox init from out here: dropped, by pid_namespaces(7).
	if err := syscall.Kill(inner, syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM inner: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if !stillWebkit(inner) {
		t.Fatalf("SIGTERM killed the sandbox init %d; the reaper's SIGKILL is then not the only fix and this test is stale", inner)
	}
	if ppid, _, _ := procStat(leaf); ppid != inner {
		t.Fatalf("leaf %d no longer under init %d", leaf, inner)
	}

	// SIGKILL: init dies, and the kernel takes every process in its namespace.
	if err := syscall.Kill(inner, syscall.SIGKILL); err != nil {
		t.Fatalf("SIGKILL inner: %v", err)
	}
	if !waitFor(3*time.Second, func() bool { return !stillWebkit(inner) && !stillWebkit(leaf) }) {
		t.Fatalf("after SIGKILL: init alive=%v leaf alive=%v", stillWebkit(inner), stillWebkit(leaf))
	}
}
