package webui

import (
	"strings"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
)

// The panel's Go side is the whole decision: whether a question is answered in a
// conversation or in a card, which control answers what, and what a keystroke
// means. The painting is checked by eye.

func ask(kind proto.Kind, level proto.Level, sid string) *proto.Item {
	return &proto.Item{
		ID: "a1", Kind: kind, Level: level, Title: "Persist the offsets?",
		Identity: proto.Identity{Agent: "claude-code", Project: "grabbit", Session: sid},
	}
}

func TestInlineRoutableDecidesPanelOrCard(t *testing.T) {
	tests := []struct {
		name  string
		it    *proto.Item
		shown bool
		open  bool
		want  bool
	}{
		{"a tagged choice goes inline", ask(proto.KindChoice, proto.LevelInfo, "s1"), true, true, true},
		{"so does a confirm", ask(proto.KindConfirm, proto.LevelWarning, "s1"), true, true, true},
		{"so does a notice", ask(proto.KindNotify, proto.LevelSuccess, "s1"), true, true, true},
		{"text needs a field, so it keeps its card", ask(proto.KindText, proto.LevelInfo, "s1"), true, true, false},
		{"secret keeps its card", ask(proto.KindSecret, proto.LevelInfo, "s1"), true, true, false},
		{"a form keeps its card", ask(proto.KindForm, proto.LevelInfo, "s1"), true, true, false},
		{"a diff keeps its card", ask(proto.KindDiff, proto.LevelInfo, "s1"), true, true, false},
		{"a veto keeps its card", ask(proto.KindVeto, proto.LevelWarning, "s1"), true, true, false},
		{"urgent keeps its card and its escalation", ask(proto.KindChoice, proto.LevelUrgent, "s1"), true, true, false},
		{"an untagged item was not asked by a session", ask(proto.KindChoice, proto.LevelInfo, ""), true, true, false},
		{"a session this surface is not showing cannot answer", ask(proto.KindChoice, proto.LevelInfo, "s9"), false, true, false},
		{"a closed window has nowhere to put it", ask(proto.KindChoice, proto.LevelInfo, "s1"), true, false, false},
		{"nothing to route", nil, true, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := inlineRoutable(tc.it, tc.shown, tc.open); got != tc.want {
				t.Fatalf("inlineRoutable = %v, want %v", got, tc.want)
			}
		})
	}
}

// A tagged question must reach the session that asked it and no other, whether or
// not that session is the one on screen - the switcher row needs to know too.
func TestRouteAskAttachesToItsSession(t *testing.T) {
	u := testUI(&fakeResolver{}, nil)
	u.sess.SetDemo([]wireSession{{ID: "s1", Selected: true}, {ID: "s2"}})

	v := daemon.View{Item: ask(proto.KindChoice, proto.LevelInfo, "s2")}
	v.Item.Options = []proto.Option{{Label: "Yes please"}}
	if !u.sess.routeAsk(v, true) {
		t.Fatal("a tagged choice with the window open should route inline")
	}

	snap := u.sess.snapshot()
	if snap[0].Ask != nil {
		t.Error("the question landed on a session that did not ask it")
	}
	if snap[1].Ask == nil {
		t.Fatal("the asking session has no panel")
	}
	if snap[1].Ask.ID != "a1" {
		t.Errorf("Ask.ID = %q, want a1", snap[1].Ask.ID)
	}

	// Resolving clears it: the daemon presents an empty view and the panel goes.
	if u.sess.routeAsk(daemon.View{}, true) {
		t.Error("an empty view routed inline")
	}
	if snap := u.sess.snapshot(); snap[1].Ask != nil {
		t.Error("the panel survived the item resolving")
	}
}

// The canned demo list is shared state; attaching an ask must not write into it,
// or the panel would still be there after the item resolved.
func TestSnapshotDoesNotMutateTheDemoList(t *testing.T) {
	u := testUI(&fakeResolver{}, nil)
	u.sess.SetDemo([]wireSession{{ID: "s1", Selected: true}})
	u.sess.routeAsk(daemon.View{Item: ask(proto.KindConfirm, proto.LevelInfo, "s1")}, true)

	if u.sess.snapshot()[0].Ask == nil {
		t.Fatal("no panel to begin with")
	}
	u.sess.routeAsk(daemon.View{}, true)
	if u.sess.snapshot()[0].Ask != nil {
		t.Fatal("the demo list kept the ask, so the panel would never clear")
	}
}

