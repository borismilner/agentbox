package webui

import (
	"sync"
	"time"

	"github.com/jezek/xgb/xproto"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
)

// The control strip (FR74): one window on screen for as long as an agent has the
// desktop, and nothing at all when it does not.
//
// Everything about where and how it maps follows from one rule Boris set - "once
// it's gone, I know the hands-off is over and I can work freely with the keyboard
// and the mouse". Presence is the signal, so:
//
//   - It is top-centre, borrowing the screen-is-being-shared convention, and the
//     only placement with room for a whole activity sentence.
//   - It maps without taking the keyboard, and that is not a nicety: it appears
//     while he is mid-sentence in a terminal, and a focus steal would be worse
//     than the problem it solves.
//   - It is top-most. Declining focus is what put the progress bar *underneath* a
//     fullscreen app (x11.raise has the Mutter layering), and a hands-off sign
//     nobody can see is worse than none - it would read as "the desktop is yours"
//     while an agent drives.
//   - It is not in the taskbar and has no close button. It is state, not a card:
//     dismissing it would be lying to yourself about whose desktop it is.
//   - It stays above agentbox's OWN windows too. Boris: "this part must be on top of
//     anything else, even AgentBox can't cover it." A card, a toast or the board mapping
//     later would otherwise land on top of it inside the same stacking layer, and
//     a covered hands-off sign reads as "the desktop is yours" while an agent
//     drives - the one wrong answer this element can give. Two measures, because
//     one is a hint and the other is a fact: notification window type, which
//     Mutter layers above ordinary always-on-top windows, and a keeper that
//     restacks it while a run is live (see keepOnTop).
//
// And one exception to top-most, which is the marker (FR74, session 34). A
// fullscreen window is the one case where staying in front is the wrong answer:
// Boris watches a film, an agent takes the desktop, and a 620x62 strip pinned over
// the top of the picture for the whole run is worse than the problem. But the
// guarantee cannot lapse either - a covered strip reads as "the desktop is yours"
// while an agent drives, which is the one wrong answer this feature can give. So
// while the focused window is fullscreen the strip stops fighting for the top and
// a 4px amber line takes over the very top edge of that screen: too thin to spoil
// a picture, impossible to mistake for nothing. It goes when the strip comes back.
const (
	controlW = 620
	controlH = 62
	// markH is the marker's height. Three pixels reads as a rendering seam on this
	// pitch; four is still nothing over a film and is unmistakably drawn.
	markH = 4
	// keepOnTopEvery is how often the strip restacks itself while a run is live.
	// It is a fact rather than a hint: window type and _NET_WM_STATE_ABOVE both
	// say "keep me up", and both lose to another window mapping into the same
	// layer afterwards. One restack request costs a single X round trip, and the
	// thing it buys is that a hands-off sign is never covered by agentbox's own card.
	keepOnTopEvery = 1200 * time.Millisecond
)

// markKind is why the marker is up, and it is the difference between a sign that
// cannot be covered and one that can. Same window, same four pixels, two window
// treatments: FR74's steps aside from a fullscreen app and must still be seen over
// it; FR95's is demoted for a recording and is allowed to be overlapped.
type markKind int

const (
	markNone markKind = iota
	markFullscreen
	markDemoted
)

type control struct {
	ui *UI

	mu    sync.Mutex
	state *daemon.ControlState
	win   *application.WebviewWindow
	stop  chan struct{} // closed to end the keeper when the run ends

	// The fullscreen marker's half. fs is the last answer the keeper got, so the
	// open and the step-aside happen on the edge rather than every tick.
	fs      bool
	mark    *application.WebviewWindow
	markXID xproto.Window
	kind    markKind
	// winXID is the strip's own X id, recorded where it is already known - on the
	// main thread, inside openWindow. Asking a Wails window for its native handle
	// from any other goroutine is a GTK call off the UI thread, which is not a
	// thing to do for a number that never changes.
	winXID xproto.Window

	// Recording mode (FR95). quiet is the last state the daemon sent, kept so the
	// promote path can tell "still loud" from "loud again". stripMon is where the
	// strip was when it was last on screen: the demoted marker belongs on that
	// monitor, and by the time it opens there is no strip left to ask.
	quiet    bool
	stripMon mon
	haveMon  bool
}

