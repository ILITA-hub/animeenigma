package videoutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDomainAllowed_EmptyList(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: []string{},
	})

	assert.False(t, proxy.isDomainAllowed("example.com"), "empty AllowedDomains should fail-closed")
}

func TestIsDomainAllowed_NilList(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: nil,
	})

	assert.False(t, proxy.isDomainAllowed("example.com"), "nil AllowedDomains should fail-closed")
}

func TestIsDomainAllowed_ExactMatch(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: []string{"example.com", "trusted.org"},
	})

	assert.True(t, proxy.isDomainAllowed("example.com"))
	assert.True(t, proxy.isDomainAllowed("trusted.org"))
}

func TestIsDomainAllowed_SubdomainMatch(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: []string{"example.com"},
	})

	assert.True(t, proxy.isDomainAllowed("cdn.example.com"))
	assert.True(t, proxy.isDomainAllowed("a.b.c.example.com"))
}

func TestIsDomainAllowed_DisallowedDomain(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: []string{"example.com", "trusted.org"},
	})

	assert.False(t, proxy.isDomainAllowed("evil.com"))
	assert.False(t, proxy.isDomainAllowed("malicious.org"))
	assert.False(t, proxy.isDomainAllowed("notexample.com"), "should not match suffix without dot separator")
}

func TestIsDomainAllowed_CaseInsensitive(t *testing.T) {
	proxy := NewVideoProxy(ProxyConfig{
		AllowedDomains: []string{"Example.COM"},
	})

	assert.True(t, proxy.isDomainAllowed("example.com"))
	assert.True(t, proxy.isDomainAllowed("EXAMPLE.COM"))
	assert.True(t, proxy.isDomainAllowed("cdn.Example.Com"))
}

func TestIsHLSDomainAllowed_KnownDomains(t *testing.T) {
	knownDomains := []string{
		"megacloud.tv",
		"netmagcdn.com",
		"rapid-cloud.co",
		"jimaku.cc",
		"cdnlibs.org",
		"mcloud.to",
	}

	for _, domain := range knownDomains {
		assert.True(t, isHLSDomainAllowed(domain), "known domain %q should be allowed", domain)
	}
}

func TestIsHLSDomainAllowed_KnownSubdomains(t *testing.T) {
	assert.True(t, isHLSDomainAllowed("cdn.megacloud.tv"))
	assert.True(t, isHLSDomainAllowed("s1.netmagcdn.com"))
	assert.True(t, isHLSDomainAllowed("files.jimaku.cc"))
}

func TestIsHLSDomainAllowed_UnknownDomain(t *testing.T) {
	unknownDomains := []string{
		"evil.live",
		"random.pro",
		"hacker.xyz",
		"malware.com",
		"phishing.net",
	}

	for _, domain := range unknownDomains {
		assert.False(t, isHLSDomainAllowed(domain), "unknown domain %q should not be allowed", domain)
	}
}

func TestIsHLSDomainAllowed_PortStripping(t *testing.T) {
	assert.True(t, isHLSDomainAllowed("megacloud.tv:443"), "should strip port and match domain")
	assert.True(t, isHLSDomainAllowed("netmagcdn.com:8080"), "should strip port and match domain")
	assert.False(t, isHLSDomainAllowed("evil.com:443"), "should strip port but still reject unknown domain")
}
