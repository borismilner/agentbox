package daemon

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

func TestActivityRingKeepsWhatTheSessionMovedPast(t *testing.T) {
	r := newRoster(slog.New(slog.DiscardHandler))
	id := proto.Identity{Agent: "claude", Key: "sk-1"}
	r.rows["sk-1"] = &rosterRow{identity: id, announced: true, touched: time.Now()}

	for _, line := range []string{"reading the spec", "writing the parser", "running the tests"} {
		r.Activity(proto.SyncActivityParams{Identity: id, Activity: line})
	}

	got, ok := r.historyOf("sk-1")
	if !ok {
		t.Fatal("no row for a session that just reported")
	}
	// The CURRENT line is not history: it is on the row itself. Two moved past.
	if len(got) != 2 || got[0].line != "reading the spec" || got[1].line != "writing the parser" {
		t.Fatalf("history = %+v", got)
	}
}

func TestActivityRingIgnoresARepeatedLine(t *testing.T) {
	// An unchanged line is not progress - the same rule that keeps it from
	// resetting the age. A ring that recorded it would fill up with one sentence.
	r := newRoster(slog.New(slog.DiscardHandler))
	id := proto.Identity{Agent: "claude", Key: "sk-1"}
	r.rows["sk-1"] = &rosterRow{identity: id, announced: true, touched: time.Now()}

	r.Activity(proto.SyncActivityParams{Identity: id, Activity: "compiling"})
	for range 5 {
		r.Activity(proto.SyncActivityParams{Identity: id, Activity: "compiling"})
	}
	r.Activity(proto.SyncActivityParams{Identity: id, Activity: "linking"})

	got, _ := r.historyOf("sk-1")
	if len(got) != 1 || got[0].line != "compiling" {
		t.Fatalf("history = %+v, want one tick", got)
	}
}

func TestActivityRingIsBounded(t *testing.T) {
	r := newRoster(slog.New(slog.DiscardHandler))
	id := proto.Identity{Agent: "claude", Key: "sk-1"}
	r.rows["sk-1"] = &rosterRow{identity: id, announced: true, touched: time.Now()}

	for i := range historyKeep * 3 {
		r.Activity(proto.SyncActivityParams{Identity: id, Activity: "step " + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + itoa(i)})
	}
	got, _ := r.historyOf("sk-1")
	if len(got) != historyKeep {
		t.Fatalf("history holds %d, want the cap %d", len(got), historyKeep)
	}
	// The ring keeps the NEWEST, so the first line ever said must be gone.
	for _, tick := range got {
		if strings.HasSuffix(tick.line, "a0") {
			t.Fatal("the oldest line survived the cap")
		}
	}
}

func TestHistoryOfAnUnknownSessionIsNotFound(t *testing.T) {
	r := newRoster(slog.New(slog.DiscardHandler))
	if _, ok := r.historyOf("nobody"); ok {
		t.Fatal("an unknown key was found")
	}
}

func TestReceivedRingRecordsWhoHeardIt(t *testing.T) {
	// The store cannot answer this: a signal is fanned out by meaning and one row
	// is read by every listener, so the poster's key is all it holds.
	s := newSignals(slog.New(slog.DiscardHandler))
	at := time.Now().Add(-90 * time.Second).UnixMilli()
	s.recordReceived("sk-listener", []proto.Signal{
		{Topic: "tests:green", Data: json.RawMessage(`{"n":1}`), AtMS: at},
		{Topic: "deploy:done", AtMS: at},
	})

	got := s.receivedBy("sk-listener")
	if len(got) != 2 || got[0].topic != "tests:green" || got[1].topic != "deploy:done" {
		t.Fatalf("received = %+v", got)
	}
	if got[0].data != `{"n":1}` {
		t.Fatalf("payload glimpse = %q", got[0].data)
	}
	if other := s.receivedBy("sk-someone-else"); len(other) != 0 {
		t.Fatalf("a second session was handed %d signals it never heard", len(other))
	}
}

func TestReceivedRingIsBoundedAndForgettable(t *testing.T) {
	s := newSignals(slog.New(slog.DiscardHandler))
	for i := range signalsKeep * 2 {
		s.recordReceived("sk-1", []proto.Signal{{Topic: "t" + itoa(i)}})
	}
	if got := s.receivedBy("sk-1"); len(got) != signalsKeep {
		t.Fatalf("ring holds %d, want the cap %d", len(got), signalsKeep)
	}
	s.forgetReceived("sk-1")
	if got := s.receivedBy("sk-1"); len(got) != 0 {
		t.Fatalf("a forgotten session still holds %d signals", len(got))
	}
}

func TestRecordReceivedIgnoresAKeylessCaller(t *testing.T) {
	s := newSignals(slog.New(slog.DiscardHandler))
	s.recordReceived("", []proto.Signal{{Topic: "t"}})
	s.recordReceived("  ", []proto.Signal{{Topic: "t"}})
	if len(s.received) != 0 {
		t.Fatalf("keyless callers left %d rings behind", len(s.received))
	}
}

func TestGlimpseBoundsAPayload(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := glimpse(long)
	if len([]rune(got)) != 81 || !strings.HasSuffix(got, "…") {
		t.Fatalf("glimpse returned %d runes ending %q", len([]rune(got)), got[len(got)-3:])
	}
	if short := glimpse("  hi  "); short != "hi" {
		t.Fatalf("short payload came back %q", short)
	}
}

func TestSinceMSNeverGoesNegative(t *testing.T) {
	// A clock that moved backwards would otherwise render as a thing that has not
	// happened yet.
	now := time.Now()
	if got := sinceMS(now, now.Add(time.Minute)); got != 0 {
		t.Fatalf("a future time reported %d ms ago", got)
	}
	if got := sinceMS(now, time.Time{}); got != 0 {
		t.Fatalf("a zero time reported %d ms ago", got)
	}
	if got := sinceMS(now, now.Add(-2*time.Second)); got < 1900 || got > 2100 {
		t.Fatalf("two seconds ago reported as %d ms", got)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
