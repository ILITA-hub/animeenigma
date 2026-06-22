package netguard

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPrivateIP(t *testing.T) {
	cases := []struct {
		ip      string
		private bool
	}{
		// loopback
		{"127.0.0.1", true},
		{"127.5.5.5", true},
		{"::1", true},
		// RFC1918 private
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		// link-local (incl. cloud metadata 169.254.169.254)
		{"169.254.169.254", true},
		{"169.254.0.1", true},
		{"fe80::1", true},
		// IPv6 unique-local
		{"fc00::1", true},
		{"fd12:3456::1", true},
		// CGNAT 100.64.0.0/10 (RFC6598)
		{"100.64.0.1", true},
		{"100.127.255.255", true},
		// unspecified
		{"0.0.0.0", true},
		{"::", true},
		// multicast
		{"224.0.0.1", true},
		{"ff02::1", true},
		// IPv4-mapped IPv6 private
		{"::ffff:10.0.0.1", true},
		{"::ffff:127.0.0.1", true},
		// public — must NOT be flagged
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"172.32.0.1", false},  // just outside 172.16/12
		{"100.128.0.1", false}, // just outside 100.64/10
		{"93.184.216.34", false},
		{"2606:2800:220:1:248:1893:25c8:1946", false},
	}
	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		require.NotNil(t, ip, "parse %s", tc.ip)
		assert.Equal(t, tc.private, IsPrivateIP(ip), "IsPrivateIP(%s)", tc.ip)
	}
	// nil is treated as private (fail closed)
	assert.True(t, IsPrivateIP(nil))
}

func TestDenyPrivateControl(t *testing.T) {
	// post-DNS ip:port that the dialer is about to connect to
	assert.Error(t, DenyPrivateControl("tcp4", "169.254.169.254:80", nil), "metadata IP must be blocked")
	assert.Error(t, DenyPrivateControl("tcp4", "127.0.0.1:8080", nil), "loopback must be blocked")
	assert.Error(t, DenyPrivateControl("tcp4", "10.1.2.3:443", nil), "private must be blocked")
	assert.NoError(t, DenyPrivateControl("tcp4", "8.8.8.8:443", nil), "public must pass")
	assert.NoError(t, DenyPrivateControl("tcp6", "[2606:2800:220:1:248:1893:25c8:1946]:443", nil), "public v6 must pass")
	assert.Error(t, DenyPrivateControl("tcp4", "not-an-addr", nil), "unparseable must error")
}

func TestValidatePublicURL(t *testing.T) {
	ok := []string{
		"https://example.com/x.jpg",
		"http://cdn.example.org/a/b.png",
		"https://images.example.com:8443/p.webp",
		"http://minio:9000/bucket/k", // hostname (not IP) passes the cheap pre-flight
	}
	for _, u := range ok {
		assert.NoError(t, ValidatePublicURL(u), "should allow %s", u)
	}
	bad := []string{
		"ftp://example.com/x",                     // scheme
		"file:///etc/passwd",                      // scheme
		"gopher://example.com",                    // scheme
		"http://127.0.0.1/x",                      // loopback IP literal
		"http://169.254.169.254/latest/meta-data", // metadata IP literal
		"https://10.0.0.5/x",                      // private IP literal
		"http://[::1]/x",                          // v6 loopback literal
		"https:///nohostonly",                     // empty host
		"not a url at all ::::",                   // unparseable / empty scheme
	}
	for _, u := range bad {
		assert.Error(t, ValidatePublicURL(u), "should reject %s", u)
	}
}

func TestHostIsFirstParty(t *testing.T) {
	allow := []string{"minio", "stealth-scraper", "minio.internal"}
	assert.True(t, HostIsFirstParty("minio", allow))
	assert.True(t, HostIsFirstParty("MINIO", allow), "case-insensitive")
	assert.True(t, HostIsFirstParty("stealth-scraper", allow))
	assert.True(t, HostIsFirstParty("api.minio.internal", allow), "strict subdomain")
	assert.True(t, HostIsFirstParty("minio.", allow), "trailing dot tolerated")
	assert.False(t, HostIsFirstParty("minio.evil.com", allow), "suffix-only must not match")
	assert.False(t, HostIsFirstParty("notminio", allow))
	assert.False(t, HostIsFirstParty("evil.com", allow))
	assert.False(t, HostIsFirstParty("minio2", allow))
}