// markPlan is what one beat of the keeper decided. It is a value rather than a
// pile of branches because the rule is the interesting part and a display is not
// available to a test: see planMark.
type markPlan struct {
	mark    bool // the marker belongs on screen
	markMon mon  // and on this monitor - the fullscreen window's, not the pointer's
	step    bool // the strip stops claiming the top: it is what is being covered
}

// planMark is the decision, over rectangles.
//
// The marker follows the fullscreen window, because that is the screen being
// looked at. The strip only steps aside when the fullscreen window is on the strip's
// OWN monitor - that is the only case where it is in the way. On the other screen
// the strip is covering nothing and hiding it would trade a legible sign for a
// four-pixel one at no gain, so it keeps its place and the marker joins it.
func planMark(fs bool, fsM, stripM mon) markPlan {
	if !fs {
		return markPlan{}
	}
	return markPlan{mark: true, markMon: fsM, step: fsM == stripM}
}

func newControlStrip(ui *UI) *control { return &control{ui: ui} }

// ShowControl paints the strip, opening the window if this is the start of a run.
// Satisfies the surface half of the daemon's control (daemon.SetSurface).
func (u *UI) ShowControl(st *daemon.ControlState) {
	u.ctrl.set(st)
}

// HideControl ends the run and takes the window with it, which is the whole of how
// the human learns the desktop is his again.
func (u *UI) HideControl() {
	u.ctrl.clear()
}

func (c *control) set(st *daemon.ControlState) {
	c.mu.Lock()
	c.state = st
	w := c.win
	wasQuiet := c.quiet
	c.quiet = st.Quiet
	c.mu.Unlock()

	// The emit goes to every open surface, the marker's included: that is how four
	// pixels learn they are green rather than amber while he is paused (FR95).
	c.ui.emit("agentbox:control", st)

	switch p := planSurface(st.Quiet, wasQuiet, w != nil); {
	case p.demote:
		c.demote()
	case p.promote:
		c.promote()
	case p.open:
		c.openWindow()
	}
}

// surfacePlan is what one repaint means for the two windows this owns. Pulled out
// for the same reason planMark was: the rule is the interesting part, and neither
// window is available to a test.
type surfacePlan struct {
	demote  bool // the strip comes down, the 4px marker goes up (FR95)
	promote bool // the marker goes, the strip comes back with its whole treatment
	open    bool // first paint of a run: the strip has no window yet
}

// planSurface decides between the strip and the demoted marker.
//
// demote is returned on EVERY repaint while recording mode is on, not only on the
// edge into it, and that is deliberate: a run that starts while the mode is already
// armed has to come up demoted, and the daemon's answer to "is it quiet" is the only
// thing that knows. demote itself is idempotent, which is what makes that safe.
func planSurface(quiet, wasQuiet, haveStrip bool) surfacePlan {
	switch {
	case quiet:
		return surfacePlan{demote: true}
	case wasQuiet:
		// Loud again. promote covers both halves, because after a demote there is
		// never a strip window left to keep.
		return surfacePlan{promote: true}
	case !haveStrip:
		return surfacePlan{open: true}
	default:
		return surfacePlan{} // a repaint of a strip that is already up
	}
}

// demote is recording mode arriving (FR95): the strip comes off the screen and the
// 4px marker takes its place, mapped and then left alone. Idempotent, because it is
// reached from every repaint while the mode is on and not only from the transition.
//
// The keeper stops with it. Its whole job is restacking the strip and holding the
// FR74 marker in front of a fullscreen window, and both are exactly what this mode
// gives up - a beat still running here would raise the sign back over the recording
// 1.2 seconds after it was demoted.
func (c *control) demote() {
	c.mu.Lock()
	w, xid := c.win, c.winXID
	already := c.mark != nil && c.kind == markDemoted
	c.mu.Unlock()

	// Read the monitor before the window that answers for it goes.
	if xid != 0 && c.ui.x != nil {
		m := c.ui.x.windowMon(xid)
		c.mu.Lock()
		c.stripMon, c.haveMon = m, true
		c.mu.Unlock()
	}
	c.stopKeeper()
	if w != nil {
		c.mu.Lock()
		c.win, c.winXID = nil, 0
		c.mu.Unlock()
		// Drop the slot here rather than leaving it to the closing hook. The hook
		// now declines to speak for a window that is no longer the live one (see
		// openWindow), which is what stops it stealing the NEXT strip's slot - so
		// this path has to release its own, or the column keeps a 62px gap that
		// every toast sits below.
		c.ui.top.drop("control")
		application.InvokeSync(func() { w.Close() })
	}
	if already {
		return
	}
	// A marker already up for the fullscreen reason is the WRONG one now: it has
	// the notification type and the keeper behind it, which is the whole thing
	// being given up here. Swap it rather than keeping it.
	c.closeMark()
	c.mu.Lock()
	m, have := c.stripMon, c.haveMon
	c.fs = false
	c.mu.Unlock()
	if !have {
		if c.ui.x == nil {
			return
		}
		// Armed on an idle desktop, so there has never been a strip to ask. The
		// pointer's screen is the best answer available and the right one on the
		// single-monitor case this is nearly always used on.
		m = c.ui.x.activeMon()
	}
	c.openMark(m, markDemoted)
}

