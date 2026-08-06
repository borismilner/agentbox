//go:build linux

package webui

/*
#cgo pkg-config: gtk4 gtk4-x11
#include <gtk/gtk.h>
#include <gdk/x11/gdkx.h>

// xidFor realizes the toplevel without presenting it, then digs the X11
// window id out of its GdkSurface. Realizing early is the whole trick: it
// gives us a real X window to set _NET_WM properties on BEFORE the window
// maps, so the WM sees "do not focus this" at map time rather than after.
static unsigned long xidFor(void *win) {
    GtkWidget *w = GTK_WIDGET(win);
    if (!gtk_widget_get_realized(w)) {
        gtk_widget_realize(w);
    }
    GdkSurface *s = gtk_native_get_surface(GTK_NATIVE(w));
    if (!s) return 0;
    if (!GDK_IS_X11_SURFACE(s)) return 0;   // Wayland session: caller degrades
    return (unsigned long)gdk_x11_surface_get_xid(s);
}

// showNoActivate maps the window WITHOUT gtk_window_present(). present() is
// what Wails' Show() calls, and it stamps _NET_WM_USER_TIME with "now" plus
// an activation token, which is precisely the request to steal the keyboard.
// Zeroing the surface's user time and flipping visibility directly maps the
// window with no activation request attached.
static void showNoActivate(void *win) {
    GtkWidget *w = GTK_WIDGET(win);
    GdkSurface *s = gtk_native_get_surface(GTK_NATIVE(w));
    if (s && GDK_IS_X11_SURFACE(s)) {
        gdk_x11_surface_set_user_time(s, 0);
    }
    gtk_widget_set_visible(w, TRUE);
}
*/
import "C"

import (
	"encoding/binary"
	"unsafe"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/xproto"
)

// xidOf returns the X11 window id backing a Wails window's GtkWindow, or 0
// when there is no X11 surface (Wayland).
func xidOf(native unsafe.Pointer) xproto.Window {
	if native == nil {
		return 0
	}
	return xproto.Window(C.xidFor(native))
}

// showNoActivate maps a Wails window without asking the WM for focus. Must
// run on the GTK thread.
func showNoActivate(native unsafe.Pointer) {
	if native != nil {
		C.showNoActivate(native)
	}
}

type x11 struct {
	c     *xgb.Conn
	root  xproto.Window
	atoms map[string]xproto.Atom

	// rootRect is the whole X root - every monitor's bounding box. It is the
	// fallback for a display RandR cannot describe, and on a single-head machine
	// it is also the right answer. It is NOT a screen; see monitor.go.
	rootRect mon
	// haveRandr gates every randr call: xgb PANICS on a request against an
	// extension it never initialised, so this is not an optimisation.
	haveRandr bool
}

var atomNames = []string{
	"_MOTIF_WM_HINTS",
	"_NET_WM_WINDOW_TYPE", "_NET_WM_WINDOW_TYPE_NOTIFICATION", "_NET_WM_WINDOW_TYPE_UTILITY",
	"_NET_WM_STATE", "_NET_WM_STATE_ABOVE", "_NET_WM_STATE_SKIP_TASKBAR", "_NET_WM_STATE_SKIP_PAGER",
	// Read, not written: the hands-off strip has to know when the focused window
	// is fullscreen so it can step out of the picture and leave the marker
	// (FR74). Same signal FR29's presence gate reads, from agentbox's own connection.
	"_NET_WM_STATE_FULLSCREEN",
	// Interned because stateMsg asks for it by name: it was missing, so the call
	// that drops Mutter's "demands attention" flag was sending atom 0 and doing
	// nothing at all. Every card since has carried the flag it tried to clear.
	"_NET_WM_STATE_DEMANDS_ATTENTION",
	"_NET_WM_USER_TIME", "_NET_ACTIVE_WINDOW", "_NET_RESTACK_WINDOW",
	"_NET_WM_NAME", "UTF8_STRING",
}

func dialX11() *x11 {
	c, err := xgb.NewConn()
	if err != nil {
		return nil
	}
	scr := xproto.Setup(c).DefaultScreen(c)
	x := &x11{c: c, root: scr.Root, atoms: map[string]xproto.Atom{},
		rootRect: mon{w: int(scr.WidthInPixels), h: int(scr.HeightInPixels), primary: true}}
	for _, n := range atomNames {
		r, err := xproto.InternAtom(c, false, uint16(len(n)), n).Reply()
		if err != nil {
			c.Close()
			return nil
		}
		x.atoms[n] = r.Atom
	}
	x.haveRandr = initRandr(c)
	return x
}

