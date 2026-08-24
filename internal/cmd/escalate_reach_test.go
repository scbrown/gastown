package cmd

import (
	"strings"
	"testing"
)

// "Routed to:" must describe what delivery ACTUALLY achieved (aegis-xkale).
//
// It used to render `strings.Join(targets, ", ")` where targets was MAIL targets
// only, so it printed EMPTY both when nobody was reached (MEDIUM: bead+log) and
// when somebody WAS (HIGH: a published push). Those two are the states an
// operator most needs told apart at 3am, and the field rendered them identically
// — under a success glyph and rc=0.
//
// Both directions in every case: what must appear, and what must not.
func TestDescribeReach(t *testing.T) {
	cases := []struct {
		name     string
		statuses []deliveryStatus
		want     string
		wantNot  string
	}{{
		name: "MEDIUM: bead+log only — the blank line that started this",
		statuses: []deliveryStatus{
			{Channel: "bead", Created: true},
			{Channel: "log", Target: "log"},
		},
		want:    "nobody",
		wantNot: "log",
	}, {
		name: "HIGH: a push that PUBLISHED must be named",
		statuses: []deliveryStatus{
			{Channel: "bead", Created: true},
			{Channel: "log", Target: "log"},
			{Channel: "sms", Target: "human"},
		},
		want:    "sms:human",
		wantNot: "nobody",
	}, {
		name: "a FAILED channel is not reach",
		statuses: []deliveryStatus{
			{Channel: "bead", Created: true},
			{Channel: "sms", Target: "human", Error: "ntfy 500"},
		},
		want:    "nobody",
		wantNot: "sms",
	}, {
		name: "partial: one fails, one succeeds — only the success is reach",
		statuses: []deliveryStatus{
			{Channel: "bead", Created: true},
			{Channel: "sms", Target: "human", Error: "ntfy 500"},
			{Channel: "mail", Target: "aegis/crew/sattler"},
		},
		want:    "mail:aegis/crew/sattler",
		wantNot: "sms",
	}, {
		name: "nothing at all succeeded, not even a record",
		statuses: []deliveryStatus{
			{Channel: "sms", Target: "human", Error: "ntfy 500"},
		},
		want:    "no delivery succeeded",
		wantNot: "sms",
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := describeReach(c.statuses)
			if got == "" {
				t.Fatalf("rendered EMPTY — a blank is not a statement, which is the whole defect")
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("want %q in %q", c.want, got)
			}
			if c.wantNot != "" && strings.Contains(got, c.wantNot) {
				t.Errorf("must NOT contain %q, got %q", c.wantNot, got)
			}
		})
	}
}
