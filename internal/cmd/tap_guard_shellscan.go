package cmd

import (
	"regexp"
	"strings"
)

// This file implements the shell-aware command scanner behind the
// dangerous-command guard (and any future tap guard that needs command-position
// matching instead of substring containment).
//
// WHY: substring containment over the raw command string has two failure modes,
// both measured in production (aegis-ptfb):
//
//  1. FALSE POSITIVES on prose that NAMES a hazard: a quoted-heredoc body
//     mentioning `git reset --hard`, `echo "use sudo apt-get install
//     manually"`, `grep -rn "git clean -fdx" docs/`, and a commit message
//     forbidding a dangerous command all exited 2. The more precisely an agent
//     documents a hazard, the likelier the guard is to block the documentation.
//     Worse, the documented-safe way to write prose (a quoted heredoc,
//     aegis-0214) was itself blocked whenever the prose named a guarded
//     command.
//
//  2. PATH-PREFIX EVASION: `/usr/bin/sudo id` sailed past `f == "sudo"`.
//
// The fix: parse the command string the way a shell would, just far enough to
// know (a) which text is at COMMAND POSITION and (b) which text is inert data
// (single/double-quoted words, heredoc bodies). Matchers then check argv0 and
// unquoted argument tokens instead of raw substrings. Command substitutions
// ($(...), `...`) and quoted `bash -c` / `eval` / `xargs` payloads EXECUTE, so
// their interiors are recursively scanned as commands, not treated as data.

// shellToken is one word of a simple command. quoted is true if ANY part of
// the token came from inside single or double quotes — such tokens are data
// for matching purposes (a quoted "--hard" is not the flag --hard).
type shellToken struct {
	text   string
	quoted bool
}

// simpleCommand is one command-position unit: argv0 plus its arguments, after
// separator splitting, keyword/env-assignment stripping, and wrapper peeling.
type simpleCommand struct {
	tokens []shellToken
	// raw is the joined original text of the command's tokens (case-lowered),
	// used only by containment-class matchers that must look inside quoted
	// arguments of specific interpreters (SQL passed to a db CLI).
	raw string
}

