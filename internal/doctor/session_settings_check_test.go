package doctor

import "testing"

// The classifier is the load-bearing logic: a crew session is recognized by
// the Gas Town beacon, and only crew sessions are audited for --settings.
func TestClassifySessionCmdline(t *testing.T) {
	launcher := `claude --dangerously-skip-permissions --settings /x/crew/.claude/settings.json [GAS TOWN] crew arnold ... <- human • start`
	handoff := `claude --dangerously-skip-permissions [GAS TOWN] crew arnold ... <- self • handoff`
	bare := `claude --dangerously-skip-permissions`
	unrelated := `vim notes.md`

	if crew, has := classifySessionCmdline(launcher); !crew || !has {
		t.Fatalf("launcher session must classify crew=true settings=true, got %v %v", crew, has)
	}
	// THE BUG SHAPE (aegis-05up): a handoff-respawned crew session without
	// --settings must be flagged — this is the exact process state observed
	// on every guardless session.
	if crew, has := classifySessionCmdline(handoff); !crew || has {
		t.Fatalf("handoff session without --settings must classify crew=true settings=false, got %v %v", crew, has)
	}
	// A bare interactive claude is NOT a crew session and must not be flagged.
	if crew, _ := classifySessionCmdline(bare); crew {
		t.Fatal("bare claude must not classify as crew")
	}
	if crew, _ := classifySessionCmdline(unrelated); crew {
		t.Fatal("non-claude process must not classify as crew")
	}
}
