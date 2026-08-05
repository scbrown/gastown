package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

// End-to-end for aegis-n3izl: a policy file holding NO credential still pushes.
//
// Verified by DELIVERY, not by config parsing — the assertion is that the
// server was actually POSTed to. A config that loads is not a webhook that
// fires, and that distinction is the entire reason this bead exists.
//
// The live policy routes `low` to `["bead"]` only, so a real low escalation
// cannot page and could not demonstrate this; high and critical carry
// `sms:human` and would page a human for a test. A local listener proves the
// same wire behaviour without either problem.
func TestPolicyWithNoCredentialInItStillDelivers(t *testing.T) {
	var got struct {
		posted bool
		method string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.posted, got.method = true, r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(realReceipt))
	}))
	defer srv.Close()

	// The credential lives in the environment; the file holds only a reference.
	t.Setenv("AEGIS_TEST_SMS_WEBHOOK", srv.URL)

	dir := t.TempDir()
	path := filepath.Join(dir, "escalation.json")
	policy := `{
  "type": "escalation",
  "version": 1,
  "routes": {"high": ["bead", "sms:human", "log"]},
  "contacts": {"human_sms": "someone", "sms_webhook": "${AEGIS_TEST_SMS_WEBHOOK}"}
}`
	if err := os.WriteFile(path, []byte(policy), 0o600); err != nil {
		t.Fatalf("writing policy: %v", err)
	}

	// This is the property the bead is about: the file on disk is safe to commit.
	if raw, _ := os.ReadFile(path); len(raw) > 0 {
		if strings.Contains(string(raw), srv.URL) {
			t.Fatal("the policy file contains the credential — the whole point is that it does not")
		}
	}

	cfg, err := config.LoadEscalationConfig(path)
	if err != nil {
		t.Fatalf("loading policy: %v", err)
	}
	if reason := cfg.Contacts.UnresolvedRef("sms_webhook"); reason != "" {
		t.Fatalf("reference did not resolve: %s", reason)
	}

	receipt, err := sendEscalationSMS(cfg, "bead-1", "high", "disk is full")
	if err != nil {
		t.Fatalf("push failed: %v", err)
	}
	if !got.posted {
		t.Fatal("NOTHING WAS POSTED — the config resolved but no webhook fired, which is exactly " +
			"the difference this bead is about")
	}
	if got.method != http.MethodPost {
		t.Errorf("escalation must PUBLISH (POST), got %s", got.method)
	}
	if receipt == "" {
		t.Error("no receipt returned — delivery must be observable, not assumed")
	}
}

// An unresolved reference must not silently read as "not configured".
func TestUnresolvedReferenceIsDistinguishableFromNotConfigured(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "escalation.json")
	policy := `{
  "type": "escalation",
  "version": 1,
  "routes": {"high": ["bead", "sms:human"]},
  "contacts": {"human_sms": "someone", "sms_webhook": "${AEGIS_TEST_NEVER_SET_ANYWHERE}"}
}`
	if err := os.WriteFile(path, []byte(policy), 0o600); err != nil {
		t.Fatalf("writing policy: %v", err)
	}

	cfg, err := config.LoadEscalationConfig(path)
	if err != nil {
		t.Fatalf("an unresolved credential must not stop the policy loading — the bead still "+
			"has to be created: %v", err)
	}
	if cfg.Contacts.SMSWebhook != "" {
		t.Errorf("unresolved reference survived as %q; it would be POSTed to as a URL",
			cfg.Contacts.SMSWebhook)
	}
	reason := cfg.Contacts.UnresolvedRef("sms_webhook")
	if reason == "" {
		t.Fatal("no reason recorded — the operator would be told 'not configured' and sent to " +
			"edit a file that is already correct")
	}
	if !strings.Contains(reason, "AEGIS_TEST_NEVER_SET_ANYWHERE") {
		t.Errorf("reason %q does not name the reference that failed", reason)
	}
}
