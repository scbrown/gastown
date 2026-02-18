// Package pane provides agent-driven pane tracking and rate limiting for shanty sessions.
//
// Pane state is persisted to ~/.config/shanty/panes/<session>.json. Each file tracks
// which panes were created by agents and their metadata, enabling cleanup and auditing.
package pane

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MaxPanes is the default maximum number of agent-created panes per session.
const MaxPanes = 6

// RateLimitWindow is the time window for rate limiting.
const RateLimitWindow = time.Minute

// RateLimitMax is the maximum pane operations per window per agent.
const RateLimitMax = 3

// PaneInfo tracks metadata about an agent-created pane.
type PaneInfo struct {
	PaneID    string    `json:"pane_id"`
	SessionID string    `json:"session"`
	Command   string    `json:"command,omitempty"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	Type      string    `json:"type"` // "open", "exec", "show", "layout"
}

// SessionState tracks all agent-created panes for a session.
type SessionState struct {
	Session    string      `json:"session"`
	Panes      []*PaneInfo `json:"panes"`
	Operations []time.Time `json:"operations"` // timestamps for rate limiting
	mu         sync.Mutex
}

// Tracker manages pane tracking for shanty sessions.
type Tracker struct {
	configDir string
}

// NewTracker creates a tracker using the default config directory.
func NewTracker() *Tracker {
	home, _ := os.UserHomeDir()
	return &Tracker{
		configDir: filepath.Join(home, ".config", "shanty", "panes"),
	}
}

// statePath returns the path to the session state file.
func (t *Tracker) statePath(session string) string {
	return filepath.Join(t.configDir, session+".json")
}

// Load reads the session state from disk.
func (t *Tracker) Load(session string) (*SessionState, error) {
	path := t.statePath(session)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &SessionState{Session: session}, nil
		}
		return nil, fmt.Errorf("reading pane state: %w", err)
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing pane state: %w", err)
	}
	state.Session = session
	return &state, nil
}

// Save writes the session state to disk.
func (t *Tracker) Save(state *SessionState) error {
	if err := os.MkdirAll(t.configDir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling pane state: %w", err)
	}

	path := t.statePath(state.Session)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing pane state: %w", err)
	}
	return nil
}

// AddPane records a new agent-created pane.
func (state *SessionState) AddPane(info *PaneInfo) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.Panes = append(state.Panes, info)
}

// RemovePane removes a pane from tracking by ID.
func (state *SessionState) RemovePane(paneID string) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	for i, p := range state.Panes {
		if p.PaneID == paneID {
			state.Panes = append(state.Panes[:i], state.Panes[i+1:]...)
			return true
		}
	}
	return false
}

// PaneCount returns the number of tracked panes.
func (state *SessionState) PaneCount() int {
	state.mu.Lock()
	defer state.mu.Unlock()
	return len(state.Panes)
}

// CheckRateLimit returns an error if the agent has exceeded the rate limit.
func (state *SessionState) CheckRateLimit() error {
	state.mu.Lock()
	defer state.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-RateLimitWindow)

	// Prune old operations
	var recent []time.Time
	for _, t := range state.Operations {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	state.Operations = recent

	if len(recent) >= RateLimitMax {
		oldest := recent[0]
		waitFor := RateLimitWindow - now.Sub(oldest)
		return fmt.Errorf("rate limit exceeded: %d operations in last minute (max %d), retry in %s",
			len(recent), RateLimitMax, waitFor.Round(time.Second))
	}
	return nil
}

// RecordOperation records a pane operation for rate limiting.
func (state *SessionState) RecordOperation() {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.Operations = append(state.Operations, time.Now())
}

// CheckPaneLimit returns an error if the session has too many agent panes.
func (state *SessionState) CheckPaneLimit() error {
	if state.PaneCount() >= MaxPanes {
		return fmt.Errorf("pane limit reached: %d/%d agent-created panes in session %q",
			state.PaneCount(), MaxPanes, state.Session)
	}
	return nil
}

// Clean removes the state file for a session.
func (t *Tracker) Clean(session string) error {
	path := t.statePath(session)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing pane state: %w", err)
	}
	return nil
}
