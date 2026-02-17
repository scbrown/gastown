# Walker Baseline — Gastown Rig Health

## What "Healthy" Looks Like

A healthy gastown rig has:

- **Clean main branch** — All tests passing, linting clean
- **Active polecat pipeline** — Witness alive, polecats completing work
- **Current planning docs** — Updated within the last week
- **Low bead backlog** — No P0/P1 beads unassigned for >24h
- **Formula coverage** — Key workflows have molecule definitions
- **Documentation currency** — Concepts and design docs match current code

## Rig Components

| Component | Health Indicator | Check |
|-----------|-----------------|-------|
| CLI binary | Builds cleanly | `make build` |
| Test suite | All passing | `make test` |
| Linting | No violations | `make lint` (if available) |
| Formulas | Load without error | `gt formula list` |
| Witness | Session alive | `gt peek gastown/witness` |
| Refinery | Session alive | `gt peek gastown/refinery` |
| Polecats | Completing work | `bd list --status=in_progress` |

## Bead Health Thresholds

| Metric | Healthy | Warning | Critical |
|--------|---------|---------|----------|
| Open P0 beads | 0 | 1 | >1 |
| Open P1 beads | <3 | 3-5 | >5 |
| Stale in-progress (>48h) | 0 | 1-2 | >2 |
| Unassigned P2 beads | <10 | 10-20 | >20 |

## Planning Doc Currency

| Document | Expected Update Frequency | Location |
|----------|--------------------------|----------|
| `stiwi-wants.md` | When new directives arrive | `docs/plans/` |
| `rig-plans.md` | Every patrol session | `docs/plans/` |
| `ideas-and-directions.md` | When observations accumulate | `docs/plans/` |

## Known State (Initial)

This is walker's first deployment. Baseline observations pending from first patrol.

- Formula count: TBD (audit needed)
- Active molecule types: TBD
- Documentation gaps: TBD
- Desire-path bead count: TBD

## Recovery Procedures

### Build Broken
1. Check `git log --oneline -5` for recent changes
2. Run `make build` to identify the error
3. If obvious fix: commit directly
4. If unclear: file P1 bead, escalate to arnold

### Polecat Pipeline Stalled
1. Check witness health: `gt peek gastown/witness`
2. Check for stuck polecats: `bd list --status=in_progress`
3. Nudge witness if unresponsive: `gt nudge witness "Health check"`
4. Escalate if persistent: `gt escalate "Pipeline stalled" -s HIGH`

### Planning Docs Stale
1. Run patrol protocol Phase 2
2. Update all three planning docs
3. Mail arnold with summary of changes
