// Package logging is the daemon's event log: structured JSONL with stable
// event names, written for AI maintainers reading the file, not humans
// watching a terminal (NFR13).
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	FileName    = "agentbox.jsonl"
	rotatedName = "agentbox.jsonl.1"
)

// Path returns the log file location under the given state directory.
func Path(stateDir string) string {
	return filepath.Join(stateDir, "log", FileName)
}

// rotatingWriter renames the file aside once it grows past maxBytes. One
// rotation generation is enough: the log is an event stream, not an
// archive; history that matters lives in the store.
type rotatingWriter struct {
	mu       sync.Mutex
	f        *os.File
	path     string
	size     int64
	maxBytes int64
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > w.maxBytes {
		w.f.Close()
		if err := os.Rename(w.path, filepath.Join(filepath.Dir(w.path), rotatedName)); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("rotate log: %w", err)
		}
		f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return 0, fmt.Errorf("reopen log: %w", err)
		}
		w.f = f
		w.size = 0
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}

// Open returns a logger writing JSONL to the state dir, creating
// directories as needed. The caller closes the returned closer on
// shutdown.
func Open(stateDir string, level slog.Level, maxMB int) (*slog.Logger, io.Closer, error) {
	path := Path(stateDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	w := &rotatingWriter{f: f, path: path, size: st.Size(), maxBytes: int64(maxMB) * 1024 * 1024}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(h), w, nil
}

// Event names form a stable vocabulary; grep the log by these, not by
// message prose.
const (
	EvDaemonStart    = "daemon.start"
	EvDaemonStop     = "daemon.stop"
	EvDaemonLockLost = "daemon.lock_busy"
	EvStoreMigrated  = "store.migrated"
	EvItemCreated    = "item.created"
	EvItemDisplayed  = "item.displayed"
	EvItemAnswered   = "item.answered"
	EvItemDismissed  = "item.dismissed"
	EvItemExpired    = "item.expired"
	EvItemCancelled  = "item.cancelled"
	// Flood control (FR30). EvFlooded fires once per stack card - the moment a
	// caller went over its budget - and EvItemCollapsed once per item folded into
	// one, so the log answers both "when did agentbox start collapsing" and "what
	// exactly did the human not see as its own card".
	EvFlooded       = "item.flooded"
	EvItemCollapsed = "item.collapsed"
	EvIPCCall       = "ipc.call"
	EvSoundPlayed   = "sound.played"
	EvSoundFailed   = "sound.failed"
	EvSpeechSpoke   = "speech.spoke"
	EvSpeechStarted = "speech.engine_started"
	EvSpeechStopped = "speech.engine_stopped"
	EvSpeechFailed  = "speech.failed"
	EvSpeechAloud   = "speech.aloud" // the human working the read-aloud transport
	EvControl       = "control"      // the desktop handover strip (FR74)
	EvRenderFailed  = "render.failed"
	// Multi-agent coordination (FR83). One vocabulary for the whole roster
	// lifecycle - attach, announced, detach - because the question asked of this
	// log is always "which agents were here, and what were they each doing".
	EvSync = "sync"
	// The assignment scheduler (M12/FR82). One vocabulary for the whole
	// lifecycle - armed, started, finished, skipped - because the question asked
	// of this log is always "did the thing that was supposed to run, run".
	EvAssignmentRun = "assignment.run"
	EvStoreError    = "store.error"
	EvPanic         = "panic"
)
