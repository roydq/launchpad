---
name: launchpad-feature
description: >
  Launchpad feature development workflow. Use when starting a new feature,
  long-horizon task, or multi-session implementation. Covers spec → plan →
  branch → layer commits → PR. Triggers on "new feature", "feature branch",
  "implementation plan", "write spec", "long horizon", "start feature",
  "feature development", "/launchpad-feature".
---

# Launchpad Feature Development

**Read first:** `docs/FEATURE-DEVELOPMENT.md` (canonical workflow).

## Quick start

When asked to build a feature:

1. **Classify scope** — bug fix (skip workflow), small/medium/large feature (full or lightweight workflow)
2. **Check MVP boundary** — if crossing deferred scope in `AGENTS.md`, spec + `docs/DOMAIN.md` update required first
3. **Follow phases below** — do not skip to code without an approved spec for non-trivial work

## Phase 1: Design

```
Read docs/DOMAIN.md
     ↓
Copy docs/superpowers/templates/spec-template.md
     → docs/superpowers/specs/YYYY-MM-DD-<name>-design.md
     ↓
Present design → get human approval
     ↓
Update docs/DOMAIN.md if mental model changes
```

Invoke `/launchpad-domain` for entity, schema, or API design questions.

**Gate:** Do not write implementation code until spec is approved.

**Autonomous exception:** If the user authorized ADM (`/launchpad-autonomous`, `docs/AUTONOMOUS-MODE.md`), self-approve only after that protocol’s spec self-review checklist. Still write spec + plan; still open a PR; do not merge unless asked.

## Phase 2: Plan

```
Copy docs/superpowers/templates/plan-template.md
     → docs/superpowers/plans/YYYY-MM-DD-<name>.md
     ↓
Decompose by Go layer: domain → store → service → target/worker → api → cli
     ↓
Each task: files, checkboxes, verify commands, exact commit message
```

Delete or mark N/A any template tasks that don't apply. Add tasks for feature-specific work.

Commit the spec and plan on the feature branch before implementation:

```bash
git add docs/superpowers/specs/ docs/superpowers/plans/
git commit -m "docs: add <feature-name> spec and implementation plan"
```

## Phase 3: Branch

**Never implement on `main`.**

```bash
git checkout main && git pull
git checkout -b feat/<short-name>
# Or isolated worktree:
git worktree add .worktrees/feat-<short-name> -b feat/<short-name>
cd .worktrees/feat-<short-name>
mise exec -- make test   # must pass before starting
```

Update plan status header: `> **Status: In Progress** — branch feat/<short-name>`

## Phase 4: Implement

For each plan task, in order:

1. Mark task in progress (mentally or in plan file)
2. Implement minimal change for that task only
3. Run task verification commands
4. Commit with **exact message from plan**
5. Check off task steps in plan file

### Verification (every task)

```bash
mise exec -- make test
mise exec -- make build
mise exec -- go vet ./...
```

Deploy-flow changes (`internal/service`, `internal/jobs`, `internal/target`): also smoke test per `/launchpad-dev`.

### Commit rules

- One Go layer per commit: `feat(domain):`, `feat(store):`, `feat(service):`, `feat(api):`, `feat(cli):`, `chore:`, `docs:`
- Tests in same commit as code
- No drive-by refactors
- Periodically commit plan progress: `docs: update <feature> plan progress`

### Resuming a session

1. Read plan file — find first unchecked task
2. `git log --oneline main..HEAD` — see completed work
3. `mise exec -- make test` — confirm branch health
4. Continue from unchecked task

## Phase 5: Finish

When all tasks complete:

1. Run full verification (plan "Final verification" section)
2. Update plan: `> **Status: Completed** — merged YYYY-MM-DD` (or ready for PR)
3. Push and open PR:

```bash
git push -u origin feat/<short-name>
gh pr create --title "feat: <description>" --body "..."
```

PR body must link the spec file. Include test plan checklist.

4. Keep worktree alive until PR is merged (if using worktree)

## Large features: stacked PRs

When the spec decomposes into independent subsystems, use `/execute-plan` on the design doc to produce a PR stack. Each stack PR follows the same layer-commit rules within its scope.

## Decision table

| Signal | Action |
|--------|--------|
| Changes entities or API | `/launchpad-domain` + spec required |
| Touches deploy flow | Smoke test required |
| Crosses MVP deferred boundary | Spec + DOMAIN.md update before code |
| Multi-day / multi-session | Plan with checkboxes required |
| Single-file fix | Skip workflow — branch and PR directly |

## Related skills

| Skill | When |
|-------|------|
| `launchpad-autonomous` | User-authorized low-input / multi-step ADM loop |
| `launchpad-domain` | Entity design, invariants, MVP scope |
| `launchpad-dev` | Build, test, smoke, local API/worker |