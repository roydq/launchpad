# Implicit Changeset CLI Implementation Plan

> **Status: In Progress** — branch `feat/implicit-changeset-cli`, started 2026-07-09

> **For agentic workers:** Read `docs/FEATURE-DEVELOPMENT.md` and the spec. Use `/launchpad-dev` for verification. Commit after each task with the message specified below. REQUIRED SUB-SKILL when dispatching: `superpowers:subagent-driven-development` or execute inline with `superpowers:executing-plans`.

**Goal:** Make CLI staging implicit (config/scale/image stage by default; `diff`/`status`/`reset`/`deploy`) while keeping the changeset HTTP API unchanged.

**Architecture:** Thin CLI composer over existing `POST/GET/DELETE …/changeset*` and push. Shared helpers in `internal/cli` for stage, load pending, require clean, push, and client-side diff vs last release. No service/worker/schema changes.

**Tech Stack:** Go, cobra, apiclient

**Spec:** `docs/superpowers/specs/2026-07-09-implicit-changeset-cli-design.md`

**Branch:** `feat/implicit-changeset-cli`

---

## File map

| File | Responsibility |
|------|----------------|
| `pkg/apiclient/client.go` | Extend `Release` with snapshot fields for diff baseline; keep existing changeset methods |
| `internal/cli/staging.go` | Shared helpers: parse mutations, stage, loadPending, requireClean, push, print deploy result |
| `internal/cli/diff.go` | Pure fold + format staged vs last release |
| `internal/cli/diff_test.go` | Unit tests for fold/diff (no network) |
| `internal/cli/root.go` | Command map rewrite; remove `changeset` group |
| `README.md`, `docs/DOMAIN.md` | User-facing CLI examples |

---

## Task 1: apiclient — release snapshot fields

**Files:**
- Modify: `pkg/apiclient/client.go`

- [ ] Extend `Release` so list responses carry diff baselines:

```go
type ProcessSnapshot struct {
	Command  string `json:"command"`
	Quantity int    `json:"quantity"`
	Expose   string `json:"expose"`
}

type Release struct {
	ID              string                     `json:"id"`
	Version         int                        `json:"version"`
	ArtifactRef     string                     `json:"artifact_ref"`
	ConfigResolved  map[string]string          `json:"config_resolved"`
	ProcessSnapshot map[string]ProcessSnapshot `json:"process_snapshot"`
	Status          string                     `json:"status"`
	Description     string                     `json:"description"`
}
```

- [ ] Verify: `mise exec -- go test ./pkg/apiclient/...` (package may have no tests; compile via `go test ./pkg/...`)
- [ ] Commit: `feat(apiclient): include release snapshot fields for CLI diff`

---

## Task 2: CLI staging helpers + pure diff (TDD)

**Files:**
- Create: `internal/cli/staging.go`
- Create: `internal/cli/diff.go`
- Create: `internal/cli/diff_test.go`

### staging.go

Package-level helpers used by commands (signatures can match closely):

```go
// Change builders → []map[string]any matching StageChangeInput wire shape:
// config: {"type":"config","key":K,"value":V}  // value omitted/null for unset
// scale:  {"type":"scale","process":P,"quantity":N}
// image:  {"type":"image","image":ref}

func parseKEYVALArgs(args []string) ([]map[string]any, error)
func parseScaleFlag(scale string) (map[string]any, error) // "web=3"
func imageChange(ref string) map[string]any
func configUnsetChange(key string) map[string]any

func loadPending(ctx, client, project) (*apiclient.Changeset, error) // empty Changes OK
func pendingCount(cs *apiclient.Changeset) int
func requireCleanStaging(ctx, client, project) error
// Error text must mention: diff, deploy, reset

func stage(ctx, client, project, changes []map[string]any) (*apiclient.Changeset, error)
func push(ctx, client, project, message string) (*apiclient.DeployResult, error)
func printDeployResult(result *apiclient.DeployResult)
func latestRelease(ctx, client, project) (*apiclient.Release, error)
// Prefer highest version with status=="succeeded"; else highest version; else nil
```

`requireCleanStaging`: if `pendingCount > 0`, return error like:
`staging has N pending change(s); run "launchpad diff", "launchpad deploy", or "launchpad reset" before using --now`

`reset` handler (in root): call DiscardChangeset; if error message/status indicates not found, print friendly “nothing to reset” and return nil.

### diff.go — pure logic

```go
type FoldedPending struct {
	Image   string            // last staged image, or ""
	Config  map[string]*string // key → value; nil value = delete
	Scales  map[string]int     // process → quantity
}

func foldChanges(changes []apiclient.ChangesetChange) (FoldedPending, error)
// Unmarshal each payload:
//  config: {"key","value"}
//  scale:  {"process","quantity"}
//  image:  {"artifact_ref"}

func formatDiff(pending FoldedPending, baseline *apiclient.Release) string
// Sections only if non-empty effective delta:
// ## Image
//   old → new
// ## Config
//   + KEY=val / ~ KEY: old → new / - KEY
// ## Scale
//   process: old → new
// If no pending ops at all: "No pending changes\n"
// If pending but all no-ops vs baseline: still show sections or a one-liner that staged ops match release — prefer showing only effective deltas; if zero effective deltas print "No effective changes vs last release (staged ops are no-ops)\n" OR show staged ops annotated — pick: **show only effective deltas**; if fold non-empty but no effective delta: "Staged changes match last release (no effective delta)\n"
// If baseline nil: treat all staged as additions (image new, config +, scale as new quantities)
```

### Tests (`diff_test.go`)

- [ ] Write failing tests first:

