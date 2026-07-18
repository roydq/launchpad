# Secrets-typed config — design delivery plan

> **Status: Completed** — branch `feat/secrets-design`  
> **Mode:** Design only (ADM). No product code. Human model review required before S1.

**Goal:** Land an approved-for-review design for secrets-typed config that unblocks env clone and safer multi-tenant defaults without implementing them yet.

**Architecture:** Spec + program/docs sync only. Implementation phases S1–S3 are described in the spec for later PRs after human acceptance.

**Spec:** `docs/superpowers/specs/2026-07-18-secrets-typed-config-design.md`

**Branch:** `feat/secrets-design`

---

## Task 1: Design spec

**Files:**
- Create: `docs/superpowers/specs/2026-07-18-secrets-typed-config-design.md`

- [x] Write approaches A/B/C with recommendation A (sensitivity on existing layers)
- [x] Scope, domain impact, API/CLI/schema sketches, phases S0–S3
- [x] Open questions with recommended defaults
- [x] ADM self-review checklist; status = human model review gate
- [x] Commit: `docs: secrets-typed config design (review gate)`

## Task 2: Program + vision sync

**Files:**
- Modify: `docs/superpowers/program/QUEUE.md`
- Modify: `docs/DX-VISION.md`
- Modify: `docs/DOMAIN.md` (pointer only — full sensitivity section waits for S1)

- [x] QUEUE: `secrets-design` → `pr-open` (after PR); interim `designing`/`implementing` as docs land
- [x] DX-VISION: Active/next + Track D link to spec
- [x] DOMAIN: open question / roadmap pointer to secrets design
- [x] Commit: `docs: queue and vision for secrets design`

## Task 3: Final verification (docs PR)

- [x] No Go code changes
- [x] Spec linked from QUEUE + DX-VISION
- [x] Plan status → **Completed** after PR open
- [x] PR body: summary, open questions for human, hard stop before S1

```bash
# Sanity only — no code expected to change
git diff main --stat
```

## PR checklist

- [x] Spec complete with self-review
- [x] Human review requested (model acceptance)
- [x] No `*.db`, `.env`, or `bin/` committed
- [x] **Do not merge** unless user asks; **do not implement S1** in this PR

## After human accepts model

1. Mark spec **Approved (human)** (or fold amendments).
2. Add QUEUE rows for `secrets-s1-typing-redaction` and optionally `secrets-s2-encryption`.
3. Set `secrets-design` → `shipped` when design PR merges.
4. Separate ADM/feature session for S1 implementation.
