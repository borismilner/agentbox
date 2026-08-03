package webui

import "github.com/borismilner/agentbox/internal/proto"

// The toast treatment (03-ui-ux.md "Toast"): the rendering of notify items. A
// notify is the one kind nobody has to answer, so it does not get the middle of
// the screen and it does not get a card's weight - it gets a strip at the top,
// loud enough to read from across the desk and cheap enough to ignore.
//
// Which items land here, what icon a level wears and whether the strip sits
// there until clicked are all decided in this file rather than in the surface,
// for the same reason the triage keymap is: they are agentbox's vocabulary, they
// have to agree with the card and the inbox, and they are worth a table test.

const (
	toastH = 96 // opening height; the surface measures itself and calls Fit
)

// isToast reports whether an item gets the strip instead of a card. Urgent is
// the carve-out: 03-ui-ux.md sends it to a card with escalation, because a
// notice that mattered enough to be urgent must not be allowed to slide away
// unread while the user is looking at something else.
func isToast(it *proto.Item) bool {
	return it != nil && it.Kind == proto.KindNotify && it.EffectiveLevel() != proto.LevelUrgent
}

// treatment names the window an item needs. The prompt window is reused across
// items, so this is also what decides whether the open one can be handed the
// next item or has to be replaced: a toast and a card differ in size, type,
// placement and shape, none of which survive a swap of contents.
func treatment(it *proto.Item) string {
	if isToast(it) {
		return "toast"
	}
	return "card"
}

// severityGlyph names the icon a level wears (03-ui-ux.md: circled i, check,
// warning triangle, crossed circle, bell). The surface draws the shape it is
// named; it does not decide which shape a level deserves.
func severityGlyph(l proto.Level) string {
	switch l {
	case proto.LevelSuccess:
		return "check"
	case proto.LevelWarning:
		return "warning"
	case proto.LevelError:
		return "cross"
	case proto.LevelUrgent:
		return "bell"
	}
	return "info"
}

// toastHeight is the opening guess for the window, refined by Fit once the
// surface has laid itself out. A toast is a strip, so the guess is deliberately
// tight: two lines of body, not a card's worth of room.
func (u *UI) toastHeight(it *proto.Item) int {
	h := 62
	h += 19 * min(countLines(it.Body, 56), 3)
	if len(it.Actions) > 0 {
		h += 40
	}
	if _, maxH, _ := u.toastGeom(); h > maxH {
		h = maxH
	}
	return h
}
