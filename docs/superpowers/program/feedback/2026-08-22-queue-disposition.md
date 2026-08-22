# Queue disposition — remaining deferred items (2026-08-22)

| Field | Value |
|-------|-------|
| **Trigger** | Human asked to decide remaining QUEUE rows after the 2026-07-20 drain |
| **Branch** | `main` (docs only; no ADM authorization, no code) |
| **Result** | Promoted integrations (yaml → MCP); parked OIDC, multi-service, and bindings |

## Context

The 2026-07-20 ADM drain left five `deferred` rows and an empty ready queue. Since then, `main` has only process/docs/hygiene work. Core DX (Track A) and confidence (Track B) for a **single primary service** are shipped, including runtime depth.

DX-VISION still says A+B until dogfood is boring, and forbids multi-service theater. Identity phase 1 (tokens, service accounts, audit) already answers “who deployed?” for CLI/CI/agents.

## Decisions

| ID | Call | Why |
|----|------|-----|
| **launchpad-yaml** | **Next (`ready`)** | Closest product primitive to the “mise of runtime” metaphor. DOMAIN already forbids GitOps reconcile: import sets desired state; CLI/API remain live mutation. v1 is the **current** model only so we do not wait on phase 3. |
| **mcp-server** | **Ready, second** | Agent-native surface on a now-stable API. Keep it a thin OpenAPI/CLI wrapper — no new entities. Sequence after yaml so `apply` can be a first-class tool instead of 20 curl verbs. |
| **oidc-design** | **Park** | No interactive-login user yet. Bootstrap + workspace tokens are enough. When hosted or a second human shows up: **generic OIDC first**, Azure/Google as presets. |
| **multi-service** | **Park** | Web+worker is already a process, not a service. Trigger is a second independently versioned artifact in one project. Still needs a full spec (ReleaseSet, coordination, rollback depth). |
| **bindings** | **Park (fold)** | Empty without multiple services. Spec with multi-service as slice 2, not as its own program. |

## What this is not

- Not ADM authorization to implement yaml or MCP.
- Not a DOMAIN phase-number rewrite (3 still precedes 4; 6 can slice v1 of today’s model).
- Not a Track C TUI/docs/dashboard start — those stay Later until the project file + MCP exist.

## Next session

1. Spec `launchpad.yaml` v1 (`docs/superpowers/specs/YYYY-MM-DD-launchpad-yaml-design.md`) under `/launchpad-feature` or ADM single-feature.
2. Spec MCP against that manifest + existing OpenAPI.
3. Leave parked rows untouched until their start triggers fire.
