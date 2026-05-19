package beads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var bdTargetEnvKeys = []string{
	"BEADS_DIR",
	"BEADS_DB",
	"BD_DB",
	"BEADS_DOLT_DATA_DIR",
	"BEADS_DOLT_HOST",
	"BEADS_DOLT_PORT",
	"BEADS_DOLT_SERVER_DATABASE",
	"BEADS_DOLT_SERVER_HOST",
	"BEADS_DOLT_SERVER_PORT",
	"BEADS_DOLT_SERVER_SOCKET",
	"BEADS_DOLT_SERVER_MODE",
	"BEADS_DOLT_SHARED_SERVER",
	"BEADS_SHARED_SERVER_DIR",
}

// DatabaseNameFromMetadata reads the dolt_database field from .beads/metadata.json.
// Returns empty string if metadata doesn't exist or has no database configured.
func DatabaseNameFromMetadata(beadsDir string) string {
	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		return ""
	}
	var meta struct {
		DoltDatabase string `json:"dolt_database"`
	}
	if json.Unmarshal(data, &meta) != nil {
		return ""
	}
	return meta.DoltDatabase
}

// DatabaseEnv returns the BEADS_DOLT_SERVER_DATABASE=<name> env var string
// for the given beadsDir, or empty string if no database is configured.
func DatabaseEnv(beadsDir string) string {
	db := DatabaseNameFromMetadata(beadsDir)
	if db == "" {
		return ""
	}
	return "BEADS_DOLT_SERVER_DATABASE=" + db
}

// StripBDTargetEnv removes inherited environment variables that can make a bd
// subprocess select a database/server other than the .beads directory chosen by
// Gas Town. It intentionally preserves BEADS_DOLT_AUTO_START so callers can keep
// the shared-server guardrail enabled.
func StripBDTargetEnv(env []string) []string {
	filtered := env
	for _, key := range bdTargetEnvKeys {
		filtered = stripEnvKey(filtered, key)
	}
	return filtered
}

// BuildPinnedBDEnv returns env for a bd subprocess pinned to beadsDir. The
// selected .beads metadata is authoritative over inherited BEADS_DOLT_* values.
func BuildPinnedBDEnv(base []string, beadsDir string) []string {
	env := SuppressBDSideEffects(StripBDTargetEnv(base))
	if beadsDir == "" {
		return addGTDerivedDoltTargetEnv(env)
	}
	env = append(env, "BEADS_DIR="+beadsDir)
	env = append(env, doltTargetEnvFromBeadsDir(beadsDir, true)...)
	return addGTDerivedDoltTargetEnv(env)
}

// BuildRoutingBDEnv returns env for a bd subprocess that intentionally relies on
// bd prefix routing. It strips stale target/database selectors, then re-adds only
// connection host/port from fallbackBeadsDir so routing can choose the database.
func BuildRoutingBDEnv(base []string, fallbackBeadsDir string) []string {
	env := SuppressBDSideEffects(StripBDTargetEnv(base))
	env = append(env, doltTargetEnvFromBeadsDir(fallbackBeadsDir, false)...)
	return addGTDerivedDoltTargetEnv(env)
}

// SuppressBDSideEffects disables Beads JSONL export/backup/push side effects for
// Gas Town-managed subprocesses. The authoritative data plane is Dolt; exporting
// JSONL from high-frequency gt callers re-invalidates Beads' import freshness
// checks and can create a self-feeding Dolt load loop.
func SuppressBDSideEffects(env []string) []string {
	for _, key := range []string{
		"BD_EXPORT_AUTO",
		"BD_BACKUP_ENABLED",
		"BD_DOLT_AUTO_PUSH",
		"BD_NO_PUSH",
		"BD_EXPORT_GIT_ADD",
		"BD_NO_GIT_OPS",
	} {
		env = stripEnvKey(env, key)
	}
	return append(env,
		"BD_EXPORT_AUTO=false",
		"BD_BACKUP_ENABLED=false",
		"BD_DOLT_AUTO_PUSH=false",
		"BD_NO_PUSH=true",
		"BD_EXPORT_GIT_ADD=false",
		"BD_NO_GIT_OPS=true",
	)
}

func doltTargetEnvFromBeadsDir(beadsDir string, includeDatabase bool) []string {
	if beadsDir == "" {
		return nil
	}
	meta := readDoltMetadata(beadsDir)
	var env []string
	if includeDatabase && meta.Database != "" {
		env = append(env, "BEADS_DOLT_SERVER_DATABASE="+meta.Database)
	}
	if meta.Host != "" {
		env = append(env, "BEADS_DOLT_SERVER_HOST="+meta.Host)
	}
	if meta.Port != "" {
		env = append(env, "BEADS_DOLT_SERVER_PORT="+meta.Port)
		env = append(env, "BEADS_DOLT_PORT="+meta.Port)
	}
	return env
}

type doltMetadata struct {
	Database string
	Host     string
	Port     string
}

func readDoltMetadata(beadsDir string) doltMetadata {
	var meta doltMetadata
	if data, err := os.ReadFile(filepath.Join(beadsDir, "dolt-server.port")); err == nil {
		meta.Port = strings.TrimSpace(string(data))
	}
	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		return meta
	}
	var raw struct {
		DoltDatabase   string `json:"dolt_database"`
		DoltServerHost string `json:"dolt_server_host"`
		DoltServerPort int    `json:"dolt_server_port"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return meta
	}
	meta.Database = strings.TrimSpace(raw.DoltDatabase)
	meta.Host = strings.TrimSpace(raw.DoltServerHost)
	if meta.Port == "" && raw.DoltServerPort > 0 {
		meta.Port = strconv.Itoa(raw.DoltServerPort)
	}
	return meta
}

func stripEnvKey(env []string, key string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func addGTDerivedDoltTargetEnv(env []string) []string {
	gtHost := envValue(env, "GT_DOLT_HOST")
	gtPort := envValue(env, "GT_DOLT_PORT")
	if gtHost != "" && envValue(env, "BEADS_DOLT_SERVER_HOST") == "" {
		env = append(env, "BEADS_DOLT_SERVER_HOST="+gtHost)
	}
	if gtPort != "" {
		if envValue(env, "BEADS_DOLT_SERVER_PORT") == "" {
			env = append(env, "BEADS_DOLT_SERVER_PORT="+gtPort)
		}
		if envValue(env, "BEADS_DOLT_PORT") == "" {
			env = append(env, "BEADS_DOLT_PORT="+gtPort)
		}
	}
	return env
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}
