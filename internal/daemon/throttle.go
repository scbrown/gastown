package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// contentionState is the JSON structure written by resource_contention.sh.
type contentionState struct {
	ContentionLevel   int     `json:"contention_level"`
	GamingActive      int     `json:"gaming_active"`
	Load1m            float64 `json:"load_1m"`
	MemoryUsedPercent int     `json:"memory_used_percent"`
	GPUUtilization    int     `json:"gpu_utilization"`
	ClaudeProcesses   int     `json:"claude_processes"`
	Timestamp         string  `json:"timestamp"`
}

// defaultStateFile returns the default contention state file path.
func defaultStateFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "aegis", "resource_contention.json")
}

// shouldThrottleSpawns checks whether polecat spawns should be deferred
// due to resource contention on the host.
//
// Returns (shouldThrottle, reason). Fails open: if config is missing,
// disabled, or state file is stale/unreadable, spawns proceed normally.
func (d *Daemon) shouldThrottleSpawns() (bool, string) {
	cfg := d.throttleConfig()
	if cfg == nil || !cfg.Enabled {
		return false, ""
	}

	stateFile := cfg.StateFile
	if stateFile == "" {
		stateFile = defaultStateFile()
	}
	if stateFile == "" {
		return false, ""
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		// File missing or unreadable — fail open
		return false, ""
	}

	var state contentionState
	if err := json.Unmarshal(data, &state); err != nil {
		d.logger.Printf("WARN: throttle: failed to parse %s: %v", stateFile, err)
		return false, ""
	}

	// Check staleness
	staleSeconds := cfg.StaleSeconds
	if staleSeconds <= 0 {
		staleSeconds = 600 // 10 minutes
	}
	ts, err := time.Parse(time.RFC3339, state.Timestamp)
	if err != nil {
		// Try alternative format from date -Iseconds
		ts, err = time.Parse("2006-01-02T15:04:05-07:00", state.Timestamp)
	}
	if err == nil && time.Since(ts) > time.Duration(staleSeconds)*time.Second {
		d.logger.Printf("WARN: throttle: state file is stale (%s), proceeding with spawns", time.Since(ts).Round(time.Second))
		return false, ""
	}

	throttleLevel := cfg.ThrottleLevel
	if throttleLevel <= 0 {
		throttleLevel = 2 // Default: only throttle on high contention
	}

	if state.ContentionLevel >= throttleLevel {
		label := "high"
		if state.ContentionLevel == 1 {
			label = "moderate"
		}
		reason := fmt.Sprintf("%s contention (load=%.1f, mem=%d%%, gpu=%d%%, claude=%d, gaming=%d)",
			label, state.Load1m, state.MemoryUsedPercent, state.GPUUtilization,
			state.ClaudeProcesses, state.GamingActive)
		return true, reason
	}

	return false, ""
}

// throttleConfig returns the throttle config, or nil if not configured.
func (d *Daemon) throttleConfig() *ThrottleConfig {
	if d.patrolConfig == nil {
		return nil
	}
	return d.patrolConfig.Throttle
}
