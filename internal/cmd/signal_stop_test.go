package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/mail"
)

func TestIsSelfHandoff(t *testing.T) {
	tests := []struct {
		name    string
		msg     *mail.Message
		address string
		want    bool
	}{
		{
			name: "self handoff",
			msg: &mail.Message{
				From:    "gastown/crew/max",
				Subject: "🤝 HANDOFF: Session cycling",
			},
			address: "gastown/crew/max",
			want:    true,
		},
		{
			name: "handoff from other",
			msg: &mail.Message{
				From:    "gastown/crew/tom",
				Subject: "🤝 HANDOFF: Session cycling",
			},
			address: "gastown/crew/max",
			want:    false,
		},
		{
			name: "non-handoff from self",
			msg: &mail.Message{
				From:    "gastown/crew/max",
				Subject: "Regular message",
			},
			address: "gastown/crew/max",
			want:    false,
		},
		{
			name: "handoff keyword in subject",
			msg: &mail.Message{
				From:    "gastown/crew/max",
				Subject: "HANDOFF notes for next session",
			},
			address: "gastown/crew/max",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSelfHandoff(tt.msg, tt.address)
			if got != tt.want {
				t.Errorf("isSelfHandoff() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutputStopAllow(t *testing.T) {
	// outputStopAllow should not return an error
	err := outputStopAllow()
	if err != nil {
		t.Errorf("outputStopAllow() returned error: %v", err)
	}
}

func TestOutputStopBlock(t *testing.T) {
	// outputStopBlock should not return an error
	err := outputStopBlock("test reason")
	if err != nil {
		t.Errorf("outputStopBlock() returned error: %v", err)
	}
}
