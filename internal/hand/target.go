package hand

import (
	"fmt"
	"strings"
	"time"

	"github.com/jezek/xgb/xproto"
)

// Knowing what is about to be clicked or typed into, rather than trusting that
// it is still where it was.
//
// `window TITLE` used to resolve a rectangle once and never look again. Between
// that lookup and the click, the window can move, be covered, lose focus or
// close, and every one of those sends the event somewhere else - into whatever
// happens to be there. A click lands on the wrong button; a `type` goes into
// the wrong document, which is the one that cannot be taken back.
//
// So a named window is a lock, not a coordinate: X can say which window the
// pointer is over (QueryPointer, walked down to the deepest child) and which
// one the keyboard is in (GetInputFocus), and both are compared with the lock
// before the event is sent. A mismatch raises the target and tries once more;
// if that does not fix it the step fails, naming what was actually there.
//
// Two allowances, both learned from what real toolkits do:
//
//   - A menu, a tooltip or a combo box popup is an override-redirect window of
//     its own. It is not another document, it is the target's own menu, so a
//     chain that passes through one is accepted.
//   - So is a window that declares WM_TRANSIENT_FOR the target: a modal dialog
//     belongs to the window that opened it.
//
// Without a lock (`screen`, or a script that never named a window) nothing is
// enforced - there is no target to compare against - but the trace still says
// where each event went, so the log answers the question afterwards.

// winInfo is one window on the way from a hit up to the root.
type winInfo struct {
	Win        uint32
	Name       string
	Class      string
	Override   bool   // override-redirect: a menu, a tooltip, a popup
	TransFor   uint32 // WM_TRANSIENT_FOR, when it has one
	IsRootLike bool   // the root itself, which ends every chain
}

// label is what a human would call this window.
func (w winInfo) label() string {
	switch {
	case w.Name != "" && w.Class != "":
		return fmt.Sprintf("%q (%s)", w.Name, w.Class)
	case w.Name != "":
		return fmt.Sprintf("%q", w.Name)
	case w.Class != "":
		return w.Class
	default:
		return fmt.Sprintf("an unnamed window (0x%x)", w.Win)
	}
}

// judgeChain decides whether a chain running from the window an event would hit
// up to the root belongs to target, and what to call what it found. Pure, so
// the rules can be tested without a display.
func judgeChain(chain []winInfo, target uint32) (ok bool, what string) {
	if len(chain) == 0 {
		return false, "nothing"
	}
	popup := false
	for _, w := range chain {
		if w.Win == target {
			return true, describeChain(chain, popup)
		}
		if w.Override {
			popup = true
		}
		if w.TransFor != 0 && w.TransFor == target {
			return true, describeChain(chain, popup)
		}
	}
	// A popup that belongs to nothing we can name is still a popup: menus are
	// posted as children of the root, so the chain never reaches the window
	// that opened them. Refusing those would break every menu a script drives.
	if popup {
		return true, describeChain(chain, true)
	}
	return false, describeChain(chain, false)
}

// describeChain names a chain by its first window that carries a name, which is
// the one the human would point at.
func describeChain(chain []winInfo, popup bool) string {
	for _, w := range chain {
		if w.Name != "" || w.Class != "" {
			if popup {
				return "a menu or popup of " + w.label()
			}
			return w.label()
		}
	}
	if popup {
		return "a menu or popup"
	}
	return chain[0].label()
}

// pointerChain is the window under the pointer and its ancestors. The walk goes
// down first (QueryPointer reports the child of each window that contains the
// pointer, so it must be repeated to reach the deepest) and then back up.
func (h *Hand) pointerChain() ([]winInfo, error) {
	w := h.root
	for range 32 {
		r, err := xproto.QueryPointer(h.conn, w).Reply()
		if err != nil {
			return nil, fmt.Errorf("cannot read what is under the pointer: %w", err)
		}
		if r == nil || r.Child == 0 {
			break
		}
		w = r.Child
	}
	return h.chainUp(w), nil
}

// focusChain is the window the keyboard is in and its ancestors. PointerRoot
// means the display has no focused window and typing follows the pointer, so
// that is the question to ask instead.
func (h *Hand) focusChain() ([]winInfo, error) {
	r, err := xproto.GetInputFocus(h.conn).Reply()
	if err != nil || r == nil {
		return nil, fmt.Errorf("cannot read which window has the keyboard: %w", err)
	}
	switch r.Focus {
	case xproto.WindowNone:
		return nil, nil
	case xproto.InputFocusPointerRoot:
		return h.pointerChain()
	}
	return h.chainUp(r.Focus), nil
}

// chainUp reads a window and every ancestor up to the root. Bounded, because a
// broken tree must not cost the caller its script.
func (h *Hand) chainUp(w xproto.Window) []winInfo {
	var out []winInfo
	netName, _ := h.atom("_NET_WM_NAME")
	transFor, _ := h.atom("WM_TRANSIENT_FOR")
	for range 32 {
		if w == 0 {
			break
		}
		info := winInfo{Win: uint32(w), IsRootLike: w == h.root}
		if !info.IsRootLike {
			info.Name = h.windowName(w, netName)
			info.Class = h.wmClass(w)
			if attr, err := xproto.GetWindowAttributes(h.conn, w).Reply(); err == nil && attr != nil {
				info.Override = attr.OverrideRedirect
			}
			info.TransFor = h.transientFor(w, transFor)
		}
		out = append(out, info)
		if info.IsRootLike {
			break
		}
		t, err := xproto.QueryTree(h.conn, w).Reply()
		if err != nil || t == nil {
			break
		}
		w = t.Parent
	}
	return out
}

