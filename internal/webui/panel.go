package webui

import (
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The drop-down panel (M10): a session you can reach without going to look for
// it. A hotkey rolls it down from the top edge, over whatever you were doing, and
// the same hotkey rolls it back up. It shows the session surface the app window
// shows - one conversation, two ways in - so dropping it down mid-task lands you
// where you already were.
//
// Two things about it are deliberately unlike every other agentbox window:
//
//   - It takes the keyboard. Vision principle 3 says agentbox never grabs focus, but
//     that rule is about agentbox interrupting you; this is you summoning agentbox, the
//     same exception `agentbox summon` has always had. You are going to type in it.
//
//   - It animates, and how it animates was forced by two measurements (both made
//     against this GTK4/Mutter/X11 stack, both worth knowing before touching it):
//     Mutter CLAMPS a managed window to the work area, so a fixed-size window
//     cannot be parked above the top edge and slid down - a negative y is silently
//     refused. And the window is NOT translucent even with WindowIsTranslucent and
//     a transparent background (ADR-0009 again), so the web way - a full-height
//     transparent window with a CSS transform - is not available either.
//
//     What is left, and what this does: pin the window at the top edge of the
//     monitor, centred on it, and animate its HEIGHT. The surface simply fills the
//     window. It used to lay its content out once into a fixed-height box anchored
//     to the viewport's bottom, so a frame cost a composite rather than a reflow -
//     and that measured height went stale the moment the panel appeared on a
//     monitor of a different size, leaving a band of empty background above the
//     header that reads exactly like a window somebody dragged out of place. With
//     the roll off by default, an occasional reflow is the cheaper mistake.
//
//     Top-centre of ONE monitor, decided once per roll (see resolve). It used to
//     centre in the X root, which on two screens is the seam between them.
//
//     And the roll is OFF by default ([panel] slide_ms = 0), because it was shown
//     to Boris and it is not what a game console does. The measurements, so nobody
//     spends the afternoon again: on the clock it holds 38 frames in 222 ms, about
//     170 a second, more than the display shows - it is not dropping frames and it
//     still does not read as sliding. The reason is the mechanism, not the timing.
//     Quake moves a texture inside one surface that is already mapped; this resizes
//     the WINDOW, so every frame is a compositor re-composite and a GTK re-clip of
//     the webview, and the eye reads the growing rectangle rather than movement.
//     The one route that would actually slide is an override-redirect window, which
//     the WM does not manage and therefore does not clamp to the work area: park it
//     full-height at a negative y and move it down. That means owning focus, input
//     and stacking by hand, which is a different window, not a tuning of this one.
//     Set slide_ms if you want the roll; everything below still honours it.
const (
	// panelFrameGap is how often the roll WANTS a frame. It is not how often it
	// gets one: a resize plus a composite of a window this wide costs more than
	// this on a busy compositor, and the clock-driven loop below simply drops the
	// frames it cannot afford. Asking for more than the display can show costs
	// nothing and asking for fewer is visible as stepping.
	panelFrameGap   = 6 * time.Millisecond
	panelShutHeight = 1 // the height it rests at while up: a line, never zero
)

// panelEase is a cubic ease-out: it leaves fast and settles, so the panel reads as
// arriving rather than stopping. Quadratic was not enough at this duration and
// linear reads as mechanical, which is the complaint that got this rewritten.
func panelEase(f float64) float64 {
	if f <= 0 {
		return 0
	}
	if f >= 1 {
		return 1
	}
	g := 1 - f
	return 1 - g*g*g
}

// slideMS is how long a roll lasts: [panel] slide_ms, which is 0 by default, and
// forced to 0 by theme.motion = "none" - one switch for every animation agentbox has,
// which is both a taste and an accessibility answer.
func (p *panel) slideMS() int {
	c := p.ui.conf()
	if c.Theme.Motion == "none" {
		return 0
	}
	return c.Panel.SlideMS
}

// rolling reports whether there is an animation to run at all. The map depends on
// it: a roll starts shut, no roll goes straight to full height.
func (p *panel) rolling() bool { return p.slideMS() > 0 }

// open01 is the end state when there is no animation at all.
func open01(down bool) float64 {
	if down {
		return 1
	}
	return 0
}

type panel struct {
	ui *UI

	mu        sync.Mutex
	win       *application.WebviewWindow
	open      bool
	animating bool
	w, h      int
	ox, oy    int // top-left of the rolled-down window, in root coordinates
}

func newPanel(ui *UI) *panel { return &panel{ui: ui} }

// Toggle is the hotkey's whole contract: down if it is up, up if it is down.
func (p *panel) Toggle() {
	p.mu.Lock()
	open, animating := p.open, p.animating
	p.mu.Unlock()
	if animating {
		return // mid-roll; a second press should not fight the first
	}
	if open {
		p.Hide()
		return
	}
	p.Show()
}

// Open reports whether the panel is down, for the tray and the CLI.
func (p *panel) Open() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.open
}

