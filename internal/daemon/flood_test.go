package daemon

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// floodCfg is the shape every test here shares: a tight budget so a burst is
// three calls rather than a sleep, and a window long enough that no test ever
// races the clock it is asserting against.
func floodCfg() Config {
	return Config{FloodBurst: 2, FloodWindow: time.Minute}
}

func floodNotify(title string) proto.Item {
	return proto.Item{
		Kind: proto.KindNotify, Level: proto.LevelInfo, Title: title,
		Identity: proto.Identity{Agent: "claude", Project: "agentbox", Key: "sk-1"},
	}
}

func TestBurstUnderTheBudgetIsUntouched(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, floodCfg())
	first := callNotify(t, d, floodNotify("one"))
	second := callNotify(t, d, floodNotify("two"))

	// Two arrivals, two items, and the second is queued behind the first rather
	// than collapsed: flood control that trips at the budget rather than past it
	// would turn every ordinary pair of notices into a stack card.
	if v := ui.last(); v.Item == nil || v.Item.ID != first.ID {
		t.Fatalf("on screen = %+v, want the first item %s", v.Item, first.ID)
	}
	if v := ui.last(); v.Waiting != 1 {
		t.Fatalf("waiting = %d, want the second item queued", v.Waiting)
	}
	if second.ID == first.ID {
		t.Fatal("the second notify reused the first id")
	}
}

func TestPastTheBudgetItemsCollapseIntoOneStackCard(t *testing.T) {
	d, ui, snd, st := newTestDaemon(t, floodCfg())
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))

	snd.mu.Lock()
	before := len(snd.played)
	snd.mu.Unlock()

	third := callNotify(t, d, floodNotify("three"))
	fourth := callNotify(t, d, floodNotify("four"))
	fifth := callNotify(t, d, floodNotify("five"))

	// One stack card for three collapsed items, not three cards and not three
	// stacks. It is queued behind the two that arrived legitimately.
	var stack *proto.Item
	d.mu.Lock()
	for _, q := range d.queue {
		if q.Kind == proto.KindStack {
			if stack != nil {
				d.mu.Unlock()
				t.Fatal("a burst made more than one stack card")
			}
			stack = q
		}
	}
	d.mu.Unlock()
	if stack == nil {
		t.Fatal("nothing collapsed: no stack card in the queue")
	}
	if len(stack.Stack) != 3 {
		t.Fatalf("stack holds %d entries, want 3", len(stack.Stack))
	}
	// The three arrived inside a millisecond, so the span reads as prose rather
	// than as "0s" - a burst that took no measurable time did not take zero time.
	if stack.Title != "claude: 3 notifications in under a second" {
		t.Fatalf("stack title = %q, want the count and the span in it", stack.Title)
	}
	if stack.Level != proto.LevelWarning {
		t.Fatalf("stack level = %q, want warning: the collapse IS the warning FR30 asks for", stack.Level)
	}
	if stack.Blocking() {
		t.Fatal("a stack card reads as blocking; nothing is parked on it")
	}

	// Order is arrival order, and every entry points at an item that really exists.
	for i, want := range []string{third.ID, fourth.ID, fifth.ID} {
		if stack.Stack[i].ID != want {
			t.Fatalf("entry %d = %s, want %s (arrival order)", i, stack.Stack[i].ID, want)
		}
		got, err := st.Item(want)
		if err != nil || got == nil {
			t.Fatalf("collapsed item %s is not in the store (err %v)", want, err)
		}
		if got.State != store.StatePending {
			t.Fatalf("collapsed item %s is %q, want still pending: nothing is dropped", want, got.State)
		}
	}

	// Three collapsed items make no more noise than one queued arrival would, and
	// here that is none at all: a card is already on screen, so the stack card
	// takes its place in the queue silently exactly as an ordinary item does.
	// What must never happen is three - which is what the desktop did before
	// FR30, and the whole complaint behind it.
	snd.mu.Lock()
	defer snd.mu.Unlock()
	if got := len(snd.played) - before; got != 0 {
		t.Fatalf("played %d earcons for 3 collapsed items behind an open card, want 0", got)
	}
	if v := ui.last(); v.Item == nil {
		t.Fatal("the view went out empty")
	}
}

