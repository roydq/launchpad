CREATE TABLE changesets (
    id          UUID PRIMARY KEY,
    app_id      UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'open',
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX changesets_one_open_per_app
    ON changesets(app_id)
    WHERE status = 'open';

CREATE TABLE changeset_changes (
    id           UUID PRIMARY KEY,
    changeset_id UUID NOT NULL REFERENCES changesets(id) ON DELETE CASCADE,
    type         TEXT NOT NULL,
    payload      JSONB NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX changeset_changes_changeset_id_idx ON changeset_changes(changeset_id);