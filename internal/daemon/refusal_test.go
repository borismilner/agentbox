package daemon

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/borismilner/agentbox/internal/proto"
)

// U-02. Every method on the answer path now says whether it did the thing: ""
// when it did, a sentence for the surface to show when it did not. Before that
// they returned nothing at all, so a daemon that knew perfectly well it had
// refused - a promote for an item held nowhere, an undo after the grace had
// closed, a defer with an empty queue - had no way to tell the person who had
// just pressed the key, and every one of those looked exactly like success.
//
// The tests below are the refusals, one per reason. They assert the sentence is
// there and readable rather than matching it word for word: the wording is meant
// to be improved, the fact that something comes back is the contract.

// readable is the whole of what a surface needs: something to show, and a
// sentence rather than an error code. Anything shorter than this is a marker
// that only a developer could act on.
func readable(t *testing.T, why, when string) {
	t.Helper()
	if why == "" {
		t.Fatalf("%s: nothing came back, so the surface has nothing to show", when)
	}
	if len(why) < 20 || !strings.HasSuffix(why, ".") {
		t.Fatalf("%s: %q is not a sentence a human can read", when, why)
	}
	if r := []rune(why)[0]; unicode.IsUpper(r) {
		// The surfaces put these mid-line, under a row or beside a control, so
		// they read as a continuation and not as a heading.
		t.Fatalf("%s: %q starts capitalised; these are shown mid-sentence", when, why)
	}
}

func TestPromoteSaysSoWhenTheItemIsGone(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	readable(t, d.Promote("no-such-item"), "promoting an item that never existed")
}

func TestPromoteSaysSoWhenTheItemAlreadyEnded(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)
	if why := d.Answer(it.ID, "Yes"); why != "" {
		t.Fatalf("answering the item on screen was refused: %q", why)
	}
	<-ch

	// The inbox row for an item somebody else answered a moment ago. This is the
	// case that happens on an ordinary day with two windows open.
	why := d.Promote(it.ID)
	readable(t, why, "promoting an item that was already answered")
	if !strings.Contains(why, "answered") {
		t.Fatalf("%q does not say how the item ended, which is the one useful thing here", why)
	}
}

func TestAnsweringAnItemThatHasEndedSaysSo(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)
	d.Answer(it.ID, "Yes")
	<-ch

	readable(t, d.Answer(it.ID, "No"), "a second answer to a finished item")
	readable(t, d.Reply(it.ID, "late"), "a reply to a finished item")
	readable(t, d.AnswerForm(it.ID, map[string]string{"a": "b"}), "a form for a finished item")
	readable(t, d.Review(it.ID, true, ""), "a verdict for a finished item")
	readable(t, d.Veto(it.ID), "a stop for a finished item")
	readable(t, d.Dismiss(it.ID), "dismissing a finished item")
}

// The secret is the one refusal worth wording separately: the value the human
// typed went nowhere, and nobody else can tell whoever asked for it.
func TestSecretSaysTheValueWasNotDelivered(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	why := d.Secret("no-such-item", "hunter2")
	readable(t, why, "a secret for a request that has ended")
	if !strings.Contains(why, "not delivered") {
		t.Fatalf("%q does not say the value went nowhere", why)
	}
}

func TestUndoAfterTheGraceHasClosedSaysSo(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)
	d.Answer(it.ID, "Yes") // no grace configured, so it is already gone
	<-ch

	readable(t, d.Undo(it.ID), "undo after the answer has shipped")
}

// A second answer while the strip is up is dropped by design (FR28). Saying so
// is the difference between a considered refusal and a dead keyboard.
func TestASecondAnswerDuringTheGraceSaysSo(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{UndoGrace: 10 * time.Second})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)
	if why := d.Answer(it.ID, "Yes"); why != "" {
		t.Fatalf("the first answer was refused: %q", why)
	}

	why := d.Answer(it.ID, "No")
	readable(t, why, "a second answer while the first is in its undo window")
	if !strings.Contains(why, "undo") {
		t.Fatalf("%q does not point at the one control that is live", why)
	}

	// And the way out the sentence names still works, which is what makes it
	// honest rather than a brush-off.
	if why := d.Undo(it.ID); why != "" {
		t.Fatalf("undo during the grace was refused: %q", why)
	}
	if why := d.Dismiss(it.ID); why != "" {
		t.Fatalf("dismissing the restored card was refused: %q", why)
	}
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("the caller was never released")
	}
}

