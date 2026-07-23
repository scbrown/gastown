package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var tapGuardCmd = &cobra.Command{
	Use:   "guard",
	Short: "Block forbidden operations (PreToolUse hook)",
	Long: `Block forbidden operations via Claude Code PreToolUse hooks.

Guard commands exit with code 2 to BLOCK tool execution when a policy
is violated. They're called before the tool runs, preventing the
forbidden operation entirely.

Available guards:
  pr-workflow        - Block PR creation and feature branches
  bd-init            - Block bd init in wrong directories
  mol-patrol         - Block mol patrol from agent contexts
  dangerous-command  - Block rm -rf, force push, hard reset, git clean

External guards (standalone scripts, not compiled into gt):
  context-budget   - scripts/guards/context-budget-guard.sh

Example hook configuration (matcher MUST be the tool name "Bash" —
the "Bash(cmd*)" form is PERMISSIONS syntax and as a hook matcher NEVER fires;
guards filter the command themselves):
  {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"command": "gt tap guard dangerous-command"}]
    }]
  }`,
}

var tapGuardPRWorkflowCmd = &cobra.Command{
	Use:   "pr-workflow",
	Short: "Block PR creation and feature branches",
	Long: `Block PR workflow operations in Gas Town.

Gas Town workers push directly to main. PRs add friction that breaks
the autonomous execution model (GUPP principle).

This guard blocks:
  - gh pr create
  - git checkout -b (feature branches)
  - git switch -c (feature branches)

Exit codes:
  0 - Operation allowed (not in Gas Town agent context, not maintainer origin)
  2 - Operation BLOCKED (in agent context OR maintainer origin)

The guard blocks in two scenarios:
  1. Running as a Gas Town agent (crew, polecat, witness, etc.)
  2. Origin remote is steveyegge/gastown (maintainer should push directly)

Humans running outside Gas Town with a fork origin can still use PRs.`,
	RunE: runTapGuardPRWorkflow,
}

func init() {
	tapCmd.AddCommand(tapGuardCmd)
	tapGuardCmd.AddCommand(tapGuardPRWorkflowCmd)
}

// prWorkflowCommandMatch reports whether the hook command actually attempts a
// PR-workflow operation (gh pr create / git checkout -b / git switch -c) at
// command position. Rides the same shell-aware scanner as dangerous-command
// (tap_guard_shellscan.go), so path-prefixed argv0, wrappers (bash -c, eval,
// xargs) and command substitutions are seen, while quoted prose, heredoc
// bodies and commit messages that merely NAME the operations are not.
func prWorkflowCommandMatch(command string) bool {
	for _, c := range scanCommands(command) {
		switch c.argv0Base() {
		case "gh":
			if len(c.tokens) >= 3 &&
				!c.tokens[1].quoted && strings.EqualFold(c.tokens[1].text, "pr") &&
				!c.tokens[2].quoted && strings.EqualFold(c.tokens[2].text, "create") {
				return true
			}
		case "git":
			if len(c.tokens) >= 2 && !c.tokens[1].quoted {
				switch strings.ToLower(c.tokens[1].text) {
				case "checkout":
					if c.hasUnquotedToken("-b") {
						return true
					}
				case "switch":
					if c.hasUnquotedToken("-c") {
						return true
					}
				}
			}
		}
	}
	return false
}

