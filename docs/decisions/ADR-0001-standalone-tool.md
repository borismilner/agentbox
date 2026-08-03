# ADR-0001: Standalone tool, own repository

Status: accepted (2026-06-12)

## Context

devtool exists and could host this as a subcommand. But devtool is a headless
HTTP verification runner; this is a resident desktop daemon with a GUI
toolkit dependency, sound assets, a tray icon and its own release cadence.
The two share no code paths. Boris leans standalone.

## Decision

New repository at `~/me/projects/agentbox`, module
`github.com/borismilner/agentbox`. No dependency in either direction between
AgentBox and devtool.

## Consequences

- devtool stays CGO-free and headless; AgentBox owns the desktop complexity.
- AgentBox can be versioned and released on its own schedule.
- Anything devtool wants to tell the user goes through AgentBox's public CLI
  like every other agent - which doubles as dogfooding.
