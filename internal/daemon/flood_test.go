package daemon

import (
	"context"
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
	d, _, _, _ := newTestDaemon(t, Config{FloodBurst: 2, FloodWindow: 30 * time.Millisecond})
	callNotify(t, d, floodNotify("one"))
	callNotify(t, d, floodNotify("two"))
	callNotify(t, d, floodNotify("three")) // over: opens the stack

	time.Sleep(60 * time.Millisecond) // the whole window has passed
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
	d, _, _, _ := newTestDaemon(t, floodCfg())
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

	// The whole promise of collapsing rather than dropping: the parked caller is
	// still reachable, through the row.
	d.OpenStacked(stack.ID, askID)
	d.mu.Lock()
	onScreen := d.current
	d.mu.Unlock()
	if onScreen == nil || onScreen.ID != askID {
		t.Fatalf("on screen = %+v, want the promoted question %s", onScreen, askID)
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
