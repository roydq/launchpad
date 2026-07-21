# Launchpad DX Vision

| Field | Value |
|-------|-------|
| **Status** | Living document |
| **Date** | 2026-07-14 |
| **Related** | `docs/DOMAIN.md`, `docs/FEATURE-DEVELOPMENT.md`, `docs/superpowers/specs/2026-07-14-prod-readiness-design.md` |

North star: **the mise of runtime application management** — zero ceremony for a solo engineer, composable depth for large systems. Crush DX so the control plane feels inevitable and invisible.

**Bar:** Someone should go from zero → running on stub in under a minute, switch environments without renaming anything, see exactly what will change, and trust the release history.

---

## Principles

1. **One name, many places** — project identity is stable; env/target are ambient context.
2. **Same verbs everywhere** — `config`, `image`, `diff`, `deploy` do not grow a second dialect per environment.
3. **Diff before trust** — pending vs release, release vs release, env vs env over time.
4. **Safe by default, fast when solo** — prod can be careful later; dev never feels heavy.
5. **Agent-native** — humans and coding agents are equal-class users of the API/CLI.
6. **Progressive disclosure** — day-one path stays short; power appears when needed.
7. **Anti-features** — no early GitOps reconciliation wars, Helm surface, multi-service theater, or OpenAPI-as-product before dogfood.

---

## Shipped (foundation)

| Item | Notes |
|------|-------|
| Project / env / service / process model | Correct hierarchy vs app-per-env |
| Implicit staging CLI | Stage by default; `diff` / `status` / `reset` / `deploy` |
| Multi-env (phase 2a) | Ambient `X-Launchpad-Environment`; `env create/list/use`; changeset pin |
| Release snapshot as deploy truth | Worker applies release only |
| Atomic changeset push | Materialize + job in one TX |
| Stub + Kubernetes targets | Pluggable runtime |
| Tiered e2e (stub + kind) | CI confidence |
| `deploy --wait` / `--timeout` | Poll job until terminal |
| Project-local `.launchpad/config` | Walk-up context; `launchpad context` |
| Rollback | New release from prior version; config re-resolved per env |
| Promote (Wave 3) | Artifact + process topology; **re-resolve** target config |
| `launchpad doctor` | Healthz, token, project/env checks |
| Process `logs` (target-backed) | API `GET …/logs`, CLI `launchpad logs` |
| `launchpad inspect` | Project@env snapshot: pending, last deploy, processes |
| Release archaeology | `releases show N`, release↔release `diff` |
| Layered config (phase 2b) | Shared + service layers; resolve at release; `?layer=` |
| Problem+json recovery hints | `code` + `hints[]` on API errors; structured apiclient errors |
| CLI recovery hint printing | Multi-line `recovery:` from `*APIError` |
| Sensitive-env CLI gate | `production`/`prod` deploy/promote/rollback need `--yes` |
| e2e multi-env promote | Stub CI asserts target config re-resolution |

---

## Domain roadmap (from DOMAIN.md)

| Phase | Focus | Status |
|-------|-------|--------|
| **2a** | Multi-env | **Shipped** |
| **2b** | Layered config | **Shipped** |
| **3** | Multi-service + ReleaseSet | Planned (deferred — do not half-build) |
| **4** | Bindings | Planned (deferred — do not half-build) |
| **5** | Promote | **Shipped** (primary service) |
| **6** | `launchpad.yaml` | Planned |

Do not half-build deferred phases. Each gets a spec.

---

## Program tracks (world-class OSS → hosted)

Four parallel tracks. **A + B lead** until daily dogfood is boring. Surfaces and SaaS build on a stable API.

### Track A — Core DX loop (product)

