# External Bead Sync Gap: Gas Town Doesn't Auto-Pull Externally Created Beads

**Issue ID:** `gt-rgs4z`
**Type:** Bug
**Priority:** Medium
**Labels:** sync, architecture, external-integration

---

## Problem

When an external agent (outside Gas Town) creates a bead and pushes it to the repo, Gas Town's mayor/rig clone doesn't automatically see it.

This creates a synchronization gap where externally created issues are invisible to Gas Town until an explicit sync trigger occurs.

## Current Behavior

Sync only happens in these scenarios:

### Automatic Sync (Worker Spawn)
- **Polecat spawn**: When `gt sling` creates a polecat, pre-sync runs
- **Crew member start**: When crew session starts, pre-sync runs
- **Refinery start**: When refinery starts, pre-sync runs

All three roles have `NeedsPreSync=true` which triggers:
```bash
git fetch origin
git pull --rebase origin <default-branch>
bd sync
```

### Manual Sync (Human Intervention)
```bash
cd ~/gt/myproject/mayor/rig
bd sync
```

### What Does NOT Trigger Sync

- ❌ Periodically/automatically
- ❌ When Mayor is running continuously
- ❌ When external agents push new beads
- ❌ When Deacon/Witness are monitoring
- ❌ On git webhook events

## Root Cause

### Code Location: `internal/daemon/lifecycle.go:451-457`

```go
// getNeedsPreSync determines if a workspace needs git sync before starting.
func (d *Daemon) getNeedsPreSync(config *beads.RoleConfig, parsed *ParsedIdentity) bool {
    if config != nil {
        return config.NeedsPreSync
    }

    // Fallback: roles with persistent git clones need pre-sync
    switch parsed.RoleType {
    case "refinery", "crew", "polecat":
        return true
    default:
        return false  // ← Mayor, Deacon, Witness return false
    }
}
```

### Analysis

1. **Only worker roles sync**: refinery, crew, polecat have `NeedsPreSync=true`
2. **Coordinator roles don't sync**: mayor, deacon, witness return `false`
3. **No background sync mechanism**: No polling, webhooks, or periodic sync exists
4. **Mayor has stale view**: Mayor can run for days without seeing external updates

## Example Scenario

### External Developer Workflow

```bash
# Developer outside Gas Town
git clone https://github.com/you/myproject
cd myproject

# Create issue using beads
bd create "Critical bug found in parser"
# Creates: myproject-789

# Commit and push
git add .beads/issues.jsonl
git commit -m "Add critical bug issue"
git push
```

### Gas Town Perspective

```bash
# Gas Town's mayor/rig clone: STALE
cd ~/gt/myproject/mayor/rig
bd list
# Does NOT show myproject-789 yet!

# Won't see it until:
# Option 1: Someone spawns a worker
gt sling myproject-123 myproject
# → Polecat spawns → pre-sync runs → sees myproject-789

# Option 2: Manual sync
cd ~/gt/myproject/mayor/rig && bd sync
# → Pulls updates → sees myproject-789
```

## Impact

### User Experience
- **Invisible issues**: Externally created beads don't appear in Gas Town
- **Manual intervention required**: Humans must remember to sync manually
- **Confusion**: "Why can't Gas Town see the issue I just created?"

### Architectural Concerns
- **Breaks cross-tool workflow**: Assumes external tools can create beads that Gas Town orchestrates
- **Violates expectations**: Users expect issue trackers to show recent issues
- **Race conditions**: External issue created → Mayor assigns work → worker syncs → "surprise, different issue appeared"

### Operational Issues
- **Stale coordination**: Mayor makes decisions on incomplete information
- **Missed work**: External urgent issues invisible until next worker spawn
- **Team friction**: Gas Town users vs external users have different views

## Possible Solutions

### Option 1: Mayor Pre-Sync (Simple)

**Make Mayor role sync on restart:**

```go
// In internal/daemon/lifecycle.go:451-457
case "refinery", "crew", "polecat", "mayor":  // Add mayor
    return true
```

**Pros:**
- Simple 1-line change
- Mayor sessions restart periodically (context compaction, handoff)
- Consistent with other persistent clone roles

**Cons:**
- Still not real-time (only syncs on restart)
- Mayor might run for hours without restart

