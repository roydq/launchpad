# Identity: principals, tokens, and audit (phase 1)

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve — agent feature program) |
| **Date** | 2026-07-14 |
| **Domain** | `docs/DOMAIN.md` |
| **Scope** | Principal model, token↔principal, release attribution, audit events; **no OIDC yet** |

---

## Goal

Multi-user and multi-agent control planes need a **who** for every mutating action—not only workspace-scoped tokens.

**Phase 1 success:**

1. **Principal** exists (`user` | `service_account`) and can belong to a workspace (membership + role).
2. **API tokens** reference a principal; `CreateToken` mints a **service account** principal for CLI/agent tokens.
3. Auth middleware puts `workspace_id`, `token_id`, `principal_id`, and scopes on context.
4. New releases record **created_by** principal/token; an **audit_events** row is written for deploy/promote/rollback/push.
5. Release JSON includes `created_by` when known.
6. Bootstrap token still works (no principal); local dogfood unbroken.

```bash
# After minting a token named "ci-bot":
launchpad deploy --image app:1 --wait
launchpad releases show 1
# → created_by.kind=service_account, created_by.display_name=ci-bot
```

---

## Approaches

### A. Principals + membership + audit now; OIDC later (recommended)

Generic OIDC (Azure AD, Google, …) becomes “login that creates/links User + Identity”. Phase 1 does not implement IdP callbacks.

### B. OIDC first without principals

Reject — login without durable principal/audit does not answer “who promoted prod?”

**Recommendation:** A.

---

## Scope

### In scope (this feature)

- Domain types: Principal, WorkspaceMember, AuditEvent; APIToken.PrincipalID
- Migration: tables + `releases.created_by_*` + `api_tokens.principal_id`
- Auth context: token ID + principal ID (fix existing `ContextTokenID` which was unused)
- CreateToken → service account principal + workspace membership
- Attribute releases + audit on release enqueue paths
- Release list/show DTO `created_by`
- Optional `GET /v1/audit` (workspace-scoped, recent events)
- DOMAIN + DX-VISION Track D updates

### Out of scope

- OIDC / Azure AD / Google login
- User invite UI, password auth
- SCIM, group→role mapping
- Fine-grained per-env RBAC
- Replacing scope checks with roles only (scopes stay; roles stored for future)
- Renaming all `team_id` → `workspace_id` (document; partial if touch points are free)

### Deferred (phase 2+)

- `identities` table + OIDC providers config
- Human User create/link flows
- Session cookies / device flow for CLI `auth login`
- Policy engine beyond scopes

---

## Domain impact

| Entity | Change |
|--------|--------|
| Principal | **New** — `user` \| `service_account` |
| WorkspaceMember | **New** — principal ∈ workspace + role |
| APIToken | **Modify** — optional `principal_id` |
| Release | **Modify** — optional `created_by_principal_id`, `created_by_token_id` |
| AuditEvent | **New** — append-only action log |
| Identity (OIDC) | Deferred schema stub optional; **not** required phase 1 |

**Roles (stored, lightly used phase 1):** `owner`, `admin`, `operator`, `viewer`  
Token scopes remain authoritative for API authorization in phase 1. Membership role is set from scopes on CreateToken (`admin` scope → `admin` role, else `operator`).

**Invariants:**

- Tokens remain workspace-scoped.
- Principals may eventually span workspaces via multiple memberships.
- Audit and release attribution must not block deploy if principal is missing (bootstrap).

---

## API sketch

### Token create (existing, extended response)

```
POST /v1/tokens
→ { id, name, workspace, scopes, token, principal_id, principal_kind }
```

### Release (extended)

```json
{
  "version": 3,
  "created_by": {
    "principal_id": "...",
    "kind": "service_account",
    "display_name": "ci-bot",
    "token_id": "..."
  }
}
```

`created_by` omitted when unknown (bootstrap path).

### Audit list (new)

```
GET /v1/audit?limit=50
Scope: project:read (or admin)
→ [ { id, action, resource_type, resource_id, principal_id, token_id, detail, created_at } ]
```

Actions: `release.create`, `release.promote`, `release.rollback`, `changeset.push` (push uses same enqueue; action distinguishes via description/plan when possible).

---

## Auth context

| Key | Meaning |
|-----|---------|
| `team_id` / workspace | Existing workspace ID (legacy name kept) |
| `scopes` | Existing |
| `token_id` | Set for real API tokens; unset for bootstrap |
| `principal_id` | Set when token has principal; unset for bootstrap |

---

## Test strategy

- Store: create principal + token FK + audit insert
- Auth: Authenticate sets token/principal; CreateToken links SA
- Service: CreateRelease with context principal → release fields + audit row
- API/response: created_by on list when present
- Bootstrap deploy still works without principal

---

## Self-review checklist

- [x] OIDC explicitly deferred
- [x] Local bootstrap preserved
- [x] Agents = service accounts via tokens
- [x] No multi-service / secrets coupling
