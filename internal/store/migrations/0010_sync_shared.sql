-- FR83 slice 4: shared values, the compare-and-swap blackboard.
--
-- Coordination that is neither a turn nor an event. A lock says whose turn it is
-- and a signal says something happened; neither can say "chunk 7 is mine", which
-- is what a fanned-out job needs before it starts work another agent has started.
--
-- A later migration rather than a second table in 0008, for the reason 0008's own
-- comment gives: a table with no code above it rots. This one arrives with the
-- verbs that read and write it.
--
-- version is what makes this a blackboard rather than a shared variable. It starts
-- at 1 and rises by one per write, so a writer that read version 3 and asks to
-- write "only if it is still 3" cannot lose an update it never saw. Version 0 is
-- therefore free to mean "this key does not exist yet", which is the whole claim
-- idiom: ten workers CAS-from-empty on claims/<chunk> and exactly one of them wins
-- each key, with no lock, no read-modify-write and no retry loop.
--
-- owner and owner_agent are RECORDED rather than deduced, which is the lesson
-- migration 0009 cost a session. A claim's value cannot say whether the session
-- that made it is still alive, and once that session's roster row is gone there is
-- nothing left to ask - so the writer's session key goes in the row at write time,
-- and the agent NAME beside it. Without the name, an orphaned claim can only be
-- reported as "owned by somebody who is gone"; with it, the read says which agent
-- died holding chunk 7, which is what a human draining the rest needs to know.
--
-- Nothing here is trimmed, deliberately, and that is the difference from signals.
-- Retention on a claim table would hand one chunk to two agents, which is the exact
-- failure the gap check exists to prevent. Shared values leave when an agent
-- deletes them; the cap on how many may exist REFUSES a new key instead of evicting
-- an old one (see sharedKeyMax), because coordination state that vanishes silently
-- is worse than a write that fails loudly.
CREATE TABLE sync_shared (
    key         TEXT    PRIMARY KEY,
    value       TEXT    NOT NULL,
    version     INTEGER NOT NULL,
    -- The owning session's key, empty when the value is unowned. A progress counter
    -- wants no owner; a claim does.
    owner       TEXT    NOT NULL DEFAULT '',
    -- The owner's agent name at the time of the write, so a value whose session is
    -- gone can still say who left it.
    owner_agent TEXT    NOT NULL DEFAULT '',
    updated_ms  INTEGER NOT NULL
);
