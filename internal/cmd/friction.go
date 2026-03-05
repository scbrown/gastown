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
	frictionTool    string
	frictionTried   string
	frictionGot     string
	frictionWant    string
	frictionAgent   string
	frictionListAll bool
)

var frictionCmd = &cobra.Command{
	Use:     "friction",
	GroupID: GroupDiag,
	Short:   "Log tool/workflow friction points",
	Long: `Log and query friction points encountered with tools and workflows.

Submit a friction entry with prose or structured flags:

  # Prose (auto-parsed if it contains "tried"/"got"/"want")
  gt friction "tried gt worktree tapestry, got unknown command hook, want clean worktree creation"

  # Structured flags
  gt friction --tool "gt worktree" --tried "create tapestry worktree" --got "bd hook error" --want "clean creation"

  # List unresolved friction
  gt friction list
  gt friction list --all`,
	Args: cobra.MaximumNArgs(1),
	RunE: runFriction,
}

var frictionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List friction log entries",
	Long:  `List friction log entries from the HQ database. Shows unresolved by default.`,
	RunE:  runFrictionList,
}

func init() {
	rootCmd.AddCommand(frictionCmd)
	frictionCmd.AddCommand(frictionListCmd)

	frictionCmd.Flags().StringVar(&frictionTool, "tool", "", "Tool or command that caused friction")
	frictionCmd.Flags().StringVar(&frictionTried, "tried", "", "What you tried to do")
	frictionCmd.Flags().StringVar(&frictionGot, "got", "", "What happened instead")
	frictionCmd.Flags().StringVar(&frictionWant, "want", "", "What you expected/wanted")
	frictionCmd.Flags().StringVar(&frictionAgent, "agent", "", "Agent identity override (default: BD_ACTOR)")

	frictionListCmd.Flags().BoolVar(&frictionListAll, "all", false, "Show all entries including resolved")
}

// FrictionEntry represents a single friction log entry.
type FrictionEntry struct {
	ID        string      `json:"id"`
	Agent     string      `json:"agent"`
	Tool      string      `json:"tool,omitempty"`
	Tried     string      `json:"tried,omitempty"`
	Got       string      `json:"got,omitempty"`
	Want      string      `json:"want,omitempty"`
	Resolved  interface{} `json:"resolved"` // Dolt returns "0"/"1" not bool
	Timestamp string      `json:"timestamp"`
}

func (e FrictionEntry) IsResolved() bool {
	switch v := e.Resolved.(type) {
	case bool:
		return v
	case string:
		return v == "1" || v == "true"
	case float64:
		return v != 0
	}
	return false
}

func runFriction(cmd *cobra.Command, args []string) error {
	// If no args and no flags, show help
	if len(args) == 0 && frictionTried == "" && frictionGot == "" && frictionWant == "" {
		return cmd.Help()
	}

	agent := frictionAgent
	if agent == "" {
		agent = os.Getenv("BD_ACTOR")
	}
	if agent == "" {
		return fmt.Errorf("agent identity not set (set BD_ACTOR or use --agent)")
	}

	// Parse prose arg if provided
	if len(args) == 1 && frictionTried == "" {
		parseFrictionProse(args[0])
	}

	id, err := generateFrictionID()
	if err != nil {
		return fmt.Errorf("generating ID: %w", err)
	}

	now := time.Now().UTC()

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)

	toolSQL := sqlNullOrEscape(frictionTool)
	triedSQL := sqlNullOrEscape(frictionTried)
	gotSQL := sqlNullOrEscape(frictionGot)
	wantSQL := sqlNullOrEscape(frictionWant)

	query := fmt.Sprintf(
		"USE beads_hq; INSERT INTO friction_log (id, agent, timestamp, tool, tried, got, want) VALUES (%s, %s, %s, %s, %s, %s, %s);",
		escapeSQL(id),
		escapeSQL(agent),
		escapeSQL(now.Format("2006-01-02 15:04:05")),
		toolSQL,
		triedSQL,
		gotSQL,
		wantSQL,
	)

	commitQuery := fmt.Sprintf(
		"USE beads_hq; CALL dolt_add('friction_log'); CALL dolt_commit('-m', 'friction: %s from %s');",
		escapeSQLValue(frictionTool), escapeSQLValue(agent),
	)

	sqlCmd := exec.Command("dolt",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(config.Port),
		"--user", config.User,
		"--password", "",
		"--no-tls",
		"sql", "-q", query,
	)
	if output, err := sqlCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("inserting friction entry: %w\n%s", err, string(output))
	}

	commitCmd := exec.Command("dolt",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(config.Port),
		"--user", config.User,
		"--password", "",
		"--no-tls",
		"sql", "-q", commitQuery,
	)
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("committing friction entry: %w\n%s", err, string(output))
	}

	fmt.Printf("%s Friction logged: %s\n", style.Success.Render("✓"), id)
	if frictionTool != "" {
		fmt.Printf("  Tool: %s\n", frictionTool)
	}
	if frictionTried != "" {
		fmt.Printf("  Tried: %s\n", frictionTried)
	}
	if frictionGot != "" {
		fmt.Printf("  Got: %s\n", frictionGot)
	}
	if frictionWant != "" {
		fmt.Printf("  Want: %s\n", frictionWant)
	}

	return nil
}

