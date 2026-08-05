package cmd

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/doltserver"
)

func localOrphan(name string) doltserver.OrphanedDatabase {
	return doltserver.OrphanedDatabase{
		Name:     name,
		Path:     "/town/.dolt-data/" + name,
		LocalDir: true,
		Source:   "/town/.dolt-data",
	}
}

func remoteOrphan(name string) doltserver.OrphanedDatabase {
	return doltserver.OrphanedDatabase{
		Name:         name,
		Source:       "dolt.example.invalid:3306",
		Remote:       true,
		Unverifiable: true,
	}
}

// TestSelectCleanupTargets_NoArgsRemovesNothing is the safety property asked for
// in aegis-hphtm: a bare `gt dolt cleanup` used to remove every orphan it had just
// listed. Listing is now the whole of the default behaviour.
func TestSelectCleanupTargets_NoArgsRemovesNothing(t *testing.T) {
	orphans := []doltserver.OrphanedDatabase{localOrphan("testdb_a"), remoteOrphan("bobbin")}

	selected, err := selectCleanupTargets(orphans, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 0 {
		t.Errorf("expected nothing selected with no arguments, got %+v", selected)
	}
}

func TestSelectCleanupTargets_NamedLocalOrphanIsSelected(t *testing.T) {
	orphans := []doltserver.OrphanedDatabase{localOrphan("testdb_a"), localOrphan("testdb_b")}

	selected, err := selectCleanupTargets(orphans, []string{"testdb_b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 || selected[0].Name != "testdb_b" {
		t.Fatalf("expected only testdb_b, got %+v", selected)
	}
}

// TestSelectCleanupTargets_RefusesRemoteEvenWhenNamed keeps the shared server
// out of reach of this command. Naming a database is confirmation that you mean
// it, not evidence that this town is entitled to drop it.
func TestSelectCleanupTargets_RefusesRemoteEvenWhenNamed(t *testing.T) {
	orphans := []doltserver.OrphanedDatabase{remoteOrphan("bobbin")}

	_, err := selectCleanupTargets(orphans, []string{"bobbin"})
	if err == nil {
		t.Fatal("expected a refusal for a database with no local directory")
	}
	if !strings.Contains(err.Error(), "dolt.example.invalid:3306") {
		t.Errorf("refusal does not name the server: %v", err)
	}
}

// TestSelectCleanupTargets_RefusesUnlistedName stops this from becoming a
// general-purpose DROP: only what the command itself reported as orphaned is
// removable by it.
func TestSelectCleanupTargets_RefusesUnlistedName(t *testing.T) {
	orphans := []doltserver.OrphanedDatabase{localOrphan("testdb_a")}

	_, err := selectCleanupTargets(orphans, []string{"beads_aegis"})
	if err == nil {
		t.Fatal("expected a refusal for a database that was not listed as an orphan")
	}
	if !strings.Contains(err.Error(), "beads_aegis") {
		t.Errorf("refusal does not name the database: %v", err)
	}
}

func TestSelectCleanupTargets_DeduplicatesRepeatedNames(t *testing.T) {
	orphans := []doltserver.OrphanedDatabase{localOrphan("testdb_a")}

	selected, err := selectCleanupTargets(orphans, []string{"testdb_a", "testdb_a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 {
		t.Errorf("expected one selection for a repeated name, got %d", len(selected))
	}
}

func TestFirstRemovableName_PrefersALocalOrphan(t *testing.T) {
	orphans := []doltserver.OrphanedDatabase{remoteOrphan("bobbin"), localOrphan("testdb_a")}

	if got := firstRemovableName(orphans); got != "testdb_a" {
		t.Errorf("hint = %q, want the orphan this command could actually remove", got)
	}
}

func TestFirstRemovableName_PlaceholderWhenNoneRemovable(t *testing.T) {
	orphans := []doltserver.OrphanedDatabase{remoteOrphan("bobbin")}

	if got := firstRemovableName(orphans); got != "<database>" {
		t.Errorf("hint = %q, want a placeholder rather than a name that would be refused", got)
	}
}
