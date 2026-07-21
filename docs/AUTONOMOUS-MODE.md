# Autonomous Development Mode (ADM)

| Field | Value |
|-------|-------|
| **Status** | Experimental — process protocol (docs + skill + program files) |
| **Date** | 2026-07-21 |
| **Related** | `docs/FEATURE-DEVELOPMENT.md`, `docs/DX-VISION.md`, `.grok/skills/launchpad-autonomous/SKILL.md`, `docs/superpowers/program/` |

Long-running, low-input agent work: pick recommended paths, implement planned features, keep docs in sync, expand verification when risk warrants it, use subagents, and stop cleanly when human judgment is required.

This is **process**, not product code. Refined from real ADM runs (2026-07-18–21); keep experiments cheap while the product is early.

### Program files (Approach B)

| Path | Purpose |
|------|---------|
| [`docs/superpowers/program/QUEUE.md`](superpowers/program/QUEUE.md) | Ordered work items + status for ADM selection |
| [`docs/superpowers/program/IDEAS.md`](superpowers/program/IDEAS.md) | Edge cases / future ideas (not a build commitment) |
| [`docs/superpowers/program/PERSONA-SCRIPTS.md`](superpowers/program/PERSONA-SCRIPTS.md) | Synthetic user dogfood scripts |
| [`docs/superpowers/program/feedback/`](superpowers/program/feedback/) | Persona write-ups + optional session logs |
| [`scripts/adm-status`](../scripts/adm-status) | Snapshot: ready queue, integration tip, dirty worktrees, open PRs |

---

## When to use

| Situation | Mode |
|-----------|------|
| Human wants a multi-step feature with little back-and-forth | ADM (this doc) |
| One small fix or single-file change | Skip — normal branch + PR |
| Ambiguous product direction / deferred-boundary design | Spec with human gate (not self-approve) |
| Human wants design debate | Normal `/launchpad-feature` interactive flow |

**Authorization required.** The user must explicitly enable ADM for a session (or a named budget), e.g.:

> “ADM authorized: implement server-side X; up to 1 PR; no merge; stop on domain ambiguity.”  
> “ADM authorized: queue drain on `adm/queue-YYYY-MM-DD`; merge into integration; final PR to main.”

Without that phrase (or equivalent), do **not** self-approve designs or run multi-feature loops.

---

## Principles

