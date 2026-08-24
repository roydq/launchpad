# Persona feedback — MCP server

| Field | Value |
|-------|-------|
| **Branch** | `feat/mcp-server` |
| **Scripts** | MCP S1-equivalent + stdio token smoke + S4-ish missing token (unit) |
| **Result** | Pass (`make e2e-stub` including `TestMCPApplyDeployInspect`, `TestMCPApplySecretNeedsValue`, `TestMCPStdioListProjects`) |

## What I did

1. MCP `create_project` (no saved CLI context) → stub project.
2. `apply_manifest` with image document → staged; `preview` showed pending; no release until deploy.
3. `deploy` omitting `wait` (default true) → `wait.status=succeeded`.
4. `inspect.last_deploy.version` set; `get_manifest` without environment included a second env.
5. `apply_manifest` yaml `secret_keys` → `needs_value` contains `DATABASE_URL`; no secret plaintext.
6. Stdio: `launchpad mcp` via CommandTransport + minted `LAUNCHPAD_TOKEN` → `list_projects`.

## Friction

None P0. `json.RawMessage` on environment/changeset first failed go-sdk output schema (byte array); fixed same-PR by JSON round-trip.

Missing token: unit test `list_projects` returns `LAUNCHPAD_TOKEN is not set` plus `export LAUNCHPAD_TOKEN=...` hint.

## Promotion

Same-PR: RawMessage output schema fix. Scout → IDEAS only (HTTP MCP, procfile tool).