func TestAGrowingStackCardIsSilent(t *testing.T) {
	d, _, snd, _ := newTestDaemon(t, floodCfg())
	one := callNotify(t, d, floodNotify("one"))
	two := callNotify(t, d, floodNotify("two"))
	d.Dismiss(one.ID)
	d.Dismiss(two.ID)

	snd.mu.Lock()
	before := len(snd.played)
	snd.mu.Unlock()

	// Nothing on screen, so the stack card is a real arrival and earns one
	// earcon: the human is owed the news that an agent has started flooding.
	callNotify(t, d, floodNotify("three"))
	snd.mu.Lock()
	afterFirst := len(snd.played) - before
	snd.mu.Unlock()
	if afterFirst != 1 {
		t.Fatalf("the stack card's own arrival played %d earcons, want 1", afterFirst)
	}

	// Everything after that folds into a card already on screen. Chiming again
	// would make flood control audible per item, which is the noise it exists to
	// end - and it is the failure a reader of this code would not notice, because
	// the card looks right the whole time.
	for range 5 {
		callNotify(t, d, floodNotify("more"))
	}
	snd.mu.Lock()
	defer snd.mu.Unlock()
	if got := len(snd.played) - before - afterFirst; got != 0 {
		t.Fatalf("growing the stack played %d earcons, want 0", got)
	}
}

func TestTheStackCardIsWrittenThroughAsItGrows(t *testing.T) {
	d, _, _, st := newTestDaemon(t, floodCfg())
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))
	callNotify(t, d, floodNotify("three"))
	callNotify(t, d, floodNotify("four"))

	d.mu.Lock()
	var id string
	for _, q := range d.queue {
		if q.Kind == proto.KindStack {
			id = q.ID
		}
	}
	d.mu.Unlock()

	// The point of the column: a daemon that restarts mid-burst re-presents what
	// the human was actually looking at, not the first entry of it.
	stored, err := st.Item(id)
	if err != nil || stored == nil {
		t.Fatalf("stack card %s not in the store (err %v)", id, err)
	}
	if len(stored.Stack) != 2 {
		t.Fatalf("stored stack holds %d entries, want 2 - the growth was never written back", len(stored.Stack))
	}
	if stored.Title != "claude: 2 notifications in under a second" {
		t.Fatalf("stored title = %q, want the grown count rather than the one it was created with", stored.Title)
	}
}

func TestEachSessionFloodsOnItsOwnBudget(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, floodCfg())
	mine := floodNotify("mine")
	theirs := floodNotify("theirs")
	theirs.Identity.Key = "sk-2"

	callNotify(t, d, mine)
	callNotify(t, d, mine)
	callNotify(t, d, mine) // over: sk-1 starts collapsing

	// The neighbour is a different session with the same agent NAME. Charging it
	// to the same budget is how one looping agent would collapse the innocent
	// session sitting next to it in the same repo.
	callNotify(t, d, theirs)
	callNotify(t, d, theirs)

	d.mu.Lock()
	defer d.mu.Unlock()
	stacks := 0
	all := append([]*proto.Item{}, d.queue...)
	if d.current != nil {
		all = append(all, d.current)
	}
	for _, it := range all {
		if it.Kind == proto.KindStack {
			stacks++
			if it.Identity.Key != "sk-1" {
				t.Fatalf("the stack card belongs to %q, want sk-1", it.Identity.Key)
			}
		}
	}
	if stacks != 1 {
		t.Fatalf("%d stack cards, want 1: the second session was inside its own budget", stacks)
	}
}

func TestTheBudgetRefillsWhenTheWindowPasses(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{FloodBurst: 2, FloodWindow: 40 * time.Millisecond})
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))
	time.Sleep(70 * time.Millisecond)
	callNotify(t, d, floodNotify("three"))

	d.mu.Lock()
	defer d.mu.Unlock()
	for _, q := range d.queue {
		if q.Kind == proto.KindStack {
			t.Fatal("collapsed an item that arrived after the window had passed")
		}
	}
}

