// Package transport — synthetic Grafana webhook tests for the Phase 23
// Plan 23-03 self-healing dispatch path.
//
// These tests assert that:
//   1. A Pattern 6 payload (provider=gogoanime, server=vibeplayer, reason=ad_decoy)
//      unmarshals into domain.GrafanaWebhookPayload and the handler returns
//      202 Accepted with the labels intact on the submitAlert callback.
//   2. A Pattern 7 payload (provider=gogoanime, server=streamhg, reason=zero_match)
//      similarly round-trips.
//   3. A payload missing one of {provider, server, reason} still returns 202
//      (Grafana's contract) — documenting the gap so the dispatcher can decide
//      how to degrade gracefully.
//
// CRITICAL safety note: these tests use httptest.NewServer wrapping the same
// webhookHandler the production binary uses. The live maintenance container
// (port 8087 on the host) is NEVER hit by this test — even on a developer
// machine where it happens to be running. This is the documented mitigation
// for T-23-09 / T-23-10 in Phase 23 CONTEXT.md §risks.
package transport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
)

// syntheticAlert constructs a single-alert GrafanaWebhookPayload with the
// caller-supplied labels — used by all three Pattern tests below.
func syntheticAlert(alertname, severity, provider, server, reason string) domain.GrafanaWebhookPayload {
	labels := map[string]string{
		"alertname": alertname,
		"severity":  severity,
	}
	if provider != "" {
		labels["provider"] = provider
	}
	if server != "" {
		labels["server"] = server
	}
	if reason != "" {
		labels["reason"] = reason
	}
	return domain.GrafanaWebhookPayload{
		Receiver: "maintenance-webhook",
		Status:   "firing",
		Alerts: []domain.GrafanaWebhookAlert{
			{
				Status: "firing",
				Labels: labels,
				Annotations: map[string]string{
					"summary":     "synthetic: " + alertname,
					"description": "synthetic test payload for Phase 23 Plan 23-03 dispatch verification",
				},
				StartsAt: time.Now().UTC().Format(time.RFC3339),
				EndsAt:   "0001-01-01T00:00:00Z",
			},
		},
		GroupLabels:  map[string]string{"alertname": alertname},
		CommonLabels: labels,
	}
}

// postSyntheticAlert wraps webhookHandler in httptest.NewServer, posts the
// payload with valid BasicAuth, and returns (statusCode, captured-payload).
// The captured payload is read from a buffered channel that the submitAlert
// callback writes into; the test fails if nothing arrives within 1s (which
// would indicate the handler silently dropped the alert).
func postSyntheticAlert(t *testing.T, p domain.GrafanaWebhookPayload) (int, domain.GrafanaWebhookPayload) {
	t.Helper()

	captured := make(chan domain.GrafanaWebhookPayload, 1)
	submit := func(payload domain.GrafanaWebhookPayload) {
		captured <- payload
	}

	srv := httptest.NewServer(webhookHandler(submit, "testuser", "testpass"))
	t.Cleanup(srv.Close)

	body, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("testuser", "testpass")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	select {
	case got := <-captured:
		return resp.StatusCode, got
	case <-time.After(1 * time.Second):
		// 202 with no submit call would be a bug — but we still return what
		// we got so the caller can branch on the status code.
		return resp.StatusCode, domain.GrafanaWebhookPayload{}
	}
}

// TestWebhook_SyntheticPattern6_Accepted verifies that an ad-decoy alert
// (Pattern 6 in .claude/maintenance-prompt.md) round-trips through the
// webhook handler with all three dispatch labels preserved.
func TestWebhook_SyntheticPattern6_Accepted(t *testing.T) {
	t.Parallel()

	payload := syntheticAlert(
		"ScraperAdDecoySurge",
		"warning",
		"gogoanime",
		"vibeplayer",
		"ad_decoy",
	)

	status, got := postSyntheticAlert(t, payload)
	if status != http.StatusAccepted {
		t.Fatalf("status = %d; want %d (202)", status, http.StatusAccepted)
	}
	if len(got.Alerts) != 1 {
		t.Fatalf("got.Alerts length = %d; want 1", len(got.Alerts))
	}
	labels := got.Alerts[0].Labels
	if labels["provider"] != "gogoanime" {
		t.Errorf("labels.provider = %q; want %q", labels["provider"], "gogoanime")
	}
	if labels["server"] != "vibeplayer" {
		t.Errorf("labels.server = %q; want %q", labels["server"], "vibeplayer")
	}
	if labels["reason"] != "ad_decoy" {
		t.Errorf("labels.reason = %q; want %q", labels["reason"], "ad_decoy")
	}
	if labels["alertname"] != "ScraperAdDecoySurge" {
		t.Errorf("labels.alertname = %q; want %q", labels["alertname"], "ScraperAdDecoySurge")
	}
}