// initRandr negotiates RandR 1.5, which is the version that reports monitors
// (1.4 and earlier only know about outputs and CRTCs, and a mirrored pair of
// outputs is one monitor). A failure here is not fatal: agentbox falls back to the
// root, which is what it always used.
func initRandr(c *xgb.Conn) bool {
	if err := randr.Init(c); err != nil {
		return false
	}
	v, err := randr.QueryVersion(c, 1, 5).Reply()
	if err != nil || v == nil {
		return false
	}
	return v.MajorVersion > 1 || (v.MajorVersion == 1 && v.MinorVersion >= 5)
}

// monitors asks the server for the current layout. Deliberately asked every time
// rather than cached at startup: a monitor plugged in, unplugged or rotated while
// the daemon runs is the normal case for a laptop, and one round trip per card is
// nothing next to painting it.
func (x *x11) monitors() []mon {
	if !x.haveRandr {
		return nil
	}
	r, err := randr.GetMonitors(x.c, x.root, true).Reply()
	if err != nil || r == nil {
		return nil
	}
	out := make([]mon, 0, len(r.Monitors))
	for _, m := range r.Monitors {
		out = append(out, mon{
			x: int(m.X), y: int(m.Y), w: int(m.Width), h: int(m.Height),
			primary: m.Primary,
		})
	}
	return out
}

// pointer is the pointer's position in root coordinates.
func (x *x11) pointer() (int, int, bool) {
	r, err := xproto.QueryPointer(x.c, x.root).Reply()
	if err != nil || r == nil {
		return 0, 0, false
	}
	return int(r.RootX), int(r.RootY), true
}

// activeMon is the monitor agentbox should put a window on right now: see pickMon
// for why that is the one under the pointer.
//
// It is resolved per placement rather than remembered per window, so a queued
// card follows you to the other screen and no stale rectangle can outlive a
// window (X reuses window ids, which makes a per-window cache a hazard rather
// than an optimisation). The one caller that must NOT re-resolve is the panel's
// slide: it fixes the monitor once, before the first frame.
func (x *x11) activeMon() mon {
	px, py, ok := x.pointer()
	return pickMon(x.monitors(), px, py, ok, x.rootRect)
}

func card32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// activeWindow reads _NET_ACTIVE_WINDOW so we can hand focus back if the WM
// takes it anyway.
func (x *x11) activeWindow() xproto.Window {
	r, err := xproto.GetProperty(x.c, false, x.root, x.atoms["_NET_ACTIVE_WINDOW"],
		xproto.AtomWindow, 0, 1).Reply()
	if err != nil || len(r.Value) < 4 {
		return 0
	}
	return xproto.Window(binary.LittleEndian.Uint32(r.Value))
}

// prepare is agentbox's existing card treatment, aimed at a GTK window instead
// of a Gio one: no decorations, notification/utility type, above everything,
// out of the taskbar, and _NET_WM_USER_TIME=0 so Mutter maps it without
// giving it the keyboard.
//
// The type used to be _NET_WM_WINDOW_TYPE_DIALOG, and that is why agentbox had an
// icon in the dock. Mutter REFUSES skip-taskbar for a dialog - reasonably: a
// dialog is a window you must be able to get back to - and it refuses it whether
// the request arrives as a pre-map property or as a client message. Measured on
// this desktop, on one live window, by changing only its type: as a dialog the
// published state stays "ABOVE"; as utility, dock, splash or notification it
// becomes "SKIP_PAGER, SKIP_TASKBAR, ABOVE". Utility is the one that keeps the
// rest of what a card needs - it can still take the keyboard when you summon it,
// which notification and dock cannot - so a card and the panel are utility
// windows now, and only a toast is a notification.
func (x *x11) prepare(win xproto.Window, notification bool) {
	motif := make([]byte, 20)
	binary.LittleEndian.PutUint32(motif, 2) // flags = MWM_HINTS_DECORATIONS, decorations = 0
	xproto.ChangeProperty(x.c, xproto.PropModeReplace, win,
		x.atoms["_MOTIF_WM_HINTS"], x.atoms["_MOTIF_WM_HINTS"], 32, 5, motif)

	typ := "_NET_WM_WINDOW_TYPE_UTILITY"
	if notification {
		typ = "_NET_WM_WINDOW_TYPE_NOTIFICATION"
	}
	xproto.ChangeProperty(x.c, xproto.PropModeReplace, win,
		x.atoms["_NET_WM_WINDOW_TYPE"], xproto.AtomAtom, 32, 1, card32(uint32(x.atoms[typ])))

	state := append(append([]byte{},
		card32(uint32(x.atoms["_NET_WM_STATE_ABOVE"]))...),
		append(card32(uint32(x.atoms["_NET_WM_STATE_SKIP_TASKBAR"])),
			card32(uint32(x.atoms["_NET_WM_STATE_SKIP_PAGER"]))...)...)
	xproto.ChangeProperty(x.c, xproto.PropModeReplace, win,
		x.atoms["_NET_WM_STATE"], xproto.AtomAtom, 32, 3, state)

	xproto.ChangeProperty(x.c, xproto.PropModeReplace, win,
		x.atoms["_NET_WM_USER_TIME"], xproto.AtomCardinal, 32, 1, card32(0))

	xproto.GetInputFocus(x.c).Reply() // flush
}

