package daemon

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComputeTier(t *testing.T) {
	tests := []struct {
		name         string
		budgetPct    float64
		hoursToReset float64
		want         ActivityTier
	}{
		{"full budget, no reset info", 100, 0, TierFull},
		{"high budget long reset", 80, 5, TierConservation}, // time >4h dominates
		{"mid-high budget", 60, 5, TierConservation},     // time >4h overrides
		{"mid budget", 40, 3.5, TierCrewOnly},             // both agree on tier 3
		{"low budget", 20, 5, TierConservation},            // budget <25 and time >4h
		{"reset imminent overrides", 10, 0.5, TierFull},    // <1h to reset = tier 1
		{"budget 75 boundary", 75, 1.5, TierFull},          // >=75 = tier 1
		{"budget 50 boundary", 50, 1.5, TierCrewPlanning},  // >=50 <75 = tier 2
		{"budget 25 boundary", 25, 1.5, TierCrewOnly},      // >=25 <50 = tier 3
		{"budget 24", 24, 1.5, TierConservation},            // <25 = tier 4
		{"time dominates", 90, 4.5, TierConservation},       // budget tier 1, time tier 4
		{"budget dominates", 15, 1.5, TierConservation},     // budget tier 4, time tier 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeTier(tt.budgetPct, tt.hoursToReset)
			if got != tt.want {
				t.Errorf("computeTier(%.0f, %.1f) = %d, want %d", tt.budgetPct, tt.hoursToReset, got, tt.want)
			}
		})
	}
}

func TestReadBudgetMetrics(t *testing.T) {
	t.Run("parses metrics", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "# HELP claude_token_budget_remaining_pct Budget remaining")
			fmt.Fprintln(w, "# TYPE claude_token_budget_remaining_pct gauge")
			fmt.Fprintln(w, "claude_token_budget_remaining_pct 42.5")
			fmt.Fprintln(w, "# HELP claude_token_reset_hours Hours to reset")
			fmt.Fprintln(w, "# TYPE claude_token_reset_hours gauge")
			fmt.Fprintln(w, "claude_token_reset_hours 3.2")
		}))
		defer srv.Close()

		budget, hours := readBudgetMetrics(srv.URL)
		if budget != 42.5 {
			t.Errorf("budget = %f, want 42.5", budget)
		}
		if hours != 3.2 {
			t.Errorf("hours = %f, want 3.2", hours)
		}
	})

	t.Run("defaults on error", func(t *testing.T) {
		budget, hours := readBudgetMetrics("http://localhost:1/nonexistent")
		if budget != 100 || hours != 0 {
			t.Errorf("got budget=%f hours=%f, want 100/0", budget, hours)
		}
	})

	t.Run("defaults on empty metrics", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "# some unrelated metric")
			fmt.Fprintln(w, "unrelated_metric 99")
		}))
		defer srv.Close()

		budget, hours := readBudgetMetrics(srv.URL)
		if budget != 100 || hours != 0 {
			t.Errorf("got budget=%f hours=%f, want 100/0", budget, hours)
		}
	})
}
