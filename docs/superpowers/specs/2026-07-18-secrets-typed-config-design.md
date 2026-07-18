# Secrets-typed config (design)

| Field | Value |
|-------|-------|
| **Status** | **Approved (human)** — model accepted 2026-07-18; S1 implementation authorized |
| **Date** | 2026-07-18 |
| **Domain** | `docs/DOMAIN.md` |
| **Scope** | Design only: sensitivity model, redaction, encryption, clone policy, phased implementation. **No product code in this PR.** |
| **Queue** | `secrets-design` |
| **Unlocks** | Env clone (`env-clone` blocked); safer multi-tenant / SaaS story; redacted diffs & archaeology |

---

## Goal

Config today is **all plaintext strings** in `config_vars` / `shared_config_vars` and in `release.config_resolved`. That is fine for solo dogfood, but blocks:

1. **Environment clone** without blindly copying credentials into a new env.
2. **Safe archaeology** — release show / preview / logs must not spray secrets into terminals, CI logs, or agent context.
3. **Hosted / multi-tenant** — plaintext DB columns + full-value API responses are an unacceptable default.

**Design success criteria (this document):**

- A single recommended mental model that fits layered config, release snapshots, promote re-resolve, and future env clone.
- Explicit API/CLI redaction rules and storage rules.
- Phased implementation slices small enough for later ADM/feature PRs **after human review of this model**.
- Open product decisions listed with a recommended default (human can override before code).

**Implementation success criteria (later; not this PR):**

```bash
# Write a secret on the service layer
launchpad config set --secret DATABASE_URL=postgres://...
# List / get never prints the value
launchpad config get
# → DATABASE_URL=[secret]

# Diff / preview redact values for secret keys
launchpad diff
# → DATABASE_URL: [secret] → [secret]  (or set/unset without plaintext)

# Release archaeology redacts
launchpad releases show 3
# → config_resolved.DATABASE_URL=[secret]

# Worker still deploys real values into the target
```

---

## Approaches considered

### A. Sensitivity on existing config layers (recommended)

Add a **sensitivity** (or `value_type`) on each config entry in **shared** and **service** layers:

| Sensitivity | Meaning |
|-------------|---------|
| `plain` | Default. Value may appear in API/CLI/diff. |
| `secret` | Value is write-only from the API client's perspective; redacted on all control-plane reads; encrypted at rest. |

Same verbs (`config set/get/unset`, staged changes, layers). Secrets are **not** a second product surface.

**Pros:**

- Matches DX principle: same verbs everywhere; progressive disclosure via `--secret` / type on set.
- Layers, promote re-resolve, and changeset staging stay one pipeline.
- Unlocks clone policy with key metadata without a parallel store.
- Fits agent-native use: one config API with redaction, not two.

**Cons:**

- Touches every config read path (list, get, release DTO, preview/diff, possibly audit).
- `config_resolved` must remain deploy-capable internally while API views redact.
- Encryption key management is a new operational concern.

### B. Parallel secrets store + API

Separate tables and CLI: `launchpad secrets set KEY=VAL`, merge into resolved config at release.

**Pros:** Clear mental split; easier to bolt external SM later.  
**Cons:** Second dialect for “env vars”; layering/staging/promote must special-case two sources; worse agent DX. **Reject for v1.**

### C. External secret manager only (Vault / AWS SM / GCP SM)

Config values become refs (`sm://…`); Launchpad never stores secret material.

**Pros:** Best multi-tenant isolation; no encryption key in control plane.  
**Cons:** Blocks local stub dogfood; heavy ops for solo path; deferred platform work. **Reject as sole model**; allow as a **future** sensitivity or value kind (`secret_ref`) after core typing ships.

**Recommendation:** **A**. Optionally leave schema room for a future `secret_ref` kind without implementing it.

---

## Scope

### In scope (this design document)

- Domain model: sensitivity on config entries; redaction rules; release snapshot rules.
- API / CLI sketch for set/get/list/diff/preview/releases.
- Encryption-at-rest approach and key management for control plane.
- Env clone policy **as a consumer** of this model (clone itself remains a later feature).
- Interaction with promote, rollback, layered config, audit.
- Phased implementation plan (S0 design → S1 typing+redaction → S2 encryption → S3 clone unlock).
- DOMAIN.md section draft for human approval.

