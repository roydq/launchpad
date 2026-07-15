-- Principals (users + service accounts), membership, token link, release attribution, audit.

CREATE TABLE principals (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL,
    display_name TEXT NOT NULL,
    email        TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'active',
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE workspace_members (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    role         TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (workspace_id, principal_id)
);

CREATE INDEX workspace_members_principal_idx ON workspace_members(principal_id);

ALTER TABLE api_tokens ADD COLUMN principal_id TEXT REFERENCES principals(id);

ALTER TABLE releases ADD COLUMN created_by_principal_id TEXT REFERENCES principals(id);
ALTER TABLE releases ADD COLUMN created_by_token_id TEXT REFERENCES api_tokens(id);

CREATE TABLE audit_events (
    id            TEXT PRIMARY KEY,
    workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    principal_id  TEXT REFERENCES principals(id),
    token_id      TEXT REFERENCES api_tokens(id),
    action        TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   TEXT NOT NULL,
    project_name  TEXT NOT NULL DEFAULT '',
    detail        TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX audit_events_workspace_created_idx ON audit_events(workspace_id, created_at DESC);
