CREATE TABLE teams (
    id         UUID PRIMARY KEY,
    name       TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE apps (
    id                     UUID PRIMARY KEY,
    team_id                UUID NOT NULL REFERENCES teams(id),
    name                   TEXT NOT NULL,
    stack                  TEXT NOT NULL DEFAULT 'container',
    target_type            TEXT NOT NULL DEFAULT 'kubernetes',
    target_config          JSONB NOT NULL DEFAULT '{}',
    status                 TEXT NOT NULL DEFAULT 'created',
    active_deployment_id   UUID,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at             TIMESTAMPTZ,
    UNIQUE (team_id, name)
);

CREATE TABLE config_vars (
    app_id     UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, key)
);

CREATE TABLE process_types (
    id         UUID PRIMARY KEY,
    app_id     UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    command    TEXT NOT NULL DEFAULT '',
    quantity   INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (app_id, name)
);

CREATE TABLE builds (
    id          UUID PRIMARY KEY,
    app_id      UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    source_ref  TEXT NOT NULL DEFAULT '',
    image_ref   TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'pending',
    logs_url    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE releases (
    id              UUID PRIMARY KEY,
    app_id          UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    build_id        UUID REFERENCES builds(id),
    version         INT NOT NULL,
    config_snapshot JSONB NOT NULL DEFAULT '{}',
    image_ref       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    description     TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (app_id, version)
);

CREATE TABLE deployments (
    id          UUID PRIMARY KEY,
    app_id      UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    release_id  UUID NOT NULL REFERENCES releases(id),
    status      TEXT NOT NULL DEFAULT 'pending',
    version     INT NOT NULL DEFAULT 1,
    target_ref  TEXT NOT NULL DEFAULT '',
    message     TEXT NOT NULL DEFAULT '',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX deployments_one_active_per_app
    ON deployments(app_id)
    WHERE status IN ('pending', 'deploying');

CREATE INDEX deployments_app_id_created_at_idx
    ON deployments(app_id, created_at DESC);

CREATE TABLE deployment_events (
    id            UUID PRIMARY KEY,
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    type          TEXT NOT NULL,
    message       TEXT NOT NULL,
    metadata      JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX deployment_events_deployment_id_created_at_idx
    ON deployment_events(deployment_id, created_at);

CREATE TABLE jobs (
    id            UUID PRIMARY KEY,
    type          TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   UUID NOT NULL,
    status        TEXT NOT NULL DEFAULT 'queued',
    payload       JSONB NOT NULL,
    attempt       INT NOT NULL DEFAULT 0,
    max_attempts  INT NOT NULL DEFAULT 5,
    run_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    leased_until  TIMESTAMPTZ,
    leased_by     TEXT,
    last_error    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX jobs_poll_idx ON jobs (status, run_at)
    WHERE status IN ('queued', 'failed');

CREATE INDEX jobs_lease_reclaim_idx ON jobs (leased_until)
    WHERE status = 'leased';

CREATE TABLE api_tokens (
    id          UUID PRIMARY KEY,
    team_id     UUID REFERENCES teams(id),
    name        TEXT NOT NULL,
    token_hash  BYTEA NOT NULL,
    scopes      TEXT NOT NULL,
    expires_at  TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX api_tokens_hash_idx ON api_tokens(token_hash)
    WHERE revoked_at IS NULL;

CREATE TABLE idempotency_keys (
    key           TEXT NOT NULL,
    team_id       UUID NOT NULL,
    endpoint      TEXT NOT NULL,
    request_hash  BYTEA NOT NULL,
    response_code INT NOT NULL,
    response_body JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (team_id, key)
);

CREATE INDEX idempotency_keys_expires_at_idx ON idempotency_keys(expires_at);

INSERT INTO teams (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'default');