# Runtime target depth (design)

| Field | Value |
|-------|-------|
| **Status** | **Approved (human discussion)** — end-state model; implementation queued for ADM |
| **Date** | 2026-07-20 |
| **Domain** | `docs/DOMAIN.md` |
| **Scope** | End-state design for process commands, deploy health, release-immutable config materialization, and target-specific extensions. **No product code in this PR.** |
| **Queue** | `process-commands`, `deploy-health`, `release-config-materialization`, `target-extensions` |
| **Related** | Target interface in DOMAIN; `docs/superpowers/specs/2026-07-19-target-conformance-design.md`; secrets-typed config (control-plane only) |

---

## Goal

Launchpad’s control plane already has the right skeleton: **releases own desired state**, environments own **targets**, processes describe **how** a shared image runs. Four depth gaps remain before multi-process and multi-backend dogfood feels production-grade:

1. **Config materialization** — cluster config objects should be as immutable as the release.
2. **Process commands** — one image, many roles; first-class commands and Procfile import.
3. **Deploy success** — “ready” must mean healthy, with timeouts and portable health.
4. **Target-specific power** — Kubernetes (and future backends) need resources, annotations, etc. without polluting the portable domain.

**Design success criteria (this document):**

- One recommended end-state model per gap, consistent with release snapshot invariants.
- Clear portable vs target-native boundaries.
- ADM-ready implementation slices with acceptance criteria (CLI/API/target behavior).
- No half-measures required as permanent product surface — transitional steps only where implementation order is forced.

**Product success criteria (when all slices ship):**

```bash
# Multi-process from one image
launchpad process set web --command "serve --port $PORT"
launchpad process set worker --command "run-worker" --quantity 2 --expose none
# or
launchpad process apply --procfile Procfile

# Health drives deploy success
launchpad process set web --health http --health-path /healthz
launchpad deploy --image ghcr.io/acme/api:1.2 --wait
# succeeds only when readiness is true within timeout

# Inspect shows process commands + health in snapshot
launchpad releases show N

# Target-native knobs without new domain entities
launchpad process set web --target-ext kubernetes.resources.requests.memory=256Mi
# or via launchpad.yaml / API target_extensions
```

---

## Principles (normative for all slices)

1. **Release is deploy source of truth.** Artifact, `config_resolved`, `process_snapshot`, and any portable/target extension snapshots are frozen at release creation. Workers never re-read live process/config/extension tables for desired state.
2. **Portable first, target-native second.** Anything ≥2 backends need (command, scale, HTTP health) lives on Process / portable snapshot fields. Backend-only power lives under namespaced **target extensions**.
3. **Environment owns where; process owns how.** `target_config` = cluster/env defaults. Process (and optional service defaults) = per-role runtime.
4. **Immutability at the data plane.** Materialized runtime objects for a release should not be silently rewritten out from under other revisions when avoidable.
5. **Capabilities over sprawl.** Targets advertise what they support; CLI/API validate and document; unknown extension keys fail closed or ignore per target policy (default: **reject unknown keys for the active target type** at stage/push).

---

## Slice 1 — Release-immutable config materialization

### Problem

Today the Kubernetes target upserts a **stable** Secret (`launchpad-{project}-{service}-config`) and mounts it via `envFrom`. Deploy overwrites the same object. That couples all pod generations to a mutable shared Secret and weakens the “release snapshot is desired state” story at the data plane.

Control-plane secrets typing/encryption is orthogonal (already shipped S1/S2); this slice is **how the target applies** resolved plaintext config.

### End state (recommended)

**Immutable config object per release (or per content hash), pinned by the Deployment/pod template.**