func runTapGuardPRWorkflow(cmd *cobra.Command, args []string) error {
	// SELF-FILTERING (aegis-ptfb): this guard historically never read the
	// command — it blocked on CONTEXT alone (GT_* env or an agent cwd), and
	// relied on `Bash(gh pr create*)` hook matchers for command filtering.
	// Claude Code hook matchers match TOOL NAMES; the paren form is
	// permissions syntax and fires NEVER (measured: matcher "Bash(echo *)"
	// did not fire on an echo; matcher "Bash" did) — so wired correctly on
	// matcher "Bash", the unfiltered guard would block EVERY bash call in
	// every agent session (measured: exit 2 on `echo hello` with GT_CREW
	// set). Filter here: only commands that actually attempt a PR-workflow
	// operation reach the context decision. When stdin carries no command
	// (manual invocation, non-Bash hook), keep the historical context-only
	// behavior.
	if input, err := io.ReadAll(os.Stdin); err == nil {
		if command := extractCommand(input); command != "" && !prWorkflowCommandMatch(command) {
			return nil
		}
	}

	// Check if we're in a Gas Town agent context
	if isGasTownAgentContext() {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════════════════╗")
		fmt.Fprintln(os.Stderr, "║  ❌ PR WORKFLOW BLOCKED                                          ║")
		fmt.Fprintln(os.Stderr, "╠══════════════════════════════════════════════════════════════════╣")
		fmt.Fprintln(os.Stderr, "║  Gas Town workers push directly to main. PRs are forbidden.     ║")
		fmt.Fprintln(os.Stderr, "║                                                                  ║")
		fmt.Fprintln(os.Stderr, "║  Instead of:  gh pr create / git checkout -b / git switch -c    ║")
		fmt.Fprintln(os.Stderr, "║  Do this:     git add . && git commit && git push origin main   ║")
		fmt.Fprintln(os.Stderr, "║                                                                  ║")
		fmt.Fprintln(os.Stderr, "║  Why? PRs add friction that breaks autonomous execution.        ║")
		fmt.Fprintln(os.Stderr, "║  See: ~/gt/docs/PRIMING.md (GUPP principle)                     ║")
		fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════════════════╝")
		fmt.Fprintln(os.Stderr, "")
		return NewSilentExit(2) // Exit 2 = BLOCK in Claude Code hooks
	}

	// Check if origin is the maintainer's repo (steveyegge/gastown)
	if isMaintainerOrigin() {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════════════════╗")
		fmt.Fprintln(os.Stderr, "║  ❌ PR BLOCKED - MAINTAINER ORIGIN                               ║")
		fmt.Fprintln(os.Stderr, "╠══════════════════════════════════════════════════════════════════╣")
		fmt.Fprintln(os.Stderr, "║  Your origin is steveyegge/gastown - push directly to main.     ║")
		fmt.Fprintln(os.Stderr, "║  PRs are for external contributors, not maintainers.            ║")
		fmt.Fprintln(os.Stderr, "║                                                                  ║")
		fmt.Fprintln(os.Stderr, "║  Instead of:  gh pr create                                      ║")
		fmt.Fprintln(os.Stderr, "║  Do this:     git push origin main                              ║")
		fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════════════════╝")
		fmt.Fprintln(os.Stderr, "")
		return NewSilentExit(2) // Exit 2 = BLOCK in Claude Code hooks
	}

	// Not in Gas Town context and not maintainer origin - allow PRs
	return nil
}

// isGasTownAgentContext returns true if we're running as a Gas Town managed agent.
func isGasTownAgentContext() bool {
	// Check environment variables set by Gas Town session management
	envVars := []string{
		"GT_POLECAT",
		"GT_CREW",
		"GT_WITNESS",
		"GT_REFINERY",
		"GT_MAYOR",
		"GT_DEACON",
	}
	for _, env := range envVars {
		if os.Getenv(env) != "" {
			return true
		}
	}

	// Also check if we're in a crew or polecat worktree by path
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	agentPaths := []string{"/crew/", "/polecats/"}
	for _, path := range agentPaths {
		if strings.Contains(cwd, path) {
			return true
		}
	}

	return false
}

// isMaintainerOrigin returns true if the origin remote points to the maintainer's repo.
// This prevents the maintainer from accidentally creating PRs in their own repo.
func isMaintainerOrigin() bool {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	url := strings.TrimSpace(string(output))
	// Match both HTTPS and SSH URL formats:
	// - https://github.com/steveyegge/gastown.git
	// - git@github.com:steveyegge/gastown.git
	return strings.Contains(url, "steveyegge/gastown")
}