### Option 2: Patrol-Based Sync (Background)

**Add periodic sync to Deacon patrol:**

```go
// Every 5-10 minutes, sync all mayor/rig dirs
func (d *Daemon) syncMayorRigs() {
    rigs := listRigs(townRoot)
    for _, rig := range rigs {
        mayorRig := filepath.Join(townRoot, rig, "mayor/rig")
        exec.Command("git", "fetch", "origin").Dir(mayorRig).Run()
        exec.Command("bd", "sync").Dir(mayorRig).Run()
    }
}
```

**Pros:**
- Near real-time visibility (5-10 min latency)
- Works even if Mayor never restarts
- Consistent with Deacon's monitoring role

**Cons:**
- Background overhead (git operations every 5 min)
- Network traffic on every poll
- Complexity: patrol loop management

### Option 3: Git Polling (Smart)

**Poll `git fetch` and only sync on changes:**

```go
// Check remote for new commits, only sync if changed
func (d *Daemon) checkAndSyncIfChanged(mayorRig string) {
    // git fetch (lightweight)
    exec.Command("git", "fetch", "origin")

    // Check if local is behind remote
    output := exec.Command("git", "rev-list", "--count", "HEAD..origin/main")
    if behindCount > 0 {
        exec.Command("git", "pull", "--rebase")
        exec.Command("bd", "sync")
    }
}
```

**Pros:**
- Only syncs when needed (efficient)
- Detects external changes quickly
- Lower overhead than blind polling

**Cons:**
- Still polling overhead
- Requires network access
- Complexity: tracking per-rig remote state

### Option 4: Webhook Integration (Advanced)

**GitHub webhook triggers sync:**

```bash
# GitHub webhook on push → POST to Gas Town daemon
POST /api/webhook/sync?rig=myproject

# Daemon immediately syncs that rig
```

**Pros:**
- Real-time sync (millisecond latency)
- Zero polling overhead
- Scales to many rigs

**Cons:**
- Requires webhook infrastructure
- Network configuration (firewall, ports)
- Security considerations (webhook auth)
- Complexity: HTTP server in daemon

## Recommended Approach

**Phase 1: Quick Fix (Option 1)**
- Add Mayor to `NeedsPreSync=true` list
- Gets partial improvement immediately
- Low risk, 1-line change

**Phase 2: Background Sync (Option 2 or 3)**
- Add patrol-based sync to Deacon
- Poll every 10 minutes or on fetch changes
- Provides near real-time sync

**Phase 3: Future Enhancement (Option 4)**
- Add webhook support when/if needed
- Real-time sync for production environments
- Can be added without breaking existing sync

## Files Involved

| File | Lines | Description |
|------|-------|-------------|
| `internal/daemon/lifecycle.go` | 451-457 | `getNeedsPreSync()` - determines which roles sync |
| `internal/daemon/lifecycle.go` | 532-570 | `syncWorkspace()` - performs git pull + bd sync |
| `internal/beads/fields.go` | 520-522 | `NeedsPreSync` field definition in RoleConfig |
| `internal/rig/manager.go` | 552-570 | `initBeads()` - beads initialization during rig add |

## Related Issues

- Beads issue: `gt-rgs4z` (this issue)
- Related to beads sync architecture
- Related to cross-tool integration design

## Testing Considerations

### Manual Test Case

1. Clone repo externally, create bead, push
2. Check Gas Town mayor/rig - issue should NOT appear
3. Spawn polecat with `gt sling`
4. Check Gas Town mayor/rig - issue SHOULD appear now

### Automated Test

```go
func TestExternalBeadSync(t *testing.T) {
    // Setup Gas Town with rig
    // Create external clone
    // Add bead in external clone
    // Push to remote
    // Verify Gas Town doesn't see it (before fix)
    // Spawn worker
    // Verify Gas Town sees it (after worker pre-sync)
}
```

## Workaround for Users

Until fixed, users can:

```bash
# Add to cron or run periodically
*/10 * * * * cd ~/gt/*/mayor/rig && bd sync

# Or manual sync before checking for work
alias gt-sync='for rig in ~/gt/*/mayor/rig; do (cd $rig && bd sync); done'
```

---

**Filed:** 2026-01-12
**Reported by:** gastown/crew/claude
**Status:** Open
