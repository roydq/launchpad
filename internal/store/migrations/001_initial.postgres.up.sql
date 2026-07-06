CREATE TABLE workspaces (
    id         UUID PRIMARY KEY,
    name       TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE projects (
    id              UUID PRIMARY KEY,
    workspace_id    UUID NOT NULL REFERENCES workspaces(id),
    name            TEXT NOT NULL,
    primary_service TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'created',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (workspace_id, name)
);

CREATE TABLE environments (
    id            UUID PRIMARY KEY,
    project_id    UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    target_type   TEXT NOT NULL DEFAULT 'kubernetes',
    target_config JSONB NOT NULL DEFAULT '{}',
    ephemeral     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, name)
);

CREATE TABLE services (
    id         UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL DEFAULT 'application',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, name)
);

CREATE TABLE processes (
    id         UUID PRIMARY KEY,
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    command    TEXT NOT NULL DEFAULT '',
    quantity   INT NOT NULL DEFAULT 1,
    expose     TEXT NOT NULL DEFAULT 'none',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (service_id, name)
);

CREATE TABLE config_vars (
    service_id     UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    environment_id UUID NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    key            TEXT NOT NULL,
    value          TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (service_id, environment_id, key)
);

CREATE TABLE releases (
    id               UUID PRIMARY KEY,
    service_id       UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    version          INT NOT NULL,
    artifact_ref     TEXT NOT NULL,
    config_resolved  JSONB NOT NULL DEFAULT '{}',
    process_snapshot JSONB NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL DEFAULT 'pending',
    description      TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (service_id, version)
);

CREATE TABLE deployments (
    id             UUID PRIMARY KEY,
    service_id     UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    environment_id UUID NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    release_id     UUID NOT NULL REFERENCES releases(id),
    status         TEXT NOT NULL DEFAULT 'pending',
    version        INT NOT NULL DEFAULT 1,
    target_ref     TEXT NOT NULL DEFAULT '',
    message        TEXT NOT NULL DEFAULT '',
    started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX deployments_one_active_per_service_env
    ON deployments(service_id, environment_id)
    WHERE status IN ('pending', 'deploying');

CREATE INDEX deployments_service_env_created_at_idx
    ON deployments(service_id, environment_id, created_at DESC);

CREATE TABLE changesets (
    id          UUID PRIMARY KEY,
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'open',
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX changesets_one_open_per_project
    ON changesets(project_id)
    WHERE status = 'open';

CREATE TABLE changeset_changes (
    id           UUID PRIMARY KEY,
    changeset_id UUID NOT NULL REFERENCES changesets(id) ON DELETE CASCADE,
    service_id   UUID REFERENCES services(id),
    service_name TEXT NOT NULL DEFAULT '',
    type         TEXT NOT NULL,
    payload      JSONB NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX changeset_changes_changeset_id_idx ON changeset_changes(changeset_id);

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
    id           UUID PRIMARY KEY,
    workspace_id UUID REFERENCES workspaces(id),
    name         TEXT NOT NULL,
    token_hash   BYTEA NOT NULL,
    scopes       TEXT NOT NULL,
    expires_at   TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX api_tokens_hash_idx ON api_tokens(token_hash)
    WHERE revoked_at IS NULL;

INSERT INTO workspaces (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'default');