| Item | Status |
|------|--------|
| Promote + layered config dogfood | **Shipped** (main) |
| Problem+json recovery hints | **Shipped** |
| CLI prints recovery hints | **Shipped** |
| Sensitive-env confirmations (`production` + `--yes`) | **Shipped** (CLI policy) |
| Server-side pending/diff preview | **Shipped** |
| Diff env↔env | **Shipped** |
| Unstage last mutation | **Shipped** |
| Recipes / `launchpad new` templates | **Shipped** (ADM #32) |
| Process commands + Procfile | **Designed** — QUEUE `process-commands` ([runtime-target-depth](superpowers/specs/2026-07-20-runtime-target-depth-design.md)) |
| Portable health / deploy readiness | **Designed** — QUEUE `deploy-health` |
| Target extensions (resources, annotations, …) | **Designed** — QUEUE `target-extensions` |
| MCP server | After core loop solid |

### Track B — Confidence (engineering)

| Item | Status |
|------|--------|
| Unit + service invariants | **Shipped** |
| e2e-stub happy path (CI) | **Shipped** |
| e2e multi-env + promote + config re-resolution | **Shipped** (`TestPromoteReResolvesTargetConfig`) |
| Failure-path e2e (409, pin mismatch) | **Shipped** |
| OpenAPI + CI contract drift | **Shipped** (`docs/openapi.yaml` + `make openapi-check`) |
| Postgres matrix in CI | **Shipped** — `test-postgres` + pgx driver fix (ADM #34) |
| Target conformance suite (stub/k8s/…) | **Shipped** — stub conformance (ADM #35) |
| Worker lease/supersede stress tests | **Shipped** — concurrent lease + reclaim (ADM #36) |
| Release-immutable config materialization (K8s) | **Designed** — QUEUE `release-config-materialization` |

### Track C — Surfaces (CLI → TUI → web → docs)

| Item | Status |
|------|--------|
| CLI verbs + wait + context | **Shipped** |
| Completions / man pages | Later |
| TUI (inspect / deploy / releases) | Later — same apiclient |
| Docs site (get-started + mental model) | Later |
| `examples/` + 60s path CI | **Shipped** (`examples/hello-stub`, `make example-60s`) |
| Web dashboard MVP | Later — OpenAPI + auth first |

### Track D — Platform readiness (slow burn → hosted)

| Item | Status |
|------|--------|
| Workspace-scoped tokens | **Shipped** |
| Principals + membership + audit (phase 1) | **Shipped** (service accounts on tokens; release `created_by`; `GET /v1/audit`) |
| OIDC (Azure AD / Google / generic) | After phase 1 principals |
| Secrets-typed config | **S1+S2 shipped** — typing/redaction + AES-GCM at rest. [spec](superpowers/specs/2026-07-18-secrets-typed-config-design.md) |
| Idempotency keys | Later |
| Deployment events / SSE | Later |
| HA workers / packaging | Later |
| Richer RBAC / env policy | After roles + OIDC |
| **Hosted control plane** | Future: same binary; BYO data plane |

**Hosted thesis:** we run the control plane; customers point environments at their clusters (or free-tier stub). Do not build multi-region billing or dual domain models early.

---

## DX backlog (detail)

### P0 — Feedback loop

| Idea | Status |
|------|--------|
| `deploy --wait` | **Shipped** |
| Process `logs` (target-backed) | **Shipped** |
| Job progress lines | Covered by `--wait` |

### P1 — Context and gravity

| Idea | Status |
|------|--------|
| Multi-env context stack | **Shipped** |
| Project-local config | **Shipped** |
| `launchpad doctor` | **Shipped** |
| `launchpad inspect` | **Shipped** |
| Shell prompt awareness | **Shipped** — `launchpad prompt` / `shell-init` (ADM #33) |

### P2 — Trust and archaeology

| Idea | Status |
|------|--------|
| Rollback CLI | **Shipped** |
| Promote CLI | **Shipped** |
| `releases show N` | **Shipped** |
| Diff release↔release | **Shipped** |
| Diff env↔env | **Shipped** (`--from-env` / `--to-env`) |
| Unstage last mutation | **Shipped** (`launchpad unstage`) |
| Sensitive-env confirmations | **Shipped** (`--yes` on production) |

### P3 — Local parity and previews

| Idea | Notes |
|------|-------|
| Server-side pending/diff preview | **Shipped** — `GET …/preview` |
| `launchpad run` / env pull | Later |
| Ephemeral / PR environments | Later |
| Env clone | **Shipped** — `env clone` / `POST …/clone` (ADM #37) |

### P4 — Agent surface

| Idea | Notes |
|------|-------|
| Problem+json recovery hints | **Shipped** |
| CLI surfaces hints | **Shipped** |
| MCP server | After core DX loop solid |
| Idempotency keys | Later |
| Recipes / templates | **Shipped** — `launchpad new` / `new list` |

### Explicit non-goals (for now)

- Continuous GitOps reconciliation
- Helm as primary UX
- Full build system
- Multi-cloud target sprawl before multi-env DX is excellent
- Multi-service theater before single-service dogfood is boring

---

## Suggested sequencing (current)

1. ~~Wave 1 DX + 2b config + Promote + recovery hints~~ (**Shipped**)
2. ~~Prod-readiness dogfood~~ (**Shipped** — e2e promote, CLI hints, production `--yes`)
3. ~~Server-side pending/diff preview~~ (**Shipped**)
4. ~~Failure-path e2e + OpenAPI contract + examples/60s~~ (**Shipped**)
5. Secrets S1+S2 — **shipped** (PRs #28, #29) — [spec](superpowers/specs/2026-07-18-secrets-typed-config-design.md)
6. ADM remaining ready queue (base `adm/queue-2026-07-19`): recipes → shell-prompt → Track B → env-clone
7. Surfaces (docs site → TUI → dashboard) only with stable API
8. OIDC (after principals phase 1 dogfood)
9. **Multi-service** only after dogfood of core loop

### Autonomous feature program

Agents may ship recommended options without per-feature design debates when the user **explicitly authorizes** Autonomous Development Mode (ADM).

**Canonical protocol:** [`docs/AUTONOMOUS-MODE.md`](AUTONOMOUS-MODE.md) · skill: `/launchpad-autonomous`

Summary (see protocol for full rules):

- **Named modes:** single-feature (default) · integration-stack · queue-drain
- Spec + plan for medium+; self-approve only after checklist + **Definition of Done**
- Implementers in **worktrees**; branch lease in QUEUE; push checkpoints
- Verification: L0 always; **e2e-stub required** for service/jobs/target/deploy CLI; persona S1 once per stack before final PR to main
- Merge to **main** only when user allows; stack/drain may merge into `adm/*` only
- Hard stops: deferred-boundary, self-review fail, 3× verify fail, branch ownership conflict, …
- Program files + `scripts/adm-status`; persona/scout → feedback/ and IDEAS without silent scope expansion

Experimental while the project is early; protocol updated from real runs (2026-07-21). When ADM starts an item, prefer updating **QUEUE** status and link the spec here under Active / next.

---

## How to use this doc

- When brainstorming a feature, check tracks A–D and fold into or defer explicitly.
- When a backlog item starts, write `docs/superpowers/specs/YYYY-MM-DD-<name>-design.md` and link it here.
- Update **Shipped** when PRs merge.

### Active / next

| Work | Spec / queue |
|------|----------------|
| **ADM integration** | `adm/queue-2026-07-20` — runtime depth + polish; final PR → main |
| Queue ready work | **Empty** — only deferred (OIDC, MCP, multi-service, bindings, launchpad.yaml) |
| Runtime target depth | **Shipped** slices 1–4 — [design](superpowers/specs/2026-07-20-runtime-target-depth-design.md) |

Ordered agent work: [`docs/superpowers/program/QUEUE.md`](superpowers/program/QUEUE.md).