// promote is going loud again: the demoted marker goes and the strip comes back
// with all of its treatment - notification type, ABOVE, and the keeper that makes
// "nothing of agentbox's own may cover it" a fact rather than a hint.
func (c *control) promote() {
	c.mu.Lock()
	kind, w := c.kind, c.win
	c.mu.Unlock()
	if kind == markDemoted {
		c.closeMark()
	}
	if w == nil {
		c.openWindow()
	}
}

// stopKeeper ends the beat without touching the windows, for the one caller that
// wants the strip's restacking to stop while the run lives on.
func (c *control) stopKeeper() {
	c.mu.Lock()
	if c.stop != nil {
		close(c.stop)
		c.stop = nil
	}
	c.mu.Unlock()
}

func (c *control) clear() {
	c.mu.Lock()
	c.state = nil
	w := c.win
	m := c.mark
	c.mark, c.markXID, c.fs, c.kind = nil, 0, false, markNone
	if c.stop != nil {
		close(c.stop)
		c.stop = nil
	}
	c.mu.Unlock()

	c.ui.emit("agentbox:control", nil)
	if w != nil {
		application.InvokeSync(func() { w.Close() })
	}
	// The marker goes with the run, not with the fullscreen state: the desktop is
	// his again, and nothing of this may outlive that.
	if m != nil {
		application.InvokeSync(func() { m.Close() })
	}
}

func (c *control) snapshot() *daemon.ControlState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *control) openWindow() {
	c.ui.onMain("control", func() {
		w := c.ui.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:             "agentbox-control",
			Title:            "agentbox · hands off",
			Width:            controlW,
			Height:           controlH,
			MinWidth:         360,
			MinHeight:        44,
			Frameless:        true,
			Hidden:           true,
			URL:              "/?surface=control",
			BackgroundType:   application.BackgroundTypeSolid,
			BackgroundColour: rgba(c.ui.themeGround()),
		})

		c.mu.Lock()
		c.win = w
		c.mu.Unlock()

		w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
			// Only for the window this hook belongs to. FR95 closes and reopens the
			// strip on every demote and promote, and Wails delivers this event
			// asynchronously: the old window's hook landed AFTER the new one had
			// claimed its slot, dropped it, and left the returning strip wherever
			// Mutter had first placed it - measured on screen at +1300+96 instead of
			// top-centre. Comparing the window is what makes the late event harmless.
			c.mu.Lock()
			mine := c.win == w
			if mine {
				c.win, c.winXID = nil, 0
			}
			c.mu.Unlock()
			if !mine {
				return
			}
			// Release the slot, or the column keeps a gap where the strip was and
			// every toast under it stays pushed down for nothing.
			c.ui.top.drop("control")
		})

		if c.ui.x != nil {
			if xid := xidOf(w.NativeWindow()); xid != 0 {
				// notification type, set before the first map: Mutter layers a
				// notification above ordinary always-on-top windows, which is what
				// keeps agentbox's own cards from landing on top of this one.
				c.ui.x.prepare(xid, true)
				c.ui.x.quiet(xid)
				showNoActivate(w.NativeWindow())
				c.ui.x.above(xid)
				c.ui.x.unlisted(xid)
				// The strip claims the first slot of the top-centre column rather
				// than placing itself: it shares that edge with the toasts, which is
				// where a browser puts "your screen is being shared" and the meaning
				// is borrowed on purpose. first: nothing of agentbox's own may cover it, and
				// being second in a column is a quieter way of being covered (FR75).
				c.ui.top.put("control", xid, controlW, controlH, true)
				c.ui.x.setName(xid, "agentbox · hands off")
				c.mu.Lock()
				c.winXID = xid
				c.mu.Unlock()
				c.keepOnTop(xid)
				return
			}
		}
		w.Show()
	})
}

