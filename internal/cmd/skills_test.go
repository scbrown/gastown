package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skillDirs are the three directories where SKILL.md files are expected.
// .agents/skills/ is the source of truth; the others are symlinks.
var skillDirs = [3]string{
	".agents/skills",
	".claude/skills",
	"docs/skills",
}

// findProjectRoot walks up from the test binary's working directory until it
// finds the marker file (go.mod) that indicates the project root.
func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (no go.mod found)")
		}
		dir = parent
	}
}

// looksLikeSkillFile returns true if the file content has the minimum markers
// of a SKILL.md: YAML frontmatter delimiters and a name + description field.
func looksLikeSkillFile(content []byte) bool {
	s := string(content)
	if !strings.HasPrefix(s, "---\n") {
		return false
	}
	// Must have closing frontmatter delimiter
	rest := s[4:]
	if !strings.Contains(rest, "\n---\n") && !strings.Contains(rest, "\n---\r\n") {
		return false
	}
	return strings.Contains(s, "name:") && strings.Contains(s, "description:")
}

// discoverSkills returns the names of all skills in .agents/skills/ (source of truth).
// Each skill is a directory containing a SKILL.md with valid frontmatter.
func discoverSkills(t *testing.T, root string) []string {
	t.Helper()
	agentsDir := filepath.Join(root, ".agents", "skills")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read .agents/skills: %v", err)
	}

	var skills []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillPath := filepath.Join(agentsDir, e.Name(), "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			continue // directory without SKILL.md is not a skill
		}
		if looksLikeSkillFile(content) {
			skills = append(skills, e.Name())
		}
	}
	return skills
}

// TestSkills_SourceOfTruthIsRealFile verifies that every skill in
// .agents/skills/<name>/SKILL.md is a real file (not a symlink).
func TestSkills_SourceOfTruthIsRealFile(t *testing.T) {
	root := findProjectRoot(t)
	skills := discoverSkills(t, root)
	if len(skills) == 0 {
		t.Skip("no skills found in .agents/skills/")
	}

	for _, name := range skills {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(root, ".agents", "skills", name, "SKILL.md")
			info, err := os.Lstat(p)
			if err != nil {
				t.Fatalf("stat: %v", err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				t.Errorf(".agents/skills/%s/SKILL.md should be a real file, not a symlink", name)
			}
		})
	}
}

// TestSkills_DocsSymlinkPointsToSource verifies that docs/skills/<name>/SKILL.md
// is a file symlink that resolves to .agents/skills/<name>/SKILL.md.
func TestSkills_DocsSymlinkPointsToSource(t *testing.T) {
	root := findProjectRoot(t)
	skills := discoverSkills(t, root)
	if len(skills) == 0 {
		t.Skip("no skills found in .agents/skills/")
	}

	for _, name := range skills {
		t.Run(name, func(t *testing.T) {
			docPath := filepath.Join(root, "docs", "skills", name, "SKILL.md")
			info, err := os.Lstat(docPath)
			if err != nil {
				t.Fatalf("docs/skills/%s/SKILL.md does not exist: %v\n"+
					"  Create it: mkdir -p docs/skills/%s && "+
					"ln -s ../../../.agents/skills/%s/SKILL.md docs/skills/%s/SKILL.md",
					name, err, name, name, name)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				t.Errorf("docs/skills/%s/SKILL.md should be a symlink, not a real file", name)
				return
			}

			// Resolve and verify it points to the source of truth
			resolved, err := filepath.EvalSymlinks(docPath)
			if err != nil {
				t.Fatalf("cannot resolve symlink: %v", err)
			}
			expected, _ := filepath.EvalSymlinks(filepath.Join(root, ".agents", "skills", name, "SKILL.md"))
			if resolved != expected {
				t.Errorf("docs/skills/%s/SKILL.md resolves to %s, want %s", name, resolved, expected)
			}
		})
	}
}

