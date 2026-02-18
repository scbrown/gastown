package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/pane"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/townlog"
	"github.com/steveyegge/gastown/internal/workspace"
)

var paneCmd = &cobra.Command{
	Use:     "pane",
	GroupID: GroupWorkspace,
	Short:   "Agent-driven terminal pane control",
	Long: `Manage tmux panes in shanty sessions.

Agents can create, list, and close panes in their tmux sessions. All pane
operations are tracked, rate-limited, and logged.

Commands:
  open     Split a new pane and run a command
  exec     Run a command in a new pane (auto-closes on exit)
  show     Open a read-only pane showing another agent's session
  close    Close an agent-created pane
  list     List agent-created panes in a session
  layout   Create a multi-pane layout

Limits:
  - Max 6 agent-created panes per session
  - Rate limit: 3 pane operations per minute per agent
  - Only agents (crew/polecat/witness/refinery) can use pane commands

Examples:
  gt pane open my-session "htop"
  gt pane exec my-session "go test ./..."
  gt pane show my-session gt-gastown-witness
  gt pane list my-session
  gt pane close my-session %5
  gt pane layout my-session --right "htop" --bottom "tail -f app.log"`,
	RunE: requireSubcommand,
}

var paneOpenCmd = &cobra.Command{
	Use:   "open <session> <command>",
	Short: "Split a new pane and run a command",
	Long: `Split a new pane in the target session and run a command.

The pane persists after the command exits (remain-on-exit). Use 'gt pane close'
to remove it. Smart split direction is chosen based on terminal geometry.

Examples:
  gt pane open my-session "htop"
  gt pane open my-session "tail -f /var/log/syslog"
  gt pane open my-session --vertical "watch df -h"`,
	Args: cobra.ExactArgs(2),
	RunE: runPaneOpen,
}

var paneExecCmd = &cobra.Command{
	Use:   "exec <session> <command>",
	Short: "Run a command in a new pane (auto-closes on exit)",
	Long: `Run a command in a new pane that automatically closes when done.

Unlike 'open', the pane is destroyed when the command exits. This is useful
for one-shot commands like test runs or builds.

Examples:
  gt pane exec my-session "go test ./..."
  gt pane exec my-session "make build"`,
	Args: cobra.ExactArgs(2),
	RunE: runPaneExec,
}

var paneShowCmd = &cobra.Command{
	Use:   "show <session> <agent-session>",
	Short: "Open a read-only pane showing another agent's session",
	Long: `Open a new pane that displays another agent's tmux session in read-only mode.

This uses tmux capture-pane to provide a live view into the target agent's
work without interfering.

Examples:
  gt pane show my-session gt-gastown-witness
  gt pane show my-session gt-gastown-furiosa`,
	Args: cobra.ExactArgs(2),
	RunE: runPaneShow,
}

var paneCloseCmd = &cobra.Command{
	Use:   "close <session> <pane-id>",
	Short: "Close an agent-created pane",
	Long: `Close a specific agent-created pane by its tmux pane ID.

Only panes tracked by the pane manager can be closed. Use 'gt pane list'
to see available pane IDs.

Examples:
  gt pane close my-session %5
  gt pane close my-session %12`,
	Args: cobra.ExactArgs(2),
	RunE: runPaneClose,
}

var paneListCmd = &cobra.Command{
	Use:   "list <session>",
	Short: "List agent-created panes in a session",
	Long: `List all panes created by agents in the target session.

Shows pane ID, command, creator, and creation time.

Examples:
  gt pane list my-session`,
	Args: cobra.ExactArgs(1),
	RunE: runPaneList,
}

