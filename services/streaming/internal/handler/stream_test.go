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

	// No AllowedDomains wiring: the HLS trust gate is `preauth OR first-party
	// OR provenance-signed` (allowlist retired 2026-07-14), so an unsigned
	// external URL fails the gate with no further config.
	proxyCfg := videoutils.DefaultProxyConfig()

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

// TestStorageProxyWiring_S3HostNotFirstParty locks the SSRF-guard contract:
// FirstPartyHosts exempts ONLY Docker-private hosts (stealth-scraper, minio)
// from the dial-time private-IP + redirect checks. The external S3 host
// resolves public and passes the guarded dialer with no exemption — listing
// it would only strip DNS-rebind protection for that host. Presigning for it
// is unaffected: the MultiStorage still wraps both backends.
func TestStorageProxyWiring_S3HostNotFirstParty(t *testing.T) {
	newStorage := func(endpoint string, ssl bool) *videoutils.Storage {
		s, err := videoutils.NewStorage(videoutils.StorageConfig{
			Endpoint:        endpoint,
			AccessKeyID:     "fake",
			SecretAccessKey: "fake",
			UseSSL:          ssl,
			BucketName:      "raw-library",
			Region:          "us-east-1",
		})
		if err != nil {
			t.Fatalf("NewStorage(%q): %v", endpoint, err)
		}
		return s
	}
	minio := newStorage("minio:9000", false)
	s3 := newStorage("s3-fake.example", true)

	multi, firstParty := storageProxyWiring(minio, s3)

	has := func(host string) bool {
		for _, h := range firstParty {
			if h == host {
				return true
			}
		}
		return false
	}
	if !has("stealth-scraper") || !has("minio") {
		t.Errorf("FirstPartyHosts = %v; want it to contain stealth-scraper and minio", firstParty)
	}
	if has("s3-fake.example") {
		t.Errorf("FirstPartyHosts = %v; external S3 host must NOT be exempted from the SSRF dial guard", firstParty)
	}

	// Both backends must still be presign-routable via the MultiStorage.
	if !multi.IsOwnHost("http://minio:9000/raw-library/x.m3u8") ||
		!multi.IsOwnHost("https://s3-fake.example/raw-library/x.m3u8") {
		t.Errorf("MultiStorage must recognize both storage hosts as own; hosts = %v", multi.Hosts())
	}
}