// TestSkills_ClaudeSymlinkPointsToSource verifies that .claude/skills/<name>/
// is a folder symlink whose SKILL.md resolves to .agents/skills/<name>/SKILL.md.
func TestSkills_ClaudeSymlinkPointsToSource(t *testing.T) {
	root := findProjectRoot(t)
	skills := discoverSkills(t, root)
	if len(skills) == 0 {
		t.Skip("no skills found in .agents/skills/")
	}

	for _, name := range skills {
		t.Run(name, func(t *testing.T) {
			claudeDir := filepath.Join(root, ".claude", "skills", name)
			info, err := os.Lstat(claudeDir)
			if err != nil {
				t.Fatalf(".claude/skills/%s does not exist: %v\n"+
					"  Create it: ln -s ../../.agents/skills/%s .claude/skills/%s",
					name, err, name, name)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				t.Errorf(".claude/skills/%s should be a folder symlink, not a real directory", name)
				return
			}

			// Verify SKILL.md is reachable through the symlink
			skillPath := filepath.Join(claudeDir, "SKILL.md")
			resolved, err := filepath.EvalSymlinks(skillPath)
			if err != nil {
				t.Fatalf("cannot resolve .claude/skills/%s/SKILL.md: %v", name, err)
			}
			expected, _ := filepath.EvalSymlinks(filepath.Join(root, ".agents", "skills", name, "SKILL.md"))
			if resolved != expected {
				t.Errorf(".claude/skills/%s/SKILL.md resolves to %s, want %s", name, resolved, expected)
			}
		})
	}
}

// TestSkills_FrontmatterHasRequiredFields verifies that every SKILL.md has
// valid frontmatter with name and description fields.
func TestSkills_FrontmatterHasRequiredFields(t *testing.T) {
	root := findProjectRoot(t)
	skills := discoverSkills(t, root)
	if len(skills) == 0 {
		t.Skip("no skills found in .agents/skills/")
	}

	for _, name := range skills {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(root, ".agents", "skills", name, "SKILL.md")
			content, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			s := string(content)
			// Extract frontmatter between first and second ---
			if !strings.HasPrefix(s, "---\n") {
				t.Fatal("SKILL.md must start with --- frontmatter delimiter")
			}
			endIdx := strings.Index(s[4:], "\n---\n")
			if endIdx < 0 {
				endIdx = strings.Index(s[4:], "\n---\r\n")
			}
			if endIdx < 0 {
				t.Fatal("SKILL.md missing closing --- frontmatter delimiter")
			}
			frontmatter := s[4 : 4+endIdx]

			if !strings.Contains(frontmatter, "name:") {
				t.Error("frontmatter missing 'name:' field")
			}
			if !strings.Contains(frontmatter, "description:") {
				t.Error("frontmatter missing 'description:' field")
			}

			// Name in frontmatter should match directory name
			for _, line := range strings.Split(frontmatter, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					val := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
					if val != name {
						t.Errorf("frontmatter name %q does not match directory name %q", val, name)
					}
				}
			}
		})
	}
}

// TestSkills_NoStraySkillFiles verifies that no SKILL.md files exist outside
// the three expected directories. This catches skills that were manually
// created in the wrong location or not cleaned up after a move.
func TestSkills_NoStraySkillFiles(t *testing.T) {
	root := findProjectRoot(t)

	// Build set of allowed SKILL.md paths (resolved to absolute)
	allowed := map[string]bool{}
	skills := discoverSkills(t, root)
	for _, name := range skills {
		for _, dir := range skillDirs {
			p := filepath.Join(root, dir, name, "SKILL.md")
			// Resolve symlinks so we match the real path
			if resolved, err := filepath.EvalSymlinks(p); err == nil {
				allowed[resolved] = true
			}
			// Also allow the symlink path itself
			abs, _ := filepath.Abs(p)
			allowed[abs] = true
		}
	}

	var stray []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden directories other than .agents and .claude
		base := filepath.Base(path)
		if info.IsDir() && strings.HasPrefix(base, ".") {
			switch base {
			case ".agents", ".claude":
				return nil // search inside these
			default:
				return filepath.SkipDir
			}
		}
		// Skip vendor, node_modules, etc
		if info.IsDir() {
			switch base {
			case "vendor", "node_modules", "testdata":
				return filepath.SkipDir
			}
		}

		if base != "SKILL.md" {
			return nil
		}

		// Read and check if it looks like a real skill file
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if !looksLikeSkillFile(content) {
			return nil
		}

		// Resolve to absolute for comparison
		abs, _ := filepath.Abs(path)
		resolved, _ := filepath.EvalSymlinks(path)
		if !allowed[abs] && !allowed[resolved] {
			rel, _ := filepath.Rel(root, path)
			stray = append(stray, rel)
		}
		return nil
	})

	if len(stray) > 0 {
		t.Errorf("found SKILL.md files outside the expected directories:\n  %s\n\n"+
			"  Skills should live in .agents/skills/<name>/SKILL.md (source of truth)\n"+
			"  with symlinks in docs/skills/ and .claude/skills/.\n"+
			"  See .agents/AGENTS.md for the convention.",
			strings.Join(stray, "\n  "))
	}
}