// argv0Base returns the basename of the first token, lowercased: a leading
// directory path and leading backslashes (alias suppression) are stripped, so
// `/usr/bin/sudo` and `\sudo` both present as `sudo`.
func (c *simpleCommand) argv0Base() string {
	if len(c.tokens) == 0 {
		return ""
	}
	a := strings.TrimLeft(c.tokens[0].text, `\`)
	if i := strings.LastIndex(a, "/"); i >= 0 {
		a = a[i+1:]
	}
	return strings.ToLower(a)
}

// hasUnquotedToken reports whether any token AFTER argv0 equals s (case-insensitive)
// outside quotes.
func (c *simpleCommand) hasUnquotedToken(s string) bool {
	for i, t := range c.tokens {
		if i == 0 {
			continue
		}
		if !t.quoted && strings.EqualFold(t.text, s) {
			return true
		}
	}
	return false
}

var heredocIntroRe = regexp.MustCompile(`<<-?\s*(['"]?)([A-Za-z_][A-Za-z0-9_]*)['"]?`)

// stripHeredocBodies removes heredoc BODIES from cmd — they are data, not
// command position. The intro line (`cat <<'EOF'`) is kept: it IS at command
// position. For UNQUOTED delimiters the body still undergoes substitution, so
// $(...) and `...` interiors found in those bodies are returned as extra
// command strings to scan; a quoted delimiter ('EOF') makes the body fully
// inert and it is dropped wholesale.
func stripHeredocBodies(cmd string) (cleaned string, extraCmds []string) {
	if !strings.Contains(cmd, "<<") {
		return cmd, nil
	}
	var out []string
	lines := strings.Split(cmd, "\n")
	inBody := false
	delim := ""
	bodyExpands := false
	for _, line := range lines {
		if inBody {
			if strings.TrimSpace(line) == delim {
				inBody = false
				continue
			}
			if bodyExpands {
				extraCmds = append(extraCmds, extractSubstitutions(line)...)
			}
			continue
		}
		if m := heredocIntroRe.FindStringSubmatch(line); m != nil && !strings.Contains(line, "<<<") {
			delim = m[2]
			bodyExpands = m[1] == "" // unquoted delimiter → body substitutes
			inBody = true
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n"), extraCmds
}

// extractSubstitutions pulls the interiors of $(...) and `...` out of s.
// Used for unquoted heredoc bodies, where only substitutions execute.
func extractSubstitutions(s string) []string {
	var found []string
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '$' && i+1 < len(s) && s[i+1] == '(':
			depth := 1
			j := i + 2
			for ; j < len(s) && depth > 0; j++ {
				if s[j] == '(' {
					depth++
				} else if s[j] == ')' {
					depth--
				}
			}
			found = append(found, s[i+2:j-1])
			i = j - 1
		case s[i] == '`':
			if j := strings.IndexByte(s[i+1:], '`'); j >= 0 {
				found = append(found, s[i+1:i+1+j])
				i = i + 1 + j
			}
		}
	}
	return found
}

// scanCommands parses cmd into every simple command that would execute:
// separator-split segments plus the interiors of command substitutions
// (including those inside double quotes — they execute there too). Payloads of
// `sh -c '...'`, `eval ...` and `xargs cmd` are peeled recursively.
func scanCommands(cmd string) []simpleCommand {
	cleaned, extras := stripHeredocBodies(cmd)
	cmds := scanText(cleaned, 0)
	for _, e := range extras {
		cmds = append(cmds, scanText(e, 0)...)
	}
	return cmds
}

const maxScanDepth = 8 // substitution/wrapper recursion bound

func scanText(s string, depth int) []simpleCommand {
	if depth > maxScanDepth {
		return nil
	}
	var (
		cmds    []simpleCommand
		cur     []shellToken
		tok     strings.Builder
		tokAny  bool // token has content (possibly empty quotes)
		tokQuot bool
	)
	endToken := func() {
		if tokAny || tok.Len() > 0 {
			cur = append(cur, shellToken{text: tok.String(), quoted: tokQuot})
		}
		tok.Reset()
		tokAny, tokQuot = false, false
	}
	endCommand := func() {
		endToken()
		if len(cur) > 0 {
			cmds = append(cmds, finishCommand(cur, depth, &cmds))
			cur = nil
		}
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\\' && i+1 < len(s):
			tok.WriteByte(s[i+1])
			tokAny = true
			i++
		case ch == '\'':
			if j := strings.IndexByte(s[i+1:], '\''); j >= 0 {
				tok.WriteString(s[i+1 : i+1+j])
				tokAny, tokQuot = true, true
				i = i + 1 + j
			} else {
				tokAny, tokQuot = true, true // unterminated: rest is data
				tok.WriteString(s[i+1:])
				i = len(s)
			}
		case ch == '"':
			j := i + 1
			for ; j < len(s); j++ {
				if s[j] == '\\' && j+1 < len(s) {
					j++
					continue
				}
				if s[j] == '"' {
					break
				}
			}
			inner := s[i+1 : min(j, len(s))]
			// substitutions inside double quotes still execute
			for _, sub := range extractSubstitutions(inner) {
				cmds = append(cmds, scanText(sub, depth+1)...)
			}
			tok.WriteString(inner)
			tokAny, tokQuot = true, true
			i = j
		case ch == '$' && i+1 < len(s) && s[i+1] == '(':
			depthP := 1
			j := i + 2
			for ; j < len(s) && depthP > 0; j++ {
				if s[j] == '(' {
					depthP++
				} else if s[j] == ')' {
					depthP--
				}
			}
			cmds = append(cmds, scanText(s[i+2:j-1], depth+1)...)
			tok.WriteString(s[i : min(j, len(s))])
			tokAny, tokQuot = true, true // substitution result is data in the outer command
			i = j - 1
		case ch == '`':
			if j := strings.IndexByte(s[i+1:], '`'); j >= 0 {
				cmds = append(cmds, scanText(s[i+1:i+1+j], depth+1)...)
				tokAny, tokQuot = true, true
				i = i + 1 + j
			} else {
				tokAny = true
				i = len(s)
			}
		case ch == ';' || ch == '&' || ch == '|' || ch == '\n' || ch == '(' || ch == ')':
			endCommand()
		case ch == ' ' || ch == '\t':
			endToken()
		default:
			tok.WriteByte(ch)
			tokAny = true
		}
	}
	endCommand()
	return cmds
}

// shellKeywords are leading words that precede a command without being one.
var shellKeywords = map[string]bool{
	"if": true, "then": true, "elif": true, "else": true, "fi": true,
	"while": true, "until": true, "do": true, "done": true, "for": true,
	"time": true, "!": true, "{": true, "}": true,
}

// transparentWrappers run their argument vector as a command unchanged.
var transparentWrappers = map[string]bool{
	"nohup": true, "exec": true, "command": true, "builtin": true, "env": true,
}

var envAssignRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// finishCommand normalizes a raw token list into a simpleCommand: strips
// leading shell keywords, VAR=val assignments and transparent wrappers, and
// recursively scans sh -c / eval / xargs payloads (they execute).
func finishCommand(tokens []shellToken, depth int, sink *[]simpleCommand) simpleCommand {
	i := 0
	for i < len(tokens) {
		t := tokens[i]
		low := strings.ToLower(t.text)
		switch {
		case !t.quoted && shellKeywords[low]:
			i++
		case !t.quoted && envAssignRe.MatchString(t.text):
			i++
		case !t.quoted && transparentWrappers[low]:
			i++
			// env/nohup flags (-i, -u NAME, --) are arguments of the wrapper
			for i < len(tokens) && !tokens[i].quoted &&
				(strings.HasPrefix(tokens[i].text, "-") || envAssignRe.MatchString(tokens[i].text)) {
				i++
			}
		default:
			goto normalized
		}
	}
normalized:
	tokens = tokens[i:]
	c := simpleCommand{tokens: tokens}
	var words []string
	for _, t := range tokens {
		words = append(words, t.text)
	}
	c.raw = strings.ToLower(strings.Join(words, " "))

	if depth < maxScanDepth && len(tokens) > 0 {
		switch c.argv0Base() {
		case "sh", "bash", "zsh", "ksh", "dash":
			// scan the -c payload — it is a full command line
			for j := 1; j < len(tokens); j++ {
				if !tokens[j].quoted && tokens[j].text == "-c" && j+1 < len(tokens) {
					*sink = append(*sink, scanText(tokens[j+1].text, depth+1)...)
					break
				}
			}
		case "eval":
			// eval concatenates and re-parses its arguments
			var parts []string
			for j := 1; j < len(tokens); j++ {
				parts = append(parts, tokens[j].text)
			}
			*sink = append(*sink, scanText(strings.Join(parts, " "), depth+1)...)
		case "xargs":
			// xargs runs its first non-flag argument as a command
			for j := 1; j < len(tokens); j++ {
				if tokens[j].quoted || strings.HasPrefix(tokens[j].text, "-") {
					continue
				}
				var parts []string
				for _, t := range tokens[j:] {
					parts = append(parts, t.text)
				}
				*sink = append(*sink, scanText(strings.Join(parts, " "), depth+1)...)
				break
			}
		}
	}
	return c
}
