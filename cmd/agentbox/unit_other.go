//go:build !linux

package main

// unitShow everywhere else. macOS has launchd and Windows has the SCM, and
// agentbox ships a unit for neither, so there is no service manager holding an
// opinion about this daemon and nothing to report. Same contract as the Linux
// side (unit.go), answered honestly by an empty string rather than by a guess.
func unitShow() string { return "" }
