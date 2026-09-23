//go:build windows

package webui

// killProcess has nothing to do here. The reaper finds its candidates in
// /proc, which Windows does not have, so it never holds a pid to pass; the
// function exists so the portable caller compiles on every target.
func killProcess(int) error { return nil }
