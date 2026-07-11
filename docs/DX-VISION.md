# Launchpad DX Vision

| Field | Value |
|-------|-------|
| **Status** | Living document |
| **Date** | 2026-07-11 |
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
| Release snapshot as deploy truth | Worker applies release only |
| Atomic changeset push | Materialize + job in one TX |
| Stub + Kubernetes targets | Pluggable runtime |
| Tiered e2e (stub + kind) | CI confidence |

---

## Domain roadmap (from DOMAIN.md)

| Phase | Focus | DX unlock |
|-------|-------|-----------|
| **2a** | **Multi-env (next)** | `env create/use`; config + deploy per env |
| **2b** | Layered config | workspace / shared / service layers |
| **3** | Multi-service + ReleaseSet | Coordinated multi-service deploys |
| **4** | Bindings | `${{ refs }}` service linking |
| **5** | Promote | staging → production as a product moment |
| **6** | `launchpad.yaml` | CI / agent import-export |

Do not half-build deferred phases. Each gets a spec.

---

## DX backlog (beyond / beside the phase table)

Priorities are **guidance**, not tickets. Promote items into specs when starting work.

### P0 — Feedback loop (high leverage)

| Idea | Why |
|------|-----|
| `deploy --wait` / `--follow` | Enqueue → hope is not a product |
| Job/deployment progress in CLI | Alive loop |
| Process `logs` (target-backed) | Debug without leaving Launchpad |

### P1 — Context and gravity

| Idea | Why |
|------|-----|
| Multi-env context stack (`env use`, ambient env) | **Next feature (2a)** |
| Project-local config (`.launchpad/` or `launchpad.toml`) | Auto-context on `cd` |
| Shell prompt / status line awareness | Always know `project@env` |
| `launchpad doctor` | API, auth, target reachability |
| `launchpad inspect` | One page: context, pending, running release, target |

### P2 — Trust and archaeology

| Idea | Why |
|------|-----|
| Diff release↔release | Snapshot model already supports this |
| Diff env↔env (running / last release) | Multi-env payoff |
| `releases show N` full snapshot | “What’s in v12?” |
| Rollback as first-class CLI | New release from prior snapshot |
| Unstage last mutation | Undo culture beyond full `reset` |
| Confirmations for sensitive envs | Safe defaults without killing solo flow |

### P3 — Local parity and previews

| Idea | Why |
|------|-----|
| `launchpad run` / env pull for local process | Local ↔ remote config parity |
| Ephemeral / PR environments | Category-defining; DOMAIN already has `ephemeral` |
| Env bootstrap copy (clone config shape) | **Blocked on secrets:** do not clone config until secrets are stored/typed differently from plain config values |

### P4 — Agent and integration surface

| Idea | Why |
|------|-----|
| Launchpad MCP server | Agents drive create/stage/deploy without curl |
| Server-side pending/diff preview API | Reuse beyond CLI |
| Idempotency keys | Safe agent retries |
| Problem+json with recovery hints | Actionable errors for humans and agents |
| Recipes / templates (`--recipe web`) | Zero blank-page syndrome |

### Explicit non-goals (for now)

- Continuous GitOps reconciliation
- Helm as primary UX
- Full build system
- Multi-cloud target sprawl before multi-env DX is excellent

---

## Suggested sequencing (DX-obsessed)

1. **Multi-env 2a** — context + per-env config/deploy ([spec](superpowers/specs/2026-07-11-multi-env-design.md))
2. **Deploy wait/follow + basic logs** — make multi-env *feel* real
3. **Layered config 2b *or* rollback** — pick by dogfood pain
4. **MCP + project-local context** — agent + repo gravity
5. **Promote, bindings, multi-service, yaml** — composition once env model is solid

---

## How to use this doc

- When brainstorming a feature, check this list for related DX wins to fold in or explicitly defer.
- When a backlog item starts, write `docs/superpowers/specs/YYYY-MM-DD-<name>-design.md` and link it here.
- Update **Shipped** when PRs merge; move ideas out of the backlog.

### Active / next

| Work | Spec |
|------|------|
| Multi-environment (phase 2a) | `docs/superpowers/specs/2026-07-11-multi-env-design.md` |
