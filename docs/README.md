# Docs index

Reading order:

| Doc | Contents |
|-----|----------|
| [../HANDOFF.md](../HANDOFF.md) | Resume brief: do-this-next, live state, verified vs assumed |
| [STATUS.md](STATUS.md) | Where the project stands, what to do next |
| [history.md](history.md) | The session-by-session record: decisions, defects, verifications |
| [00-vision.md](00-vision.md) | Problem, principles, non-goals |
| [01-requirements.md](01-requirements.md) | Functional and non-functional requirements |
| [02-architecture.md](02-architecture.md) | Binary layout, daemon, IPC protocol, MCP bridge |
| [03-ui-ux.md](03-ui-ux.md) | Surfaces, card design, keyboard map, sound design |
| [04-platform.md](04-platform.md) | X11/Wayland, GNOME tray, audio, autostart, paths |
| [05-roadmap.md](05-roadmap.md) | Milestones with acceptance checks |
| [06-configuration.md](06-configuration.md) | Every knob, its default, and why |
| [07-field-requests.md](07-field-requests.md) | Use cases found while using AgentBox on real work; picked up before "Later / parked" |
| [agent-manual.md](agent-manual.md) | Complete reference for an AI agent driving AgentBox (MCP + CLI) |
| [recipes.md](recipes.md) | Copy-paste integration snippets |
| [sample.md](sample.md) | The renderer's fixture: every block the viewer draws (`agentbox show docs/sample.md`) |
| [showcase.md](showcase.md) | The self-performing sales deck: preflight, traps, slide notes |
| [recording.md](recording.md) | The recorder behind the showcase takes |
| [youtube.md](youtube.md) | The upload listing for the recorded take; chapters regenerate per take |
| [decisions/](decisions/) | ADRs, one decision per file |

The two ADRs a change to the UI most often needs:
[ADR-0009](decisions/ADR-0009-ui-toolkit-wails-v3.md) (why a webview, and the
GTK4 rules that came with it) and
[ADR-0010](decisions/ADR-0010-artifact-sandbox.md) (what agent-authored HTML is
allowed to touch, and the one way out of it).

Tools worth knowing about: `tools/uidrive/uidrive.py` drives a live AgentBox window
over XTEST (keys, clicks, scroll, screenshot) on a desktop with no xdotool - how
the web UI's keyboard map and settings panel were verified. `tools/genicon` and
`tools/genearcons` generate the app icon and the earcon set.
`tools/artifact-probe.html` is an artifact that tries to escape its own sandbox and
prints what happened, so ADR-0010's guarantees can be re-checked in ten seconds:
`agentbox show --artifact tools/artifact-probe.html`.

ADR convention: `ADR-NNNN-slug.md`, statuses are `proposed`, `accepted`,
`superseded by ADR-XXXX`. A proposed ADR contains a concrete recommendation;
it flips to accepted only after Boris signs off. Never edit an accepted ADR's
decision; write a superseding one.
