# Convoy Leg Formula Support

## Related Issues

- **#288**: "gt sling should auto-attach mol-polecat-work when slinging to polecats"
  - Same pattern applied to regular polecat dispatch
  - Our work is the convoy-specific version of this
- **#355**: "gt sling with formula fails"
  - Bug with `--on` flag and variable auto-population
  - May need addressing for this feature to work smoothly

## Problem

Convoy formulas create leg beads but dispatch them with plain `gt sling <leg> <rig>` - no workflow formula is applied. This means polecats receive work without step-by-step guidance, leading to:

- Polecats going off-track (not following the intended analysis workflow)
- No crash-recovery (can't resume from last completed step)
- Inconsistent execution across legs

This is the same problem described in #288, but for convoy leg dispatch rather than general polecat dispatch.

## Solution

Allow convoy formulas to specify a workflow formula for leg execution, following the same pattern as `gt sling <formula> --on <bead> <rig>`.

## Architectural Context

```
CURRENT STATE:
  gt sling <bead> <rig>                    → No formula (bare bead)
  gt sling <formula> --on <bead> <rig>     → Formula applied (explicit)
  gt formula run <convoy>                  → Legs get bare beads (no formula)

PROPOSED (this work):
  gt formula run <convoy>                  → Legs get leg_formula applied

RELATED (#288):
  gt sling <bead> polecat                  → Auto-apply mol-polecat-work
```

Both #288 and this work address: **"When should formula application be implicit vs explicit?"**

## Design

### Option C: Convoy default + per-leg override

```toml
# Convoy-level default formula for all legs
[convoy]
leg_formula = "mol-nfr-leg"

[[legs]]
id = "testability"
title = "Testability Analysis"
# uses default mol-nfr-leg

[[legs]]
id = "security"
title = "Security Analysis"
formula = "mol-security-audit"  # override for this leg
```

### Dispatch Behavior

**Current:**
```bash
gt sling <leg-bead> <rig> -a "<description>" -s "<title>"
```

**Proposed:**
```bash
# If leg.formula or convoy.leg_formula is set:
gt sling <formula> --on <leg-bead> <rig> -a "<description>" -s "<title>"

# Otherwise (backwards compatible):
gt sling <leg-bead> <rig> -a "<description>" -s "<title>"
```

## Implementation

### Phase 1: Type Changes

**File: `internal/formula/types.go`**

```go
// Leg represents a parallel execution unit in a convoy formula.
type Leg struct {
	ID          string `toml:"id"`
	Title       string `toml:"title"`
	Focus       string `toml:"focus"`
	Description string `toml:"description"`
	Formula     string `toml:"formula"` // NEW: workflow formula for this leg
}

// ConvoyConfig holds convoy-level configuration.
type ConvoyConfig struct {
	LegFormula string `toml:"leg_formula"` // NEW: default formula for all legs
}

// Formula represents a parsed formula.toml file.
type Formula struct {
	// ... existing fields ...

	// Convoy-specific
	Convoy    *ConvoyConfig     `toml:"convoy"` // NEW: convoy configuration
	// ... rest of fields ...
}
```

### Phase 2: Dispatch Changes

**File: `internal/cmd/formula.go`** (~line 447)

```go
// Determine which formula to use for this leg
legFormula := leg.Formula
if legFormula == "" && f.Convoy != nil {
	legFormula = f.Convoy.LegFormula
}

// Build sling args
var slingArgs []string
if legFormula != "" {
	// Apply formula to leg bead (like reckoning dispatch)
	slingArgs = []string{
		"sling", legFormula, "--on", legBeadID, targetRig,
		"-a", leg.Description,
		"-s", leg.Title,
	}
} else {
	// Backwards compatible: just sling the bead
	slingArgs = []string{
		"sling", legBeadID, targetRig,
		"-a", leg.Description,
		"-s", leg.Title,
	}
}
```

### Phase 3: Create NFR Leg Formula

**File: `internal/formula/formulas/mol-nfr-leg.formula.toml`**

A workflow formula with steps:
1. `load-context` - Understand assigned NFR dimension
2. `explore-scope` - Find relevant files and recent changes
3. `analyze` - Systematic analysis per dimension checklist
4. `file-beads` - Create improvement beads for findings
5. `summarize` - Update leg bead with summary
6. `complete` - Submit via gt done

### Phase 4: Update NFR Reflection Formula

**File: `internal/formula/formulas/mol-nfr-reflection.formula.toml`**

Add convoy config:
```toml
[convoy]
leg_formula = "mol-nfr-leg"
```

## Testing

1. **Unit tests** - `internal/formula/parser_test.go`
   - Parse convoy with leg_formula
   - Parse leg with formula override
   - Verify formula resolution (leg > convoy > none)

2. **Integration test** - Manual
   - Run `gt formula run mol-nfr-reflection --rig=reckoning`
   - Verify legs are dispatched with `--on mol-nfr-leg`
   - Verify polecats follow step workflow

## Migration

- Backwards compatible: existing convoy formulas without `[convoy]` or `formula` fields continue to work
- No database changes required
- No config migration needed

## Files Changed

| File | Change |
|------|--------|
| `internal/formula/types.go` | Add `Formula` to Leg, add `ConvoyConfig` |
| `internal/formula/parser.go` | Parse new fields (automatic via toml tags) |
| `internal/formula/parser_test.go` | Add tests for new fields |
| `internal/cmd/formula.go` | Update dispatch logic |
| `internal/formula/formulas/mol-nfr-leg.formula.toml` | New file |
| `internal/formula/formulas/mol-nfr-reflection.formula.toml` | Add convoy config |

## Documentation Updates

### Required Updates

| File | Changes Needed |
|------|----------------|
| `docs/convoy.md` | Add section on convoy formulas with `[convoy] leg_formula` config |
| `docs/molecules.md` | Add convoy formula type, explain leg formula dispatch |
| `docs/reference.md` | Document `leg_formula` in convoy formula examples |
| `docs/glossary.md` | Add terms: `leg_formula`, `ConvoyConfig` |

### New Documentation (Optional)

Consider creating `docs/convoy-formulas.md` with:
- Convoy formula type explained
- leg_formula configuration
- Per-leg formula overrides
- Examples (design.formula.toml, mol-nfr-reflection.formula.toml)
- Comparison to workflow formulas

### Example Doc Addition for `docs/convoy.md`

```markdown
## Convoy Formulas

Convoy formulas define parallel work with a synthesis step. Each "leg" runs
independently, and results are combined in synthesis.

### Leg Formulas

By default, convoy legs are dispatched as bare beads. To give polecats
step-by-step workflow guidance, specify a `leg_formula`:

\`\`\`toml
[convoy]
leg_formula = "mol-analysis-workflow"  # All legs use this

[[legs]]
id = "api"
title = "API Analysis"
# Uses mol-analysis-workflow

[[legs]]
id = "security"
title = "Security Analysis"
formula = "mol-security-audit"  # Override for this leg
\`\`\`

This causes dispatch to use `gt sling <formula> --on <leg-bead> <rig>`
instead of plain `gt sling <leg-bead> <rig>`.
```

## Future Considerations

### Unified Default Formula Config

Issue #288 proposes auto-applying `mol-polecat-work` for all polecat slings.
This could be generalized to a rig-level config:

```json
// settings/config.json
{
  "workflow": {
    "default_formula": "reckoning-work"
  }
}
```

If implemented, convoy `leg_formula` would take precedence over rig default.

### Priority Order

```
1. leg.formula (per-leg override)
2. convoy.leg_formula (convoy default)
3. rig.workflow.default_formula (rig default, if #288 implemented)
4. none (bare bead, current behavior)
```
