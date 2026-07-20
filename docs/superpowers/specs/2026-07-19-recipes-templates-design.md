# Recipes / `launchpad new` templates

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve — ADM) |
| **Date** | 2026-07-19 |
| **Domain** | `docs/DOMAIN.md` (CLI DX only; no entity change) |
| **Scope** | CLI recipe catalog + `launchpad new` that bootstraps a project from a named template |
| **Queue** | `recipes-templates` |
| **Base branch** | `adm/queue-2026-07-19` |

---

## Goal

Shorten day-one path further: one command creates a project with sensible defaults and project-local context, without inventing a second deploy dialect.

```bash
launchpad new list
# hello-stub   Stub target hello project (default)
# web-stub     Stub web service with PORT=8080 staged

launchpad new hello-stub my-api
# → creates project my-api --target stub
# → launchpad use my-api (project-local .launchpad/config when run from a git/work dir)
# → optional: stage default image + config per recipe
# → prints next steps (diff / deploy --wait)

launchpad new my-api
# → recipe defaults to hello-stub when name looks like a project (see CLI rules)
```

**Success criteria:**

- `launchpad new list` prints available recipes with one-line descriptions.
- `launchpad new <recipe> <name>` creates the project via existing API, sets context, applies recipe steps.
- Does not lengthen the README 60s path; recipes are progressive disclosure.
- No server-side recipe registry; no new domain entities.

---

## Approaches considered

### A. CLI-embedded recipes + existing create/stage APIs (recommended)

Recipes are Go structs (or small embedded JSON) in the CLI package. `new` orchestrates `CreateProject`, optional `StageChanges` / `config set` / `image`, and local context write.

**Pros:** Zero control-plane surface; easy to test; matches “thin API, rich CLI”; keeps day-one short.  
**Cons:** Adding recipes requires a CLI release (acceptable for MVP).

### B. Server-side recipe catalog API

`GET /v1/recipes`, `POST /v1/projects?recipe=…`.

**Pros:** Multi-client reuse.  
**Cons:** Premature API surface; recipes are mostly client UX; conflicts with MVP “no half-built platform.”

### C. File scaffolding only (copy examples/)

Generate local files without calling the API.

**Pros:** Offline.  
**Cons:** Does not create control-plane project; weak dogfood vs current `projects create`.

**Recommendation:** A.

---

## Scope

### In scope

- `launchpad new list`
- `launchpad new <recipe> <project-name>` (and ergonomic arg parse: `new <name>` → default recipe)
- Built-in recipes:
  - **`hello-stub`** (default): `projects create --target stub`; set project context; stage image `hello:v1` (or recipe image).
  - **`web-stub`**: like hello-stub plus stage service config `PORT=8080` (plain).
- Flags: `--target` override (default from recipe), `--namespace` for k8s recipes later, `--no-stage` (create only), `--use` / always write context like `use`.
- Unit tests for recipe registry + arg parse; CLI smoke if easy.
- Docs: README one-liner under Solo workflow; DX-VISION + QUEUE.

### Out of scope

- Kubernetes-specific recipes (can add later as `web-k8s`).
- Interactive wizard / TUI.
- Deploy inside `new` (user runs `deploy --wait` — keeps “diff before trust”).
- Server API for recipes.
- `launchpad.yaml` manifest (deferred domain-6).

### Deferred

- Community recipes directory, recipe versioning, multi-service recipes.

---

## Domain impact

None. Uses existing Project / Environment bootstrap on create and existing staging.

| Entity | Change |
|--------|--------|
| Project / Environment / Service | Unchanged (create path only) |

---

## CLI sketch

| Command | Behavior |
|---------|----------|
| `new list` | List recipe id + short description |
| `new <recipe> <name>` | Apply recipe |
| `new <name>` | Apply default recipe `hello-stub` when first arg is not a known recipe id |

Arg rules:

1. If first arg is `list` → list.
2. If first arg is a known recipe and second arg present → recipe + name.
3. If first arg is a known recipe and no name → error “project name required”.
4. If first arg is unknown and no second → treat as project name with default recipe.
5. If first arg unknown and second present → error unknown recipe.

Flags:

| Flag | Default | Notes |
|------|---------|-------|
| `--target` | recipe default (`stub`) | Passed to create |
| `--namespace` | `default` | Passed to create |
| `--no-stage` | false | Skip image/config staging |
| `--dir` | `.` | Where to write project-local `.launchpad/config` if writing local |

After success print:

```text
created project my-api (recipe hello-stub, target stub)
context: my-api @ dev
staged: image hello:v1
next: launchpad diff && launchpad deploy --wait
```

---

## Recipe definition (internal)

```go
type Recipe struct {
    ID          string
    Description string
    TargetType  string // default target
    Namespace   string
    Image       string            // optional staged image
    ServiceConfig map[string]string // optional plain service-layer keys
}
```

No secret values in recipes for v1.

---

## Test strategy

- **Unit:** recipe lookup, arg parsing table tests, apply steps with mocked apiclient or thin orchestration tests.
- **Integration:** not required (uses existing create/stage).
- **Manual / persona:** S1 variant using `launchpad new` instead of `projects create`.

---

## Spec self-review (ADM)

| Check | Result |
|-------|--------|
| 1. No placeholders | Pass |
| 2. Internal consistency | Pass |
| 3. Single plan scope | Pass — CLI only |
| 4. No DOMAIN contradiction | Pass |
| 5. MVP boundary | Pass — not multi-service / yaml / server catalog |
| 6. Recommended path recorded | A |
| 7. Test strategy | Unit + optional persona |

**Status:** Approved (self-approve — ADM)

---

## Open questions

None blocking.
