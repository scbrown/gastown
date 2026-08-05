package doltserver

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/testenv"
)

// TestMain neutralises an inherited remote-server environment for this
// package's unit tests.
//
// This is not hygiene, it is a blast-radius guard. These tests build their town
// under t.TempDir(), which reads as isolation — but townRoot only chooses the
// data directory. The SERVER comes from GT_DOLT_HOST/GT_DOLT_PORT in the
// ambient environment, and on an operator's machine those point at the
// authoritative shared Dolt server. With them set, ListDatabases enumerates
// production and RemoveDatabase issues DROP DATABASE against it, from a test
// whose every visible input is a temp directory. A test suite run that way is
// what left an orphan database on the shared server (aegis-hphtm). Tests that want
// a real server ask for a container via testutil.
func TestMain(m *testing.M) {
	testenv.ScrubAmbientDoltEnv()
	os.Exit(m.Run())
}

func TestDatabaseSource_LocalReportsDataDir(t *testing.T) {
	townRoot := t.TempDir()

	source, remote := DatabaseSource(townRoot)
	if remote {
		t.Errorf("expected local, got remote source %q", source)
	}
	if want := filepath.Join(townRoot, ".dolt-data"); source != want {
		t.Errorf("source = %q, want %q", source, want)
	}
}

func TestDatabaseSource_RemoteNamesTheServer(t *testing.T) {
	t.Setenv("GT_DOLT_HOST", "dolt.example.invalid")
	t.Setenv("GT_DOLT_PORT", "3306")

	source, remote := DatabaseSource(t.TempDir())
	if !remote {
		t.Fatalf("expected remote for a non-loopback host, got local %q", source)
	}
	if !strings.Contains(source, "dolt.example.invalid") {
		t.Errorf("source %q does not name the server", source)
	}
	if strings.Contains(source, ".dolt-data") {
		t.Errorf("source %q reports a local directory for a remote server", source)
	}
}

// TestBuildOrphanList_RemoteOnlyHasNoPathOrSize is the regression test for the
// defect itself: a database enumerated from a remote server, with nothing on
// this host, must not be described as a local directory containing 0 bytes.
func TestBuildOrphanList_RemoteOnlyHasNoPathOrSize(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dolt-data")

	orphans := buildOrphanList(dataDir, []string{"bobbin"}, map[string]bool{}, "dolt.example.invalid:3306", true)

	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(orphans))
	}
	o := orphans[0]
	if o.Path != "" {
		t.Errorf("Path = %q, want empty — the directory does not exist and the path was manufactured", o.Path)
	}
	if o.LocalDir {
		t.Error("LocalDir = true for a database with no local directory")
	}
	if o.SizeBytes != 0 || !o.Unverifiable {
		t.Errorf("expected an unverifiable orphan with no measured size, got %+v", o)
	}
	if o.Source != "dolt.example.invalid:3306" || !o.Remote {
		t.Errorf("orphan does not record the server it came from: %+v", o)
	}
}

func TestBuildOrphanList_LocalKeepsPathAndSize(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dolt-data")
	setupDoltDB(t, dataDir, "testdb_leftover")

	orphans := buildOrphanList(dataDir, []string{"testdb_leftover"}, map[string]bool{}, dataDir, false)

	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(orphans))
	}
	o := orphans[0]
	if !o.LocalDir {
		t.Error("LocalDir = false for a database that is on disk")
	}
	if o.Path != filepath.Join(dataDir, "testdb_leftover") {
		t.Errorf("Path = %q, want the real directory", o.Path)
	}
	if o.Unverifiable {
		t.Error("a local directory is verifiable — we can look at it")
	}
}

