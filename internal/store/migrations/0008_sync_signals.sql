-- FR83 slice 3: signals - fire-and-forget events between agents, with durable
-- pickup.
--
-- Two decisions are baked into this table rather than into the code above it:
--
-- AUTOINCREMENT is load-bearing, not decoration. seq is the ONE global cursor
-- every waiter carries, so it must never go backwards. A plain INTEGER PRIMARY
-- KEY reuses rowids: retention deletes the whole table on a quiet week, the next
-- insert lands back at 1, and every agent holding a cursor from before then
-- silently re-reads signals it has already acted on. AUTOINCREMENT keeps the
-- high-water mark in sqlite_sequence, which is also how a reader distinguishes
-- "nothing new" from "your cursor fell off the trimmed edge" when the table is
-- empty.
--
-- The poster's identity is stored flat rather than as a JSON blob because it is
-- read on every delivery: session_key is what makes a reply addressable
-- (post to "to:<key>"), and agent/project are what a human reads in the log.
CREATE TABLE sync_signals (
    seq         INTEGER PRIMARY KEY AUTOINCREMENT,
    topic       TEXT    NOT NULL,
    agent       TEXT    NOT NULL DEFAULT '',
    project     TEXT    NOT NULL DEFAULT '',
    session_key TEXT    NOT NULL DEFAULT '',
    -- The payload as the posting agent wrote it: raw JSON, capped by size, never
    -- interpreted. It is the agent's own vocabulary and agentbox has no business
    -- having an opinion about its shape.
    data        TEXT    NOT NULL DEFAULT '',
    at_ms       INTEGER NOT NULL
);

-- Catch-up reads are always "this topic (or prefix), above this seq", so the
-- index carries both columns in that order.
CREATE INDEX idx_sync_signals_topic ON sync_signals(topic, seq);
-- Retention trims by age across every topic at once.
CREATE INDEX idx_sync_signals_at ON sync_signals(at_ms);
