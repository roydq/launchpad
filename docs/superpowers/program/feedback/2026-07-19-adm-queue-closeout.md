# Persona feedback — ADM queue closeout (2026-07-19)

| Field | Value |
|-------|-------|
| **Branch** | `adm/queue-2026-07-19` |
| **Scripts** | S1 (via `launchpad new`), S2-ish (config/secret), S3-ish (env clone), S4 light (doctor) |
| **Result** | Pass after fix #38 |

## What I did

1. Started API + worker with `LAUNCHPAD_SECRETS_KEY`, bootstrap token, stub target.
2. `launchpad new list` / `launchpad new web-stub adm-web` → staged image + PORT.
3. `launchpad prompt` → `adm-web@dev`.
4. `launchpad diff` / `deploy --wait` → success.
5. `config set --secret DATABASE_URL=…` + plain EXTRA; deploy.
6. `env clone dev staging` → plain keys cloned; `needs_value: DATABASE_URL`.
7. `env use staging` (initially broken — stayed on dev due to project-local shadow).
8. Fixed via #38; re-verified path conceptually.

## Friction

| Sev | Finding | Action |
|-----|---------|--------|
| P1 | Project-local `environment: dev` from `new` blocked `env use` | **Fixed** #38 |
| P2 | Clone has no sticky secret placeholder rows | QUEUE `clone-secret-placeholder` |
| P2 | No e2e for new/clone | QUEUE e2e rows |
| P0 | Postgres never opened via `sql.Open("postgres")` | **Fixed** #34 (pgx) |

## Pass bar

Day-one recipe path works; multi-env clone is usable with secrets awareness; doctor green.
