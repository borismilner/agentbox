//go:build !linux || nox11

package webui

import "unsafe"

// The placement layer on a desktop that has no X11: macOS, Windows, a Wayland
// session with no Xwayland, or a Linux build made with `-tags nox11`.
//
// It exists so that "no X11" is a SUPPORTED configuration rather than a build
// error. x11.go used to be the only file in the tree with a build tag, and there
// was nothing on the other side of it - so the tag read as portability while the
// build broke the moment anything but Linux asked for it.
//
// Two rules make this file safe rather than a pile of silent holes:
//
//   - dialX11 answers nil, which is the signal the surfaces already understand.
//     Every one of them tests `u.x != nil` and takes a designed degrade: the
//     window is shown by the toolkit, placed by the window manager, and stacked
//     however the desktop stacks things. That path was always written and never
//     run, which is exactly how R-12 hid a panel that reported itself open. It is
//     run now - `make check-nox11` runs the whole suite through it.
//   - The methods below are still nil-safe no-ops, as a backstop for a call site
//     that forgets its guard. A missed guard should cost placement, not the
//     daemon: an unplaced card is a card you can still answer, and a panic in the
//     UI thread is every card gone. They mirror x11.go method for method for the
//     same reason - a call added on Linux breaks the other platforms at compile
//     time, here, rather than in somebody's release.
//
// What is genuinely lost without X11 is written down in docs/04-platform.md, per
// surface, and none of it is a message going missing: pop-above-without-focus
// becomes pop-above-with-focus, the top-centre column becomes wherever the WM
// puts things, and the global hotkey and pointer driving are unavailable and say
// so (internal/hotkey, internal/hand).

// xidOf answers "no native window", which is what every caller already handles:
// they test the result against 0 before using it.
func xidOf(unsafe.Pointer) winID { return 0 }

// showNoActivate is a no-op. The toolkit's own Show is the degrade, and it takes
// focus - the one visible difference, and the reason the map-without-focus trick
// is documented as X11-only rather than as a guarantee.
func showNoActivate(unsafe.Pointer) {}

type x11 struct{}

// dialX11 reports that there is no X11 to talk to. Nil, not an empty struct: the
// twenty guard sites above this layer read nil as "place it yourself", and an
// empty struct would tell them they had a display.
func dialX11() *x11 { return nil }

func (x *x11) monitors() []mon                 { return nil }
func (x *x11) pointer() (int, int, bool)       { return 0, 0, false }
func (x *x11) activeMon() mon                  { return mon{} }
func (x *x11) activeWindow() winID             { return 0 }
func (x *x11) fullscreenActive() (winID, bool) { return 0, false }

func (x *x11) prepare(winID, bool)                  {}
func (x *x11) plain(winID)                          {}
func (x *x11) stateMsg(winID, uint32, ...string)    {}
func (x *x11) settle(winID, int, int, bool, int)    {}
func (x *x11) above(winID)                          {}
func (x *x11) raise(winID)                          {}
func (x *x11) lower(winID)                          {}
func (x *x11) windowMon(winID) mon                  { return mon{} }
func (x *x11) place(winID, int, int, bool, int)     {}
func (x *x11) corner(winID, int, int)               {}
func (x *x11) moveResize(winID, int, int, int, int) {}
func (x *x11) unlisted(winID)                       {}
func (x *x11) unlistedPlain(winID)                  {}
func (x *x11) moveTo(winID, int, int)               {}
func (x *x11) flush()                               {}
func (x *x11) setName(winID, string)                {}
func (x *x11) quiet(winID)                          {}
func (x *x11) activate(winID)                       {}
