package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state", "agentbox.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func testItem(id string) *proto.Item {
	return &proto.Item{
		ID:       id,
		Kind:     proto.KindChoice,
		Title:    "Deploy?",
		Body:     "to staging",
		Options:  []proto.Option{{Label: "Yes"}, {Label: "No", Desc: "abort"}},
		Identity: proto.Identity{Agent: "claude-code", Project: "devtool"},
	}
}

func TestOpenCreatesMissingDirAndDB(t *testing.T) {
	s := openTemp(t)
	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if v != len(ms) {
		t.Fatalf("schema version = %d, want %d (all embedded migrations applied)", v, len(ms))
	}
}

func TestFormFieldsAndValuesRoundTrip(t *testing.T) {
	s := openTemp(t)
	it := &proto.Item{
		ID: "f1", Kind: proto.KindForm, Title: "Release",
		Fields: []proto.Field{
			{Key: "env", Type: proto.FieldChoice, Options: []string{"staging", "prod"}, Default: "staging"},
			{Key: "tag", Type: proto.FieldText},
			{Key: "notify", Type: proto.FieldBool, Default: "yes"},
		},
		Identity: proto.Identity{Agent: "test"},
	}
	if err := s.CreateItem(it); err != nil {
		t.Fatal(err)
	}
	pending, err := s.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending[0].Fields) != 3 || pending[0].Fields[0].Options[1] != "prod" {
		t.Fatalf("fields not preserved: %+v", pending[0].Fields)
	}

	values := map[string]string{"env": "prod", "tag": "v1.4.0", "notify": "no"}
	if err := s.Resolve("f1", StateAnswered, Outcome{Values: values}); err != nil {
		t.Fatal(err)
	}
	recent, err := s.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	got := recent[0].Values
	if got["env"] != "prod" || got["tag"] != "v1.4.0" || got["notify"] != "no" {
		t.Fatalf("values not preserved: %+v", got)
	}
}

func TestActionsAndCwdRoundTrip(t *testing.T) {
	s := openTemp(t)
	it := &proto.Item{
		ID: "a1", Kind: proto.KindNotify, Title: "PR opened",
		Cwd: "/home/dev/repo",
		Actions: []proto.Action{
			{Label: "Open PR", Exec: "gh pr view --web"},
			{Label: "View CI", Exec: "gh run watch"},
		},
		Identity: proto.Identity{Agent: "test"},
	}
	if err := s.CreateItem(it); err != nil {
		t.Fatal(err)
	}
	pending, err := s.Pending()
	if err != nil {
		t.Fatal(err)
	}
	got := pending[0]
	if got.Cwd != "/home/dev/repo" {
		t.Fatalf("cwd not preserved: %q", got.Cwd)
	}
	if len(got.Actions) != 2 || got.Actions[0].Label != "Open PR" || got.Actions[1].Exec != "gh run watch" {
		t.Fatalf("actions not preserved: %+v", got.Actions)
	}
}

