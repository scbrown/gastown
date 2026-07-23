package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var tapGuardDangerousCmd = &cobra.Command{
	Use:   "dangerous-command",
	Short: "Block dangerous commands (sudo, package installs, rm -rf, force push, etc.)",
	Long: `Block dangerous commands via Claude Code PreToolUse hooks.

This guard blocks operations that could cause irreversible damage:
  - sudo <anything>      (agents must never elevate privileges)
  - apt/apt-get/dnf/yum/pacman install (system package managers)
  - brew install          (Homebrew package installs)
  - pip install --system  (system-level Python installs)
  - npm install -g        (global npm installs)
  - gem install           (system-level Ruby installs)
  - rm -rf /             (only blocks root target; rm -rf ./build/ is allowed)
  - git push --force/-f  (--force-with-lease is allowed)
  - git reset --hard
  - git clean -f / git clean -fd
  - drop table/database, truncate table (as arguments to a database CLI)

Matching is COMMAND-POSITION aware, not substring containment: the command
line is shell-scanned (quotes, heredoc bodies, command substitutions,
sh -c / eval / xargs payloads), so prose that merely NAMES a dangerous
command — a quoted heredoc documenting 'git reset --hard', an
echo/grep/commit-message mentioning sudo — is not blocked, while
'/usr/bin/sudo' and '$(sudo ...)' are. See tap_guard_shellscan.go.

The guard reads the tool input from stdin (Claude Code hook protocol)
and exits with code 2 to block dangerous operations.

Exit codes:
  0 - Operation allowed
  2 - Operation BLOCKED`,
	RunE: runTapGuardDangerous,
}

func init() {
	tapGuardCmd.AddCommand(tapGuardDangerousCmd)
}

// sqlFragments are checked by containment against the arguments of a known
// database CLI (they live inside quoted -e/-q strings, so token matching
// cannot see them). They are NOT checked against arbitrary commands — a bead
// comment mentioning "drop table" is prose, not SQL.
var sqlFragments = []struct {
	contains []string
	reason   string
}{
	{[]string{"drop", "table"}, "database table destruction"},
	{[]string{"drop", "database"}, "database destruction"},
	{[]string{"truncate", "table"}, "database table truncation"},
}

// dbClients are argv0 basenames whose arguments are treated as SQL.
var dbClients = map[string]bool{
	"mysql": true, "mariadb": true, "psql": true, "sqlite3": true,
	"dolt": true, "mycli": true, "pgcli": true, "sqlcmd": true,
}

func runTapGuardDangerous(cmd *cobra.Command, args []string) error {
	// Read hook input from stdin (Claude Code protocol)
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil // fail open
	}

	command := extractCommand(input)
	if command == "" {
		return nil
	}

	if reason := findDangerousCommand(command); reason != "" {
		printDangerousBlock(reason, command)
		return NewSilentExit(2)
	}
	return nil
}

// findDangerousCommand shell-scans the command line and returns the reason for
// the first dangerous simple command found, or "" if none.
func findDangerousCommand(command string) string {
	for _, c := range scanCommands(command) {
		if len(c.tokens) == 0 {
			continue
		}
		for _, check := range []func(*simpleCommand) string{
			matchesSudo,
			matchesPackageInstall,
			matchesDangerousRm,
			matchesDangerousGit,
			matchesDangerousSQL,
		} {
			if reason := check(&c); reason != "" {
				return reason
			}
		}
	}
	return ""
}

// printDangerousBlock prints the standard block banner to stderr.
func printDangerousBlock(reason, originalCommand string) {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════════════════╗")
	fmt.Fprintln(os.Stderr, "║  ❌ DANGEROUS COMMAND BLOCKED                                    ║")
	fmt.Fprintln(os.Stderr, "╠══════════════════════════════════════════════════════════════════╣")
	fmt.Fprintf(os.Stderr, "║  Command: %-53s ║\n", truncateStr(originalCommand, 53))
	fmt.Fprintf(os.Stderr, "║  Reason:  %-53s ║\n", truncateStr(reason, 53))
	fmt.Fprintln(os.Stderr, "║                                                                  ║")
	fmt.Fprintln(os.Stderr, "║  If this is intentional, ask the user to run it manually.        ║")
	fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(os.Stderr, "")
}

