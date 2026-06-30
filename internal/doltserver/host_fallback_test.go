package doltserver

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultConfig_HostFromDaemonEnvFallback reproduces aegis-c6o: a process
// whose own env lacks GT_DOLT_HOST (notably the deacon, which runs sling
// connection-capacity checks) must still resolve the remote host from the
// town's daemon/daemon.env, rather than letting EffectiveHost default to
// 127.0.0.1 and dialing a non-existent local server.
func TestDefaultConfig_HostFromDaemonEnvFallback(t *testing.T) {
	town := t.TempDir()
	t.Setenv("GT_DOLT_HOST", "")
	os.Unsetenv("GT_DOLT_HOST")

	daemonDir := filepath.Join(town, "daemon")
	if err := os.MkdirAll(daemonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(daemonDir, "daemon.env"),
		[]byte("# town daemon env\nGT_DOLT_HOST=dolt.lan\nGT_DOLT_PORT=3306\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	config := DefaultConfig(town)
	if config.Host != "dolt.lan" {
		t.Fatalf("config.Host = %q, want dolt.lan (daemon.env fallback)", config.Host)
	}
	if got := config.EffectiveHost(); got != "dolt.lan" {
		t.Fatalf("EffectiveHost = %q, want dolt.lan", got)
	}
}

// TestDefaultConfig_HostEnvBeatsDaemonEnv confirms explicit process env still
// wins over the daemon.env fallback.
func TestDefaultConfig_HostEnvBeatsDaemonEnv(t *testing.T) {
	town := t.TempDir()
	t.Setenv("GT_DOLT_HOST", "override.lan")

	daemonDir := filepath.Join(town, "daemon")
	if err := os.MkdirAll(daemonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(daemonDir, "daemon.env"),
		[]byte("GT_DOLT_HOST=dolt.lan\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if config := DefaultConfig(town); config.Host != "override.lan" {
		t.Fatalf("config.Host = %q, want override.lan (env wins)", config.Host)
	}
}

// TestDefaultConfig_HostNoneStaysLocal confirms that with no host configured
// anywhere, EffectiveHost keeps its 127.0.0.1 default (no regression).
func TestDefaultConfig_HostNoneStaysLocal(t *testing.T) {
	town := t.TempDir()
	t.Setenv("GT_DOLT_HOST", "")
	os.Unsetenv("GT_DOLT_HOST")

	config := DefaultConfig(town)
	if config.Host != "" {
		t.Fatalf("config.Host = %q, want empty", config.Host)
	}
	if got := config.EffectiveHost(); got != "127.0.0.1" {
		t.Fatalf("EffectiveHost = %q, want 127.0.0.1 default", got)
	}
}