func TestEncodeAskChoiceControls(t *testing.T) {
	u := testUI(&fakeResolver{}, nil)
	it := ask(proto.KindChoice, proto.LevelWarning, "s1")
	it.Body = "Older state files stop loading."
	it.Options = []proto.Option{
		{Label: "Change the shape"},
		{Label: "Migrate on read", Desc: "slower"},
		{Label: "Leave it"},
	}
	it.Default = "Migrate on read"

	a := u.encodeAsk(daemon.View{Item: it, Waiting: 2, ExpiresAt: time.Now().Add(time.Minute)})

	if len(a.Options) != 3 {
		t.Fatalf("got %d controls, want 3", len(a.Options))
	}
	for i, want := range []string{"1", "2", "3"} {
		if a.Options[i].Key != want {
			t.Errorf("option %d key = %q, want %q", i, a.Options[i].Key, want)
		}
		if a.Options[i].Answer != a.Options[i].Label {
			t.Errorf("option %d answers %q but is labelled %q", i, a.Options[i].Answer, a.Options[i].Label)
		}
	}
	if a.Options[0].Primary || !a.Options[1].Primary {
		t.Error("the default option is the one that should read as primary")
	}
	if a.Options[1].Desc != "slower" {
		t.Errorf("the option description was dropped: %q", a.Options[1].Desc)
	}
	if a.Glyph != "warning" {
		t.Errorf("glyph = %q, want warning", a.Glyph)
	}
	if !strings.Contains(a.BodyHTML, "<p>") {
		t.Errorf("the body was not rendered: %q", a.BodyHTML)
	}
	if a.Hue == "" || a.Hint == "" || a.Lead == "" {
		t.Errorf("hue/hint/lead: %q %q %q", a.Hue, a.Hint, a.Lead)
	}
	if a.ExpiresAtMS == 0 || a.Waiting != 2 {
		t.Errorf("expiry %d, waiting %d", a.ExpiresAtMS, a.Waiting)
	}
	if len(a.Actions) != 0 {
		t.Error("a choice has no action buttons")
	}
}

// The panel shows "Yes" and delivers "yes": the vocabulary the daemon expects
// stays in Go, exactly as Bridge.Confirm keeps it out of the card.
func TestEncodeAskConfirmKeepsYesNoInGo(t *testing.T) {
	u := testUI(&fakeResolver{}, nil)
	a := u.encodeAsk(daemon.View{Item: ask(proto.KindConfirm, proto.LevelInfo, "s1")})

	if len(a.Options) != 2 {
		t.Fatalf("got %d controls, want 2", len(a.Options))
	}
	if a.Options[0].Key != "y" || a.Options[0].Answer != "yes" || a.Options[0].Label != "Yes" {
		t.Errorf("yes control = %+v", a.Options[0])
	}
	if a.Options[1].Key != "n" || a.Options[1].Answer != "no" || a.Options[1].Label != "No" {
		t.Errorf("no control = %+v", a.Options[1])
	}
	if !a.Options[0].Primary {
		t.Error("with no default, yes reads as primary")
	}

	it := ask(proto.KindConfirm, proto.LevelInfo, "s1")
	it.Default = "no"
	a = u.encodeAsk(daemon.View{Item: it})
	if a.Options[0].Primary || !a.Options[1].Primary {
		t.Error("a default of no should make no the primary control")
	}
}

func TestEncodeAskNoticeGetsDismissAndItsActions(t *testing.T) {
	u := testUI(&fakeResolver{}, nil)
	it := ask(proto.KindNotify, proto.LevelSuccess, "s1")
	it.Actions = []proto.Action{{Label: "Show the diff", Exec: "git diff"}}

	a := u.encodeAsk(daemon.View{Item: it, ActionsEnabled: true})
	if len(a.Options) != 1 || a.Options[0].Verb != "dismiss" || a.Options[0].Key != "d" {
		t.Fatalf("a notice's control = %+v", a.Options)
	}
	if a.Options[0].Answer != "" {
		t.Error("dismissing is not answering")
	}
	if len(a.Actions) != 1 || a.Actions[0].Index != 0 || a.Actions[0].Exec != "git diff" {
		t.Fatalf("actions = %+v", a.Actions)
	}
	if a.Lead == u.encodeAsk(daemon.View{Item: ask(proto.KindChoice, proto.LevelInfo, "s1")}).Lead {
		t.Error("a notice and a question should not introduce themselves the same way")
	}

	// FR32's kill switch: with actions off the buttons must not be sent at all,
	// not merely hidden by the surface.
	if a := u.encodeAsk(daemon.View{Item: it}); len(a.Actions) != 0 {
		t.Errorf("actions survived the kill switch: %+v", a.Actions)
	}
}

func TestAskKeyAppliesTheTriageVocabulary(t *testing.T) {
	it := ask(proto.KindChoice, proto.LevelInfo, "s1")
	it.Options = []proto.Option{{Label: "Change the shape"}, {Label: "Migrate on read"}}
	it.Default = "Migrate on read"

	res := &fakeResolver{}
	u := testUI(res, nil)
	u.sess.SetDemo([]wireSession{{ID: "s1", Selected: true}})
	u.sess.routeAsk(daemon.View{Item: it}, true)

	if !u.sess.askKey("a1", "1") {
		t.Fatal("a digit should answer the first option")
	}
	if got := res.answers["a1"]; got != "Change the shape" {
		t.Errorf("answered %q", got)
	}
	if !u.sess.askKey("a1", "Enter") {
		t.Fatal("Enter should take the default")
	}
	if got := res.answers["a1"]; got != "Migrate on read" {
		t.Errorf("Enter answered %q", got)
	}
	if !u.sess.askKey("a1", "d") {
		t.Fatal("d should dismiss (FR50)")
	}
	if len(res.dismiss) != 1 {
		t.Errorf("dismiss = %v", res.dismiss)
	}

	if u.sess.askKey("a1", "3") {
		t.Error("a digit past the options must not answer")
	}
	if u.sess.askKey("a1", "q") {
		t.Error("an unmapped key must not act")
	}
	if u.sess.askKey("someone-else", "1") {
		t.Error("a key for another item must not answer this one")
	}

	u.sess.routeAsk(daemon.View{}, true)
	if u.sess.askKey("a1", "1") {
		t.Error("a key answered an item that is no longer on screen")
	}
}

