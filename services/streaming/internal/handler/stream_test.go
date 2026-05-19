package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
)

// TestHLSProxy_DomainNotAllowed_Returns502 locks the Phase 25 SCRAPER-HEAL-24
// contract: when ProxyWithReferer returns *videoutils.DomainNotAllowedError,
// the HLS proxy handler must emit HTTP 502 with a non-empty descriptive body.
// Previously this code path fell through to a generic log line and Go's
// http package defaulted to 200 OK / Content-Length:0 — the silent-200 bug
// the audit's W-INT-03 finding flagged.
func TestHLSProxy_DomainNotAllowed_Returns502(t *testing.T) {
	log, err := logger.New(logger.Config{Level: "error", Encoding: "console"})
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}

	proxyCfg := videoutils.DefaultProxyConfig()
	proxyCfg.AllowedDomains = videoutils.HLSProxyAllowedDomains

	h := &StreamHandler{
		streamingService: nil, // HLSProxy path does not use streamingService
		videoProxy:       videoutils.NewVideoProxy(proxyCfg),
		log:              log,
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/?url=https%3A%2F%2Fdefinitely-not-allowed-domain.invalid%2Fmaster.m3u8",
		nil,
	)
	rec := httptest.NewRecorder()

	h.HLSProxy(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d; want %d (BadGateway)", rec.Code, http.StatusBadGateway)
	}

	body := rec.Body.String()
	if !strings.Contains(strings.ToLower(body), "domain not allowed") {
		t.Errorf("body = %q; want substring 'domain not allowed'", body)
	}

	if len(body) == 0 {
		t.Errorf("body is empty; the silent-200 bug emitted Content-Length:0 — we want a real error message")
	}
}
