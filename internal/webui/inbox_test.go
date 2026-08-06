package webui

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// The inbox's Go side is where the surface's meaning lives - which key answers
// what, what a resolved item is called, which rows come first - so it is the
// part worth testing. Painting is the webview's job and is checked by eye.

func item(id string, kind proto.Kind, state string) store.StoredItem {
	return store.StoredItem{
		Item:      proto.Item{ID: id, Kind: kind, Identity: proto.Identity{Agent: "claude-code"}},
		State:     state,
		CreatedAt: time.Now(),
	}
}

func TestTriageForMapsKeysPerKind(t *testing.T) {
	choice := item("c", proto.KindChoice, store.StatePending)
	choice.Options = []proto.Option{{Label: "Run now"}, {Label: "Dry run"}, {Label: "Skip"}}
	choice.Default = "Dry run"

	confirm := item("y", proto.KindConfirm, store.StatePending)
	confirm.Default = "no"

	tests := []struct {
		name string
		it   store.StoredItem
		key  string
		want triageCmd
	}{
		{"choice digit answers that option", choice, "2", triageCmd{triageAnswer, "Dry run"}},
		{"choice digit past the options does nothing", choice, "4", triageCmd{intent: triageNone}},
		{"choice enter takes the default", choice, "Enter", triageCmd{triageAnswer, "Dry run"}},
		{"choice d dismisses (FR50)", choice, "d", triageCmd{intent: triageDismiss}},
		{"backspace dismisses too", choice, "Backspace", triageCmd{intent: triageDismiss}},
		{"confirm y", confirm, "y", triageCmd{triageAnswer, "yes"}},
		{"confirm n", confirm, "n", triageCmd{triageAnswer, "no"}},
		{"confirm enter takes the default", confirm, "Enter", triageCmd{triageAnswer, "no"}},
		{"confirm digit means nothing", confirm, "1", triageCmd{intent: triageNone}},
		{"veto s stops it", item("v", proto.KindVeto, store.StatePending), "s", triageCmd{intent: triageVeto}},
		{"veto cannot be walked away from", item("v", proto.KindVeto, store.StatePending), "d", triageCmd{intent: triageNone}},
		{"diff cannot be walked away from", item("r", proto.KindDiff, store.StatePending), "Backspace", triageCmd{intent: triageNone}},
		{"diff enter opens the card", item("r", proto.KindDiff, store.StatePending), "Enter", triageCmd{intent: triagePromote}},
		{"text needs its card", item("t", proto.KindText, store.StatePending), "Enter", triageCmd{intent: triagePromote}},
		{"secret needs its card", item("s", proto.KindSecret, store.StatePending), "Enter", triageCmd{intent: triagePromote}},
		{"form needs its card", item("f", proto.KindForm, store.StatePending), "Enter", triageCmd{intent: triagePromote}},
		{"notify dismisses", item("n", proto.KindNotify, store.StatePending), "d", triageCmd{intent: triageDismiss}},
		{"an unmapped key is ignored", choice, "q", triageCmd{intent: triageNone}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := triageFor(tc.it, tc.key); got != tc.want {
				t.Fatalf("triageFor(%s, %q) = %+v, want %+v", tc.it.Kind, tc.key, got, tc.want)
			}
		})
	}
}

// A choice with no default must not answer on Enter: Enter would then pick
// whatever option happened to be first, which is exactly the accident the
// keyboard map exists to prevent.
func TestTriageEnterNeedsADefault(t *testing.T) {
	it := item("c", proto.KindChoice, store.StatePending)
	it.Options = []proto.Option{{Label: "Run now"}, {Label: "Skip"}}
	if got := triageFor(it, "Enter"); got.intent != triageNone {
		t.Fatalf("Enter without a default = %+v, want none", got)
	}
}