// plain is prepare with exactly the two things that make a window impossible to
// cover taken out: no notification type and no ABOVE. Everything that is not
// about stacking stays - undecorated, out of the taskbar and the switcher, and
// never given the keyboard.
//
// It exists for FR95's demoted marker, where being coverable IS the feature:
// Boris asked for the sign to step out of a screen recording, and "generally it
// should live on top of any and all surfaces; when demoted for purposes of
// recording or stuff like that it can be overlapped". Session 49 proved a
// notification-type window cannot be pushed under a fullscreen one however hard
// you lower it, so giving up the type is the only route there.
//
// The undecorated half is not cosmetic either. Mutter frames a bare window, and a
// 4px window then comes back about 30px tall wearing a title bar - measured, while
// answering this feature's own question with a probe that had forgotten to say it.
func (x *x11) plain(win xproto.Window) {
	motif := make([]byte, 20)
	binary.LittleEndian.PutUint32(motif, 2) // flags = MWM_HINTS_DECORATIONS, decorations = 0
	xproto.ChangeProperty(x.c, xproto.PropModeReplace, win,
		x.atoms["_MOTIF_WM_HINTS"], x.atoms["_MOTIF_WM_HINTS"], 32, 5, motif)

	// No _NET_WM_WINDOW_TYPE at all: a normal window is the most coverable thing
	// this stack has, and utility already carries stacking hints of its own.
	state := append(card32(uint32(x.atoms["_NET_WM_STATE_SKIP_TASKBAR"])),
		card32(uint32(x.atoms["_NET_WM_STATE_SKIP_PAGER"]))...)
	xproto.ChangeProperty(x.c, xproto.PropModeReplace, win,
		x.atoms["_NET_WM_STATE"], xproto.AtomAtom, 32, 2, state)

	xproto.ChangeProperty(x.c, xproto.PropModeReplace, win,
		x.atoms["_NET_WM_USER_TIME"], xproto.AtomCardinal, 32, 1, card32(0))

	xproto.GetInputFocus(x.c).Reply() // flush
}

// stateMsg changes a mapped window's _NET_WM_STATE. Once a window is mapped
// the WM owns that property, so writing it directly is ignored - Mutter had
// already replaced our pre-map ABOVE with DEMANDS_ATTENTION by the time the
// card was on screen. Client messages are the supported route.
func (x *x11) stateMsg(win xproto.Window, action uint32, names ...string) {
	var a1, a2 uint32
	if len(names) > 0 {
		a1 = uint32(x.atoms[names[0]])
	}
	if len(names) > 1 {
		a2 = uint32(x.atoms[names[1]])
	}
	ev := xproto.ClientMessageEvent{
		Format: 32, Window: win, Type: x.atoms["_NET_WM_STATE"],
		Data: xproto.ClientMessageDataUnionData32New([]uint32{action, a1, a2, 2, 0}),
	}
	xproto.SendEvent(x.c, false, x.root,
		uint32(xproto.EventMaskSubstructureNotify|xproto.EventMaskSubstructureRedirect),
		string(ev.Bytes()))
}