// extractCommand extracts the bash command from Claude Code hook input JSON.
func extractCommand(input []byte) string {
	if len(input) == 0 {
		return ""
	}
	var hookInput struct {
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(input, &hookInput); err != nil {
		return ""
	}
	return hookInput.ToolInput.Command
}

// matchesAllFragments returns true if all fragments appear in the command.
func matchesAllFragments(command string, fragments []string) bool {
	for _, f := range fragments {
		if !strings.Contains(command, strings.ToLower(f)) {
			return false
		}
	}
	return true
}

// matchesSudo blocks privilege escalation: sudo/doas at command position
// (path-normalized, so /usr/bin/sudo counts) or as an unquoted argument
// (find -exec sudo, etc.). A quoted "sudo" is prose and does not match.
func matchesSudo(c *simpleCommand) string {
	const reason = "Agents must never use sudo — do not elevate privileges or modify the host OS"
	a := c.argv0Base()
	if a == "sudo" || a == "doas" {
		return reason
	}
	if c.hasUnquotedToken("sudo") || c.hasUnquotedToken("doas") {
		return reason
	}
	return ""
}

// packageManagerVerbs maps a package-manager argv0 to its install verb.
var packageManagerVerbs = map[string]struct {
	verb   string
	reason string
}{
	"apt":     {"install", "System package install (apt) — use workspace tools instead"},
	"apt-get": {"install", "System package install (apt-get) — use workspace tools instead"},
	"dnf":     {"install", "System package install (dnf) — use workspace tools instead"},
	"yum":     {"install", "System package install (yum) — use workspace tools instead"},
	"brew":    {"install", "Package install (brew) — use workspace tools instead"},
	"gem":     {"install", "System gem install — use workspace tools instead"},
}

// matchesPackageInstall blocks system package manager install commands.
// The manager must be at command position; `echo "apt install foo"` is prose.
func matchesPackageInstall(c *simpleCommand) string {
	a := c.argv0Base()
	if p, ok := packageManagerVerbs[a]; ok && c.hasUnquotedToken(p.verb) {
		return p.reason
	}
	if a == "pacman" {
		for _, t := range c.tokens[1:] {
			if !t.quoted && strings.HasPrefix(strings.ToLower(t.text), "-s") {
				return "System package install (pacman) — use workspace tools instead"
			}
		}
	}
	if (a == "pip" || a == "pip2" || a == "pip3") &&
		c.hasUnquotedToken("install") && c.hasUnquotedToken("--system") {
		return "System-level pip install — use a virtualenv or workspace tools instead"
	}
	if a == "npm" && c.hasUnquotedToken("install") &&
		(c.hasUnquotedToken("-g") || c.hasUnquotedToken("--global")) {
		return "Global npm install — use workspace tools instead"
	}
	return ""
}

// matchesDangerousRm blocks "rm -rf /" targeting the root filesystem.
// Only blocks when the target is literally "/" or "/*". Normal cleanup
// commands like "rm -rf ./build/" are allowed.
func matchesDangerousRm(c *simpleCommand) string {
	if c.argv0Base() != "rm" {
		return ""
	}
	hasRecursiveForce := false
	for _, t := range c.tokens[1:] {
		if t.quoted {
			continue
		}
		low := strings.ToLower(t.text)
		if strings.HasPrefix(low, "-") && strings.Contains(low, "r") && strings.Contains(low, "f") {
			hasRecursiveForce = true
		}
		if hasRecursiveForce && (t.text == "/" || t.text == "/*") {
			return "filesystem destruction (rm -rf /)"
		}
	}
	return ""
}

// matchesDangerousGit blocks destructive git operations at command position:
// reset --hard, clean -f, push --force/-f. Flags must be real unquoted
// tokens, so `git commit -m "forbid git reset --hard"` is prose and allowed.
// Safe force variants (--force-with-lease, --force-if-includes) are distinct
// exact tokens and simply never match --force/-f.
func matchesDangerousGit(c *simpleCommand) string {
	if c.argv0Base() != "git" {
		return ""
	}
	if c.hasUnquotedToken("reset") {
		for _, t := range c.tokens[1:] {
			if !t.quoted && strings.EqualFold(t.text, "--hard") {
				return "Hard reset discards all uncommitted changes irreversibly"
			}
		}
	}
	if c.hasUnquotedToken("clean") {
		for _, t := range c.tokens[1:] {
			if t.quoted {
				continue
			}
			low := strings.ToLower(t.text)
			if strings.HasPrefix(low, "-") && !strings.HasPrefix(low, "--") && strings.Contains(low, "f") {
				return "git clean -f deletes untracked files irreversibly"
			}
		}
	}
	if c.hasUnquotedToken("push") {
		for _, t := range c.tokens[1:] {
			if !t.quoted && (t.text == "--force" || t.text == "-f") {
				return "Force push rewrites remote history and can destroy others' work"
			}
		}
	}
	return ""
}

// matchesDangerousSQL blocks destructive SQL handed to a database CLI.
// SQL rides inside quoted -e/-q arguments, so this is the one place
// containment matching is still used — scoped to known db clients.
func matchesDangerousSQL(c *simpleCommand) string {
	a := c.argv0Base()
	if !dbClients[a] && a != "drop" && a != "truncate" {
		return ""
	}
	for _, p := range sqlFragments {
		if matchesAllFragments(c.raw, p.contains) {
			return p.reason
		}
	}
	return ""
}