func TestAnOpenStackKeepsCollectingPastTheWindow(t *testing.T) {
	// The bug this pins was found on the real desktop, not in a test: the window
	// refills underneath an open stack card, so a loop that keeps going gets a
	// fresh budget every window - three cards per ten seconds on the shipped
	// defaults, which is eighteen a minute and not "calm survives buggy callers".
	d, _, _, _ := newTestDaemon(t, floodCfg())
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))
	callNotify(t, d, floodNotify("three")) // over: opens the stack

	// Backdating every arrival on record IS "the whole window has passed", and it
	// is what this used to sleep for. The sleep made the test a race it could
	// lose: with a 30ms window, the three calls above have to land inside 30ms,
	// and on a two-core CI runner under -race they did not - the burst never
	// opened a stack, and the failure read as a product bug rather than a clock.
	d.mu.Lock()
	for _, f := range d.flood {
		for i := range f.recent {
			f.recent[i] = f.recent[i].Add(-2 * time.Minute)
		}
	}
	d.mu.Unlock()

	callNotify(t, d, floodNotify("four"))
	callNotify(t, d, floodNotify("five"))

	d.mu.Lock()
	var stack *proto.Item
	stacks := 0
	for _, q := range d.queue {
		if q.Kind == proto.KindStack {
			stacks++
			stack = q
		}
	}
	d.mu.Unlock()
	if stacks != 1 {
		t.Fatalf("%d stack cards, want 1: the burst never stopped", stacks)
	}
	if len(stack.Stack) != 3 {
		t.Fatalf("stack holds %d, want 3 - the budget refilled under an open card", len(stack.Stack))
	}

	// Ending it is the human's move, and once he has made it the next item is a
	// card again. Otherwise a session that flooded once would be collapsed for
	// the rest of the day.
	d.Dismiss(stack.ID)
	callNotify(t, d, floodNotify("after"))
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, q := range d.queue {
		if q.Kind == proto.KindStack {
			t.Fatal("collapsed again after the human closed the stack")
		}
	}
	if d.current != nil && d.current.Kind == proto.KindStack {
		t.Fatal("collapsed again after the human closed the stack")
	}
}

func TestTheStackWearsTheWorstLevelInIt(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, floodCfg())
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))
	callNotify(t, d, floodNotify("three")) // info, collapses: the card is warning

	d.mu.Lock()
	var stack *proto.Item
	for _, q := range d.queue {
		if q.Kind == proto.KindStack {
			stack = q
		}
	}
	d.mu.Unlock()
	if stack.Level != proto.LevelWarning {
		t.Fatalf("a stack of info notices is %q, want warning: the collapse is itself the warning", stack.Level)
	}

	bad := floodNotify("the one that matters")
	bad.Level = proto.LevelError
	callNotify(t, d, bad)
	if stack.Level != proto.LevelError {
		t.Fatalf("stack level = %q after collapsing an error, want error: one line has to speak for the whole burst", stack.Level)
	}
}

