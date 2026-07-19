# Secrets S2: AES-GCM at rest — implementation plan

> **Status: In progress** — branch `feat/secrets-s2`  
> **Mode:** ADM (authorized 2026-07-18). S1 shipped (PR #28).

**Goal:** Encrypt `secret` config values at rest (live layers + release `config_resolved`) with AES-GCM; decrypt for resolve/deploy; refuse new secret writes without `LAUNCHPAD_SECRETS_KEY`.

**Architecture:** `internal/secrets` box (32-byte key from base64 env). Store seals/opens secret values with `v1:` ciphertext prefix. Domain/API redaction unchanged. No Target interface change. No schema migration (TEXT value column holds ciphertext).

**Spec:** `docs/superpowers/specs/2026-07-18-secrets-typed-config-design.md` § S2

**Branch:** `feat/secrets-s2`

---

## Task 1: Crypto package

**Files:**
- Create: `internal/secrets/box.go`, `internal/secrets/box_test.go`

- [ ] Parse base64 32-byte key; Encrypt/Decrypt with AES-GCM; `v1:` prefix; IsCiphertext
- [ ] Unit tests round-trip + bad key + wrong ciphertext
- [ ] Commit: `feat(secrets): AES-GCM box for config values`

## Task 2: Store seal/open

**Files:**
- Modify: `internal/store/store.go`, `internal/store/config_vars.go`, `internal/store/releases.go`
- Create: `internal/store/secrets_codec.go`, tests

- [ ] `Store.WithSecrets(*secrets.Box)`; nil box = no encryption (plain-only OK)
- [ ] Merge: encrypt when sensitivity=secret (fail without key)
- [ ] List/resolve: decrypt ciphertext secrets; legacy plaintext secrets pass through
- [ ] CreateRelease: seal secret keys in config_resolved JSON; scanRelease: open
- [ ] Tests: encrypt round-trip, refuse write without key, deploy path plaintext in memory
- [ ] Commit: `feat(store): encrypt secret config at rest`

## Task 3: Wire API, worker, e2e, service tests

**Files:**
- Modify: `cmd/api/main.go`, `cmd/worker/main.go`, `scripts/e2e-stub.sh`
- Modify: service/store tests that write secrets

- [ ] Load `LAUNCHPAD_SECRETS_KEY` in API + worker; same key both processes
- [ ] e2e-stub exports fixed test key to API + worker
- [ ] Update sensitivity tests with test box
- [ ] Commit: `feat(cmd): wire LAUNCHPAD_SECRETS_KEY into api and worker`

## Task 4: Docs + queue

**Files:**
- Modify: `docs/DOMAIN.md`, `docs/DX-VISION.md`, `docs/superpowers/program/QUEUE.md`, `README.md`
- Create: persona feedback

- [ ] DOMAIN: S2 storage row; residual risk note for unencrypted legacy rows until rewrite
- [ ] QUEUE: secrets-s1 → shipped; secrets-s2 → pr-open; env-clone still blocked until S2 preferred ships (or note min S1)
- [ ] README env table
- [ ] Commit: `docs: secrets S2 encryption and queue`

## Task 5: Verify + PR

```bash
mise exec -- make test && make build && go vet ./...
mise exec -- make openapi-check   # no route shape change expected
make e2e-stub
```

- [ ] Persona light dogfood if CLI path exercises secrets
- [ ] Scout → IDEAS.md
- [ ] Open PR; no merge
