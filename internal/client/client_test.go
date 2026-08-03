package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/server"
)

func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func echoHandler(_ context.Context, method string, params json.RawMessage) (any, *proto.RPCError) {
	return map[string]string{"method": method}, nil
}

func TestClientServerRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agentbox")
	l, err := server.Listen(dir, discard())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx := t.Context()
	go l.Serve(ctx, echoHandler)

	conn, err := Dial(ctx, dir, func() error {
		t.Error("spawn must not run when the daemon is up")
		return nil
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	var res map[string]string
	if err := conn.Call(ctx, proto.MethodStatus, nil, &res); err != nil {
		t.Fatalf("call: %v", err)
	}
	if res["method"] != proto.MethodStatus {
		t.Fatalf("echo = %v", res)
	}
}

// The auto-spawn race: the daemon comes up only after the client first
// finds no socket; Dial must converge on it.
func TestDialAutoSpawn(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agentbox")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	spawned := make(chan struct{})
	spawn := func() error {
		go func() {
			time.Sleep(100 * time.Millisecond) // simulate daemon startup
			l, err := server.Listen(dir, discard())
			if err != nil {
				t.Error(err)
				return
			}
			close(spawned)
			l.Serve(ctx, echoHandler)
			l.Close()
		}()
		return nil
	}

	conn, err := Dial(ctx, dir, spawn)
	if err != nil {
		t.Fatalf("dial with auto-spawn: %v", err)
	}
	defer conn.Close()
	select {
	case <-spawned:
	default:
		t.Fatal("dial returned before the daemon was up")
	}
	if err := conn.Call(ctx, proto.MethodStatus, nil, &map[string]string{}); err != nil {
		t.Fatalf("call after spawn: %v", err)
	}
}

func TestDialSpawnFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agentbox")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	wantErr := errors.New("exec failed")
	_, err := Dial(ctx, dir, func() error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want spawn error", err)
	}
}
