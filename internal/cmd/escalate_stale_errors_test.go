package cmd

import (
	"fmt"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/mail"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestEscalateStaleDoesNotHideRecordFailure(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"mayor", "settings", ".beads", "bin"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", root)
	t.Setenv("GT_ROOT", root)
	t.Setenv("GT_TOWN_ROOT", root)
	t.Setenv("BEADS_DIR", filepath.Join(root, ".beads"))
	t.Setenv("PATH", filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Chdir(root)
	for name, body := range map[string]string{
		"settings/escalation.json": `{"type":"escalation","version":1,"routes":{"high":["bead"]},"stale_threshold":"4h","max_reescalations":2}`,
		"bin/br":                   "#!/bin/sh\ncase \" $* \" in *\" show \"*) echo fixture-record-read-failed >&2; exit 97;; esac\nprintf '%s\\n' '[{\"id\":\"hq-fixture\",\"title\":\"fixture\",\"status\":\"open\",\"created_at\":\"2000-01-01T00:00:00Z\",\"labels\":[\"gt:escalation\"],\"description\":\"severity: medium\"}]'\n",
		"bin/bd":                   "#!/bin/sh\necho fixture-record-read-failed >&2\nexit 97\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	oldDry, oldJSON := escalateDryRun, escalateStaleJSON
	t.Cleanup(func() { escalateDryRun, escalateStaleJSON = oldDry, oldJSON })
	escalateDryRun = false
	for _, asJSON := range []bool{false, true} {
		escalateStaleJSON = asJSON
		err := runEscalateStale(&cobra.Command{}, nil)
		if err == nil || !strings.Contains(err.Error(), "fixture-record-read-failed") {
			t.Errorf("json=%v: failed record read must return error, got %v", asJSON, err)
		}
	}
}

func TestReescalateStaleBatchDeliveryFailures(t *testing.T) {
	for _, tc := range []struct {
		name                                              string
		updateFail, mailFail, externalFail, warning, skip bool
	}{
		{name: "success"}, {name: "record", updateFail: true}, {name: "mail", mailFail: true}, {name: "external", externalFail: true}, {name: "unconfigured", warning: true}, {name: "skip", skip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("GT_ROOT", root)
			t.Setenv("GT_TOWN_ROOT", root)
			t.Chdir(root)
			cfg := &config.EscalationConfig{Routes: map[string][]string{"high": {"mail:reviewer", "sms:human"}}}
			updates, sends, externals := 0, 0, 0
			results, err := reescalateStaleBatch([]*beads.Issue{{ID: "first"}, {ID: "second"}}, "tester", 2, cfg, root,
				func(id, actor string, max int) (*beads.ReescalationResult, error) {
					updates++
					if tc.updateFail && id == "first" {
						return nil, fmt.Errorf("record failed")
					}
					return &beads.ReescalationResult{ID: id, Title: "fixture", OldSeverity: "medium", NewSeverity: "high", Skipped: tc.skip}, nil
				},
				func(msg *mail.Message) error {
					sends++
					if msg.To != "reviewer" || msg.Priority != mail.PriorityHigh {
						t.Fatalf("wrong message: %#v", msg)
					}
					if tc.mailFail {
						return fmt.Errorf("mail failed")
					}
					return nil
				},
				func(actions []string, c *config.EscalationConfig, id, sev, title, reason, root string) []deliveryStatus {
					externals++
					if sev != "high" || !strings.Contains(reason, id) {
						t.Fatal("missing reescalation context")
					}
					status := deliveryStatus{Channel: "sms", RuntimeNotified: true}
					if tc.externalFail {
						status.Error = "push failed"
					}
					if tc.warning {
						status.Warning = "contact missing"
					}
					return []deliveryStatus{status}
				})
			wantError := tc.updateFail || tc.mailFail || tc.externalFail || tc.warning
			if (err != nil) != wantError {
				t.Fatalf("error=%v wantError=%v", err, wantError)
			}
			want := 2
			if tc.updateFail {
				want = 1
			}
			if len(results) != want || updates != 2 {
				t.Fatalf("batch stopped: results=%d updates=%d", len(results), updates)
			}
			if tc.skip {
				want = 0
			}
			if sends != want || externals != want {
				t.Fatalf("routes: mail=%d external=%d want=%d", sends, externals, want)
			}
		})
	}
}

func TestReescalateStalePublishesConfiguredExternalRoute(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		wantError   bool
	}{
		{"receipt", realReceipt, false}, {"html", webUIHTML, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("GT_ROOT", root)
			t.Setenv("GT_TOWN_ROOT", root)
			t.Chdir(root)
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != "POST" {
					t.Errorf("method=%s", r.Method)
				}
				_, _ = w.Write([]byte(tc.reply))
			}))
			defer server.Close()
			cfg := &config.EscalationConfig{Routes: map[string][]string{"critical": {"sms:human"}}}
			cfg.Contacts.HumanSMS = "tester"
			cfg.Contacts.SMSWebhook = server.URL
			_, err := reescalateStaleBatch([]*beads.Issue{{ID: "fixture"}}, "tester", 2, cfg, root,
				func(id, actor string, max int) (*beads.ReescalationResult, error) {
					return &beads.ReescalationResult{ID: id, Title: "fixture", OldSeverity: "high", NewSeverity: "critical"}, nil
				},
				func(*mail.Message) error { t.Fatal("unexpected mail route"); return nil }, executeExternalActions)
			if requests != 1 || (err != nil) != tc.wantError {
				t.Fatalf("requests=%d error=%v", requests, err)
			}
		})
	}
}
