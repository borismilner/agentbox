package webui

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/config"
	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// confUI is a UI carrying the default configuration and nothing else: enough for
// every helper that reads a size, a measure or a fraction out of it. The window
// geometry is configuration now, so a test UI without a config would measure
// everything as zero.
func confUI() *UI {
	u := &UI{log: slog.New(slog.NewTextHandler(io.Discard, nil)), theme: Theme{Mode: "dark"},
		cfg: config.Default()}
	u.sess = newSessions(u)
	u.inbox = newInbox(u)
	u.view = newViewer(u)
	u.prog = newProgress(u)
	u.pan = newPanel(u)
	u.board = newBoard(u)
	return u
}

// gateUI is a UI whose "UI thread" runs work inline, so the main-loop gate can
// be driven without a webview.
func gateUI() *UI {
	u := &UI{log: slog.New(slog.NewTextHandler(io.Discard, nil)), theme: Theme{Mode: "dark"},
		cfg: config.Default()}
	u.invoke = func(fn func()) { fn() }
	u.inbox = newInbox(u)
	u.sess = newSessions(u)
	u.view = newViewer(u)
	u.prog = newProgress(u)
	u.board = newBoard(u)
	return u
}

// Window work that arrives before the main loop is up must be held, not run:
// application.InvokeSync dereferences a platform application that does not
// exist until Run, and the daemon presents a restored item from inside
// daemon.New. Nothing may reach the UI thread before loopStarted.
func TestOnMainHoldsWorkUntilTheLoopIsUp(t *testing.T) {
	u := gateUI()
	var ran []string

	u.onMain("card", func() { ran = append(ran, "card-1") })
	u.onMain("viewer", func() { ran = append(ran, "viewer") })
	if len(ran) != 0 {
		t.Fatalf("work ran before the loop was up: %v", ran)
	}

	u.loopStarted()
	if len(ran) != 2 || ran[0] != "card-1" || ran[1] != "viewer" {
		t.Fatalf("replay = %v, want [card-1 viewer] in order", ran)
	}

	// Once up, work runs straight through.
	u.onMain("card", func() { ran = append(ran, "card-2") })
	if len(ran) != 3 || ran[2] != "card-2" {
		t.Fatalf("after the loop is up: %v", ran)
	}
}

// A repeat of the same operation replaces the one waiting rather than queueing
// beside it: two cards presented before the loop starts must not open two
// windows.
func TestOnMainKeyReplacesPendingWork(t *testing.T) {
	u := gateUI()
	var ran []string

	u.onMain("card", func() { ran = append(ran, "first") })
	u.onMain("card", func() { ran = append(ran, "second") })
	u.loopStarted()

	if len(ran) != 1 || ran[0] != "second" {
		t.Fatalf("replay = %v, want just [second]", ran)
	}
}

