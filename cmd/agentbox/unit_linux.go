//go:build linux

package main

import (
	"context"
	"os/exec"
	"time"
)

// unitShow on Linux, where the service manager is systemd and the daemon has a
// user unit. The contract is stated once in the portable caller (unit.go):
// cheap, read-only, never fatal.
//
// The deadline is not decoration. `systemctl --user` talks to a bus, and a bus
// that is not answering is exactly the kind of broken desktop somebody runs
// `agentbox status` on - a status command that hangs there would be worse than
// one that says nothing about the unit.
func unitShow() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "--user", "show", unitName,
		"-p", "ActiveState", "-p", "UnitFileState").Output()
	if err != nil {
		// No systemctl, no session bus, no such unit: all of them mean there is
		// nothing to report, which is different from a report of trouble.
		return ""
	}
	return string(out)
}