func runFrictionList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)

	query := "USE beads_hq; SELECT id, agent, timestamp, tool, tried, got, want, resolved FROM friction_log"
	if !frictionListAll {
		query += " WHERE resolved = FALSE"
	}
	query += " ORDER BY timestamp DESC LIMIT 50;"

	sqlCmd := exec.Command("dolt",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(config.Port),
		"--user", config.User,
		"--password", "",
		"--no-tls",
		"sql", "-r", "json", "-q", query,
	)
	output, err := sqlCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("querying friction log: %w\n%s", err, string(output))
	}

	var result struct {
		Rows []FrictionEntry `json:"rows"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		// Fallback: just print raw output
		fmt.Print(string(output))
		return nil
	}

	if len(result.Rows) == 0 {
		fmt.Println(style.Dim.Render("No friction entries found."))
		return nil
	}

	for _, entry := range result.Rows {
		status := style.Dim.Render("open")
		if entry.IsResolved() {
			status = style.Success.Render("resolved")
		}
		fmt.Printf("%s [%s] %s — %s\n", entry.ID, status, entry.Agent, entry.Tool)
		if entry.Tried != "" {
			fmt.Printf("  Tried: %s\n", entry.Tried)
		}
		if entry.Got != "" {
			fmt.Printf("  Got:   %s\n", entry.Got)
		}
		if entry.Want != "" {
			fmt.Printf("  Want:  %s\n", entry.Want)
		}
		fmt.Println()
	}

	return nil
}

// parseFrictionProse tries to extract tried/got/want from prose like:
// "tried X, got Y, want Z"
func parseFrictionProse(prose string) {
	lower := strings.ToLower(prose)

	// Try structured parsing
	triedIdx := strings.Index(lower, "tried ")
	gotIdx := strings.Index(lower, "got ")
	wantIdx := strings.Index(lower, "want ")

	if triedIdx >= 0 && gotIdx > triedIdx {
		// Structured: extract segments
		triedEnd := gotIdx
		if gotIdx > 0 {
			triedEnd = gotIdx
		}
		frictionTried = strings.TrimRight(strings.TrimSpace(prose[triedIdx+6:triedEnd]), ",;")

		if wantIdx > gotIdx {
			frictionGot = strings.TrimRight(strings.TrimSpace(prose[gotIdx+4:wantIdx]), ",;")
			frictionWant = strings.TrimRight(strings.TrimSpace(prose[wantIdx+5:]), ",;.")
		} else {
			frictionGot = strings.TrimRight(strings.TrimSpace(prose[gotIdx+4:]), ",;.")
		}
	} else {
		// Unstructured: put entire prose in "tried"
		frictionTried = prose
	}
}

func generateFrictionID() (string, error) {
	b := make([]byte, 13)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "fr-" + hex.EncodeToString(b)[:10], nil
}

func sqlNullOrEscape(s string) string {
	if s == "" {
		return "NULL"
	}
	return escapeSQL(s)
}

// escapeSQLValue is like escapeSQL but returns the raw value (no quotes) for use in commit messages.
func escapeSQLValue(s string) string {
	return strings.ReplaceAll(s, "'", "")
}
