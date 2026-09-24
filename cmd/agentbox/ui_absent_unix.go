//go:build noui && unix

package main

import (
	"fmt"
	"os"
	"syscall"
)

// execUI replaces this process with the full build: same pid, so a systemd
// unit's MainPID and the single-instance flock both stay truthful.
func execUI(path string, args []string) {
	err := syscall.Exec(path, append([]string{path}, args...), os.Environ())
	fmt.Fprintf(os.Stderr, "agentbox: exec %s: %v\n", path, err)
	os.Exit(1)
}
