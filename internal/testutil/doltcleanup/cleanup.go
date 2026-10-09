// Package doltcleanup registers ownership-checked Dolt cleanup for E2E tests.
// It is separate from testutil so doltserver integration tests can import
// container helpers without importing doltserver back through testutil.
package doltcleanup

import (
	"testing"

	"github.com/steveyegge/gastown/internal/doltserver"
)

// ReapOwnedDoltOnCleanup registers test cleanup for Dolt servers whose metadata
// and process args prove they belong to townRoot. It never kills by broad name or
// port, so production Dolt is protected when tests run inside a real workspace.
func ReapOwnedDoltOnCleanup(t testing.TB, townRoot string) {
	t.Helper()
	t.Cleanup(func() {
		stopped, err := doltserver.ReapOwnedTestServers(townRoot)
		if err != nil {
			t.Logf("owned Dolt cleanup skipped: %v", err)
			return
		}
		if stopped > 0 {
			t.Logf("stopped %d owned Dolt sql-server process(es)", stopped)
		}
	})
}
