// Package hand moves the real pointer and presses real keys, so an agent can do
// something on the desktop the way the person sitting there would do it.
//
// Why agentbox owns this at all: every other tool in agentbox puts something in front
// of the human and waits. This is the one place where the agent acts on the
// desktop instead of asking about it, and that is exactly why it belongs here
// rather than in a pile of one-off scripts - it is the same daemon, the same
// display, the same log, and the same single place to look when something moved
// that should not have.
//
// It is X11 only, through the XTEST extension: the events it synthesises are
// indistinguishable from a mouse and a keyboard, which is the point (an
// application that "handles automation" specially cannot tell). Wayland
// deliberately has no equivalent, so Open reports that it cannot drive the
// display rather than pretending it did.
//
// Two things learned the hard way and now built in, because both look like "the
// webview ignores the mouse":
//
//   - Window geometry must come from the X server, translated to root
//     coordinates. `wmctrl -lG` reports doubled coordinates on a HiDPI display,
//     so anything computed from it lands in the wrong place.
//   - A click needs the pointer to settle first. Press in the same instant the
//     last motion event goes out and the application sees a press at the old
//     position.
package hand

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"

	"github.com/borismilner/agentbox/internal/hotkey"
)

// Rect is a window or the screen, in root coordinates.
type Rect struct{ X, Y, W, H int }

func (r Rect) String() string { return fmt.Sprintf("%d %d %d %d", r.X, r.Y, r.W, r.H) }

// Hand is one live connection to the display, with the coordinate frame and the
// pacing the script is currently using.
type Hand struct {
	conn   *xgb.Conn
	root   xproto.Window
	screen Rect
	layout *Layout
	rnd    *rand.Rand

	frame Rect
	inWin string // the window the frame came from, for messages
	// The window a script named, when it named one. Holding the id rather than
	// only the rectangle is what lets every click and keystroke be checked
	// against the window it was aimed at (target.go).
	targetWin xproto.Window
	speed     float64
	wpm       int
	settle    time.Duration

	xkb   bool // the XKB handshake succeeded; the group lock works
	xkbOp byte // the extension's major opcode

	// Trace, when set, is called with one line per step. The CLI prints it under
	// --verbose; the daemon logs it.
	Trace func(string)
}

// modifier keysyms, by the mask hotkey.Parse reports.
var modKeysyms = []struct {
	mask   uint16
	keysym uint32
}{
	{uint16(xproto.ModMaskShift), 0xffe1},   // Shift_L
	{uint16(xproto.ModMaskControl), 0xffe3}, // Control_L
	{uint16(xproto.ModMask1), 0xffe9},       // Alt_L
	{uint16(xproto.ModMask4), 0xffeb},       // Super_L
}

// Open connects to the display and reads the keyboard layout. Seed 0 takes a
// varying seed, so two identical scripts do not trace the identical path;
// any other value makes the whole session reproducible.
func Open(seed int64) (*Hand, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("no X11 display to drive: %w", err)
	}
	if err := xtest.Init(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("this display has no XTEST extension, so input cannot be synthesised: %w", err)
	}
	setup := xproto.Setup(conn)
	scr := setup.DefaultScreen(conn)
	h := &Hand{
		conn:   conn,
		root:   scr.Root,
		screen: Rect{W: int(scr.WidthInPixels), H: int(scr.HeightInPixels)},
		speed:  1,
		wpm:    defaultWPM,
		settle: 90 * time.Millisecond,
	}
	h.frame = h.screen
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	h.rnd = rand.New(rand.NewSource(seed))

	first, count := setup.MinKeycode, byte(setup.MaxKeycode-setup.MinKeycode+1)
	m, err := xproto.GetKeyboardMapping(conn, first, uint8(count)).Reply()
	if err != nil || m == nil || m.KeysymsPerKeycode == 0 {
		conn.Close()
		return nil, fmt.Errorf("cannot read the keyboard layout: %w", err)
	}
	syms := make([]uint32, len(m.Keysyms))
	for i, ks := range m.Keysyms {
		syms[i] = uint32(ks)
	}
	h.layout = NewLayout(byte(first), int(m.KeysymsPerKeycode), syms)
	h.xkbInit()
	return h, nil
}

