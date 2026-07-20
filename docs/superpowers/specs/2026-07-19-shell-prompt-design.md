# Shell prompt awareness (project@env)

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve — ADM) |
| **Date** | 2026-07-19 |
| **Domain** | CLI DX only |
| **Scope** | Print resolved project@env for shell prompts; optional shell-init snippets |
| **Queue** | `shell-prompt` |
| **Base branch** | `adm/queue-2026-07-19` |

---

## Goal

Make ambient Launchpad context visible in the terminal without a full `context` dump.

```bash
launchpad prompt
# my-api@dev

launchpad prompt --format short   # same: project@env (default)
launchpad prompt --format long    # project=my-api env=dev

# Optional eval snippet
eval "$(launchpad shell-init zsh)"
# → PS1 shows (lp:my-api@dev) when project is set
```

**Success:** Zero network calls; uses same config resolution as other CLI commands.

---

## Approaches considered

### A. `prompt` + `shell-init` commands (recommended)

Pure local config resolution; no API. `shell-init` emits bash/zsh-safe functions.

**Pros:** Instant; no token required; progressive disclosure.  
**Cons:** Users must opt into PS1 (correct for progressive disclosure).

### B. Hook only via docs (“put `launchpad context` in PS1”)

**Cons:** Too slow/verbose; context prints multi-line.

### C. OSC/title bar only

**Cons:** Less portable; skip.

**Recommendation:** A.

---

## Scope

### In scope

- `launchpad prompt` with `--format short|long` (default short)
- Empty project → empty stdout (exit 0) so PS1 stays clean
- `launchpad shell-init [bash|zsh]` prints eval-able snippet defining `_launchpad_prompt` and optional PS1 prefix
- Unit tests for format helpers
- README one-liner

### Out of scope

- Fish shell (docs note later)
- Auto-install into rc files
- API health in prompt

---

## Spec self-review

| Check | Result |
|-------|--------|
| No placeholders | Pass |
| Consistency | Pass |
| Single slice | Pass |
| DOMAIN | Unchanged |
| MVP boundary | Pass |
| Tests | Unit for format |

**Status:** Approved (self-approve — ADM)
