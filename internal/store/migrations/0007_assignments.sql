-- M12 / FR82: assignments - work AgentBox hands to a Claude agent on its own.
--
-- Two tables, and the split matters: an assignment is a DEFINITION that a human
-- and an agent both edit over months, and a run is a fact about one execution
-- that must still be readable long after the definition changed under it. So a
-- run keeps the parameter values it actually used rather than pointing at the
-- ones the assignment holds now.

CREATE TABLE assignments (
    id          TEXT PRIMARY KEY,
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    -- The prompt, with {{placeholder}} spans filled from params at run time.
    prompt      TEXT    NOT NULL,
    -- params_spec is the typed knobs (JSON array); params is their current
    -- values (JSON object). The values are stored SEPARATELY from the spec, and
    -- separately from any custom panel, so that a panel which fails to render
    -- can never make an assignment uneditable - the knobs are always a way in.
    params_spec TEXT    NOT NULL DEFAULT '[]',
    params      TEXT    NOT NULL DEFAULT '{}',
    -- The escape hatch (ADR-0010 sandbox). Empty means the typed knobs render.
    panel_html  TEXT    NOT NULL DEFAULT '',
    model       TEXT    NOT NULL DEFAULT '',      -- '' = whatever claude defaults to
    mode        TEXT    NOT NULL DEFAULT 'plan',  -- plan | full, as the session surface
    dir         TEXT    NOT NULL DEFAULT '',      -- working directory; '' = daemon's
    -- '' = ad-hoc. Otherwise "every 30m" | "every 4h" | "every 1d" |
    -- "daily 09:00" | "weekly mon 09:00". Parsed by internal/assign.
    schedule    TEXT    NOT NULL DEFAULT '',
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_ms  INTEGER NOT NULL,
    updated_ms  INTEGER NOT NULL,
    last_run_ms INTEGER NOT NULL DEFAULT 0,
    -- next_run_ms is stored rather than derived so a restart does not reset every
    -- interval, and so the panel can say when the next one is without knowing the
    -- schedule grammar.
    next_run_ms INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE assignment_runs (
    id            TEXT    PRIMARY KEY,
    assignment_id TEXT    NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    started_ms    INTEGER NOT NULL,
    ended_ms      INTEGER NOT NULL DEFAULT 0,
    -- running | ok | failed | skipped. skipped is a slot that came due while the
    -- machine was off: recorded rather than caught up, so "3 missed while off" is
    -- something the panel can say instead of three runs firing at once.
    state         TEXT    NOT NULL,
    trigger       TEXT    NOT NULL,            -- schedule | manual | agent
    params        TEXT    NOT NULL DEFAULT '{}',
    summary       TEXT    NOT NULL DEFAULT '',
    error         TEXT    NOT NULL DEFAULT '',
    session_id    TEXT    NOT NULL DEFAULT '',
    -- Whatever the run chose to record for later analysis (JSON). This is the
    -- half of "collect usage statistics" that outlives the run that took them.
    data          TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX idx_assignment_runs_assignment ON assignment_runs(assignment_id, started_ms DESC);
CREATE INDEX idx_assignments_next ON assignments(enabled, next_run_ms);
