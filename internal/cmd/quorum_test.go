package cmd

import (
	"testing"
)

func TestResolveCrewAddress(t *testing.T) {
	tests := []struct {
		name     string
		rig      string
		member   string
		expected string
	}{
		{
			name:     "simple crew member",
			rig:      "gastown",
			member:   "arnold",
			expected: "gastown/crew/arnold",
		},
		{
			name:     "different rig",
			rig:      "aegis",
			member:   "ellie",
			expected: "aegis/crew/ellie",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveCrewAddress(tt.rig, tt.member)
			if got != tt.expected {
				t.Errorf("resolveCrewAddress(%q, %q) = %q, want %q",
					tt.rig, tt.member, got, tt.expected)
			}
		})
	}
}

func TestQuorumCreateValidation(t *testing.T) {
	// Test that lead must be in members list
	t.Run("lead must be in members", func(t *testing.T) {
		// Set up flags manually
		quorumTopic = "Test Topic"
		quorumMembers = []string{"arnold", "ellie", "ian"}
		quorumLead = "maldoon" // Not in members
		quorumOutput = "docs/test.md"

		err := quorumCreateCmd.RunE(quorumCreateCmd, nil)
		if err == nil {
			t.Error("expected error when lead not in members, got nil")
		}
		if err != nil && !contains(err.Error(), "must be one of the members") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestQuorumCommandRegistration(t *testing.T) {
	// Verify quorum command is registered with correct group and subcommands
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "quorum" {
			found = true
			if cmd.GroupID != GroupComm {
				t.Errorf("quorum GroupID = %q, want %q", cmd.GroupID, GroupComm)
			}

			// Check subcommands
			subCmds := make(map[string]bool)
			for _, sub := range cmd.Commands() {
				subCmds[sub.Name()] = true
			}
			for _, expected := range []string{"create", "status", "list"} {
				if !subCmds[expected] {
					t.Errorf("quorum missing subcommand %q", expected)
				}
			}
			break
		}
	}
	if !found {
		t.Error("quorum command not registered on root")
	}
}