var paneLayoutCmd = &cobra.Command{
	Use:   "layout <session>",
	Short: "Create a multi-pane layout",
	Long: `Create a predefined multi-pane layout in the target session.

Layouts:
  --main          Apply main-vertical layout after creating panes
  --right <cmd>   Add a right-side pane running <cmd>
  --bottom <cmd>  Add a bottom pane running <cmd>

Examples:
  gt pane layout my-session --right "htop" --bottom "tail -f app.log"
  gt pane layout my-session --main --right "watch df -h"`,
	Args: cobra.ExactArgs(1),
	RunE: runPaneLayout,
}

// Flags
var (
	paneVertical bool
	paneJSON     bool
	layoutRight  string
	layoutBottom string
	layoutMain   bool
)

func init() {
	paneOpenCmd.Flags().BoolVarP(&paneVertical, "vertical", "v", false, "Force vertical split")

	paneLayoutCmd.Flags().StringVar(&layoutRight, "right", "", "Command for right pane")
	paneLayoutCmd.Flags().StringVar(&layoutBottom, "bottom", "", "Command for bottom pane")
	paneLayoutCmd.Flags().BoolVar(&layoutMain, "main", false, "Apply main-vertical layout")

	paneListCmd.Flags().BoolVar(&paneJSON, "json", false, "Output as JSON")

	paneCmd.AddCommand(paneOpenCmd)
	paneCmd.AddCommand(paneExecCmd)
	paneCmd.AddCommand(paneShowCmd)
	paneCmd.AddCommand(paneCloseCmd)
	paneCmd.AddCommand(paneListCmd)
	paneCmd.AddCommand(paneLayoutCmd)

	rootCmd.AddCommand(paneCmd)
}

// checkPaneAccess verifies the caller is an agent role that may use pane commands.
func checkPaneAccess() (RoleInfo, error) {
	info, err := GetRole()
	if err != nil {
		return info, fmt.Errorf("detecting role: %w", err)
	}

	switch info.Role {
	case RoleCrew, RolePolecat, RoleWitness, RoleRefinery, RoleMayor, RoleDeacon:
		return info, nil
	default:
		return info, fmt.Errorf("pane commands require an agent role (got %s)", info.Role)
	}
}

// panePrecheck performs common pre-checks: role check, rate limit, session validation.
func panePrecheck(session string) (*pane.Tracker, *pane.SessionState, RoleInfo, error) {
	role, err := checkPaneAccess()
	if err != nil {
		return nil, nil, role, err
	}

	tracker := pane.NewTracker()
	state, err := tracker.Load(session)
	if err != nil {
		return nil, nil, role, fmt.Errorf("loading pane state: %w", err)
	}

	if err := state.CheckRateLimit(); err != nil {
		return nil, nil, role, err
	}

	return tracker, state, role, nil
}

