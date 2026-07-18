# Persona user scripts

Synthetic dogfood for ADM. The persona agent behaves like a **real solo engineer**, not a unit test author.

**Protocol:** `docs/AUTONOMOUS-MODE.md`  
**Skill context:** `/launchpad-autonomous`  
**Dev setup:** `.grok/skills/launchpad-dev/SKILL.md`

---

## Persona

| Field | Value |
|-------|-------|
| **Who** | Solo engineer, first week with Launchpad |
| **Goal** | Zero → running on stub quickly; multi-env promote without renaming projects |
| **Bar** | `docs/DX-VISION.md` principles (one name many places, same verbs, diff before trust, recovery hints) |
| **Voice** | Concrete friction (“I typed X and got Y”); severity P0–P3 |

**Not this persona:** adversary red-team, k8s cluster admin deep dive (optional script later), or “rewrite the domain model.”

---

## Environment assumptions

Prefer the **stub e2e harness** when possible:

```bash
mise exec -- make e2e-stub
```

For interactive CLI dogfood (API + worker already running):

```bash
# Terminal A
make migrate-up
LAUNCHPAD_BOOTSTRAP_TOKEN=dev-bootstrap-token make run-api

# Terminal B
LAUNCHPAD_DATABASE_URL="file:launchpad.db?_pragma=foreign_keys(1)" make run-worker
```

Use `mise exec --` for all `go` / built binaries if PATH lacks Go. Prefer `./bin/launchpad` after `mise exec -- make build`.

Do **not** require kind/Kubernetes for the default scripts below.

---

## Script S1 — Day-one happy path (required when CLI/deploy UX changes)

1. Build CLI (`mise exec -- make build`).
2. Obtain a token (bootstrap → `POST /v1/tokens` or whatever README documents).
3. `launchpad projects create persona-demo --target stub --namespace default` (or current flags).
4. `launchpad use persona-demo` / project-local context.
5. Stage an image (implicit changeset): set image / `deploy --image` per current CLI.
6. `launchpad diff` (or preview) — expect something intelligible before deploy.
7. `launchpad deploy --wait` (or equivalent) — expect terminal success or a clear failure.
8. `launchpad inspect` and/or process list; `launchpad logs` if applicable.
9. Record: time-to-success feel, confusing flags, missing hints.

**Pass bar:** Completes without reading source; errors (if any) include recovery hints where the API supports them.

---

## Script S2 — Config and layers

1. Set shared-layer config and service-layer config (CLI `config` + `--layer` if present).
2. Diff / preview pending vs release.
3. Deploy and confirm resolved config behavior (inspect or target-facing view on stub).
4. Note any layer default confusion.

---

## Script S3 — Multi-env promote (when feature touches env/promote)

1. Ensure `dev` works (S1).
2. Create or select a second environment (e.g. staging) via `env` commands.
3. Deploy or promote per current verbs; confirm config **re-resolution** expectations from DOMAIN.
4. `launchpad rollback` once if safe on stub.
5. Note ambient env mistakes (`X-Launchpad-Environment` / `env use`).

---

## Script S4 — Deliberate mistakes (recovery UX)

Run at least two:

| Mistake | Expect |
|---------|--------|
| Bad / missing token | 401/403 with problem+json; CLI prints recovery if wired |
| Wrong environment header / name | Clear not-found or mismatch; hint to `env use` / list |
| Deploy while worker stopped (if easy) | Job pending/fail with actionable message — not silent hang forever without `--wait` timeout |
| Production-named env without `--yes` | CLI refuses sensitive deploy/promote/rollback |

---

## Script S5 — Doctor and archaeology (optional)

1. `launchpad doctor`
2. `releases` list / `releases show N` / release↔release diff if data exists
3. Note gaps vs “trust the release history”

---

## Output

Write feedback to:

```text
docs/superpowers/program/feedback/YYYY-MM-DD-<feature-or-session>.md
```

Create `feedback/` on first use. Suggested structure:

```markdown
# Persona feedback — <feature>

| Field | Value |
|-------|-------|
| Date | YYYY-MM-DD |
| Scripts run | S1, S4, … |
| Build / e2e | pass/fail + command |

## Narrative
…

## Findings
| Severity | Finding | Repro | Suggested follow-up |
|----------|---------|-------|---------------------|
| P1 | … | … | IDEAS or QUEUE id |

## What felt good
…
```

Also paste a short summary into the PR body when ADM opens a PR.

### Promotion rules

| Severity | Action |
|----------|--------|
| P0 | Fix in same PR if caused by this feature; else hard-stop or QUEUE `ready` fix if user authorized auto-queue P0s |
| P1 | Append `IDEAS.md`; offer QUEUE promotion in PR notes |
| P2–P3 | Append `IDEAS.md` only |

**Never** expand feature scope from persona feedback without QUEUE/human promotion (except same-PR fixes for regressions the feature introduced).

---

## Orchestrator tips

- Run persona **after** L0–L3 verification is green.
- Prefer `make e2e-stub` plus a thin CLI script over a full long-lived cluster when CI-like confidence is enough.
- If persona cannot run (no docker/network/etc.), record `blocked` in feedback with reason — do not fake a pass.
