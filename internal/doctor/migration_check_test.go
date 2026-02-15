package doctor

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestDoltServerReachableCheck_NoRigsConfigured(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewDoltServerReachableCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}
	if result.Message != "No rigs configured for Dolt server mode" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestDoltServerReachableCheck_ReadsHostPortFromMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	// Start a TCP listener on a random port to simulate a reachable server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	// Create town-level .beads/metadata.json with the listener's host:port
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]interface{}{
		"backend":          "dolt",
		"dolt_mode":        "server",
		"dolt_database":    "hq",
		"dolt_server_host": "127.0.0.1",
		"dolt_server_port": port,
	}
	data, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	check := NewDoltServerReachableCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when server is reachable, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltServerReachableCheck_UnreachableServer(t *testing.T) {
	tmpDir := t.TempDir()

	// Create town-level .beads/metadata.json pointing to an unreachable address
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]interface{}{
		"backend":          "dolt",
		"dolt_mode":        "server",
		"dolt_database":    "hq",
		"dolt_server_host": "127.0.0.1",
		"dolt_server_port": 19999, // unlikely to be listening
	}
	data, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	check := NewDoltServerReachableCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError when server is unreachable, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltServerReachableCheck_DefaultsWhenHostPortMissing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create metadata.json with dolt_mode=server but NO host/port fields
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]interface{}{
		"backend":       "dolt",
		"dolt_mode":     "server",
		"dolt_database": "hq",
	}
	data, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	check := NewDoltServerReachableCheck()

	// readServerConfig should return defaults
	cfg, ok := check.readServerConfig(beadsDir, "hq")
	if !ok {
		t.Fatal("expected readServerConfig to return true for server-mode metadata")
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected default host 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.Port != 3307 {
		t.Errorf("expected default port 3307, got %d", cfg.Port)
	}
}

func TestDoltServerReachableCheck_CustomHostPort(t *testing.T) {
	tmpDir := t.TempDir()

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]interface{}{
		"backend":          "dolt",
		"dolt_mode":        "server",
		"dolt_database":    "hq",
		"dolt_server_host": "dolt.svc",
		"dolt_server_port": 3306,
	}
	data, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	check := NewDoltServerReachableCheck()

	cfg, ok := check.readServerConfig(beadsDir, "hq")
	if !ok {
		t.Fatal("expected readServerConfig to return true")
	}
	if cfg.Host != "dolt.svc" {
		t.Errorf("expected host dolt.svc, got %s", cfg.Host)
	}
	if cfg.Port != 3306 {
		t.Errorf("expected port 3306, got %d", cfg.Port)
	}
	if cfg.addr() != "dolt.svc:3306" {
		t.Errorf("expected addr dolt.svc:3306, got %s", cfg.addr())
	}
}

func TestDoltServerReachableCheck_MultipleRigsGroupedByAddress(t *testing.T) {
	tmpDir := t.TempDir()

	// Start a listener to simulate a reachable server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	// Create town-level .beads with the reachable server
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]interface{}{
		"backend":          "dolt",
		"dolt_mode":        "server",
		"dolt_database":    "hq",
		"dolt_server_host": "127.0.0.1",
		"dolt_server_port": port,
	}
	data, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	// Create a rig with the same server address (via mayor/rigs.json + rig beads dir)
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := map[string]interface{}{
		"rigs": map[string]interface{}{
			"testrig": map[string]interface{}{},
		},
	}
	rigsData, _ := json.Marshal(rigsJSON)
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), rigsData, 0600); err != nil {
		t.Fatal(err)
	}

	rigBeadsDir := filepath.Join(tmpDir, "testrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigMetadata := map[string]interface{}{
		"backend":          "dolt",
		"dolt_mode":        "server",
		"dolt_database":    "testrig",
		"dolt_server_host": "127.0.0.1",
		"dolt_server_port": port,
	}
	rigData, _ := json.Marshal(rigMetadata)
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "metadata.json"), rigData, 0600); err != nil {
		t.Fatal(err)
	}

	check := NewDoltServerReachableCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}
	// Both rigs should be counted
	expected := "Dolt server reachable (2 rig(s) in server mode)"
	if result.Message != expected {
		t.Errorf("expected message %q, got %q", expected, result.Message)
	}
}

func TestReadServerConfig_NotServerMode(t *testing.T) {
	tmpDir := t.TempDir()

	metadata := map[string]interface{}{
		"backend":       "dolt",
		"dolt_mode":     "embedded",
		"dolt_database": "hq",
	}
	data, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(tmpDir, "metadata.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	check := NewDoltServerReachableCheck()
	_, ok := check.readServerConfig(tmpDir, "hq")
	if ok {
		t.Error("expected readServerConfig to return false for non-server mode")
	}
}

func TestReadServerConfig_NoMetadataFile(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewDoltServerReachableCheck()
	_, ok := check.readServerConfig(tmpDir, "hq")
	if ok {
		t.Error("expected readServerConfig to return false for missing metadata")
	}
}
