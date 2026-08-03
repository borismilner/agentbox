# Vision

## Problem

Terminal-based agent sessions fail at the last meter:

- An agent asks a question and blocks. The terminal is buried under other
  windows; minutes or hours are lost before anyone notices.
- Terminals get closed by accident. The pending question dies with them.
- There is no sound, no urgency level, no escalation, and no record of what
  was asked while you were away.
- With several agents running at once, there is no single place where "who
  needs me right now" is visible.

## What AgentBox is

One always-available desktop presence shared by all agents. Any agent (or
script, or hook) can:

- post an event, warning or notification, with severity and sound;
- ask a blocking question (choose among options, type text, confirm) and get
  the answer back as the call's return value;
- escalate when something urgent is being ignored;
- show rich content the terminal butchers: full markdown with real tables,
  highlighted code and charts, in a proper document viewer.

AgentBox either runs as a background daemon (tray icon near the clock, window
pops when needed) or is started ad-hoc by the first agent call. Both paths
behave identically; the first client call auto-spawns the daemon.

## Principles

1. Unmissable, not annoying. Calm visuals, short sounds, escalation only when
   the caller asks for it.
2. Keyboard first. The median question is answered in under two seconds
   without touching the mouse.
3. Pop above, never grab. Cards appear above all windows, but keyboard focus
   is never stolen while you are typing elsewhere. A stolen keystroke that
   answers a question by accident is the worst possible failure.
4. Agent-agnostic. Anything that can exec a CLI or speak MCP can use it.
5. Local only. Unix socket, no network listener, no cloud, no telemetry.
6. One binary, no cloud, no Electron. Amended 2026-07-24 (ADR-0009): AgentBox is
   still a single executable you can copy, still local-only, but it is no
   longer "native rendering, no webview" - the UI is a WebKitGTK webview and
   the binary needs libwebkitgtk-6.0 and GTK4 at runtime. The constraint was
   traded deliberately for principle 7.
7. The card is the product. It has to look better than a native dialog, not
   worse. Visual quality is a requirement, not polish to add later.
8. Configurable, never demanding. Zero required setup; every critical
   behavior (sound, focus, placement, escalation, timeouts) has a knob with
   a default chosen so most users never open the file. See
   06-configuration.md.
9. Self-teaching. An agent holding nothing but the executable can learn
   every feature: the manual is embedded in the binary, a quickstart sized
   for an agent's context is one command away, and misuse errors reply
   with the correct form instead of a shrug.

## Non-goals

- Not a chat client. Conversations stay in the agent's own UI; AgentBox carries
  discrete events and questions.
- Not a terminal replacement and not a general notification center.
- No remote or mobile delivery in v1 (a webhook relay is a later idea, see
  roadmap).
- No Windows/macOS support in v1. Keep code portable where it costs nothing,
  but never trade Linux quality for it.