// A UI with no way onto a UI thread (the encoders-only test UI) drops window
// work instead of panicking on a nil application.
func TestOnMainWithoutAnInvokerDropsWork(t *testing.T) {
	u := &UI{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ran := false
	u.onMain("card", func() { ran = true })
	u.loopStarted()
	if ran {
		t.Error("work ran without a UI thread to run it on")
	}
}

// Present before the loop keeps the view and replays it when the loop comes up -
// the restored-item path that used to take the daemon down on startup. The
// replay re-reads the current view, so an item answered in the meantime shows
// nothing rather than a stale card.
func TestPresentBeforeTheLoopReplaysTheCurrentView(t *testing.T) {
	u := gateUI()
	views := 0
	u.OnView = func(daemon.View) { views++ }

	u.Present(daemon.View{})
	if views != 1 {
		t.Fatalf("OnView fired %d times, want 1", views)
	}
	u.mu.Lock()
	queued := len(u.deferred)
	key := ""
	if queued == 1 {
		key = u.deferred[0].key
	}
	u.mu.Unlock()
	if queued != 1 || key != "present" {
		t.Fatalf("deferred = %d op(s) %q, want one keyed present", queued, key)
	}

	u.loopStarted()
	if views != 2 {
		t.Errorf("OnView fired %d times, want 2 (the replay presents again)", views)
	}
}

// The reuse-or-replace decision runs inside the card closure, on the UI
// thread, not at the call site. Present runs outside the daemon lock, so two
// items can reach showCard at once; the call-site read used to see a nil
// prompt for both, each closure then opened a window, and the loser fell off
// tracking - the card-shaped ghost that outlived every item's resolution
// once shipped. Here the prompt appears between showCard's call and its
// closure running, which is that race in miniature: the closure must hand the
// view to the window it finds, not open a second one.
func TestShowCardDecidesReuseOnTheUIThread(t *testing.T) {
	u := confUI()
	var q []func()
	u.invoke = func(fn func()) { q = append(q, fn) }
	u.up = true

	u.showCard(daemon.View{Item: &proto.Item{ID: "a", Kind: proto.KindConfirm}})
	if len(q) != 1 {
		t.Fatalf("queued %d ops, want 1", len(q))
	}

	// Another item's closure won the window creation while ours waited.
	win := &application.WebviewWindow{}
	u.mu.Lock()
	u.prompt, u.promptKind = win, "card"
	u.mu.Unlock()

	q[0]()

	u.mu.Lock()
	got := u.prompt
	u.mu.Unlock()
	if got != win {
		t.Fatal("the closure replaced the open window instead of reusing it")
	}
}

// shoutingResolver reports the ids it is told about down a channel, so a test can
// wait for a report that arrives on its own goroutine without racing a slice.
type shoutingResolver struct {
	fakeResolver
	shownCh chan string
}

func (s *shoutingResolver) SurfaceShown(id string) { s.shownCh <- id }

// R-06. A queue of three toasts is one window: only the first goes through
// armCard, and the rest arrive down the reuse branch. If that branch stays quiet
// the daemon never learns their surfaces are up, so each waits out the whole
// surface grace and is warned about while sitting on screen perfectly well - and
// its six seconds then begin five seconds late.
func TestAReusedWindowStillReportsItsSurface(t *testing.T) {
	u := confUI()
	res := &shoutingResolver{shownCh: make(chan string, 4)}
	u.res = res
	var q []func()
	u.invoke = func(fn func()) { q = append(q, fn) }
	u.up = true

	// A window is already open in the treatment the next item wants.
	u.mu.Lock()
	u.prompt, u.promptKind = &application.WebviewWindow{}, "toast"
	u.mu.Unlock()

	u.showCard(daemon.View{Item: &proto.Item{ID: "b", Kind: proto.KindNotify, Level: proto.LevelInfo}})
	if len(q) != 1 {
		t.Fatalf("queued %d ops, want 1", len(q))
	}
	q[0]()

	select {
	case got := <-res.shownCh:
		if got != "b" {
			t.Fatalf("reported %q on screen, want the item that was just shown", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the reused window never reported its surface, so the toast's clock never starts")
	}
}

// A close ordered after a show must not run ahead of it: both travel under
// the one "card" onMain key. Before the loop is up that key makes the close
// replace the queued show outright - a card for an item that resolved while
// the loop was still coming up must never open at all (the show would reach
// for an application this test does not have, so surviving loopStarted is
// the assertion).
func TestCloseCardReplacesAQueuedShow(t *testing.T) {
	u := gateUI()

	u.showCard(daemon.View{Item: &proto.Item{ID: "a", Kind: proto.KindConfirm}})
	u.closeCard()

	u.mu.Lock()
	ops := len(u.deferred)
	u.mu.Unlock()
	if ops != 1 {
		t.Fatalf("deferred holds %d ops, want the close to have replaced the show", ops)
	}

	u.loopStarted()

	u.mu.Lock()
	defer u.mu.Unlock()
	if u.prompt != nil {
		t.Fatal("a window survived for a resolved item")
	}
}

// The panel's rectangle comes from the config as a fraction of the screen. The
// floor matters: a fraction small enough to make the panel useless is more likely
// a typo than an intention, and the panel still has to hold a conversation.
func TestPanelSizeFromConfigFractions(t *testing.T) {
	u := gateUI()
	u.pan = newPanel(u)

	u.cfg.Panel.WidthFrac, u.cfg.Panel.HeightFrac = 0.5, 0.5
	if w, h := u.pan.size(); w != 800 || h != 450 {
		t.Errorf("size = %dx%d, want 800x450 against the 1600x900 fallback screen", w, h)
	}

	// Height stops at half the monitor however big the fraction is: a console that
	// covers more than half of what you were reading is not a console.
	u.cfg.Panel.HeightFrac = 0.9
	if _, h := u.pan.size(); h != 450 {
		t.Errorf("height = %d with height_frac 0.9, want half the screen (450)", h)
	}

	// A tiny fraction is floored rather than honoured.
	u.cfg.Panel.WidthFrac, u.cfg.Panel.HeightFrac = 0.21, 0.21
	if w, h := u.pan.size(); w != panelMinW || h != panelMinH {
		t.Errorf("size = %dx%d, want the %dx%d floor", w, h, panelMinW, panelMinH)
	}
}
