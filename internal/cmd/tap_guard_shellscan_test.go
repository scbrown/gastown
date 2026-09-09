package cmd

import (
	"strings"
	"testing"
)

// aegis-7uetct: an UNTERMINATED heredoc must not disarm the scanner.
//
// stripHeredocBodies DROPS body lines. If a `<< WORD` never closes, every
// following line was dropped, so the matchers received an empty command while
// the real one sailed past. Measured against the deployed gt before the fix:
// `echo "prose << EOF inline"` + a destructive command exited 0, while the bare
// destructive command exited 2.
//
// The pair is the point. The unterminated arm alone passes against a scanner
// that never strips anything; the well-formed arm is what proves the stripper
// still does its job, which is the whole reason it exists (documenting a hazard
// must not be blocked).
func TestUnterminatedHeredocDoesNotDisarmScanner(t *testing.T) {
	const hazard = "git reset --hard HEAD~1"

	t.Run("unterminated heredoc keeps the following command visible", func(t *testing.T) {
		cmd := "echo \"prose << EOF inline\"\n" + hazard
		cleaned, _ := stripHeredocBodies(cmd)
		if !strings.Contains(cleaned, hazard) {
			t.Fatalf("unterminated heredoc swallowed the command; cleaned=%q", cleaned)
		}
	})

	t.Run("CONTROL well-formed heredoc still has its body stripped", func(t *testing.T) {
		cmd := "cat > /tmp/x <<'EOF'\n" + hazard + "\nEOF\necho done"
		cleaned, _ := stripHeredocBodies(cmd)
		if strings.Contains(cleaned, hazard) {
			t.Fatalf("body was NOT stripped; documenting a hazard would be blocked: cleaned=%q", cleaned)
		}
		if !strings.Contains(cleaned, "echo done") {
			t.Fatalf("stripper over-consumed past the delimiter; cleaned=%q", cleaned)
		}
	})

	t.Run("CONTROL closed heredoc then a real hazard stays visible", func(t *testing.T) {
		cmd := "cat > /tmp/x <<'EOF'\nprose\nEOF\n" + hazard
		cleaned, _ := stripHeredocBodies(cmd)
		if !strings.Contains(cleaned, hazard) {
			t.Fatalf("hazard after a closed body was dropped; cleaned=%q", cleaned)
		}
	})
}