func TestFloodControlOffByDefault(t *testing.T) {
	// Burst 0 is the human saying never collapse, and it is also every daemon
	// built without the knob - which is every test written before FR30.
	d, _, _, _ := newTestDaemon(t, Config{})
	for range 8 {
		callNotify(t, d, floodNotify("noisy"))
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, q := range d.queue {
		if q.Kind == proto.KindStack {
			t.Fatal("collapsed with flood control off")
		}
	}
}

func TestAQuestionCaughtInABurstStaysAnswerable(t *testing.T) {
	d, _, _, st := newTestDaemon(t, floodCfg())
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))

	ask := askItem()
	ask.Identity = floodNotify("").Identity
	answered := make(chan proto.Result, 1)
	go func() {
		res, rpcErr := d.Handle(context.Background(), proto.MethodAsk, mustJSON(t, ask))
		if rpcErr != nil {
			answered <- proto.Result{}
			return
		}
		answered <- res.(proto.Result)
	}()

	// Wait for the question to be collapsed rather than displayed.
	var stack *proto.Item
	var askID string
	deadline := time.Now().Add(2 * time.Second)
	for {
		d.mu.Lock()
		for _, q := range d.queue {
			if q.Kind == proto.KindStack && len(q.Stack) > 0 {
				stack = q
				askID = q.Stack[0].ID
			}
		}
		d.mu.Unlock()
		if stack != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the question was never collapsed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !stack.Stack[0].Blocking {
		t.Fatal("the collapsed question does not read as blocking; the card cannot warn about it")
	}
	// A burst holding a question is not a burst of notifications, and the title is
	// the line a person decides from.
	if stack.Title != "claude: 1 item in under a second" {
		t.Fatalf("stack title = %q, want it to stop calling a question a notification", stack.Title)
	}

	// Open it from the state the human is actually in: the stack card ON SCREEN,
	// not queued behind something else. That distinction is the whole test - the
	// first version of this only ever had the stack in the queue, so it passed
	// while the number key on the real desktop put the stack straight back up and
	// left the promoted question behind it reading "1 waiting".
	d.mu.Lock()
	for d.current != nil && d.current.ID != stack.ID {
		id := d.current.ID
		d.mu.Unlock()
		d.Dismiss(id)
		d.mu.Lock()
	}
	onStack := d.current != nil && d.current.ID == stack.ID
	d.mu.Unlock()
	if !onStack {
		t.Fatal("could not get the stack card on screen")
	}

	// The whole promise of collapsing rather than dropping: the parked caller is
	// still reachable, through the row.
	d.OpenStacked(stack.ID, askID)
	d.mu.Lock()
	onScreen := d.current
	next := ""
	if len(d.queue) > 0 {
		next = d.queue[0].ID
	}
	d.mu.Unlock()
	if onScreen == nil || onScreen.ID != askID {
		t.Fatalf("on screen = %+v, want the promoted question %s", onScreen, askID)
	}
	if next != stack.ID {
		t.Fatalf("next in the queue is %q, want the stack card back right behind the row that was opened", next)
	}

	d.Answer(askID, "Yes")
	select {
	case res := <-answered:
		if !res.Answered || res.Answer != "Yes" {
			t.Fatalf("caller got %+v, want the answer it was parked for", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the caller never heard back: a question inside a burst was swallowed")
	}

	// And the list it came from stops asking. Watched failing on the real
	// desktop: the answer shipped, the stack card came back, and its row still
	// read "waiting on you" under a footer still counting one.
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range stack.Stack {
		if e.ID == askID && !e.Done {
			t.Fatal("the answered row still reads as waiting")
		}
	}
	stored, err := st.Item(stack.ID)
	if err != nil || stored == nil {
		t.Fatalf("stack card missing from the store (err %v)", err)
	}
	for _, e := range stored.Stack {
		if e.ID == askID && !e.Done {
			t.Fatal("the quieted row was not written back; a restart would ask again")
		}
	}
}

func TestDismissingAStackKeepsTheQuestionsAndClearsTheNotices(t *testing.T) {
	d, _, _, st := newTestDaemon(t, floodCfg())
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))
	third := callNotify(t, d, floodNotify("three"))

	ask := askItem()
	ask.Identity = floodNotify("").Identity
	go d.Handle(context.Background(), proto.MethodAsk, mustJSON(t, ask))

	var stack *proto.Item
	deadline := time.Now().Add(2 * time.Second)
	for {
		d.mu.Lock()
		for _, q := range d.queue {
			if q.Kind == proto.KindStack && len(q.Stack) == 2 {
				stack = q
			}
		}
		d.mu.Unlock()
		if stack != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the burst never reached two collapsed entries")
		}
		time.Sleep(5 * time.Millisecond)
	}
	askID := stack.Stack[1].ID

	d.Dismiss(stack.ID)

	// The notification goes with the summary: the human read the count and said
	// enough, and leaving it pending means a restart replays a flood he closed.
	got, err := st.Item(third.ID)
	if err != nil || got == nil {
		t.Fatalf("collapsed notice %s missing (err %v)", third.ID, err)
	}
	if got.State != store.StateDismissed {
		t.Fatalf("collapsed notice is %q, want dismissed with its stack", got.State)
	}
	// The question does NOT: closing a summary is not an answer, and an agent is
	// parked on it right now.
	q, err := st.Item(askID)
	if err != nil || q == nil {
		t.Fatalf("collapsed question %s missing (err %v)", askID, err)
	}
	if q.State != store.StatePending {
		t.Fatalf("collapsed question is %q, want still pending for inbox triage", q.State)
	}
}

