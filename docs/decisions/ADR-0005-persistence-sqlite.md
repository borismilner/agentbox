# ADR-0005: SQLite application database (modernc.org/sqlite), managed migrations

Status: proposed

## Context

History (FR10) and crash safety (NFR7) were the first drivers, but the
database is the general store for whatever AgentBox needs over its life:
scheduled items (FR31), stats (FR35), and needs that do not exist yet.
NFR1 wants a CGO-free build. The DB must exist without ceremony, and since
AgentBox is long-lived and AI-maintained, schema evolution must be managed,
not ad hoc.

## Decision

- modernc.org/sqlite (pure Go, no CGO), WAL mode, single file at
  `$XDG_STATE_HOME/agentbox/agentbox.db`. Created automatically (directory
  included) at daemon start when missing; first start and thousandth start
  run the same code path.
- Migrations: numbered SQL files embedded in the binary
  (`internal/store/migrations/NNNN_name.sql`, go:embed), forward-only,
  each applied in its own transaction, tracked in a `schema_migrations`
  table (version, name, applied_at). They run at daemon startup after the
  instance lock and before the socket binds, so no request ever sees a
  half-migrated schema.
- A failed migration aborts startup with a teaching error and a log event
  naming the migration; the transaction rollback leaves the previous
  schema intact.
- Version skew: a newer binary upgrades an older DB; an older binary that
  finds a higher schema version refuses to start rather than corrupt data.
- Runner: hand-rolled (~60 lines). Forward-only embedded SQL needs no
  framework; the runner lives in `internal/store` and is tested against
  golden schemas.
- Initial schema: `items` and `transitions`; every later feature adds a
  migration instead of a side file.

## Alternatives

- golang-migrate / goose: capable, but they bring CLI surface and driver
  matrices for a fraction of their features. Revisit only if
  down-migrations or multi-database support ever matter.
- JSONL append log: search, stats and crash re-presentation all end up
  reimplementing a database badly.
- bbolt: KV shape fights query-shaped needs (filter by agent, level, time
  range, text).
- CGO sqlite (mattn): faster, breaks the static no-CGO build for zero
  benefit at this scale.

## Consequences

- One DB through `internal/store` for every persistence need; no side
  files, one thing to back up (a file copy).
- `agentbox status --json` reports db path, size and schema version;
  migrations emit `store.migrated` log events (NFR13).
- Schema history reads as the migration directory listing; an AI
  maintainer can reconstruct the schema's evolution from the binary alone.
- Forward-only has one sharp edge and `make rollback` is where it cuts: the
  build being restored may be older than the store, and refusing to start is
  a total outage at the moment somebody is already recovering from something
  else (R-23). So the rollback asks first. `agentbox store schema` reports
  what the store is at and what the asking build knows, exiting 1 when that
  build would refuse it, and `make rollback-check` stops the rollback on that
  answer. The build being restored is the one asked, because it is the only
  thing that knows which migrations it carries.