func TestTriageHintStatesTheKeysThatWork(t *testing.T) {
	choice := item("c", proto.KindChoice, store.StatePending)
	choice.Options = []proto.Option{{Label: "Run now"}, {Label: "Skip"}}
	choice.Default = "Skip"
	h := triageHint(choice)
	for _, want := range []string{"1 run now", "2 skip", "enter skip", "d dismiss", "c copy"} {
		if !strings.Contains(h, want) {
			t.Errorf("choice hint %q missing %q", h, want)
		}
	}

	// The hint must not offer a key the item does not honour.
	if h := triageHint(item("v", proto.KindVeto, store.StatePending)); strings.Contains(h, "dismiss") {
		t.Errorf("veto hint offers dismiss: %q", h)
	}
	if h := triageHint(item("r", proto.KindDiff, store.StatePending)); strings.Contains(h, "dismiss") {
		t.Errorf("diff hint offers dismiss: %q", h)
	}
}

func TestOutcomeOf(t *testing.T) {
	answered := item("a", proto.KindChoice, store.StateAnswered)
	answered.Answer = "Dry run"
	replied := item("b", proto.KindText, store.StateAnswered)
	replied.Reply = "v0.4.1"
	secret := item("s", proto.KindSecret, store.StateAnswered) // no answer, no reply
	missed := item("m", proto.KindNotify, store.StateExpired)
	missed.MissedWhileAway = true

	tests := []struct {
		name string
		it   store.StoredItem
		text string
		tone string
	}{
		{"pending waits", item("p", proto.KindChoice, store.StatePending), "waiting", "info"},
		{"the answer is the outcome", answered, "Dry run", "success"},
		{"a free-text answer reads as replied", replied, "replied", "success"},
		{"answered with no label", secret, "answered", "success"},
		{"expired", item("e", proto.KindChoice, store.StateExpired), "expired", "muted"},
		{"cancelled", item("x", proto.KindChoice, store.StateCancelled), "cancelled", "muted"},
		// A veto's states mean the opposite of the words: expired means it went
		// ahead, answered means it was stopped.
		{"veto pending", item("v", proto.KindVeto, store.StatePending), "deciding", "info"},
		{"veto answered was stopped", item("v", proto.KindVeto, store.StateAnswered), "vetoed", "error"},
		{"veto expired went ahead", item("v", proto.KindVeto, store.StateExpired), "proceeded", "success"},
		{"missed while away wins (FR44)", missed, "missed while away", "warning"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text, tone := outcomeOf(tc.it)
			if text != tc.text || tone != tc.tone {
				t.Fatalf("outcomeOf = (%q, %q), want (%q, %q)", text, tone, tc.text, tc.tone)
			}
		})
	}
}

func TestPendingFirstKeepsOrderWithinGroups(t *testing.T) {
	in := []store.StoredItem{
		item("r1", proto.KindNotify, store.StateExpired),
		item("p1", proto.KindChoice, store.StatePending),
		item("r2", proto.KindNotify, store.StateAnswered),
		item("p2", proto.KindConfirm, store.StatePending),
	}
	got := pendingFirst(in)
	var ids []string
	for _, it := range got {
		ids = append(ids, it.ID)
	}
	want := "p1 p2 r1 r2"
	if strings.Join(ids, " ") != want {
		t.Fatalf("order = %v, want %s", ids, want)
	}
}

func TestTodayCountUsesTheLocalCalendarDay(t *testing.T) {
	now := time.Date(2026, 7, 24, 9, 30, 0, 0, time.Local)
	mk := func(at time.Time) store.StoredItem {
		it := item("x", proto.KindNotify, store.StateExpired)
		it.CreatedAt = at
		return it
	}
	items := []store.StoredItem{
		mk(now.Add(-30 * time.Minute)), // today
		mk(now.Add(-9 * time.Hour)),    // 00:30 today
		mk(now.Add(-10 * time.Hour)),   // 23:30 yesterday
		mk(now.Add(-48 * time.Hour)),   // two days ago
	}
	if n := todayCount(items, now); n != 2 {
		t.Fatalf("todayCount = %d, want 2", n)
	}
}

