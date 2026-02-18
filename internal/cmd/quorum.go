package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Quorum command flags
var (
	quorumTopic   string
	quorumMembers []string
	quorumLead    string
	quorumOutput  string
	quorumBrief   string
	quorumJSON    bool
)

var quorumCmd = &cobra.Command{
	Use:     "quorum",
	GroupID: GroupComm,
	Short:   "Collaborative design reviews with multiple crew members",
	RunE:    requireSubcommand,
	Long: `Assemble a quorum of crew members for collaborative design reviews.

A quorum sends a design brief to all members, designates a lead to
assemble the final document, and tracks progress via beads.

WORKFLOW:
  1. Create a quorum with topic, members, lead, and output path
  2. All members receive the brief via mail
  3. Members exchange input via mail threads
  4. Lead assembles the unified document at the output path
  5. Caller reviews the result

Commands:
  gt quorum create    Create a new quorum and send briefs
  gt quorum status    Check status of an active quorum
  gt quorum list      List active quorums`,
}

var quorumCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new quorum and send design briefs",
	Long: `Create a quorum for collaborative design review.

Sends the design brief to all members and designates a lead who will
assemble the final document at the specified output path.

Examples:
  gt quorum create --topic 'Human Intent Archive' \
    --members arnold,ellie,ian,maldoon \
    --lead arnold \
    --output docs/designs/human-intent-archive.md

  gt quorum create --topic 'Auth Redesign' \
    --members dave,emma,fred \
    --lead emma \
    --output docs/designs/auth-redesign.md \
    --brief "We need to migrate from session-based to JWT auth..."`,
	RunE: runQuorumCreate,
}

