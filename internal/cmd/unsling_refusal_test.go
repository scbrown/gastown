package cmd

import "testing"

// aegis-1xol: a guard refusal must not drag the usage block with it. Runtime
// errors from runUnslingWith set SilenceUsage so cobra prints only the Error
// line; syntax errors (bad args/flags) still get usage because they fail
// before RunE runs.
func TestUnslingRefusalSilencesUsage(t *testing.T) {
	unslingCmd.SilenceUsage = false
	_ = runUnslingWith(unslingCmd, []string{"definitely-not-a-bead", "no/such/agent"}, false, false)
	if !unslingCmd.SilenceUsage {
		t.Error("runUnslingWith must set SilenceUsage before any runtime error can return")
	}
}

// aegis-1xol: detach's RunE always passed hookDryRun through, but the flag was
// never registered — the only path that reveals the guard's reasoning did not
// exist on the door whose --help claims equivalence with clear.
func TestHookDetachHasDryRun(t *testing.T) {
	if hookDetachCmd.Flags().Lookup("dry-run") == nil {
		t.Fatal("gt hook detach must register --dry-run (its RunE already consumes hookDryRun)")
	}
	if hookClearCmd.Flags().Lookup("dry-run") == nil {
		t.Fatal("gt hook clear must keep --dry-run")
	}
}
