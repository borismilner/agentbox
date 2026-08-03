// Package hotkey grabs one desktop-wide key combination on X11 and calls back
// when it is pressed. agentbox uses it for the drop-down panel (M10): a panel you
// have to bind a shortcut for by hand is a panel you will not use, so the daemon
// takes the key itself rather than asking the desktop to run `agentbox panel`.
//
// It is X11 only. A grab needs a server that lets a client claim a key globally,
// which Wayland deliberately does not - there the CLI plus a compositor shortcut
// stays the route, and Open reports that it could not grab rather than pretending.
package hotkey

import (
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Hotkey is a live grab. Close releases it and stops the reader.
type Hotkey struct {
	conn *xgb.Conn
	root xproto.Window
	log  *slog.Logger

	spec  string
	mods  uint16
	codes []xproto.Keycode

	mu     sync.Mutex
	closed bool
}

// Modifier masks. Super is Mod4 on every desktop agentbox targets; Alt is Mod1.
const (
	modShift = uint16(xproto.ModMaskShift)
	modCtrl  = uint16(xproto.ModMaskControl)
	modAlt   = uint16(xproto.ModMask1)
	modSuper = uint16(xproto.ModMask4)

	// Lock modifiers are *active state*, not part of the combo: a grab that
	// ignores them stops working the moment Caps Lock or Num Lock is on, which
	// reads as "the hotkey randomly does nothing".
	maskCaps = uint16(xproto.ModMaskLock)
	maskNum  = uint16(xproto.ModMask2)
)

// named keys worth supporting by name; anything else is taken as a single
// character (a-z, 0-9, punctuation) and mapped to its Latin-1 keysym.
var namedKeys = map[string]xproto.Keysym{
	"grave": 0x0060, "backtick": 0x0060, "`": 0x0060,
	"space": 0x0020, "escape": 0xff1b, "esc": 0xff1b,
	"return": 0xff0d, "enter": 0xff0d, "tab": 0xff09,
	"comma": 0x002c, "period": 0x002e, "slash": 0x002f,
	"semicolon": 0x003b, "apostrophe": 0x0027,
	"bracketleft": 0x005b, "bracketright": 0x005d,
	"minus": 0x002d, "equal": 0x003d, "backslash": 0x005c,
	// Navigation and editing keys. A hotkey rarely wants these, but
	// internal/hand presses keys through the same parser, and driving a
	// window means End, Page Down and the arrows.
	"up": 0xff52, "down": 0xff54, "left": 0xff51, "right": 0xff53,
	"home": 0xff50, "end": 0xff57,
	"pageup": 0xff55, "prior": 0xff55, "pagedown": 0xff56, "next": 0xff56,
	"backspace": 0xff08, "delete": 0xffff, "del": 0xffff, "insert": 0xff63,
}

// Parse turns "Super+grave" into a modifier mask and a keysym. Order does not
// matter and case does not either, so "super+GRAVE" and "Grave+Super" are the
// same combo.
func Parse(spec string) (uint16, xproto.Keysym, error) {
	parts := strings.Split(spec, "+")
	var mods uint16
	key := ""
	for _, raw := range parts {
		p := strings.ToLower(strings.TrimSpace(raw))
		switch p {
		case "":
			continue
		case "super", "meta", "mod4", "win", "cmd":
			mods |= modSuper
		case "ctrl", "control":
			mods |= modCtrl
		case "alt", "mod1", "option":
			mods |= modAlt
		case "shift":
			mods |= modShift
		default:
			if key != "" {
				return 0, 0, fmt.Errorf("hotkey %q names two keys (%q and %q)", spec, key, p)
			}
			key = p
		}
	}
	if key == "" {
		return 0, 0, fmt.Errorf("hotkey %q names no key", spec)
	}
	if ks, ok := namedKeys[key]; ok {
		return mods, ks, nil
	}
	if len(key) > 1 && key[0] == 'f' {
		var n int
		if _, err := fmt.Sscanf(key, "f%d", &n); err == nil && n >= 1 && n <= 24 {
			return mods, xproto.Keysym(0xffbd + n), nil // XK_F1 = 0xffbe
		}
	}
	if r := []rune(key); len(r) == 1 && r[0] < 0x100 {
		return mods, xproto.Keysym(r[0]), nil
	}
	return 0, 0, fmt.Errorf("hotkey %q: unknown key %q", spec, key)
}

// Open grabs spec and runs fn on every press, until Close. A modifier-less spec
// is refused: grabbing a bare key takes it away from every application on the
// desktop, which is never what anyone meant.
func Open(spec string, log *slog.Logger, fn func()) (*Hotkey, error) {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	mods, ks, err := Parse(spec)
	if err != nil {
		return nil, err
	}
	if mods == 0 {
		return nil, fmt.Errorf("hotkey %q has no modifier; that would take the key from every app", spec)
	}

	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("hotkey: no X11 display: %w", err)
	}
	h := &Hotkey{conn: conn, root: xproto.Setup(conn).DefaultScreen(conn).Root,
		log: log, spec: spec, mods: mods}

	h.codes = keycodesFor(conn, ks)
	if len(h.codes) == 0 {
		conn.Close()
		return nil, fmt.Errorf("hotkey %q: the key is not on the current layout", spec)
	}
	if err := h.grab(); err != nil {
		conn.Close()
		return nil, err
	}

	go h.read(fn)
	return h, nil
}