func (p *panel) Show() {
	p.mu.Lock()
	if p.animating {
		p.mu.Unlock()
		return
	}
	w := p.win
	p.mu.Unlock()

	// A session before the window, not after: the composer is the point of the
	// panel, and it should be typeable the moment the panel is there.
	p.ui.sess.EnsureOne()

	if w == nil {
		p.openWindow()
		return
	}
	// Re-resolve before it is visible again: you may have rolled it up on one
	// screen and asked for it back while sitting at the other, and a window that
	// maps at its old position and then jumps is worse than either. The showing
	// itself belongs to slide - see the note there about why nothing else may
	// dispatch to the UI thread during a roll.
	p.resolve()
	go p.slide(true)
}

func (p *panel) Hide() {
	p.mu.Lock()
	w, open := p.win, p.open
	p.mu.Unlock()
	if w == nil || !open {
		return
	}
	go func() {
		p.slide(false)
		p.ui.onMain("panel.hide", func() {
			w.Hide()
			w.SetSize(p.w, panelShutHeight) // shut, ready to grow again
		})
		// A question being answered in this conversation has just lost its
		// surface, so it needs somewhere else to be - a card, unless the app
		// window is up and can still hold it. After slide(), never before: slide
		// is what clears p.open, and routing asked any earlier still sees a host.
		p.ui.rerouteAsk()
	}()
}

// monitor is the screen the panel belongs to: the one the pointer is on. The
// 1600x900 stand-in is for a UI with no X connection at all (a test, or Wayland),
// where nothing is going to be placed anyway.
func (p *panel) monitor() mon {
	if p.ui.x != nil {
		return p.ui.x.activeMon()
	}
	return mon{w: 1600, h: 900}
}

// panelMinW / panelMinH are the smallest panel that is still a console: narrower
// than this and a line of prose wraps every three words, shorter and there is room
// for the composer or the conversation but not both. They are the floor for the
// configured fraction AND the window's own minimum size, so dragging an edge
// cannot produce the sliver Boris managed to drag.
const (
	panelMinW = 720
	panelMinH = 360
)

// sizeOn is the panel's rectangle on a given monitor: a fraction of it from the
// config ([panel] width_frac / height_frac), floored at something a conversation
// fits in and capped at half the monitor's height - a drop-down console that
// covers more than half of what you were looking at has stopped being one. On a
// screen too small for the floor, the screen wins.
func (p *panel) sizeOn(m mon) (int, int) {
	wf, hf := p.ui.panelFracs()
	w, h := int(float64(m.w)*wf), int(float64(m.h)*hf)
	if half := m.h / 2; half > 0 && h > half {
		h = half
	}
	if w < panelMinW {
		w = min(panelMinW, m.w)
	}
	if h < panelMinH {
		h = min(panelMinH, m.h)
	}
	return w, h
}

func (p *panel) size() (int, int) { return p.sizeOn(p.monitor()) }