```go
func TestFoldChangesLastWriteWins(t *testing.T)
// stage PORT=1 then PORT=2 → Config["PORT"] = "2"
// image a then b → Image = b
// scale web=1 then web=3 → Scales["web"]=3

func TestFormatDiffConfigAddChangeRemove(t *testing.T)
// baseline config A=1,B=2; pending A=9, B=null, C=3
// expect lines for ~A, -B, +C

func TestFormatDiffNoPending(t *testing.T)
// empty fold → "No pending changes"

func TestFormatDiffFirstRelease(t *testing.T)
// baseline nil, image + config → additions only
```

- [ ] Run: `mise exec -- go test ./internal/cli/... -count=1` — expect FAIL until impl
- [ ] Implement `diff.go` + `staging.go` (helpers that need client can be thin wrappers)
- [ ] Run: `mise exec -- go test ./internal/cli/... -count=1` — expect PASS
- [ ] Commit: `feat(cli): add staging helpers and client-side diff`

---

## Task 3: CLI command map rewrite

**Files:**
- Modify: `internal/cli/root.go`

### Remove
- Entire `changeset` command group (`status`, `add`, `reset`, `push`)

### Rewire / add

**`config set [KEY=VALUE...]`**
- Flags: `--now bool`, `--message/-m string` (used with `--now`)
- Default: `stage` config changes; print `Staged config KEY[, KEY…]`
- `--now`: `requireCleanStaging` → stage → `push(message)`; print deploy result

**`config unset [KEY...]`**
- Flags: `--now`, `--message/-m`
- Stage config deletes (`value: null`); same `--now` path

**`config get`**
- Unchanged: live `GetConfig`

**`scale [PROC=N...]`** (new top-level command)
- Args: one or more `web=3`
- Flags: `--now`, `--message/-m`
- Stage scale changes; `--now` as above

**`image [ref]`** (new top-level)
- Args: ExactArgs(1)
- Flags: `--now`, `--message/-m`
- Stage image; `--now` as above

**`diff`**
- loadPending + latestRelease + formatDiff; print string

**`status`**
- Project name; pending count; type breakdown (config/scale/image counts); hints for diff/deploy/reset
- If zero pending: “No pending changes”

**`deploy`**
- Flags: `--image`, `--scale` (repeatable or single `web=3` like today), `--message/-m`
- Args: optional `KEY=VALUE…`
- Mark `--image` **not** required
- Algorithm from spec: stage optional mutations → if pending empty fail `nothing to deploy` → push
- Do **not** call `client.Deploy` (POST /releases) for the happy path; always push
- One-shot empty staging with only `--image` still stages then pushes

**`reset`**
- DiscardChangeset; not-found → friendly no-op

### Keep
- `projects`, `use`, `ps`, `releases` as today

### Wire message flag consistency
- Prefer `StringP("message", "m", "", "release description")` on deploy and `--now` commands

### parseStageArgs
- Can move into staging.go and reuse from deploy; delete dead local helpers if unused

- [ ] Implement command map
- [ ] Verify: `mise exec -- make build`
- [ ] Verify: `mise exec -- go test ./internal/cli/... ./pkg/apiclient/...`
- [ ] Manual sanity (optional if no API): `mise exec -- go run ./cmd/launchpad --help` and ensure no `changeset` in help
- [ ] Commit: `feat(cli): implicit staging with diff status reset deploy`

---

## Task 4: Docs alignment

**Files:**
- Modify: `README.md`
- Modify: `docs/DOMAIN.md` (CLI examples under Changeset workflow only; entity stays)
- Modify: plan status header when done

### README
Replace solo workflow and changeset sections with:

```bash
launchpad projects create my-api
launchpad use my-api
launchpad config set PORT=3000
launchpad image my-api:v1
launchpad diff
launchpad deploy -m "initial"

# one-shot
launchpad deploy --image my-api:v2 PORT=8080 -m "bump"

# immediate
launchpad config set DEBUG=true --now -m "debug"

# discard
launchpad reset
```

Note that the HTTP API still uses `/v1/projects/{project}/changeset*`.

### DOMAIN.md
Update **Changeset workflow** CLI examples to implicit staging; add one sentence that CLI stages via changeset API and users use `deploy` not `changeset push`. Keep entity docs.

- [ ] Edit docs
- [ ] Verify: docs only + `mise exec -- make test && mise exec -- make build`
- [ ] Commit: `docs: align CLI examples with implicit staging`
- [ ] Update this plan: `> **Status: Completed** — ready for PR` and check off tasks

---

## Final verification

```bash
mise exec -- make test
mise exec -- make build
mise exec -- go vet ./...
```

CLI does not change worker deploy path; smoke optional. If smoke is run, prefer:

```bash
# against running API+worker
launchpad projects create …
launchpad use …
launchpad deploy --image smoke:v1 -m smoke
```

## PR checklist

- [ ] All tasks checked off
- [ ] Plan status **Completed**
- [ ] Spec linked in PR body
- [ ] No `changeset` CLI command
- [ ] No `*.db`, `.env`, `bin/` committed

---

## Spec coverage checklist

| Spec requirement | Task |
|------------------|------|
| Stage-by-default config/scale/image | 3 |
| `diff` vs last release | 2, 3 |
| `status` summary | 3 |
| `reset` discard | 3 |
| `deploy` submit + one-shot mutations | 3 |
| `--now` on mutations only; block if dirty | 2, 3 |
| `--now` = stage + push | 3 |
| No `deploy --now` | 3 |
| Remove changeset CLI (no aliases) | 3 |
| API unchanged | all (no API tasks) |
| Shared helpers | 2 |
| README + DOMAIN CLI examples | 4 |
| apiclient snapshot fields for diff | 1 |