// keepOnTop restacks the strip for as long as the run lasts, so a window mapped
// after it - including one of agentbox's own - cannot end up in front. It never touches
// focus: x11.raise uses _NET_RESTACK_WINDOW for exactly this, restacking without
// activating, which is what lets agentbox be in front without taking the keyboard.
func (c *control) keepOnTop(xid xproto.Window) {
	stop := make(chan struct{})
	c.mu.Lock()
	if c.stop != nil {
		close(c.stop) // a previous run's keeper, if the window was reopened
	}
	c.stop = stop
	c.mu.Unlock()

	go func() {
		tick := time.NewTicker(keepOnTopEvery)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if c.ui.x == nil {
					return
				}
				c.beat(xid)
			}
		}
	}()
}

// beat is one tick of the keeper: read the desktop, decide, act. Split out from
// keepOnTop because it now has a decision in it and the goroutine does not.
func (c *control) beat(strip xproto.Window) {
	x := c.ui.x
	fsWin, fs := x.fullscreenActive()
	var fsM mon
	if fs {
		fsM = x.windowMon(fsWin)
	}
	// The strip's own monitor, read from the window rather than from the pointer:
	// mid-run the pointer is wherever the agent last moved it, and the strip has
	// not moved since it mapped.
	plan := planMark(fs, fsM, x.windowMon(strip))

	c.mu.Lock()
	was := c.fs
	c.fs = plan.mark
	markXID := c.markXID
	c.mu.Unlock()

	if !plan.mark {
		if was {
			c.closeMark()
		}
		x.raise(strip)
		return
	}
	if !was {
		c.openMark(plan.markMon, markFullscreen)
		if plan.step {
			x.lower(strip)
		}
		return
	}
	if !plan.step {
		x.raise(strip)
	}
	// The marker needs the keeper for the same reason the strip did: window type
	// and ABOVE are hints, and the window that maps next wins without this.
	if markXID != 0 {
		x.raise(markXID)
		x.moveResize(markXID, plan.markMon.x, plan.markMon.y, plan.markMon.w, markH)
		x.flush()
	}
}

// openMark puts the line across the top edge of a monitor. Not in the top-centre
// column, which this is not in: it sits ON the screen edge, above where the column
// starts, and it is the one agentbox window whose whole job is to be four pixels of
// nothing in particular.
//
// kind decides its treatment, and the two are opposites. markFullscreen keeps the
// strip's own - notification type, ABOVE, raised on every beat - because it is
// standing in for a sign that must be seen over a fullscreen app. markDemoted (FR95)
// drops all three: it is mapped once and then left alone, so a window over the top
// edge covers it, which is what a screen recording needs.
func (c *control) openMark(m mon, kind markKind) {
	c.ui.onMain("control-mark", func() {
		c.mu.Lock()
		already := c.mark != nil
		c.kind = kind
		c.mu.Unlock()
		if already {
			return
		}
		w := c.ui.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:             "agentbox-control-mark",
			Title:            "agentbox · hands off marker",
			Width:            m.w,
			Height:           markH,
			Frameless:        true,
			Hidden:           true,
			URL:              "/?surface=mark",
			BackgroundType:   application.BackgroundTypeSolid,
			BackgroundColour: rgba(c.ui.themeWarn()),
		})

		c.mu.Lock()
		c.mark = w
		c.mu.Unlock()

		w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
			c.mu.Lock()
			c.mark, c.markXID, c.kind = nil, 0, markNone
			c.mu.Unlock()
		})

		if c.ui.x != nil {
			if xid := xidOf(w.NativeWindow()); xid != 0 {
				if kind == markDemoted {
					// Everything except the stacking (FR95). No notification type, no
					// ABOVE and no raise: a newly mapped window is on top of the stack
					// anyway, so it is visible from the first frame, and the next window
					// over the top edge simply covers it. Measured on this desktop: with
					// the type dropped the line is still visible over GNOME's own top
					// bar, which is what makes a demoted sign a sign at all.
					c.ui.x.plain(xid)
				} else {
					c.ui.x.prepare(xid, true)
					c.ui.x.above(xid)
				}
				c.ui.x.quiet(xid)
				showNoActivate(w.NativeWindow())
				// The demoted one gets the version that does NOT add ABOVE back.
				// unlisted sends it as a post-map client message, which is the route
				// the WM honours, so this line is the one that decides whether a
				// fullscreen window can cover the sign.
				if kind == markDemoted {
					c.ui.x.unlistedPlain(xid)
				} else {
					c.ui.x.unlisted(xid)
				}
				c.ui.x.setName(xid, "agentbox · hands off marker")
				// After the map, and again on every beat: GTK opens the window at
				// whatever it thinks the minimum is, and the WM places it where it
				// likes. Four pixels tall is neither.
				c.ui.x.moveResize(xid, m.x, m.y, m.w, markH)
				if kind != markDemoted {
					c.ui.x.raise(xid)
				}
				c.ui.x.flush()
				c.mu.Lock()
				c.markXID = xid
				c.mu.Unlock()
				return
			}
		}
		w.Show()
	})
}

