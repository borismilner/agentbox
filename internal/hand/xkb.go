// The XKB group lock, hand-rolled because the xgb library has no xkb package.
//
// Why it exists: Layout is read from the keyboard's first group, but the server
// resolves an XTEST keycode in whatever group is *active*. With a second layout
// selected, the planned keycodes type that layout's characters instead - the
// release tag 2026.7.3 once reached a card as 2026ץ7ץ3, on camera.
//
// Why the lock is per-stroke rather than per-call: the desktop fights back.
// GNOME re-asserts the human's input source the moment it sees the group move
// (measured here: one read in two hundred caught our lock before the revert),
// so a lock taken once at the start of a word is gone by its second letter.
// What does hold is ordering within one connection: a lock sent immediately
// before a key press is processed back-to-back with it, and the desktop's
// re-assertion can only arrive after both. So every synthetic press is
// preceded by its own unchecked lock, and the human's group is restated once
// at the end. The race is not closed in theory - another client's request can
// land between two of ours - but in practice the revert needs a round trip it
// never wins. Note for archaeologists: writing the deprecated gsettings key
// `input-sources current` (what record.sh and perform.py used to do) moves
// nothing on GNOME 46; only this X-level lock does.
//
// Three requests, built byte by byte from XKBproto.h's wire structs:
// UseExtension (the handshake the extension refuses to talk without),
// GetState (which group is locked right now) and LatchLockState (lock one).
package hand

import (
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const (
	xkbExtName        = "XKEYBOARD"
	xkbUseCoreKbd     = 0x0100 // deviceSpec: the core keyboard
	xkbUseExtension   = 0      // minor opcodes, from XKBproto.h
	xkbGetState       = 4
	xkbLatchLockState = 5
)

// xkbInit finds the extension and performs its version handshake. On any
// refusal h.xkb stays false and typing behaves as it always did.
func (h *Hand) xkbInit() {
	r, err := xproto.QueryExtension(h.conn, uint16(len(xkbExtName)), xkbExtName).Reply()
	if err != nil || r == nil || !r.Present {
		return
	}
	h.xkbOp = r.MajorOpcode
	buf := make([]byte, 8)
	buf[0] = h.xkbOp
	buf[1] = xkbUseExtension
	xgb.Put16(buf[2:], 2) // request length, in 4-byte units
	xgb.Put16(buf[4:], 1) // the protocol version this file speaks
	xgb.Put16(buf[6:], 0)
	c := h.conn.NewCookie(true, true)
	h.conn.NewRequest(buf, c)
	rep, err := c.Reply()
	h.xkb = err == nil && len(rep) > 1 && rep[1] != 0 // byte 1: supported
}

// lockedGroup is the group the keyboard is locked to right now, 0-based.
func (h *Hand) lockedGroup() (byte, bool) {
	if !h.xkb {
		return 0, false
	}
	buf := make([]byte, 8)
	buf[0] = h.xkbOp
	buf[1] = xkbGetState
	xgb.Put16(buf[2:], 2)
	xgb.Put16(buf[4:], xkbUseCoreKbd)
	c := h.conn.NewCookie(true, true)
	h.conn.NewRequest(buf, c)
	rep, err := c.Reply()
	if err != nil || len(rep) < 14 {
		return 0, false
	}
	return rep[13], true // lockedGroup, per xkbGetStateReply
}

func (h *Hand) latchLockBuf(g byte) []byte {
	buf := make([]byte, 16)
	buf[0] = h.xkbOp
	buf[1] = xkbLatchLockState
	xgb.Put16(buf[2:], 4)
	xgb.Put16(buf[4:], xkbUseCoreKbd)
	buf[8] = 1 // lockGroup: yes
	buf[9] = g // groupLock; every other lock and latch field stays zero
	return buf
}

// lockGroup locks the keyboard to one group, exactly as the desktop's layout
// switcher would, and waits to hear whether the server took it.
func (h *Hand) lockGroup(g byte) bool {
	if !h.xkb {
		return false
	}
	c := h.conn.NewCookie(true, false)
	h.conn.NewRequest(h.latchLockBuf(g), c)
	return c.Check() == nil
}

// lockGroupUnchecked sends the same lock without waiting. This is the form the
// per-stroke guard uses: no round trip between the lock and the press it
// protects, so nothing can be processed between them.
func (h *Hand) lockGroupUnchecked(g byte) {
	if !h.xkb {
		return
	}
	h.conn.NewRequest(h.latchLockBuf(g), h.conn.NewCookie(false, false))
}

// groupGuard keeps synthetic strokes in the group they were planned against.
// Zero value: no guarding needed.
type groupGuard struct {
	h   *Hand
	was byte
}

// guardGroup decides once per Type or Press call whether guarding is needed:
// only when XKB works and the human's locked group is not the planned one.
func (h *Hand) guardGroup() groupGuard {
	was, ok := h.lockedGroup()
	if !ok || was == 0 {
		return groupGuard{}
	}
	h.trace("keyboard group %d is active; each stroke locks group 1 first", was+1)
	return groupGuard{h: h, was: was}
}

// hold re-locks the planned group. Call immediately before every synthetic
// key press, with nothing in between - see the file comment for why once is
// not enough.
func (g groupGuard) hold() {
	if g.h != nil {
		g.h.lockGroupUnchecked(0)
	}
}

// release states the human's group one last time. On a desktop that already
// re-asserted it this repeats the truth; anywhere else it undoes the guard.
func (g groupGuard) release() {
	if g.h != nil {
		g.h.lockGroup(g.was)
	}
}
