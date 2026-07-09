# End-to-End Testing (Stub + Kind)

| Field | Value |
|-------|-------|
| **Status** | Approved |
| **Date** | 2026-07-09 |
| **Domain spec** | `docs/DOMAIN.md` (no entity changes) |
| **Branch** | `feat/e2e-testing` |
| **Scope** | Tiered e2e harness: stub on PRs, real Kubernetes via kind on schedule/label |

---

## Goal

Give Launchpad a reliable **end-to-end** suite that exercises the real API + worker loop—not only unit tests and a manual stub smoke script.

**Success criteria (v1):**

1. `make e2e-stub` passes on a clean machine with Go/mise only (starts API + worker, runs suite, tears down).
2. `make e2e-kind` passes with Docker + kind + kubectl (real cluster deploy of a public image).
3. Default `mise exec -- make test` stays fast and **never** runs e2e.
4. Stub tier exercises one **CLI** command in addition to the API happy path.
5. Kind tier additionally asserts that the Kubernetes Deployment becomes Ready (and a config Secret exists).

```bash
# Local developer loop (fast)
make e2e-stub

# Real cluster (heavier)
make e2e-kind
```

---

## Approaches considered

### A. Go e2e package + thin shell for orchestration (chosen)

- `test/e2e` with `//go:build e2e`, driven by env vars.
- `scripts/e2e-stub.sh` / `scripts/e2e-kind.sh` own process lifecycle and kind cluster.
- Same Go tests for both tiers; kind adds cluster assertions.

**Pros:** Reuses `pkg/apiclient`; CI-friendly; clear split of concerns.  
**Cons:** Needs build tags and env guards so unit CI stays clean.

### B. Shell-only (extend smoke-stub.sh)

**Pros:** Fastest to land.  
**Cons:** Brittle JSON/kubectl parsing; hard to grow; diverges from Go client.

### C. Fully in-process Go (no external binaries)

**Pros:** Fast stub path.  
**Cons:** Weaker “real binary” signal; kind still needs external orchestration; two models.

**Recommendation:** A.

---

## Scope

### In scope (v1)

- Tiered e2e: **stub** (PR / local) and **kubernetes via kind** (nightly, workflow_dispatch, optional PR label).
- Drive primarily via **HTTP API** (`pkg/apiclient`); one **CLI** smoke in stub tier.
- **Happy path only:** health → token → create project → deploy image → poll job until succeeded.
- Kind: public image (`nginx:stable` default; override via env).
- Kind: assert Deployment Ready + config Secret present for the project service.
- Makefile targets and GitHub Actions workflows.
- Document prerequisites and env vars in README or `scripts/` header comments.

### Out of scope (v1)

- Changeset stage/push, multi-service, multi-env.
- Failure-path e2e (bad image, active-deploy conflict).
- Supersede / snapshot-isolation e2e (unit tests already cover some of this).
- Postgres-backed e2e (SQLite is enough for control-plane e2e).
- Replacing unit tests or K8s fake-client unit tests.
- Full Helm/packaging install of Launchpad into the cluster (API/worker run on the host against kind’s API server).

### Deferred (later suite growth)

- Changeset + config round-trip e2e.
- Failure and concurrency cases.
- Optional in-cluster API/worker later if packaging demands it.

---

## Domain impact

None. No new entities, APIs, or schema. This is control-plane **verification infrastructure**.

| Area | Change |
|------|--------|
| Domain / store / service | Unchanged |
| Target | Exercised as black box in kind tier |
| API / CLI | Black-box clients only |

---

## Architecture

```text
  PR / local                 Nightly / label / dispatch
  ──────────                 ─────────────────────────
  scripts/e2e-stub.sh        scripts/e2e-kind.sh
       │                          │
       │  temp SQLite             │  kind create/reuse
       │  start api+worker        │  namespace per run
       │  TARGET=stub             │  start api+worker
       │                          │  TARGET=kubernetes
       └──────────┬───────────────┘
                  v
           go test -tags=e2e ./test/e2e/
                  │
                  ├─ happy path via apiclient
                  ├─ stub: one CLI smoke
                  └─ k8s: Deployment Ready + Secret
```

### Environment contract

| Variable | Required | Purpose |
|----------|----------|---------|
| `LAUNCHPAD_E2E` | yes (`1`) | Gate: tests skip if unset |
| `LAUNCHPAD_API_URL` | yes | API base URL |
| `LAUNCHPAD_BOOTSTRAP_TOKEN` | yes | Bootstrap auth |
| `LAUNCHPAD_E2E_TARGET` | yes | `stub` or `kubernetes` |
| `LAUNCHPAD_E2E_IMAGE` | no | Deploy image (default `nginx:stable`) |
| `LAUNCHPAD_E2E_NAMESPACE` | kind only | K8s namespace for project target config |
| `LAUNCHPAD_E2E_CLI` | no | Path to `launchpad` binary (default `./bin/launchpad`) |
| `KUBECONFIG` | kind only | Cluster access for assertions (kind sets this) |
| `LAUNCHPAD_E2E_TIMEOUT` | no | Job poll timeout (default stub 30s, kind 3m) |

### Rules

1. Default `go test ./...` **must not** compile or run `test/e2e` (`//go:build e2e`).
2. E2e tests talk only to a **running** API (and kubectl/client-go for kind assertions). They do not import `internal/service` for business logic shortcuts.
3. Scripts own lifecycle: free/fixed ports, PIDs, temp DB, trap EXIT cleanup, unique project names (`e2e-<unix>-<rand>`).
4. Kind script prefers **reuse** of a named cluster (e.g. `launchpad-e2e`) and **delete namespace only** for speed; optional full cluster recreate via flag/env for clean slate.

