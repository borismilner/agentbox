package webui

import (
	"testing"

	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
)

// R-05. The card and toast were the only surfaces with no pull. Their first view
// was pushed as a fire-and-forget Wails event with nothing to buffer it, and
// armCard gave up waiting for the surface after two seconds and emitted anyway -
// two seconds being a guess about WebKit process startup on a loaded machine
// that is measured nowhere. A bundle mounting after that guess got no view at
// all, and nothing re-sent one until the next queue change: the human heard the
// earcon and looked at an empty window while the daemon had already logged
// item.displayed and armed escalation.
//
// The live half of this - a cold daemon under load, driven with uidrive - is NOT
// covered here and is named in the backlog entry. What is covered is that the
// pull exists, answers the same thing the push carries, and is honest when there
// is nothing on screen.

func TestBridgeViewAnswersWhatIsOnScreen(t *testing.T) {
	u := testUI(&fakeResolver{}, &fakeSource{})
	b := &Bridge{ui: u}

	it := &proto.Item{
		ID: "itm-1", Kind: proto.KindChoice, Title: "Where should 2026.7.30 go first?",
		Options:  []proto.Option{{Label: "eu-west"}, {Label: "us-east"}},
		Identity: proto.Identity{Agent: "release-bot", Project: "checkout-api"},
	}
	u.mu.Lock()
	u.cur = daemon.View{Item: it}
	u.mu.Unlock()

	got := b.View()
	if got.Item == nil {
		t.Fatal("View() answered no item while one is on screen: the pull is the " +
			"whole of R-05 and it has nothing to hand a surface that mounted late")
	}
	if got.Item.ID != it.ID || got.Item.Title != it.Title {
		t.Fatalf("View() = %+v, want the displayed item", got.Item)
	}
	if len(got.Item.Options) != 2 {
		t.Errorf("View() dropped the options; a card that pulled this could not be answered")
	}
}

// The push and the pull must be the same payload. If they drift, a card that
// mounted late renders differently from one that mounted in time, and the defect
// this fixes comes back as a subtler one.
func TestBridgeViewMatchesWhatThePushWouldCarry(t *testing.T) {
	u := testUI(&fakeResolver{}, &fakeSource{})
	b := &Bridge{ui: u}

	v := daemon.View{Item: &proto.Item{
		ID: "itm-2", Kind: proto.KindConfirm, Title: "Deploy?",
		Identity: proto.Identity{Agent: "release-bot"},
	}}
	u.mu.Lock()
	u.cur = v
	u.mu.Unlock()

	pushed := u.encode(u.currentView()) // what armCard emits
	pulled := b.View()                  // what a late surface asks for

	if pulled.Item == nil || pushed.Item == nil {
		t.Fatal("one of the two carries no item")
	}
	if pulled.Item.ID != pushed.Item.ID || pulled.Item.Title != pushed.Item.Title {
		t.Errorf("pull %+v and push %+v disagree about the item", pulled.Item, pushed.Item)
	}
	if pulled.Waiting != pushed.Waiting || pulled.Glyph != pushed.Glyph {
		t.Errorf("pull and push disagree about the card's chrome: %+v vs %+v", pulled, pushed)
	}
}

// An empty view is a real answer, not an error. A window can outlive its item,
// and a surface that pulls nothing must know to stay quiet rather than paint
// whatever it had - which is why the frontend guards on v?.item.
func TestBridgeViewIsHonestWhenNothingIsDisplayed(t *testing.T) {
	u := testUI(&fakeResolver{}, &fakeSource{})
	b := &Bridge{ui: u}

	got := b.View()
	if got.Item != nil {
		t.Fatalf("View() invented an item %+v with nothing on screen", got.Item)
	}
}
