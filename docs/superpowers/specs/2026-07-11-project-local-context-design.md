# Project-Local Context

| Field | Value |
|-------|-------|
| **Status** | Approved (lightweight) |
| **Date** | 2026-07-11 |
| **Integration** | `feat/dx-stack` |

## Goal

Walking into a repo auto-selects project (and optional env) without global `use`.

```bash
cd ~/code/my-api   # has .launchpad/config
launchpad status   # uses project from file
```

## Design

**File:** `.launchpad/config` (JSON), walk parents from cwd up to filesystem root.

```json
{ "project": "my-api", "environment": "staging" }
```

**Precedence (high → low):**

| Key | Order |
|-----|--------|
| Project | `LAUNCHPAD_PROJECT` → project-local → `~/.launchpad/config` |
| Environment | `LAUNCHPAD_ENV` → project-local → `~/.launchpad/config` → `dev` |

**CLI:**

- `launchpad use <project>` still writes **global** `~/.launchpad/config`
- `launchpad env use` same (global); optional later: `--local` to write project file
- `launchpad context` prints resolved project, env, and which sources won

## Non-goals

- TOML/YAML dual format (JSON only for 1st cut)
- Auto-create `.launchpad` on `use`
- Git-aware discovery beyond parent walk
