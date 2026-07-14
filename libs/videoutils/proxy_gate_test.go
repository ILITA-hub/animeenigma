package videoutils

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests lock the HLS proxy's trust gate after the 2026-07-14 allowlist
// retirement (docs/plans/2026-07-14-retire-allowlist-blocklist.md, S3):
//
//	admit ⇔ preauth (sealed stream token) OR first-party internal host OR
//	        valid provenance signature
//
// No static external-domain list exists anymore — an unsigned external host
// must be rejected with *DomainNotAllowedError BEFORE any upstream dial.

// gateFixture serves a tiny payload and returns the server plus its sourceURL.
func gateFixture(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write([]byte("segment-bytes"))
	}))
	t.Cleanup(srv.Close)
	return srv, srv.URL + "/seg-1.ts"
}

// TestTrustGate_UnsignedExternalRejected: an external host with no provenance
// signature (and no preauth) is rejected with *DomainNotAllowedError before
// the proxy dials anything — there is no allowlist to fall back to.
func TestTrustGate_UnsignedExternalRejected(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
	rec := httptest.NewRecorder()
	// Formerly-allowlisted hosts included: with the list retired they carry no
	// inherent trust, exactly like any other external domain.
	for _, sourceURL := range []string{
		"https://external-cdn.example.com/master.m3u8",
		"https://cloud.solodcdn.com/useruploads/x/720.mp4:hls:manifest.m3u8",
		"https://a4.mp4upload.com:183/video.mp4",
		"https://files.jimaku.cc/sub.ass",
	} {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL), nil)
		_, _, err := proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req)
		require.Error(t, err, "unsigned external URL %s must be rejected", sourceURL)
		var dna *DomainNotAllowedError
		assert.True(t, errors.As(err, &dna),
			"unsigned external URL %s must fail the gate as DomainNotAllowedError, got %v", sourceURL, err)
	}
}

// TestTrustGate_SignedPasses: a valid provenance signature admits an
// otherwise-unknown host through the gate.
func TestTrustGate_SignedPasses(t *testing.T) {
	_, sourceURL := gateFixture(t)
	exp, sig := signProvenance(sourceURL, time.Now())
	require.NotEmpty(t, sig, "test secret must mint a provenance token")

	proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL)+"&exp="+exp+"&sig="+sig, nil)

	_, _, err := proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req)
	assert.NoError(t, err, "signed URL must pass the trust gate")
	assert.Equal(t, "segment-bytes", rec.Body.String(), "signed fetch must proxy the upstream body")

	// A garbled signature must NOT pass — signing is the authorization.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet,
		"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL)+"&exp="+exp+"&sig=deadbeefdeadbeefdeadbeefdeadbeef", nil)
	_, _, err = proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec2, req2)
	var dna *DomainNotAllowedError
	assert.True(t, errors.As(err, &dna), "a forged signature must fail the gate, got %v", err)
}

// TestTrustGate_FirstPartyUnsignedPasses: a host in the configured
// FirstPartyHosts set (stealth-scraper, minio in production) passes the gate
// with NO signature — it resolves Docker-private, so SignStreamURL's
// netguard.ValidatePublicURL rejects it by design and it can never be
// publicly signed. Matching mirrors firstPartyAddr: exact host,
// case-insensitive, port-stripped.
func TestTrustGate_FirstPartyUnsignedPasses(t *testing.T) {
	_, sourceURL := gateFixture(t) // http://127.0.0.1:<port>/seg-1.ts
	parsed, err := url.Parse(sourceURL)
	require.NoError(t, err)

	proxy := NewVideoProxy(ProxyConfig{
		UserAgent:       "test-agent",
		FirstPartyHosts: []string{parsed.Hostname()}, // "127.0.0.1", port-stripped like "minio" vs minio:9000
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL), nil)

	_, _, err = proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req)
	assert.NoError(t, err, "unsigned first-party host must pass the trust gate")
	assert.Equal(t, "segment-bytes", rec.Body.String())

	// The same unsigned URL WITHOUT the first-party config is rejected — the
	// exemption is the configured set, not the host's dialability.
	bare := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
	rec2 := httptest.NewRecorder()
	_, _, err = bare.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec2, req)
	var dna *DomainNotAllowedError
	assert.True(t, errors.As(err, &dna),
		"unsigned host outside FirstPartyHosts must fail the gate, got %v", err)
}

// TestTrustGate_PreauthPasses: ProxyPreauthCounted skips the gate entirely —
// the caller already authorized the URL by opening a sealed AES-GCM stream
// token, so neither a signature nor first-party membership is needed.
func TestTrustGate_PreauthPasses(t *testing.T) {
	_, sourceURL := gateFixture(t)

	proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL), nil)

	_, _, err := proxy.ProxyPreauthCounted(context.Background(), sourceURL, "", rec, req)
	assert.NoError(t, err, "preauth call must skip the trust gate")
	assert.Equal(t, "segment-bytes", rec.Body.String())
}