func TestSnippetFlattensAndTruncates(t *testing.T) {
	got := snippet("Adds a **non-null** column\nto `events`.", 80)
	want := "Adds a non-null column to events."
	if got != want {
		t.Fatalf("snippet = %q, want %q", got, want)
	}
	long := snippet(strings.Repeat("word ", 60), 40)
	if len([]rune(long)) > 41 || !strings.HasSuffix(long, "…") {
		t.Fatalf("snippet did not truncate cleanly: %q", long)
	}
	if snippet("", 40) != "" {
		t.Fatal("empty body should stay empty")
	}
}

// --- the snapshot and dispatch path ----------------------------------------

type fakeSource struct {
	items    []store.StoredItem
	muted    []string
	promoted []string
	err      error
}

func (f *fakeSource) RecentItems(int) ([]store.StoredItem, error) { return f.items, f.err }
func (f *fakeSource) RecentBySession(key string, _ int) ([]store.StoredItem, error) {
	var out []store.StoredItem
	for _, it := range f.items {
		if it.Identity.Key == key {
			out = append(out, it)
		}
	}
	return out, f.err
}
func (f *fakeSource) Promote(id string)                    { f.promoted = append(f.promoted, id) }
func (f *fakeSource) MutedAgents() []string                { return f.muted }
func (f *fakeSource) Stats(time.Time) (proto.Stats, error) { return proto.Stats{}, nil }

type fakeResolver struct {
	answers map[string]string
	vetoed  []string
	dismiss []string
	opened  [][2]string // FR30: (stack id, item id) pairs the surface asked to open
	events  []proto.ArtifactEvent
}

func (f *fakeResolver) Answer(id, label string) {
	if f.answers == nil {
		f.answers = map[string]string{}
	}
	f.answers[id] = label
}
func (f *fakeResolver) Reply(string, string)                 {}
func (f *fakeResolver) AnswerForm(string, map[string]string) {}
func (f *fakeResolver) Dismiss(id string)                    { f.dismiss = append(f.dismiss, id) }
func (f *fakeResolver) Defer(string)                         {}
func (f *fakeResolver) Undo(string)                          {}
func (f *fakeResolver) Veto(id string)                       { f.vetoed = append(f.vetoed, id) }
func (f *fakeResolver) Secret(string, string)                {}
func (f *fakeResolver) RunAction(string, int)                {}
func (f *fakeResolver) Review(string, bool, string)          {}
func (f *fakeResolver) OpenStacked(stackID, itemID string) {
	f.opened = append(f.opened, [2]string{stackID, itemID})
}
func (f *fakeResolver) ArtifactEvent(ev proto.ArtifactEvent) { f.events = append(f.events, ev) }

// testUI is a UI with no webview behind it: enough for the encoders, the
// snapshot and the dispatch, which is everything decided in Go.
func testUI(res Resolver, src Source) *UI {
	u := &UI{
		res:   res,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		theme: Theme{Mode: "dark"},
	}
	u.inbox = newInbox(u)
	u.sess = newSessions(u)
	u.src = src
	return u
}

func TestSnapshotEncodesRowsPendingFirst(t *testing.T) {
	pending := item("p1", proto.KindChoice, store.StatePending)
	pending.Title = "Run DB migration?"
	pending.Options = []proto.Option{{Label: "Run now"}, {Label: "Skip"}}
	done := item("r1", proto.KindNotify, store.StateExpired)
	done.Identity.Agent = "codex"

	src := &fakeSource{items: []store.StoredItem{done, pending}, muted: []string{"codex"}}
	u := testUI(&fakeResolver{}, src)

	snap := u.inbox.snapshot()
	if snap.Pending != 1 {
		t.Fatalf("pending = %d, want 1", snap.Pending)
	}
	if len(snap.Items) != 2 || snap.Items[0].ID != "p1" {
		t.Fatalf("rows = %+v, want the pending one first", snap.Items)
	}
	if snap.Items[0].Hint == "" {
		t.Error("a pending row should carry its triage hint")
	}
	if snap.Items[1].Hint != "" {
		t.Error("a resolved row has no keys, so no hint")
	}
	if !snap.Items[1].Muted {
		t.Error("codex is muted; the row should say so (FR47)")
	}
	if snap.Items[0].Hue == "" {
		t.Error("every row carries its agent's identity hue")
	}
	if snap.Today != 2 {
		t.Fatalf("today = %d, want 2", snap.Today)
	}
}