func (h *Hand) Close() {
	if h.conn != nil {
		h.conn.Close()
		h.conn = nil
	}
}

// Screen is the whole display. Frame is the coordinate frame in use.
func (h *Hand) Screen() Rect { return h.screen }
func (h *Hand) Frame() Rect  { return h.frame }

// Speed and WPM set the pacing directly, for callers that are not running a script.
func (h *Hand) SetSpeed(f float64) {
	if f > 0 {
		h.speed = f
	}
}

func (h *Hand) SetWPM(n int) {
	if n > 0 {
		h.wpm = n
	}
}

func (h *Hand) trace(format string, args ...any) {
	if h.Trace != nil {
		h.Trace(fmt.Sprintf(format, args...))
	}
}

// Pointer is where the pointer is now, in root coordinates.
func (h *Hand) Pointer() (Pt, error) {
	r, err := xproto.QueryPointer(h.conn, h.root).Reply()
	if err != nil {
		return Pt{}, fmt.Errorf("cannot read the pointer position: %w", err)
	}
	return Pt{X: int(r.RootX), Y: int(r.RootY)}, nil
}

// UseWindow points the coordinate frame at a window, found by title, and returns
// where it is. It also locks onto that window: from here on every click and
// keystroke is checked against it (target.go), and the window is raised so the
// first one lands in a window that is actually in front and holding the
// keyboard. UseScreen puts the frame back to the whole display and, with it,
// gives up the lock - which is the way to say "I mean the desktop itself".
func (h *Hand) UseWindow(title string) (Rect, error) {
	got, err := h.look(title)
	if err != nil {
		return Rect{}, err
	}
	win := xproto.Window(got.Win)
	h.frame, h.inWin, h.targetWin = got.Rect, got.Name, win
	if err := h.Activate(win); err != nil {
		return Rect{}, err
	}
	if err := h.settleAfterActivate(); err != nil {
		return Rect{}, err
	}
	// Raising can move it (a window manager unmaximises on some paths), so the
	// rectangle the caller gets back is the one after the raise, not before.
	if err := h.follow(); err != nil {
		return Rect{}, err
	}
	return h.frame, nil
}

func (h *Hand) UseScreen() {
	h.frame, h.inWin, h.targetWin = h.screen, "", 0
}

// Look finds a window by title and returns it in root coordinates. Which window
// wins when several match is Choose's business, and it is the part that matters
// in practice.
func (h *Hand) Look(title string) (Rect, error) {
	got, err := h.look(title)
	return got.Rect, err
}

func (h *Hand) look(title string) (Candidate, error) {
	if strings.TrimSpace(strings.TrimPrefix(title, "=")) == "" {
		return Candidate{}, fmt.Errorf("no window title to look for")
	}
	netName, err := h.atom("_NET_WM_NAME")
	if err != nil {
		return Candidate{}, err
	}

	var cands []Candidate
	order := 0
	var walk func(win xproto.Window)
	walk = func(win xproto.Window) {
		tree, err := xproto.QueryTree(h.conn, win).Reply()
		if err != nil {
			return // a window can vanish mid-walk; that is not our error
		}
		for _, child := range tree.Children {
			order++
			if name := h.windowName(child, netName); name != "" {
				if r, ok := h.viewable(child); ok {
					cands = append(cands, Candidate{Name: name, Rect: r, Order: order, Win: uint32(child)})
				}
			}
			walk(child)
		}
	}
	walk(h.root)

	got, ok := Choose(cands, title)
	if !ok {
		return Candidate{}, fmt.Errorf("no window on screen matches %q", title)
	}
	return got, nil
}

