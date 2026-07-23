package cmd

import (
	"strings"
	"testing"
	"time"
)

// The checkpoint-resume path re-presents work captured at handoff/checkpoint
// time. These tests pin the two staleness defenses:
//   1. stampHandoffMessage — the frozen note carries its capture timestamp.
//   2. checkpointBeadLine  — a bead CLOSED since capture is delivered flagged,
//      never as plain "resume work" (mirror of the sling closed-bead guard).
// Regression context: a closed bead was silently flipped closed->in_progress
// twice in one night by successors acting on a frozen "LIVE emergency" note.

func TestStampHandoffMessage(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 7, 23, 1, 14, 0, 0, time.UTC)
	out := stampHandoffMessage("kota criticals are LIVE. Real, urgent.", ts)

	if !strings.Contains(out, "captured 2026-07-23 01:14 UTC") {
		t.Errorf("stamp missing capture timestamp: %q", out)
	}
	if !strings.Contains(out, "frozen at capture time") {
		t.Errorf("stamp missing staleness warning: %q", out)
	}
	if !strings.Contains(out, "must not be silently reopened") {
		t.Errorf("stamp missing reopen prohibition: %q", out)
	}
	if !strings.HasSuffix(out, "kota criticals are LIVE. Real, urgent.") {
		t.Errorf("original note must survive verbatim after the stamp: %q", out)
	}
}

func TestCheckpointBeadLineFlagsClosedBead(t *testing.T) {
	orig := checkpointBeadStatus
	defer func() { checkpointBeadStatus = orig }()

	ctx := RoleContext{Role: RoleCrew, Rig: "myrig"}

	t.Run("closed bead is flagged, not presented as resume work", func(t *testing.T) {
		checkpointBeadStatus = func(RoleContext, string) string { return "closed" }
		line := checkpointBeadLine(ctx, "gt-lnmc")
		for _, want := range []string{"NOW CLOSED", "Do NOT restart work", "never silently"} {
			if !strings.Contains(line, want) {
				t.Errorf("closed-bead line missing %q: %q", want, line)
			}
		}
	})

	t.Run("tombstone bead is flagged too", func(t *testing.T) {
		checkpointBeadStatus = func(RoleContext, string) string { return "tombstone" }
		line := checkpointBeadLine(ctx, "gt-lnmc")
		if !strings.Contains(line, "NOW TOMBSTONE") {
			t.Errorf("tombstone not flagged: %q", line)
		}
	})

	t.Run("open bead renders plainly", func(t *testing.T) {
		checkpointBeadStatus = func(RoleContext, string) string { return "in_progress" }
		line := checkpointBeadLine(ctx, "gt-abc")
		if strings.Contains(line, "⚠️") || strings.Contains(line, "Do NOT") {
			t.Errorf("open bead must not be flagged: %q", line)
		}
		if !strings.Contains(line, "gt-abc") {
			t.Errorf("bead id missing: %q", line)
		}
	})

	t.Run("unknown status (lookup failed) renders plainly — availability over false alarm", func(t *testing.T) {
		checkpointBeadStatus = func(RoleContext, string) string { return "" }
		line := checkpointBeadLine(ctx, "gt-abc")
		if strings.Contains(line, "⚠️") {
			t.Errorf("lookup failure must not fabricate a closed flag: %q", line)
		}
	})
}
