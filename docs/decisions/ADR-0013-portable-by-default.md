# ADR-0013: Portable by default, with X11 as an enhancement

Status: accepted (2026-08-11)

Amends [00-vision.md](../00-vision.md) non-goal 4 and
[01-requirements.md](../01-requirements.md) NFR10.

## Context

The vision's fourth non-goal read "No Windows/macOS support in v1. Keep code
portable where it costs nothing, but never trade Linux quality for it." NFR10
read "X11 today, Wayland-ready", and asked that no X11-only mechanism be
load-bearing without a documented Wayland equivalent.

Boris reopened it on 2026-08-11: as portable as possible, without breaking how it
works or looks on the machine he uses, and portability written up as a feature
rather than a limitation. Some things may stay X11-only where that is what makes
them work at all.

The non-goal turned out to be describing something that had quietly stopped being
true. An audit of the whole tree found the distance to another platform much
shorter than the sentence implied - six syscalls, two process-tree reads, and one
build tag with nothing behind it. Set against that, the second clause ("keep code
portable where it costs nothing") had been honoured well enough that the first one
was mostly inertia.

But it was also hiding a real defect, and that is what made this worth doing now
rather than later. `internal/webui/x11.go` carried the only build tag in the
source tree, `//go:build linux`, and there was no file on the other side of it.
Twenty call sites above it are written as `if u.x != nil { place it } else { let
the desktop place it }`, so the no-X11 path was fully written, fully reachable,
and never once executed. R-12 is what that costs: the drop-down panel's roll took
that branch, recorded itself as open from a `defer` that ran on the failure path
too, and every question routed to it went to a surface nobody could see. Found by
reading, not by running, because nothing ran it.

## Decision

Portability is a supported property, checked by the build, with three tiers that
are named rather than implied:

1. **The daemon, the store, the socket, the protocol, the CLI and the MCP server
   are portable.** Every platform-specific call sits in a file whose name says
   which platform it is for, and the contract it satisfies is stated once in the
   portable caller rather than three times beside three syscalls.
2. **The UI runs wherever Wails v3 does** - GTK4/WebKitGTK on Linux, WebKit on
   macOS, WebView2 on Windows. Placement, stacking and focus control are a
   separate layer behind one seam (`dialX11`, returning nil when there is none).
   Without it every surface still appears, still carries its content, and is
   still answerable; it is placed by the window manager instead of by us.
3. **Two capabilities stay X11-only and say so at the point of use**: the global
   hotkey (`internal/hotkey`) and pointer/keyboard driving (`internal/hand`).
   Both need a display server that lets a client claim input globally, which
   Wayland refuses by design and which the other platforms answer differently
   enough to be their own piece of work. `Open` on each reports that it could not,
   rather than pretending.

`make check` enforces all of it. `test-nox11` runs the entire suite through the
no-X11 layer; `cross` compiles windows/amd64 over the whole tree and both darwin
architectures over everything that does not link a native UI.

## Consequences

**Non-goal 4 is amended, not deleted.** "Never trade Linux quality for it" stays,
and it is the reason the X11 layer was kept whole rather than reduced to a lowest
common denominator: this desktop still gets pop-above-without-focus, the
top-centre column, the exact-centre card and the rolled panel, because those are
X11 calls that no longer stand in anybody's way.

**One security guarantee genuinely changes shape on one platform**, and it is
written down instead of being quietly weaker. NFR8's peer-UID check has no
equivalent on Windows: AF_UNIX there carries no credentials and no supported call
asks for them. So on Windows NFR8 rests on the 0700 socket directory alone, which
is the first line of defence every platform has - it is the second, independent
one that is missing. `internal/server/peer_windows.go` says so in full, and
R-46 is the connect-token that would close it.

**Speech is the one feature that is genuinely thinner elsewhere.** The pipeline is
an engine writing raw PCM into a player's stdin, and macOS's own `afplay` cannot
read raw PCM from a pipe. So speech works off Linux if sox is installed and is
silent if it is not - the same fail-soft it already had without piper. Native
`say` and SAPI would be better and are a separate feature, not a port.

**A Mac build is unverified until somebody builds on a Mac.** Everything except
the two packages that link a native UI is compiled for darwin on every check;
those two need clang and a macOS SDK. The claim this ADR makes is therefore
precise: no platform-locked call remains outside a tagged file, and the UI toolkit
supports the platform. Not: somebody has run it.

## Alternatives considered

**Leave the non-goal and keep the tag decorative.** Rejected because the tag was
not free - it was actively hiding R-12, and a second platform would have found the
same class of bug in every one of the twenty guards at once.

**Abstract the placement layer behind an interface with real macOS and Windows
implementations.** Rejected for now as the wrong order: an interface with one
implementation and two stubs is the same thing as a build tag with more ceremony.
The seam is where it needs to be, so a real backend is an addition rather than a
refactor when somebody actually runs it on a Mac.

**Move to platform-idiomatic paths** (`~/Library/Application Support`, `%APPDATA%`).
Rejected deliberately: one layout on every machine is easier to support, easier to
document and easier to copy between hosts, and touching path derivation is the one
change in this area that could move somebody's existing store. XDG everywhere,
with `os.UserHomeDir` underneath, which is already correct on all three.