// viewable reports a window's rect if it is actually on screen and big enough to
// be the thing the caller meant.
func (h *Hand) viewable(win xproto.Window) (Rect, bool) {
	attr, err := xproto.GetWindowAttributes(h.conn, win).Reply()
	if err != nil || attr.MapState != xproto.MapStateViewable {
		return Rect{}, false
	}
	geo, err := xproto.GetGeometry(h.conn, xproto.Drawable(win)).Reply()
	if err != nil || geo.Width < 16 || geo.Height < 16 {
		return Rect{}, false
	}
	// Root coordinates, from the server. This is the HiDPI trap: anything that
	// adds up parent offsets by hand, or reads wmctrl, gets this wrong.
	t, err := xproto.TranslateCoordinates(h.conn, win, h.root, 0, 0).Reply()
	if err != nil {
		return Rect{}, false
	}
	return Rect{X: int(t.DstX), Y: int(t.DstY), W: int(geo.Width), H: int(geo.Height)}, true
}

func (h *Hand) windowName(win xproto.Window, netName xproto.Atom) string {
	if netName != 0 {
		if s := h.textProp(win, netName); s != "" {
			return s
		}
	}
	return h.textProp(win, xproto.AtomWmName)
}

func (h *Hand) textProp(win xproto.Window, prop xproto.Atom) string {
	r, err := xproto.GetProperty(h.conn, false, win, prop, xproto.GetPropertyTypeAny, 0, 256).Reply()
	if err != nil || r == nil || len(r.Value) == 0 {
		return ""
	}
	return string(r.Value)
}

func (h *Hand) atom(name string) (xproto.Atom, error) {
	r, err := xproto.InternAtom(h.conn, true, uint16(len(name)), name).Reply()
	if err != nil || r == nil {
		return 0, fmt.Errorf("cannot intern the %s atom: %w", name, err)
	}
	return r.Atom, nil
}

// send is one synthetic event. Checked, because an XTEST error that is only
// discovered later shows up as "the click did nothing".
func (h *Hand) send(typ, detail byte, x, y int) error {
	return xtest.FakeInputChecked(h.conn, typ, detail, 0, h.root, int16(x), int16(y), 0).Check()
}

// MoveTo glides the pointer to a root coordinate and lets it settle.
func (h *Hand) MoveTo(to Pt) error {
	from, err := h.Pointer()
	if err != nil {
		return err
	}
	to = h.clampToScreen(to)
	for _, p := range PlanMove(from, to, Motion{Speed: h.speed, Rand: h.rnd}) {
		time.Sleep(p.After)
		if err := h.send(xproto.MotionNotify, 0, p.X, p.Y); err != nil {
			return fmt.Errorf("moving the pointer: %w", err)
		}
	}
	time.Sleep(h.settle)
	return nil
}

// clampToScreen keeps a target on the display. A coordinate off the edge is
// almost always a mistake in the caller's arithmetic, and X would silently clip
// it in a way that is harder to see than a click landing at the edge.
func (h *Hand) clampToScreen(p Pt) Pt {
	if p.X < 0 {
		p.X = 0
	}
	if p.Y < 0 {
		p.Y = 0
	}
	if h.screen.W > 0 && p.X > h.screen.W-1 {
		p.X = h.screen.W - 1
	}
	if h.screen.H > 0 && p.Y > h.screen.H-1 {
		p.Y = h.screen.H - 1
	}
	return p
}

// Click presses and releases a button where the pointer is. The gap between the
// two is the length of a real click; zero would be a press and release in the
// same millisecond, which some toolkits drop.
func (h *Hand) Click(button byte) error {
	if err := h.send(xproto.ButtonPress, button, 0, 0); err != nil {
		return fmt.Errorf("pressing button %d: %w", button, err)
	}
	time.Sleep(time.Duration(40+h.rnd.Intn(45)) * time.Millisecond)
	if err := h.send(xproto.ButtonRelease, button, 0, 0); err != nil {
		return fmt.Errorf("releasing button %d: %w", button, err)
	}
	return nil
}

// DoubleClick is two clicks close enough together to count as one gesture.
func (h *Hand) DoubleClick(button byte) error {
	if err := h.Click(button); err != nil {
		return err
	}
	time.Sleep(90 * time.Millisecond)
	return h.Click(button)
}