// resolve settles where the panel is about to be and remembers it: the monitor is
// chosen ONCE per roll, before the first frame, because the slide moves the window
// on every frame and re-asking mid-animation would let the panel chase the pointer
// across the seam. It rolls down from the top edge of that monitor, centred on it.
func (p *panel) resolve() (int, int, int, int) {
	m := p.monitor()
	w, h := p.sizeOn(m)
	ox, oy := m.x+atLeast0((m.w-w)/2), m.y

	p.mu.Lock()
	p.w, p.h, p.ox, p.oy = w, h, ox, oy
	p.mu.Unlock()
	return ox, oy, w, h
}

// resizeToConfig re-shapes the panel after a config change. If it is down it
// resizes under you (that is the point - you are tuning it while looking at it);
// if it is up, the next roll uses the new numbers.
func (p *panel) resizeToConfig() {
	p.mu.Lock()
	win, open, wasW, wasH := p.win, p.open, p.w, p.h
	p.mu.Unlock()

	px, py, w, h := p.resolve()
	if win == nil || (w == wasW && h == wasH) {
		return
	}
	if open {
		p.ui.onMain("panel.resize", func() {
			win.SetSize(w, h)
			if p.ui.x != nil {
				if xid := xidOf(win.NativeWindow()); xid != 0 {
					p.ui.x.moveTo(xid, px, py)
					p.ui.x.flush()
				}
			}
		})
		p.ui.emit("agentbox:panel", true) // the surface re-reads its height
	}
}

func (p *panel) openWindow() {
	ox, oy, w, _ := p.resolve() // the height is the slide's business, not the map's
	p.ui.onMain("panel", func() {
		win := p.ui.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:        "agentbox-panel",
			Title:       "agentbox · panel",
			Width:       w,
			Height:      panelShutHeight, // it opens by growing; see slide
			Frameless:   true,
			AlwaysOnTop: true,
			Hidden:      true,
			URL:         "/?surface=panel",
			// A floor on the drag, not just on the configured size. MinHeight is
			// deliberately NOT panelMinH: the panel legitimately lives at one pixel
			// while it is up, and a minimum the WM enforces would fight the roll.
			MinWidth: panelMinW,
			// Opaque on purpose: there is no ARGB visual here (see the note above),
			// so the panel owns its rectangle and the slide is what sells it.
			BackgroundType:   application.BackgroundTypeSolid,
			BackgroundColour: rgba(p.ui.themeGround()),
		})

		p.mu.Lock()
		p.win = win
		p.mu.Unlock()

		win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
			p.mu.Lock()
			p.win, p.open = nil, false
			p.mu.Unlock()
			// Same reason as Hide's: an inline question here has lost its surface.
			// Off this goroutine because rerouting places a window and this is the
			// main loop closing one.
			go p.ui.rerouteAsk()
		})

		// Prepared and pinned to the top edge before it is ever visible, so the map
		// itself cannot flash the panel at full height: it is one line tall until
		// the slide grows it. The mapping is slide's, not this function's.
		if p.ui.x != nil {
			if xid := xidOf(win.NativeWindow()); xid != 0 {
				p.ui.x.prepare(xid, false)
				p.ui.x.moveTo(xid, ox, oy)
				p.ui.x.setName(xid, "agentbox · panel")
			}
		}
		go p.slide(true)
	})
}

