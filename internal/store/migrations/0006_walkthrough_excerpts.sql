-- What the citations pointed at, as they were when the review was written.
--
-- Until this table, a walkthrough stored the citation (path, line range) and
-- nothing else, and the board read the file off disk on every render. That
-- makes a review true only while the working tree stays where it was: a
-- checkout, a rename or a delete turns a step into an error, and - worse - an
-- edit that leaves the file long enough renders whatever now sits at those line
-- numbers, under the original prose and margin notes, saying nothing.
--
-- This is not the second copy of a citation FR61 forbids. The citation still
-- lives in the spec alone and is still what everything derives from; this is
-- the pinned source it names, kept so the review can be read after the tree has
-- moved on. Rows are per cited range rather than per file so a walkthrough
-- carries the lines it actually shows and not whole files it does not.
--
-- source says where the text came from, because they are not equally
-- trustworthy: 'worktree' is what the authoring agent had in front of it,
-- 'git' is the blob at pinned_sha, recovered afterwards.

CREATE TABLE walkthrough_excerpts (
    walkthrough_id TEXT NOT NULL REFERENCES walkthroughs (id) ON DELETE CASCADE,
    path           TEXT NOT NULL,
    from_line      INTEGER NOT NULL,
    to_line        INTEGER NOT NULL,
    text           TEXT NOT NULL,
    source         TEXT NOT NULL DEFAULT 'worktree',
    captured_at    INTEGER NOT NULL,
    PRIMARY KEY (walkthrough_id, path, from_line, to_line)
);
CREATE INDEX idx_wte_wt ON walkthrough_excerpts (walkthrough_id);
