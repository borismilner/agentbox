//go:build !linux

package webui

import "runtime/debug"

// releaseMemory has no allocator of GTK's to trim off Linux; the Go heap is
// the part that is ours everywhere.
func releaseMemory() { debug.FreeOSMemory() }
