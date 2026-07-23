package nudge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

func TestEnqueueAndDrain(t *testing.T) {
	townRoot := t.TempDir()

	session := "gt-gastown-crew-sean"
	n1 := QueuedNudge{
		Sender:   "mayor",
		Message:  "Check your hook",
		Priority: PriorityNormal,
	}
	n2 := QueuedNudge{
		Sender:   "gastown/witness",
		Message:  "Polecat alpha is stuck",
		Priority: PriorityUrgent,
	}

	// Enqueue two nudges
	if err := Enqueue(townRoot, session, n1); err != nil {
		t.Fatalf("Enqueue n1: %v", err)
	}
	// Small delay to ensure different timestamps
	time.Sleep(time.Millisecond)
	if err := Enqueue(townRoot, session, n2); err != nil {
		t.Fatalf("Enqueue n2: %v", err)
	}

	// Check pending count
	count, err := Pending(townRoot, session)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != 2 {
		t.Errorf("Pending = %d, want 2", count)
	}

	// Drain
	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 2 {
		t.Fatalf("Drain returned %d nudges, want 2", len(nudges))
	}

	// Verify FIFO order
	if nudges[0].Sender != "mayor" {
		t.Errorf("nudges[0].Sender = %q, want %q", nudges[0].Sender, "mayor")
	}
	if nudges[1].Sender != "gastown/witness" {
		t.Errorf("nudges[1].Sender = %q, want %q", nudges[1].Sender, "gastown/witness")
	}

	// After drain, pending should be 0
	count, err = Pending(townRoot, session)
	if err != nil {
		t.Fatalf("Pending after drain: %v", err)
	}
	if count != 0 {
		t.Errorf("Pending after drain = %d, want 0", count)
	}
}

func TestDrainEmptyQueue(t *testing.T) {
	townRoot := t.TempDir()

	nudges, err := Drain(townRoot, "nonexistent-session")
	if err != nil {
		t.Fatalf("Drain empty: %v", err)
	}
	if len(nudges) != 0 {
		t.Errorf("Drain empty returned %d nudges, want 0", len(nudges))
	}
}

func TestDrainSkipsMalformed(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test"

	// Create queue dir and a malformed file
	dir := filepath.Join(townRoot, ".runtime", "nudge_queue", session)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "100.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	// Enqueue a valid nudge (with later timestamp)
	n := QueuedNudge{
		Sender:    "test",
		Message:   "valid",
		Timestamp: time.Now().Add(time.Second),
	}
	if err := Enqueue(townRoot, session, n); err != nil {
		t.Fatal(err)
	}

	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 1 {
		t.Fatalf("got %d nudges, want 1 (malformed should be skipped)", len(nudges))
	}
	if nudges[0].Message != "valid" {
		t.Errorf("got message %q, want %q", nudges[0].Message, "valid")
	}

	// Malformed file should have been cleaned up (renamed to .claimed then removed)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("queue dir should be empty after drain, got %d entries: %v", len(entries), names)
	}
}

func TestFormatForInjection_Normal(t *testing.T) {
	nudges := []QueuedNudge{
		{Sender: "mayor", Message: "Check status", Priority: PriorityNormal},
	}
	output := FormatForInjection(nudges)

	if output == "" {
		t.Fatal("FormatForInjection returned empty string")
	}
	if !strings.Contains(output, "<system-reminder>") {
		t.Error("missing <system-reminder> tag")
	}
	if !strings.Contains(output, "background notification") {
		t.Error("normal nudges should mention background notification")
	}
	if strings.Contains(output, "URGENT") {
		t.Error("normal nudges should not contain URGENT")
	}
}

func TestFormatForInjection_Urgent(t *testing.T) {
	nudges := []QueuedNudge{
		{Sender: "witness", Message: "Polecat stuck", Priority: PriorityUrgent},
		{Sender: "mayor", Message: "FYI", Priority: PriorityNormal},
	}
	output := FormatForInjection(nudges)

	if !strings.Contains(output, "URGENT") {
		t.Error("should mention URGENT for urgent nudges")
	}
	if !strings.Contains(output, "Handle urgent") {
		t.Error("should instruct agent to handle urgent nudges")
	}
	if !strings.Contains(output, "non-urgent") {
		t.Error("should mention non-urgent nudges")
	}
}

func TestFormatForInjection_Empty(t *testing.T) {
	output := FormatForInjection(nil)
	if output != "" {
		t.Errorf("FormatForInjection(nil) = %q, want empty", output)
	}
}

