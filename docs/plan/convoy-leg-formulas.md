# Convoy Leg Formula Support

## Problem

Convoy formulas create leg beads but dispatch them with plain `gt sling <leg> <rig>` - no workflow formula is applied. This means polecats receive work without step-by-step guidance, leading to:

- Polecats going off-track (not following the intended analysis workflow)
- No crash-recovery (can't resume from last completed step)
- Inconsistent execution across legs

## Solution

Allow convoy formulas to specify a workflow formula for leg execution, following the same pattern as `gt sling <formula> --on <bead> <rig>`.

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
