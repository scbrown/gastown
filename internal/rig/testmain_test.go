package rig

import (
	"os"
	"testing"

	"github.com/steveyegge/gastown/internal/testenv"
)

// TestMain clears the inherited Dolt server environment. See
// testenv.ScrubAmbientDoltEnv — this package's tests touch doltserver, and
// t.TempDir() isolates the data directory, not the server (aegis-hy83r).
//
// These tests PASS today under an ambient crew environment. That is not
// evidence of safety, only that nothing they did failed loudly: dropRigOrphanDBs
// reaches RemoveDatabase, which is a destructive path.
func TestMain(m *testing.M) {
	testenv.ScrubAmbientDoltEnv()
	os.Exit(m.Run())
}