---

## Scenarios (v1)

### Shared happy path (API)

1. `GET /healthz` → 200.
2. `POST /v1/tokens` with bootstrap → admin token.
3. `POST /v1/projects` with name + target:
   - stub: `{"type":"stub","namespace":"default"}`
   - kubernetes: `{"type":"kubernetes","namespace":"<E2E_NAMESPACE>"}`
4. `POST /v1/projects/{p}/releases` with image `LAUNCHPAD_E2E_IMAGE`.
5. Poll `GET /v1/jobs/{id}` until `succeeded` (fail on `failed`/`dead` or timeout).
6. `GET /v1/projects/{p}/releases` non-empty; latest status consistent with success.

### Stub-only: CLI smoke

After API path succeeds (or using same project):

- Set `LAUNCHPAD_TOKEN`, `LAUNCHPAD_API_URL`, `LAUNCHPAD_PROJECT` (or `launchpad use`).
- Run e.g. `launchpad releases` or `launchpad ps` and assert exit 0 and non-empty output.

### Kind-only: cluster assertions

Using client-go or `kubectl` from Go:

1. Deployment named per target helper `deploymentName(project, service, "web")` → `launchpad-{project}-{service}-web` in `LAUNCHPAD_E2E_NAMESPACE` has Ready replicas ≥ 1.
2. Config Secret named `secretName(project, service)` → `launchpad-{project}-{service}-config` exists (data may be empty).

Image: public **`nginx:stable`** by default. Document that kind nodes need pull access; optional note for `kind load docker-image` if offline.

---

## File layout

```text
test/e2e/
  doc.go                 # package docs + build tag note
  main_test.go           # TestMain: require LAUNCHPAD_E2E=1
  helpers_test.go        # client, unique names, pollJob
  happy_path_test.go     # shared API happy path
  cli_smoke_test.go      # stub-oriented CLI check (skip if no CLI binary)
  k8s_assert_test.go     # kind assertions (skip unless TARGET=kubernetes)

scripts/
  e2e-stub.sh
  e2e-kind.sh
  smoke-stub.sh          # keep; may call e2e-stub or remain thin manual helper

.github/workflows/
  ci.yml                 # make test + e2e-stub
  e2e-kind.yml           # schedule + workflow_dispatch + optional label

Makefile                 # e2e-stub, e2e-kind targets
```

### Makefile (sketch)

```make
e2e-stub:
	./scripts/e2e-stub.sh

e2e-kind:
	./scripts/e2e-kind.sh
```

Scripts call:

```bash
mise exec -- go test -tags=e2e ./test/e2e/ -count=1 -timeout=10m
```

---

## CI

| Workflow | Trigger | Jobs |
|----------|---------|------|
| `ci.yml` | PR, push to `main` | `make test` / `go vet`; then `make e2e-stub` |
| `e2e-kind.yml` | `schedule` (nightly), `workflow_dispatch`, PR with label `e2e-kind` | install kind/kubectl, `make e2e-kind` |

Runners: GitHub-hosted Linux with Docker for kind.

---

## Target / worker impact

None for product code in v1. Kind tier requires:

- Worker with Kubernetes target enabled (default when kubeconfig present).
- API/worker share the same SQLite file URL.
- Project `target_config` namespace matches e2e namespace.
- Image pull policy / cluster DNS must allow public image pull.

If worker fails to register k8s, kind e2e fails clearly at deploy (job failed) — scripts should print worker logs on failure.

---

## Test strategy (pyramid)

| Layer | Role | Remains |
|-------|------|---------|
| Unit | Domain, store, service, jobs, k8s fake client | Yes — primary PR feedback |
| E2e stub | Real processes, stub target | New — every PR |
| E2e kind | Real processes + real cluster | New — nightly / label |
| Manual smoke | `smoke-stub.sh` | Optional convenience; e2e-stub is the canonical automated path |

---

## Risks and mitigations

| Risk | Mitigation |
|------|------------|
| Kind flaky / slow | Not on default PR path; 3m+ deploy timeout; stable public image |
| Image pull failure | Document registry access; optional `kind load` |
| Port conflicts | Scripts bind fixed high ports (e.g. 18080) or probe free ports |
| Orphan processes | `trap` EXIT kill api/worker PIDs |
| CI cost | Kind nightly only by default |
| Naming drift | Assert using same `deploymentName` helpers as target package or documented convention only |

---

## Open questions (resolved in design)

| Question | Decision |
|----------|----------|
| PR vs nightly | Stub on PR; kind nightly/label |
| Driver | API primary + one CLI smoke |
| v1 scenarios | Happy path only |
| Images | Public `nginx:stable` |
| Harness | Go + thin shell |

---

## Implementation order (for later plan)

1. Scaffold `test/e2e` with build tag + happy path against externally started API (manual first).
2. `scripts/e2e-stub.sh` + `make e2e-stub`.
3. CLI smoke test.
4. `scripts/e2e-kind.sh` + k8s assertions.
5. GitHub Actions (`ci.yml` e2e-stub, `e2e-kind.yml`).
6. Docs (README / AGENTS.md pointer).

---

## Approval

- [x] Brainstorm design approved (architecture + scenarios + layout + CI)
- [ ] Spec file reviewed by human
- [ ] Implementation plan written (`docs/superpowers/plans/…`)
- [ ] Implementation starts only after plan exists
