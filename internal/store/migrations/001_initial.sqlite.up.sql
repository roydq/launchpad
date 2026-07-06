CREATE TABLE workspaces (
    id         TEXT PRIMARY KEY,
    name       TEXT UNIQUE NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE projects (
    id              TEXT PRIMARY KEY,
    workspace_id    TEXT NOT NULL REFERENCES workspaces(id),
    name            TEXT NOT NULL,
    primary_service TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'created',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at      TEXT,
    UNIQUE (workspace_id, name)
);

CREATE TABLE environments (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    target_type   TEXT NOT NULL DEFAULT 'kubernetes',
    target_config TEXT NOT NULL DEFAULT '{}',
    ephemeral     INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (project_id, name)
);

CREATE TABLE services (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL DEFAULT 'application',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (project_id, name)
);

CREATE TABLE processes (
    id         TEXT PRIMARY KEY,
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    command    TEXT NOT NULL DEFAULT '',
    quantity   INTEGER NOT NULL DEFAULT 1,
    expose     TEXT NOT NULL DEFAULT 'none',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (service_id, name)
);

CREATE TABLE config_vars (
    service_id     TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    key            TEXT NOT NULL,
    value          TEXT NOT NULL,
    created_at     TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (service_id, environment_id, key)
);

CREATE TABLE releases (
    id               TEXT PRIMARY KEY,
    service_id       TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    version          INTEGER NOT NULL,
    artifact_ref     TEXT NOT NULL,
    config_resolved  TEXT NOT NULL DEFAULT '{}',
    process_snapshot TEXT NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL DEFAULT 'pending',
    description      TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (service_id, version)
);

CREATE TABLE deployments (
    id             TEXT PRIMARY KEY,
    service_id     TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    release_id     TEXT NOT NULL REFERENCES releases(id),
    status         TEXT NOT NULL DEFAULT 'pending',
    version        INTEGER NOT NULL DEFAULT 1,
    target_ref     TEXT NOT NULL DEFAULT '',
    message        TEXT NOT NULL DEFAULT '',
    started_at     TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at    TEXT,
    created_at     TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX deployments_one_active_per_service_env
    ON deployments(service_id, environment_id)
    WHERE status IN ('pending', 'deploying');

CREATE INDEX deployments_service_env_created_at_idx
    ON deployments(service_id, environment_id, created_at DESC);

CREATE TABLE changesets (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'open',
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX changesets_one_open_per_project
    ON changesets(project_id)
    WHERE status = 'open';

CREATE TABLE changeset_changes (
    id           TEXT PRIMARY KEY,
    changeset_id TEXT NOT NULL REFERENCES changesets(id) ON DELETE CASCADE,
    service_id   TEXT REFERENCES services(id),
    service_name TEXT NOT NULL DEFAULT '',
    type         TEXT NOT NULL,
    payload      TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX changeset_changes_changeset_id_idx ON changeset_changes(changeset_id);

CREATE TABLE jobs (
    id            TEXT PRIMARY KEY,
    type          TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'queued',
    payload       TEXT NOT NULL,
    attempt       INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 5,
    run_at        TEXT NOT NULL DEFAULT (datetime('now')),
    leased_until  TEXT,
    leased_by     TEXT,
    last_error    TEXT,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX jobs_poll_idx ON jobs (status, run_at)
    WHERE status IN ('queued', 'failed');

CREATE INDEX jobs_lease_reclaim_idx ON jobs (leased_until)
    WHERE status = 'leased';

CREATE TABLE api_tokens (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT REFERENCES workspaces(id),
    name         TEXT NOT NULL,
    token_hash   BLOB NOT NULL,
    scopes       TEXT NOT NULL,
    expires_at   TEXT,
    revoked_at   TEXT,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX api_tokens_hash_idx ON api_tokens(token_hash)
    WHERE revoked_at IS NULL;

INSERT INTO workspaces (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'default');