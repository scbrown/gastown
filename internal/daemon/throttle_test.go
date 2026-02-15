package daemon

import (
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestDaemon(throttle *ThrottleConfig) *Daemon {
	return &Daemon{
		logger: log.New(os.Stderr, "test: ", 0),
		patrolConfig: &DaemonPatrolConfig{
			Throttle: throttle,
		},
	}
}

func TestThrottle_Disabled(t *testing.T) {
	d := newTestDaemon(nil)
	throttle, _ := d.shouldThrottleSpawns()
	if throttle {
		t.Error("expected no throttle when config is nil")
	}

	d = newTestDaemon(&ThrottleConfig{Enabled: false})
	throttle, _ = d.shouldThrottleSpawns()
	if throttle {
		t.Error("expected no throttle when disabled")
	}
}

func TestThrottle_MissingStateFile(t *testing.T) {
	d := newTestDaemon(&ThrottleConfig{
		Enabled:   true,
		StateFile: "/nonexistent/path/state.json",
	})
	throttle, _ := d.shouldThrottleSpawns()
	if throttle {
		t.Error("expected fail-open when state file missing")
	}
}

func TestThrottle_HighContention(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	ts := time.Now().Format(time.RFC3339)
	os.WriteFile(stateFile, []byte(`{"contention_level":2,"gaming_active":1,"load_1m":18.5,"memory_used_percent":82,"gpu_utilization":65,"claude_processes":12,"timestamp":"`+ts+`"}`), 0644)

	d := newTestDaemon(&ThrottleConfig{
		Enabled:   true,
		StateFile: stateFile,
	})
	throttle, reason := d.shouldThrottleSpawns()
	if !throttle {
		t.Error("expected throttle on high contention")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestThrottle_ModerateContention_DefaultLevel(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	ts := time.Now().Format(time.RFC3339)
	os.WriteFile(stateFile, []byte(`{"contention_level":1,"gaming_active":0,"load_1m":10.0,"memory_used_percent":55,"gpu_utilization":0,"claude_processes":20,"timestamp":"`+ts+`"}`), 0644)

	d := newTestDaemon(&ThrottleConfig{
		Enabled:   true,
		StateFile: stateFile,
	})
	// Default throttle_level is 2, so moderate (1) should NOT throttle
	throttle, _ := d.shouldThrottleSpawns()
	if throttle {
		t.Error("expected no throttle on moderate contention with default level=2")
	}
}

func TestThrottle_ModerateContention_LoweredLevel(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	ts := time.Now().Format(time.RFC3339)
	os.WriteFile(stateFile, []byte(`{"contention_level":1,"gaming_active":0,"load_1m":10.0,"memory_used_percent":55,"gpu_utilization":0,"claude_processes":20,"timestamp":"`+ts+`"}`), 0644)

	d := newTestDaemon(&ThrottleConfig{
		Enabled:       true,
		StateFile:     stateFile,
		ThrottleLevel: 1, // Throttle on moderate too
	})
	throttle, reason := d.shouldThrottleSpawns()
	if !throttle {
		t.Error("expected throttle on moderate contention with level=1")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestThrottle_StaleStateFile(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	// Timestamp 20 minutes ago
	ts := time.Now().Add(-20 * time.Minute).Format(time.RFC3339)
	os.WriteFile(stateFile, []byte(`{"contention_level":2,"gaming_active":1,"load_1m":20.0,"memory_used_percent":90,"gpu_utilization":80,"claude_processes":5,"timestamp":"`+ts+`"}`), 0644)

	d := newTestDaemon(&ThrottleConfig{
		Enabled:      true,
		StateFile:    stateFile,
		StaleSeconds: 600, // 10 minutes
	})
	throttle, _ := d.shouldThrottleSpawns()
	if throttle {
		t.Error("expected fail-open on stale state file")
	}
}

func TestThrottle_NoContention(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	ts := time.Now().Format(time.RFC3339)
	os.WriteFile(stateFile, []byte(`{"contention_level":0,"gaming_active":0,"load_1m":2.0,"memory_used_percent":35,"gpu_utilization":0,"claude_processes":8,"timestamp":"`+ts+`"}`), 0644)

	d := newTestDaemon(&ThrottleConfig{
		Enabled:   true,
		StateFile: stateFile,
	})
	throttle, _ := d.shouldThrottleSpawns()
	if throttle {
		t.Error("expected no throttle with zero contention")
	}
}
