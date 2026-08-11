//go:build linux

package editor

// xdg-open is the desktop's own handler on Linux and the BSDs. It is present on
// every desktop worth the name and absent on a bare server, which is why Resolve
// still checks LookPath rather than assuming it.
func fallbackArgv() []string { return []string{"xdg-open"} }