// TestWebhook_SyntheticPattern7_Accepted verifies that a zero-match alert
// (Pattern 7 in .claude/maintenance-prompt.md — schema drift / packed-JS
// rotation) round-trips through the webhook handler.
func TestWebhook_SyntheticPattern7_Accepted(t *testing.T) {
	t.Parallel()

	payload := syntheticAlert(
		"ScraperPlayabilityRegression",
		"warning",
		"gogoanime",
		"streamhg",
		"zero_match",
	)

	status, got := postSyntheticAlert(t, payload)
	if status != http.StatusAccepted {
		t.Fatalf("status = %d; want %d (202)", status, http.StatusAccepted)
	}
	if len(got.Alerts) != 1 {
		t.Fatalf("got.Alerts length = %d; want 1", len(got.Alerts))
	}
	labels := got.Alerts[0].Labels
	if labels["provider"] != "gogoanime" {
		t.Errorf("labels.provider = %q; want %q", labels["provider"], "gogoanime")
	}
	if labels["server"] != "streamhg" {
		t.Errorf("labels.server = %q; want %q", labels["server"], "streamhg")
	}
	if labels["reason"] != "zero_match" {
		t.Errorf("labels.reason = %q; want %q", labels["reason"], "zero_match")
	}
	if labels["alertname"] != "ScraperPlayabilityRegression" {
		t.Errorf("labels.alertname = %q; want %q", labels["alertname"], "ScraperPlayabilityRegression")
	}
}

// TestWebhook_RequiredLabels_PresentInDispatched documents the contract
// expected by the maintenance bot's reason-enum dispatch table — every
// dispatch-worthy alert MUST carry provider+server+reason.
//
// Negative test: a payload missing `server` still returns 202 (the webhook
// handler does not gate on label presence — that's Grafana's contract).
// The dispatcher downstream is expected to handle the missing label
// gracefully (degrade to escalate, not crash). This test is information-
// only; if the dispatcher does NOT handle missing labels gracefully, that's
// a downstream issue surfaced by a future test, not this one.
func TestWebhook_RequiredLabels_PresentInDispatched(t *testing.T) {
	t.Parallel()

	// Positive case — all labels present.
	full := syntheticAlert("ScraperUnplayableSpike", "critical", "gogoanime", "earnvids", "cdn_unreachable")
	status, got := postSyntheticAlert(t, full)
	if status != http.StatusAccepted {
		t.Fatalf("full payload status = %d; want 202", status)
	}
	for _, k := range []string{"provider", "server", "reason"} {
		if _, ok := got.Alerts[0].Labels[k]; !ok {
			t.Errorf("dispatched payload missing required label %q", k)
		}
	}

	// Negative case — `server` missing. Webhook still returns 202; the
	// dispatched payload simply lacks the label. Document, don't fail.
	partial := syntheticAlert("ScraperPlayabilityRegression", "warning", "gogoanime", "", "zero_match")
	status, got = postSyntheticAlert(t, partial)
	if status != http.StatusAccepted {
		t.Fatalf("partial payload status = %d; want 202 (handler does not gate on labels)", status)
	}
	if _, ok := got.Alerts[0].Labels["server"]; ok {
		t.Errorf("partial payload should NOT have server label; got %q", got.Alerts[0].Labels["server"])
	}
	if _, ok := got.Alerts[0].Labels["provider"]; !ok {
		t.Errorf("partial payload should still have provider label")
	}
}

// TestWebhook_AuthRejected_NoSubmit asserts that a payload posted without
// valid BasicAuth is rejected with 401 and the submitAlert callback is NOT
// invoked — the existing T-23-09 mitigation. Included here so any future
// refactor of webhookHandler that accidentally drops auth fails this test.
func TestWebhook_AuthRejected_NoSubmit(t *testing.T) {
	t.Parallel()

	captured := make(chan domain.GrafanaWebhookPayload, 1)
	submit := func(payload domain.GrafanaWebhookPayload) {
		captured <- payload
	}
	srv := httptest.NewServer(webhookHandler(submit, "testuser", "testpass"))
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(syntheticAlert("ScraperAdDecoySurge", "warning", "gogoanime", "vibeplayer", "ad_decoy"))
	req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No BasicAuth set.
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	select {
	case p := <-captured:
		t.Errorf("submit was called despite auth failure; received payload %+v", p)
	case <-time.After(200 * time.Millisecond):
		// Expected — submit not invoked.
	}
}
