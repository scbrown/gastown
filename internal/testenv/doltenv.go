package testenv

import "os"

// ambientDoltEnv are the variables that point a process at a Dolt server.
//
// Both families matter and they are separate doors to the same server: Gas Town
// reads GT_DOLT_*, and the beads library reads BEADS_DOLT_*. A guard in one
// module does not protect the other (aegis-hy83r).
var ambientDoltEnv = []string{
	"GT_DOLT_HOST", "GT_DOLT_PORT", "GT_DOLT_USER", "GT_DOLT_PASSWORD",
	"BEADS_DOLT_HOST", "BEADS_DOLT_PORT", "BEADS_DOLT_PASSWORD",
	"BEADS_DOLT_SERVER_HOST", "BEADS_DOLT_SERVER_PORT",
}

// ScrubAmbientDoltEnv clears the Dolt server variables inherited from the
// caller's environment. Call it from TestMain in any package whose tests touch
// doltserver.
//
// This is a blast-radius guard, not hygiene. Tests here build a town under
// t.TempDir(), which reads as total isolation — but townRoot only chooses the
// DATA DIRECTORY. The SERVER comes from the environment, and on an operator's
// machine those variables point at the authoritative shared Dolt server. With
// them set, a suite enumerates production and its fix paths act on the result,
// from tests whose every visible input is a temp directory. Measured: eight
// tests in internal/doctor failed against an UNMODIFIED tree for exactly that
// reason, and they were failing because they were reading real databases.
//
// That is the more dangerous shape of the two — the tests still RUN, so it
// reads as ordinary breakage rather than as a suite pointed at production.
//
// Deliberately NOT using t.Setenv: this must run before any test starts, and
// TestMain has no *testing.T. Packages that want a real server ask for one
// explicitly (see EnsureDoltContainerForTestMain).
func ScrubAmbientDoltEnv() {
	for _, key := range ambientDoltEnv {
		_ = os.Unsetenv(key)
	}
}
