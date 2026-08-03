# ADR-0002: UI toolkit - Gio, with gotk4/libadwaita as spike challenger

Status: superseded by [ADR-0009](ADR-0009-ui-toolkit-wails-v3.md) (2026-07-24)

Accepted 2026-06-12; the no-webview constraint it rests on was reopened by
Boris on 2026-07-24 in favour of visual quality. The Gio implementation in
`internal/ui` shipped M1 through M8 and was deleted at the M9 cutover;
ADR-0009 describes the replacement.

## Context

Hard constraints: single native executable, no webview/browser (rules out
Wails/Tauri style), bespoke visual quality as a requirement (vision
principle 7), X11 now and Wayland later (NFR10), animation, live dark/light.

Landscape as of June 2026 (web-verified; versions and dates checked):

- Gio v0.10.0 (May 2026). Immediate-mode custom GPU renderer. First-class
  native backends for BOTH X11 and Wayland, selectable by build tags - the
  best Wayland story in pure Go. Full pixel control, animation is natural to
  the model, smallest binaries (~8-12 MB stripped). Funded (sponsors +
  OpenCollective), steady releases, monthly newsletters. Costs: still 0.x
  with deliberate breaking changes; you build the design system yourself;
  no screen-reader support; cgo on Linux (X11/Wayland/EGL headers).
- gotk4 + gotk4-adwaita: the only genuinely GNOME-native option - real
  Adwaita widgets, Pango text, free dark/light, free AT-SPI accessibility,
  GTK4 handles xdg-activation correctly. But the bindings are coasting:
  last tag Aug 2024 (generated against GTK 4.14), last push July 2025,
  regeneration is DIY. The no-cgo alternative binding (puregotk) was
  archived Feb 2026. Pinning to GTK 4.14-era APIs matches GNOME 46 today
  but is a bet on a one-maintainer project that has slowed.
- Cogent Core v0.3.35 (June 2026). Prettiest defaults (Material 3, theming,
  animations built in), very active. But WebGPU surfaces are X11-only (runs
  under XWayland on Wayland - weakest native-Wayland story), heavy
  dependency tree, largest binaries, real API churn within 0.x.
- Fyne v2.7.4 (May 2026). Most mature project process; accessibility
  first-steps land in 2.8 (June 2026). But the visual ceiling is the lowest
  for bespoke cards (apps look like Fyne apps), and the Wayland build is a
  compile-time fork with open focus bugs.
- No credible new pure-Go entrant in 2025-2026.

## Decision

Gio, validated by the M1 spike: build the card mock from 03-ui-ux.md in Gio
and in gotk4/libadwaita, dark and light, side by side. Gio wins unless the
spike shows its text rendering or input handling unfit, in which case gotk4
with GTK pinned at 4.14 takes over and this ADR is superseded.

Why Gio over gotk4 despite gotk4's free polish: the card design we want is
not an Adwaita dialog; we would fight the house style either way. Gio's
Wayland backend is native and maintained; gotk4's binding layer is the
single biggest staleness risk in either stack. And Gio keeps the binary
self-contained instead of requiring GTK4/libadwaita at runtime.

## Consequences

- `internal/ui` owns a small design system: theme tokens (spacing, radius,
  severity palette, identity hues), a card layout, a markdown-subset
  renderer. Budgeted as real work in M1/M2, not incidental.
- Window-per-prompt is the windowing pattern (see 04-platform.md): create a
  fresh window per card; never unhide a long-lived one. Gio exposes raw X11
  handles for the user-time-0 / _NET_ACTIVE_WINDOW work.
- Accessibility is keyboard + contrast only for now (NFR9 narrowed);
  revisit if Gio grows AT-SPI support.
- Pin a Gio minor per milestone; expect small API migrations between 0.x
  minors and absorb them deliberately.

## Validation (2026-06-12, M1 spike outcome)

Gio is confirmed; the gotk4 side of the spike was not needed. The card from
03-ui-ux.md was built and verified live on GNOME 46/X11, dark and light:
text rendering (Cantarell via fontconfig path), drawn severity icons,
keyboard and mouse answering, and the full blocking round trip all work.
Two platform facts discovered and worked around in `internal/ui/x11.go`:
Gio's X11 backend never disables decorations and cannot position windows,
so a side pure-Go X11 connection (jezek/xgb) sets _MOTIF_WM_HINTS, window
type (dialog/notification), _NET_WM_STATE_ABOVE and exact placement.
Build deps beyond the docs list: libvulkan-dev and libx11-xcb-dev are
required (Gio links Vulkan unconditionally on Linux).

## Sources

gioui.org/news/2026-05, pkg.go.dev/gioui.org, github.com/fyne-io/fyne
(releases, issues 5908/5471), cogentcore.org + cogentcore/webgpu,
github.com/diamondburned/gotk4, github.com/jwijenbergh/puregotk (archived).
