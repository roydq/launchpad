# Postgres matrix in CI

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve — ADM) |
| **Date** | 2026-07-19 |
| **Scope** | CI job + env-gated store integration test against Postgres |
| **Queue** | `postgres-ci` |

## Goal

Catch SQL dialect / `rebind()` / `FOR UPDATE SKIP LOCKED` bugs that SQLite `:memory:` tests miss.

```bash
# Local (optional)
docker run -d --name lp-pg -e POSTGRES_PASSWORD=launchpad -e POSTGRES_DB=launchpad -p 5432:5432 postgres:16
export LAUNCHPAD_TEST_DATABASE_URL=postgres://postgres:launchpad@localhost:5432/launchpad?sslmode=disable
mise exec -- go test ./internal/store/ -run TestPostgres -count=1
```

## Approach

**A (recommended):** Separate CI job `test-postgres` with `services: postgres:16`; env-gated `TestPostgres*` in `internal/store` (skip if env unset so default `make test` stays SQLite-only).

**B:** Matrix entire unit suite — too invasive (all tests hardcode `:memory:`).

**C:** Only e2e-stub against postgres — slower; still valuable later.

## Scope

- CI service + job
- `TestPostgresMigrateBootstrapLease` covering migrate, project bootstrap, job lease
- Makefile target `test-postgres` optional
- Docs: QUEUE / DX-VISION Track B

## Self-review

Pass — engineering confidence only; no product surface.
