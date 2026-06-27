CREATE TABLE teams (
    id         TEXT PRIMARY KEY,
    name       TEXT UNIQUE NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE apps (
    id                     TEXT PRIMARY KEY,
    team_id                TEXT NOT NULL REFERENCES teams(id),
    name                   TEXT NOT NULL,
    stack                  TEXT NOT NULL DEFAULT 'container',
    target_type            TEXT NOT NULL DEFAULT 'kubernetes',
    target_config          TEXT NOT NULL DEFAULT '{}',
    status                 TEXT NOT NULL DEFAULT 'created',
    active_deployment_id   TEXT,
    created_at             TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at             TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at             TEXT,
    UNIQUE (team_id, name)
);

CREATE TABLE config_vars (
    app_id     TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (app_id, key)
);

CREATE TABLE process_types (
    id         TEXT PRIMARY KEY,
    app_id     TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    command    TEXT NOT NULL DEFAULT '',
    quantity   INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (app_id, name)
);

CREATE TABLE builds (
    id          TEXT PRIMARY KEY,
    app_id      TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    source_ref  TEXT NOT NULL DEFAULT '',
    image_ref   TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'pending',
    logs_url    TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE releases (
    id              TEXT PRIMARY KEY,
    app_id          TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    build_id        TEXT REFERENCES builds(id),
    version         INTEGER NOT NULL,
    config_snapshot TEXT NOT NULL DEFAULT '{}',
    image_ref       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    description     TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (app_id, version)
);

CREATE TABLE deployments (
    id          TEXT PRIMARY KEY,
    app_id      TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    release_id  TEXT NOT NULL REFERENCES releases(id),
    status      TEXT NOT NULL DEFAULT 'pending',
    version     INTEGER NOT NULL DEFAULT 1,
    target_ref  TEXT NOT NULL DEFAULT '',
    message     TEXT NOT NULL DEFAULT '',
    started_at  TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX deployments_one_active_per_app
    ON deployments(app_id)
    WHERE status IN ('pending', 'deploying');

CREATE INDEX deployments_app_id_created_at_idx
    ON deployments(app_id, created_at DESC);

CREATE TABLE deployment_events (
    id            TEXT PRIMARY KEY,
    deployment_id TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    type          TEXT NOT NULL,
    message       TEXT NOT NULL,
    metadata      TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX deployment_events_deployment_id_created_at_idx
    ON deployment_events(deployment_id, created_at);

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
    id          TEXT PRIMARY KEY,
    team_id     TEXT REFERENCES teams(id),
    name        TEXT NOT NULL,
    token_hash  BLOB NOT NULL,
    scopes      TEXT NOT NULL,
    expires_at  TEXT,
    revoked_at  TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX api_tokens_hash_idx ON api_tokens(token_hash)
    WHERE revoked_at IS NULL;

CREATE TABLE idempotency_keys (
    key           TEXT NOT NULL,
    team_id       TEXT NOT NULL,
    endpoint      TEXT NOT NULL,
    request_hash  BLOB NOT NULL,
    response_code INTEGER NOT NULL,
    response_body TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at    TEXT NOT NULL,
    PRIMARY KEY (team_id, key)
);

CREATE INDEX idempotency_keys_expires_at_idx ON idempotency_keys(expires_at);

INSERT INTO teams (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'default');