// TestBuildOrphanList_MixedSources covers the case that actually occurs on a
// remote town: one database happens to have a local directory, the rest do not.
func TestBuildOrphanList_MixedSources(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dolt-data")
	setupDoltDB(t, dataDir, "here")

	orphans := buildOrphanList(dataDir, []string{"here", "elsewhere"}, map[string]bool{}, "dolt.example.invalid:3306", true)

	if len(orphans) != 2 {
		t.Fatalf("expected 2 orphans, got %d", len(orphans))
	}
	byName := map[string]OrphanedDatabase{}
	for _, o := range orphans {
		byName[o.Name] = o
	}
	if !byName["here"].LocalDir || byName["here"].Unverifiable {
		t.Errorf("'here' should be local and verifiable: %+v", byName["here"])
	}
	if byName["elsewhere"].LocalDir || !byName["elsewhere"].Unverifiable {
		t.Errorf("'elsewhere' should be remote-only and unverifiable: %+v", byName["elsewhere"])
	}
}

func TestBuildOrphanList_SkipsReferencedAndProtected(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dolt-data")

	orphans := buildOrphanList(dataDir,
		[]string{"beads_global", "spoken_for", "loose"},
		map[string]bool{"spoken_for": true},
		"dolt.example.invalid:3306", true)

	if len(orphans) != 1 || orphans[0].Name != "loose" {
		t.Fatalf("expected only 'loose' proposed, got %+v", orphans)
	}
}

// TestFindOrphanedDatabases_RigNameCountsAsReferenced is the fleet case: a rig
// named "bobbin" whose prefix is "bo" and whose metadata points at a different
// database name. The database literally called "bobbin" matched none of the
// precise lookups and was proposed for removal.
func TestFindOrphanedDatabases_RigNameCountsAsReferenced(t *testing.T) {
	townRoot := t.TempDir()
	dataDir := filepath.Join(townRoot, ".dolt-data")

	setupDoltDB(t, dataDir, "bobbin")
	setupRigsJSON(t, townRoot, []string{"bobbin"})
	setupRigMetadata(t, townRoot, "bobbin", "bobbin_v52")

	orphans, err := FindOrphanedDatabases(townRoot)
	if err != nil {
		t.Fatalf("FindOrphanedDatabases: %v", err)
	}
	for _, o := range orphans {
		if o.Name == "bobbin" {
			t.Fatalf("database named after the rig 'bobbin' was proposed for removal: %+v", o)
		}
	}
}

// TestRemoveDatabase_RefusesRemoteOnlyAndSaysSo pins both halves: the refusal,
// and what the refusal says. The old message reported the database "not found"
// at a .dolt-data/ path that this command had invented — which reads as a stale
// registry entry rather than as a live database on another host.
func TestRemoveDatabase_RefusesRemoteOnlyAndSaysSo(t *testing.T) {
	t.Setenv("GT_DOLT_HOST", "dolt.example.invalid")
	t.Setenv("GT_DOLT_PORT", "3306")
	townRoot := t.TempDir()

	err := RemoveDatabase(townRoot, "bobbin", true)
	if err == nil {
		t.Fatal("expected refusal for a database with no local directory")
	}
	if !errors.Is(err, ErrNoLocalDatabaseDir) {
		t.Errorf("error is not ErrNoLocalDatabaseDir: %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "dolt.example.invalid") {
		t.Errorf("refusal does not name the server: %v", msg)
	}
	if !strings.Contains(msg, "not on this host") {
		t.Errorf("refusal does not say the database is elsewhere: %v", msg)
	}
}

func TestRemoveDatabase_LocalMissingStillSaysNotFound(t *testing.T) {
	townRoot := t.TempDir()

	err := RemoveDatabase(townRoot, "gone", true)
	if err == nil {
		t.Fatal("expected an error for a database that is not there")
	}
	if !errors.Is(err, ErrNoLocalDatabaseDir) {
		t.Errorf("error is not ErrNoLocalDatabaseDir: %v", err)
	}
	if strings.Contains(err.Error(), "remote Dolt server") {
		t.Errorf("local town reported a remote server: %v", err)
	}
}
