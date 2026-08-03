// Package presence reads the desktop signals behind FR29/FR44. It is
// toolkit-independent on purpose: the daemon needs an idle signal, not a UI, so
// this lives outside internal/webui and the two never depend on each other.
package presence

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/screensaver"
	"github.com/jezek/xgb/xproto"
)

// Monitor reads the desktop presence signals behind FR29 from X11 (plus
// gsettings for the desktop's own DND). On a session without X11 or the
// screensaver extension - a Wayland-only desktop, a headless run - it reports
// not idle and no fullscreen, and without gsettings it reports no desktop DND.
// Every absence reads as "user present, interrupt freely": marking a toast
// missed-while-away or suppressing one by mistake is the harm to avoid, so the
// monitor never invents a reason to go quiet.
//
// It keeps its own connection rather than sharing the UI's window-prep side
// connection (internal/webui/x11.go), so its checked round trips never
// interleave with the unchecked property writes there.
type Monitor struct {
	mu   sync.Mutex
	conn *xgb.Conn
	root xproto.Window
	ok   bool

	// EWMH atoms for the fullscreen check; fsOk is false if they could not
	// be interned (a broken connection).
	fsOk          bool
	atomActiveWin xproto.Atom
	atomWmState   xproto.Atom
	atomFullscr   xproto.Atom

	gsettings string // gsettings path for the desktop-DND read, "" when absent
}

// New connects and initializes the screensaver extension once, and locates
// gsettings for the desktop-DND read. Any failure leaves the corresponding
// signal answering "present".
func New() *Monitor {
	m := &Monitor{}
	if p, err := exec.LookPath("gsettings"); err == nil {
		m.gsettings = p
	}
	conn, err := xgb.NewConn()
	if err != nil {
		return m
	}
	if err := screensaver.Init(conn); err != nil {
		conn.Close()
		return m
	}
	m.conn = conn
	m.root = xproto.Setup(conn).DefaultScreen(conn).Root
	m.ok = true
	m.internFullscreenAtoms()
	return m
}

// internFullscreenAtoms resolves the EWMH atoms used by FullscreenActive.
// onlyIfExists=false always returns a usable atom, so reads of a property no
// client has set yet simply come back empty rather than erroring.
func (m *Monitor) internFullscreenAtoms() {
	intern := func(name string) (xproto.Atom, bool) {
		r, err := xproto.InternAtom(m.conn, false, uint16(len(name)), name).Reply()
		if err != nil || r == nil {
			return 0, false
		}
		return r.Atom, true
	}
	a, ok1 := intern("_NET_ACTIVE_WINDOW")
	b, ok2 := intern("_NET_WM_STATE")
	c, ok3 := intern("_NET_WM_STATE_FULLSCREEN")
	m.atomActiveWin, m.atomWmState, m.atomFullscr = a, b, c
	m.fsOk = ok1 && ok2 && ok3
}

// IdleFor reports whether the user has been idle for at least d. A
// non-positive threshold, an unavailable monitor, or a query error all read
// as not idle.
func (m *Monitor) IdleFor(d time.Duration) bool {
	if d <= 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.ok {
		return false
	}
	reply, err := screensaver.QueryInfo(m.conn, xproto.Drawable(m.root)).Reply()
	if err != nil {
		return false
	}
	return time.Duration(reply.MsSinceUserInput)*time.Millisecond >= d
}

// FullscreenActive reports whether the focused window is fullscreen (a video,
// game, presentation, or screen share). It reads _NET_ACTIVE_WINDOW from the
// root, then checks the active window's _NET_WM_STATE for
// _NET_WM_STATE_FULLSCREEN (04-platform.md). Any unavailability or query error
// reads as not fullscreen. X11 only; a Wayland client's fullscreen state is
// not visible to other clients (a known M7 gap).
func (m *Monitor) FullscreenActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.ok || !m.fsOk {
		return false
	}
	r, err := xproto.GetProperty(m.conn, false, m.root, m.atomActiveWin,
		xproto.GetPropertyTypeAny, 0, 1).Reply()
	if err != nil || r == nil || r.Format != 32 || len(r.Value) < 4 {
		return false
	}
	win := xproto.Window(xgb.Get32(r.Value))
	if win == 0 {
		return false
	}
	sr, err := xproto.GetProperty(m.conn, false, win, m.atomWmState,
		xproto.GetPropertyTypeAny, 0, 64).Reply()
	if err != nil || sr == nil || sr.Format != 32 {
		return false
	}
	for i := 0; i+4 <= len(sr.Value); i += 4 {
		if xproto.Atom(xgb.Get32(sr.Value[i:])) == m.atomFullscr {
			return true
		}
	}
	return false
}

// DesktopDND reports whether the desktop's own do-not-disturb is on, read from
// GNOME's org.gnome.desktop.notifications show-banners (false = banners
// suppressed = DND). No gsettings, a non-GNOME desktop, or any error reads as
// off. Not held under the X11 mutex - it shells out and never touches the
// connection.
func (m *Monitor) DesktopDND() bool {
	if m.gsettings == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, m.gsettings, "get",
		"org.gnome.desktop.notifications", "show-banners").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "false"
}
