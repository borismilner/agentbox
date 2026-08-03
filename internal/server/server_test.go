package server

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSingleInstanceLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agentbox")
	l1, err := Listen(dir, discard())
	if err != nil {
		t.Fatalf("first listen: %v", err)
	}
	defer l1.Close()

	_, err = Listen(dir, discard())
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second listen: got %v, want ErrAlreadyRunning", err)
	}
}

func TestLockReleasedOnClose(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agentbox")
	l1, err := Listen(dir, discard())
	if err != nil {
		t.Fatal(err)
	}
	l1.Close()

	l2, err := Listen(dir, discard())
	if err != nil {
		t.Fatalf("listen after close: %v", err)
	}
	l2.Close()
}

func TestStaleSocketIsReclaimed(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agentbox")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// A leftover file at the socket path, as after a SIGKILL.
	if err := os.WriteFile(SocketPath(dir), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := Listen(dir, discard())
	if err != nil {
		t.Fatalf("listen over stale socket: %v", err)
	}
	l.Close()
}

func TestRuntimeDirInstances(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got := RuntimeDir(""); got != "/run/user/1000/agentbox" {
		t.Fatalf("default instance dir = %q", got)
	}
	if got := RuntimeDir("test"); got != "/run/user/1000/agentbox-test" {
		t.Fatalf("named instance dir = %q", got)
	}
	if RuntimeDir("") == RuntimeDir("test") {
		t.Fatal("instances must not share a dir")
	}
}
