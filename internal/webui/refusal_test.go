package webui

import (
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// U-02, the webview's half. The daemon can now say it refused; these assert the
// keyhole carries the sentence out to the surface instead of dropping it. Every
// one of these methods used to return nothing at all, so a surface had no way to
// tell an answer that landed from a keystroke that went nowhere.

const refusal = "that item has already ended, so there was nothing left to answer."

func TestEveryAnswerPathMethodCarriesTheRefusalOut(t *testing.T) {
	res := &fakeResolver{refuse: refusal}
	b := &Bridge{ui: testUI(res, &fakeSource{})}

	// One line per method, because the defect was per-method: a signature that
	// returned nothing, eleven times over.
	got := map[string]string{
		"Answer":      b.Answer("i1", "Yes"),
		"Reply":       b.Reply("i1", "text"),
		"AnswerForm":  b.AnswerForm("i1", map[string]string{"a": "b"}),
		"Confirm":     b.Confirm("i1", true),
		"Secret":      b.Secret("i1", "hunter2"),
		"Review":      b.Review("i1", true, ""),
		"Veto":        b.Veto("i1"),
		"Defer":       b.Defer("i1"),
		"Dismiss":     b.Dismiss("i1"),
		"Undo":        b.Undo("i1"),
		"RunAction":   b.RunAction("i1", 0),
		"OpenStacked": b.OpenStacked("s1", "i1"),
	}
	for method, why := range got {
		if why != refusal {
			t.Errorf("%s answered %q, want the daemon's sentence: a refusal it swallows is U-02 all over again", method, why)
		}
	}
}

func TestAnAnswerThatLandsCarriesNothingBack(t *testing.T) {
	b := &Bridge{ui: testUI(&fakeResolver{}, &fakeSource{})}
	if why := b.Answer("i1", "Yes"); why != "" {
		t.Fatalf("a successful answer said %q; the surface would paint a failure over it", why)
	}
	if why := b.Confirm("i1", false); why != "" {
		t.Fatalf("a successful no said %q", why)
	}
}

func TestPromoteCarriesTheRefusalOut(t *testing.T) {
	src := &fakeSource{refuse: "that item was already answered, so there is no card to open."}
	b := &Bridge{ui: testUI(&fakeResolver{}, src)}
	if why := b.Promote("i1"); why != src.refuse {
		t.Fatalf("promote answered %q, want the daemon's sentence", why)
	}
}

// A window that opens before the daemon is wired in gets an honest answer rather
// than silence, which is the shape the old code had everywhere.
func TestPromoteWithNoSourceSaysSo(t *testing.T) {
	b := &Bridge{ui: testUI(&fakeResolver{}, nil)}
	if why := b.Promote("i1"); why == "" {
		t.Fatal("promote with no daemon behind it said nothing at all")
	}
}

// Triage keeps its bool (FR34): the surface is told the key did nothing, and the
// reason goes to the log. What matters is that a refusal from the answer path is
// no longer reported as a working keystroke.
func TestTriageReportsAKeyTheAnswerPathRefused(t *testing.T) {
	row := item("p1", proto.KindChoice, store.StatePending)
	row.Options = []proto.Option{{Label: "Yes"}, {Label: "No"}}
	src := &fakeSource{items: []store.StoredItem{row}}

	ok := testUI(&fakeResolver{}, src)
	ok.inbox.snapshot() // act works against the rows the surface was shown
	if !ok.inbox.act("p1", "1") {
		t.Fatal("an answer the daemon accepted was reported as a key that did nothing")
	}

	refused := testUI(&fakeResolver{refuse: refusal}, src)
	refused.inbox.snapshot()
	if refused.inbox.act("p1", "1") {
		t.Fatal("the daemon refused the answer and triage still reported success")
	}
}

func TestTriageReportsAPromoteThatLedNowhere(t *testing.T) {
	// A typed question: the kinds the inbox promotes rather than answering in
	// place, which is exactly where R-01 was invisible.
	row := item("p1", proto.KindText, store.StatePending)
	src := &fakeSource{items: []store.StoredItem{row}}

	ok := testUI(&fakeResolver{}, src)
	ok.inbox.snapshot()
	if !ok.inbox.act("p1", "Enter") {
		t.Fatal("a promote the daemon accepted was reported as a key that did nothing")
	}

	src.refuse = "that item is gone: agentbox is not holding it and the store has no record of it."
	gone := testUI(&fakeResolver{}, src)
	gone.inbox.snapshot()
	if gone.inbox.act("p1", "Enter") {
		t.Fatal("the row led nowhere and triage still reported success; this is R-01 from the inbox")
	}
}
