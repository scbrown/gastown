package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// SessionSettingsCheck flags live crew Claude sessions running WITHOUT the
// --settings flag — i.e. without any of their hooks, including the
// rm -rf / git-push-force tap guards, priming, mail injection, and the
// PreCompact self-cycle (aegis-05up / gt-qiz2s3u).
//
// WHY A PROCESS SCAN: --settings binds at process start and registered hooks
// cannot be acquired later by any in-session means, so a guardless session is
// invisible from the inside and looks healthy from the outside. The only
// authority is the process table. Degradation is monotonic — a session that
// lost its hooks never regains them — so one flagged process is a standing
// exposure, not a transient.
type SessionSettingsCheck struct {
	BaseCheck
}

// NewSessionSettingsCheck creates the crew-session --settings audit.
func NewSessionSettingsCheck() *SessionSettingsCheck {
	return &SessionSettingsCheck{
		BaseCheck: BaseCheck{
			CheckName:        "session-settings",
			CheckDescription: "Check that every live crew Claude session carries --settings (hooks + tap guards)",
			CheckCategory:    CategoryHooks,
		},
	}
}

// classifySessionCmdline decides whether a process command line is a Gas Town
// crew Claude session, and if so whether it carries --settings. Pure so it is
// unit-testable without a process table.
//
// A crew session is recognized by the Gas Town beacon in its prompt argument
// ("[GAS TOWN] crew ..."), which every launch path embeds — bare interactive
// `claude` runs are deliberately NOT flagged (they are not crew sessions and
// carry no expectation of crew hooks).
// argv0 must BE the claude binary (basename match), not merely appear as a
// substring anywhere in the command line: a shell that happens to CARRY these
// strings as arguments (a grep, a test harness, a script quoting the beacon)
// is not a session — the first live run of this check flagged its own test
// shell and escalated a phantom before this tightening.
func classifySessionCmdline(argv []string) (isCrew bool, hasSettings bool) {
	if len(argv) == 0 {
		return false, false
	}
	base := argv[0]
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if base != "claude" {
		return false, false
	}
	joined := strings.Join(argv[1:], " ")
	if !strings.Contains(joined, "[GAS TOWN] crew") {
		return false, false
	}
	// The flag is its own argv element on every real launch path; substring
	// over the remaining args is safe because argv[0] is already pinned.
	return true, strings.Contains(joined, "--settings")
}

// Run scans the process table for crew Claude sessions missing --settings.
func (c *SessionSettingsCheck) Run(ctx *CheckContext) *CheckResult {
	if runtime.GOOS != "linux" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "process scan requires /proc (linux only) — skipped",
		}
	}

	procs, err := filepath.Glob("/proc/[0-9]*/cmdline")
	if err != nil || procs == nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "could not enumerate /proc — crew sessions unaudited",
		}
	}

	var flagged []string
	crewCount := 0
	for _, path := range procs {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue // process exited mid-scan, or not ours to read
		}
		argv := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		isCrew, hasSettings := classifySessionCmdline(argv)
		cmdline := strings.Join(argv, " ")
		if !isCrew {
			continue
		}
		crewCount++
		if !hasSettings {
			pid := filepath.Base(filepath.Dir(path))
			// The beacon names the crew member; carry what we can see.
			name := "?"
			if i := strings.Index(cmdline, "[GAS TOWN] crew "); i >= 0 {
				rest := cmdline[i+len("[GAS TOWN] crew "):]
				name = strings.Fields(rest)[0]
			}
			flagged = append(flagged, fmt.Sprintf("pid %s (%s)", pid, name))
		}
	}

	if len(flagged) > 0 {
		return &CheckResult{
			Name:   c.Name(),
			Status: StatusError,
			Message: fmt.Sprintf("%d of %d crew Claude session(s) running WITHOUT --settings — no hooks, no rm-rf/push-force guards",
				len(flagged), crewCount),
			Details: append([]string{
				"These sessions have NO PreToolUse tap guards, no priming, no mail",
				"injection, and no PreCompact self-cycle. They cannot recover in-session:",
				"--settings binds at process start (aegis-05up / gt-qiz2s3u).",
			}, flagged...),
			FixHint: "Kill the flagged session(s); the crew watchdog relaunches them through the guarded launcher path within ~3 minutes. `gt handoff` cannot produce a guarded session until the respawn fix is installed.",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("all %d crew Claude session(s) carry --settings", crewCount),
	}
}
