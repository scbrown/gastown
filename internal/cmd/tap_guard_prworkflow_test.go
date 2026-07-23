package cmd

import "testing"

// TestPRWorkflowCommandMatch pins the self-filtering added in aegis-ptfb: the
// pr-workflow guard historically read NO command (context-only), relying on
// Bash(gh pr create*) hook matchers that — being permissions syntax — never
// fire. Wired on matcher "Bash" it must pass ordinary commands and gate only
// actual PR-workflow operations, seen through the shell-aware scanner.
func TestPRWorkflowCommandMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		// the three gated operations
		{"gh pr create", "gh pr create --title x", true},
		{"git checkout -b", "git checkout -b feature/x", true},
		{"git switch -c", "git switch -c feature/x", true},
		// evasions the scanner must see through
		{"path-prefixed git", "/usr/bin/git checkout -b feature/x", true},
		{"after &&", "cd /tmp && gh pr create", true},
		{"bash -c wrapper", `bash -c "gh pr create"`, true},
		{"command substitution", `echo $(gh pr create)`, true},
		// ordinary commands must pass (the measured 'echo hello exit 2' defect)
		{"echo hello", "echo hello", false},
		{"git status", "git status", false},
		{"git checkout branch (no -b)", "git checkout main", false},
		{"git switch branch (no -c)", "git switch main", false},
		// prose naming the operations is not the operations
		{"quoted prose", `echo "run gh pr create later"`, false},
		{"commit message", `git commit -m "forbid git checkout -b in agents"`, false},
		{"quoted heredoc body", "cat > /tmp/x.md <<'EOF'\ngh pr create\ngit checkout -b\nEOF", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prWorkflowCommandMatch(tt.command); got != tt.want {
				t.Errorf("prWorkflowCommandMatch(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}
