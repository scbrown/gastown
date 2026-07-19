package tmux

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestRI87_DashLeadingPayloadDelivers is the positive control for aegis-ri87
// acceptance item 2: message content beginning with a dash must be delivered
// literally, never parsed by tmux as a send-keys flag.
//
// Before the fix, sendMessageToTarget emitted `send-keys -t X -l <payload>`;
// a payload (or, for long messages, a mid-message chunk) starting with a dash
// was consumed by tmux getopt as a flag -> "unknown flag -D". The fix inserts
// a `--` end-of-options terminator: `send-keys -t X -l -- <payload>`.
//
// This test drives the REAL Tmux methods against a REAL isolated tmux server.
func TestRI87_DashLeadingPayloadDelivers(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	sock := "ri87-test-sock"
	tm := NewTmuxWithSocket(sock)
	defer func() { _ = exec.Command("tmux", "-L", sock, "kill-server").Run() }()

	const sess = "ri87"
	if err := tm.NewSession(sess, "/tmp"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sess) }()
	// let the shell paint a prompt so captured content is stable
	time.Sleep(300 * time.Millisecond)

	cases := []struct {
		name    string
		payload string
		needle  string // distinctive literal substring expected in the pane
	}{
		{
			// small-message path -> sendKeysLiteralWithRetry (tmux.go:1596)
			name:    "small_leading_dash",
			payload: "-D ri87 dash-leading small payload NEEDLESMALL",
			needle:  "NEEDLESMALL",
		},
		{
			// long-message path -> chunk branch (tmux.go:1565). The chunk
			// boundary at 512 lands right before a dash so the SECOND chunk
			// starts with "-D...", exactly the failure mode.
			name:    "chunk_boundary_dash",
			payload: strings.Repeat("x", sendKeysChunkSize) + "-Dri87chunkNEEDLECHUNK",
			needle:  "NEEDLECHUNK",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// clear the input line before each case
			_, _ = tm.run("send-keys", "-t", sess, "C-u")
			time.Sleep(100 * time.Millisecond)

			if err := tm.sendMessageToTarget(sess, tc.payload); err != nil {
				t.Fatalf("sendMessageToTarget returned error (delivery FAILED): %v", err)
			}
			time.Sleep(200 * time.Millisecond)

			pane, err := tm.CapturePaneAll(sess)
			if err != nil {
				t.Fatalf("CapturePaneAll: %v", err)
			}
			if !strings.Contains(pane, tc.needle) {
				t.Fatalf("payload not found in pane — delivery did not land.\nwant substring: %q\npane:\n%s", tc.needle, pane)
			}
		})
	}
}

// TestRI87_NegativeControl proves the ROOT CAUSE: the pre-fix invocation form
// (`send-keys -l <dash-payload>` with no `--`) really does error on this host's
// tmux, and the fixed form (`send-keys -l -- <dash-payload>`) succeeds. If this
// ever stops failing, the bug is unreproducible and the fix is untestable —
// which is itself worth knowing (aegis-0214: a check never observed to fail is
// not evidence).
func TestRI87_NegativeControl(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	sock := "ri87-neg-sock"
	tm := NewTmuxWithSocket(sock)
	defer func() { _ = exec.Command("tmux", "-L", sock, "kill-server").Run() }()

	const sess = "ri87neg"
	if err := tm.NewSession(sess, "/tmp"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sess) }()
	time.Sleep(300 * time.Millisecond)

	// OLD form (no terminator) MUST fail with a flag-parse error.
	if _, err := tm.run("send-keys", "-t", sess, "-l", "-D dash payload"); err == nil {
		t.Fatal("expected pre-fix form to error on dash-leading payload, but it succeeded — bug is unreproducible on this tmux")
	} else if !strings.Contains(err.Error(), "flag") {
		t.Fatalf("pre-fix form errored but not with a flag-parse error: %v", err)
	}

	// NEW form (with terminator) MUST succeed.
	if _, err := tm.run("send-keys", "-t", sess, "-l", "--", "-D dash payload"); err != nil {
		t.Fatalf("fixed form (-- terminator) should succeed on dash-leading payload: %v", err)
	}
}
