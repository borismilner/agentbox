// Package client dials the daemon socket, spawning the daemon first when
// none is running (ADR-0003). The CLI and the MCP bridge both sit on it.
package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/server"
)

// SpawnFunc starts a daemon process; replaced in tests.
type SpawnFunc func() error

// SpawnDaemon execs this same binary as a detached daemon. Spawning when a
// daemon already runs is harmless: the loser of the lock exits 0 (NFR12).
func SpawnDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate own executable: %w", err)
	}
	cmd := exec.Command(exe, "daemon")
	// Detached from this session, so the daemon outlives the shell that first
	// asked for it. See detach for what that means per platform.
	detach(cmd)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn daemon: %w", err)
	}
	return cmd.Process.Release()
}

// Dial connects to the daemon in dir, auto-spawning it if absent and
// retrying until ctx expires (default expectation: under 2 s).
func Dial(ctx context.Context, dir string, spawn SpawnFunc) (*proto.Conn, error) {
	sock := server.SocketPath(dir)
	conn, err := net.Dial("unix", sock)
	if err == nil {
		return proto.NewConn(conn), nil
	}

	if spawn == nil {
		spawn = SpawnDaemon
	}
	if err := spawn(); err != nil {
		return nil, err
	}
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("daemon did not come up at %s: %w", sock, ctx.Err())
		case <-tick.C:
			conn, err := net.Dial("unix", sock)
			if err == nil {
				return proto.NewConn(conn), nil
			}
		}
	}
}
