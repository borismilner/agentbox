//go:build darwin

package editor

// open is macOS's xdg-open: it hands the file to whatever the Finder would use.
// Always present, and still checked, so this file needs no special case.
func fallbackArgv() []string { return []string{"open"} }
