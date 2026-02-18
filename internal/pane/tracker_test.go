package pane

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrackerLoadSave(t *testing.T) {
	dir := t.TempDir()
	tracker := &Tracker{configDir: dir}

	// Load non-existent session returns empty state
	state, err := tracker.Load("test-session")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Session != "test-session" {
		t.Errorf("Session = %q, want %q", state.Session, "test-session")
	}
	if len(state.Panes) != 0 {
		t.Errorf("Panes = %d, want 0", len(state.Panes))
	}

	// Add pane and save
	state.AddPane(&PaneInfo{
		PaneID:    "%5",
		SessionID: "test-session",
		Command:   "htop",
		CreatedBy: "gastown/crew/max",
		CreatedAt: time.Now(),
		Type:      "open",
	})

	if err := tracker.Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "test-session.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not found: %v", err)
	}

	// Reload and verify
	loaded, err := tracker.Load("test-session")
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if len(loaded.Panes) != 1 {
		t.Fatalf("Panes = %d, want 1", len(loaded.Panes))
	}
	if loaded.Panes[0].PaneID != "%5" {
		t.Errorf("PaneID = %q, want %%5", loaded.Panes[0].PaneID)
	}
	if loaded.Panes[0].Command != "htop" {
		t.Errorf("Command = %q, want htop", loaded.Panes[0].Command)
	}
}

func TestSessionStateAddRemovePane(t *testing.T) {
	state := &SessionState{Session: "test"}

	state.AddPane(&PaneInfo{PaneID: "%1", Type: "open"})
	state.AddPane(&PaneInfo{PaneID: "%2", Type: "exec"})
	state.AddPane(&PaneInfo{PaneID: "%3", Type: "show"})

	if state.PaneCount() != 3 {
		t.Fatalf("PaneCount = %d, want 3", state.PaneCount())
	}

	// Remove middle pane
	if !state.RemovePane("%2") {
		t.Error("RemovePane(%2) = false, want true")
	}
	if state.PaneCount() != 2 {
		t.Fatalf("PaneCount after remove = %d, want 2", state.PaneCount())
	}

	// Remove non-existent pane
	if state.RemovePane("%99") {
		t.Error("RemovePane(%99) = true, want false")
	}

	// Verify remaining panes
	if state.Panes[0].PaneID != "%1" || state.Panes[1].PaneID != "%3" {
		t.Errorf("remaining panes = [%s, %s], want [%%1, %%3]",
			state.Panes[0].PaneID, state.Panes[1].PaneID)
	}
}

func TestCheckPaneLimit(t *testing.T) {
	state := &SessionState{Session: "test"}

	// Under limit - should pass
	for i := 0; i < MaxPanes-1; i++ {
		state.AddPane(&PaneInfo{PaneID: "%" + string(rune('0'+i))})
	}
	if err := state.CheckPaneLimit(); err != nil {
		t.Errorf("CheckPaneLimit with %d panes: %v", state.PaneCount(), err)
	}

	// At limit - should fail
	state.AddPane(&PaneInfo{PaneID: "%z"})
	if err := state.CheckPaneLimit(); err == nil {
		t.Error("CheckPaneLimit at max: expected error, got nil")
	}
}

func TestCheckRateLimit(t *testing.T) {
	state := &SessionState{Session: "test"}

	// First 3 operations should succeed
	for i := 0; i < RateLimitMax; i++ {
		if err := state.CheckRateLimit(); err != nil {
			t.Fatalf("CheckRateLimit op %d: %v", i+1, err)
		}
		state.RecordOperation()
	}

	// 4th should fail
	if err := state.CheckRateLimit(); err == nil {
		t.Error("CheckRateLimit after max ops: expected error, got nil")
	}
}

func TestCheckRateLimitExpiry(t *testing.T) {
	state := &SessionState{Session: "test"}

	// Add old operations (more than 1 minute ago)
	oldTime := time.Now().Add(-2 * time.Minute)
	state.Operations = []time.Time{oldTime, oldTime, oldTime}

	// Should pass since old ops expired
	if err := state.CheckRateLimit(); err != nil {
		t.Errorf("CheckRateLimit with expired ops: %v", err)
	}
}

func TestTrackerClean(t *testing.T) {
	dir := t.TempDir()
	tracker := &Tracker{configDir: dir}

	// Create a state file
	state := &SessionState{Session: "cleanup-test"}
	state.AddPane(&PaneInfo{PaneID: "%1"})
	if err := tracker.Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Clean should remove the file
	if err := tracker.Clean("cleanup-test"); err != nil {
		t.Fatalf("Clean: %v", err)
	}

	path := filepath.Join(dir, "cleanup-test.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("state file still exists after Clean")
	}

	// Cleaning non-existent should not error
	if err := tracker.Clean("nonexistent"); err != nil {
		t.Errorf("Clean nonexistent: %v", err)
	}
}