// slide runs the roll, one frame per main-loop turn. It must NOT run on the UI
// thread: GTK only applies a resize when the main loop iterates, so a loop that
// blocks the thread produces exactly two visible sizes - shut, then open - which
// is what the first version of this did. Each frame is dispatched onto the thread
// and the wait between frames happens here, off it.
//
// The roll is driven by the CLOCK, not by a frame count, and that is the whole
// difference between smooth and mechanical. Each frame dispatches onto the GTK
// thread and BLOCKS until it has run, so a frame costs whatever a resize plus a
// composite costs that moment - and a fixed sleep after each one turns every slow
// frame into a longer roll with uneven steps. Reading the clock instead means the
// height is a function of elapsed time: a slow frame is dropped rather than
// stretched, and the panel always takes exactly [panel] slide_ms to arrive.
//
// A roll owns the UI thread for its duration, and that is a rule rather than an
// observation. Show used to dispatch "size it shut, then map it" while this
// function dispatched the first frame from a different goroutine; InvokeSync does
// not order two goroutines, so when the frame won the panel ended up ONE PIXEL
// TALL with the state machine certain it was open - a hotkey that did nothing,
// twice out of three presses. Everything a roll needs to do on the UI thread is
// dispatched from here, in order, including the map.
func (p *panel) slide(down bool) {
	p.mu.Lock()
	x, win, h := p.ui.x, p.win, p.h
	px, py, w := p.ox, p.oy, p.w // fixed by resolve; never re-asked mid-roll
	p.animating = true
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.animating = false
		p.open = down
		p.mu.Unlock()
	}()

	if x == nil || win == nil {
		return // no X11: the window is simply there, which is still usable
	}
	xid := xidOf(win.NativeWindow())
	if xid == 0 {
		return
	}

	// The map. Shut first when there is a roll to run, so it has somewhere to grow
	// from; straight to full height when there is not, so nothing flashes a
	// one-pixel line on the way. Position goes on in the same op: the panel must
	// never be visible anywhere but where resolve put it.
	if down {
		start := h
		if p.rolling() {
			start = panelShutHeight
		}
		p.ui.onMain("panel.show", func() {
			win.SetSize(w, start) // keeps Wails' own bookkeeping in step
			win.Show()
			x.moveResize(xid, px, py, w, start)
			x.flush()
		})
		// Above everything and out of the taskbar, claimed after the map like every
		// other agentbox window (the WM owns _NET_WM_STATE once mapped).
		x.unlisted(xid)
	}

	ms := p.slideMS()

	frame := func(e float64) {
		ph := panelShutHeight + int(float64(h-panelShutHeight)*e)
		p.ui.onMain("panel.frame", func() {
			win.SetSize(w, ph)
			// Position with the size, every frame: a growing window drifts, and the
			// size is the WM's to apply (see moveResize).
			x.moveResize(xid, px, py, w, ph)
			x.flush()
		})
	}

	if ms <= 0 {
		frame(open01(down))
	} else {
		// One frame at the starting height before the clock starts. The first
		// resize after a map is much the most expensive one - it was costing the
		// roll 80ms of its 160, all of it in the first step, which is exactly where
		// a pop is visible - and paying for it outside the timed loop means the
		// roll that the eye sees is evenly paced from its first frame.
		if down {
			frame(0)
		}
		dur := time.Duration(ms) * time.Millisecond
		start := time.Now()
		frames := 0
		for {
			el := time.Since(start)
			f := float64(el) / float64(dur)
			if f > 1 {
				f = 1
			}
			if !down {
				f = 1 - f
			}
			frame(panelEase(f))
			frames++
			if el >= dur {
				break
			}
			// Pace off the start, not off now: a frame that overran eats into the
			// next interval instead of pushing the whole roll back.
			time.Sleep(time.Until(start.Add(time.Duration(frames) * panelFrameGap)))
		}
		p.ui.log.Debug("webui.panel_slide", "component", "webui", "down", down,
			"frames", frames, "ms", time.Since(start).Milliseconds(), "want_ms", ms)
	}

	if down {
		// Once more now the window is certainly mapped: the map is asynchronous, so
		// the first attempt above can land too early to stick.
		x.unlisted(xid)
		// Now take the keyboard, which no other agentbox window does on its own: the
		// composer is the point of the panel and you asked for it.
		x.activate(xid)
		p.ui.emit("agentbox:panel", true)
	} else {
		p.ui.emit("agentbox:panel", false)
	}
}

// TogglePanel / ShowPanel / HidePanel are the daemon-facing verbs (the hotkey,
// the tray and `agentbox panel`).
func (u *UI) TogglePanel() { u.pan.Toggle() }
func (u *UI) ShowPanel()   { u.pan.Show() }
func (u *UI) HidePanel()   { u.pan.Hide() }
func (u *UI) PanelOpen() bool {
	if u.pan == nil {
		return false
	}
	return u.pan.Open()
}