func TestTheCliDismissAlsoSweepsTheStack(t *testing.T) {
	// The other door, and the one the sweep was originally missing. `agentbox
	// dismiss <stack>` cleared the summary and left five invisible notices
	// pending, so `agentbox pending` read as empty-ish and a restart would have
	// replayed a flood the human had closed.
	d, _, _, st := newTestDaemon(t, floodCfg())
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))
	third := callNotify(t, d, floodNotify("three"))

	d.mu.Lock()
	var stackID string
	for _, q := range d.queue {
		if q.Kind == proto.KindStack {
			stackID = q.ID
		}
	}
	d.mu.Unlock()

	if _, rpcErr := d.DismissItems(proto.DismissParams{ID: stackID, Human: true}); rpcErr != nil {
		t.Fatalf("dismiss: %v", rpcErr)
	}
	got, err := st.Item(third.ID)
	if err != nil || got == nil {
		t.Fatalf("collapsed notice missing (err %v)", err)
	}
	if got.State != store.StateDismissed {
		t.Fatalf("collapsed notice is %q after the CLI dismissed its stack, want dismissed", got.State)
	}
}

func TestNoCallerMaySubmitAStackCard(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, floodCfg())
	it := proto.Item{
		Kind: proto.KindStack, Title: "claude: 400 notifications",
		Stack:    []proto.StackEntry{{ID: "x", Title: "made up"}},
		Identity: proto.Identity{Agent: "claude"},
	}
	if _, rpcErr := d.Handle(context.Background(), proto.MethodAsk, mustJSON(t, it)); rpcErr == nil {
		t.Fatal("ask accepted a caller-made stack card")
	}
	if _, rpcErr := d.Handle(context.Background(), proto.MethodNotify, mustJSON(t, it)); rpcErr == nil {
		t.Fatal("notify accepted a caller-made stack card")
	}
}

func TestOpenStackedRefusesWhatTheHumanIsNotLookingAt(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, floodCfg())
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))
	loose := callNotify(t, d, floodNotify("three"))

	d.mu.Lock()
	var stackID string
	for _, q := range d.queue {
		if q.Kind == proto.KindStack {
			stackID = q.ID
		}
	}
	d.mu.Unlock()

	// An item that exists but belongs to no stack the human has on screen.
	other := callNotify(t, d, floodNotify("four"))
	_ = other
	d.OpenStacked("no-such-stack", loose.ID)
	d.mu.Lock()
	current := d.current
	d.mu.Unlock()
	if current != nil && current.ID == loose.ID {
		t.Fatal("promoted an item through a stack id that is not on screen")
	}

	d.OpenStacked(stackID, "no-such-item")
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.current != nil && d.current.ID == "no-such-item" {
		t.Fatal("promoted an item that is in no stack")
	}
}