### Out of scope (this PR / design spike)

- Any Go implementation, migrations, or OpenAPI route changes.
- Implementing env clone.
- OIDC / RBAC that gates who can **write** secrets (phase 1 principals scopes stay; finer policy later).
- External secret managers.
- Workspace config layer (still deferred).
- Bindings (`${{ refs }}`) interaction beyond a short note.
- Encrypting non-config data (tokens already hashed; audit payloads separate).

### Deferred (future phases — do not half-build)

| Item | When |
|------|------|
| Env clone CLI/API | After S1 (min) or S2 (preferred) ships |
| `secret_ref` / external SM | Platform readiness; after hosted thesis solidifies |
| Per-role “reveal secret” | After OIDC + richer RBAC |
| Customer-managed keys (CMK) | Hosted multi-tenant |
| Secret rotation workflows | After basic typing + write path |
| Auto-detect secret keys by name heuristics alone | Never as sole mechanism; optional UX assist only |

---

## Domain impact

### Entities

| Entity | Change |
|--------|--------|
| Config entry (shared + service layers) | Add `sensitivity`: `plain` \| `secret` (default `plain`) |
| `ConfigVar` / shared row | Store encrypted ciphertext when `secret` (S2); always store sensitivity |
| Changeset change `config` / `shared_config` | Payload gains optional `sensitivity` (and value still present on write) |
| Release | `config_resolved` remains full resolved map **internally** for deploy; wire DTO redacts |
| Deployment / Target / Worker | **Unchanged** contract: `DeployRequest.Config map[string]string` plaintext at apply time |
| Environment | No new fields; clone policy later uses sensitivity metadata |

### Invariants to preserve

1. Resolution only at release create / push / promote / rollback re-resolve — live tables are inputs to snapshots, not to the worker.
2. Workers and targets never re-load live config for desired state; they use `release.config_resolved` (decrypted in process).
3. Promote **re-resolves** target env layers; does not copy source `config_resolved` (including secrets — target must have its own secret values).
4. Layer order unchanged: shared then service (service wins). Sensitivity is per **winning key entry** from the layer that provided the value (see resolution rules).
5. At most one open changeset per project; env pin unchanged.

### Invariants to add

1. **Write-only secrets on control-plane reads:** any API/CLI response that includes config values **must** redact secret values. Presence and key name are allowed; plaintext is not.
2. **Sensitivity is sticky on key:** once a key is `secret` in a layer, subsequent sets without an explicit demote remain `secret`. Unset removes the key.
3. **Plain → secret:** allowed on set (`--secret` or `"sensitivity":"secret"`). Value is re-written.
4. **Secret → plain:** allowed only with an **explicit** demote (`--plain` or sensitivity plain **and** a new value). Forbids accidental “make readable” without re-supplying the value through a conscious flag.
5. **Release immutability:** historical `config_resolved` ciphertext/plaintext in DB is immutable; only **presentation** redacts. Do not rewrite old releases when sensitivity of live keys changes.
6. **Deploy path exception:** internal service → worker → target may hold plaintext in memory; never log full secret values (slog redaction / no `fmt` of config maps).

### Resolution and sensitivity

```
resolved[key] = service_layer[key] if present else shared_layer[key]
sensitivity_resolved[key] = sensitivity of the layer entry that won
```

If shared has `secret` FOO and service has `plain` FOO, service wins as plain (override is total). Prefer documenting this; do not merge sensitivities.

Optional later: forbid plain override of a shared secret without `--force` — **not** in S1; keep rules simple.

### Env clone policy (consumer; implement with `env-clone`)

When cloning env A → env B:

| Key sensitivity in source | Clone behavior |
|---------------------------|----------------|
| `plain` | Copy value to same layer in B |
| `secret` | Create key in B with **no value** (or empty placeholder) and sensitivity `secret`, marked **needs_value** in clone report — operator must `config set --secret` before deploy |

Do **not** copy secret material between environments by default. Optional future: `--include-secrets` with dual confirmation for break-glass (hosted policy may disable).

This is why secrets typing **unblocks** clone without implementing clone in the same feature.

---