func TestPendingNonexistentDir(t *testing.T) {
	count, err := Pending("/nonexistent/path", "session")
	if err != nil {
		t.Fatalf("Pending on nonexistent dir should not error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestEnqueueDefaults(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-defaults"

	// Enqueue with zero timestamp and empty priority — should get defaults
	n := QueuedNudge{
		Sender:  "test",
		Message: "hello",
	}
	if err := Enqueue(townRoot, session, n); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 1 {
		t.Fatalf("got %d nudges, want 1", len(nudges))
	}
	if nudges[0].Priority != PriorityNormal {
		t.Errorf("Priority = %q, want %q", nudges[0].Priority, PriorityNormal)
	}
	if nudges[0].Timestamp.IsZero() {
		t.Error("Timestamp should have been set to non-zero default")
	}
	if nudges[0].ExpiresAt.IsZero() {
		t.Error("ExpiresAt should have been set to non-zero default")
	}
	// Normal priority should get DefaultNormalTTL
	expectedExpiry := nudges[0].Timestamp.Add(DefaultNormalTTL)
	if !nudges[0].ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("ExpiresAt = %v, want %v (Timestamp + DefaultNormalTTL)", nudges[0].ExpiresAt, expectedExpiry)
	}
}

func TestEnqueueUrgentTTL(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-urgent-ttl"

	n := QueuedNudge{
		Sender:   "test",
		Message:  "urgent message",
		Priority: PriorityUrgent,
	}
	if err := Enqueue(townRoot, session, n); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 1 {
		t.Fatalf("got %d nudges, want 1", len(nudges))
	}
	// Urgent priority should get DefaultUrgentTTL
	expectedExpiry := nudges[0].Timestamp.Add(DefaultUrgentTTL)
	if !nudges[0].ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("ExpiresAt = %v, want %v (Timestamp + DefaultUrgentTTL)", nudges[0].ExpiresAt, expectedExpiry)
	}
}

func TestEnqueueCustomExpiry(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-custom-expiry"

	customExpiry := time.Now().Add(5 * time.Minute)
	n := QueuedNudge{
		Sender:    "test",
		Message:   "custom expiry",
		ExpiresAt: customExpiry,
	}
	if err := Enqueue(townRoot, session, n); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 1 {
		t.Fatalf("got %d nudges, want 1", len(nudges))
	}
	// Custom expiry should be preserved, not overwritten by default TTL
	if !nudges[0].ExpiresAt.Equal(customExpiry) {
		t.Errorf("ExpiresAt = %v, want %v (custom)", nudges[0].ExpiresAt, customExpiry)
	}
}

func TestDrainSkipsExpired(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-expired"

	// Enqueue an already-expired nudge
	expired := QueuedNudge{
		Sender:    "old-sender",
		Message:   "stale message",
		Timestamp: time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(-30 * time.Minute), // expired 30 min ago
	}
	if err := Enqueue(townRoot, session, expired); err != nil {
		t.Fatalf("Enqueue expired: %v", err)
	}

	// Enqueue a fresh nudge
	time.Sleep(time.Millisecond)
	fresh := QueuedNudge{
		Sender:  "new-sender",
		Message: "fresh message",
	}
	if err := Enqueue(townRoot, session, fresh); err != nil {
		t.Fatalf("Enqueue fresh: %v", err)
	}

	// Pending counts both (doesn't check expiry)
	pending, err := Pending(townRoot, session)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if pending != 2 {
		t.Errorf("Pending = %d, want 2 (counts all files)", pending)
	}

	// Drain should skip the expired nudge
	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 1 {
		t.Fatalf("Drain returned %d nudges, want 1 (expired should be skipped)", len(nudges))
	}
	if nudges[0].Sender != "new-sender" {
		t.Errorf("got sender %q, want %q", nudges[0].Sender, "new-sender")
	}

	// After drain, queue dir should be empty (both files removed)
	dir := filepath.Join(townRoot, ".runtime", "nudge_queue", session)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("queue dir should be empty after drain, got %d entries", len(entries))
	}
}

