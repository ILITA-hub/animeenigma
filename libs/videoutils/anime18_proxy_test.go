package videoutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAnime18DomainsAllowed locks in the HLS-proxy allowlist entries the
// 18anime (18+) player depends on. isHLSDomainAllowed is the package-level
// checker ProxyWithReferer uses; it strips the port before matching, so
// mp4upload's :183 stream host resolves too.
func TestAnime18DomainsAllowed(t *testing.T) {
	for _, host := range []string{
		"a4.mp4upload.com",       // mp4upload progressive MP4 CDN
		"a4.mp4upload.com:183",   // ...with the :183 port the spike observed
		"www.mp4upload.com",      // embed host
		"cdn4.turboviplay.com",   // turbovid master m3u8 host
		"g276.turbosplayer.com",  // turbovid nested variant / segment host
	} {
		assert.True(t, isHLSDomainAllowed(host), "expected %s to be allowed", host)
	}

	// Negative control — a lookalike must NOT match.
	assert.False(t, isHLSDomainAllowed("evil-mp4upload.com.attacker.net"),
		"suffix-spoof host must not be allowed")
}

// TestHanimeWildcard_AnchoredToTLD locks in the SSRF fix for the Hanime
// prefix-wildcard entries (htv-*.com / hydaelyn-*.top / zodiark-*.top): legit
// rotating CDNs still match, but the old unanchored "htv-*" bypass — any host
// starting with the prefix on ANY TLD — is now rejected.
func TestHanimeWildcard_AnchoredToTLD(t *testing.T) {
	for _, host := range []string{
		"htv-belias.com",        // htv-*.com leftmost label
		"htv-hydaelyn.com",      // htv-*.com
		"p34.htv-hydaelyn.com",  // prefix after a dot, still .com
		"hydaelyn-25x-07.top",   // hydaelyn-*.top
		"zodiark-25x-03.top",    // zodiark-*.top
	} {
		assert.True(t, isHLSDomainAllowed(host), "legit Hanime CDN %s should be allowed", host)
	}

	for _, host := range []string{
		"htv-attacker.evil.io",  // right prefix, wrong TLD
		"htv-x.attacker.net",    // right prefix, wrong TLD
		"sub.htv-evil.ru",       // prefix after dot, wrong TLD
		"hydaelyn-evil.com",     // hydaelyn family is anchored to .top, so .com is rejected
		"zodiark-evil.net",      // wrong TLD for zodiark
	} {
		assert.False(t, isHLSDomainAllowed(host),
			"off-TLD lookalike %s must be rejected (anchor closes the old prefix bypass)", host)
	}
}