func (c *control) closeMark() {
	c.mu.Lock()
	m := c.mark
	c.mark, c.markXID, c.kind = nil, 0, markNone
	c.mu.Unlock()
	if m != nil {
		application.InvokeSync(func() { m.Close() })
	}
}

// Control is the surface asking for the run it should paint, on mount. A window
// that opens mid-run must not start blank, and it must not restart the activity
// clock either - the age comes from the daemon.
func (b *Bridge) Control() *daemon.ControlState {
	return b.ui.ctrl.snapshot()
}

// ControlDeny is the human pressing Deny on the strip. It answers the agent's
// blocked request, which is what makes the strip the place the decision happens
// rather than a card beside it.
func (b *Bridge) ControlDeny(id string) proto.ControlResult {
	v := b.ui.handoverSrc()
	if v == nil {
		return proto.ControlResult{}
	}
	v.Deny(id)
	return proto.ControlResult{OK: true}
}

// ControlAllow is the human granting it now instead of waiting the countdown out.
// Silence already grants, so this only buys back the remaining seconds - which is
// worth a button when an agent is waiting on him and he is right there.
func (b *Bridge) ControlAllow(id string) proto.ControlResult {
	v := b.ui.handoverSrc()
	if v == nil {
		return proto.ControlResult{}
	}
	v.Allow(id)
	return proto.ControlResult{OK: true}
}

// ControlPause is the human taking the desktop back mid-run from the strip
// itself (FR94). The strip is the one target that exists by construction - it is
// always on screen and always on top while a run is live - which is why the
// button belongs here and not on a card somewhere. The hotkey is the fast path;
// this is the discoverable one.
func (b *Bridge) ControlPause() proto.ControlResult {
	v := b.ui.handoverSrc()
	if v == nil {
		return proto.ControlResult{}
	}
	return v.Pause("the strip")
}

// ControlResume hands it back, and is deliberately the only way out of a pause
// together with the hotkey and the CLI: no agent can un-pause itself.
func (b *Bridge) ControlResume() proto.ControlResult {
	v := b.ui.handoverSrc()
	if v == nil {
		return proto.ControlResult{}
	}
	return v.Resume("the strip")
}

// Handover is the daemon's control as the surface needs it: the answers the
// human can give. Its own keyhole, like Voice, so the surface cannot reach
// anything else on the way.
type Handover interface {
	Deny(id string)
	Allow(id string)
	Pause(how string) proto.ControlResult
	Resume(how string) proto.ControlResult
}

// SetHandover wires it. Nil is valid: a demo build has a strip to look at and
// nobody behind it, and the buttons then report OK and change nothing rather than
// erroring at somebody who can do nothing about it.
func (u *UI) SetHandover(h Handover) {
	u.mu.Lock()
	u.handover = h
	u.mu.Unlock()
}

func (u *UI) handoverSrc() Handover {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.handover
}