## API sketch

No new top-level resource. Extend existing config surfaces.

### Wire: config entry view

```json
{
  "PORT": { "value": "3000", "sensitivity": "plain" },
  "DATABASE_URL": { "value": null, "sensitivity": "secret", "set": true }
}
```

**Compatibility options:**

| Option | Behavior |
|--------|----------|
| **A1 (recommended)** | `GET …/config` stays `map[string]string` for plain keys only values; secret keys appear as `"***"` or omitted value with parallel metadata — **breaking if clients assume all values real**. Prefer explicit structured response versioning. |
| **A2** | New `GET …/config?view=typed` returns structured entries; default map redacts secrets as `"***"` (string sentinel). |
| **A3** | Always structured entries (breaking). |

**Recommendation:** **A2** for S1 — default map uses sentinel `"***"` for secret values (never the real string); `?view=typed` (or `Accept` / `meta=1`) returns `{value, sensitivity, set}`. Document that clients must not treat `"***"` as a deployable value. CLI always uses typed view internally.

**Sentinel choice:** `"***"` is simple; alternative empty string is ambiguous with real empty values. Prefer `"***"` **or** omit value and use typed view only for secrets. **Open for human:** sentinel vs typed-only for secrets on the default map.

**Recommended default for human approval:** default `GET` returns `map[string]string` where secret keys map to `"***"`; typed view is available for agents that need sensitivity flags.

### Write paths

| Method | Path | Notes |
|--------|------|-------|
| `PATCH` | `/v1/projects/{project}/config` | Body gains optional per-key `sensitivity`; default `plain` for new keys; sticky for existing secret keys |
| `POST` | `…/changeset/changes` | `config` / `shared_config` payload: `{ "key", "value"?, "sensitivity"? }` |

Unset: `value: null` removes key (sensitivity irrelevant after delete).

### Read paths (all redacting)

| Method | Path | Redaction |
|--------|------|-----------|
| `GET` | `…/config` | Secret values → `"***"` (or typed view) |
| `GET` | `…/config?layer=shared\|service` | Same |
| `GET` | `…/releases`, `…/releases/{n}` | `config_resolved` redacted |
| `GET` | `…/preview` | Diff `from`/`to` for secret keys redacted; ops still listed |
| `GET` | `…/audit` | Must not log raw secret values in event payloads going forward |

### Errors

- Demote secret → plain without explicit flag → `400` problem+json `code: secret_demote_required` + hints.
- Setting secret with empty value when empty disallowed → `400` (product: empty secrets **allowed** or not — recommend **allow** empty only for clone placeholder if we use empty; else use explicit `set: false` placeholder row).

### Auth

Workspace token scopes unchanged in S1. Future: `config:secret:write` scope optional; not required for design approval.

---

## CLI sketch

```bash
# Secret write (service layer default)
launchpad config set --secret DATABASE_URL='postgres://…'

# Shared secret
launchpad config set --shared --secret STRIPE_KEY=sk_…

# Explicit plain (default; demote requires --plain + value)
launchpad config set --plain WAS_SECRET=now-public

# Get: secrets show as [secret]
launchpad config get
launchpad config get --shared

# Staging still works
launchpad config set --secret FOO=bar
launchpad diff    # redacted
launchpad deploy --wait
```

`--now` behavior unchanged (stage + push). Sensitive-env `--yes` gate unchanged and independent.

---

## Schema sketch

### S1 — typing only (no encryption yet)

```sql
-- shared_config_vars + config_vars
ALTER TABLE config_vars ADD COLUMN sensitivity TEXT NOT NULL DEFAULT 'plain';
-- CHECK sensitivity IN ('plain', 'secret')

ALTER TABLE shared_config_vars ADD COLUMN sensitivity TEXT NOT NULL DEFAULT 'plain';
```

Changeset change JSON payload gains optional `sensitivity` (application-level; no schema if payload is JSON blob).

Releases: **no schema change** if `config_resolved` remains a JSON object of string values; redaction is serialization-time. Optionally store parallel `config_sensitivity` snapshot map on release for accurate historical redaction when live key types change:

```sql
-- recommended for correct archaeology
ALTER TABLE releases ADD COLUMN config_sensitivity TEXT; -- JSON map[string]string "plain"|"secret"
```