func TestAskKeyConfirmAndNotice(t *testing.T) {
	res := &fakeResolver{}
	u := testUI(res, nil)
	u.sess.SetDemo([]wireSession{{ID: "s1", Selected: true}})

	u.sess.routeAsk(daemon.View{Item: ask(proto.KindConfirm, proto.LevelInfo, "s1")}, true)
	if !u.sess.askKey("a1", "y") || res.answers["a1"] != "yes" {
		t.Errorf("y answered %q", res.answers["a1"])
	}
	if !u.sess.askKey("a1", "n") || res.answers["a1"] != "no" {
		t.Errorf("n answered %q", res.answers["a1"])
	}

	u.sess.routeAsk(daemon.View{Item: ask(proto.KindNotify, proto.LevelInfo, "s1")}, true)
	if !u.sess.askKey("a1", "d") {
		t.Error("d should dismiss a notice")
	}
	if u.sess.askKey("a1", "y") {
		t.Error("a notice has nothing to say yes to")
	}
}

// pendingAsk is what the window-close path reads to know a question would be
// stranded, so it has to be honest about both states.
func TestPendingAskTracksTheRoutedItem(t *testing.T) {
	u := testUI(&fakeResolver{}, nil)
	u.sess.SetDemo([]wireSession{{ID: "s1", Selected: true}})

	if _, ok := u.sess.pendingAsk(); ok {
		t.Fatal("nothing is routed yet")
	}
	u.sess.routeAsk(daemon.View{Item: ask(proto.KindChoice, proto.LevelInfo, "s1")}, true)
	if v, ok := u.sess.pendingAsk(); !ok || v.Item.ID != "a1" {
		t.Fatal("the routed question is not readable")
	}

	// The same item with the window shut belongs to a card, and the panel must let
	// go of it - otherwise closing the window would leave two claims on one item.
	u.sess.routeAsk(daemon.View{Item: ask(proto.KindChoice, proto.LevelInfo, "s1")}, false)
	if _, ok := u.sess.pendingAsk(); ok {
		t.Error("the panel kept a question the card is now showing")
	}
}

// Closing a session takes its row off the switcher and moves the selection to
// the neighbour: a surface pointing at a session that no longer exists shows an
// empty conversation with no way back.
func TestDropRowMovesSelectionToTheNeighbour(t *testing.T) {
	rows := []wireSession{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	id := func(s wireSession) string { return s.ID }

	kept, sel := dropRow(rows, id, "b", "b")
	if len(kept) != 2 || kept[0].ID != "a" || kept[1].ID != "c" {
		t.Fatalf("kept = %+v, want a and c", kept)
	}
	if sel != "a" {
		t.Errorf("selection = %q, want a (the row before the one that went)", sel)
	}

	// Closing the first row falls forward, since there is nothing before it.
	if _, sel := dropRow(rows, id, "a", "a"); sel != "b" {
		t.Errorf("selection after closing the first row = %q, want b", sel)
	}
	// Closing a row that is not selected leaves the selection alone.
	if _, sel := dropRow(rows, id, "a", "c"); sel != "a" {
		t.Errorf("selection = %q, want a - closing another row must not move it", sel)
	}
	// The last row leaves nothing selected rather than a stale id.
	if kept, sel := dropRow([]wireSession{{ID: "a"}}, id, "a", "a"); len(kept) != 0 || sel != "" {
		t.Errorf("emptying the list gave %+v / %q, want nothing selected", kept, sel)
	}
	// An id that is not there changes nothing.
	if kept, sel := dropRow(rows, id, "b", "zz"); len(kept) != 3 || sel != "b" {
		t.Errorf("unknown id changed the list: %+v / %q", kept, sel)
	}
}

// Close drops the row it was given and nothing else, and reports the session it
// ended - the demo path exercises it end to end without a `claude` child.
func TestCloseSessionDropsOnlyThatRow(t *testing.T) {
	u := testUI(&fakeResolver{}, nil)
	u.sess.SetDemo([]wireSession{{ID: "s1", Title: "one"}, {ID: "s2", Title: "two"}})
	u.sess.Select("s2")

	u.sess.Close("s2")

	got := u.sess.snapshot()
	if len(got) != 1 || got[0].ID != "s1" {
		t.Fatalf("snapshot = %+v, want only s1", got)
	}
}
