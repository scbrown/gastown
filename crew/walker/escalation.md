# Walker Escalation Protocol

## Priority Matrix

| Level | Priority | Route | Action |
|-------|----------|-------|--------|
| **CRITICAL** | P0 | arnold + Stiwi | Act immediately, notify both |
| **HIGH** | P1 | arnold | Notify, wait for response, continue if unblocked |
| **MEDIUM** | P2 | arnold (mail) | Async, no rush |
| **LOW** | P3 | Patrol report | Include in next brief |

## When to Escalate

- **Architecture decisions** that affect gastown's direction
- **Pitch rejections** you disagree with (escalate to Stiwi, not around arnold)
- **Cross-rig conflicts** where gastown needs aren't being met
- **Blocked >15 minutes** on the same problem after trying alternatives
- **Unclear requirements** after checking docs and code
- **3+ failed attempts** at resolving an issue

## When NOT to Escalate

- Filing pitch beads (that's your job)
- Updating planning docs
- Routine patrol observations
- Creating task beads for approved work
- Sending patrol reports

## How to Escalate

### To Arnold (Keeper) — Primary

```bash
gt mail send aegis/crew/arnold -s "[P{0-3}] Brief summary" -m "
Context: <what you were doing>
Observed: <what happened>
Tried: <what you attempted>
Proposed: <your recommendation>
Need: <what you need from arnold>"
```

### To Stiwi (Human) — For Hard Limits

```bash
gt mail send --human -s "[P{0-3}] Brief summary" -m "
Context: <what you were doing>
Observed: <what happened>
Tried: <what you attempted>
Proposed: <your recommendation>
Need: <what you need from Stiwi>"
```

### To Goldblum (Orchestrator) — For Dispatch Needs

```bash
gt mail send aegis/crew/goldblum -s "Dispatch request: <topic>" -m "
Approved pitch: <bead-id>
Scope: <what needs executing>
Priority: <P level>
Suggested: <polecat dispatch recommended>"
```

### To Witness — For Pipeline Issues

```bash
gt nudge witness "<brief issue description>"
```

## After Escalating

1. If you can continue with other work (other patrol phases) — continue
2. If completely blocked — `gt handoff` with context notes
3. Do NOT sit idle waiting for response
4. Note the escalation in your next patrol report
