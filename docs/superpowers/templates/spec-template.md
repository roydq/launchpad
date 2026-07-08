# <Feature Name>

| Field | Value |
|-------|-------|
| **Status** | Draft |
| **Date** | YYYY-MM-DD |
| **Domain spec** | `docs/DOMAIN.md` |
| **Scope** | <one-line scope summary> |

---

## Goal

<What this feature delivers. Include the developer-facing success criteria — CLI commands, API calls, or observable behavior.>

```bash
# Example success criteria
launchpad <command> <args>
```

---

## Approaches Considered

### A. <Approach name> (recommended)

<Description>

**Pros:** <bullets>  
**Cons:** <bullets>

### B. <Alternative>

<Description>

**Pros:** <bullets>  
**Cons:** <bullets>

**Recommendation:** <A/B and why>

---

## Scope

### In scope

- <item>

### Out of scope (this feature)

- <item>

### Deferred (future phase — do not half-build)

- <item — reference AGENTS.md deferred list if applicable>

---

## Domain impact

<New or changed entities, lifecycle transitions, invariants. Link to DOMAIN.md sections that change.>

| Entity | Change |
|--------|--------|
| <Entity> | <create / modify / unchanged> |

**Invariants to preserve:**

- <invariant>

**Invariants to add:**

- <invariant>

---

## API sketch

<REST paths, request/response shapes. RFC 7807 errors for failure cases.>

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/v1/...` | <description> |

---

## Schema sketch

<New tables, columns, indexes. Reference migration file name if known.>

---

## Target / worker impact

<Changes to `internal/target.Target`, deploy FSM, job types — or "none".>

---

## Test strategy

- **Unit:** <packages and what they cover>
- **Integration:** <store tests, API handler tests>
- **Smoke:** <stub deploy path if deploy flow changes>

---

## Open questions

- [ ] <question — resolve before implementation>

---

## Approval

- [ ] Design reviewed and approved (required before implementation)