| Property | Rule |
|----------|------|
| Name | Content-addressed preferred: `launchpad-{project}-{service}-cfg-{hash12}` where hash is of the canonical sorted `config_resolved` map; alternatively `…-cfg-v{version}` |
| Lifetime | Created on deploy if missing; never mutated after create |
| Pinning | Deployment container `envFrom` references **that** Secret name; pod template annotation includes release version + config hash |
| Sharing | Identical config across consecutive releases may reuse the same content-hashed Secret (no rewrite) |
| GC | Target (or a periodic janitor) deletes unreferenced Secrets for the service older than N releases / not selected by any managed Deployment |
| Mount style | Keep `envFrom` (all keys as env). Optional future: projected volume for large values — not required for end state v1 of this slice |
| Plain vs secret split | Optional later: ConfigMap for plain + Secret for sensitive keys when RBAC warrants it. **Not required** for correctness; single Secret is fine while control plane already redacts |

**Stub target:** no cluster objects; may ignore materialization details. Conformance still receives the same `DeployRequest.Config`.

### Approaches considered

| | Approach | Verdict |
|--|----------|---------|
| **A** | Per-release / content-hash immutable Secret + pin | **Recommended** |
| B | Stable Secret + pod restart annotation only | Reject as end state — still mutable shared data |
| C | Inline `env:` on Deployment only | Reject — no immutability, ugly diffs, size limits |

### Acceptance (when implemented)

- Two consecutive deploys with **different** config produce **two** Secrets (or two hashes); old Deployment generation is not required to keep running, but new pin is explicit.
- Rollback to a prior release reuses or recreates the Secret matching that release’s `config_resolved` (same hash ⇒ same object).
- Destroy removes managed config objects for the service (or leaves only unreferenced GC candidates — pick one policy and document; **recommended:** Destroy deletes all Launchpad-labeled config objects for that project/service in the namespace).

### Out of scope

- External secret managers (`secret_ref`) — separate future.
- Syncing Secret RBAC beyond default namespace access.

---

## Slice 2 — Process commands and Procfile

### Problem

Domain already models `Process.command` and freezes it in `process_snapshot`. Kubernetes applies non-empty command as container `command`. Gaps vs end state:

- No first-class **mutation** path (API list-only; scale quantity only via changeset).
- Command is a single string applied as a **one-element** argv — multi-word / shell form unspecified.
- No **Procfile** import for Heroku-style multi-process from one image.

### End state (recommended)

#### Process definition (portable)

| Field | End state |
|-------|-----------|
| `name` | Unique per service (`web`, `worker`, …) |
| `command` | Override image entrypoint/CMD. **Empty** = image default |
| `quantity` | Desired replicas |
| `expose` | `http` \| `tcp` \| `none` |
| `health` | See slice 3 |

**Command semantics (normative):**

- Wire form remains a **string** for DX (Procfile parity, CLI friendliness).
- **Empty** → do not set container command/args (image ENTRYPOINT/CMD).
- **Non-empty** → target runs via **shell form** for portability of multi-word commands:

  ```text
  Kubernetes: command = ["/bin/sh", "-c", process.Command]
  ```

  Document that the image must include `/bin/sh` (true for almost all app images). Future opt-in exec form (`command_argv: []string`) may be added if users need no-shell; **not required** for end state v1.

- `process_snapshot` continues to store `command` as the string; sufficient for rebuild without live tables.

#### Control plane DX

Process changes are **staged** like config/scale (open changeset, env-pinned), then applied on push:

```bash
launchpad process set <name> [--command CMD] [--quantity N] [--expose http|tcp|none] …
launchpad process unset <name>          # remove process definition (not last web without replacement — product rule)
launchpad process apply --procfile path # replace or merge process set from Procfile
launchpad processes                     # list live definitions
```