// settle is everything that has to happen after the map: claim the top of the
// stack, stay out of the taskbar, drop the "demands attention" flag Mutter
// adds to any window that declined focus, and place the window where we want it
// rather than where the WM put it - a card dead center, a toast at the top.
func (x *x11) settle(win xproto.Window, w, h int, top bool, topInset int) {
	x.stateMsg(win, 1, "_NET_WM_STATE_ABOVE")
	x.stateMsg(win, 1, "_NET_WM_STATE_SKIP_TASKBAR", "_NET_WM_STATE_SKIP_PAGER")
	x.stateMsg(win, 0, "_NET_WM_STATE_DEMANDS_ATTENTION")
	x.raise(win)
	x.place(win, w, h, top, topInset)
}

// above is settle's stacking half on its own: stay on top, never take focus, and
// keep the taskbar entry. Progress is the surface that needs exactly this - it is
// an ordinary window a person can click, minimise and close (progress.go), but a
// report that a fullscreen app is covering is a report nobody reads. It was
// covered: the showcase recorded seventeen minutes with the deck fullscreen and
// the progress bar ran the whole time underneath it, on camera and invisible.
// Boris's rule, 2026-07-26: every surface agentbox puts up must be top-most,
// otherwise it will be missed.
func (x *x11) above(win xproto.Window) {
	x.stateMsg(win, 1, "_NET_WM_STATE_ABOVE")
	x.stateMsg(win, 0, "_NET_WM_STATE_DEMANDS_ATTENTION")
	x.raise(win)
	xproto.GetInputFocus(x.c).Reply() // flush
}

// raise puts a mapped window at the top of its stacking layer without touching
// focus, and it is not redundant with ABOVE above.
//
// Mutter promotes a FOCUSED FULLSCREEN window into the same layer that
// always-on-top windows live in - so the top bar does not cover a full-screen
// video - and inside one layer the focused window is on top. A card declines
// focus by design (vision principle 3: agentbox never takes your keyboard), so
// against a fullscreen presentation, video or game it was mapped *underneath*
// and the human saw nothing. That is the one case where "a card over whatever you
// are doing" is most obviously the whole promise, and it was the case that failed.
//
// _NET_RESTACK_WINDOW is the supported route: it restacks without activating, so
// agentbox can be in front and still not steal the keyboard. The ConfigureWindow
// afterwards is the belt: a WM that ignores the pager message usually honours a
// plain stack-mode request, and neither costs anything when the other worked.
func (x *x11) raise(win xproto.Window) {
	ev := xproto.ClientMessageEvent{
		Format: 32, Window: win, Type: x.atoms["_NET_RESTACK_WINDOW"],
		// source 2 = pager, sibling None, detail Above
		Data: xproto.ClientMessageDataUnionData32New([]uint32{2, 0, uint32(xproto.StackModeAbove), 0, 0}),
	}
	xproto.SendEvent(x.c, false, x.root,
		uint32(xproto.EventMaskSubstructureNotify|xproto.EventMaskSubstructureRedirect),
		string(ev.Bytes()))
	xproto.ConfigureWindow(x.c, win, xproto.ConfigWindowStackMode,
		[]uint32{uint32(xproto.StackModeAbove)})
	xproto.GetInputFocus(x.c).Reply() // flush
}

// lower is raise's opposite, and it exists for exactly one caller: the hands-off
// strip stepping out of a fullscreen window's way (FR74). Everything else agentbox puts
// on screen wants to be on top; this one thing has to be able to stop wanting it,
// because a 620px strip pinned over the top of somebody's film is a worse answer
// than a 4px line that says the same thing.
//
// The window stays MAPPED. It is not withdrawn and not hidden: the moment the
// fullscreen window loses the focus the keeper raises it again, and a window that
// was only restacked comes back in one round trip with nothing to rebuild.
func (x *x11) lower(win xproto.Window) {
	ev := xproto.ClientMessageEvent{
		Format: 32, Window: win, Type: x.atoms["_NET_RESTACK_WINDOW"],
		// source 2 = pager, sibling None, detail Below
		Data: xproto.ClientMessageDataUnionData32New([]uint32{2, 0, uint32(xproto.StackModeBelow), 0, 0}),
	}
	xproto.SendEvent(x.c, false, x.root,
		uint32(xproto.EventMaskSubstructureNotify|xproto.EventMaskSubstructureRedirect),
		string(ev.Bytes()))
	xproto.ConfigureWindow(x.c, win, xproto.ConfigWindowStackMode,
		[]uint32{uint32(xproto.StackModeBelow)})
	xproto.GetInputFocus(x.c).Reply() // flush
}