func TestSnapshotSurvivesAMissingOrBrokenSource(t *testing.T) {
	u := testUI(&fakeResolver{}, nil)
	if snap := u.inbox.snapshot(); len(snap.Items) != 0 || snap.Items == nil {
		t.Fatalf("no source should give an empty (not nil) row list, got %+v", snap.Items)
	}

	u = testUI(&fakeResolver{}, &fakeSource{err: io.ErrUnexpectedEOF})
	if snap := u.inbox.snapshot(); len(snap.Items) != 0 || snap.Muted == nil {
		t.Fatalf("a store error should degrade to an empty surface, got %+v", snap)
	}
}

func TestActDispatchesAgainstTheDaemon(t *testing.T) {
	choice := item("p1", proto.KindChoice, store.StatePending)
	choice.Options = []proto.Option{{Label: "Run now"}, {Label: "Skip"}}
	veto := item("p2", proto.KindVeto, store.StatePending)
	form := item("p3", proto.KindForm, store.StatePending)
	notify := item("p4", proto.KindNotify, store.StatePending)
	resolved := item("r1", proto.KindChoice, store.StateAnswered)

	res := &fakeResolver{}
	src := &fakeSource{items: []store.StoredItem{choice, veto, form, notify, resolved}}
	u := testUI(res, src)
	u.inbox.snapshot() // act works against the rows the surface was shown

	if !u.inbox.act("p1", "1") {
		t.Fatal("digit on a choice should act")
	}
	if res.answers["p1"] != "Run now" {
		t.Fatalf("answer = %q, want Run now", res.answers["p1"])
	}
	if !u.inbox.act("p2", "s") || len(res.vetoed) != 1 {
		t.Fatal("s should stop the veto")
	}
	if !u.inbox.act("p3", "Enter") || len(src.promoted) != 1 || src.promoted[0] != "p3" {
		t.Fatalf("a form should be promoted to its card, promoted = %v", src.promoted)
	}
	if !u.inbox.act("p4", "d") || len(res.dismiss) != 1 {
		t.Fatal("d should dismiss a notice")
	}

	// Nothing may act on an item that is no longer waiting, or on a key the
	// item does not honour, or on an id that was never on screen.
	if u.inbox.act("r1", "1") {
		t.Error("a resolved item must not be answerable")
	}
	if u.inbox.act("p1", "y") {
		t.Error("y means nothing to a choice")
	}
	if u.inbox.act("nope", "1") {
		t.Error("an unknown id must not act")
	}
}

func TestClipTextOnlyServesRowsOnScreen(t *testing.T) {
	it := item("p1", proto.KindChoice, store.StatePending)
	it.Title = "Run DB migration?"
	u := testUI(&fakeResolver{}, &fakeSource{items: []store.StoredItem{it}})
	u.inbox.snapshot()

	got := u.inbox.clipText("p1")
	if !strings.Contains(got, "Run DB migration?") || !strings.Contains(got, "agentbox p1") {
		t.Fatalf("clipText = %q, want the pasteable item form (FR43)", got)
	}
	if u.inbox.clipText("nope") != "" {
		t.Error("an unknown id must copy nothing")
	}
}

// --- reading a row back (FR73) ----------------------------------------------