// wmClass is the application's own name for itself, which stays put when the
// title changes. "terminator.X-terminal-emulator" says more in a failure than a
// title that is whatever file is open.
func (h *Hand) wmClass(win xproto.Window) string {
	raw := h.textProp(win, xproto.AtomWmClass)
	if raw == "" {
		return ""
	}
	parts := strings.Split(strings.TrimRight(raw, "\x00"), "\x00")
	return strings.Join(parts, ".")
}

func (h *Hand) transientFor(win xproto.Window, prop xproto.Atom) uint32 {
	if prop == 0 {
		return 0
	}
	r, err := xproto.GetProperty(h.conn, false, win, prop, xproto.GetPropertyTypeAny, 0, 1).Reply()
	if err != nil || r == nil || len(r.Value) < 4 {
		return 0
	}
	return uint32(r.Value[0]) | uint32(r.Value[1])<<8 | uint32(r.Value[2])<<16 | uint32(r.Value[3])<<24
}

// Activate asks the window manager to raise a window, deiconify it and give it
// the keyboard - what clicking its taskbar entry does. Source 2 (a pager) is
// the indication window managers honour without focus-stealing prevention;
// source 1 (an application) is the one they second-guess.
func (h *Hand) Activate(win xproto.Window) error {
	active, err := h.atom("_NET_ACTIVE_WINDOW")
	if err != nil {
		return err
	}
	ev := xproto.ClientMessageEvent{
		Format: 32,
		Window: win,
		Type:   active,
		Data:   xproto.ClientMessageDataUnionData32New([]uint32{2, 0, 0, 0, 0}),
	}
	mask := uint32(xproto.EventMaskSubstructureNotify | xproto.EventMaskSubstructureRedirect)
	if err := xproto.SendEventChecked(h.conn, false, h.root, mask, string(ev.Bytes())).Check(); err != nil {
		return fmt.Errorf("cannot ask the window manager to raise the window: %w", err)
	}
	return nil
}

// target is the window a script named, or 0. Locking one is what turns the
// checks below on.
func (h *Hand) target() (xproto.Window, bool) {
	return h.targetWin, h.targetWin != 0
}

// follow re-reads the locked window's position, so a window that moved between
// two steps is followed rather than clicked through. A window that is gone is
// an error here rather than a click into whatever took its place.
func (h *Hand) follow() error {
	win, locked := h.target()
	if !locked {
		return nil
	}
	r, ok := h.viewable(win)
	if !ok {
		return fmt.Errorf("the window %q is no longer on screen", h.inWin)
	}
	if r != h.frame {
		h.trace("window %q moved to %s", h.inWin, r)
		h.frame = r
	}
	return nil
}

// aimedAt checks that the pointer is over the locked window before a press. On
// a mismatch it raises the target once and looks again: the usual cause is
// another window on top, and raising is what a person would do about it.
func (h *Hand) aimedAt(what string) error {
	win, locked := h.target()
	chain, err := h.pointerChain()
	if err != nil {
		return err
	}
	if !locked {
		h.trace("%s into %s", what, describeChain(chain, false))
		return nil
	}
	if ok, found := judgeChain(chain, uint32(win)); ok {
		h.trace("%s into %s", what, found)
		return nil
	}

	if err := h.Activate(win); err != nil {
		return err
	}
	if err := h.settleAfterActivate(); err != nil {
		return err
	}
	if err := h.follow(); err != nil {
		return err
	}
	chain, err = h.pointerChain()
	if err != nil {
		return err
	}
	if ok, found := judgeChain(chain, uint32(win)); ok {
		h.trace("%s into %s, after raising it", what, found)
		return nil
	}
	_, found := judgeChain(chain, uint32(win))
	return fmt.Errorf("the pointer is over %s, not %q - refusing to %s something this script did not aim at", found, h.inWin, what)
}

// focusedOn checks that the keyboard is in the locked window before typing.
// This is the check that matters: a click into the wrong window is a wasted
// click, and text typed into the wrong document is an edit nobody asked for.
func (h *Hand) focusedOn(what string) error {
	win, locked := h.target()
	chain, err := h.focusChain()
	if err != nil {
		return err
	}
	if !locked {
		if len(chain) == 0 {
			h.trace("%s with nothing focused", what)
			return nil
		}
		h.trace("%s into %s", what, describeChain(chain, false))
		return nil
	}
	if ok, found := judgeChain(chain, uint32(win)); ok {
		h.trace("%s into %s", what, found)
		return nil
	}

	if err := h.Activate(win); err != nil {
		return err
	}
	if err := h.settleAfterActivate(); err != nil {
		return err
	}
	if err := h.follow(); err != nil {
		return err
	}
	chain, err = h.focusChain()
	if err != nil {
		return err
	}
	if ok, found := judgeChain(chain, uint32(win)); ok {
		h.trace("%s into %s, after raising it", what, found)
		return nil
	}
	if len(chain) == 0 {
		return fmt.Errorf("no window has the keyboard, and %q would not take it - refusing to %s into nothing", h.inWin, what)
	}
	_, found := judgeChain(chain, uint32(win))
	return fmt.Errorf("the keyboard is in %s, not %q - refusing to %s into it", found, h.inWin, what)
}

// settleAfterActivate waits for the window manager to act on the request. It is
// a round trip plus a beat: raising is asynchronous, and checking again in the
// same instant reads the state from before the ask.
func (h *Hand) settleAfterActivate() error {
	time.Sleep(220 * time.Millisecond)
	// A round trip proves the server has processed the request, rather than
	// trusting the sleep alone on a loaded machine.
	if _, err := xproto.GetInputFocus(h.conn).Reply(); err != nil {
		return fmt.Errorf("waiting for the window manager: %w", err)
	}
	return nil
}
