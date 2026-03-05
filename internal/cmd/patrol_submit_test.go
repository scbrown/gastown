package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeneratePatrolID(t *testing.T) {
	id, err := generatePatrolID()
	if err != nil {
		t.Fatalf("generatePatrolID() error: %v", err)
	}
	if !strings.HasPrefix(id, "pr-") {
		t.Errorf("generatePatrolID() = %q, want prefix 'pr-'", id)
	}
	if len(id) != 13 { // "pr-" + 10 hex chars
		t.Errorf("generatePatrolID() = %q, want length 13, got %d", id, len(id))
	}

	// IDs should be unique
	id2, _ := generatePatrolID()
	if id == id2 {
		t.Errorf("generatePatrolID() returned same ID twice: %q", id)
	}
}

func TestEscapeSQL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "'hello'"},
		{"it's", "'it''s'"},
		{"a'b'c", "'a''b''c'"},
		{"", "''"},
	}

	for _, tt := range tests {
		got := escapeSQL(tt.input)
		if got != tt.expected {
			t.Errorf("escapeSQL(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestPatrolReportJSON(t *testing.T) {
	report := PatrolReport{
		Agent:    "aegis/crew/wu",
		Domain:   "observability",
		Findings: json.RawMessage(`[{"severity":"ok","service":"prometheus"}]`),
		Metrics:  json.RawMessage(`{"alerts_firing":3}`),
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded PatrolReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Agent != "aegis/crew/wu" {
		t.Errorf("Agent = %q, want aegis/crew/wu", decoded.Agent)
	}
	if decoded.Domain != "observability" {
		t.Errorf("Domain = %q, want observability", decoded.Domain)
	}
}
