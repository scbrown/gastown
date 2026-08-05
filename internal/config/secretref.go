package config

import (
	"fmt"
	"os"
	"strings"
)

// IsSecretRef reports whether a configured value is an indirection rather than
// a literal.
func IsSecretRef(raw string) bool {
	return strings.HasPrefix(raw, "${") && strings.HasSuffix(raw, "}")
}

// ResolveSecretRef resolves an indirection in a config value (aegis-n3izl).
//
// Two forms, both fail-closed:
//
//	${ENV_VAR}          value of that environment variable
//	${file:/path/to/f}  first line of that file, whitespace-trimmed
//
// Anything else is returned unchanged — a literal stays a literal, so existing
// configs keep working.
//
// WHY BOTH FORMS. Env alone would be fragile exactly where this matters: the
// escalation path runs from cron, timers and freshly-spawned agent sessions,
// and this fleet's own notes record that ~/.bash_env has a sourcing guard which
// can leave variables unset in sessions where PATH was already populated. An
// escalation that cannot page because a variable silently did not load is the
// failure this whole mechanism exists to prevent, so the file form is offered
// beside it rather than after it.
//
// WHY IT ERRORS RATHER THAN RETURNING EMPTY. An empty webhook is
// indistinguishable from "not configured", and the escalation path treats that
// as a reason to skip the push while still reporting the escalation as created.
// That is the silent-degradation shape this fleet keeps being bitten by, so an
// unresolved reference is an ERROR the caller must carry to the operator, not a
// value.
func ResolveSecretRef(raw string) (string, error) {
	if !IsSecretRef(raw) {
		return raw, nil
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(raw, "${"), "}")

	if path, ok := strings.CutPrefix(inner, "file:"); ok {
		path = strings.TrimSpace(path)
		if path == "" {
			return "", fmt.Errorf("%s names no file", raw)
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from the operator's own settings file
		if err != nil {
			return "", fmt.Errorf("%s could not be read: %w", raw, err)
		}
		// First line only: secret files routinely end with a newline, and a
		// trailing newline in a URL or bearer token fails in ways that are
		// tedious to diagnose.
		value := strings.TrimSpace(data2FirstLine(data))
		if value == "" {
			return "", fmt.Errorf("%s is empty", raw)
		}
		return value, nil
	}

	name := strings.TrimSpace(inner)
	if name == "" {
		return "", fmt.Errorf("%s names no variable", raw)
	}
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is not set in the environment", raw)
	}
	return value, nil
}

// data2FirstLine returns the first line of b.
func data2FirstLine(b []byte) string {
	s := string(b)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// resolveContactSecrets expands indirections in the secret-bearing contact
// fields, recording any that could not be resolved.
//
// An unresolved reference leaves the field EMPTY and records the reason. Empty
// is what the delivery path already treats as "not configured" and refuses to
// send on, so this cannot post a credential-shaped string like "${FOO}" to a
// webhook. The recorded reason is what turns the resulting skip message from
// "not configured" — which reads as an operator's omission — into a statement of
// which reference failed and why.
func resolveContactSecrets(c *EscalationContacts) {
	if c == nil {
		return
	}
	fields := []struct {
		name string
		ptr  *string
	}{
		{"sms_webhook", &c.SMSWebhook},
		{"slack_webhook", &c.SlackWebhook},
		{"smtp_pass", &c.SMTPPass},
		{"smtp_user", &c.SMTPUser},
	}
	for _, f := range fields {
		if !IsSecretRef(*f.ptr) {
			continue
		}
		value, err := ResolveSecretRef(*f.ptr)
		if err != nil {
			if c.UnresolvedRefs == nil {
				c.UnresolvedRefs = make(map[string]string)
			}
			c.UnresolvedRefs[f.name] = err.Error()
			*f.ptr = ""
			continue
		}
		*f.ptr = value
	}
}

// UnresolvedRef returns the reason field could not be resolved, or "" if it
// resolved or was never a reference.
func (c *EscalationContacts) UnresolvedRef(field string) string {
	if c == nil || c.UnresolvedRefs == nil {
		return ""
	}
	return c.UnresolvedRefs[field]
}