// fullscreenActive reports whether the FOCUSED window is fullscreen, and which
// window it is. Same signal FR29's presence gate reads (_NET_ACTIVE_WINDOW, then
// that window's _NET_WM_STATE), read here from agentbox's own connection because the
// strip needs it on its own tick rather than on the daemon's five-second poll.
//
// Focused is the right test and not a shortcut: Mutter only promotes a fullscreen
// window above the always-on-top layer while it HAS the focus, so an unfocused
// fullscreen window is not covering anything and there is nothing to step aside
// from. Any error, and a display that cannot answer, reads as not fullscreen -
// the direction that keeps the strip visible.
func (x *x11) fullscreenActive() (xproto.Window, bool) {
	win := x.activeWindow()
	if win == 0 {
		return 0, false
	}
	r, err := xproto.GetProperty(x.c, false, win, x.atoms["_NET_WM_STATE"],
		xproto.GetPropertyTypeAny, 0, 64).Reply()
	if err != nil || r == nil || r.Format != 32 {
		return 0, false
	}
	fs := x.atoms["_NET_WM_STATE_FULLSCREEN"]
	for i := 0; i+4 <= len(r.Value); i += 4 {
		if xproto.Atom(binary.LittleEndian.Uint32(r.Value[i:])) == fs {
			return win, true
		}
	}
	return 0, false
}

// windowMon is the monitor a given window is on, by its centre. Used for a window
// agentbox does not own (the fullscreen one) and for one it does but did not place this
// tick, where the pointer - activeMon's answer - is the wrong question: during a
// drive run the pointer is wherever the agent last moved it.
//
// A window whose geometry cannot be read falls back to activeMon, which is what
// every other placement uses.
func (x *x11) windowMon(win xproto.Window) mon {
	g, gerr := xproto.GetGeometry(x.c, xproto.Drawable(win)).Reply()
	t, terr := xproto.TranslateCoordinates(x.c, win, x.root, 0, 0).Reply()
	if gerr != nil || terr != nil || g == nil || t == nil {
		return x.activeMon()
	}
	cx := int(t.DstX) + int(g.Width)/2
	cy := int(t.DstY) + int(g.Height)/2
	return pickMon(x.monitors(), cx, cy, true, x.rootRect)
}

// place centers a window on the monitor the person is at, or pins it to that
// monitor's top-center inset when top is set (the inset is configuration:
// [window] toast_top_inset). Both axes are ours: Mutter's own placement runs at
// map time and puts a frameless override-ish window wherever it likes - and what
// it likes, on two monitors, is not where you are looking.
func (x *x11) place(win xproto.Window, w, h int, top bool, topInset int) {
	m := x.activeMon()
	px, py := centreIn(m, w, h)
	if top {
		px, py = topCentreIn(m, w, topInset)
	}
	xproto.ConfigureWindow(x.c, win, xproto.ConfigWindowX|xproto.ConfigWindowY,
		[]uint32{uint32(px), uint32(py)})
	xproto.GetInputFocus(x.c).Reply()
}

// corner pins a window to the bottom-right of that monitor, inset from its edges.
// This is where progress goes (FR21): a long task reports where it cannot cover
// the middle of the screen, which belongs to whatever is asking a question.
func (x *x11) corner(win xproto.Window, w, h int) {
	const inset = 28
	px, py := cornerIn(x.activeMon(), w, h, inset, inset+24)
	xproto.ConfigureWindow(x.c, win, xproto.ConfigWindowX|xproto.ConfigWindowY,
		[]uint32{uint32(px), uint32(py)})
	xproto.GetInputFocus(x.c).Reply()
}

// moveResize puts a window at an exact position AND size in one request, which is
// the only thing that actually resizes the drop-down panel.
//
// Wails' SetSize is gtk_window_set_default_size, and a default size is what a
// window opens at - it does not resize one that is already mapped. So the panel
// mapped at its one-pixel opening height and stayed there, with the state machine
// certain it was open: a hotkey that did nothing. Asking the WM directly is the
// same route the position already takes (a managed window resizes on
// ConfigureWindow, which is how wmctrl -e does it), and GTK follows the
// ConfigureNotify that comes back.
func (x *x11) moveResize(win xproto.Window, px, py, w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	xproto.ConfigureWindow(x.c, win,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
		[]uint32{uint32(int32(px)), uint32(int32(py)), uint32(w), uint32(h)})
}

