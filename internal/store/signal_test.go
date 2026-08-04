package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

func newSignalStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "agentbox.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func post(t *testing.T, st *Store, topic, data string) proto.Signal {
	t.Helper()
	sig, err := st.PostSignal(topic, proto.Identity{Agent: "claude", Key: "k1"}, data)
	if err != nil {
		t.Fatalf("post %s: %v", topic, err)
	}
	return sig
}

func TestPostSignalNumbersOneGlobalSequence(t *testing.T) {
	st := newSignalStore(t)
	a := post(t, st, "tests:green", `{"suite":"race"}`)
	b := post(t, st, "done:one", "")
	if a.Seq != 1 || b.Seq != 2 {
		t.Fatalf("seq should be one global counter, got %d and %d", a.Seq, b.Seq)
	}
	if string(a.Data) != `{"suite":"race"}` {
		t.Fatalf("payload came back as %q", a.Data)
	}
	if b.Data != nil {
		t.Fatalf("an empty payload should stay empty, got %q", b.Data)
	}
	if a.Agent != "claude" || a.Key != "k1" {
		t.Fatalf("the poster's identity should ride along, got %+v", a)
	}
	if a.AtMS == 0 {
		t.Fatal("a signal should be stamped")
	}
}

func TestSignalsSinceMatchesExactAndPrefix(t *testing.T) {
	st := newSignalStore(t)
	post(t, st, "tests:green", "")
	post(t, st, "done:one", "")
	post(t, st, "done:two", "")
	post(t, st, "build:failed", "")

	got, more, err := st.SignalsSince(0, []string{"tests:green", "done:*"}, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if more {
		t.Fatal("four signals should not report more")
	}
	if len(got) != 3 {
		t.Fatalf("want the exact one and both prefixed ones, got %d", len(got))
	}
	// Sequence order, always: the cursor is only resumable if the batch is ordered.
	for i := 1; i < len(got); i++ {
		if got[i].Seq <= got[i-1].Seq {
			t.Fatalf("batch is out of order: %+v", got)
		}
	}
	// And the SQL predicate must agree with the Go matcher, or a signal delivered
	// live would be missing from the batch a restarted agent reads back.
	for _, sig := range got {
		if !proto.TopicsMatch([]string{"tests:green", "done:*"}, sig.Topic) {
			t.Fatalf("SQL returned %q, which the Go matcher rejects", sig.Topic)
		}
	}
}

func TestSignalsSinceHonoursTheCursor(t *testing.T) {
	st := newSignalStore(t)
	post(t, st, "t", "")
	second := post(t, st, "t", "")
	post(t, st, "t", "")

	got, _, err := st.SignalsSince(second.Seq, []string{"t"}, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 || got[0].Seq != second.Seq+1 {
		t.Fatalf("a cursor should exclude itself and everything before it, got %+v", got)
	}
}

func TestSignalsSinceCaps(t *testing.T) {
	st := newSignalStore(t)
	for range 5 {
		post(t, st, "t", "")
	}
	got, more, err := st.SignalsSince(0, []string{"t"}, 2)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 2 || !more {
		t.Fatalf("want 2 with more=true, got %d with more=%v", len(got), more)
	}
	// The cap must be a boundary and not a truncation that loses its place: the
	// next read from the last returned seq picks up exactly the rest.
	rest, more, err := st.SignalsSince(got[1].Seq, []string{"t"}, 2)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rest) != 2 || !more || rest[0].Seq != got[1].Seq+1 {
		t.Fatalf("the next page should continue from the cursor, got %+v", rest)
	}
}

func TestSignalsSinceNoUsablePatternMatchesNothing(t *testing.T) {
	st := newSignalStore(t)
	post(t, st, "tests:green", "")
	got, _, err := st.SignalsSince(0, []string{"", "  "}, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("blank patterns must match nothing rather than everything, got %d", len(got))
	}
}

// A topic with a LIKE metacharacter in it must not become a wildcard. This is the
// one place the SQL path could silently deliver more than the Go matcher would.
func TestSignalsSinceEscapesLikeWildcards(t *testing.T) {
	st := newSignalStore(t)
	post(t, st, "claims/a_b:1", "")
	post(t, st, "claims/aXb:1", "")

	got, _, err := st.SignalsSince(0, []string{"claims/a_b*"}, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 || got[0].Topic != "claims/a_b:1" {
		t.Fatalf("an underscore in a prefix is a literal, not a single-character wildcard: got %+v", got)
	}
}

func TestSignalBoundsAndTrimByCount(t *testing.T) {
	st := newSignalStore(t)
	for range 5 {
		post(t, st, "chatty", "")
	}
	post(t, st, "quiet", "")

	n, err := st.TrimSignals(2, 0)
	if err != nil {
		t.Fatalf("trim: %v", err)
	}
	if n != 3 {
		t.Fatalf("keeping 2 of 5 should drop 3, dropped %d", n)
	}
	// Per topic, not globally: the quiet topic's only signal survives a chatty
	// neighbour, which is the whole reason the count is per topic.
	quiet, _, err := st.SignalsSince(0, []string{"quiet"}, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(quiet) != 1 {
		t.Fatalf("a quiet topic must not be evicted by a chatty one, got %d", len(quiet))
	}
	oldest, err := st.SignalOldest()
	if err != nil {
		t.Fatalf("oldest: %v", err)
	}
	newest, err := st.SignalHighWater()
	if err != nil {
		t.Fatalf("high water: %v", err)
	}
	if oldest != 4 || newest != 6 {
		t.Fatalf("bounds after the trim = %d..%d, want 4..6", oldest, newest)
	}
}

func TestTrimSignalsByAge(t *testing.T) {
	st := newSignalStore(t)
	old := post(t, st, "t", "")
	// Backdate it: the trim reads at_ms, so this is the only honest way to make a
	// signal a week old inside a test.
	if _, err := st.db.Exec(`UPDATE sync_signals SET at_ms = ? WHERE seq = ?`,
		time.Now().Add(-8*24*time.Hour).UnixMilli(), old.Seq); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	fresh := post(t, st, "t", "")

	n, err := st.TrimSignals(0, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("trim: %v", err)
	}
	if n != 1 {
		t.Fatalf("one signal was older than the window, dropped %d", n)
	}
	got, _, err := st.SignalsSince(0, []string{"t"}, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 || got[0].Seq != fresh.Seq {
		t.Fatalf("the fresh signal should survive, got %+v", got)
	}
}

// The high-water mark is what tells "nothing ever happened" apart from "your
// cursor fell off the edge", and it is also what stops a reused rowid handing an
// old cursor somebody else's signal.
func TestSignalSequenceNeverGoesBackwards(t *testing.T) {
	st := newSignalStore(t)
	for range 3 {
		post(t, st, "t", "")
	}
	if _, err := st.db.Exec(`DELETE FROM sync_signals`); err != nil {
		t.Fatalf("clear: %v", err)
	}
	oldest, err := st.SignalOldest()
	if err != nil {
		t.Fatalf("oldest: %v", err)
	}
	newest, err := st.SignalHighWater()
	if err != nil {
		t.Fatalf("high water: %v", err)
	}
	if oldest != 0 || newest != 3 {
		t.Fatalf("an empty table should read 0..3, got %d..%d", oldest, newest)
	}
	next := post(t, st, "t", "")
	if next.Seq != 4 {
		t.Fatalf("the sequence must continue past a full trim, got %d", next.Seq)
	}
}

// TrimTopic is the post-time path: one topic, one indexed delete, and nobody
// else's history touched.
func TestTrimTopicTouchesOneTopicOnly(t *testing.T) {
	st := newSignalStore(t)
	for range 4 {
		post(t, st, "chatty", "")
	}
	post(t, st, "quiet", "")

	n, err := st.TrimTopic("chatty", 2)
	if err != nil {
		t.Fatalf("trim topic: %v", err)
	}
	if n != 2 {
		t.Fatalf("keeping 2 of 4 should drop 2, dropped %d", n)
	}
	quiet, _, err := st.SignalsSince(0, []string{"quiet"}, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(quiet) != 1 {
		t.Fatalf("another topic must be untouched, got %d", len(quiet))
	}
	if n, err := st.TrimTopic("quiet", 2); err != nil || n != 0 {
		t.Fatalf("a topic under the cap should lose nothing, got %d %v", n, err)
	}
}

// The bug a live run found within an hour of shipping 0008, kept as a test so it
// cannot come back. Retention is PER TOPIC, so the global oldest surviving
// sequence says nothing about whether the topic a caller asked about was trimmed:
// here seq 1 survives on a quiet topic while seq 2 and 3 are taken from the busy
// one, and a cursor of 1 must still be told.
func TestSignalGapIsPerTopicNotGlobal(t *testing.T) {
	st := newSignalStore(t)
	post(t, st, "quiet", "") // seq 1, never trimmed
	post(t, st, "busy", "")  // seq 2
	post(t, st, "busy", "")  // seq 3
	post(t, st, "busy", "")  // seq 4
	if _, err := st.TrimTopic("busy", 1); err != nil {
		t.Fatalf("trim: %v", err)
	}
	// The global minimum is still 1, so the plausible check would find no gap.
	if oldest, err := st.SignalOldest(); err != nil || oldest != 1 {
		t.Fatalf("expected the quiet topic to hold the global minimum at 1, got %d %v", oldest, err)
	}
	trimmedTo, err := st.SignalGap(1, []string{"busy"})
	if err != nil {
		t.Fatalf("gap: %v", err)
	}
	if trimmedTo != 3 {
		t.Fatalf("a cursor of 1 on busy lost seq 2 and 3, so the watermark is 3, got %d", trimmedTo)
	}
	// A cursor at or above the watermark reads completely.
	if trimmedTo, err := st.SignalGap(3, []string{"busy"}); err != nil || trimmedTo != 0 {
		t.Fatalf("a cursor of 3 lost nothing, got %d %v", trimmedTo, err)
	}
	// And an untouched topic reports nothing, which is why the watermark is per
	// topic: a global one would cry gap at every unrelated cursor.
	if trimmedTo, err := st.SignalGap(1, []string{"quiet"}); err != nil || trimmedTo != 0 {
		t.Fatalf("the quiet topic was never trimmed, got %d %v", trimmedTo, err)
	}
}

// A topic aged away entirely leaves no signals to reason from, which is the case
// the wrong version of this check could not answer at all.
func TestSignalGapSurvivesATopicTrimmedToNothing(t *testing.T) {
	st := newSignalStore(t)
	// A first signal so the ephemeral one is not seq 1: a cursor of zero means
	// "from now on" and by definition cannot have missed anything.
	post(t, st, "anchor", "")
	gone := post(t, st, "ephemeral", "")
	if _, err := st.db.Exec(`UPDATE sync_signals SET at_ms = ? WHERE seq = ?`,
		time.Now().Add(-8*24*time.Hour).UnixMilli(), gone.Seq); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if n, err := st.TrimSignals(0, 7*24*time.Hour); err != nil || n != 1 {
		t.Fatalf("trim by age dropped %d %v", n, err)
	}
	trimmedTo, err := st.SignalGap(gone.Seq-1, []string{"ephemeral"})
	if err != nil {
		t.Fatalf("gap: %v", err)
	}
	if trimmedTo != gone.Seq {
		t.Fatalf("the watermark should outlive the topic's signals, got %d", trimmedTo)
	}
	// A prefix pattern has to reach it too, or `done:*` misses a chunk topic that
	// was aged out whole.
	if trimmedTo, err := st.SignalGap(gone.Seq-1, []string{"ephem*"}); err != nil || trimmedTo != gone.Seq {
		t.Fatalf("a prefix pattern should match the trim record, got %d %v", trimmedTo, err)
	}
}

// The watermark must be recorded before the rows go, so that a process dying
// between the two over-reports a gap rather than hiding one. A reader told about a
// gap that is not there re-reads and pays a call; a reader served a hole and told
// nothing acts on a false history.
func TestTrimRecordsTheWatermarkBeforeDeleting(t *testing.T) {
	st := newSignalStore(t)
	post(t, st, "anchor", "")
	post(t, st, "t", "") // seq 2
	post(t, st, "t", "") // seq 3

	// A recordTrim that cannot write must abort the trim rather than delete first
	// and lose the record. Forced by dropping the table the record goes into.
	if _, err := st.db.Exec(`DROP TABLE sync_signal_trim`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := st.TrimTopic("t", 1); err == nil {
		t.Fatal("a trim whose record cannot be written must fail")
	}
	got, _, err := st.SignalsSince(0, []string{"t"}, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("nothing may be deleted when the watermark could not be recorded, got %d", len(got))
	}
}
