# ADR-0004: Agent API is MCP-first, CLI-equal

Status: implemented (M4, 2026-06-13; go-sdk v1.6.1)

## Context

The primary callers are AI coding agents. In 2026 MCP is the common protocol
agent hosts speak for tools; a blocking question maps exactly to a tool call
that returns the user's answer. But hooks, shell scripts and cron jobs need
something exec-able with exit codes.

## Decision

Two front doors of equal rank over the same socket protocol:

1. `agentbox mcp` - MCP stdio server exposing `notify_user`, `ask_user`,
   `confirm_action`. Tool descriptions are written for the model reading
   them: when to interrupt the user, when not to.
2. The CLI subcommands - same capabilities, exit-code and stdout semantics,
   used by hooks and scripts.

The daemon knows nothing about MCP; the bridge is a proxy process.

## Alternatives

- CLI only: every MCP-speaking host would need a wrapper; tool-call ergonomics
  (typed schema, streaming errors) lost.
- MCP only: hooks and plain scripts locked out.
- Letting the daemon itself listen for MCP: couples daemon lifetime to host
  sessions and complicates the security story for zero benefit.

## Consequences

- SDK: the official modelcontextprotocol/go-sdk. It carries a v1 stability
  promise (v1.0.0 Sept 2025, v1.6.x current mid-2026, security patches
  flowing); mark3labs/mcp-go is healthy but still 0.x with regular breaking
  bumps - wrong risk profile for a long-lived daemon. Churn, if any, stays
  isolated in `internal/mcp`.
- Answer semantics (timeout, default, cancel) must be expressible in both
  front doors from day one; the proto types are the source of truth.