func TestStats(t *testing.T) {
	s := openTemp(t)

	mk := func(id, agent string, kind proto.Kind) *proto.Item {
		it := &proto.Item{ID: id, Kind: kind, Title: "t", Identity: proto.Identity{Agent: agent}}
		if kind != proto.KindNotify {
			it.Options = []proto.Option{{Label: "Yes"}, {Label: "No"}}
		}
		return it
	}
	// alpha: 2 questions + 1 notify; beta: 1 question.
	for _, it := range []*proto.Item{
		mk("a1", "alpha", proto.KindChoice),
		mk("a2", "alpha", proto.KindChoice),
		mk("a3", "alpha", proto.KindNotify),
		mk("b1", "beta", proto.KindConfirm),
	} {
		if err := s.CreateItem(it); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(5 * time.Millisecond) // resolved_at must outrun created_at for a positive median
	for _, id := range []string{"a1", "a2", "b1"} {
		if err := s.Resolve(id, StateAnswered, Outcome{Answer: "Yes"}); err != nil {
			t.Fatalf("resolve %s: %v", id, err)
		}
	}

	st, err := s.Stats(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 4 || st.Questions != 3 || st.Answered != 3 {
		t.Fatalf("totals wrong: %+v", st)
	}
	if st.MedianAnswerMS <= 0 {
		t.Fatalf("median time-to-answer should be positive, got %d", st.MedianAnswerMS)
	}
	if len(st.ByAgent) != 2 || st.ByAgent[0].Agent != "alpha" || st.ByAgent[0].Total != 3 || st.ByAgent[0].Questions != 2 {
		t.Fatalf("by-agent (busiest first): %+v", st.ByAgent)
	}
	if st.ByAgent[1].Agent != "beta" || st.ByAgent[1].Answered != 1 {
		t.Fatalf("beta slice: %+v", st.ByAgent[1])
	}
	if len(st.ByDay) != 1 || st.ByDay[0].Count != 4 {
		t.Fatalf("by-day: %+v", st.ByDay)
	}

	empty, err := s.Stats(time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if empty.Total != 0 {
		t.Fatalf("future cutoff should exclude everything, got %d", empty.Total)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentbox.db")
	for i := range 3 {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open #%d: %v", i+1, err)
		}
		s.Close()
	}
}

func TestSchemaTooNewRefuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentbox.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (99, '0099_future.sql', 0)`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	_, err = Open(path)
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("got %v, want ErrSchemaTooNew", err)
	}
}

func TestCreateAndReadBack(t *testing.T) {
	s := openTemp(t)
	if err := s.CreateItem(testItem("k1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	pending, err := s.Pending()
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("got %d pending, want 1", len(pending))
	}
	it := pending[0]
	if it.ID != "k1" || it.Kind != proto.KindChoice || it.Title != "Deploy?" {
		t.Fatalf("read back mismatch: %+v", it)
	}
	if len(it.Options) != 2 || it.Options[1].Desc != "abort" {
		t.Fatalf("options not preserved: %+v", it.Options)
	}
	if it.Identity.Agent != "claude-code" || it.Identity.Project != "devtool" {
		t.Fatalf("identity not preserved: %+v", it.Identity)
	}
	if it.CreatedAt.IsZero() {
		t.Fatal("created_at not set")
	}
}

func TestMissedWhileAwayRoundTrip(t *testing.T) {
	s := openTemp(t)
	if err := s.CreateItem(testItem("k1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.Resolve("k1", StateDismissed, Outcome{MissedAway: true}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	recent, err := s.Recent(1)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if !recent[0].MissedWhileAway {
		t.Fatal("missed_while_away did not persist")
	}
	// The column defaults off for a plain answer (FR44 marks only idle-lapsed toasts).
	if err := s.CreateItem(testItem("k2")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.Resolve("k2", StateAnswered, Outcome{Answer: "Yes"}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	recent, err = s.Recent(1)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if recent[0].ID != "k2" || recent[0].MissedWhileAway {
		t.Fatalf("plain answer should not be missed-while-away: %+v", recent[0])
	}
}

func TestNeverShownRoundTrip(t *testing.T) {
	s := openTemp(t)
	if err := s.CreateItem(testItem("k1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.Resolve("k1", StateDismissed, Outcome{NeverShown: true}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	recent, err := s.Recent(1)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if !recent[0].NeverShown {
		t.Fatal("never_shown did not persist")
	}
	// Two markers, two columns: R-06's row must not read as FR44's.
	if recent[0].MissedWhileAway {
		t.Fatalf("never-shown row also claims missed-while-away: %+v", recent[0])
	}
	// And a toast that was on screen and lapsed keeps the column off.
	if err := s.CreateItem(testItem("k2")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.Resolve("k2", StateDismissed, Outcome{MissedAway: true}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	recent, err = s.Recent(1)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if recent[0].ID != "k2" || recent[0].NeverShown {
		t.Fatalf("a toast that appeared should not be never-shown: %+v", recent[0])
	}
}

func TestCreateWithoutIDFails(t *testing.T) {
	s := openTemp(t)
	it := testItem("")
	if err := s.CreateItem(it); err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestResolveExactlyOnce(t *testing.T) {
	s := openTemp(t)
	if err := s.CreateItem(testItem("k1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Resolve("k1", StateAnswered, Outcome{Answer: "Yes"}); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	err := s.Resolve("k1", StateAnswered, Outcome{Answer: "No"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("second resolve: got %v, want ErrNotFound", err)
	}

	recent, err := s.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if recent[0].Answer != "Yes" {
		t.Fatalf("answer = %q, want Yes (second resolve must not overwrite)", recent[0].Answer)
	}
}

func TestResolveUnknownItem(t *testing.T) {
	s := openTemp(t)
	if err := s.Resolve("ghost", StateExpired, Outcome{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestPendingSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentbox.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateItem(testItem("k1")); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateItem(testItem("k2")); err != nil {
		t.Fatal(err)
	}
	if err := s.Resolve("k2", StateCancelled, Outcome{}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	pending, err := s2.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "k1" {
		t.Fatalf("pending after reopen = %+v, want only k1", pending)
	}
}

func TestTransitionsAudit(t *testing.T) {
	s := openTemp(t)
	if err := s.CreateItem(testItem("k1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Resolve("k1", StateAnswered, Outcome{Answer: "Yes"}); err != nil {
		t.Fatal(err)
	}
	trail, err := s.Transitions("k1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"(new) -> pending", "pending -> answered"}
	if len(trail) != len(want) {
		t.Fatalf("trail = %v, want %v", trail, want)
	}
	for i := range want {
		if trail[i] != want[i] {
			t.Fatalf("trail[%d] = %q, want %q", i, trail[i], want[i])
		}
	}
}

func TestRecentOrderAndLimit(t *testing.T) {
	s := openTemp(t)
	for _, id := range []string{"a", "b", "c"} {
		if err := s.CreateItem(testItem(id)); err != nil {
			t.Fatal(err)
		}
	}
	recent, err := s.Recent(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 {
		t.Fatalf("got %d items, want 2", len(recent))
	}
}

func (s *Store) ageItem(t *testing.T, id string, age time.Duration) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE items SET created_at = ? WHERE id = ?`,
		time.Now().Add(-age).UnixMilli(), id); err != nil {
		t.Fatal(err)
	}
}

func TestPruneEvictsOldUnimportantOnly(t *testing.T) {
	s := openTemp(t)
	month := 30 * 24 * time.Hour

	mk := func(id string, level proto.Level, resolved bool, age time.Duration) {
		it := testItem(id)
		it.Kind = proto.KindNotify
		it.Options = nil
		it.Level = level
		if err := s.CreateItem(it); err != nil {
			t.Fatal(err)
		}
		if resolved {
			if err := s.Resolve(id, StateDismissed, Outcome{}); err != nil {
				t.Fatal(err)
			}
		}
		s.ageItem(t, id, age)
	}

	mk("old-info", proto.LevelInfo, true, 40*24*time.Hour)        // evict
	mk("old-success", proto.LevelSuccess, true, 40*24*time.Hour)  // evict
	mk("fresh-info", proto.LevelInfo, true, 5*24*time.Hour)       // keep: young
	mk("old-warning", proto.LevelWarning, true, 400*24*time.Hour) // keep: important
	mk("old-error", proto.LevelError, true, 400*24*time.Hour)     // keep: important
	mk("old-pending", proto.LevelInfo, false, 400*24*time.Hour)   // keep: pending

	n, err := s.Prune(month, proto.LevelWarning)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 2 {
		t.Fatalf("pruned %d items, want 2", n)
	}

	recent, err := s.Recent(100)
	if err != nil {
		t.Fatal(err)
	}
	left := map[string]bool{}
	for _, it := range recent {
		left[it.ID] = true
	}
	for _, want := range []string{"fresh-info", "old-warning", "old-error", "old-pending"} {
		if !left[want] {
			t.Errorf("%s was evicted, must be kept", want)
		}
	}
	for _, gone := range []string{"old-info", "old-success"} {
		if left[gone] {
			t.Errorf("%s survived, must be evicted", gone)
		}
	}

	// The audit trail of evicted items goes with them.
	trail, err := s.Transitions("old-info")
	if err != nil {
		t.Fatal(err)
	}
	if len(trail) != 0 {
		t.Fatalf("transitions of evicted item remain: %v", trail)
	}
	trail, err = s.Transitions("old-warning")
	if err != nil {
		t.Fatal(err)
	}
	if len(trail) == 0 {
		t.Fatal("transitions of kept item were lost")
	}
}

func TestPruneKeepEverythingThreshold(t *testing.T) {
	s := openTemp(t)
	it := testItem("k1")
	it.Level = proto.LevelInfo
	if err := s.CreateItem(it); err != nil {
		t.Fatal(err)
	}
	if err := s.Resolve("k1", StateDismissed, Outcome{}); err != nil {
		t.Fatal(err)
	}
	s.ageItem(t, "k1", 400*24*time.Hour)

	n, err := s.Prune(30*24*time.Hour, proto.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("pruned %d with keep-everything threshold, want 0", n)
	}
}

func TestReplyStoredSeparately(t *testing.T) {
	s := openTemp(t)
	if err := s.CreateItem(testItem("k1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Resolve("k1", StateAnswered, Outcome{Reply: "do it differently"}); err != nil {
		t.Fatal(err)
	}
	recent, err := s.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	if recent[0].Answer != "" || recent[0].Reply != "do it differently" {
		t.Fatalf("got answer=%q reply=%q", recent[0].Answer, recent[0].Reply)
	}
}

// Migration 0012's three columns. Speak and diff were written into FR73's
// read-back and taken out again when the insert turned out not to name them;
// session_key is the only identity that names one session.
func TestItemKeepsSessionKeySpeakAndDiff(t *testing.T) {
	s := openTemp(t)
	it := proto.Item{
		ID: "k000000000000001", Kind: proto.KindDiff, Title: "apply this?",
		Diff:     "--- a/f.go\n+++ b/f.go\n@@ -1 +1 @@\n-old\n+new\n",
		Speak:    "a patch is waiting for you",
		Identity: proto.Identity{Agent: "claude", Project: "agentbox", Key: "sk-one"},
	}
	if err := s.CreateItem(&it); err != nil {
		t.Fatal(err)
	}
	got, err := s.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("recent = %d items", len(got))
	}
	if got[0].Diff != it.Diff {
		t.Fatalf("diff came back %q", got[0].Diff)
	}
	if got[0].Speak != it.Speak {
		t.Fatalf("speak came back %q", got[0].Speak)
	}
	if got[0].Identity.Key != "sk-one" {
		t.Fatalf("session key came back %q", got[0].Identity.Key)
	}
}

func TestRecentBySessionSeesOnlyThatSession(t *testing.T) {
	s := openTemp(t)
	mk := func(id, key string) {
		it := proto.Item{ID: id, Kind: proto.KindNotify, Title: id,
			Identity: proto.Identity{Agent: "claude", Key: key}}
		if err := s.CreateItem(&it); err != nil {
			t.Fatal(err)
		}
	}
	mk("k000000000000001", "sk-one")
	mk("k000000000000002", "sk-two")
	mk("k000000000000003", "sk-one")
	mk("k000000000000004", "") // pre-0012 shape: no author recorded

	got, err := s.RecentBySession("sk-one", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items for sk-one, want 2", len(got))
	}
	// An empty key is not a wildcard: the rows with no recorded author belong to
	// no row on the board, and returning them would put them on every row.
	blank, err := s.RecentBySession("", 10)
	if err != nil || len(blank) != 0 {
		t.Fatalf("empty key returned %d items (err %v), want none", len(blank), err)
	}
}