func TestEnqueueQueueDepthLimit(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-depth"

	// Fill the queue to MaxQueueDepth
	for i := 0; i < MaxQueueDepth; i++ {
		n := QueuedNudge{
			Sender:  "sender",
			Message: "msg",
		}
		if err := Enqueue(townRoot, session, n); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	// Next enqueue should fail
	overflow := QueuedNudge{
		Sender:  "sender",
		Message: "overflow",
	}
	err := Enqueue(townRoot, session, overflow)
	if err == nil {
		t.Fatal("expected error when queue is full")
	}
	if !strings.Contains(err.Error(), "is full") {
		t.Errorf("got error %q, want to contain 'is full'", err.Error())
	}

	// Verify pending count is at max
	pending, _ := Pending(townRoot, session)
	if pending != MaxQueueDepth {
		t.Errorf("Pending = %d, want %d", pending, MaxQueueDepth)
	}

	// After draining, enqueue should work again
	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != MaxQueueDepth {
		t.Errorf("Drain returned %d, want %d", len(nudges), MaxQueueDepth)
	}

	err = Enqueue(townRoot, session, overflow)
	if err != nil {
		t.Errorf("Enqueue after drain should succeed: %v", err)
	}
}

func TestDrainSweepsOrphanedClaims(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-orphans"

	dir := filepath.Join(townRoot, ".runtime", "nudge_queue", session)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create an orphaned .claimed file with old mod time
	// Claim files now use the format: <original>.json.claimed.<suffix>
	orphanPath := filepath.Join(dir, "100.json.claimed.deadbeef")
	if err := os.WriteFile(orphanPath, []byte(`{"sender":"ghost"}`), 0644); err != nil {
		t.Fatal(err)
	}
	// Set mod time to well past the stale threshold
	oldTime := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(orphanPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// Create a fresh .claimed file (should NOT be swept)
	freshClaimPath := filepath.Join(dir, "200.json.claimed.cafebabe")
	if err := os.WriteFile(freshClaimPath, []byte(`{"sender":"active"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Enqueue a valid nudge
	n := QueuedNudge{Sender: "test", Message: "valid"}
	if err := Enqueue(townRoot, session, n); err != nil {
		t.Fatal(err)
	}

	// First Drain: requeues the orphaned claim (rename .claimed → .json),
	// keeps the fresh claim, and returns the valid nudge.
	// The requeued file isn't in the current ReadDir snapshot, so it's
	// picked up on the next Drain call.
	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 1 {
		t.Fatalf("first Drain got %d nudges, want 1", len(nudges))
	}
	if nudges[0].Message != "valid" {
		t.Errorf("got message %q, want %q", nudges[0].Message, "valid")
	}

	// The orphaned .claimed file should have been requeued as .json
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Error("orphaned .claimed file should no longer exist (requeued to .json)")
	}
	// Restored path strips everything from ".claimed" onward
	restoredPath := filepath.Join(dir, "100.json")
	if _, err := os.Stat(restoredPath); os.IsNotExist(err) {
		t.Error("restored .json file should exist after requeue")
	}

	// Second Drain: picks up the requeued orphan
	nudges2, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("second Drain: %v", err)
	}
	if len(nudges2) != 1 {
		t.Fatalf("second Drain got %d nudges, want 1 (the requeued orphan)", len(nudges2))
	}
	if nudges2[0].Sender != "ghost" {
		t.Errorf("got sender %q, want %q", nudges2[0].Sender, "ghost")
	}

	// The fresh claim should still exist (not old enough to sweep)
	if _, err := os.Stat(freshClaimPath); os.IsNotExist(err) {
		t.Error("fresh .claimed file should NOT have been swept")
	}
}

func TestConcurrentEnqueueNoDuplicateLoss(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-concurrent"

	// Fire 20 concurrent enqueues — all should succeed without collision.
	const count = 20
	var wg sync.WaitGroup
	errs := make(chan error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			n := QueuedNudge{
				Sender:  "sender",
				Message: strings.Repeat("x", i+1), // unique per goroutine
			}
			if err := Enqueue(townRoot, session, n); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Enqueue failed: %v", err)
	}

	// All 20 should be pending
	pending, err := Pending(townRoot, session)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if pending != count {
		t.Errorf("Pending = %d, want %d (some nudges lost to collision?)", pending, count)
	}

	// Drain should return all 20
	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != count {
		t.Errorf("Drain returned %d, want %d", len(nudges), count)
	}
}

// --- DeliverAfter tests ---

// TestDrainSkipsDeferredNudge verifies that a nudge with a future DeliverAfter
// is not returned by Drain and remains in the queue.
func TestDrainSkipsDeferredNudge(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-deferred"

	deferred := QueuedNudge{
		Sender:       "system",
		Message:      "reply reminder",
		DeliverAfter: time.Now().Add(10 * time.Second), // far future
	}
	if err := Enqueue(townRoot, session, deferred); err != nil {
		t.Fatalf("Enqueue deferred: %v", err)
	}

	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 0 {
		t.Fatalf("Drain returned %d nudges, want 0 (deferred not ready)", len(nudges))
	}

	// File should still be in queue (not discarded)
	pending, err := Pending(townRoot, session)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if pending != 1 {
		t.Errorf("Pending = %d, want 1 (deferred nudge still in queue)", pending)
	}
}

// TestDrainDeliversDeferredNudgeWhenReady verifies that a nudge with a past
// DeliverAfter is delivered normally.
func TestDrainDeliversDeferredNudgeWhenReady(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-deferred-ready"

	ready := QueuedNudge{
		Sender:       "system",
		Message:      "reply reminder",
		DeliverAfter: time.Now().Add(-1 * time.Second), // already past
	}
	if err := Enqueue(townRoot, session, ready); err != nil {
		t.Fatalf("Enqueue ready: %v", err)
	}

	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 1 {
		t.Fatalf("Drain returned %d nudges, want 1 (deferred is ready)", len(nudges))
	}
	if nudges[0].Message != "reply reminder" {
		t.Errorf("got message %q, want %q", nudges[0].Message, "reply reminder")
	}
}

// TestDrainMixedDeferredAndReady verifies that only ready nudges are returned
// when a mix of deferred and immediately-deliverable nudges are queued.
func TestDrainMixedDeferredAndReady(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-mixed-deferred"

	// Enqueue: immediate, then deferred, then immediate (interleaved order).
	n1 := QueuedNudge{Sender: "mayor", Message: "immediate-1"}
	if err := Enqueue(townRoot, session, n1); err != nil {
		t.Fatalf("Enqueue n1: %v", err)
	}
	time.Sleep(time.Millisecond)

	deferred := QueuedNudge{
		Sender:       "system",
		Message:      "deferred",
		DeliverAfter: time.Now().Add(60 * time.Second),
	}
	if err := Enqueue(townRoot, session, deferred); err != nil {
		t.Fatalf("Enqueue deferred: %v", err)
	}
	time.Sleep(time.Millisecond)

	n2 := QueuedNudge{Sender: "witness", Message: "immediate-2"}
	if err := Enqueue(townRoot, session, n2); err != nil {
		t.Fatalf("Enqueue n2: %v", err)
	}

	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 2 {
		t.Fatalf("Drain returned %d nudges, want 2 (deferred stays in queue)", len(nudges))
	}
	if nudges[0].Message != "immediate-1" {
		t.Errorf("nudges[0].Message = %q, want %q", nudges[0].Message, "immediate-1")
	}
	if nudges[1].Message != "immediate-2" {
		t.Errorf("nudges[1].Message = %q, want %q", nudges[1].Message, "immediate-2")
	}

	// Deferred nudge remains in queue
	pending, err := Pending(townRoot, session)
	if err != nil {
		t.Fatalf("Pending after drain: %v", err)
	}
	if pending != 1 {
		t.Errorf("Pending = %d, want 1 (deferred nudge still in queue)", pending)
	}
}

func TestRemoveKindByThread(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-remove"

	keep := QueuedNudge{Sender: "system", Message: "keep", Kind: "mail", ThreadID: "thread-1"}
	removeA := QueuedNudge{Sender: "system", Message: "remove-a", Kind: "reply-reminder", ThreadID: "thread-1"}
	removeB := QueuedNudge{Sender: "system", Message: "remove-b", Kind: "reply-reminder", ThreadID: "thread-1"}
	otherThread := QueuedNudge{Sender: "system", Message: "other-thread", Kind: "reply-reminder", ThreadID: "thread-2"}

	for _, n := range []QueuedNudge{keep, removeA, removeB, otherThread} {
		if err := Enqueue(townRoot, session, n); err != nil {
			t.Fatalf("Enqueue(%q): %v", n.Message, err)
		}
		time.Sleep(time.Millisecond)
	}

	removed, err := RemoveKindByThread(townRoot, session, "reply-reminder", "thread-1")
	if err != nil {
		t.Fatalf("RemoveKindByThread: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}

	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 2 {
		t.Fatalf("Drain returned %d nudges, want 2", len(nudges))
	}
	if nudges[0].Message != "keep" {
		t.Fatalf("nudges[0].Message = %q, want %q", nudges[0].Message, "keep")
	}
	if nudges[1].Message != "other-thread" {
		t.Fatalf("nudges[1].Message = %q, want %q", nudges[1].Message, "other-thread")
	}
}

// TestDeferredNudgeDeliveredAfterDelay uses a very short DeliverAfter to confirm
// that the same nudge is skipped on first Drain and delivered on a second Drain
// after the deadline elapses.
func TestDeferredNudgeDeliveredAfterDelay(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-deferred-sequence"

	shortDelay := QueuedNudge{
		Sender:       "system",
		Message:      "reply via mail",
		DeliverAfter: time.Now().Add(50 * time.Millisecond),
	}
	if err := Enqueue(townRoot, session, shortDelay); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// First Drain: not ready yet.
	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("first Drain: %v", err)
	}
	if len(nudges) != 0 {
		t.Fatalf("first Drain: got %d nudges, want 0 (deferred not ready)", len(nudges))
	}

	// Wait for deadline.
	time.Sleep(60 * time.Millisecond)

	// Second Drain: ready now.
	nudges, err = Drain(townRoot, session)
	if err != nil {
		t.Fatalf("second Drain: %v", err)
	}
	if len(nudges) != 1 {
		t.Fatalf("second Drain: got %d nudges, want 1 (deferred now ready)", len(nudges))
	}
	if nudges[0].Message != "reply via mail" {
		t.Errorf("got message %q, want %q", nudges[0].Message, "reply via mail")
	}

	// Queue should now be empty.
	pending, err := Pending(townRoot, session)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if pending != 0 {
		t.Errorf("Pending = %d, want 0 (deferred nudge delivered)", pending)
	}
}

// TestZeroDeliverAfterIsImmediate verifies that a zero DeliverAfter (unset)
// is treated as immediately deliverable (not deferred).
func TestZeroDeliverAfterIsImmediate(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-zero-deliver-after"

	n := QueuedNudge{
		Sender:  "mayor",
		Message: "no delay",
		// DeliverAfter intentionally left zero
	}
	if err := Enqueue(townRoot, session, n); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	nudges, err := Drain(townRoot, session)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(nudges) != 1 {
		t.Fatalf("got %d nudges, want 1 (zero DeliverAfter = immediate)", len(nudges))
	}
}

func TestConcurrentDrainNoDoubleDeli(t *testing.T) {
	townRoot := t.TempDir()
	session := "gt-test-drain-race"

	// Enqueue 10 nudges
	const count = 10
	for i := 0; i < count; i++ {
		n := QueuedNudge{
			Sender:  "sender",
			Message: strings.Repeat("m", i+1),
		}
		if err := Enqueue(townRoot, session, n); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
		time.Sleep(time.Millisecond) // ensure ordering
	}

	// Race 5 concurrent Drains — total nudges collected should equal count.
	const drainers = 5
	var wg sync.WaitGroup
	results := make(chan []QueuedNudge, drainers)

	for i := 0; i < drainers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nudges, err := Drain(townRoot, session)
			if err != nil {
				t.Errorf("concurrent Drain: %v", err)
				return
			}
			results <- nudges
		}()
	}
	wg.Wait()
	close(results)

	total := 0
	for nudges := range results {
		total += len(nudges)
	}

	// On Windows, transient sharing violations (antivirus, search indexer)
	// can prevent all concurrent drainers from claiming a file.  The nudge
	// stays as .json and is picked up on the next Drain — mirror that here
	// with a straggler sweep so the test validates no-loss, not one-shot
	// completeness.
	for retries := 0; retries < 3 && total < count; retries++ {
		time.Sleep(50 * time.Millisecond)
		stragglers, err := Drain(townRoot, session)
		if err != nil {
			t.Fatalf("straggler Drain: %v", err)
		}
		total += len(stragglers)
	}

	if total != count {
		t.Errorf("concurrent Drains delivered %d total nudges, want exactly %d (double-delivery or loss)", total, count)
	}

	// Verify no double-delivery: total must be exactly count, not more.
	if total > count {
		t.Errorf("double delivery detected: got %d total nudges, want exactly %d", total, count)
	}
}

// --- SweepExpired -----------------------------------------------------------

// countDiscardsBySession returns how many discards a sweep recorded for a given
// session name.
func countDiscardsBySession(res SweepResult, session string) int {
	n := 0
	for _, d := range res.Discarded {
		if d.Session == session {
			n++
		}
	}
	return n
}

func dirRemoved(res SweepResult, session string) bool {
	for _, s := range res.DirsRemoved {
		if s == session {
			return true
		}
	}
	return false
}

// TestSweepExpiredReclaimsDeadSessionDir is the aegis-l7nn case: a queue whose
// session no longer exists is never drained, so an expired nudge (and its dir)
// strand forever. The sweep must discard the nudge and reclaim the empty dir.
func TestSweepExpiredReclaimsDeadSessionDir(t *testing.T) {
	townRoot := t.TempDir()
	session := "aegis-crew-ghost"

	expired := QueuedNudge{
		Sender:    "gt-mayor",
		Message:   "window-GO",
		Kind:      "direct",
		Timestamp: time.Now().Add(-3 * time.Hour),
		ExpiresAt: time.Now().Add(-2 * time.Hour),
	}
	if err := Enqueue(townRoot, session, expired); err != nil {
		t.Fatalf("Enqueue expired: %v", err)
	}

	res, err := SweepExpired(townRoot, false)
	if err != nil {
		t.Fatalf("SweepExpired: %v", err)
	}
	if got := countDiscardsBySession(res, session); got != 1 {
		t.Fatalf("discarded %d for %s, want 1", got, session)
	}
	if res.Discarded[0].Reason != "expired" {
		t.Errorf("reason = %q, want expired", res.Discarded[0].Reason)
	}
	if res.Discarded[0].Kind != "direct" {
		t.Errorf("kind = %q, want direct", res.Discarded[0].Kind)
	}
	if !dirRemoved(res, session) {
		t.Errorf("empty dead-session dir %s should have been reclaimed", session)
	}
	if _, err := os.Stat(filepath.Join(townRoot, ".runtime", "nudge_queue", session)); !os.IsNotExist(err) {
		t.Errorf("dir %s should no longer exist on disk", session)
	}
}

// TestSweepExpiredKeepsUnexpired: a still-valid nudge (and its dir, which may
// belong to a session that is merely restarting) must survive untouched.
func TestSweepExpiredKeepsUnexpired(t *testing.T) {
	townRoot := t.TempDir()
	session := "aegis-crew-live"

	fresh := QueuedNudge{Sender: "ian", Message: "reindex done"} // Enqueue sets a future TTL
	if err := Enqueue(townRoot, session, fresh); err != nil {
		t.Fatalf("Enqueue fresh: %v", err)
	}

	res, err := SweepExpired(townRoot, false)
	if err != nil {
		t.Fatalf("SweepExpired: %v", err)
	}
	if got := countDiscardsBySession(res, session); got != 0 {
		t.Errorf("discarded %d for live session, want 0", got)
	}
	if dirRemoved(res, session) {
		t.Errorf("non-empty dir %s must not be removed", session)
	}
	pending, _ := Pending(townRoot, session)
	if pending != 1 {
		t.Errorf("Pending = %d, want 1 (fresh nudge preserved)", pending)
	}
}

// TestSweepExpiredMixed: expired discarded, fresh kept, dir NOT reclaimed
// because it is still non-empty.
func TestSweepExpiredMixed(t *testing.T) {
	townRoot := t.TempDir()
	session := "aegis-crew-mixed"

	expired := QueuedNudge{
		Sender:    "old",
		Message:   "stale",
		Timestamp: time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	if err := Enqueue(townRoot, session, expired); err != nil {
		t.Fatalf("Enqueue expired: %v", err)
	}
	time.Sleep(time.Millisecond)
	if err := Enqueue(townRoot, session, QueuedNudge{Sender: "new", Message: "fresh"}); err != nil {
		t.Fatalf("Enqueue fresh: %v", err)
	}

	res, err := SweepExpired(townRoot, false)
	if err != nil {
		t.Fatalf("SweepExpired: %v", err)
	}
	if got := countDiscardsBySession(res, session); got != 1 {
		t.Fatalf("discarded %d, want 1 (only the expired one)", got)
	}
	if dirRemoved(res, session) {
		t.Errorf("dir still holds a fresh nudge; must not be reclaimed")
	}
	pending, _ := Pending(townRoot, session)
	if pending != 1 {
		t.Errorf("Pending = %d, want 1 (fresh survives)", pending)
	}
}

// TestSweepExpiredDryRun reports the discard but changes nothing on disk.
func TestSweepExpiredDryRun(t *testing.T) {
	townRoot := t.TempDir()
	session := "aegis-crew-dry"

	expired := QueuedNudge{
		Sender:    "old",
		Message:   "stale",
		Timestamp: time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	if err := Enqueue(townRoot, session, expired); err != nil {
		t.Fatalf("Enqueue expired: %v", err)
	}

	res, err := SweepExpired(townRoot, true)
	if err != nil {
		t.Fatalf("SweepExpired dry-run: %v", err)
	}
	if got := countDiscardsBySession(res, session); got != 1 {
		t.Errorf("dry-run should still report 1 discard, got %d", got)
	}
	if len(res.DirsRemoved) != 0 {
		t.Errorf("dry-run must not remove dirs, got %v", res.DirsRemoved)
	}
	// File must still be on disk.
	pending, _ := Pending(townRoot, session)
	if pending != 1 {
		t.Errorf("dry-run left %d pending, want 1 (nothing removed)", pending)
	}
}

// TestSweepExpiredMalformed reclaims an unparseable file that no drain would
// ever deliver.
func TestSweepExpiredMalformed(t *testing.T) {
	townRoot := t.TempDir()
	session := "aegis-crew-junk"
	dir := queueDir(townRoot, session)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "100-abcd.json"), []byte("not json"), 0644); err != nil {
		t.Fatalf("write malformed: %v", err)
	}

	res, err := SweepExpired(townRoot, false)
	if err != nil {
		t.Fatalf("SweepExpired: %v", err)
	}
	if got := countDiscardsBySession(res, session); got != 1 || res.Discarded[0].Reason != "malformed" {
		t.Fatalf("want 1 malformed discard, got %+v", res.Discarded)
	}
	if !dirRemoved(res, session) {
		t.Errorf("dir should be reclaimed after removing the only (malformed) file")
	}
}

// TestSweepExpiredLegacyNoExpiresAt: a nudge written before ExpiresAt existed
// (zero ExpiresAt) still gets reclaimed via the Timestamp+TTL fallback once old
// enough, and a recent one with zero ExpiresAt is kept.
func TestSweepExpiredLegacyNoExpiresAt(t *testing.T) {
	townRoot := t.TempDir()
	dir := queueDir(townRoot, "aegis-legacy")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Old legacy nudge: zero ExpiresAt, timestamp well beyond the urgent TTL.
	old := QueuedNudge{Sender: "ancient", Message: "x", Timestamp: time.Now().Add(-DefaultUrgentTTL - time.Hour)}
	oldData, _ := json.Marshal(old)
	if err := os.WriteFile(filepath.Join(dir, "100-old.json"), oldData, 0644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	// Recent legacy nudge: zero ExpiresAt, timestamp within the fallback window.
	recent := QueuedNudge{Sender: "recent", Message: "y", Timestamp: time.Now().Add(-time.Minute)}
	recentData, _ := json.Marshal(recent)
	if err := os.WriteFile(filepath.Join(dir, "200-recent.json"), recentData, 0644); err != nil {
		t.Fatalf("write recent: %v", err)
	}

	res, err := SweepExpired(townRoot, false)
	if err != nil {
		t.Fatalf("SweepExpired: %v", err)
	}
	if got := countDiscardsBySession(res, "aegis-legacy"); got != 1 {
		t.Fatalf("want 1 discard (only the old legacy nudge), got %d", got)
	}
	if res.Discarded[0].Sender != "ancient" {
		t.Errorf("discarded wrong nudge: %q, want ancient", res.Discarded[0].Sender)
	}
}

// TestSweepExpiredSkipsClaimed leaves an in-flight .claimed file to the live
// drainer's own orphan sweep.
func TestSweepExpiredSkipsClaimed(t *testing.T) {
	townRoot := t.TempDir()
	session := "aegis-crew-claimed"
	dir := queueDir(townRoot, session)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	claimed := QueuedNudge{Sender: "inflight", Timestamp: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(-time.Minute)}
	data, _ := json.Marshal(claimed)
	claimedPath := filepath.Join(dir, "100-abcd.json.claimed.deadbeef")
	if err := os.WriteFile(claimedPath, data, 0644); err != nil {
		t.Fatalf("write claimed: %v", err)
	}

	res, err := SweepExpired(townRoot, false)
	if err != nil {
		t.Fatalf("SweepExpired: %v", err)
	}
	if got := countDiscardsBySession(res, session); got != 0 {
		t.Errorf("sweep must not touch .claimed files, discarded %d", got)
	}
	if _, err := os.Stat(claimedPath); err != nil {
		t.Errorf("claimed file should still exist: %v", err)
	}
}

// TestSweepExpiredEmptyRoot: no nudge_queue directory yet is not an error.
func TestSweepExpiredEmptyRoot(t *testing.T) {
	res, err := SweepExpired(t.TempDir(), false)
	if err != nil {
		t.Fatalf("SweepExpired on empty town: %v", err)
	}
	if res.DirsScanned != 0 || len(res.Discarded) != 0 {
		t.Errorf("empty town should sweep nothing, got %+v", res)
	}
}

// --- rename-following (aegis-qlkj) ------------------------------------------

func TestCrewMember(t *testing.T) {
	cases := []struct {
		session string
		extra   []string
		want    string
		ok      bool
	}{
		{"aegis-crew-kelly", nil, "kelly", true},
		{"gt-crew-kelly", nil, "kelly", true},
		{"gt-gastown-crew-sean", nil, "sean", true}, // multi-segment rig prefix
		{"shanty-kelly", []string{"shanty-"}, "kelly", true},
		{"shanty-kelly", nil, "", false},             // prefix not configured
		{"hq-mayor", []string{"shanty-"}, "", false}, // no scheme matches
		{"aegis-crew-", nil, "", false},              // empty member
		{"shanty-", []string{"shanty-"}, "", false},  // empty member
	}
	for _, c := range cases {
		got, ok := crewMember(c.session, c.extra)
		if got != c.want || ok != c.ok {
			t.Errorf("crewMember(%q, %v) = (%q, %v), want (%q, %v)",
				c.session, c.extra, got, ok, c.want, c.ok)
		}
	}
}

// The measured failure: nudges enqueued to the member's OLD session name are
// delivered when the member drains under a DIFFERENT scheme's name.
func TestDrainFollowsRenameAcrossSchemes(t *testing.T) {
	townRoot := t.TempDir()
	writeNudgeConfig(t, townRoot, &config.NudgeThresholds{
		SessionAliasPrefixes: []string{"shanty-"},
	})

	// Sender addressed kelly under the crew scheme...
	if err := Enqueue(townRoot, "aegis-crew-kelly", QueuedNudge{
		Sender: "sattler", Message: "stranded-1", Priority: PriorityNormal,
	}); err != nil {
		t.Fatalf("Enqueue crew: %v", err)
	}
	time.Sleep(time.Millisecond)
	// ...and a second under a different rig prefix, same member.
	if err := Enqueue(townRoot, "gt-crew-kelly", QueuedNudge{
		Sender: "dearing", Message: "stranded-2", Priority: PriorityNormal,
	}); err != nil {
		t.Fatalf("Enqueue crew2: %v", err)
	}
	time.Sleep(time.Millisecond)
	// kelly's own live session has a message too.
	if err := Enqueue(townRoot, "shanty-kelly", QueuedNudge{
		Sender: "arnold", Message: "own", Priority: PriorityNormal,
	}); err != nil {
		t.Fatalf("Enqueue own: %v", err)
	}

	// kelly, running as shanty-kelly, drains — and gets ALL three, FIFO.
	got, err := Drain(townRoot, "shanty-kelly")
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Drain returned %d nudges, want 3 (own + 2 stranded)", len(got))
	}
	wantOrder := []string{"stranded-1", "stranded-2", "own"}
	for i, w := range wantOrder {
		if got[i].Message != w {
			t.Errorf("nudge[%d] = %q, want %q (FIFO by timestamp across dirs)", i, got[i].Message, w)
		}
	}

	// The alias queues are now empty — a second drain yields nothing.
	if again, _ := Drain(townRoot, "shanty-kelly"); len(again) != 0 {
		t.Errorf("second Drain returned %d, want 0 (aliases consumed, no double-delivery)", len(again))
	}
	if n, _ := Pending(townRoot, "aegis-crew-kelly"); n != 0 {
		t.Errorf("alias dir still holds %d after drain, want 0", n)
	}
}

// A different member's queue must never be swept up by the alias scan.
func TestDrainAliasDoesNotCrossMembers(t *testing.T) {
	townRoot := t.TempDir()
	writeNudgeConfig(t, townRoot, &config.NudgeThresholds{
		SessionAliasPrefixes: []string{"shanty-"},
	})
	if err := Enqueue(townRoot, "aegis-crew-tim", QueuedNudge{
		Sender: "x", Message: "for-tim", Priority: PriorityNormal,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// kelly drains; tim's queue must be untouched.
	got, err := Drain(townRoot, "shanty-kelly")
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("kelly drained %d nudges, want 0 (tim's must not cross over)", len(got))
	}
	if n, _ := Pending(townRoot, "aegis-crew-tim"); n != 1 {
		t.Errorf("tim's queue holds %d, want 1 (undisturbed)", n)
	}
}

// Without the alias prefix configured, cross-scheme drain does NOT happen —
// the native crew scheme still cross-drains, but shanty- does not.
func TestDrainNoAliasWhenUnconfigured(t *testing.T) {
	townRoot := t.TempDir()
	writeNudgeConfig(t, townRoot, &config.NudgeThresholds{}) // no alias prefixes
	if err := Enqueue(townRoot, "aegis-crew-kelly", QueuedNudge{
		Sender: "x", Message: "stranded", Priority: PriorityNormal,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// shanty-kelly is not a recognized member without the prefix -> no aliasing.
	got, _ := Drain(townRoot, "shanty-kelly")
	if len(got) != 0 {
		t.Errorf("drained %d without alias prefix configured, want 0", len(got))
	}
	// But a crew-scheme session for the same member DOES cross-drain natively.
	got2, _ := Drain(townRoot, "gt-crew-kelly")
	if len(got2) != 1 || got2[0].Message != "stranded" {
		t.Errorf("native crew cross-drain failed: got %d", len(got2))
	}
}

// writeNudgeConfig writes a town settings file so nudgeConfig() picks up the
// given thresholds — the same on-disk path production loads from.
func writeNudgeConfig(t *testing.T, townRoot string, n *config.NudgeThresholds) {
	t.Helper()
	dir := filepath.Join(townRoot, "settings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir settings: %v", err)
	}
	ts := config.TownSettings{
		Type:        "town-settings",
		Version:     1,
		Operational: &config.OperationalConfig{Nudge: n},
	}
	data, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}
