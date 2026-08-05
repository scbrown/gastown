package testenv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEveryDoltTouchingPackageScrubsAmbientEnv fails when a test package that
// touches doltserver has no TestMain calling ScrubAmbientDoltEnv.
//
// The gap this closes is invisible by construction (aegis-hy83r). A package
// without the guard does not announce itself: its tests run, they usually pass,
// and when they fail they look like ordinary breakage rather than a suite
// pointed at the authoritative shared Dolt server. Three hand-written copies of
// the scrub loop would drift, and nothing would say so — so the check is the
// mechanism and the helper is only the convenience.
//
// It parses build tags rather than grepping. A survey that greps for
// "func TestMain" is wrong twice over: it also matches `func TestMaintain...`,
// and it counts a `//go:build integration` TestMain as covering the plain
// `go test ./...` run that most people actually execute. Both mistakes were
// made while investigating this, and both report MORE coverage than exists.
func TestEveryDoltTouchingPackageScrubsAmbientEnv(t *testing.T) {
	root := repoRoot(t)

	var unguarded []string
	var checked int

	for _, dir := range testDirsTouchingDoltserver(t, root) {
		checked++
		if !hasUnconditionalScrubbingTestMain(t, dir) {
			rel, _ := filepath.Rel(root, dir)
			unguarded = append(unguarded, rel)
		}
	}

	// CONTROL: if the walk found nothing, this test proves nothing — an empty
	// result would silently report full coverage. That is the same vacuous-probe
	// failure the bead is about, one level up.
	if checked == 0 {
		t.Fatal("CONTROL FAILED: found no test packages touching doltserver — the walk is broken, " +
			"and an empty result here would read as full coverage")
	}

	if len(unguarded) > 0 {
		sort.Strings(unguarded)
		t.Errorf("%d of %d test package(s) touching doltserver have no TestMain calling "+
			"testenv.ScrubAmbientDoltEnv:\n  %s\n\n"+
			"Their tests build a town under t.TempDir(), which isolates the DATA DIRECTORY "+
			"but not the SERVER — that comes from GT_DOLT_*/BEADS_DOLT_* in the ambient "+
			"environment, which on an operator's machine points at the shared production "+
			"Dolt server. Add:\n\n"+
			"    func TestMain(m *testing.M) {\n"+
			"        testenv.ScrubAmbientDoltEnv()\n"+
			"        os.Exit(m.Run())\n"+
			"    }",
			len(unguarded), checked, strings.Join(unguarded, "\n  "))
	}
}

// repoRoot walks up from the working directory to the module root.
func repoRoot(t *testing.T) string {
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
			t.Fatal("could not find go.mod above the working directory")
		}
		dir = parent
	}
}

// testDirsTouchingDoltserver returns directories with _test.go files that
// reference the doltserver package.
func testDirsTouchingDoltserver(t *testing.T, root string) []string {
	t.Helper()
	seen := map[string]bool{}
	var dirs []string

	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
				return nil
			}
			dir := filepath.Dir(path)
			// The doltserver package's OWN tests call these functions directly,
			// with no "doltserver." qualifier, so a content match alone misses
			// the single most important package. Found by reading this check's
			// own count (3, not 4) rather than by it reporting anything wrong —
			// a coverage check that silently under-counts its input is the same
			// vacuous probe one level up.
			isDoltserverItself := dir == filepath.Join(root, "internal", "doltserver")
			if !isDoltserverItself {
				data, readErr := os.ReadFile(path)
				if readErr != nil || !strings.Contains(string(data), "doltserver.") {
					return nil
				}
			}
			// This package holds the guard itself and imports nothing.
			if dir == filepath.Join(root, "internal", "testenv") {
				return nil
			}
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", top, err)
		}
	}
	return dirs
}

// hasUnconditionalScrubbingTestMain reports whether dir declares a TestMain
// that calls ScrubAmbientDoltEnv in a file compiled by a PLAIN `go test` run —
// i.e. not gated behind a build tag such as `integration`.
func hasUnconditionalScrubbingTestMain(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// Skip files a plain `go test` would not compile. `!integration` is
		// compiled by a plain run, so it counts; `integration` does not.
		if constrainedOutOfPlainRun(string(src)) {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
		if err != nil {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "TestMain" || fn.Recv != nil {
				continue
			}
			if strings.Contains(string(src), "ScrubAmbientDoltEnv()") {
				return true
			}
		}
	}
	return false
}

// constrainedOutOfPlainRun reports whether a //go:build line excludes the file
// from a plain `go test` run. Deliberately conservative: it only recognises a
// bare positive tag (e.g. `//go:build integration`), which is the case that
// actually occurred and the one a grep-based survey miscounts.
func constrainedOutOfPlainRun(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "//go:build") {
			if line != "" && !strings.HasPrefix(line, "//") {
				return false // past the header
			}
			continue
		}
		expr := strings.TrimSpace(strings.TrimPrefix(line, "//go:build"))
		// A bare tag with no negation excludes the plain run.
		if expr != "" && !strings.ContainsAny(expr, "!|&()") {
			return true
		}
	}
	return false
}
