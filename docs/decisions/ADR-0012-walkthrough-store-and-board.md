# ADR-0012: walkthroughs - a durable review object in SQLite, rendered by a native board

Status: proposed (2026-07-29)

## Context

FR58 wants a review the human walks whole and hands back in one turn; FR59
wants that review to be a durable, searchable library object that agents can
amend; FR61 wants every number on it computed rather than claimed. Seven mock
rounds (tools/mockups/review-board.jsx) settled the UX; this ADR settles the
object and the surface behind it.

Two facts forced the shape. The artifact sandbox cannot persist anything (an
opaque origin has no storage, ADR-0010), so the mock's one-sitting limitation
is structural - the real board must be a native surface. And the hand-built
kit's checker died of a second copy of its citations (44 ranges hardcoded
twice), so wherever diff knowledge lives, it must live once.

## Decision

- **The object is a walkthrough**, FR59's own name: SQLite tables
  (migration 0005), `agentbox.v1.walkthrough_*` methods, `*_walkthrough` MCP
  tools, `agentbox walkthrough` CLI. The surface is **the board**
  (`?surface=board`, internal/webui/board.go). The shipped diff card
  (`request_review`, FR33/FR62) is a different feature and keeps its name.
- **The spec carries the change's unified diff as a manifest.** Blocks cite
  only `{path, lines}`; AgentBox derives added/removed markings, renders
  deleted lines from the manifest, and (in a later slice) computes coverage by
  intersecting citations with hunks. Stating diff status on a file-backed
  block is a validation error. AgentBox never shells into repositories - the
  agent supplies the diff, AgentBox does all arithmetic.
- **Spec and annotations live in separate tables** joined by stable step
  ids. Amendment rewrites the spec and cannot touch a mark or comment;
  survival on untouched steps is structural. A step hash recorded at each
  verdict makes amendment-under-marks detectable (stale, never silently
  wrong).
- **Files are read daemon-side at render time**, jailed to the spec's
  repo_root. The surface receives per-line chroma spans (the one {@html}
  field, class-based so themes apply) plus structured channels for line
  numbers, diff status and annotations - three visual channels that cannot
  impersonate each other because they never share an encoding.
- **Submission resolves a blocking `await_walkthrough` when an agent
  waits; otherwise it persists as `submitted`** and the next session takes
  it exactly-once (`read_walkthrough(ack)`, the store.Resolve pattern).
  The unclear verdict requires words at submit; understood may stay
  silent, marked `unsaid`.
- **Ids are caller-minted** (`w`+12hex, the artifact-id precedent), so an
  agent can await the instant create returns and a retried create is
  idempotent.

## Consequences

The board bundle loads lazily so no card pays for it (NFR2). Every
annotation is write-through to SQLite; a daemon restart or window close
loses nothing (NFR7). A walkthrough outlives its agent, its branch and its
window, and rendering improvements reach every stored review (FR59's whole
point). The deferred halves - submission payload, coverage/drift
computation, amendment, the library surface - build on these tables without
another migration unless they need new columns.
