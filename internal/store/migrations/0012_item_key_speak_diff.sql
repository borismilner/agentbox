-- Three things an item carried in memory and lost on the way to the table.
--
-- session_key is the only identity that names ONE session (proto.Identity.Key).
-- Without it an item can only be matched back to its author by the agent /
-- project / session triple, which two Claude sessions in one repo share - the
-- exact ambiguity the key was added to end. The Agents board needs it to show a
-- row its own items rather than its neighbour's.
--
-- speak and diff were written into FR73's read-back and taken straight out
-- again: proto.Item has both fields, so it compiled and the tests passed, but
-- neither was a column, and the reader would have promised two things the read
-- behind it could not deliver. A resolved review whose diff cannot be read back
-- is a record of a decision with the thing decided missing.
ALTER TABLE items ADD COLUMN session_key TEXT NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN speak TEXT NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN diff TEXT NOT NULL DEFAULT '';

-- Reading a row back by its author is the query the Agents board makes on every
-- opened row; everything else about items is read newest-first over the whole
-- table and needs no index of its own.
CREATE INDEX IF NOT EXISTS idx_items_session_key ON items(session_key, created_at DESC);