// Drag presses at the pointer, glides to a second point and releases. The pauses
// on either side of the movement are what make a drag land: a toolkit that sees
// press and motion in the same instant often treats it as a click.
func (h *Hand) Drag(to Pt) error {
	if err := h.send(xproto.ButtonPress, 1, 0, 0); err != nil {
		return fmt.Errorf("starting the drag: %w", err)
	}
	time.Sleep(120 * time.Millisecond)
	if err := h.MoveTo(to); err != nil {
		_ = h.send(xproto.ButtonRelease, 1, 0, 0) // never leave a button held
		return err
	}
	time.Sleep(120 * time.Millisecond)
	if err := h.send(xproto.ButtonRelease, 1, 0, 0); err != nil {
		return fmt.Errorf("ending the drag: %w", err)
	}
	return nil
}

// Scroll turns the wheel: positive notches scroll down, negative up.
func (h *Hand) Scroll(notches int) error {
	button := byte(5) // wheel down
	if notches < 0 {
		button, notches = 4, -notches
	}
	for range notches {
		if err := h.Click(button); err != nil {
			return err
		}
		time.Sleep(time.Duration(60+h.rnd.Intn(50)) * time.Millisecond)
	}
	return nil
}

// Type types text at the current keyboard focus, on the layout in use. It
// reports the characters the layout cannot produce rather than dropping them
// quietly, because a missing character in a typed command is a different command.
func (h *Hand) Type(text string) error {
	// The strokes below were planned against the first group's keysyms, and the
	// server resolves them in the active group. Each press re-locks the planned
	// group first, or a second layout rewrites the text (see xkb.go).
	guard := h.guardGroup()
	defer guard.release()

	strokes, skipped := PlanText(text, h.layout, Typing{WPM: h.wpm, Rand: h.rnd})
	shift, shiftOK := h.modCode(uint16(xproto.ModMaskShift))
	held := false
	release := func() {
		if held && shiftOK {
			_ = h.send(xproto.KeyRelease, shift, 0, 0)
			held = false
		}
	}
	defer release()

	for _, s := range strokes {
		time.Sleep(s.After)
		if s.Shift && !held && shiftOK {
			guard.hold()
			if err := h.send(xproto.KeyPress, shift, 0, 0); err != nil {
				return fmt.Errorf("holding shift: %w", err)
			}
			held = true
		} else if !s.Shift && held {
			release()
		}
		guard.hold()
		if err := h.send(xproto.KeyPress, s.Code, 0, 0); err != nil {
			return fmt.Errorf("typing %q: %w", s.Rune, err)
		}
		time.Sleep(time.Duration(18+h.rnd.Intn(22)) * time.Millisecond)
		if err := h.send(xproto.KeyRelease, s.Code, 0, 0); err != nil {
			return fmt.Errorf("releasing %q: %w", s.Rune, err)
		}
	}
	if len(skipped) > 0 {
		return fmt.Errorf("this keyboard layout cannot type %q", string(skipped))
	}
	return nil
}

// Press taps one combination, spelled the way agentbox spells hotkeys everywhere
// else ("ctrl+alt+t", "Escape", "shift+Tab"). Modifiers go down in order and come
// back up in reverse, so nothing is left held if a step in the middle fails.
func (h *Hand) Press(spec string) error {
	mods, ks, err := hotkey.Parse(spec)
	if err != nil {
		return err
	}
	code, needShift, ok := h.layout.Keysym(uint32(ks))
	if !ok {
		return fmt.Errorf("%q is not on the current keyboard layout", spec)
	}
	if needShift {
		mods |= uint16(xproto.ModMaskShift)
	}
	// Same group rule as Type: the keycode was planned in the first group.
	guard := h.guardGroup()
	defer guard.release()

	var down []byte
	for _, m := range modKeysyms {
		if mods&m.mask == 0 {
			continue
		}
		mc, _, ok := h.layout.Keysym(m.keysym)
		if !ok {
			continue
		}
		guard.hold()
		if err := h.send(xproto.KeyPress, mc, 0, 0); err != nil {
			h.releaseAll(down)
			return fmt.Errorf("holding a modifier for %q: %w", spec, err)
		}
		down = append(down, mc)
		time.Sleep(25 * time.Millisecond)
	}
	guard.hold()
	err = h.send(xproto.KeyPress, code, 0, 0)
	if err == nil {
		time.Sleep(time.Duration(45+h.rnd.Intn(35)) * time.Millisecond)
		err = h.send(xproto.KeyRelease, code, 0, 0)
	}
	h.releaseAll(down)
	if err != nil {
		return fmt.Errorf("pressing %q: %w", spec, err)
	}
	return nil
}

