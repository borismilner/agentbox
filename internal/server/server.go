// Package server owns the daemon's unix socket and the single-instance
// lock (NFR12, ADR-0003). The flock is authoritative; only the holder may
// remove a stale socket and bind.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/proto"
)

const (
	SocketName = "agentbox.sock"
	lockName   = "daemon.lock"
)

// ErrAlreadyRunning means another daemon holds the lock; the loser exits
// cleanly (NFR12).
var ErrAlreadyRunning = errors.New("another agentbox daemon is already running")

// RuntimeDir returns the per-user runtime directory for the given instance
// name ("" = the default instance).
func RuntimeDir(instance string) string {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), fmt.Sprintf("agentbox-%d", os.Getuid()))
	}
	name := "agentbox"
	if instance != "" {
		name = "agentbox-" + instance
	}
	return filepath.Join(base, name)
}

func SocketPath(dir string) string { return filepath.Join(dir, SocketName) }

type Listener struct {
	Dir  string
	lock *os.File
	ln   *net.UnixListener
	log  *slog.Logger

	mu    sync.Mutex
	rider proto.RiderFunc
}

// SetRider installs FR83's discovery rider on every connection accepted from now
// on. Set it before Serve; a listener without one behaves exactly as before.
func (l *Listener) SetRider(f proto.RiderFunc) {
	l.mu.Lock()
	l.rider = f
	l.mu.Unlock()
}

func (l *Listener) riderFn() proto.RiderFunc {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rider
}

// Listen acquires the instance lock and binds the socket. Order matters:
// lock first, then stale-socket cleanup, then bind (ADR-0003).
func Listen(dir string, log *slog.Logger) (*Listener, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create runtime dir: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := lockFile(lock); err != nil {
		lock.Close()
		if errors.Is(err, ErrAlreadyRunning) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	sock := SocketPath(dir)
	if err := os.Remove(sock); err != nil && !os.IsNotExist(err) {
		lock.Close()
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		lock.Close()
		return nil, fmt.Errorf("bind socket: %w", err)
	}
	return &Listener{Dir: dir, lock: lock, ln: ln, log: log}, nil
}

// Serve accepts connections until ctx is done or the listener closes. Each
// connection is served on its own goroutine after a peer-UID check (NFR8).
func (l *Listener) Serve(ctx context.Context, h proto.Handler) error {
	go func() {
		<-ctx.Done()
		l.ln.Close()
	}()
	for {
		conn, err := l.ln.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		uid, err := peerUID(conn)
		if err != nil || uid != os.Getuid() {
			l.log.Warn("ipc.rejected", "component", "server", "peer_uid", uid, "err", errStr(err))
			conn.Close()
			continue
		}
		go func() {
			defer conn.Close()
			defer func() {
				if r := recover(); r != nil {
					l.log.Error(logging.EvPanic, "component", "server", "panic", fmt.Sprint(r))
				}
			}()
			pc := proto.NewConn(conn)
			// FR83's discovery rider, installed per connection: whatever this
			// caller has not been told about its area rides back on its next
			// answer, whichever method that answer belongs to.
			if rider := l.riderFn(); rider != nil {
				pc.SetRider(rider)
			}
			if err := pc.Serve(ctx, h); err != nil {
				l.log.Warn("ipc.conn_closed", "component", "server", "err", err.Error())
			}
		}()
	}
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Close releases the socket and the lock, in that order.
func (l *Listener) Close() error {
	err := l.ln.Close()
	unlockFile(l.lock)
	l.lock.Close()
	return err
}
