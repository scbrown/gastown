package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	patrolSubmitDomain   string
	patrolSubmitFindings string
	patrolSubmitMetrics  string
	patrolSubmitJSON     string
	patrolSubmitAgent    string
)

var patrolSubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit a structured patrol report",
	Long: `Submit a structured patrol report to the aegis patrol_reports table.

Reports can be submitted as JSON or via flags:

  # Full JSON report
  gt patrol submit --json '{"domain":"observability","findings":[...],"metrics":{...}}'

  # Flag-based
  gt patrol submit --domain observability \
    --findings '[{"severity":"ok","service":"prometheus","detail":"All targets up"}]' \
    --metrics '{"alerts_firing":3,"alerts_fixed":1}'

  # Pipe JSON from stdin
  echo '{"domain":"security",...}' | gt patrol submit --json -

Agent identity is auto-detected from BD_ACTOR.`,
	RunE: runPatrolSubmit,
}

func init() {
	patrolCmd.AddCommand(patrolSubmitCmd)

	patrolSubmitCmd.Flags().StringVar(&patrolSubmitDomain, "domain", "", "Patrol domain (e.g. observability, security)")
	patrolSubmitCmd.Flags().StringVar(&patrolSubmitFindings, "findings", "", "Findings as JSON array")
	patrolSubmitCmd.Flags().StringVar(&patrolSubmitMetrics, "metrics", "", "Metrics as JSON object")
	patrolSubmitCmd.Flags().StringVar(&patrolSubmitJSON, "json", "", "Full report as JSON (or - for stdin)")
	patrolSubmitCmd.Flags().StringVar(&patrolSubmitAgent, "agent", "", "Agent identity override (default: BD_ACTOR)")
}

// PatrolReport is the structured report submitted by agents.
type PatrolReport struct {
	Agent    string          `json:"agent"`
	Domain   string          `json:"domain,omitempty"`
	Findings json.RawMessage `json:"findings,omitempty"`
	Metrics  json.RawMessage `json:"metrics,omitempty"`
}

func runPatrolSubmit(cmd *cobra.Command, args []string) error {
	// Resolve agent identity
	agent := patrolSubmitAgent
	if agent == "" {
		agent = os.Getenv("BD_ACTOR")
	}
	if agent == "" {
		return fmt.Errorf("agent identity not set (set BD_ACTOR or use --agent)")
	}

	// Build report from JSON or flags
	var report PatrolReport
	report.Agent = agent

	if patrolSubmitJSON != "" {
		var jsonData []byte
		if patrolSubmitJSON == "-" {
			var err error
			jsonData, err = os.ReadFile("/dev/stdin")
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
		} else {
			jsonData = []byte(patrolSubmitJSON)
		}
		if err := json.Unmarshal(jsonData, &report); err != nil {
			return fmt.Errorf("parsing JSON report: %w", err)
		}
		// Preserve agent from flag/env if JSON didn't set it
		if report.Agent == "" {
			report.Agent = agent
		}
	} else {
		report.Domain = patrolSubmitDomain
		if patrolSubmitFindings != "" {
			if !json.Valid([]byte(patrolSubmitFindings)) {
				return fmt.Errorf("--findings is not valid JSON")
			}
			report.Findings = json.RawMessage(patrolSubmitFindings)
		}
		if patrolSubmitMetrics != "" {
			if !json.Valid([]byte(patrolSubmitMetrics)) {
				return fmt.Errorf("--metrics is not valid JSON")
			}
			report.Metrics = json.RawMessage(patrolSubmitMetrics)
		}
	}

	// Generate ID (random hex, 13 chars to fit VARCHAR(26))
	id, err := generatePatrolID()
	if err != nil {
		return fmt.Errorf("generating ID: %w", err)
	}

	now := time.Now().UTC()

	// Build SQL INSERT
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)

	findingsSQL := "NULL"
	if len(report.Findings) > 0 {
		findingsSQL = escapeSQL(string(report.Findings))
	}
	metricsSQL := "NULL"
	if len(report.Metrics) > 0 {
		metricsSQL = escapeSQL(string(report.Metrics))
	}
	domainSQL := "NULL"
	if report.Domain != "" {
		domainSQL = escapeSQL(report.Domain)
	}

	query := fmt.Sprintf(
		"USE aegis; INSERT INTO patrol_reports (id, agent, timestamp, domain, findings, metrics) VALUES (%s, %s, %s, %s, %s, %s);",
		escapeSQL(id),
		escapeSQL(report.Agent),
		escapeSQL(now.Format("2006-01-02 15:04:05")),
		domainSQL,
		findingsSQL,
		metricsSQL,
	)

	commitQuery := fmt.Sprintf(
		"USE aegis; CALL dolt_add('patrol_reports'); CALL dolt_commit('-m', 'patrol: %s report from %s');",
		report.Domain, report.Agent,
	)

	// Execute INSERT
	sqlCmd := exec.Command("dolt",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(config.Port),
		"--user", config.User,
		"--password", "",
		"--no-tls",
		"sql", "-q", query,
	)
	if output, err := sqlCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("inserting patrol report: %w\n%s", err, string(output))
	}

	// Commit
	commitCmd := exec.Command("dolt",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(config.Port),
		"--user", config.User,
		"--password", "",
		"--no-tls",
		"sql", "-q", commitQuery,
	)
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("committing patrol report: %w\n%s", err, string(output))
	}

	fmt.Printf("%s Patrol report submitted: %s (%s/%s)\n",
		style.Success.Render("✓"), id, report.Agent, report.Domain)

	return nil
}

func generatePatrolID() (string, error) {
	b := make([]byte, 13)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "pr-" + hex.EncodeToString(b)[:10], nil
}

func escapeSQL(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	return "'" + s + "'"
}