Without `config_sensitivity` on the release, historical show must either treat unknown as plain (leak risk if key later marked secret) or treat all values as secret (over-redact). **Recommend storing sensitivity snapshot with the release.**

### S2 — encryption at rest

```sql
-- value column becomes ciphertext for secrets; plain stays UTF-8 plaintext
-- OR always store: value_enc BLOB NULL, value_plain TEXT NULL with constraint one-of
```

**Recommended storage:**

| Sensitivity | `value` column |
|-------------|----------------|
| `plain` | UTF-8 plaintext (current) |
| `secret` | Base64(nonce\|\|ciphertext) AES-GCM; app-level envelope |

Key: `LAUNCHPAD_SECRETS_KEY` — 32-byte key, base64-encoded env var (or file ref later). Missing key → API refuses **new** secret writes; existing ciphertext fails closed on read/deploy with clear problem+json.

**SQLite local dev:** generate/persist key in `.launchpad/` or require env in README; dogfood script sets a fixed dev key.

**Postgres prod:** same env injection; KMS wrap later (CMK deferred).

Rotation: support `key_id` prefix on ciphertext (`v1:…`) for dual-key re-encrypt jobs — design only in S2 notes; implement minimal single-key first.

### Release `config_resolved`

| Layer | Behavior |
|-------|----------|
| DB | Full resolved values: plain as text; secrets as ciphertext **or** plaintext if encryption not yet enabled (S1) |
| Worker load | Decrypt secrets → `map[string]string` for `DeployRequest` |
| HTTP DTO | Redact using `config_sensitivity` snapshot |

S1 without encryption still **must** redact API responses (defense in depth for stolen tokens / chat logs); DB remains plaintext until S2 — acceptable for self-hosted MVP if documented.

---

## Target / worker impact

| Component | Impact |
|-----------|--------|
| `internal/target.Target` | **None** — still receives plaintext map |
| Deploy FSM / jobs | Load path decrypts; never log config map |
| K8s target | Continues writing K8s Secret data from map (already opaque to etcd via K8s); no control-plane change |
| Stub target | Same |

Do not push sensitivity types into the Target interface for S1/S2.

---

## Diff / preview / promote / rollback

### Preview & diff

For each config op involving a secret key (by **baseline or pending** sensitivity):

```json
{ "op": "change", "key": "DATABASE_URL", "from": "***", "to": "***", "sensitivity": "secret" }
```

Add/remove still show key names. Do not include plaintext in `summary` markdown either.

### Promote

Unchanged algorithm: new release in target env with **re-resolved** config. If target env lacks a secret value that the app needs, deploy gets whatever is in target layers (possibly missing key) — same as plain config today. Document that promote does not copy secrets from source.

### Rollback

Re-resolve vs bit-identical: existing DOMAIN open question stands. If rollback re-resolves, secret values come from **current** live layers (good). If ever bit-identical config rollback, need decrypt of historical snapshot — possible only if release stored values (encrypted).

---

## Audit and logs

- Audit events for config set may record **key names** and sensitivity, never values.
- `slog`: helper `RedactConfigMap` before any debug log.
- Problem+json and CLI recovery hints: no secret interpolation.

---

## Phased implementation (after human approval)

| Phase | Deliverable | Unlocks | Est. size |
|-------|-------------|---------|-----------|
| **S0** | This design + DOMAIN draft in PR | Human review gate | **This PR** |
| **S1** | Sensitivity column; write flags; redaction on all config/release/preview DTOs; CLI `--secret`/`--plain`; release sensitivity snapshot; tests | Safe DX; partial clone policy design validation | 1 medium PR |
| **S2** | AES-GCM at rest; `LAUNCHPAD_SECRETS_KEY`; migrate secret rows; worker decrypt | Safer multi-tenant DB | 1 medium PR |
| **S3** | Env clone using clone policy above | `env-clone` QUEUE item | Separate feature |

**Do not combine S1+S2+S3 in one PR** under ADM single-slice rule. S1 alone is valuable (redaction) even before encryption.

**Suggested QUEUE after human approves model:**