1. **Files over chat** — resume from specs, plans, `QUEUE.md`, git log, and this protocol.
2. **Recommended path only** — write 2–3 approaches in the spec; pick the one that matches `DOMAIN.md` + MVP + DX-VISION sequencing; do not re-debate in chat.
3. **Spec + plan still required** for medium+ work — autonomy does not skip artifacts.
4. **PRs for humans** — open PRs for dogfood; **never force-merge to `main`** unless the user explicitly allows it. Integration-branch merges are a separate knob (see [Named modes](#named-modes)).
5. **Hard stops beat cleverness** — escalate rather than invent product policy.
6. **Docs are part of the feature** — DOMAIN, OpenAPI, DX-VISION, plan checkboxes, and queue status stay current in the same PR series.
7. **Subagents for focus** — fresh implementer context per task when the plan is multi-task; orchestrator coordinates only.
8. **Queue over vibes** — pick work from `QUEUE.md` (aligned with DX-VISION); park discoveries in `IDEAS.md` without silent scope expansion.
9. **Isolation over thrash** — implementers work in leased branches/worktrees; orchestrator does not reset or force-push work it does not own.
10. **Definition of Done before “shipped”** — a QUEUE row is not done until its acceptance criteria are green (see [Definition of Done](#definition-of-done-queue-rows)).

---

## Named modes

At authorization, pick **one** mode (or assume **single-feature** if unspecified). Modes fix the knobs that previously had to be re-derived from chat.

| Mode | Typical ask | Max features / PRs | Merge policy | Stop when |
|------|-------------|--------------------|--------------|-----------|
| **single-feature** | “ADM: ship X; 1 PR; no merge” | 1 | Open PR only; no merge | PR open or hard stop |
| **integration-stack** | “ADM: branch off main; PRs against `adm/…`; merge as you go; final PR to main” | N (user budget or default 3) | Merge feature PRs **into the integration branch only**; open (do not force-merge) final PR to `main` unless user allows | Budget done or hard stop |
| **queue-drain** | “ADM: remaining ready items until only human-review items left” | Unlimited `ready` rows | Same as integration-stack (integration branch required) | No `ready` / implementable rows left (only `deferred` / `blocked`) or hard stop |

### Integration branch (stack + drain)

1. Create `adm/queue-YYYY-MM-DD` (or user-named) from current `main`; push and use as **PR base** for all feature PRs.
2. Feature branches: `feat/<id>` from the **current tip of the integration branch** (after prior merges).
3. After each green feature PR: **merge into the integration branch** (user authorized this mode), update `QUEUE.md` on integration (status + PR link).
4. End of session: open **one final PR** `adm/…` → `main` with a stack summary and test plan. Do **not** merge that PR unless the user explicitly allows merge to main.

### Mode selection examples

```text
ADM authorized: single-feature — ship env clone; 1 PR; no merge
ADM authorized: integration-stack — base adm/queue-2026-07-21; up to 3 PRs; merge into adm only
ADM authorized: queue-drain — unlimited ready; merge into adm/queue-…; final PR to main; stop when only deferred remain
```

---

## Session budget (set at start)

Record mode + knobs (defaults depend on mode):

| Knob | single-feature | integration-stack | queue-drain |
|------|----------------|-------------------|-------------|
| Max features / PRs | 1 | 3 if unspecified | Unlimited `ready` |
| Max wall time | Session lifetime | Session lifetime | Session lifetime |
| Merge policy | Open PR only | Merge → integration only | Merge → integration only |
| Scope | Named feature or top `ready` | Named set or next N ready | All `ready` until drain stop |
| Scout | Append `IDEAS.md` | Same | Same |
| Persona | When CLI/deploy UX changes | Same + once per stack before final PR | Same + once per stack before final PR |

Always restate the resolved budget in the first agent turn after authorization.

---

## Roles

| Role | Who | Responsibility |
|------|-----|----------------|
| **Orchestrator** | Parent session | Mode/budget, gates, dispatch, docs checklist, PR, stop/continue; **owns only integration branch + merge/PR actions** |
| **Designer** | Parent or plan subagent | Spec + plan; recommended approach; self-review checklist; DoD bullets |
| **Implementer** | Subagent in **worktree** | One plan task / layer; tests; commit + **push** checkpoints; never touch other agents’ branches |
| **Spec reviewer** | Read-only subagent | Code matches spec; no extras |
| **Quality reviewer** | Read-only subagent | Conventions, clarity, test quality |
| **Docs sync** | Parent or implementer | DOMAIN / OpenAPI / DX-VISION / README / QUEUE as needed |
| **Scout** | Explore / plan | Edge cases and future ideas → `IDEAS.md` only |
| **Persona user** | Execute-capable subagent | Scripts in `PERSONA-SCRIPTS.md`; feedback under `program/feedback/` |

### Parallelism and isolation (mandatory for multi-agent)

- **One implementer per branch** and **one worktree per implementer** (`.worktrees/feat-<name>` or equivalent).
- Orchestrator **must not** `git reset --hard`, force-push, or rewrite a feature branch while an implementer is assigned to it.
- Treat QUEUE **Branch / PR** as a **lease**: only the owner edits that branch.
- Reviewers after implementer finishes that task (or plan milestone).
- Scout / persona only after green verification for the feature slice.
- Independent features: separate worktrees **only** when the mode allows multi-feature (stack or drain).

### Fast path vs required review

| Change surface | Review |
|----------------|--------|
| 1–2 file, fully specified plan task (e.g. pure store method + test) | Orchestrator may use **one combined** spec+quality review |
| `internal/service`, `api`, `jobs`, `worker`, `cli`, `domain`, deploy path | **Two-stage required**: spec compliance, then quality (or one subagent with both checklists if resources are tight — both checklists still run) |

---

## Definition of Done (QUEUE rows)

Every **`ready`** row that will be implemented under ADM needs **acceptance criteria** before code starts. Prefer writing them in:

1. The design spec success criteria, and/or  
2. The plan “Final verification” section, and/or  
3. A short **DoD** blurb in the QUEUE **Spec / notes** cell.

**Minimum DoD for any implementable row:**

- [ ] Spec self-review passed (or human-approved design already linked)
- [ ] Plan tasks checked off (or single-commit scope documented)
- [ ] L0 green: `mise exec -- make test && make build && go vet ./...`
- [ ] Triggered ladder levels (see [Verification](#verification-ladder)) green
- [ ] Docs sync checklist satisfied for what changed
- [ ] QUEUE status → `pr-open` with PR link; on merge to integration/main → `shipped` + DX-VISION if product-facing

**Do not mark `shipped` for partial surface** (e.g. API without snapshot fields, or “completions” without the CLI command working). Incomplete follow-ups go to `IDEAS.md` or a new QUEUE row — not silent half-ship.

**Slice size:** Prefer one vertical, shippable PR per QUEUE ID. Do not land an entire multi-slice design doc in one thrash-prone PR.

---

## End-to-end loop

```text
User authorizes ADM + named mode + budget
  → Orchestrator reads DOMAIN, DX-VISION, FEATURE-DEVELOPMENT, QUEUE.md, this doc
  → Optional: scripts/adm-status
  → (stack/drain) create integration branch from main if needed
  → Select work item (user-named or top ready QUEUE row / Active / next)
  → Update QUEUE status → designing; fill DoD if missing
  → Designer: approaches → recommended → spec + plan
  → Spec self-review checklist → self-approve OR hard stop
  → Feature worktree + branch from integration (or main in single-feature mode)
  → QUEUE → implementing (lease Branch column)
  → For each plan task:
        Implementer (worktree) → L0 verify → review(s) → commit → push checkpoint → check off plan
  → Docs sync if model/routes/CLI changed
  → Verification ladder (L0–L4 as triggered)
  → Persona dogfood when CLI/deploy UX in scope (PERSONA-SCRIPTS)
  → Scout pass → append IDEAS.md (no code from ideas alone)
  → Push + open PR (link spec; test plan; DoD); QUEUE → pr-open
  → (stack/drain) merge feature PR into integration; QUEUE + DX-VISION update on integration
  → Update DX-VISION status if applicable
  → (stack/drain) next ready item OR final PR integration → main
  → Stop (mode stop condition, budget, or hard stop)
```

Resume across sessions: `QUEUE.md` + plan checkboxes + `git log main..HEAD` (or `main..adm/…`) + green `make test` + optional `program/feedback/SESSION-*.md`.

---

## Spec self-review (self-approve gate)

Before implementation without human design approval, **all** must pass:

1. **No placeholders** — no TBD/TODO in requirements; no vague “handle errors appropriately.”
2. **Internal consistency** — API sketch, schema, and success criteria agree.
3. **Single plan scope** — one vertical slice; not multi-service + secrets + OIDC in one PR.
4. **No DOMAIN contradiction** — if mental model changes, DOMAIN update is in the plan.
5. **MVP boundary** — does not half-build AGENTS.md deferred items; if it would, stop or write a design-only spike for human review.
6. **Recommended path recorded** — rejected approaches listed briefly in the spec.
7. **Test strategy present** — unit and, if deploy-path, e2e-stub or smoke.
8. **DoD / acceptance criteria present** — CLI/API/test assertions someone can check without chat history.

Mark the spec status, e.g. `Approved (autonomous)` or `Approved (self-approve — ADM)`.

If any check fails → **hard stop** and present the open question to the human.

---

## Recommended path selection

When choosing an approach (and when choosing *what* to build next):

1. Prefer an in-progress plan / `implementing` or `pr-open` fix over a new QUEUE row.
2. Else prefer top **`ready`** row in `QUEUE.md` that matches the user budget (design-only rows stay design-only).
3. Cross-check DX-VISION **Active / next** and Track A/B over C/D until dogfood is boring.
4. Prefer the option that preserves existing CLI verbs and ambient env context.
5. Prefer smaller shippable PR over completeness theater.
6. Never invent multi-service, bindings, or OIDC as drive-by scope; secrets implementation only with an approved design path.
7. Never implement `deferred` / `blocked` QUEUE rows without human override + spec.

### Drain stop condition

In **queue-drain** mode, stop when every remaining active-queue row is `deferred` or `blocked` (or design-only awaiting human). Do **not** start deferred items “to keep going.”

---

## Queue hygiene

| Event | QUEUE action |
|-------|----------------|
| Start work | Status → `designing` / `implementing`; set Branch lease |
| Open feature PR | Status → `pr-open`; link PR |
| Merge into integration (stack/drain) | Keep `pr-open` until integrated, then `shipped` (or `shipped` on integration with PR# note) |
| Merge to main | Confirm `shipped`; short entry under Recently shipped; update DX-VISION if product-facing |
| Hard stop | Leave status accurate; note decision needed in Spec / notes |

**Update QUEUE on every feature merge** into the integration branch — not only at session closeout.

### Deferred / blocked: decision needed

For `deferred` and `blocked` rows, Spec / notes should include a one-line **Decision needed:** so the next human session starts cleanly, e.g.:

> `deferred` — **Decision needed:** accept OIDC IdP priority (Azure AD vs Google vs generic first) before design draft.

After a queue-drain, prefer a short session report under `program/feedback/YYYY-MM-DD-adm-session.md` listing deferred decisions (see [Session log](#session-log-optional)).

### Scout vs ship gate (after each feature)

Explicitly choose one for each finding:

1. **Same-PR fix** — regression or gap this feature introduced; DoD not met otherwise.  
2. **Promote to QUEUE** — human asked, pre-authorized class (e.g. P0 from persona), or unblocks dogfood and fits MVP.  
3. **IDEAS only** — everything else.

Do not silently expand scope from scout/persona output.

---

## Docs sync checklist

After implementation, before PR:

| Changed | Update |
|---------|--------|
| Entities, invariants, lifecycles | `docs/DOMAIN.md` |
| HTTP routes / schemas | `docs/openapi.yaml` + `make openapi-check` |
| Product sequencing / shipped table | `docs/DX-VISION.md` |
| Program work item | `docs/superpowers/program/QUEUE.md` status |
| User-facing run paths | `README.md` if the 60s path or env vars change |
| Plan progress | Checkboxes + status header in plan file |

Docs commits may be `docs:` on the feature branch; DOMAIN model changes belong in the same PR series as the code.

Closeout / QUEUE bulk status updates: commit on the **integration branch** (or a docs branch based on it) after features are green — never rewrite QUEUE from a conflicting dirty worktree that is not the integration tip.

---

## Verification ladder

| Level | When | What |
|-------|------|------|
| **L0** | Every task / commit | `mise exec -- make test && make build && go vet ./...` |
| **L1** | `internal/service`, `jobs`, `target`, deploy CLI | **`make e2e-stub` required** (not optional) for those surfaces |
| **L1.5** | Once per integration stack / before final PR to main | Persona **S1** (and S4 if CLI error paths touched); write `program/feedback/` |
| **L2** | New/changed HTTP routes | Update OpenAPI + `make openapi-check` |
| **L3** | Multi-env, promote, config resolution, secrets-aware flows | Existing e2e coverage or add cases in the same PR |
| **L4** | Before feature PR and before final integration→main PR | All triggered levels + plan “Final verification” + no debug leftovers |

**Expand verification when:** persona or reviewer finds an uncovered failure mode; auth/config/deploy semantics change; a bug fix needs a regression test. Expansion lands in the same PR when practical; otherwise note a follow-up in the PR body (not silent drop).

**Env-gated integration tests** (e.g. Postgres via `LAUNCHPAD_TEST_DATABASE_URL`) pay for dialect bugs; prefer adding focused env-gated tests over full-matrixing every package.

Always use `mise exec --` for Go commands (see AGENTS.md).

**CI:** Prefer CI green on feature and integration-branch PRs (`.github/workflows` already runs on `pull_request`). Do not treat laptop-only green as final for stack/drain modes.

---

## Hard stops (human required)

Stop ADM and report clearly:

1. **Deferred-boundary or DOMAIN fork** — secrets model, multi-service, OIDC shape, half-built deferred APIs.
2. **Spec self-review failure** — open TBD or two equally valid architectures.
3. **3× verification failure** on the same task after distinct fix attempts.
4. **P0 product break** from persona dogfood that one retry cannot fix.
5. **Destructive / shared actions** outside policy — force-push of published history, `reset --hard` of others’ work, merge to main without permission, dropping data.
6. **Budget / mode stop** — max PRs, time, or drain complete (only deferred/blocked left).
7. **Unexpected repo state** — unfamiliar dirty tree or branch that may be human WIP.
8. **Branch ownership conflict** — another agent/session holds the feature lease; do not steal the branch.

On hard stop: leave branch/worktree intact, summarize state, list the decision needed, do not thrash.

---

## Persona user (synthetic dogfood)

**Canonical scripts:** [`docs/superpowers/program/PERSONA-SCRIPTS.md`](superpowers/program/PERSONA-SCRIPTS.md)

When CLI/deploy UX is in scope (or user requests dogfood):

1. Run at least **S1** (day-one path) and **S4** (deliberate mistakes) after green verification.
2. Add S2/S3 when config or multi-env/promote is touched.
3. Write `docs/superpowers/program/feedback/YYYY-MM-DD-<feature>.md`.
4. Summarize findings in the PR body.

**Stack/drain:** before the final integration → main PR, run persona **S1** once against the integration tip (L1.5) even if individual features already dogfooded.

Promotion: same-PR fixes for regressions this feature introduced; otherwise `IDEAS.md` / QUEUE per [Scout vs ship gate](#scout-vs-ship-gate-after-each-feature). Do not treat persona output as automatic multi-feature scope expansion.

If dogfood cannot run, record `blocked` + reason in feedback — do not fake a pass.

---

## Scout (edge cases / future ideas)

**Canonical log:** [`docs/superpowers/program/IDEAS.md`](superpowers/program/IDEAS.md)

After a green feature slice (or at session end):

1. Append rows for edge cases, doc gaps, and follow-ups (date, source, severity, track).
2. Mention notable P0/P1 items in the PR body.
3. Promote to `QUEUE.md` only if human asked, pre-authorized class (e.g. auto-queue P0s), or the idea unblocks current dogfood **and** fits MVP.

Do **not** silently implement scout ideas.

---

## Subagent guidance (orchestrator)

### Worktrees (required for implementers)

```bash
# From repo root, integration tip checked out (or main in single-feature mode):
git fetch origin
git worktree add .worktrees/feat-<name> -b feat/<name> origin/adm/queue-YYYY-MM-DD
# implementer cwd = .worktrees/feat-<name>
```

After the feature PR merges into integration, the orchestrator may remove the worktree. Do not delete a worktree while the implementer is still running.

### Task packet (every implementer dispatch)

Give the implementer a **complete packet** — do not make them rediscover the plan:

1. **Base** — exact branch/SHA or `origin/adm/…` / `origin/main`
2. **Worktree path** and **feature branch name** (lease)
3. **Allowed paths** — packages/files they may edit
4. **Full plan task text** (or whole plan path + task IDs)
5. **Verify commands** — at least L0; L1 if deploy path
6. **Commit message(s)** from the plan
7. **Push checkpoint** — commit and `git push -u origin feat/<name>` after each accepted task
8. **PR base** — `adm/…` or `main` per mode
9. **Forbidden** — other agents’ branches; force-push; `reset --hard` of shared history; editing main

### Checkpoints and budgets

- Implementers **commit and push** after each plan task so progress is not only a dirty tree.
- On `BLOCKED` / `NEEDS_CONTEXT`: supply context or escalate; do not infinite-retry the same prompt.
- Prefer a **turn/time budget** per implementer; on exhaustion, orchestrator takes over or hard-stops with state summary.
- Spec compliance review **before** code quality review when both run.
- Mark plan checkboxes after each accepted task; commit plan progress on the feature branch.

Reuse patterns from superpowers `subagent-driven-development` and bundled `execute-plan` personas when available; Launchpad layer-commit rules still win.

---

## Session log (optional)

For multi-hour or multi-session stack/drain runs, write:

`docs/superpowers/program/feedback/SESSION-YYYY-MM-DD.md`

Suggested contents: mode, integration branch, PRs merged, next ready ID, hard stops, deferred decisions. Optional but cheap; pairs with `scripts/adm-status`.

---

## Relationship to other docs

| Doc | Role |
|-----|------|
| `docs/FEATURE-DEVELOPMENT.md` | Canonical feature workflow; ADM is an **authorization mode** on top |
| `docs/DX-VISION.md` | What to build next; backlog truth |
| `docs/DOMAIN.md` | Product mental model — do not contradict |
| `AGENTS.md` | Toolchain, layout, MVP cut line |
| `.grok/skills/launchpad-feature` | Interactive / default feature skill |
| `.grok/skills/launchpad-autonomous` | Agent entrypoint for this protocol |
| `.grok/skills/launchpad-dev` | Build, test, smoke, local processes |
| `.grok/skills/launchpad-domain` | Entity / API / invariant questions |

---

## What ADM is not

- Not a license to skip tests, OpenAPI, DoD, or DOMAIN updates.
- Not overnight unattended CI (no durable agent host yet).
- Not auto-merge to **main** (integration merges only when mode authorizes).
- Not multi-service or deferred-boundary implementation without an explicit design path.
- Not a substitute for human product taste on hard stops.
- Not a license for the orchestrator to thrash implementer branches.

---

## Evolution

Current = protocol + skill + program files + light status script. Learned from ADM runs:

- Shared-workspace thrash → mandatory worktrees + branch leases  
- Implicit “merge into adm / drain queue” budgets → named modes  
- Half-shipped surfaces → Definition of Done  
- Stale QUEUE → update on every merge  

When runs are boringly useful, consider next:

- Scheduler-backed continuation (session ticks, like `/pr-babysit`)
- Standalone `/launchpad-persona` skill
- Launchpad MCP for API dogfood without shell curl
- Durable overnight agent host (Approach C)
- CI matrix explicitly including `adm/**` if needed beyond default `pull_request`

Update this file when policy changes; keep experiments cheap while the product is early.
