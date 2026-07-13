# Launchpad DX Vision

| Field | Value |
|-------|-------|
| **Status** | Living document |
| **Date** | 2026-07-13 |
| **Related** | `docs/DOMAIN.md` (product model), `docs/FEATURE-DEVELOPMENT.md` |

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
7. **Anti-features** — no early GitOps reconciliation wars, Helm surface, multi-service theater, or OpenAPI-as-product.

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
| `launchpad doctor` | Healthz, token, project/env checks |
| Process `logs` (target-backed) | API `GET …/logs`, CLI `launchpad logs` |
| `launchpad inspect` | Project@env snapshot: pending, last deploy, processes |
| Release archaeology | `releases show N`, release↔release `diff` |
| Layered config (phase 2b) | Shared + service layers; resolve at release; `?layer=` |

---

## Domain roadmap (from DOMAIN.md)

| Phase | Focus | Status |
|-------|-------|--------|
| **2a** | Multi-env | **Shipped** |
| **2b** | Layered config | **Shipped** |
| **3** | Multi-service + ReleaseSet | Planned (deferred — do not half-build) |
| **4** | Bindings | Planned (deferred — do not half-build) |
| **5** | Promote | **Shipped** (primary service; re-resolve target config) |
| **6** | `launchpad.yaml` | Planned |

Do not half-build deferred phases. Each gets a spec.

---

## DX backlog

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
| Shell prompt awareness | Later |

### P2 — Trust and archaeology

| Idea | Status |
|------|--------|
| Rollback CLI | **Shipped** |
| `releases show N` | **Shipped** |
| Diff release↔release / env↔env | **Shipped** (release↔release); env↔env later |
| Unstage last mutation | Later |
| Sensitive-env confirmations | Later |

### P3 — Local parity and previews

| Idea | Notes |
|------|-------|
| `launchpad run` / env pull | Later |
| Ephemeral / PR environments | Later |
| Env clone | **Blocked** until secrets ≠ plain config |

### P4 — Agent surface

| Idea | Notes |
|------|-------|
| Server-side pending/diff preview | **Next** (after recovery hints) |
| Problem+json recovery hints | **Shipped** (`code` + `hints` extension) |
| MCP server | After core DX loop solid |
| Idempotency keys | Later |
| Recipes / templates | Later |

### Explicit non-goals (for now)

- Continuous GitOps reconciliation
- Helm as primary UX
- Full build system
- Multi-cloud target sprawl before multi-env DX is excellent

---

## Suggested sequencing (current)

1. ~~Logs + inspect + release archaeology~~ (**Shipped** — Wave 1 DX)
2. ~~Layered config 2b~~ (**Shipped** — Wave 2 domain)
3. ~~Promote~~ (**Shipped** — Wave 3)
4. ~~Problem+json recovery hints~~ (**Shipped**)
5. **Server-side pending/diff preview** — **Active / next** (agent surface)
6. **Multi-service** only after dogfood of 1–3 (hard deferred until then)

### Autonomous feature program

Agents may ship recommended options without per-feature design debates when authorized. Rules:

- Spec + plan still required for medium+ features; self-approve after checklist
- Mandatory review + `make test` / `build` / `vet` before PR
- Open PR to integration/spike branch when running a spike program; otherwise PR to `main` for human dogfood
- Hard stop: new deferred-boundary ambiguity, secrets/auth model, 3× verification failure

See program notes in session plans; update this section when cadence changes.

---

## How to use this doc

- When brainstorming a feature, check this list for related DX wins to fold in or explicitly defer.
- When a backlog item starts, write `docs/superpowers/specs/YYYY-MM-DD-<name>-design.md` and link it here.
- Update **Shipped** when PRs merge.

### Active / next

| Work | Spec |
|------|------|
| Promote (Wave 3) | `docs/superpowers/specs/2026-07-13-promote-design.md` (**Shipped**) |
| Problem+json recovery hints | `docs/superpowers/specs/2026-07-13-problem-recovery-hints-design.md` (**Shipped**) |
| Server-side pending/diff preview | *next* |
