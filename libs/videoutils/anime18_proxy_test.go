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
