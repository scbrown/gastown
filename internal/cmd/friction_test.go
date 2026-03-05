package cmd

import (
	"strings"
	"testing"
)

func TestParseFrictionProse(t *testing.T) {
	tests := []struct {
		name      string
		prose     string
		wantTried string
		wantGot   string
		wantWant  string
	}{
		{
			name:      "full structured prose",
			prose:     "tried gt worktree tapestry, got unknown command hook, want clean worktree creation",
			wantTried: "gt worktree tapestry",
			wantGot:   "unknown command hook",
			wantWant:  "clean worktree creation",
		},
		{
			name:      "tried and got only",
			prose:     "tried bd comment, got unknown command",
			wantTried: "bd comment",
			wantGot:   "unknown command",
			wantWant:  "",
		},
		{
			name:      "unstructured prose",
			prose:     "the git hooks are broken",
			wantTried: "the git hooks are broken",
			wantGot:   "",
			wantWant:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset globals
			frictionTried = ""
			frictionGot = ""
			frictionWant = ""

			parseFrictionProse(tt.prose)

			if frictionTried != tt.wantTried {
				t.Errorf("tried = %q, want %q", frictionTried, tt.wantTried)
			}
			if frictionGot != tt.wantGot {
				t.Errorf("got = %q, want %q", frictionGot, tt.wantGot)
			}
			if frictionWant != tt.wantWant {
				t.Errorf("want = %q, want %q", frictionWant, tt.wantWant)
			}
		})
	}
}

func TestGenerateFrictionID(t *testing.T) {
	id, err := generateFrictionID()
	if err != nil {
		t.Fatalf("generateFrictionID() error: %v", err)
	}
	if !strings.HasPrefix(id, "fr-") {
		t.Errorf("generateFrictionID() = %q, want prefix 'fr-'", id)
	}
	if len(id) != 13 {
		t.Errorf("generateFrictionID() = %q, want length 13, got %d", id, len(id))
	}
}

func TestSqlNullOrEscape(t *testing.T) {
	if got := sqlNullOrEscape(""); got != "NULL" {
		t.Errorf("sqlNullOrEscape(\"\") = %q, want NULL", got)
	}
	if got := sqlNullOrEscape("hello"); got != "'hello'" {
		t.Errorf("sqlNullOrEscape(\"hello\") = %q, want 'hello'", got)
	}
}
