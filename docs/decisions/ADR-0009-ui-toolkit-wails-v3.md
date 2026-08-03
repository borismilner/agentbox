# ADR-0009: UI toolkit - Wails v3 (webview), superseding ADR-0002

Status: accepted (2026-07-24)

Supersedes [ADR-0002](ADR-0002-ui-toolkit-gio.md).

## Context

ADR-0002 chose Gio under a hard constraint from 00-vision.md principle 6 and
NFR1: one static binary, native rendering, **no webview**. That constraint was
honoured and the Gio build shipped M1-M8 - cards, inbox, viewer, markdown
engine, session tab, settings.

Boris reopened the constraint on 2026-07-24 with a different goal: the
interaction with AI agents should be *beautiful*, and each surface should be
markedly more polished than its Gio original. That is a direct trade against
principle 6, and he made the call knowingly.

What the Gio build costs us against that goal:

- Every visual is hand-built. A gradient, a blur, a shadow, a spring
  animation, a rounded inset - each is layout code, so polish is expensive
  and therefore rationed.
- `internal/markdown/render.go` is 600 lines of AST-to-Gio layout that exists
  only because an HTML intermediate was banned. Tables, code blocks, charts
  and mermaid each cost bespoke work.
- The theme is a Go struct built once at daemon start. `theme.mode` is tagged
  "applies on restart" because re-theming means rebuilding the widget tree.
- No text selection, no browser-grade text shaping, no devtools.

## Decision

Move the UI to **Wails v3** (webview: WebKitGTK 6.0 under GTK4 on Linux) with
a **Svelte 5 + Tailwind v4** frontend, embedded in the binary via `go:embed`.

The architecture does not change. `internal/webui` satisfies
`daemon.Presenter` and calls the same `Resolver` that `internal/ui` did, so
the daemon, queue, store, protocol, MCP bridge, sound and session driver are
untouched. This is a toolkit swap behind a seam that already existed.

Window model is unchanged (04-platform.md): one window per prompt, born and
destroyed with the item.

## Consequences

- **Principle 6 is amended, not quietly broken.** AgentBox is still one binary
  and still local-only, but it is no longer "native rendering, no webview".
  It now requires `libwebkitgtk-6.0` and GTK4 at runtime. 00-vision.md must
  say so.
- **Wails v3 is alpha** (`v3.0.0-alpha2.117`, nightly-tagged). Pin the exact
  version in go.mod and move deliberately; do not float.
- **Focus policy needs new machinery.** GTK4 removed
  `gtk_window_set_accept_focus` / `set_focus_on_map`, and Wails' `Show()`
  calls `gtk_window_present()`, which is literally a request for focus.
  Vision principle 3 (pop above, never grab) survives only via the sequence
  in `internal/webui/x11.go` - see Validation.
- **The markdown renderer collapses.** goldmark emits HTML directly and
  chroma emits classes coloured by the same CSS variables as everything
  else, so code follows a theme change with no second palette. `parse.go`
  and its golden tests stay useful for the Gio path until it is removed;
  `render.go` becomes dead once the port completes.
- **Theming becomes live.** Tokens are CSS custom properties pushed from Go,
  so `config.Watch` re-themes every open window mid-frame. The
  "applies on restart" tag comes off theme and font.
- **Binary is smaller** (16 MB spike vs 43 MB Gio) but gains a runtime
  dependency, which is the opposite of the ADR-0002 trade.
- **The screenshot harness does not port.** `internal/ui/shots_test.go`
  renders Gio offscreen; the web surfaces need a different approach
  (headless webview or Playwright) and currently have no tests at all.
- Accessibility improves for free: WebKit gives text selection, real text
  shaping and AT-SPI, none of which Gio offered (NFR9 can widen).

## Validation (2026-07-24, spike + first two surfaces)

Spike (`scratchpad/spike`, throwaway) answered every load-bearing question
before any AgentBox code moved:

- Builds and runs on Ubuntu 24.04, GNOME, X11, GTK 4.14.5, WebKitGTK 2.52.3.
- Multi-window, frameless, always-on-top and exact placement all work - but
  `_NET_WM_STATE` and position must be set by **client message after the
  map**. Mutter replaces a pre-map state with `DEMANDS_ATTENTION` and
  re-places the window at 0,0.
- Focus is not stolen, verified by reading `_NET_ACTIVE_WINDOW` before and
  after showing a card. The working sequence is: realize the GtkWindow early
  (`gtk_widget_realize`, without presenting), read its X11 id via
  `gdk_x11_surface_get_xid`, set AgentBox's existing `_NET_WM` hints including
  `_NET_WM_USER_TIME = 0`, map with `gtk_widget_set_visible` **instead of**
  `gtk_window_present`, then settle stacking and placement by client
  message.
- `WebviewWindow.NativeWindow()` returns the GtkWindow pointer, so the xgb
  side-channel from the Gio package's `x11.go` ports over intact (it lives at
  `internal/webui/x11.go` now).

Then the card and session surfaces were built and verified live on the
desktop (screenshots in the session log).

## Cutover, 2026-07-25

`agentbox daemon` renders through `internal/webui` and `internal/ui` is deleted.
Two things worth carrying forward from doing it:

- **`application.InvokeSync` is unusable before `App.Run()`.** It dispatches
  through the platform application that `Run` creates, so calling it earlier is a
  nil dereference, and AgentBox reaches the UI earlier than it looks:
  `daemon.New` re-presents an unresolved item from the store while the daemon is
  still being constructed, and the socket is served a moment before the loop comes
  up, so a cold-start `agentbox show` / `app` / `progress` lands in the same
  gap. All four were confirmed to crash. `UI.onMain` now queues window work and
  replays it from the `events.Common.ApplicationStarted` handler, which does
  fire on GTK4 here. Anything new that opens a window must go through it, not
  straight to `InvokeSync`.
- **The presence monitor is not UI code.** The FR29/FR44 X11 idle / fullscreen /
  desktop-DND reader lived in the Gio package but has no toolkit in it; it moved to
  `internal/presence` rather than being ported or dropped. Dropping it would have
  been silent - every absent signal reads as "user present" by design.

Measured after the cutover, against M1's acceptance check: a card that needs a new
window is mounted in ~360-400 ms (budget 1.5 s cold), and a queued item arriving
while a card window is open reuses that window with no window or bundle work at
all (budget 300 ms warm). Both measured daemon-to-surface-mount, not to first
paint.

## Open risks

- Alpha churn: a nightly bump can break the window API we depend on.
- Rounded frameless corners need an ARGB visual; `WindowIsTranslucent` +
  `BackgroundTypeTransparent` did not produce one under GTK4 here, so cards
  are square-cornered today.
- The surfaces have no automated visual check. The Gio offscreen harness did not
  port and nothing replaces it; a surface is checked by driving a real window
  (`tools/uidrive/uidrive.py`).

## Sources

github.com/wailsapp/wails releases (v3.0.0-alpha2.117, 2026-07-08),
v3.wails.io/quick-start/installation, `wails3 doctor` on this machine,
the spike under `scratchpad/spike`.