// tmuxRun executes a tmux command and returns stdout.
func tmuxRun(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("tmux %s: %s", args[0], strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

// tmuxHasSession checks if a tmux session exists.
func tmuxHasSession(name string) bool {
	err := exec.Command("tmux", "has-session", "-t", "="+name).Run()
	return err == nil
}

// tmuxSetRemainOnExit controls whether a pane stays after its process exits.
func tmuxSetRemainOnExit(target string, on bool) error {
	value := "on"
	if !on {
		value = "off"
	}
	_, err := tmuxRun("set-option", "-t", target, "remain-on-exit", value)
	return err
}

// logPaneOp logs a pane operation to the town log.
func logPaneOp(op, session, detail string) {
	townRoot, _ := workspace.FindFromCwd()
	if townRoot == "" {
		return
	}
	logger := townlog.NewLogger(townRoot)
	role, _ := GetRole()
	logger.Log(townlog.EventType("pane_"+op), role.ActorString(), fmt.Sprintf("session=%s %s", session, detail))
}

// getNewestPane returns the pane ID of the most recently created pane in a session.
func getNewestPane(session string) (string, error) {
	out, err := tmuxRun("list-panes", "-t", session, "-F", "#{pane_id}")
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("no panes in session %s", session)
	}
	return lines[len(lines)-1], nil
}

func runPaneOpen(cmd *cobra.Command, args []string) error {
	session := args[0]
	command := args[1]

	tracker, state, role, err := panePrecheck(session)
	if err != nil {
		return err
	}

	if err := state.CheckPaneLimit(); err != nil {
		return err
	}

	if !tmuxHasSession(session) {
		return fmt.Errorf("session %q not found", session)
	}

	// Build split-window args: -d keeps focus on current pane
	splitArgs := []string{"split-window", "-t", session, "-d"}
	if paneVertical {
		splitArgs = append(splitArgs, "-h") // -h = horizontal split = vertical panes
	}
	splitArgs = append(splitArgs, command)

	if _, err := tmuxRun(splitArgs...); err != nil {
		return fmt.Errorf("split-window: %w", err)
	}

	paneID, err := getNewestPane(session)
	if err != nil {
		return fmt.Errorf("getting new pane ID: %w", err)
	}

	// Set remain-on-exit so pane persists after command exits
	if err := tmuxSetRemainOnExit(paneID, true); err != nil {
		fmt.Fprintf(os.Stderr, "%s could not set remain-on-exit: %v\n", style.WarningPrefix, err)
	}

	state.AddPane(&pane.PaneInfo{
		PaneID:    paneID,
		SessionID: session,
		Command:   command,
		CreatedBy: role.ActorString(),
		CreatedAt: time.Now(),
		Type:      "open",
	})
	state.RecordOperation()
	if err := tracker.Save(state); err != nil {
		fmt.Fprintf(os.Stderr, "%s saving pane state: %v\n", style.WarningPrefix, err)
	}

	logPaneOp("open", session, fmt.Sprintf("pane=%s cmd=%q", paneID, command))
	fmt.Printf("%s Opened pane %s in %s\n", style.SuccessPrefix, style.Bold.Render(paneID), session)
	return nil
}

func runPaneExec(cmd *cobra.Command, args []string) error {
	session := args[0]
	command := args[1]

	tracker, state, role, err := panePrecheck(session)
	if err != nil {
		return err
	}

	if err := state.CheckPaneLimit(); err != nil {
		return err
	}

	if !tmuxHasSession(session) {
		return fmt.Errorf("session %q not found", session)
	}

	// Split pane without remain-on-exit - auto-closes when command exits
	splitArgs := []string{"split-window", "-t", session, "-d", command}
	if _, err := tmuxRun(splitArgs...); err != nil {
		return fmt.Errorf("split-window: %w", err)
	}

	paneID, err := getNewestPane(session)
	if err != nil {
		return fmt.Errorf("getting new pane ID: %w", err)
	}

	state.AddPane(&pane.PaneInfo{
		PaneID:    paneID,
		SessionID: session,
		Command:   command,
		CreatedBy: role.ActorString(),
		CreatedAt: time.Now(),
		Type:      "exec",
	})
	state.RecordOperation()
	if err := tracker.Save(state); err != nil {
		fmt.Fprintf(os.Stderr, "%s saving pane state: %v\n", style.WarningPrefix, err)
	}

	logPaneOp("exec", session, fmt.Sprintf("pane=%s cmd=%q", paneID, command))
	fmt.Printf("%s Exec pane %s in %s (auto-closes on exit)\n",
		style.SuccessPrefix, style.Bold.Render(paneID), session)
	return nil
}

func runPaneShow(cmd *cobra.Command, args []string) error {
	session := args[0]
	agentSession := args[1]

	tracker, state, role, err := panePrecheck(session)
	if err != nil {
		return err
	}

	if err := state.CheckPaneLimit(); err != nil {
		return err
	}

	if !tmuxHasSession(session) {
		return fmt.Errorf("session %q not found", session)
	}
	if !tmuxHasSession(agentSession) {
		return fmt.Errorf("agent session %q not found", agentSession)
	}

	// Open a pane with a live read-only view of the agent's session
	viewCmd := fmt.Sprintf("watch -t -n 1 'tmux capture-pane -t %s -p -S -50'", agentSession)
	splitArgs := []string{"split-window", "-t", session, "-d", "-h", viewCmd}
	if _, err := tmuxRun(splitArgs...); err != nil {
		return fmt.Errorf("split-window: %w", err)
	}

	paneID, err := getNewestPane(session)
	if err != nil {
		return fmt.Errorf("getting new pane ID: %w", err)
	}

	if err := tmuxSetRemainOnExit(paneID, true); err != nil {
		fmt.Fprintf(os.Stderr, "%s could not set remain-on-exit: %v\n", style.WarningPrefix, err)
	}

	state.AddPane(&pane.PaneInfo{
		PaneID:    paneID,
		SessionID: session,
		Command:   viewCmd,
		CreatedBy: role.ActorString(),
		CreatedAt: time.Now(),
		Type:      "show",
	})
	state.RecordOperation()
	if err := tracker.Save(state); err != nil {
		fmt.Fprintf(os.Stderr, "%s saving pane state: %v\n", style.WarningPrefix, err)
	}

	logPaneOp("show", session, fmt.Sprintf("pane=%s agent=%s", paneID, agentSession))
	fmt.Printf("%s Showing %s in pane %s (read-only view)\n",
		style.SuccessPrefix, agentSession, style.Bold.Render(paneID))
	return nil
}

func runPaneClose(cmd *cobra.Command, args []string) error {
	session := args[0]
	paneID := args[1]

	tracker, state, _, err := panePrecheck(session)
	if err != nil {
		return err
	}

	if !state.RemovePane(paneID) {
		return fmt.Errorf("pane %s is not tracked in session %s (use 'gt pane list' to see tracked panes)", paneID, session)
	}

	if _, err := tmuxRun("kill-pane", "-t", paneID); err != nil {
		// Pane may already be gone — still save state
		fmt.Fprintf(os.Stderr, "%s pane may already be closed: %v\n", style.WarningPrefix, err)
	}

	state.RecordOperation()
	if err := tracker.Save(state); err != nil {
		fmt.Fprintf(os.Stderr, "%s saving pane state: %v\n", style.WarningPrefix, err)
	}

	logPaneOp("close", session, fmt.Sprintf("pane=%s", paneID))
	fmt.Printf("%s Closed pane %s in %s\n", style.SuccessPrefix, style.Bold.Render(paneID), session)
	return nil
}

func runPaneList(cmd *cobra.Command, args []string) error {
	session := args[0]

	if _, err := checkPaneAccess(); err != nil {
		return err
	}

	tracker := pane.NewTracker()
	state, err := tracker.Load(session)
	if err != nil {
		return fmt.Errorf("loading pane state: %w", err)
	}

	if paneJSON {
		return paneListJSON(state)
	}

	if state.PaneCount() == 0 {
		fmt.Printf("No agent-created panes in session %s\n", session)
		return nil
	}

	fmt.Printf("%s Agent panes in %s (%d/%d)\n\n",
		style.Bold.Render("Panes"),
		session,
		state.PaneCount(),
		pane.MaxPanes)

	for _, p := range state.Panes {
		age := time.Since(p.CreatedAt).Round(time.Second)
		fmt.Printf("  %s  %s  %s  %s ago\n",
			style.Bold.Render(p.PaneID),
			style.Dim.Render(fmt.Sprintf("[%s]", p.Type)),
			paneListTruncate(p.Command, 40),
			style.Dim.Render(age.String()))
		fmt.Printf("       %s %s\n",
			style.Dim.Render("by"),
			p.CreatedBy)
	}
	return nil
}

func runPaneLayout(cmd *cobra.Command, args []string) error {
	session := args[0]

	tracker, state, role, err := panePrecheck(session)
	if err != nil {
		return err
	}

	if !tmuxHasSession(session) {
		return fmt.Errorf("session %q not found", session)
	}

	newPanes := 0
	if layoutRight != "" {
		newPanes++
	}
	if layoutBottom != "" {
		newPanes++
	}

	if newPanes == 0 {
		return fmt.Errorf("specify at least one of --right or --bottom")
	}

	currentCount := state.PaneCount()
	if currentCount+newPanes > pane.MaxPanes {
		return fmt.Errorf("would exceed pane limit: %d existing + %d new > %d max",
			currentCount, newPanes, pane.MaxPanes)
	}

	// Create right pane (vertical split: -h)
	if layoutRight != "" {
		splitArgs := []string{"split-window", "-t", session, "-d", "-h", layoutRight}
		if _, err := tmuxRun(splitArgs...); err != nil {
			return fmt.Errorf("creating right pane: %w", err)
		}
		paneID, err := getNewestPane(session)
		if err != nil {
			return fmt.Errorf("getting right pane ID: %w", err)
		}
		if err := tmuxSetRemainOnExit(paneID, true); err != nil {
			fmt.Fprintf(os.Stderr, "%s remain-on-exit: %v\n", style.WarningPrefix, err)
		}
		state.AddPane(&pane.PaneInfo{
			PaneID:    paneID,
			SessionID: session,
			Command:   layoutRight,
			CreatedBy: role.ActorString(),
			CreatedAt: time.Now(),
			Type:      "layout",
		})
		fmt.Printf("  %s Right pane: %s\n", style.ArrowPrefix, style.Bold.Render(paneID))
	}

	// Create bottom pane (horizontal split: -v)
	if layoutBottom != "" {
		splitArgs := []string{"split-window", "-t", session, "-d", "-v", layoutBottom}
		if _, err := tmuxRun(splitArgs...); err != nil {
			return fmt.Errorf("creating bottom pane: %w", err)
		}
		paneID, err := getNewestPane(session)
		if err != nil {
			return fmt.Errorf("getting bottom pane ID: %w", err)
		}
		if err := tmuxSetRemainOnExit(paneID, true); err != nil {
			fmt.Fprintf(os.Stderr, "%s remain-on-exit: %v\n", style.WarningPrefix, err)
		}
		state.AddPane(&pane.PaneInfo{
			PaneID:    paneID,
			SessionID: session,
			Command:   layoutBottom,
			CreatedBy: role.ActorString(),
			CreatedAt: time.Now(),
			Type:      "layout",
		})
		fmt.Printf("  %s Bottom pane: %s\n", style.ArrowPrefix, style.Bold.Render(paneID))
	}

	// Apply main-vertical layout if requested
	if layoutMain {
		if _, err := tmuxRun("select-layout", "-t", session, "main-vertical"); err != nil {
			fmt.Fprintf(os.Stderr, "%s applying layout: %v\n", style.WarningPrefix, err)
		}
	}

	state.RecordOperation()
	if err := tracker.Save(state); err != nil {
		fmt.Fprintf(os.Stderr, "%s saving pane state: %v\n", style.WarningPrefix, err)
	}

	logPaneOp("layout", session, fmt.Sprintf("right=%q bottom=%q", layoutRight, layoutBottom))
	fmt.Printf("%s Layout created in %s\n", style.SuccessPrefix, session)
	return nil
}

// paneListJSON outputs pane list as JSON.
func paneListJSON(state *pane.SessionState) error {
	fmt.Print("[")
	for i, p := range state.Panes {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf(`{"pane_id":%q,"session":%q,"command":%q,"created_by":%q,"type":%q,"created_at":%q}`,
			p.PaneID, p.SessionID, p.Command, p.CreatedBy, p.Type, p.CreatedAt.Format(time.RFC3339))
	}
	fmt.Println("]")
	return nil
}

// paneListTruncate shortens a string to maxLen, adding "..." if truncated.
func paneListTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
