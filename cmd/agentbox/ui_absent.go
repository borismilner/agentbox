//go:build noui

package main

import (
	"fmt"
	"os"
)

// The client-only build: daemon.go and webui.go are the two files that import
// internal/webui, and with them out nothing links GTK or WebKit. The commands
// that need them are handed to the full build (see uiBinaryFor).

func runDaemon() { handToUI() }

func runWebUIDemo([]string) { handToUI() }

func handToUI() {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	ui := uiBinaryFor(exe, os.Getenv("AGENTBOX_UI_BINARY"))
	if _, err := os.Stat(ui); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: this is the client-only build, and the full build it hands %q to is not at %s (make deploy installs both; AGENTBOX_UI_BINARY overrides the path)\n", os.Args[1], ui)
		os.Exit(1)
	}
	execUI(ui, os.Args[1:])
}
