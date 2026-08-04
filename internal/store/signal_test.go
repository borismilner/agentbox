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
	oldest, newest, err := st.SignalBounds()
	if err != nil {
		t.Fatalf("bounds: %v", err)
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
	oldest, newest, err := st.SignalBounds()
	if err != nil {
		t.Fatalf("bounds: %v", err)
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