// The defect in one test: a card that resolved must still give up its whole body.
// The row's snippet stops at 140 characters, so a body longer than that is the
// case that used to be unrecoverable.
func TestDetailReadsAResolvedItemBackWhole(t *testing.T) {
	long := strings.Repeat("The migration rewrites every row in the audit table. ", 8)
	it := item("r1", proto.KindVeto, store.StateExpired)
	it.Title = "Rewriting the audit table"
	it.Body = "**Heads up.** " + long
	it.CreatedAt = time.Date(2026, 8, 5, 14, 32, 0, 0, time.Local)
	it.ResolvedAt = it.CreatedAt.Add(3*time.Minute + 20*time.Second)

	u := testUI(&fakeResolver{}, &fakeSource{items: []store.StoredItem{it}})
	u.inbox.snapshot()

	d := u.inbox.detail("r1")
	if !d.Found {
		t.Fatal("the row is on screen, so its detail has to be found")
	}
	if !strings.Contains(d.BodyHTML, "<strong>") {
		t.Errorf("the body should arrive rendered: %q", d.BodyHTML)
	}
	// The whole body, not the row's preview of it: the last sentence has to be there.
	if !strings.Contains(d.BodyHTML, strings.TrimSpace(long[len(long)-52:])) {
		t.Errorf("the body was shortened on its way out:\n%s", d.BodyHTML)
	}
	if n := len(snippet(it.Body, 140)); len(d.BodyHTML) <= n {
		t.Errorf("the detail (%d bytes) is no longer than the row's snippet (%d)", len(d.BodyHTML), n)
	}
	if d.CreatedAt != "Aug 5 14:32" || d.ResolvedAt != "Aug 5 14:35" {
		t.Errorf("timestamps = %q and %q", d.CreatedAt, d.ResolvedAt)
	}
	if d.Took != "3m" {
		t.Errorf("took = %q, want 3m", d.Took)
	}
	if d.Outcome != "proceeded" {
		t.Errorf("outcome = %q; a veto that expired proceeded", d.Outcome)
	}
	if d.Pending {
		t.Error("a resolved item must not offer its card")
	}
	if d.Hue == "" || d.Agent != "claude-code" {
		t.Errorf("identity = %q / %q", d.Agent, d.Hue)
	}
}

// An id nobody has heard of says so, rather than answering with an empty body
// that reads on screen as "the agent said nothing".
func TestDetailSaysWhenAnItemIsGone(t *testing.T) {
	u := testUI(&fakeResolver{}, &fakeSource{items: []store.StoredItem{item("r1", proto.KindNotify, store.StateExpired)}})
	u.inbox.snapshot()

	d := u.inbox.detail("nope")
	if d.Found || d.BodyHTML != "" {
		t.Errorf("an unknown id must answer not-found: %+v", d)
	}
	if d.ID != "nope" {
		t.Errorf("the answer should name the id it was asked about, got %q", d.ID)
	}
}

// The snapshot is the lookup, but it is not the only one: an id asked for before
// the surface ever painted still resolves, because the source is read again.
func TestDetailFallsBackToTheSourceOnAMiss(t *testing.T) {
	it := item("r1", proto.KindNotify, store.StateAnswered)
	it.Body = "it finished"
	u := testUI(&fakeResolver{}, &fakeSource{items: []store.StoredItem{it}})

	// No snapshot() call, so ib.rows is empty and the only path left is the re-read.
	if d := u.inbox.detail("r1"); !d.Found || !strings.Contains(d.BodyHTML, "it finished") {
		t.Fatalf("the re-read should have found it: %+v", d)
	}
	// A broken source is not a crash and not a body of nothing.
	u = testUI(&fakeResolver{}, &fakeSource{err: io.ErrUnexpectedEOF})
	if d := u.inbox.detail("r1"); d.Found {
		t.Error("a failed re-read must answer not-found")
	}
}

