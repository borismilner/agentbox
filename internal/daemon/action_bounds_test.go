package daemon

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// R-22. An action button used to run its command with no deadline and buffer
// every byte it wrote: a command that never exits held a process for the
// daemon's lifetime with nothing on screen saying the click had not finished,
// and a loud one was a way to take the daemon out by memory and drop every
// parked agent with it. These are the four bounds that replaced that.

// safeBuf is a log sink two goroutines share: the exec goroutine writes it while
// the test reads it.
type safeBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// newLoggingTestDaemon is newTestDaemon with the log kept instead of discarded,
// which is how the size of what an action wrote is observable at all: the bound
// is on memory the daemon holds, and the count of what it refused to hold is a
// field on the line the exec writes.
func newLoggingTestDaemon(t *testing.T, cfg Config) (*Daemon, *safeBuf) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "agentbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	sink := &safeBuf{}
	ui := &fakeUI{}
	d, err := New(cfg, slog.New(slog.NewJSONHandler(sink, nil)), st, &fakeSound{}, ui)
	if err != nil {
		t.Fatal(err)
	}
	ui.mu.Lock()
	ui.d = d
	ui.mu.Unlock()
	return d, sink
}

// errorCards returns the daemon-raised failure cards in the store, newest first,
// skipping the item whose button was clicked.
func errorCards(t *testing.T, d *Daemon, clicked string) []store.StoredItem {
	t.Helper()
	recent, err := d.st.Recent(20)
	if err != nil {
		t.Fatal(err)
	}
	var out []store.StoredItem
	for _, r := range recent {
		if r.ID != clicked && r.Level == proto.LevelError {
			out = append(out, r)
		}
	}
	return out
}

func TestActionThatNeverExitsIsKilledAndSaysSo(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{ActionTimeout: 300 * time.Millisecond})
	it := notifyItem(proto.LevelWarning)
	it.Actions = []proto.Action{{Label: "Wedge", Exec: "sleep 60"}}
	res := callNotify(t, d, it)

	d.RunAction(res.ID, 0)
	var card store.StoredItem
	waitFor(t, "the failure card for a command that never exits", func() bool {
		cards := errorCards(t, d, res.ID)
		if len(cards) == 0 {
			return false
		}
		card = cards[0]
		return true
	})
	if !strings.Contains(card.Item.Title, "Action failed") {
		t.Fatalf("card title = %q, want an action failure", card.Item.Title)
	}
	// The body has to say the command ran out of time rather than that it exited
	// badly, because the two are different things for whoever reads the card: one
	// is a bug in the command, the other is a command still holding something.
	if !strings.Contains(card.Item.Body, "still running") {
		t.Fatalf("card body does not say the command outlived its timeout: %q", card.Item.Body)
	}
}

func TestActionTimeoutKillsWhatTheCommandStarted(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{ActionTimeout: 300 * time.Millisecond})
	dir := t.TempDir()
	it := notifyItem(proto.LevelWarning)
	it.Cwd = dir
	// The shell records the pid of the child it backgrounds and then waits for it,
	// so killing the shell alone leaves the sleep running with nobody's deadline
	// over it - which is what a deadline without a process group buys.
	it.Actions = []proto.Action{{Label: "Spawn", Exec: "sleep 60 & echo $! > pid; wait"}}
	res := callNotify(t, d, it)

	d.RunAction(res.ID, 0)
	var pid int
	waitFor(t, "the grandchild's pid file", func() bool {
		raw, err := os.ReadFile(filepath.Join(dir, "pid"))
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		return err == nil && pid > 0
	})
	if !pidAlive(pid) {
		t.Fatalf("the test's own premise is wrong: pid %d was not running before the timeout", pid)
	}
	waitFor(t, "the grandchild to be killed with its group", func() bool { return !pidAlive(pid) })
}

func TestActionOutputIsBoundedInMemory(t *testing.T) {
	d, sink := newLoggingTestDaemon(t, Config{})
	it := notifyItem(proto.LevelWarning)
	// Twelve times what the daemon keeps, written by a command that exits cleanly:
	// the bound is not a failure path, it is what the daemon is willing to hold for
	// any command at all.
	it.Actions = []proto.Action{{Label: "Shout", Exec: "head -c 786432 /dev/zero | tr '\\0' 'x'"}}
	res := callNotify(t, d, it)

	d.RunAction(res.ID, 0)
	var line string
	waitFor(t, "the action to finish", func() bool {
		for l := range strings.SplitSeq(sink.String(), "\n") {
			if strings.Contains(l, `"msg":"action.finished"`) {
				line = l
				return true
			}
		}
		return false
	})
	if strings.Contains(line, `"dropped_bytes":0`) {
		t.Fatalf("the daemon kept the whole 768 kB: %.200s", line)
	}
	// The log line carries the kept output quoted, so its own length is an upper
	// bound on what was retained. Generous slack for the JSON escaping around it.
	if len(line) > actionOutputKeep+4096 {
		t.Fatalf("kept %d bytes of output, want at most %d", len(line), actionOutputKeep)
	}
}

func TestCappedBufferKeepsTheHeadAndCountsTheRest(t *testing.T) {
	c := &cappedBuffer{limit: 10}
	for range 5 {
		if n, err := c.Write([]byte("abcde")); n != 5 || err != nil {
			t.Fatalf("Write = %d, %v; a capped buffer must accept everything and keep some of it", n, err)
		}
	}
	if got := c.String(); got != "abcdeabcde" {
		t.Fatalf("kept %q, want the first 10 bytes", got)
	}
	if c.dropped != 15 {
		t.Fatalf("dropped = %d, want 15", c.dropped)
	}
}
