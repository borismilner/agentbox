package webui

import (
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
)

// tu is a UI with the default configuration behind it: the toast geometry these tests exercise
// is configuration now, not constants.
var tu = confUI()

// The treatment rule is the whole point of the toast slice: which items get a
// strip at the top and which get a card in the middle. It is table-tested because
// getting it wrong is silent - an urgent notice would slide off the screen unread
// and nothing would report it.
func TestTreatment(t *testing.T) {
	cases := []struct {
		name string
		it   *proto.Item
		want string
	}{
		{"info notify", &proto.Item{Kind: proto.KindNotify, Level: proto.LevelInfo}, "toast"},
		{"success notify", &proto.Item{Kind: proto.KindNotify, Level: proto.LevelSuccess}, "toast"},
		{"warning notify", &proto.Item{Kind: proto.KindNotify, Level: proto.LevelWarning}, "toast"},
		{"error notify", &proto.Item{Kind: proto.KindNotify, Level: proto.LevelError}, "toast"},
		{"levelless notify defaults to info", &proto.Item{Kind: proto.KindNotify}, "toast"},
		// 03-ui-ux.md: urgent skips the toast and becomes a card with escalation.
		{"urgent notify", &proto.Item{Kind: proto.KindNotify, Level: proto.LevelUrgent}, "card"},
		{"question", &proto.Item{Kind: proto.KindChoice, Level: proto.LevelInfo}, "card"},
		{"confirm", &proto.Item{Kind: proto.KindConfirm}, "card"},
		{"veto", &proto.Item{Kind: proto.KindVeto, Level: proto.LevelWarning}, "card"},
		{"diff", &proto.Item{Kind: proto.KindDiff}, "card"},
		{"secret", &proto.Item{Kind: proto.KindSecret}, "card"},
		{"nothing", nil, "card"},
	}
	for _, c := range cases {
		if got := treatment(c.it); got != c.want {
			t.Errorf("%s: treatment = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSeverityGlyph(t *testing.T) {
	cases := map[proto.Level]string{
		proto.LevelInfo:    "info",
		proto.LevelSuccess: "check",
		proto.LevelWarning: "warning",
		proto.LevelError:   "cross",
		proto.LevelUrgent:  "bell",
		"":                 "info", // a level agentbox does not know reads as info
		"nonsense":         "info",
	}
	for level, want := range cases {
		if got := severityGlyph(level); got != want {
			t.Errorf("severityGlyph(%q) = %q, want %q", level, got, want)
		}
	}
}

// Sticky is how the strip knows to say "click to dismiss" instead of counting
// down. It follows the daemon: a deadline means the daemon will take the toast
// away, no deadline means nothing will, and a card is never sticky because a
// question is not something you dismiss by looking at it.
func TestEncodeSticky(t *testing.T) {
	u := &UI{theme: Theme{Mode: "dark"}}

	warn := &proto.Item{ID: "a", Kind: proto.KindNotify, Level: proto.LevelWarning, Title: "Coverage dropped"}
	if got := u.encode(daemon.View{Item: warn}); !got.Sticky {
		t.Error("a warning notice with no dismiss deadline should be sticky")
	}

	ok := &proto.Item{ID: "b", Kind: proto.KindNotify, Level: proto.LevelSuccess, Title: "Build passed"}
	got := u.encode(daemon.View{Item: ok, DismissAt: time.Now().Add(6 * time.Second)})
	if got.Sticky {
		t.Error("a notice the daemon will close is not sticky")
	}
	if got.Glyph != "check" {
		t.Errorf("glyph = %q, want check", got.Glyph)
	}
	if got.DismissAtMS == 0 {
		t.Error("the dismiss deadline has to reach the surface for the countdown")
	}

	ask := &proto.Item{ID: "c", Kind: proto.KindChoice, Level: proto.LevelWarning, Title: "Migrate?"}
	if u.encode(daemon.View{Item: ask}).Sticky {
		t.Error("a card is never sticky")
	}
}

// The opening height only has to be close - the surface measures itself and calls
// Fit - but it must stay tight (a strip, not a card) and bounded.
func TestToastHeight(t *testing.T) {
	bare := tu.toastHeight(&proto.Item{Kind: proto.KindNotify, Title: "Rebased onto main"})
	if bare > 90 {
		t.Errorf("a titleless-body toast opened at %dpx; a strip should be tighter", bare)
	}

	withBody := tu.toastHeight(&proto.Item{Kind: proto.KindNotify, Title: "x", Body: "one\ntwo\nthree\nfour\nfive"})
	if withBody <= bare {
		t.Error("a body should make the strip taller")
	}

	withActions := tu.toastHeight(&proto.Item{
		Kind: proto.KindNotify, Title: "x", Body: "one\ntwo",
		Actions: []proto.Action{{Label: "Open report"}},
	})
	if withActions <= tu.toastHeight(&proto.Item{Kind: proto.KindNotify, Title: "x", Body: "one\ntwo"}) {
		t.Error("action buttons need room")
	}

	long := make([]byte, 0, 4000)
	for range 200 {
		long = append(long, "a line of body text that wraps and wraps\n"...)
	}
	_, maxToastH, _ := tu.toastGeom()
	if h := tu.toastHeight(&proto.Item{Kind: proto.KindNotify, Title: "x", Body: string(long)}); h > maxToastH {
		t.Errorf("height = %d, want <= %d: a long body scrolls inside the strip", h, maxToastH)
	}
}