func (h *Hand) releaseAll(codes []byte) {
	for i := len(codes) - 1; i >= 0; i-- {
		_ = h.send(xproto.KeyRelease, codes[i], 0, 0)
		time.Sleep(15 * time.Millisecond)
	}
}

func (h *Hand) modCode(mask uint16) (byte, bool) {
	for _, m := range modKeysyms {
		if m.mask == mask {
			code, _, ok := h.layout.Keysym(m.keysym)
			return code, ok
		}
	}
	return 0, false
}

// Run executes a parsed script. Errors name the line, because a script that
// stops in the middle needs to say where.
func (h *Hand) Run(steps []Step) error {
	for _, st := range steps {
		if err := h.step(st); err != nil {
			return fmt.Errorf("line %d (%s): %w", st.Line, st.Raw, err)
		}
	}
	return nil
}

func (h *Hand) step(st Step) error {
	if st.To {
		// Where the locked window is NOW, not where it was when it was named:
		// a window that moved between two steps must be followed, and one that
		// closed must stop the script rather than let the click through to
		// whatever is behind it.
		if err := h.follow(); err != nil {
			return err
		}
		at, err := h.Pointer()
		if err != nil {
			return err
		}
		target := Pt{
			X: st.X.Resolve(h.frame.X, h.frame.W, at.X),
			Y: st.Y.Resolve(h.frame.Y, h.frame.H, at.Y),
		}
		h.trace("%s -> %d,%d", st.Op, target.X, target.Y)
		if st.Op == OpDrag {
			if err := h.MoveTo(target); err != nil {
				return err
			}
			if err := h.aimedAt("drag"); err != nil {
				return err
			}
			end := Pt{
				X: st.X2.Resolve(h.frame.X, h.frame.W, target.X),
				Y: st.Y2.Resolve(h.frame.Y, h.frame.H, target.Y),
			}
			return h.Drag(end)
		}
		if err := h.MoveTo(target); err != nil {
			return err
		}
	}

	switch st.Op {
	case OpWindow:
		r, err := h.UseWindow(st.Text)
		if err != nil {
			return err
		}
		// Both names, because they are rarely the same word: the script asks
		// for "agentbox" and gets "agentbox · review board · ...", and a script that
		// grabbed the wrong window is obvious in the log the moment it says so.
		h.trace("window %q -> %q at %s, raised", st.Text, h.inWin, r)
	case OpScreen:
		h.UseScreen()
	case OpMove:
		// the movement was the step, and moving the pointer changes nothing
	case OpClick:
		if err := h.aimedAt("click"); err != nil {
			return err
		}
		return h.Click(st.Button)
	case OpDouble:
		if err := h.aimedAt("double-click"); err != nil {
			return err
		}
		return h.DoubleClick(st.Button)
	case OpDrag:
		// handled above, where both ends are in scope
	case OpScroll:
		if err := h.aimedAt("scroll"); err != nil {
			return err
		}
		h.trace("scroll %d", st.N)
		return h.Scroll(st.N)
	case OpType:
		if err := h.focusedOn("type"); err != nil {
			return err
		}
		h.trace("type %q", st.Text)
		return h.Type(st.Text)
	case OpKey:
		if err := h.focusedOn("press keys"); err != nil {
			return err
		}
		for _, k := range st.Keys {
			h.trace("key %s", k)
			if err := h.Press(k); err != nil {
				return err
			}
			time.Sleep(80 * time.Millisecond)
		}
	case OpWait:
		time.Sleep(time.Duration(st.N) * time.Millisecond)
	case OpSpeed:
		h.speed = st.F
	case OpWPM:
		h.wpm = st.N
	default:
		return fmt.Errorf("unknown step %q", st.Op)
	}
	return nil
}