// A choice read back has to carry what it offered as well as what was taken; the
// answer on its own is a word with nothing to read it against.
func TestDetailCarriesTheOptionsAndWhichOneWent(t *testing.T) {
	it := item("r1", proto.KindChoice, store.StateAnswered)
	it.Options = []proto.Option{{Label: "Run now", Desc: "no dry run first"}, {Label: "Dry run"}, {Label: "Skip"}}
	it.Default = "Dry run"
	it.Answer = "Skip"

	u := testUI(&fakeResolver{}, &fakeSource{items: []store.StoredItem{it}})
	u.inbox.snapshot()
	d := u.inbox.detail("r1")

	if len(d.Options) != 3 {
		t.Fatalf("options = %+v", d.Options)
	}
	if d.Options[0].Desc != "no dry run first" {
		t.Errorf("an option's description was dropped: %+v", d.Options[0])
	}
	if !d.Options[1].Default || d.Options[1].Chosen {
		t.Errorf("Dry run was the default and was not taken: %+v", d.Options[1])
	}
	if !d.Options[2].Chosen || d.Options[2].Default {
		t.Errorf("Skip was taken and was not the default: %+v", d.Options[2])
	}
	if d.Answer != "Skip" {
		t.Errorf("answer = %q", d.Answer)
	}

	// An item that expired onto its default delivered that default, and the read
	// back marks the option that went rather than pretending nothing did.
	it.State, it.Answer = store.StateExpired, "Dry run"
	u = testUI(&fakeResolver{}, &fakeSource{items: []store.StoredItem{it}})
	u.inbox.snapshot()
	d = u.inbox.detail("r1")
	if !d.Options[1].Chosen {
		t.Errorf("the applied default should be marked as taken: %+v", d.Options[1])
	}
	if d.Outcome != "expired" {
		t.Errorf("outcome = %q; the chip is what says the clock decided", d.Outcome)
	}
}

// A form's answer is a map, and a map has no order. The fields do, and a value
// under a key no field declares is still shown - dropping it silently would be
// this field request's own defect in miniature.
func TestAnsweredValuesReadInTheFormsOwnOrder(t *testing.T) {
	it := item("r1", proto.KindForm, store.StateAnswered)
	it.Fields = []proto.Field{
		{Key: "env", Label: "Environment"},
		{Key: "notes"},
		{Key: "unanswered", Label: "Never filled"},
	}
	it.Values = map[string]string{"notes": "second", "env": "staging", "zz": "9", "aa": "1"}

	got := answeredValues(it)
	want := []wireFieldValue{
		{Label: "Environment", Value: "staging"},
		{Label: "notes", Value: "second"}, // no label: the key is what it was called
		{Label: "aa", Value: "1"},         // undeclared keys follow, sorted
		{Label: "zz", Value: "9"},
	}
	if len(got) != len(want) {
		t.Fatalf("values = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("value %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if answeredValues(item("r2", proto.KindForm, store.StateExpired)) != nil {
		t.Error("an unanswered form has no values")
	}
}

// A pending row's detail is what makes promoting one click from the reader, so
// the flag the button hangs on is worth pinning.
func TestDetailOffersTheCardOnlyWhilePending(t *testing.T) {
	pending := item("p1", proto.KindText, store.StatePending)
	pending.Body = "which branch?"
	done := item("r1", proto.KindNotify, store.StateAnswered)

	u := testUI(&fakeResolver{}, &fakeSource{items: []store.StoredItem{pending, done}, muted: []string{"claude-code"}})
	u.inbox.snapshot()

	if d := u.inbox.detail("p1"); !d.Pending || d.ResolvedAt != "" || d.Took != "" {
		t.Errorf("a pending item is unresolved and offers its card: %+v", d)
	}
	if d := u.inbox.detail("r1"); d.Pending {
		t.Error("a resolved item must not offer a card there is nothing to show")
	}
	// Muting is the row's badge and the detail says it too, so a reader who opened
	// the row rather than scanning the list still learns the agent is muted.
	if d := u.inbox.detail("p1"); !d.Muted {
		t.Error("the detail should carry the muted badge")
	}
}

func TestStoodReadsInTheUnitsAPersonThinksIn(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{300 * time.Millisecond, ""},
		{9 * time.Second, "9s"},
		{95 * time.Second, "2m"},
		{42 * time.Minute, "42m"},
		{3*time.Hour + 7*time.Minute, "3h07m"},
		{50 * time.Hour, "2d2h"},
	} {
		if got := stood(tc.d); got != tc.want {
			t.Errorf("stood(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
