package videoutils

import (
	"strconv"
	"testing"
	"time"
)

// TestProvenance_RefusesPrivateAndExoticScheme locks finding #65: the proxy must
// not mint — and must not honor — a provenance token for a URL whose IP-literal
// host is private/loopback/link-local or whose scheme is not http/https. A
// compromised allow-listed CDN must not be able to self-mint authorization for
// http://169.254.169.254/… or http://127.0.0.1/….
func TestProvenance_RefusesPrivateAndExoticScheme(t *testing.T) {
	// This test exercises the guard itself, so the package-wide loopback relaxation
	// (TestMain) must be off here.
	allowLoopbackForTest = false
	t.Cleanup(func() { allowLoopbackForTest = true })

	now := time.Now()

	bad := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1/seg.ts",
		"https://10.0.0.5/seg.ts",
		"http://[::1]/seg.ts",
		"file:///etc/passwd",
		"gopher://internal/x",
	}
	for _, raw := range bad {
		if exp, sig := signProvenance(raw, now); exp != "" || sig != "" {
			t.Errorf("signProvenance(%q) must mint nothing, got exp=%q sig=%q", raw, exp, sig)
		}
		// A correctly-computed MAC must still be rejected at verify time.
		expStr := strconv.FormatInt(now.Add(provenanceTTL).Unix(), 10)
		forged := provenanceMAC(raw, expStr)
		if validProvenanceToken(raw, expStr, forged, now) {
			t.Errorf("validProvenanceToken(%q) must reject a private/exotic URL even with a valid MAC", raw)
		}
	}

	// Legit public CDNs and first-party internal HOSTNAMES still mint + verify
	// (hostnames are not IP literals, so the cheap guard passes them).
	good := []string{
		"https://cdn.mewstream.buzz/seg.ts",
		"http://minio:9000/bucket/seg.ts",
		"http://stealth-scraper:3000/hls/x.ts",
		"https://www.mp4upload.com/x",
	}
	for _, raw := range good {
		exp, sig := signProvenance(raw, now)
		if exp == "" || sig == "" {
			t.Errorf("signProvenance(%q) must mint a token for a legit URL", raw)
			continue
		}
		if !validProvenanceToken(raw, exp, sig, now) {
			t.Errorf("validProvenanceToken(%q) must accept its own freshly minted token", raw)
		}
	}
}
