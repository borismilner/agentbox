package webui

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduleReleaseCoalescesABurstIntoOneRelease(t *testing.T) {
	var n atomic.Int32
	oldQuiet, oldRelease := releaseQuiet, release
	releaseQuiet, release = 60*time.Millisecond, func() { n.Add(1) }
	t.Cleanup(func() { releaseQuiet, release = oldQuiet, oldRelease })

	for range 5 { // a stack of toasts closing together
		scheduleRelease()
		time.Sleep(10 * time.Millisecond)
	}
	if !eventually(func() bool { return n.Load() == 1 }) {
		t.Fatalf("releases = %d after the burst, want 1", n.Load())
	}
	time.Sleep(150 * time.Millisecond)
	if got := n.Load(); got != 1 {
		t.Fatalf("releases = %d, want exactly 1 for one burst", got)
	}
	scheduleRelease() // a later close is a new burst
	if !eventually(func() bool { return n.Load() == 2 }) {
		t.Fatalf("releases = %d after a second close, want 2", n.Load())
	}
}

// eventually polls cond for up to two seconds. Local rather than waitFor, which
// lives in a linux-only test file and this test is not.
func eventually(cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}