API:

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/v1/projects/{project}/processes` | List (exists) |
| `PUT` | `/v1/projects/{project}/processes/{name}` | Upsert process fields (stages) |
| `DELETE` | `/v1/projects/{project}/processes/{name}` | Remove process (stages) |
| `POST` | `/v1/projects/{project}/processes/apply` | Body: Procfile text or structured process map |

Changeset change kinds: `process.set`, `process.unset`, `process.apply` (or fold into existing mutation model consistently).

#### Procfile

Import-only DX (not a runtime entity, not read at deploy time):

```
web: bundle exec puma -C config/puma.rb
worker: bundle exec sidekiq
release: bundle exec rake db:migrate
```

- Maps lines → process name + command; default `quantity=1`; `web` → `expose=http`, others → `expose=none` unless overridden.
- `release` process: end state may treat as one-shot hook later; for this slice, allow defining it with `quantity=0` or document “not deployed until release-phase jobs exist.” **Recommended for v1 of this slice:** import all entries; only processes with `quantity > 0` generate target workloads; `release` defaults to `quantity=0` (definition stored, not deployed).

### Approaches considered

| | Approach | Verdict |
|--|----------|---------|
| **A** | String command + shell form at target; staged mutations; Procfile apply | **Recommended** |
| B | argv array only | Worse Procfile/CLI DX |
| C | Read Procfile from git at deploy | Reject — violates release snapshot; couples worker to VCS |

### Acceptance

- `process set worker --command "run-worker"` → next release snapshot includes worker; K8s Deployment runs shell form with that string.
- Empty command leaves image default.
- Procfile apply stages multiple processes; push deploys all `quantity > 0`.
- Rollback/promote copy process topology including commands (existing promote/rollback rules).

---

## Slice 3 — Deploy success, timeouts, and health

### Problem

Kubernetes deploy waits until Deployment “ready” (replica counts + Available condition), default **15m** timeout (`LAUNCHPAD_K8S_DEPLOY_TIMEOUT`). **No probes** on the pod template, so readiness often means “container started,” not “app healthy.”

### End state (recommended)

#### Portable health (on Process, snapshotted)

```json
"web": {
  "command": "",
  "quantity": 2,
  "expose": "http",
  "health": {
    "type": "http",
    "path": "/healthz",
    "port": null,
    "initial_delay_seconds": 5,
    "period_seconds": 10,
    "timeout_seconds": 2,
    "failure_threshold": 3,
    "success_threshold": 1
  }
}
```

| Field | Meaning |
|-------|---------|
| `type` | `http` \| `tcp` \| `exec` \| `none` (explicit none) |
| omitted / null health | Target default: **no probe** (image-only) |
| `path` | HTTP path (default `/healthz` when type=http and path empty — or require explicit path; **recommended:** default `/healthz` for type=http) |
| `port` | null → process/container primary port (`PORT` config or target default) |
| timings | Map to probe timings on K8s; other targets approximate |

**Liveness vs readiness:** portable `health` maps to **readiness** (deploy success + serving traffic). Optional `liveness` as a separate nested object later; **end state v1 = readiness only** to avoid restart loops from misconfigured liveness.

#### Deploy success contract

| Step | Rule |
|------|------|
| Apply | Target applies desired state from release |
| Wait | Target waits until all process workloads for the deploy are **ready** per backend semantics |
| K8s ready | `ReadyReplicas >= desired` and Deployment Available; with probes, container Ready implies probe success |
| Timeout | Fail deploy with clear error; deployment/job → `failed` |
| Default timeout | 15 minutes if unset |
| Override | `target_config.deploy_timeout` (duration string) overrides target binary defaults; env var remains operator escape hatch |

No second control-plane HTTP poller in end state v1 — **correct readiness at the target is sufficient**.

#### CLI/API

```bash
launchpad process set web --health http --health-path /ready --health-port 8080
launchpad process set web --health none
```

### Approaches considered

| | Approach | Verdict |
|--|----------|---------|
| **A** | Portable health on process → target probes; wait on backend ready; timeout on env/target | **Recommended** |
| B | Control plane curls public URL after deploy | Reject as primary — needs ingress, auth, multi-process awkwardness |
| C | Only Deployment Available without probes | Reject as end state |

### Acceptance

- HTTP health on `web` produces a readinessProbe on the K8s Deployment.
- Deploy with a never-ready probe fails within timeout with a message naming the process.
- Stub target: may treat health as no-op but must still respect a short configurable timeout in conformance if simulated.

---

## Slice 4 — Target extensions and capabilities

### Problem

Environments carry `target_type` + `target_config` (`namespace` / `cluster` for K8s only). Users will need resources, annotations, labels, service accounts, node selectors, etc. Stuffing every field into portable Process breaks multi-target and DOMAIN purity.

### End state (recommended): three layers

```text
┌─────────────────────────────────────────────────────────────┐
│  Release desired state (portable + snapshotted extensions)  │
│  artifact · config_resolved · process_snapshot · extensions │
└────────────────────────────┬────────────────────────────────┘
                             │
         ┌───────────────────┼───────────────────┐
         ▼                   ▼                   ▼
  Env target_config    Process portable     Target extensions
  (where / defaults)   (command, qty,       (namespaced by
                        expose, health)      target type)
