# ADR-0003: IPC via unix socket + JSON-RPC 2.0, client auto-spawns daemon

Status: proposed

## Context

Agents must reach a resident daemon: ad-hoc CLI calls, an MCP bridge, and
possibly direct integrations. Requirements: local-only (NFR8), blocking calls
that stay open for minutes (ask), trivial to speak from any language, and a
daemon that does not need to be running before the first call.

## Decision

- Socket: `$XDG_RUNTIME_DIR/agentbox/agentbox.sock`, directory mode 0700,
  peer UID checked via SO_PEERCRED.
- Protocol: JSON-RPC 2.0, one object per line. Long-lived `ask` calls keep
  the connection open; the response is written when the item resolves.
- Single instance by default (NFR12): `flock` on
  `$XDG_RUNTIME_DIR/agentbox/daemon.lock` is the authoritative lock, acquired
  before binding; only the holder removes a stale socket. Losing spawners
  exit 0. Socket-bind alone is not enough: it cannot arbitrate the
  spawn race or stale-socket cleanup atomically.
- Explicit multi-instance: `--instance NAME` / `AGENTBOX_INSTANCE` moves lock,
  socket and state to a per-name directory. Deliberate opt-in only; an
  agent cannot get there by accident.
- Auto-spawn: the client library execs `agentbox daemon` detached (setsid) when
  connect fails, then retries connect for up to 2 s. Spawning when a daemon
  already exists is harmless by construction.

## Alternatives

- D-Bus: idiomatic on Linux desktops and would give activation for free, but
  awkward for blocking minutes-long calls, heavier Go story, and useless to
  the MCP bridge. The tray (SNI) still uses D-Bus where needed; the core API
  does not.
- HTTP on localhost: port collisions, a network listener (violates NFR8),
  and CORS/browser surface we do not want.
- gRPC: codegen and dependency weight with no payoff at this scale.
- systemd socket activation: nice refinement, kept as an option; auto-spawn
  must work anyway for non-systemd contexts.

## Consequences

- Any language with a socket and a JSON encoder can integrate.
- Protocol versioning lives in method names (`agentbox.v1.*`).
- The client library is the single place that knows how to find or start the
  daemon; CLI and MCP bridge both sit on it.
