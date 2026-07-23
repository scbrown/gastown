package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/nudge"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	nudgeSweepDryRunFlag bool
	nudgeSweepJSONFlag   bool
	nudgeSweepQuietFlag  bool
)

func init() {
	rootCmd.AddCommand(nudgeSweepCmd)
	nudgeSweepCmd.Flags().BoolVar(&nudgeSweepDryRunFlag, "dry-run", false, "Report what would be discarded without removing anything")
	nudgeSweepCmd.Flags().BoolVar(&nudgeSweepJSONFlag, "json", false, "Emit the sweep result as JSON")
	nudgeSweepCmd.Flags().BoolVarP(&nudgeSweepQuietFlag, "quiet", "q", false, "Only print the summary line, not per-nudge detail")
}

var nudgeSweepCmd = &cobra.Command{
	Use:   "nudge-sweep",
	Short: "Discard expired nudges from every queue, including dead sessions'",
	Long: `Enforce nudge ExpiresAt across ALL queue directories, not just those a live
agent happens to drain.

ExpiresAt is otherwise only honored inside a drain, and a drain runs only for a
session that is still alive (via its turn-boundary hook or a running
nudge-poller). A queue whose session no longer exists is never drained, so its
nudges — and the directory itself — accumulate without bound (aegis-l7nn).

This sweep removes only nudges that can never be validly delivered: those past
ExpiresAt, plus any unparseable files. A live session's still-valid nudges are
left alone, so it is safe to run at any time — including from cron or a patrol.
Direct-kind nudges have no durable copy, so their expiry is a real message loss
and is logged individually.

Run with --dry-run first to see what would be dropped.`,
	Args: cobra.NoArgs,
	RunE: runNudgeSweep,
}

func runNudgeSweep(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("cannot find town root: %w", err)
	}

	result, err := nudge.SweepExpired(townRoot, nudgeSweepDryRunFlag)
	if err != nil {
		return fmt.Errorf("sweeping nudge queues: %w", err)
	}

	if nudgeSweepJSONFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	prefix := ""
	if nudgeSweepDryRunFlag {
		prefix = "[dry-run] "
	}

	if !nudgeSweepQuietFlag {
		for _, d := range result.Discarded {
			verb := "would discard"
			if !nudgeSweepDryRunFlag {
				verb = "discarded"
			}
			switch d.Reason {
			case "malformed":
				fmt.Printf("  %s%s malformed nudge in %s\n", prefix, verb, d.Session)
			default:
				sender := d.Sender
				if sender == "" {
					sender = "unknown"
				}
				kind := d.Kind
				if kind == "" {
					kind = "direct"
				}
				fmt.Printf("  %s%s %s nudge from %s in %s (expired %s ago)\n",
					prefix, verb, kind, sender, d.Session, d.Age.Round(time.Second))
			}
		}
	}

	fmt.Printf("%s %s%d nudge(s) across %d queue(s); %d empty dir(s) reclaimed\n",
		style.Bold.Render("✓"), prefix, len(result.Discarded), result.DirsScanned, len(result.DirsRemoved))

	return nil
}
