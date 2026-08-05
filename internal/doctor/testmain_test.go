package doctor

import (
	"os"
	"testing"
)

// TestMain neutralises an inherited remote-server environment for this
// package's unit tests, the same guard added to internal/doltserver.
//
// These tests build a town under t.TempDir(), which reads as isolation — but
// townRoot only chooses the data directory. The SERVER comes from
// GT_DOLT_HOST/GT_DOLT_PORT in the ambient environment, and on an operator's
// machine those point at the authoritative shared Dolt server. With them set,
// the orphan check enumerates PRODUCTION and its Fix would act on the result:
// measured on this host, eight tests in this package failed against an
// unmodified tree for exactly that reason, and they were failing because they
// were reading real databases, not because anything was broken.
//
// That is the more dangerous shape of the two — the tests still "run", so the
// failures read as ordinary breakage rather than as a suite pointed at
// production. Tests that want a real server ask for one explicitly.
func TestMain(m *testing.M) {
	for _, key := range []string{
		"GT_DOLT_HOST", "GT_DOLT_PORT", "GT_DOLT_USER", "GT_DOLT_PASSWORD",
		"BEADS_DOLT_HOST", "BEADS_DOLT_PORT", "BEADS_DOLT_PASSWORD",
		"BEADS_DOLT_SERVER_HOST", "BEADS_DOLT_SERVER_PORT",
	} {
		_ = os.Unsetenv(key)
	}
	os.Exit(m.Run())
}
