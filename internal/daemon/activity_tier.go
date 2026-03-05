package daemon

import (
	"bufio"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ActivityTier represents the current resource budget tier.
// Tier 1: Full (>75% budget OR reset <1h)
// Tier 2: Crew + Planning (50-75% budget, >2h to reset)
// Tier 3: Crew Only (25-50% budget, >3h to reset)
// Tier 4: Conservation (<25% budget, >4h to reset)
type ActivityTier int

const (
	TierFull         ActivityTier = 1
	TierCrewPlanning ActivityTier = 2
	TierCrewOnly     ActivityTier = 3
	TierConservation ActivityTier = 4
)

// activityTierFile returns the path to the tier file.
func activityTierFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "activity-tier")
}

// calculateActivityTier reads budget metrics from Pushgateway, calculates the
// activity tier, writes it to ~/.claude/activity-tier, and sets GT_ACTIVITY_TIER
// on all active crew sessions.
func (d *Daemon) calculateActivityTier() {
	budgetPct, hoursToReset := readBudgetMetrics(d.pushgatewayURL())
	tier := computeTier(budgetPct, hoursToReset)

	// Write tier to file for patrol-check.sh to read
	tierFile := activityTierFile()
	if err := os.MkdirAll(filepath.Dir(tierFile), 0755); err != nil {
		d.logger.Printf("activity_tier: mkdir failed: %v", err)
		return
	}
	if err := os.WriteFile(tierFile, []byte(fmt.Sprintf("%d\n", tier)), 0644); err != nil {
		d.logger.Printf("activity_tier: write failed: %v", err)
		return
	}

	// Set GT_ACTIVITY_TIER on all active sessions
	tierStr := strconv.Itoa(int(tier))
	sessions, err := d.tmux.ListSessions()
	if err == nil {
		for _, sess := range sessions {
			_ = d.tmux.SetEnvironment(sess, "GT_ACTIVITY_TIER", tierStr)
		}
	}

	d.logger.Printf("activity_tier: tier=%d budget=%.0f%% reset=%.1fh", tier, budgetPct, hoursToReset)
}

// pushgatewayURL returns the Pushgateway metrics URL.
func (d *Daemon) pushgatewayURL() string {
	// Default to monitoring.lan:9091
	return "http://monitoring.lan:9091/metrics"
}

// readBudgetMetrics fetches budget metrics from Pushgateway.
// Returns (budgetPct, hoursToReset). On error, returns (100, 0) which yields tier 1.
func readBudgetMetrics(metricsURL string) (budgetPct float64, hoursToReset float64) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(metricsURL)
	if err != nil {
		return 100, 0 // Default to tier 1 on error
	}
	defer func() { _ = resp.Body.Close() }()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "claude_token_budget_remaining_pct") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if v, err := strconv.ParseFloat(parts[1], 64); err == nil {
					budgetPct = v
				}
			}
		}
		if strings.HasPrefix(line, "claude_token_reset_hours") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if v, err := strconv.ParseFloat(parts[1], 64); err == nil {
					hoursToReset = v
				}
			}
		}
	}

	if budgetPct == 0 && hoursToReset == 0 {
		return 100, 0 // No metrics found, default to tier 1
	}
	return budgetPct, hoursToReset
}

// computeTier calculates the activity tier from budget and reset metrics.
// Exported for testing.
func computeTier(budgetPct, hoursToReset float64) ActivityTier {
	// Calculate tier from budget
	budgetTier := TierFull
	if budgetPct < 25 {
		budgetTier = TierConservation
	} else if budgetPct < 50 {
		budgetTier = TierCrewOnly
	} else if budgetPct < 75 {
		budgetTier = TierCrewPlanning
	}

	// Calculate tier from time-to-reset
	timeTier := TierFull
	if hoursToReset > 4 {
		timeTier = TierConservation
	} else if hoursToReset > 3 {
		timeTier = TierCrewOnly
	} else if hoursToReset > 2 {
		timeTier = TierCrewPlanning
	}

	// If reset is imminent (<1h), override to tier 1
	if hoursToReset > 0 && hoursToReset < 1 {
		return TierFull
	}

	// Take the worse (higher number) of budget and time tier
	return ActivityTier(int(math.Max(float64(budgetTier), float64(timeTier))))
}

// ReadActivityTier reads the current activity tier from the tier file.
// Returns TierFull (1) if the file doesn't exist or can't be read.
func ReadActivityTier() ActivityTier {
	data, err := os.ReadFile(activityTierFile())
	if err != nil {
		return TierFull
	}
	tier, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || tier < 1 || tier > 4 {
		return TierFull
	}
	return ActivityTier(tier)
}
