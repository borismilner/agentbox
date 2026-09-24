package webui

import (
	"sync"
	"time"
)

// NFR17, minimal footprint. A closed window's WebKit processes are killed by
// the reaper, but the daemon's own share - GTK widgets, the WebKitWebView, the
// JS bridge's buffers - goes back to glibc's malloc and the Go heap, and both
// keep freed pages resident until something asks. glibc only returns the top
// of its heap on its own; the holes a window leaves behind stay, which is how
// a daemon that has opened a few hundred cards sits at twice its idle size.
//
// So once windows stop closing for a moment, the daemon asks for both back.
// One call per burst rather than per window: closing a stack of toasts is one
// trim, not ten.
var (
	releaseQuiet = 5 * time.Second
	releaseMu    sync.Mutex
	releaseTimer *time.Timer
	release      = releaseMemory // replaced in tests
)

// scheduleRelease arms (or re-arms) the one pending release.
func scheduleRelease() {
	releaseMu.Lock()
	defer releaseMu.Unlock()
	if releaseTimer != nil {
		releaseTimer.Reset(releaseQuiet)
		return
	}
	releaseTimer = time.AfterFunc(releaseQuiet, func() {
		releaseMu.Lock()
		releaseTimer = nil
		releaseMu.Unlock()
		release()
	})
}
