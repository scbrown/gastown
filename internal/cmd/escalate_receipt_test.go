package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

// A 2xx IS NOT DELIVERY (aegis-uz6i).
//
// `gt escalate` printed "📱 SMS sent to <human>" whenever the push webhook answered
// 2xx, and never looked at the body. On the escalation path that is the worst possible
// place for a false stamp: the message that says something is badly wrong is the one
// that must not silently fail to leave.
//
// Every byte string below was captured from the live endpoint on 2026-07-20, not
// invented — a real publish and the web UI that the SAME URL serves to a GET. Both are
// HTTP 200. The old check could not tell them apart; this one must.

// A real publish receipt, exactly as returned (topic renamed — the SHAPE is what is
// under test, and a live topic name does not belong in a public repo).
const realReceipt = `{"id":"NQ2oOnDcFWQr","time":1784524628,"expires":1784567828,` +
	`"event":"message","topic":"ops-alerts","title":"TEST","message":"probe",` +
	`"priority":1,"tags":["rotating_light"]}`

// The web UI the same URL serves. THIS is the 200 that proves nothing, and it is one
// reverse-proxy redirect away from being what every escalation gets: Go's http.Post
// follows 301/302 and reissues them as a GET.
const webUIHTML = "<!DOCTYPE html>\n<html lang=\"en\">\n  <head>\n    <meta charset=\"UTF-8\" />\n" +
	"    <title>ntfy web</title>\n"

func TestAPublishReceiptIsAccepted(t *testing.T) {
	got, err := parsePublishReceipt([]byte(realReceipt))
	if err != nil {
		t.Fatalf("a real receipt must be accepted, got error: %v", err)
	}
	if got != "ops-alerts/NQ2oOnDcFWQr" {
		t.Fatalf("receipt should identify topic and id, got %q", got)
	}
}

func TestTheWebUIHTMLIsREJECTED(t *testing.T) {
	// The whole point. This body arrives with HTTP 200 from the configured host.
	if _, err := parsePublishReceipt([]byte(webUIHTML)); err == nil {
		t.Fatal("HTML from the web UI was accepted as proof of publication — " +
			"this is the exact bug: a 2xx that queued no notification")
	}
}

func TestJSONThatIsNotAReceiptIsRejected(t *testing.T) {
	// Well-formed JSON, 200, and no publication: an error object, a health endpoint,
	// or an unrelated API. Parsing alone is not enough; the receipt fields carry the
	// proof, so their absence must fail.
	for _, body := range []string{
		`{"code":40301,"http":403,"error":"forbidden"}`,
		`{"status":"ok"}`,
		`{}`,
		`{"id":"","topic":"ops-alerts"}`, // empty id: no message exists
		`{"id":"abc","topic":""}`,        // no topic: nothing to have been published to
	} {
		if _, err := parsePublishReceipt([]byte(body)); err == nil {
			t.Fatalf("accepted %q as a publish receipt", body)
		}
	}
}

func TestAnEmptyBodyIsRejected(t *testing.T) {
	// A 200 with no body at all. Silence is not a receipt.
	if _, err := parsePublishReceipt(nil); err == nil {
		t.Fatal("an empty body was accepted as proof of publication")
	}
}

func TestTheErrorSaysWhatWasActuallyReceived(t *testing.T) {
	// The operator reading this is deciding whether a P1 reached anyone. "failed" is
	// not enough — they need to see WHAT came back to tell a redirect-to-web-UI from
	// an auth failure from a dead proxy.
	_, err := parsePublishReceipt([]byte(webUIHTML))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "DOCTYPE") && !strings.Contains(err.Error(), "html") {
		t.Fatalf("error should quote the body it rejected, got: %v", err)
	}
}

func TestALongBodyIsTruncatedNotDumped(t *testing.T) {
	big := make([]byte, 5000)
	for i := range big {
		big[i] = 'x'
	}
	_, err := parsePublishReceipt(big)
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(err.Error()) > 400 {
		t.Fatalf("a rejected body must not dump a whole page into the operator's "+
			"terminal, got %d chars", len(err.Error()))
	}
}

// --- the WIRED path, not just the parser ------------------------------------
//
// parsePublishReceipt being correct is not the same as sendEscalationSMS USING it.
// These drive the real function against a server that answers exactly as the live
// endpoint does, so a future refactor that stops checking the body fails here.

func TestSendRejectsA200ThatServedTheWebUI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK) // 200 — the old check's entire test
		_, _ = w.Write([]byte(webUIHTML))
	}))
	defer srv.Close()

	cfg := &config.EscalationConfig{}
	cfg.Contacts.HumanSMS = "someone"
	cfg.Contacts.SMSWebhook = srv.URL

	receipt, err := sendEscalationSMS(cfg, "bead-1", "high", "disk is full")
	if err == nil {
		t.Fatalf("a 200 serving the web UI was reported as a successful push (receipt %q)", receipt)
	}
}

func TestSendAcceptsARealReceiptAndReturnsIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("escalation must PUBLISH (POST), got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(realReceipt))
	}))
	defer srv.Close()

	cfg := &config.EscalationConfig{}
	cfg.Contacts.HumanSMS = "someone"
	cfg.Contacts.SMSWebhook = srv.URL

	receipt, err := sendEscalationSMS(cfg, "bead-1", "high", "disk is full")
	if err != nil {
		t.Fatalf("a real publish was reported as a failure: %v", err)
	}
	if receipt != "ops-alerts/NQ2oOnDcFWQr" {
		t.Fatalf("caller needs the receipt to print what was proven, got %q", receipt)
	}
}

func TestSendReportsFailureOnAuthRejection(t *testing.T) {
	// The plain negative: a bad token. Measured against the live endpoint, this is a
	// 401, so it was already caught — but it must stay caught.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":40101,"http":401,"error":"unauthorized"}`))
	}))
	defer srv.Close()

	cfg := &config.EscalationConfig{}
	cfg.Contacts.HumanSMS = "someone"
	cfg.Contacts.SMSWebhook = srv.URL

	if _, err := sendEscalationSMS(cfg, "bead-1", "high", "disk is full"); err == nil {
		t.Fatal("a 401 was reported as a successful push")
	}
}
