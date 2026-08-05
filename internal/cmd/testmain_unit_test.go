//go:build !integration

package cmd

import (
	"os"
	"testing"

	"github.com/steveyegge/gastown/internal/testenv"
)

// TestMain clears the inherited Dolt server environment for NON-integration
// runs of this package (aegis-hy83r).
//
// The package already had a TestMain, but it is `//go:build integration` and
// routes to an ephemeral container. Under a plain `go test ./...` — which is
// what anyone who clones and runs the suite does — that file is not compiled,
// so this package had no guard at all. A coverage survey that greps for
// "func TestMain" without reading build tags reports it as protected.
//
// The build tags are mutually exclusive, so exactly one TestMain compiles.
func TestMain(m *testing.M) {
	testenv.ScrubAmbientDoltEnv()
	os.Exit(m.Run())
}
