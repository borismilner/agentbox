package main

import (
	"path/filepath"
	"strings"
)

// uiBinaryFor names the full build that a client-only build (-tags noui) hands
// the daemon to. NFR17: every Claude session holds an `agentbox mcp` child and
// every hook runs the CLI, and none of them needs GTK or WebKit - linking them
// cost each session ~7 MB PSS and 130 mapped libraries for nothing. So
// `make deploy` installs the client as $(BINDIR)/agentbox and the full build as
// ../lib/agentbox/agentbox beside it, and only `daemon` and `webui-demo` cross
// over.
//
// The full build keeps the basename, and that is load-bearing: the Makefile's
// kill-daemons finds the daemon by process NAME (pgrep -x agentbox), and an
// exec'd daemon is named after the file it was exec'd from.
//
// env is $AGENTBOX_UI_BINARY and wins when set. exe may carry the " (deleted)"
// suffix Linux gives a running binary that a deploy has replaced.
func uiBinaryFor(exe, env string) string {
	if env != "" {
		return env
	}
	exe = strings.TrimSuffix(exe, " (deleted)")
	return filepath.Join(filepath.Dir(exe), "..", "lib", "agentbox", filepath.Base(exe))
}
