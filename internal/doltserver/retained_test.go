package doltserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readRetainedLog(t *testing.T, townRoot string) []RetainedDatabase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(townRoot, RetainedDatabasesLog))
	if err != nil {
		t.Fatalf("reading retained log: %v", err)
	}
	var out []RetainedDatabase
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec RetainedDatabase
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("decoding %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

// The record must carry the rig attribution, because that is the only thing on
// this host that will not survive the removal (aegis-i08ls).
func TestRecordRetainedDatabase_KeepsTheAttributionThatIsAboutToBeLost(t *testing.T) {
	townRoot := t.TempDir()
	t.Setenv("GT_ROLE", "harding")

	if _, err := RecordRetainedDatabase(townRoot, RetainedDatabase{
		Database: "beads_ma",
		Server:   "dolt.example.invalid:3306",
		Rig:      "maldoon",
		Prefix:   "ma",
		Reason:   "orphan from bd init",
	}); err != nil {
		t.Fatalf("RecordRetainedDatabase: %v", err)
	}

	recs := readRetainedLog(t, townRoot)
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	got := recs[0]
	if got.Database != "beads_ma" || got.Server != "dolt.example.invalid:3306" {
		t.Errorf("database/server not recorded: %+v", got)
	}
	if got.Rig != "maldoon" || got.Prefix != "ma" {
		t.Errorf("rig attribution missing — this is the field that cannot be recovered later: %+v", got)
	}
	if got.Actor != "harding" {
		t.Errorf("actor = %q, want the GT_ROLE — every agent shares one unix user", got.Actor)
	}
	if got.At.IsZero() || time.Since(got.At) > time.Hour {
		t.Errorf("timestamp not filled in sensibly: %v", got.At)
	}
}

func TestRecordRetainedDatabase_AppendsRatherThanOverwrites(t *testing.T) {
	townRoot := t.TempDir()

	for _, name := range []string{"first", "second", "third"} {
		if _, err := RecordRetainedDatabase(townRoot, RetainedDatabase{
			Database: name, Server: "dolt.example.invalid:3306", Reason: "test",
		}); err != nil {
			t.Fatalf("recording %s: %v", name, err)
		}
	}

	recs := readRetainedLog(t, townRoot)
	if len(recs) != 3 {
		t.Fatalf("expected 3 appended records, got %d — a truncating log loses the earlier attributions", len(recs))
	}
	if recs[0].Database != "first" || recs[2].Database != "third" {
		t.Errorf("records out of order or overwritten: %+v", recs)
	}
}

// A failure to write the log must not be silent, and must not be fatal either.
// Leaving the database behind is already the degraded path; failing the caller's
// operation over a log file would trade wasted disk for a broken command.
func TestRecordRetainedDatabase_ReportsButDoesNotHideAWriteFailure(t *testing.T) {
	townRoot := t.TempDir()
	// Make logs/ a FILE, so creating the directory beneath it must fail.
	if err := os.WriteFile(filepath.Join(townRoot, "logs"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("setting up: %v", err)
	}

	logPath, err := RecordRetainedDatabase(townRoot, RetainedDatabase{
		Database: "x", Server: "dolt.example.invalid:3306", Reason: "test",
	})
	if err == nil {
		t.Fatal("expected an error when the log cannot be written")
	}
	if logPath == "" {
		t.Error("the path must be returned even on failure, so the caller can name it")
	}
}

// The ruling in aegis-i08ls is that Gas Town does not DROP on a shared server.
// This pins the mechanism that makes that true, so a later change that "fixes"
// the retention by adding a drop has to delete a test that says why not.
func TestRemoveDatabase_NeverDropsARemoteOnlyDatabase(t *testing.T) {
	t.Setenv("GT_DOLT_HOST", "dolt.example.invalid")
	t.Setenv("GT_DOLT_PORT", "3306")
	townRoot := t.TempDir()

	// force=true is what both live callers pass. It must not buy a remote drop:
	// it means "force the rig removal", and silently upgrading its blast radius
	// would give a cleanup command the power to destroy.
	err := RemoveDatabase(townRoot, "a_live_service_db", true)
	if err == nil {
		t.Fatal("force=true issued a drop against a shared remote server")
	}
	if !strings.Contains(err.Error(), "not on this host") {
		t.Errorf("refusal does not explain where the database actually is: %v", err)
	}
}
