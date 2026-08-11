# Docs index

## Looking for what a feature is for, rather than how it is built?

That is the wiki, not this folder. It explains each feature to somebody who has
not bought in yet, and says why it is shaped the way it is:

- https://gitlab.com/fu-bar/agentbox/-/wikis/home (the original)
- https://github.com/borismilner/agentbox/wiki (the mirror, published by
  `tools/wiki/publish.sh`)

Its source is [wiki/pages/](wiki/pages/) in this repo, so it is edited here and
reviewed in normal commits. Never edit a wiki page in a browser: the next publish
overwrites it. [wiki/DESIGN.md](wiki/DESIGN.md) has the page inventory, the
template and the voice, and [wiki/FACTS.md](wiki/FACTS.md) is the audited fact
base every claim in the wiki is written against.

The split, so nothing gets written twice: the wiki answers what a feature does
and why. The docs below carry the design, the requirements and the decisions.

Two files the docs below cite are not in this repo: `STATUS.md`, the running
state, and `history.md`, the dated log of decisions, defects and verifications.
Those are the maintainer's working notes and they live in a private tree, linked
into `docs/` on his own machine. Nothing in the product depends on them.

Reading order:

| Doc | Contents |
|-----|----------|
| [00-vision.md](00-vision.md) | Problem, principles, non-goals |
| [01-requirements.md](01-requirements.md) | Functional and non-functional requirements |
| [02-architecture.md](02-architecture.md) | Binary layout, daemon, IPC protocol, MCP bridge |
| [03-ui-ux.md](03-ui-ux.md) | Surfaces, card design, keyboard map, sound design |
| [04-platform.md](04-platform.md) | X11/Wayland, GNOME tray, audio, autostart, paths |
| [05-roadmap.md](05-roadmap.md) | Milestones with acceptance checks |
| [06-configuration.md](06-configuration.md) | Every knob, its default, and why |
| [07-field-requests.md](07-field-requests.md) | Use cases found while using AgentBox on real work; picked up before "Later / parked" |
| [08-assignments.md](08-assignments.md) | Recurring AI work AgentBox runs on its own (M12/FR82); read before touching assignments |
| [09-sync.md](09-sync.md) | Multi-agent coordination and the Agents surface (FR83): roster, discovery, locks, signals, shared values. **All five slices are built, deployed and verified live**, including the hooks that put every session on the board without being asked |
| [agent-manual.md](agent-manual.md) | Complete reference for an AI agent driving AgentBox (MCP + CLI) |
| [recipes.md](recipes.md) | Copy-paste integration snippets |
| [sample.md](sample.md) | The renderer's fixture: every block the viewer draws (`agentbox show docs/sample.md`) |
| [showcase.md](showcase.md) | The self-performing demo: preflight, traps, slide notes. Live only - the video pipeline was deleted on 2026-08-06 |
| [decisions/](decisions/) | ADRs, one decision per file |
| [wiki/](wiki/) | The public wiki: its source, its design and the audited facts it is written from |

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
