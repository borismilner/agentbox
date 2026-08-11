//go:build unix

package readsource

import (
	"path/filepath"
	"syscall"
	"testing"
)

// testFifo makes the one file shape that cannot be tested by writing bytes: a
// named pipe, which os.Open blocks on until somebody writes to the other end.
// That is why the guard in read.go stats the PATH before opening it, and a
// test for it therefore has to make a real fifo.
func testFifo(t *testing.T, dir string) (string, bool) {
	t.Helper()
	p := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Logf("no fifo on this system: %v", err)
		return "", false
	}
	return p, true
}