// unlisted keeps a window out of the taskbar and the pager, and on top, AFTER the
// map. It is sent more than once on purpose: the properties prepare() writes
// before the map are Mutter's to replace at map time, and a client message that
// arrives while the window is still being mapped is dropped - which is how the
// panel ended up with a dock icon that Boris could see and agentbox could not
// explain. Client messages are cheap; sending them twice is cheaper than a
// wrong-looking dock.
func (x *x11) unlisted(win xproto.Window) {
	x.stateMsg(win, 1, "_NET_WM_STATE_ABOVE")
	x.unlistedPlain(win)
}

// unlistedPlain is unlisted without the ABOVE, for FR95's demoted marker. It is
// split out because the ABOVE above is a client message sent AFTER the map, which
// is the one route Mutter honours - so a window that carefully avoided asking for
// ABOVE before mapping got it anyway on the next line, and a fullscreen window
// still could not cover it. Measured on screen after the code read correctly:
// four amber pixels on top of a genuinely fullscreen window.
func (x *x11) unlistedPlain(win xproto.Window) {
	x.stateMsg(win, 1, "_NET_WM_STATE_SKIP_TASKBAR", "_NET_WM_STATE_SKIP_PAGER")
	x.stateMsg(win, 0, "_NET_WM_STATE_DEMANDS_ATTENTION")
	xproto.GetInputFocus(x.c).Reply() // flush
}

// moveTo puts a window at an exact position, negative coordinates included. The
// drop-down panel is animated by moving it - a fixed-size window sliding down
// from above the top edge - rather than by growing it: a resize reflows the
// webview on every frame and a move does not touch it at all.
func (x *x11) moveTo(win xproto.Window, px, py int) {
	xproto.ConfigureWindow(x.c, win, xproto.ConfigWindowX|xproto.ConfigWindowY,
		[]uint32{uint32(int32(px)), uint32(int32(py))})
}

// flush makes the queued requests happen now. The animation needs each frame on
// the wire before it sleeps, and a checked round trip is the cheapest way to say
// so - the same trick the placement helpers use to force the server to catch up.
func (x *x11) flush() { xproto.GetInputFocus(x.c).Reply() }

// setName titles a window. Wails skips gtk_window_set_title for frameless
// windows (linux_cgo.go setTitle), and every agentbox window is frameless, so the
// property goes on directly - otherwise the reading window would show up in the
// task switcher as "agentbox" with no hint of what it is showing.
func (x *x11) setName(win xproto.Window, name string) {
	b := []byte(name)
	xproto.ChangeProperty(x.c, xproto.PropModeReplace, win,
		x.atoms["_NET_WM_NAME"], x.atoms["UTF8_STRING"], 8, uint32(len(b)), b)
	xproto.ChangeProperty(x.c, xproto.PropModeReplace, win,
		xproto.AtomWmName, xproto.AtomString, 8, uint32(len(b)), b)
	xproto.GetInputFocus(x.c).Reply()
}

// quiet is the light version of prepare, for a window that is neither a prompt
// nor decoration-free by force: it only says "do not give me the keyboard". A
// task starting in the background must not take the focus off whatever the user
// is typing into, but it is a normal window otherwise - taskbar, stacking and
// all.
func (x *x11) quiet(win xproto.Window) {
	xproto.ChangeProperty(x.c, xproto.PropModeReplace, win,
		x.atoms["_NET_WM_USER_TIME"], xproto.AtomCardinal, 32, 1, card32(0))
	xproto.GetInputFocus(x.c).Reply()
}

// activate is the deliberate opposite: agentbox summon, which does want focus.
func (x *x11) activate(win xproto.Window) {
	ev := xproto.ClientMessageEvent{
		Format: 32, Window: win, Type: x.atoms["_NET_ACTIVE_WINDOW"],
		Data: xproto.ClientMessageDataUnionData32New([]uint32{2, 0, 0, 0, 0}),
	}
	xproto.SendEvent(x.c, false, x.root,
		uint32(xproto.EventMaskSubstructureNotify|xproto.EventMaskSubstructureRedirect),
		string(ev.Bytes()))
	xproto.GetInputFocus(x.c).Reply()
}
