# Copy/Paste This Into GitHub Issue

---

**Title:** External bead sync gap: Gas Town doesn't auto-pull externally created beads

**Labels:** bug, architecture

---

## Problem

When an external agent (outside Gas Town) creates a bead and pushes it to the repo, Gas Town's mayor/rig clone doesn't automatically see it.

## Current Behavior

Sync only happens on:
- Polecat/crew/refinery spawn (NeedsPreSync=true triggers git pull + bd sync)
- Manual `bd sync` in mayor/rig

Sync does **NOT** happen:
- Periodically/automatically
- When Mayor is running continuously
- When external agents push new beads

## Root Cause

From `internal/daemon/lifecycle.go:451-457`:
```go
// getNeedsPreSync determines if a workspace needs git sync before starting.
func (d *Daemon) getNeedsPreSync(config *beads.RoleConfig, parsed *ParsedIdentity) bool {
    // ...
    switch parsed.RoleType {
    case "refinery", "crew", "polecat":
        return true
    default:
        return false  // ← Mayor, Deacon, Witness return false
    }
}
```

**Analysis:**
- Only refinery, crew, polecat have `NeedsPreSync=true`
- Mayor, deacon, witness return `false` - don't sync
- No background sync mechanism exists

## Example Scenario

```bash
# External developer (outside Gas Town)
git clone https://github.com/you/myproject
bd create "Critical bug"  # myproject-789
git push

# Gas Town's mayor/rig clone: STALE
# Won't see myproject-789 until:
# 1. Someone spawns a worker (triggers pre-sync), OR
# 2. Manual: cd ~/gt/myproject/mayor/rig && bd sync
```

## Impact

- **Invisible issues**: Externally created beads don't appear in Gas Town
- **Manual intervention required**: Must remember to sync manually
- **Breaks cross-tool workflow**: Assumes external tools can create beads that Gas Town orchestrates
- **Stale coordination**: Mayor makes decisions on incomplete information

## Possible Solutions

### Option 1: Mayor Pre-Sync (Quick Fix)
Make Mayor role sync on restart by adding it to the `NeedsPreSync=true` list.

**Pros:** Simple 1-line change, gets partial improvement
**Cons:** Still not real-time (only syncs on Mayor restart)

### Option 2: Patrol-Based Sync (Background)
Add periodic sync to Deacon patrol (every 5-10 minutes).

**Pros:** Near real-time visibility, works even if Mayor never restarts
**Cons:** Background overhead, network traffic on every poll

### Option 3: Git Polling (Smart)
Poll `git fetch` and only sync when remote has changes.

**Pros:** Only syncs when needed (efficient)
**Cons:** Still polling overhead, requires tracking per-rig remote state

### Option 4: Webhook Integration (Advanced)
GitHub webhook on push → triggers immediate sync in Gas Town.

**Pros:** Real-time sync (millisecond latency), zero polling overhead
**Cons:** Infrastructure complexity, requires HTTP server in daemon

## Recommended Approach

**Phase 1:** Quick fix - add Mayor to `NeedsPreSync=true`
**Phase 2:** Add patrol-based sync to Deacon (10 min interval)
**Phase 3:** Future - webhook support for real-time sync

## Files Involved

- `internal/daemon/lifecycle.go:451-457` (getNeedsPreSync)
- `internal/daemon/lifecycle.go:532-570` (syncWorkspace)
- `internal/beads/fields.go:520-522` (NeedsPreSync field)

## Related

- Beads issue: `gt-rgs4z`
- Full documentation: `docs/issues/external-bead-sync-gap.md`

## Workaround

Until fixed:
```bash
# Manual periodic sync
*/10 * * * * cd ~/gt/*/mayor/rig && bd sync

# Or alias
alias gt-sync='for rig in ~/gt/*/mayor/rig; do (cd $rig && bd sync); done'
```
