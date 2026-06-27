CREATE TABLE changesets (
    id          TEXT PRIMARY KEY,
    app_id      TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'open',
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX changesets_one_open_per_app
    ON changesets(app_id)
    WHERE status = 'open';

CREATE TABLE changeset_changes (
    id           TEXT PRIMARY KEY,
    changeset_id TEXT NOT NULL REFERENCES changesets(id) ON DELETE CASCADE,
    type         TEXT NOT NULL,
    payload      TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX changeset_changes_changeset_id_idx ON changeset_changes(changeset_id);