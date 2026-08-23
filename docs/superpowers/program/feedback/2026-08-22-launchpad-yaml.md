# Persona feedback — launchpad.yaml v1

| Field | Value |
|-------|-------|
| **Branch** | `feat/launchpad-yaml` |
| **Scripts** | S1-equivalent: export after recipe deploy; apply to a new project; deploy dest; secret_keys needs_value |
| **Result** | Pass (`make e2e-stub` including `TestCLIManifestExportApply` and `TestCLIManifestSecretKeysNeedsValue`) |

## What I did

1. `launchpad new web-stub` + `deploy --wait` (source has a last-deployed image).
2. `launchpad export` → `launchpad.yaml` with `image:` and no secret plaintext.
3. Rewrite `project:` and `launchpad apply -f` into a new name → created project, staged image, no deploy job.
4. `launchpad diff` non-empty; dest `deploy --wait` succeeded.
5. Separate apply with `secret_keys.service: [DATABASE_URL]` → `needs_value`, no secret material.

## Friction

None P0. Apply prints `next: launchpad diff && launchpad deploy --wait` when it staged changes.

## Promotion

Same-PR: none. MCP remains the next QUEUE row.
