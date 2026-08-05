package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/doltserver"
)

// The ruling in aegis-i08ls: on a shared remote server the doctor REPORTS and
// RECORDS an orphan, and never drops it. This exercises the wiring at the call
// site — the pieces are unit-tested separately, but only this proves Fix()
// actually takes the reporting path instead of erroring out or dropping.
func TestOrphanedDatabaseFix_RecordsRemoteOrphanInsteadOfDropping(t *testing.T) {
	t.Setenv("GT_DOLT_HOST", "dolt.example.invalid")
	t.Setenv("GT_DOLT_PORT", "3306")
	townRoot := t.TempDir()
	t.Setenv("GT_ROLE", "harding")

	check := NewDoltOrphanedDatabaseCheck()
	// A database enumerated from the remote with nothing on this host — the
	// state that made a live service database look like an empty local dir.
	check.orphanNames = []string{"a_live_service_db"}

	if err := check.Fix(&CheckContext{TownRoot: townRoot}); err != nil {
		t.Fatalf("Fix must not fail on a database it is declining to drop: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(townRoot, doltserver.RetainedDatabasesLog))
	if err != nil {
		t.Fatalf("no attribution record written — the orphan is now unattributable: %v", err)
	}
	var rec doltserver.RetainedDatabase
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &rec); err != nil {
		t.Fatalf("decoding record: %v", err)
	}
	if rec.Database != "a_live_service_db" {
		t.Errorf("wrong database recorded: %+v", rec)
	}
	if !strings.Contains(rec.Server, "dolt.example.invalid") {
		t.Errorf("record does not name the server the database remains on: %+v", rec)
	}
	if rec.Actor != "harding" {
		t.Errorf("actor = %q, want the GT_ROLE", rec.Actor)
	}
}

// CONTROL: a genuine local failure must still be an error. Without this, the
// test above passes equally against a Fix() that swallows everything.
func TestOrphanedDatabaseFix_StillErrorsOnALocalFailure(t *testing.T) {
	townRoot := t.TempDir() // local town: no remote configured

	check := NewDoltOrphanedDatabaseCheck()
	check.orphanNames = []string{"not_there"}

	if err := check.Fix(&CheckContext{TownRoot: townRoot}); err == nil {
		t.Fatal("a local removal failure was swallowed — Fix reported success having done nothing")
	}
}
