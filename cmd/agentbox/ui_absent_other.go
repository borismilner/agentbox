//go:build noui && !unix

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// execUI has no exec(2) to call here, so it runs the full build as a child and
// exits with its code.
func execUI(path string, args []string) {
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: run %s: %v\n", path, err)
		os.Exit(1)
	}
	os.Exit(0)
}