// Dismiss during the grace is the same guard from the other side: resolve
// refuses any transition but the finalizer's, and "already ended" would be a lie.
func TestDismissDuringTheGraceSaysWhichRefusalItIs(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{UndoGrace: 10 * time.Second})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)
	d.Answer(it.ID, "Yes")

	why := d.Dismiss(it.ID)
	readable(t, why, "dismissing a card that is showing its undo strip")
	if !strings.Contains(why, "undo") {
		t.Fatalf("%q reads like the item is finished; it is not, it is graced", why)
	}

	// Once the strip is gone the same call is taken, so the refusal was about the
	// grace and not about the item.
	d.Undo(it.ID)
	if why := d.Dismiss(it.ID); why != "" {
		t.Fatalf("dismissing after the undo was refused: %q", why)
	}
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("the caller was never released")
	}
}

func TestDeferSaysWhyNothingMoved(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)

	// Esc on the only card there is. It stays put, and until now that was all the
	// human learned about it.
	alone := d.Defer(it.ID)
	readable(t, alone, "deferring the only card on screen")

	// A card that is no longer the one on screen is a different reason, and the
	// two must not share a sentence.
	other := d.Defer("some-other-item")
	readable(t, other, "deferring an item that is not on screen")
	if alone == other {
		t.Fatalf("both refusals say %q; the reader cannot tell an empty queue from a stale card", alone)
	}

	d.Answer(it.ID, "Yes")
	<-ch
}

func TestRunActionSaysSoWhenActionsAreOff(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{ActionsDisabled: true})
	it := notifyItem(proto.LevelInfo)
	it.Actions = []proto.Action{{Label: "Open", Exec: "true"}}
	res := callNotify(t, d, it)
	waitForItem(t, ui)

	why := d.RunAction(res.ID, 0)
	readable(t, why, "clicking an action button with actions switched off")
	if !strings.Contains(why, "actions.enabled") {
		t.Fatalf("%q does not name the setting that would turn them back on", why)
	}
}

func TestRunActionSaysSoWhenTheButtonIsStale(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	it := notifyItem(proto.LevelInfo)
	it.Actions = []proto.Action{{Label: "Open", Exec: "true"}}
	res := callNotify(t, d, it)
	waitForItem(t, ui)

	readable(t, d.RunAction(res.ID, 7), "an action index the item does not have")
	readable(t, d.RunAction("no-such-item", 0), "an action on an item that is not on screen")
}

// The happy paths must stay empty, or the surfaces will paint a refusal over
// every successful keystroke. Cheap to assert and the one way this whole change
// can fail loudly instead of quietly.
func TestAnAnswerThatLandsSaysNothing(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)

	second := askItem()
	second.Title = "second"
	ch2 := askAsync(t, d, second)
	deadline := time.Now().Add(2 * time.Second)
	for ui.last().Waiting != 1 {
		if time.Now().After(deadline) {
			t.Fatal("the second question never queued")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if why := d.Defer(it.ID); why != "" {
		t.Fatalf("deferring with something waiting behind it was refused: %q", why)
	}
	d.mu.Lock()
	onScreen := d.current.ID
	d.mu.Unlock()
	if why := d.Promote(it.ID); why != "" {
		t.Fatalf("promoting a queued item was refused: %q", why)
	}
	if why := d.Answer(it.ID, "Yes"); why != "" {
		t.Fatalf("answering the promoted item was refused: %q", why)
	}
	<-ch
	if why := d.Answer(onScreen, "No"); why != "" {
		t.Fatalf("answering the displaced item was refused: %q", why)
	}
	<-ch2
}