// keycodesFor finds every keycode that produces this keysym on the current
// layout. There can be more than one (a numpad twin, a second layout group), and
// grabbing all of them is what makes the combo work whichever key you actually
// press.
func keycodesFor(conn *xgb.Conn, ks xproto.Keysym) []xproto.Keycode {
	setup := xproto.Setup(conn)
	first, last := setup.MinKeycode, setup.MaxKeycode
	count := byte(last - first + 1)
	m, err := xproto.GetKeyboardMapping(conn, first, count).Reply()
	if err != nil || m == nil || m.KeysymsPerKeycode == 0 {
		return nil
	}
	per := int(m.KeysymsPerKeycode)
	var out []xproto.Keycode
	for i := 0; i < int(count); i++ {
		for j := range per {
			if idx := i*per + j; idx < len(m.Keysyms) && m.Keysyms[idx] == ks {
				out = append(out, first+xproto.Keycode(i))
				break
			}
		}
	}
	return out
}

// grab claims the combo, including with the lock modifiers held, and reports the
// first failure with the reason a user can act on: BadAccess means somebody else
// already owns this combination.
func (h *Hotkey) grab() error {
	h.mu.Lock()
	codes, mods, spec := h.codes, h.mods, h.spec
	h.mu.Unlock()
	for _, code := range codes {
		for _, extra := range []uint16{0, maskCaps, maskNum, maskCaps | maskNum} {
			err := xproto.GrabKeyChecked(h.conn, true, h.root, mods|extra, code,
				xproto.GrabModeAsync, xproto.GrabModeAsync).Check()
			if err != nil {
				if strings.Contains(err.Error(), "BadAccess") {
					return fmt.Errorf("hotkey %q is already taken by another application", spec)
				}
				return fmt.Errorf("hotkey %q: %w", spec, err)
			}
		}
	}
	return nil
}

func (h *Hotkey) ungrab() {
	h.mu.Lock()
	codes, mods := h.codes, h.mods
	h.mu.Unlock()
	for _, code := range codes {
		for _, extra := range []uint16{0, maskCaps, maskNum, maskCaps | maskNum} {
			xproto.UngrabKey(h.conn, code, h.root, mods|extra)
		}
	}
	xproto.GetInputFocus(h.conn).Reply() // flush
}

// read is the event loop. A key press fires fn on its own goroutine so a slow
// callback cannot swallow the next press, and a MappingNotify (someone switched
// keyboard layout) re-resolves the keycodes so the combo survives it.
func (h *Hotkey) read(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			h.log.Error("panic", "component", "hotkey", "panic", fmt.Sprint(r))
		}
	}()
	for {
		ev, xerr := h.conn.WaitForEvent()
		h.mu.Lock()
		closed := h.closed
		h.mu.Unlock()
		if closed {
			return
		}
		if xerr != nil {
			h.log.Debug("hotkey.x11_error", "component", "hotkey", "err", xerr.Error())
			continue
		}
		switch e := ev.(type) {
		case xproto.KeyPressEvent:
			h.mu.Lock()
			codes := h.codes
			h.mu.Unlock()
			if slices.Contains(codes, e.Detail) {
				go fn()
			}
		case xproto.MappingNotifyEvent:
			h.remap()
		case nil:
			return // the connection went away
		}
	}
}

func (h *Hotkey) remap() {
	if err := h.Rebind(h.Spec()); err != nil {
		h.log.Warn("hotkey.regrab_failed", "component", "hotkey", "err", err.Error())
	}
}

// Rebind moves the grab to a new combination without dropping the connection, so
// changing the hotkey in the config (or in the settings surface) takes effect while
// you watch instead of at the next restart. On failure the old grab is left in
// place: losing the working hotkey because the new one was taken would be worse
// than refusing the change.
func (h *Hotkey) Rebind(spec string) error {
	mods, ks, err := Parse(spec)
	if err != nil {
		return err
	}
	if mods == 0 {
		return fmt.Errorf("hotkey %q has no modifier; that would take the key from every app", spec)
	}
	codes := keycodesFor(h.conn, ks)
	if len(codes) == 0 {
		return fmt.Errorf("hotkey %q: the key is not on the current layout", spec)
	}

	h.mu.Lock()
	oldSpec, oldMods, oldCodes := h.spec, h.mods, h.codes
	h.mu.Unlock()

	h.ungrab()
	h.mu.Lock()
	h.spec, h.mods, h.codes = spec, mods, codes
	h.mu.Unlock()

	if err := h.grab(); err != nil {
		// Put the old one back, so a rejected change costs nothing.
		h.mu.Lock()
		h.spec, h.mods, h.codes = oldSpec, oldMods, oldCodes
		h.mu.Unlock()
		if regrab := h.grab(); regrab != nil {
			h.log.Warn("hotkey.lost", "component", "hotkey", "err", regrab.Error())
		}
		return err
	}
	h.log.Info("hotkey.rebound", "component", "hotkey", "from", oldSpec, "to", spec)
	return nil
}

// Spec is the combination currently grabbed.
func (h *Hotkey) Spec() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.spec
}

// Close releases the grab.
func (h *Hotkey) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	h.mu.Unlock()
	h.ungrab()
	h.conn.Close()
}
