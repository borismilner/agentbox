CREATE TABLE items (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,
    level       TEXT NOT NULL,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    options     TEXT NOT NULL DEFAULT '[]',
    timeout_s   INTEGER NOT NULL DEFAULT 0,
    dflt        TEXT NOT NULL DEFAULT '',
    agent       TEXT NOT NULL,
    project     TEXT NOT NULL DEFAULT '',
    session     TEXT NOT NULL DEFAULT '',
    state       TEXT NOT NULL DEFAULT 'pending',
    answer      TEXT,
    reply       TEXT,
    created_at  INTEGER NOT NULL,
    resolved_at INTEGER
);

CREATE INDEX idx_items_state   ON items (state);
CREATE INDEX idx_items_created ON items (created_at DESC);

CREATE TABLE transitions (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id    TEXT NOT NULL REFERENCES items (id),
    from_state TEXT NOT NULL,
    to_state   TEXT NOT NULL,
    at         INTEGER NOT NULL
);

CREATE INDEX idx_transitions_item ON transitions (item_id);
