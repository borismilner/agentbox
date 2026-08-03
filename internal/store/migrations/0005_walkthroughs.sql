-- Walkthroughs (FR58/FR59): durable reviews that outlive the agent that
-- created them. The agent's half (spec, diff manifest) and the human's half
-- (marks, comments) live in separate tables joined by stable step ids, so
-- an amendment can rewrite the spec without touching a single annotation.
-- Coverage, drift and orphaned-ness are always derived, never stored
-- (FR61: nothing holds a second copy of a citation).

CREATE TABLE walkthroughs (
    id            TEXT PRIMARY KEY,               -- caller-minted, w + 12 hex
    title         TEXT NOT NULL,
    repo_root     TEXT NOT NULL,
    pinned_sha    TEXT NOT NULL,
    base_sha      TEXT NOT NULL DEFAULT '',
    change_key    TEXT NOT NULL DEFAULT '',       -- root@base..pinned; groups future traversals of one change
    spec          TEXT NOT NULL,                  -- version-1 spec JSON, diff stripped
    spec_rev      INTEGER NOT NULL DEFAULT 1,
    diff          TEXT NOT NULL DEFAULT '',       -- the change manifest
    counted_steps INTEGER NOT NULL DEFAULT 0,     -- cache of the spec's code-step count, for library rows
    pos           INTEGER NOT NULL DEFAULT 0,     -- the step the human was on (FR59: progress survives)
    state         TEXT NOT NULL DEFAULT 'open',   -- open | submitted | delivered
    agent         TEXT NOT NULL,
    project       TEXT NOT NULL DEFAULT '',
    session       TEXT NOT NULL DEFAULT '',
    payload       TEXT,                           -- last submission, verbatim: the delivery record
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    submitted_at  INTEGER
);
CREATE INDEX idx_wt_state   ON walkthroughs (state);
CREATE INDEX idx_wt_updated ON walkthroughs (updated_at DESC);
CREATE INDEX idx_wt_change  ON walkthroughs (change_key);

-- The human's per-step state. step_hash fingerprints the spec step at the
-- last human touch; an amendment that changes the step under a mark sets
-- stale instead of silently re-anchoring the verdict.
CREATE TABLE walkthrough_marks (
    walkthrough_id TEXT NOT NULL REFERENCES walkthroughs (id) ON DELETE CASCADE,
    step_id        TEXT NOT NULL,
    verdict        TEXT NOT NULL DEFAULT '',      -- '' | understood | unclear | seen
    note           TEXT NOT NULL DEFAULT '',
    revealed       TEXT NOT NULL DEFAULT '[]',    -- JSON int array: check answers currently open
    cmd_runs       TEXT NOT NULL DEFAULT '[]',    -- JSON: captured check-command runs
    step_hash      TEXT NOT NULL DEFAULT '',
    stale          INTEGER NOT NULL DEFAULT 0,
    updated_at     INTEGER NOT NULL,
    PRIMARY KEY (walkthrough_id, step_id)
);

-- Anchored and step-level comments. path '' means a step-level remark; side
-- distinguishes the new file from removed lines (GitHub's RIGHT/LEFT).
CREATE TABLE walkthrough_comments (
    id             TEXT PRIMARY KEY,              -- daemon-minted, c + 12 hex
    walkthrough_id TEXT NOT NULL REFERENCES walkthroughs (id) ON DELETE CASCADE,
    step_id        TEXT NOT NULL,
    path           TEXT NOT NULL DEFAULT '',
    side           TEXT NOT NULL DEFAULT 'new',
    from_line      INTEGER NOT NULL DEFAULT 0,
    to_line        INTEGER NOT NULL DEFAULT 0,
    exact          TEXT NOT NULL DEFAULT '',
    body           TEXT NOT NULL,
    adrift         INTEGER NOT NULL DEFAULT 0,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
);
CREATE INDEX idx_wtc_step ON walkthrough_comments (walkthrough_id, step_id);
CREATE INDEX idx_wtc_path ON walkthrough_comments (path);

-- Append-only audit, the items/transitions pattern.
CREATE TABLE walkthrough_transitions (
    seq            INTEGER PRIMARY KEY AUTOINCREMENT,
    walkthrough_id TEXT NOT NULL REFERENCES walkthroughs (id) ON DELETE CASCADE,
    from_state     TEXT NOT NULL,
    to_state       TEXT NOT NULL,
    detail         TEXT NOT NULL DEFAULT '',
    at             INTEGER NOT NULL
);
CREATE INDEX idx_wtt_wt ON walkthrough_transitions (walkthrough_id);
