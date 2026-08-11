package main

import (
	"os"
	"strings"
)

// unitName is the systemd user unit packaging/agentbox.service installs.
const unitName = "agentbox.service"

// The desktop can end up with no hub and nothing saying so, or with a daemon
// systemd does not manage, and neither shows up anywhere a person looks (R-24).
// The unit gives up after its start limit and goes `failed`, and the auto-spawn
// then half-rescues it: the next MCP call starts a daemon outside the unit, so
// something answers while nothing would ever restart it again.
//
// unitShow is the platform's answer to "what does the service manager think",
// as the raw KEY=value block, or "" where there is no such question to ask. Its
// contract, which every platform owes: cheap, read-only, and never fatal - a
// status command that cannot reach the service manager still reports the daemon.
// Everything below is portable and reads what it returns.

// parseUnitShow pulls the two fields worth reading out of `systemctl show`.
func parseUnitShow(out string) (activeState, unitFileState string) {
	for line := range strings.SplitSeq(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "ActiveState":
			activeState = value
		case "UnitFileState":
			unitFileState = value
		}
	}
	return activeState, unitFileState
}

// unitNote is the one sentence status prints about the unit, or "" when there is
// nothing worth saying. A healthy unit says nothing at all: a line printed every
// time is a line nobody reads, and the two states here are both abnormal.
//
// daemonUp separates the two ways a failed unit reads. With a daemon answering,
// the desktop works and the thing to know is that nothing is managing it any
// more. With none, that is the whole diagnosis.
func unitNote(activeState, unitFileState string, daemonUp bool) string {
	switch unitFileState {
	case "", "disabled", "not-found", "masked":
		// Nobody asked systemd to run agentbox, so nothing it says is news.
		return ""
	}
	switch activeState {
	case "failed":
		if daemonUp {
			return "the " + unitName + " unit is failed: systemd hit its start limit and stopped restarting it.\n" +
				"the daemon answering now was auto-spawned, so nothing will bring it back next time.\n" +
				"systemctl --user reset-failed " + unitName + " && systemctl --user start " + unitName
		}
		return "the " + unitName + " unit is failed: systemd hit its start limit and gave up restarting it.\n" +
			"journalctl --user -u " + unitName + " has the reason, and so does the daemon's own log.\n" +
			"systemctl --user reset-failed " + unitName + " && systemctl --user start " + unitName
	case "inactive", "deactivating":
		if daemonUp {
			return "the " + unitName + " unit is enabled but not running, so the daemon answering was auto-spawned and nothing manages it."
		}
		return "the " + unitName + " unit is enabled but not running: systemctl --user start " + unitName
	}
	return ""
}

// unitAdvice is unitNote against this machine, or "" when the question does not
// apply: a named instance is a daemon of its own (NFR12) and the unit knows
// nothing about it, so reporting the unit's state beside it would be an answer
// about a different process.
func unitAdvice(daemonUp bool) string {
	if os.Getenv("AGENTBOX_INSTANCE") != "" {
		return ""
	}
	active, fileState := parseUnitShow(unitShow())
	return unitNote(active, fileState, daemonUp)
}