```

#### A. Portable process / release fields

Always understood by every target (ignore with warning only if truly inapplicable — prefer implement or reject at validate).

#### B. Environment `target_config`

Cluster-wide / env-wide defaults for the active backend. End-state K8s shape (illustrative):

```json
{
  "namespace": "apps",
  "cluster": "prod-1",
  "deploy_timeout": "20m",
  "defaults": {
    "service_account_name": "launchpad-runtime",
    "image_pull_secrets": ["regcred"]
  }
}
```

#### C. Target extensions (snapshotted)

Namespaced map on **process** (primary) and optionally service-level defaults later:

```json
"process_snapshot": {
  "web": {
    "command": "",
    "quantity": 2,
    "expose": "http",
    "health": { "type": "http", "path": "/healthz" },
    "target_extensions": {
      "kubernetes": {
        "resources": {
          "requests": { "cpu": "100m", "memory": "128Mi" },
          "limits": { "memory": "512Mi" }
        },
        "annotations": { "prometheus.io/scrape": "true" },
        "labels": {},
        "node_selector": { "workload": "general" },
        "tolerations": [],
        "service_account_name": null,
        "pod_annotations": {},
        "service_type": "ClusterIP"
      }
    }
  }
}
```

**Rules:**

1. Only the active environment `target_type` key is applied (`kubernetes`, `stub`, …).
2. Extensions are **frozen on the release** with `process_snapshot` (same lifecycle as command/quantity).
3. Stage/push **validates** extensions against the target’s JSON Schema (or equivalent).
4. Precedence: process extension field > env `target_config.defaults` > target binary defaults.
5. Stub ignores unknown extension blocks; may accept empty `stub: {}`.

#### Capabilities discovery

```http
GET /v1/targets/{type}/capabilities
# or GET /v1/projects/{project}/environments/{env}/target/capabilities
```

Response (illustrative):

```json
{
  "type": "kubernetes",
  "supports": ["deploy", "logs", "status", "scale", "health.http", "health.tcp", "extensions"],
  "extension_schema": { "$schema": "https://json-schema.org/...", "type": "object", "properties": { ... } }
}
```

CLI: `launchpad target capabilities` (ambient env) for human/agent discovery.

#### DeployRequest evolution

```go
type DeployRequest struct {
    Project     domain.Project
    Service     domain.Service
    Environment domain.Environment
    Release     domain.Release
    Processes   []domain.Process // includes Health + TargetExtensions from snapshot
    Config      map[string]string
}
```

Domain `Process` / `ProcessSnapshot` gain `Health` and `TargetExtensions` (typed map or `json.RawMessage` per target key). Control plane does not interpret kube-specific fields beyond validation dispatch.

### Approaches considered

| | Approach | Verdict |
|--|----------|---------|
| **A** | Portable fields + env defaults + namespaced snapshotted extensions + capabilities schema | **Recommended** |
| B | Free-form YAML blob on environment only | Too coarse; no per-process resources |
| C | Promote every kube field to first-class domain | Reject — multi-target poison |
| D | Raw manifest escape hatch as primary UX | Reject — Launchpad is not Helm |

### Acceptance

- Setting `kubernetes.resources.requests.memory` on `web` appears in the next release snapshot and on the Deployment.
- Staging an invalid extension key for the env’s target type returns **422** with a problem+json hint.
- Capabilities endpoint lists health + extensions for kubernetes; stub lists a minimal set.
- Promote/rollback preserve process topology including extensions; config still re-resolves on promote.

---

## Cross-slice invariants

1. **Snapshot completeness:** `process_snapshot` must include everything needed to build target process specs: command, quantity, expose, health, target_extensions.
2. **No live reload:** Worker derives `DeployRequest` only from the release (+ environment identity/target_config for *where*, not for process desired state).
3. **Config materialization** uses only `config_resolved` from the release.
4. **Destroy** cleans managed workloads + config objects for the service in that environment.
5. **Conformance suite** grows assertions as each slice lands (stub always; kind e2e for k8s-specific).

---

## Implementation slices (ADM queue)

Order is logical dependency, not hard blocking — each slice can ship alone if scoped tightly. Recommended order:

| Order | Queue ID | Slice | Depends on |
|-------|----------|-------|------------|
| 1 | `process-commands` | Mutations, command shell form, Procfile apply | Existing process snapshot |
| 2 | `deploy-health` | Portable health, probes, timeout in target_config | Process mutation path ideal but can hardcode web health via API only if needed |
| 3 | `release-config-materialization` | Immutable config Secrets | Independent of 1–2 |
| 4 | `target-extensions` | Extensions + capabilities + validation | Cleaner after process mutations exist |

Each ADM run: write/refresh plan under `docs/superpowers/plans/`, implement one queue ID, verify (`mise exec -- make test`, build, vet; stub e2e if deploy path changes), open PR. Update DOMAIN “shipped” notes when behavior lands.

**Do not half-build:** shipping only CLI flags without snapshot fields, or extensions without release freeze, is out of policy.

---

## Domain impact summary

| Entity / area | End-state change |
|---------------|------------------|
| Process | + health; command semantics fixed; mutations staged |
| ProcessSnapshot | + health, target_extensions; command meaning clarified |
| Release | Snapshot remains sole deploy desired state |
| Environment.target_config | + deploy_timeout, defaults; still backend-specific |
| Target (K8s) | Immutable config objects; probes; apply extensions |
| Target interface | Process carries health/extensions; capabilities API |
| Changeset | Process set/unset/apply change kinds |

See `docs/DOMAIN.md` sections Process, Release, Target Interface (runtime depth).

---

## Test strategy (all slices)

- **Unit:** snapshot expand; command shell form; health→probe mapping; secret name hash stability; extension validate accept/reject.
- **Store/API:** process upsert/delete; release snapshot fields round-trip; capabilities route.
- **Target:** fake clientset tests for Secret immutability, probes present, resources applied; timeout failure.
- **Conformance:** extend stub suite; kind e2e optional for probes/resources.
- **CLI smoke:** process set + deploy --wait on stub.

---

## Open questions (resolved defaults)

| Topic | Default for implementation |
|-------|----------------------------|
| Command form | Shell form `/bin/sh -c` |
| Health default | No probe unless configured; type=http defaults path `/healthz` |
| Liveness | Not in v1 |
| Config object naming | Content hash of canonical config map |
| Extension unknown keys | Reject at stage/push for active target type |
| `release` Procfile entry | quantity=0 (stored, not deployed) until release-phase jobs |
| ConfigMap/Secret split | Deferred |
| Control-plane HTTP health check | Not in v1 |

Human may override before a slice’s implementation PR; change the design doc and DOMAIN in that PR.

---

## Approval

- [x] End-state model from product discussion 2026-07-20 (recommended paths A across all slices)
- [ ] Per-slice implementation: ADM/self-approve plan under feature protocol when queue item is taken
