# Persona feedback and session logs

## Persona feedback

Feedback files from ADM persona runs land here:

```text
YYYY-MM-DD-<feature-or-run>.md
```

See [`../PERSONA-SCRIPTS.md`](../PERSONA-SCRIPTS.md) and [`docs/AUTONOMOUS-MODE.md`](../../../AUTONOMOUS-MODE.md) (persona + L1.5 stack dogfood).

## Session logs (optional)

For multi-hour **integration-stack** or **queue-drain** ADM runs, agents may also write:

```text
SESSION-YYYY-MM-DD.md
```

Suggested contents:

- Named mode and budget  
- Integration branch name  
- Feature PRs merged into integration  
- Next ready QUEUE id (or “drain complete”)  
- Hard stops / open decisions  
- Deferred rows with **Decision needed** one-liners  

Not required for single-feature mode. Pair with `scripts/adm-status` at session start/end.
