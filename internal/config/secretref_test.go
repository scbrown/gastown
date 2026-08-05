package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A literal must survive untouched — every existing escalation.json holds one.
func TestResolveSecretRef_LiteralsPassThrough(t *testing.T) {
	for _, raw := range []string{
		"https://example.invalid/hook?token=abc",
		"",
		"not-a-reference",
		"${unterminated",
		"trailing}",
	} {
		got, err := ResolveSecretRef(raw)
		if err != nil {
			t.Errorf("ResolveSecretRef(%q) errored: %v", raw, err)
		}
		if got != raw {
			t.Errorf("ResolveSecretRef(%q) = %q, want it unchanged", raw, got)
		}
	}
}

func TestResolveSecretRef_EnvForm(t *testing.T) {
	t.Setenv("AEGIS_TEST_HOOK", "https://example.invalid/hook?token=live")

	got, err := ResolveSecretRef("${AEGIS_TEST_HOOK}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://example.invalid/hook?token=live" {
		t.Errorf("got %q, want the variable's value", got)
	}
}

func TestResolveSecretRef_FileForm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webhook")
	// Trailing newline is the normal shape of a secret file, and a newline
	// inside a URL fails in ways that are tedious to diagnose.
	if err := os.WriteFile(path, []byte("https://example.invalid/hook?token=live\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	got, err := ResolveSecretRef("${file:" + path + "}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://example.invalid/hook?token=live" {
		t.Errorf("got %q — the trailing newline must be trimmed", got)
	}
}

// Every unresolvable form must ERROR rather than return "". An empty webhook is
// indistinguishable from "not configured", and the delivery path skips on it
// while still reporting the escalation as created.
func TestResolveSecretRef_UnresolvableAlwaysErrors(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, []byte("\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	cases := map[string]string{
		"unset variable":   "${AEGIS_TEST_DEFINITELY_UNSET}",
		"empty variable":   "${AEGIS_TEST_EMPTY}",
		"missing file":     "${file:" + filepath.Join(dir, "nope") + "}",
		"empty file":       "${file:" + empty + "}",
		"no variable name": "${}",
		"no file path":     "${file:}",
	}
	t.Setenv("AEGIS_TEST_EMPTY", "")

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ResolveSecretRef(raw)
			if err == nil {
				t.Fatalf("ResolveSecretRef(%q) returned %q with no error — an empty value here "+
					"is indistinguishable from 'not configured' and silently skips the push", raw, got)
			}
			if got != "" {
				t.Errorf("got %q alongside the error; must be empty so nothing is posted", got)
			}
			if !strings.Contains(err.Error(), raw) {
				t.Errorf("error %q does not name the reference %q that failed", err, raw)
			}
		})
	}
}

// The whole point of the bead: the policy file can be versioned with the
// credential out of it, and the credential still reaches the delivery path.
func TestResolveContactSecrets_ExpandsAndRecords(t *testing.T) {
	t.Setenv("AEGIS_TEST_SMS", "https://example.invalid/sms")

	c := &EscalationContacts{
		SMSWebhook:   "${AEGIS_TEST_SMS}",
		SlackWebhook: "${AEGIS_TEST_DEFINITELY_UNSET}",
		SMTPPass:     "a-literal-password",
	}
	resolveContactSecrets(c)

	if c.SMSWebhook != "https://example.invalid/sms" {
		t.Errorf("sms_webhook = %q, want the resolved value", c.SMSWebhook)
	}
	if c.UnresolvedRef("sms_webhook") != "" {
		t.Errorf("a resolved field must record no failure, got %q", c.UnresolvedRef("sms_webhook"))
	}

	// Unresolved: emptied so nothing is posted, and the REASON is kept so the
	// skip message can say which reference failed.
	if c.SlackWebhook != "" {
		t.Errorf("slack_webhook = %q — an unresolved reference must not survive as a literal", c.SlackWebhook)
	}
	reason := c.UnresolvedRef("slack_webhook")
	if reason == "" {
		t.Fatal("no reason recorded — the skip message would say 'not configured', which is false")
	}
	if !strings.Contains(reason, "AEGIS_TEST_DEFINITELY_UNSET") {
		t.Errorf("reason %q does not name the reference", reason)
	}

	if c.SMTPPass != "a-literal-password" {
		t.Errorf("a literal must be untouched, got %q", c.SMTPPass)
	}
}

// CONTROL: without this, the tests above pass equally against a resolver that
// records a failure for every field, resolved or not.
func TestResolveContactSecrets_NoFailuresWhenAllResolve(t *testing.T) {
	t.Setenv("AEGIS_TEST_SMS", "https://example.invalid/sms")

	c := &EscalationContacts{SMSWebhook: "${AEGIS_TEST_SMS}", SlackWebhook: "https://example.invalid/slack"}
	resolveContactSecrets(c)

	if len(c.UnresolvedRefs) != 0 {
		t.Errorf("expected no unresolved refs, got %v", c.UnresolvedRefs)
	}
}