func TestARestartBringsTheBurstBackCollapsed(t *testing.T) {
	// Every collapsed item is pending in its own right, so a naive restore puts
	// the stack card AND everything it collapsed back on the queue - the collapse
	// undone at the one moment the human has no idea why. Found by reading the
	// restore path after the feature was already deployed, not by any test that
	// existed.
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "agentbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := floodCfg()
	ui := &fakeUI{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d, err := New(cfg, log, st, &fakeSound{}, ui)
	if err != nil {
		t.Fatal(err)
	}
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))
	callNotify(t, d, floodNotify("three"))
	callNotify(t, d, floodNotify("four"))
	st.Close()

	// A fresh daemon on the same store, which is what a restart is.
	st2, err := store.Open(filepath.Join(dir, "agentbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st2.Close() })
	d2, err := New(cfg, log, st2, &fakeSound{}, &fakeUI{})
	if err != nil {
		t.Fatal(err)
	}

	d2.mu.Lock()
	all := append([]*proto.Item{}, d2.queue...)
	if d2.current != nil {
		all = append(all, d2.current)
	}
	d2.mu.Unlock()

	stacks, notices := 0, 0
	var stack *proto.Item
	for _, it := range all {
		switch it.Kind {
		case proto.KindStack:
			stacks++
			stack = it
		default:
			notices++
		}
	}
	if stacks != 1 {
		t.Fatalf("%d stack cards after a restart, want 1", stacks)
	}
	if notices != 2 {
		t.Fatalf("%d loose items after a restart, want the 2 that were never collapsed", notices)
	}
	if len(stack.Stack) != 2 {
		t.Fatalf("the restored stack holds %d entries, want 2", len(stack.Stack))
	}

	// And the budget knows which card it belongs to, or the next item opens a
	// SECOND summary of the same flood beside the first.
	callNotify(t, d2, floodNotify("five"))
	d2.mu.Lock()
	defer d2.mu.Unlock()
	after := append([]*proto.Item{}, d2.queue...)
	if d2.current != nil {
		after = append(after, d2.current)
	}
	n := 0
	for _, it := range after {
		if it.Kind == proto.KindStack {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d stack cards after collapsing into a restored one, want 1", n)
	}
}

func TestAnUrgentItemStillBreaksThroughAStack(t *testing.T) {
	// Flood control must not turn the one level that is allowed to interrupt into
	// the one that cannot.
	d, _, _, _ := newTestDaemon(t, floodCfg())
	callNotify(t, d, floodNotify("one")) // on screen
	callNotify(t, d, floodNotify("two"))
	callNotify(t, d, floodNotify("three")) // opens the stack, queued behind one

	d.mu.Lock()
	onScreen := ""
	if d.current != nil {
		onScreen = string(d.current.Kind)
	}
	d.mu.Unlock()
	if onScreen == "stack" {
		t.Fatal("the stack was already on screen; this test needs it queued")
	}

	bad := floodNotify("the machine is on fire")
	bad.Level = proto.LevelUrgent
	callNotify(t, d, bad)

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.current == nil || d.current.Kind != proto.KindStack {
		t.Fatalf("on screen = %+v, want the stack card the urgent item went into", d.current)
	}
	if d.current.Level != proto.LevelUrgent {
		t.Fatalf("stack level = %q, want urgent", d.current.Level)
	}
}

// A question that outlives the stack card carrying it must still be answerable.
// Dismissing a stack sweeps its notifications and deliberately keeps every
// blocking row pending (sweepStack), which leaves an item that is pending in the
// store and held nowhere in memory. OpenStacked cannot reach it, because there is
// no live stack card any more, and the inbox row routes text, secret, form and
// diff kinds through Promote. Promote used to return silently for anything not in
// the queue, so the question could not be answered from anywhere at all while its
// caller stayed parked on it. Reproduced 2026-08-07 before this test existed.
func TestAQuestionOutlivingItsStackCardIsStillAnswerable(t *testing.T) {
	d, _, _, st := newTestDaemon(t, floodCfg())
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))

	// A typed question, because those are the kinds the inbox promotes rather than
	// answering in place.
	ask := proto.Item{
		Kind: proto.KindText, Title: "What should I tag it?",
		Identity: floodNotify("").Identity,
	}
	answered := make(chan proto.Result, 1)
	go func() {
		res, rpcErr := d.Handle(context.Background(), proto.MethodAsk, mustJSON(t, ask))
		if rpcErr != nil {
			answered <- proto.Result{}
			return
		}
		answered <- res.(proto.Result)
	}()

	var stackID, askID string
	deadline := time.Now().Add(2 * time.Second)
	for askID == "" {
		d.mu.Lock()
		all := d.queue
		if d.current != nil {
			all = append([]*proto.Item{d.current}, d.queue...)
		}
		for _, q := range all {
			if q.Kind == proto.KindStack {
				for _, e := range q.Stack {
					if e.Blocking {
						stackID, askID = q.ID, e.ID
					}
				}
			}
		}
		d.mu.Unlock()
		if askID != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the question was never collapsed into a stack card")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// The human reads the count and closes the summary. This is the step that
	// created the trap: the notifications go, the question stays pending, and the
	// only card that pointed at it is gone.
	d.Dismiss(stackID)

	d.mu.Lock()
	held := d.current != nil && d.current.ID == askID
	for _, q := range d.queue {
		if q.ID == askID {
			held = true
		}
	}
	d.mu.Unlock()
	if held {
		t.Fatal("the question is in memory, so this test is no longer exercising the gap it was written for")
	}
	stored, err := st.Item(askID)
	if err != nil || stored == nil || stored.State != store.StatePending {
		t.Fatalf("the question should be pending in the store: %+v (err %v)", stored, err)
	}

	// The inbox row, which is the last door left.
	d.Promote(askID)

	d.mu.Lock()
	onScreen := d.current != nil && d.current.ID == askID
	d.mu.Unlock()
	if !onScreen {
		t.Fatal("promoting the surviving question did nothing; it cannot be answered from anywhere")
	}

	d.Reply(askID, "2026.7.30")
	select {
	case res := <-answered:
		if res.Reply != "2026.7.30" {
			t.Fatalf("caller got reply %q, want the typed answer", res.Reply)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the caller was never released")
	}
}
