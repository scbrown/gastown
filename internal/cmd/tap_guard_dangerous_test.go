package cmd

import (
	"testing"
)

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"valid hook input", `{"tool_name":"Bash","tool_input":{"command":"rm -rf /tmp/foo"}}`, "rm -rf /tmp/foo"},
		{"empty input", "", ""},
		{"invalid json", "not json", ""},
		{"no command field", `{"tool_name":"Write","tool_input":{"file_path":"/tmp/foo"}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCommand([]byte(tt.input))
			if got != tt.want {
				t.Errorf("extractCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripHeredocBodies(t *testing.T) {
	quoted := "cat > /tmp/msg.md <<'EOF'\nnever run git reset --hard here\nEOF\nbd comment x --file /tmp/msg.md"
	cleaned, extras := stripHeredocBodies(quoted)
	if len(extras) != 0 {
		t.Errorf("quoted heredoc body must be fully inert, got extras %v", extras)
	}
	for _, banned := range []string{"reset", "--hard"} {
		if containsWord(cleaned, banned) {
			t.Errorf("quoted heredoc body leaked %q into cleaned text:\n%s", banned, cleaned)
		}
	}

	unquoted := "cat <<EOF\nresult: $(sudo id)\nEOF"
	_, extras = stripHeredocBodies(unquoted)
	if len(extras) != 1 || extras[0] != "sudo id" {
		t.Errorf("unquoted heredoc body substitutions must be extracted, got %v", extras)
	}
}

func containsWord(s, w string) bool {
	for _, c := range scanCommands(s) {
		for _, tok := range c.tokens {
			if tok.text == w {
				return true
			}
		}
	}
	return false
}

func TestScanCommandsArgv0(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		want  []string // argv0Base of each scanned command, order-insensitive not needed
	}{
		{"simple", "ls -la", []string{"ls"}},
		{"path prefix", "/usr/bin/sudo id", []string{"sudo"}},
		{"backslash alias skip", `\sudo id`, []string{"sudo"}},
		{"env assignment", "FOO=bar git status", []string{"git"}},
		{"separators", "cd /tmp && sudo id; echo done", []string{"cd", "sudo", "echo"}},
		{"pipeline", "echo foo | sudo tee /etc/x", []string{"echo", "sudo"}},
		{"keyword prefix", "if true; then sudo id; fi", []string{"true", "sudo"}},
		{"wrapper env", "env -i sudo id", []string{"sudo"}},
		{"wrapper nohup", "nohup sudo id", []string{"sudo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, c := range scanCommands(tt.cmd) {
				if len(c.tokens) > 0 {
					got = append(got, c.argv0Base())
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("scanCommands(%q) argv0s = %v, want %v", tt.cmd, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("scanCommands(%q) argv0[%d] = %q, want %q", tt.cmd, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestDangerousGuard_Integration drives the real entry point over the full
// pattern set: every legacy true-positive, the measured false positives from
// aegis-ptfb (prose naming a hazard must NOT block), and the evasion cases
// (path prefix, substitution, wrapper payloads MUST block).
func TestDangerousGuard_Integration(t *testing.T) {
	tests := []struct {
		name    string
		command string
		blocked bool
	}{
		// Blocked — privilege escalation
		{"sudo command", "sudo dnf install -y foo", true},
		{"sudo rm", "sudo rm -rf /var/log/syslog", true},
		{"sudo bare", "sudo su", true},
		{"sudo in pipeline", "echo foo | sudo tee /etc/config", true},
		{"sudo after &&", "cd /tmp && sudo id", true},
		{"doas", "doas id", true},

		// Blocked — evasion (aegis-ptfb defect 2)
		{"path-prefixed sudo", "/usr/bin/sudo id", true},
		{"path-prefixed sudo deep", "/usr/local/bin/sudo apt install x", true},
		{"backslash sudo", `\sudo id`, true},
		{"sudo in command substitution", "echo $(sudo id)", true},
		{"sudo in double-quoted substitution", `echo "now: $(sudo id)"`, true},
		{"sudo in backticks", "echo `sudo id`", true},
		{"sh -c payload", `sh -c "sudo id"`, true},
		{"bash -c payload", `bash -c 'git push --force origin main'`, true},
		{"eval payload", "eval sudo id", true},
		{"xargs payload", "echo x | xargs sudo rm", true},
		{"env wrapper", "env -i sudo id", true},
		{"nohup wrapper", "nohup sudo id", true},
		{"unquoted heredoc substitution", "cat <<EOF\n$(sudo id)\nEOF", true},

		// Blocked — package installs
		{"apt install", "apt install -y curl", true},
		{"apt-get install", "apt-get install -y build-essential", true},
		{"dnf install", "dnf install -y postgresql-contrib", true},
		{"yum install", "yum install -y gcc", true},
		{"pacman -S", "pacman -S git", true},
		{"brew install", "brew install node", true},
		{"gem install", "gem install bundler", true},
		{"pip install --system", "pip install --system requests", true},
		{"pip3 install --system", "pip3 install --system flask", true},
		{"npm install -g", "npm install -g typescript", true},
		{"npm install --global", "npm install --global eslint", true},

		// Blocked — destructive operations
		{"rm -rf /", "rm -rf /", true},
		{"rm -rf /*", "rm -rf /*", true},
		{"rm -rf / with sudo", "sudo rm -rf /", true},
		{"git push --force", "git push --force origin main", true},
		{"git push -f", "git push -f origin main", true},
		{"git push --force bare", "git push --force", true},
		{"git reset --hard", "git reset --hard HEAD~1", true},
		{"git -C reset --hard", "git -C /some/repo reset --hard", true},
		{"git clean -f", "git clean -f", true},
		{"git clean -fd", "git clean -fd", true},
		{"git clean -fdx", "git clean -fdx", true},
		{"drop table via mysql", `mysql -e "DROP TABLE users"`, true},
		{"drop database via psql", `psql -c "drop database prod"`, true},
		{"truncate via dolt", `dolt sql -q "truncate table logs"`, true},
		{"bare SQL drop", "DROP TABLE users", true},

		// Allowed — measured false positives (aegis-ptfb defect 1)
		{"quoted heredoc naming reset --hard", "cat > /tmp/msg.md <<'EOF'\nnever run git reset --hard on shared trees\nEOF\nbd comment x --file /tmp/msg.md", false},
		{"echo prose naming sudo apt-get", `echo "use sudo apt-get install manually"`, false},
		{"grep for git clean", `grep -rn "git clean -fdx" docs/`, false},
		{"commit message naming reset --hard", `git commit -m "docs: forbid git reset --hard on shared trees"`, false},
		{"single-quoted sudo prose", "echo 'do not use sudo'", false},
		{"drop table prose in comment body", `bd comment x -m "never drop table by hand"`, false},
		{"quoted push --force prose", `git commit -m "explain why git push --force is banned"`, false},

		// Allowed — legacy negatives
		{"rm -rf ./build/", "rm -rf ./build/", false},
		{"rm -rf node_modules/", "rm -rf node_modules/", false},
		{"rm -rf /tmp/test-output/", "rm -rf /tmp/test-output/", false},
		{"rm -rf relative dir", "rm -rf build", false},
		{"rm single file", "rm foo.txt", false},
		{"rm -r no force", "rm -r /", false},
		{"git push --force-with-lease", "git push --force-with-lease origin main", false},
		{"git push --force-if-includes", "git push --force-if-includes origin main", false},
		{"git push normal", "git push origin main", false},
		{"git reset soft", "git reset --soft HEAD~1", false},
		{"git status", "git status", false},
		{"pip install (venv ok)", "pip install requests", false},
		{"npm install (local ok)", "npm install express", false},
		{"npm install --save-dev", "npm install --save-dev jest", false},
		{"go install", "go install ./...", false},
		{"cargo install", "cargo install ripgrep", false},
		{"pseudocode filename", "cat pseudocode.txt", false},
		{"normal command", "ls -la", false},
		{"echo hello", "echo hello", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := findDangerousCommand(tt.command)
			if (reason != "") != tt.blocked {
				t.Errorf("command %q: blocked=%v (reason %q), want blocked=%v", tt.command, reason != "", reason, tt.blocked)
			}
		})
	}
}
