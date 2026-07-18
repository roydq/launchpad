# Feature Development Workflow

Repeatable process for long-horizon features on Launchpad. Designed for human developers and AI agents working across multiple sessions.

**Reference implementation:** MVP greenfield rewrite (`docs/superpowers/specs/2026-07-04-mvp-core-greenfield-design.md` + `docs/superpowers/plans/2026-07-04-mvp-core-greenfield.md`, branch `feat/mvp-core-greenfield`).

## Overview

```text
Design (spec)  →  Plan (tasks + commits)  →  Branch  →  Implement (layer by layer)  →  Verify  →  PR  →  Merge
     ↑                                                                                              ↓
     └──────────────── update spec/plan if scope changes ──────────────────────────────────────────┘
```

Artifacts live in `docs/superpowers/` so any session can resume without chat history:

| Artifact | Path | Purpose |
|----------|------|---------|
| Design spec | `docs/superpowers/specs/YYYY-MM-DD-<name>-design.md` | What and why — approved before code |
| Implementation plan | `docs/superpowers/plans/YYYY-MM-DD-<name>.md` | How — tasks, files, commits, verification |
| Domain changes | `docs/DOMAIN.md` | Update when the mental model changes |

Templates: `docs/superpowers/templates/`.

## When to use this workflow

| Scope | Workflow |
|-------|----------|
| Bug fix, typo, single-file tweak | Skip — branch + fix + PR |
| Small feature (1–2 layers, &lt;1 day) | Lightweight: plan optional, single `feat/` branch |
| Medium feature (vertical slice, days) | Full workflow — spec if behavior changes, plan required |
| Large feature (multiple subsystems) | Full workflow + stacked PRs (see [Stacked PRs](#stacked-prs)) |

**Rule:** If the work crosses an [MVP deferred boundary](AGENTS.md#mvp-scope-boundaries), write or update a spec and `docs/DOMAIN.md` before writing implementation code.

## Phase 1: Design

### 1.1 Explore context

Read in order:

1. `docs/DOMAIN.md`
2. Relevant existing specs in `docs/superpowers/specs/`
3. `docs/DESIGN.md` for architecture constraints

Invoke `/launchpad-domain` when changing entities, schema, or API shapes.

### 1.2 Write the spec

Copy `docs/superpowers/templates/spec-template.md` to:

```text
docs/superpowers/specs/YYYY-MM-DD-<feature-name>-design.md
```

Include:

- Goal and success criteria (CLI commands, API calls, or test assertions)
- Approaches considered with a recommendation
- Scope: in / out / deferred
- Domain impact (new entities, lifecycle changes, invariants)
- API and schema sketch (paths, tables — not full implementation)
- Test strategy

**Gate:** Human approval before implementation. For agent sessions, present the design and wait for explicit approval.

**Autonomous mode (when user authorizes ADM):** agent may self-approve the recommended approach after the [spec self-review checklist](AUTONOMOUS-MODE.md#spec-self-review-self-approve-gate) (no open TBDs, single-plan scope, no DOMAIN contradictions, MVP boundary respected). Still write the spec and plan to `docs/superpowers/`. Still open a PR for human dogfood; do not force-merge unless asked.

Canonical protocol: [`docs/AUTONOMOUS-MODE.md`](AUTONOMOUS-MODE.md). Invoke `/launchpad-autonomous`.

Program files: [`docs/superpowers/program/QUEUE.md`](superpowers/program/QUEUE.md) (work selection), [`IDEAS.md`](superpowers/program/IDEAS.md) (scout), [`PERSONA-SCRIPTS.md`](superpowers/program/PERSONA-SCRIPTS.md) (synthetic user). Brief backlog note: `docs/DX-VISION.md` → *Autonomous feature program*.

### 1.3 Update domain doc (if needed)

If the feature changes the product mental model, update `docs/DOMAIN.md` in the same PR series as the spec (can be a separate `docs:` commit on the feature branch).

## Phase 2: Plan

Copy `docs/superpowers/templates/plan-template.md` to:

```text
docs/superpowers/plans/YYYY-MM-DD-<feature-name>.md
```

### Plan structure

Decompose by **Go dependency order** (bottom-up):

```text
domain → store (migrations + repos) → service → target/worker → api → cli → cleanup/docs
```

Each task must specify:

- Files to create, modify, or delete
- Checkbox steps
- Verification commands
- **Exact commit message** (one logical layer per commit)

### Plan status header

Track progress in the plan file itself:

```markdown
> **Status: In Progress** — branch `feat/my-feature`, started 2026-07-07
```

Update to `Completed` with merge date when done. Completed plans are kept as historical records.

## Phase 3: Branch

**Never implement on `main`.**

### Single feature branch

```bash
git checkout main
git pull
git checkout -b feat/<short-name>
```

Branch naming:

| Prefix | Use |
|--------|-----|
| `feat/` | New capability |
| `fix/` | Bug fix |
| `docs/` | Documentation only |

### Isolated worktree (recommended for agents)

Keeps `main` checkout clean and supports parallel work:

```bash
git worktree add .worktrees/feat-<short-name> -b feat/<short-name>
cd .worktrees/feat-<short-name>
mise exec -- make test   # verify clean baseline
```

`.worktrees/` is gitignored. Remove when done:

```bash
cd <repo-root>
git worktree remove .worktrees/feat-<short-name>
git worktree prune
```

## Phase 4: Implement

Work through plan tasks in order. After **each task**:

1. Run verification (see [Verification gates](#verification-gates))
2. Commit with the message specified in the plan
3. Mark task checkboxes complete in the plan file
4. Commit plan progress updates periodically (`docs: update plan progress`)

### Commit conventions

Follow existing history — present tense, scoped prefix:

```text
feat(domain): add environment promotion types
feat(store): add promotion_state table and repo
feat(service): implement promote-to-staging
feat(api): add POST /v1/projects/{id}/promote endpoint
feat(cli): add launchpad promote command
docs: add multi-environment promotion spec
chore: remove deprecated single-env shims
```

Rules:

- **One logical layer per commit** when possible
- **Tests in the same commit** as the code they cover
- **Migrations + store repos** land together
- **No drive-by refactors** unrelated to the feature
- **No artifacts:** `*.db`, `.env`, `bin/` (already gitignored)

### Agent execution modes

| Mode | When | How |
|------|------|-----|
| Inline | Small plans, single session | Execute tasks directly; commit per task |
| Subagent per task | Medium plans in one session | Fresh subagent per plan task + review between tasks |
| Stacked PRs | Large plans with independent subsystems | `/execute-plan` on the design doc |
| **Autonomous (ADM)** | User authorizes low-input multi-step work | Self-approve recommended path; subagents; docs sync; verification ladder; hard stops — [`docs/AUTONOMOUS-MODE.md`](AUTONOMOUS-MODE.md) |

Invoke `/launchpad-feature` for interactive feature work. Invoke `/launchpad-autonomous` when the user authorizes ADM.

## Phase 5: Verify

### Every commit

```bash
mise exec -- make test
mise exec -- make build
mise exec -- go vet ./...
```

### Deploy-flow changes

If `internal/service`, `internal/jobs`, or `internal/target` changed:

```bash
# API + worker running, then:
scripts/smoke-stub.sh
```

See `.grok/skills/launchpad-dev/SKILL.md` for local setup.

### Before PR

- [ ] All plan tasks checked off
- [ ] Plan status updated
- [ ] No debug code or commented-out blocks
- [ ] Spec and plan committed on the feature branch

## Phase 6: Integrate

### Single PR

```bash
git push -u origin feat/<short-name>
gh pr create --title "feat: <short description>" --body "$(cat <<'EOF'
## Summary
- <bullet 1>
- <bullet 2>

## Spec
docs/superpowers/specs/YYYY-MM-DD-<name>-design.md

## Test plan
- [ ] mise exec -- make test
- [ ] mise exec -- make build
- [ ] <feature-specific verification>
EOF
)"
```

Keep the worktree (if used) until PR review is complete.

### Stacked PRs

Use when the plan has **independent subsystems** that benefit from incremental review:

```text
main
 └── feat/<name>-domain-store     (PR 1)
      └── feat/<name>-service      (PR 2, base = PR 1 branch)
           └── feat/<name>-api-cli  (PR 3, base = PR 2 branch)
```

Each PR in the stack must:

- Pass verification independently
- Be reviewable without reading the entire feature
- Merge in order (bottom of stack first)

Use Graphite (`gt`) if available, or plain-git parent branches. The `/execute-plan` skill automates this from a design doc with a PR DAG.

## Resuming across sessions

Agents and humans resume from **files**, not conversation:

1. Read the plan file — check status header and unchecked boxes
2. Read the spec for intent and scope boundaries
3. `git log --oneline main..HEAD` — see what's already committed
4. `mise exec -- make test` — confirm branch is healthy
5. Continue from the first unchecked task

If scope changed mid-flight, update the spec and plan **before** writing more code.

## Quick reference

| Question | Answer |
|----------|--------|
| Where do specs go? | `docs/superpowers/specs/YYYY-MM-DD-<name>-design.md` |
| Where do plans go? | `docs/superpowers/plans/YYYY-MM-DD-<name>.md` |
| What branch prefix? | `feat/`, `fix/`, or `docs/` |
| Commit granularity? | One Go layer per commit |
| Verify command? | `mise exec -- make test && make build && go vet ./...` |
| Domain questions? | `/launchpad-domain` + `docs/DOMAIN.md` |
| Local dev / smoke? | `/launchpad-dev` |
| Start a feature? | `/launchpad-feature` |
| Autonomous / low-input program? | `/launchpad-autonomous` + `docs/AUTONOMOUS-MODE.md` |

## Related docs

- `AGENTS.md` — agent conventions and architecture
- `docs/DOMAIN.md` — domain model (north star)
- `docs/DESIGN.md` — control-plane architecture
- `docs/AUTONOMOUS-MODE.md` — experimental autonomous development protocol
- `.grok/skills/launchpad-feature/SKILL.md` — agent skill for this workflow
- `.grok/skills/launchpad-autonomous/SKILL.md` — ADM skill