# Secrets S1: typing + redaction — implementation plan

> **Status: In Progress** — branch `feat/secrets-s1`

**Goal:** Sensitivity `plain`|`secret` on config layers; redact control-plane reads; CLI `--secret`/`--plain`; release sensitivity snapshot. No encryption (S2).

**Spec:** `docs/superpowers/specs/2026-07-18-secrets-typed-config-design.md` (Approved human)

**Branch:** `feat/secrets-s1`

---

## Task 1: Domain

- [ ] Sensitivity constants, sentinel, ConfigChangePayload.Sensitivity
- [ ] Release.ConfigSensitivity
- [ ] Redact helpers + unit tests
- [ ] Commit: `feat(domain): config sensitivity types and redaction`

## Task 2: Store

- [ ] Migration 005 sensitivity columns + releases.config_sensitivity
- [ ] List/merge/resolve with sensitivity; CreateRelease/scanRelease
- [ ] Tests
- [ ] Commit: `feat(store): config sensitivity schema and repos`

## Task 3: Service

- [ ] Stage sensitivity; materialize + sticky merge; resolve snapshot on release
- [ ] GetConfig redacts; preview/diff redacts; release plan sensitivity
- [ ] Tests
- [ ] Commit: `feat(service): sticky sensitivity and redacted reads`

## Task 4: API + OpenAPI + client

- [ ] GET redaction; view=typed; stage sensitivity; release DTO redaction
- [ ] OpenAPI update + openapi-check
- [ ] apiclient updates
- [ ] Commit: `feat(api): redact config and release secrets`

## Task 5: CLI + docs

- [ ] `--secret` / `--plain`; display `[secret]` for sentinel
- [ ] DOMAIN / DX-VISION / QUEUE / OpenAPI docs
- [ ] Commit: `feat(cli): config --secret and --plain`

## Task 6: Verify + PR

```bash
mise exec -- make test && make build && go vet ./...
mise exec -- make openapi-check
make e2e-stub  # deploy path still uses plaintext internally
```