1. Promote implementation row `secrets-s1-typing-redaction` → `ready`
2. `secrets-s2-encryption` → `ready` (after S1)
3. Unblock `env-clone` when S1 (min) or S2 (preferred) is `shipped`

---

## Test strategy (for future implementation PRs)

### S1

- **Unit:** sensitivity sticky rules; demote requires flag; resolve sensitivity winner; redact helpers.
- **Store:** migration default `plain`; round-trip sensitivity.
- **Service:** GetConfig redacts; CreateRelease stores sensitivity snapshot; preview redacts.
- **API:** GET config/releases/preview never contain fixture secret plaintext.
- **CLI:** golden or unit on print format `[secret]`.
- **e2e-stub:** set secret, deploy, assert process still sees real value via stub inspect **if** stub exposes config; else service-level assert worker DeployRequest. **Must not** assert secret appears in API JSON.

### S2

- Encrypt/decrypt round-trip; refuse write without key; deploy with encrypted row.
- Store test with key in test main.

### Ladders

- L0 always; L1 e2e-stub when deploy path decrypt lands; L2 OpenAPI when response shapes change.

---

## DOMAIN.md draft (apply when implementing S1, or in approval follow-up)

Add under layered config / new subsection **Config sensitivity**:

> Config entries are `plain` (default) or `secret`. Secret values are redacted on all control-plane reads, snapshotted with sensitivity on each release, and (when encryption is enabled) stored ciphertext at rest. Resolution order is unchanged; the winning layer’s sensitivity applies. Promote re-resolves in the target environment and never copies secret material from the source release. Environment clone (planned) copies plain values and secret **keys** without values.

Also extend Open Questions:

> **Secret reveal:** Should any principal ever read back a secret value via API? Recommendation: no in v1; break-glass is reset in place only.

> **Default GET map sentinel:** `"***"` vs typed-only entries for secrets.

---

## Open questions (human model review)

Resolve before S1 implementation. Recommended defaults in **bold**.

1. **Default GET shape:** map with `"***"` sentinel vs breaking structured entries? → **map + `"***"` + optional typed view**
2. **Secret value reveal endpoint?** → **No** in v1 (write-only)
3. **Empty secret values** for clone placeholders vs explicit `needs_value` flag column? → **`needs_value` boolean or null value + `set:false` in typed view**; avoid magic empty strings if possible
4. **S1 before S2 acceptable** (redact without encrypt)? → **Yes** for self-hosted; document residual DB risk
5. **Plain override of shared secret** from service layer? → **Allow** (service wins completely); document
6. **Changeset diff across sensitivity change** (plain→secret same value)? → Show as sensitivity metadata change + redacted value
7. **CMK / external SM timeline?** → **Out of S1–S2**; note `secret_ref` future kind only

If the human rejects **A** in favor of **B** (parallel secrets API), stop and rewrite before any S1 code.

---

## Self-review checklist (ADM design gate)

| Check | Result |
|-------|--------|
| 1. No placeholders in requirements | Pass — open questions are explicit decisions with defaults, not TBD implementation steps |
| 2. Internal consistency | Pass — API, schema, release, worker paths agree on redact-at-edge / plaintext-at-deploy |
| 3. Single plan scope | Pass — this PR is design-only; S1/S2/S3 are separate future slices |
| 4. No DOMAIN contradiction | Pass — additive sensitivity; preserves resolve-at-release and promote re-resolve; DOMAIN draft included |
| 5. MVP boundary | Pass — **no implementation**; does not half-build multi-service, bindings, OIDC, or clone |
| 6. Recommended path recorded | Pass — A recommended; B/C rejected with rationale |
| 7. Test strategy present | Pass — for future S1/S2 PRs |

**Status:** `Approved (human)` — model accepted 2026-07-18 (PR #26 merged; recommended Approach A + defaults).  
**Implementation:** S1 (`secrets-s1`) and S2 (`secrets-s2`) are `ready` on the program queue. One vertical slice per PR.

---

## Approval

- [x] Design self-reviewed under ADM (design-only deliverable)
- [x] **Human reviewed and accepted model** (2026-07-18; recommended defaults)
- [x] Amendments from review folded into this spec (none — accepted as written)
- [x] S1 implementation PR may begin (QUEUE `secrets-s1`)
