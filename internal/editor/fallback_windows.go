//go:build windows

package editor

// On Windows the handler is a shell builtin rather than a program, so the fallback
// is cmd with start. The empty title argument is not decoration: start reads its
// FIRST quoted argument as a window title, so a path in quotes would open a console
// named after the file and edit nothing.
func fallbackArgv() []string { return []string{"cmd", "/c", "start", ""} }