var quorumStatusCmd = &cobra.Command{
	Use:   "status <quorum-id>",
	Short: "Check status of an active quorum",
	Long: `Show the status of a quorum including members and lead.

Examples:
  gt quorum status gt-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runQuorumStatus,
}

var quorumListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active quorums",
	Long: `List all active (open) quorums.

Examples:
  gt quorum list
  gt quorum list --json`,
	RunE: runQuorumList,
}

func init() {
	// Create flags
	quorumCreateCmd.Flags().StringVar(&quorumTopic, "topic", "", "Design topic (required)")
	quorumCreateCmd.Flags().StringSliceVar(&quorumMembers, "members", nil, "Comma-separated list of crew member names (required)")
	quorumCreateCmd.Flags().StringVar(&quorumLead, "lead", "", "Lead member who assembles the final doc (required)")
	quorumCreateCmd.Flags().StringVar(&quorumOutput, "output", "", "Output path for the final document (required)")
	quorumCreateCmd.Flags().StringVar(&quorumBrief, "brief", "", "Design brief / additional context (optional)")
	_ = quorumCreateCmd.MarkFlagRequired("topic")
	_ = quorumCreateCmd.MarkFlagRequired("members")
	_ = quorumCreateCmd.MarkFlagRequired("lead")
	_ = quorumCreateCmd.MarkFlagRequired("output")

	// List/status flags
	quorumListCmd.Flags().BoolVar(&quorumJSON, "json", false, "Output as JSON")
	quorumStatusCmd.Flags().BoolVar(&quorumJSON, "json", false, "Output as JSON")

	// Register subcommands
	quorumCmd.AddCommand(quorumCreateCmd)
	quorumCmd.AddCommand(quorumStatusCmd)
	quorumCmd.AddCommand(quorumListCmd)

	rootCmd.AddCommand(quorumCmd)
}

func runQuorumCreate(cmd *cobra.Command, args []string) error {
	// Validate lead is in members list
	leadFound := false
	for _, m := range quorumMembers {
		if m == quorumLead {
			leadFound = true
			break
		}
	}
	if !leadFound {
		return fmt.Errorf("lead %q must be one of the members (%s)", quorumLead, strings.Join(quorumMembers, ", "))
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	// Infer rig from cwd for member address resolution
	rigName, err := inferRigFromCwd(townRoot)
	if err != nil {
		return fmt.Errorf("cannot determine rig: %w", err)
	}

	from := detectSender()

	// Create a tracking bead for this quorum
	b := beads.New(townRoot)
	memberList := strings.Join(quorumMembers, ", ")
	description := fmt.Sprintf(`Quorum: %s

Lead: %s
Members: %s
Output: %s
Requested by: %s
Created: %s`,
		quorumTopic,
		quorumLead,
		memberList,
		quorumOutput,
		from,
		time.Now().Format("2006-01-02 15:04"),
	)
	if quorumBrief != "" {
		description += "\n\nBrief:\n" + quorumBrief
	}

	issue, err := b.Create(beads.CreateOptions{
		Title:       fmt.Sprintf("Quorum: %s", quorumTopic),
		Type:        "task",
		Priority:    1,
		Description: description,
		Actor:       from,
		Ephemeral:   true, // Quorum tracking is ephemeral
	})
	if err != nil {
		return fmt.Errorf("creating quorum bead: %w", err)
	}

	// Add quorum label for easy filtering
	err = b.Update(issue.ID, beads.UpdateOptions{
		AddLabels: []string{"quorum", "quorum-lead:" + quorumLead, "quorum-output:" + quorumOutput},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s Could not add quorum labels: %v\n", style.WarningPrefix, err)
	}

	// Build the design brief message
	briefBody := fmt.Sprintf(`You have been assembled for a design quorum.

TOPIC: %s
LEAD: %s (responsible for assembling the final document)
MEMBERS: %s
OUTPUT: %s
QUORUM ID: %s

`, quorumTopic, quorumLead, memberList, quorumOutput, issue.ID)

	if quorumBrief != "" {
		briefBody += "DESIGN BRIEF:\n" + quorumBrief + "\n\n"
	}

	briefBody += `WORKFLOW:
1. Review the topic and share your input via mail replies
2. The lead will collect all perspectives
3. The lead will produce a unified document at the output path
4. The quorum creator will review the final result

Reply to this message with your input. The lead will synthesize all contributions.`

	// Send mail to all members
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("mail work dir: %w", err)
	}
	router := mail.NewRouter(workDir)

	subject := fmt.Sprintf("Quorum: %s", quorumTopic)
	var sendErrors []string
	var sentTo []string

	for _, member := range quorumMembers {
		addr := resolveCrewAddress(rigName, member)

		msg := mail.NewMessage(from, addr, subject, briefBody)
		msg.Priority = mail.PriorityHigh
		msg.Type = mail.TypeTask

		// Mark the lead's message differently
		if member == quorumLead {
			msg.Body = fmt.Sprintf("[YOU ARE THE LEAD - Assemble the final doc at: %s]\n\n%s", quorumOutput, briefBody)
		}

		if err := router.Send(msg); err != nil {
			sendErrors = append(sendErrors, fmt.Sprintf("%s: %v", addr, err))
			continue
		}
		sentTo = append(sentTo, addr)
		_ = events.LogFeed(events.TypeMail, from, events.MailPayload(addr, subject))
	}

	if len(sendErrors) > 0 {
		if len(sentTo) == 0 {
			return fmt.Errorf("all sends failed: %s", strings.Join(sendErrors, "; "))
		}
		fmt.Fprintf(os.Stderr, "%s Some deliveries failed: %s\n", style.WarningPrefix, strings.Join(sendErrors, "; "))
	}

	// Print summary
	fmt.Printf("%s Quorum created: %s\n", style.Bold.Render("✓"), quorumTopic)
	fmt.Printf("  ID:      %s\n", issue.ID)
	fmt.Printf("  Lead:    %s\n", quorumLead)
	fmt.Printf("  Members: %s\n", memberList)
	fmt.Printf("  Output:  %s\n", quorumOutput)
	fmt.Printf("  Sent to: %s\n", strings.Join(sentTo, ", "))

	return nil
}

func runQuorumStatus(cmd *cobra.Command, args []string) error {
	quorumID := args[0]

	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	b := beads.New(townRoot)
	issue, err := b.Show(quorumID)
	if err != nil {
		return fmt.Errorf("getting quorum: %w", err)
	}

	// Verify it's a quorum bead
	if !beads.HasLabel(issue, "quorum") {
		return fmt.Errorf("%s is not a quorum bead", quorumID)
	}

	if quorumJSON {
		data, err := json.MarshalIndent(issue, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Parse lead and output from labels
	var lead, output string
	for _, label := range issue.Labels {
		if strings.HasPrefix(label, "quorum-lead:") {
			lead = strings.TrimPrefix(label, "quorum-lead:")
		}
		if strings.HasPrefix(label, "quorum-output:") {
			output = strings.TrimPrefix(label, "quorum-output:")
		}
	}

	statusIcon := "●"
	if issue.Status == "closed" {
		statusIcon = "✓"
	}

	fmt.Printf("%s %s [%s]\n", statusIcon, issue.Title, strings.ToUpper(issue.Status))
	fmt.Printf("  ID:     %s\n", issue.ID)
	if lead != "" {
		fmt.Printf("  Lead:   %s\n", lead)
	}
	if output != "" {
		fmt.Printf("  Output: %s\n", output)
	}
	fmt.Printf("\n%s\n", issue.Description)

	return nil
}

func runQuorumList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	b := beads.New(townRoot)
	issues, err := b.List(beads.ListOptions{
		Status:       "open",
		Label:        "quorum",
		Priority:     -1,
		IncludeWisps: true,
	})
	if err != nil {
		return fmt.Errorf("listing quorums: %w", err)
	}

	if quorumJSON {
		data, err := json.MarshalIndent(issues, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if len(issues) == 0 {
		fmt.Println("No active quorums.")
		return nil
	}

	fmt.Printf("Active quorums (%d):\n\n", len(issues))
	for _, issue := range issues {
		var lead string
		for _, label := range issue.Labels {
			if strings.HasPrefix(label, "quorum-lead:") {
				lead = strings.TrimPrefix(label, "quorum-lead:")
			}
		}
		leadStr := ""
		if lead != "" {
			leadStr = fmt.Sprintf(" (lead: %s)", lead)
		}
		fmt.Printf("  ● %s  %s%s\n", issue.ID, issue.Title, leadStr)
	}

	return nil
}

// resolveCrewAddress builds a mail address for a crew member.
// Tries common patterns: rig/crew/name first, then rig/name.
func resolveCrewAddress(rigName, memberName string) string {
	return fmt.Sprintf("%s/crew/%s", rigName, memberName)
}
