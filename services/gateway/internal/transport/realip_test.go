package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func captureRemoteAddr(t *testing.T, headers map[string]string, peer string) string {
	t.Helper()
	var got string
	h := RealClientIP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.RemoteAddr
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/anime", nil)
	req.RemoteAddr = peer
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

// The spoofable headers (True-Client-IP, X-Forwarded-For) must be IGNORED; only
// the edge-set X-Real-IP determines the client IP the rate limiter/poison/logs
// key on.
func TestRealClientIP_IgnoresSpoofableHeaders(t *testing.T) {
	got := captureRemoteAddr(t, map[string]string{
		"True-Client-IP":  "1.2.3.4",     // attacker-supplied, nginx never clears
		"X-Forwarded-For": "5.6.7.8, 10.0.0.2", // attacker value lands first (nginx appends)
		"X-Real-IP":       "203.0.113.9", // nginx-set true peer
	}, "10.0.0.2:5555")
	if got != "203.0.113.9" {
		t.Fatalf("RemoteAddr = %q; want the trusted X-Real-IP 203.0.113.9 (spoofable headers must be ignored)", got)
	}
}

// With no X-Real-IP we must fall back to the real TCP peer, never to a
// spoofable header.
func TestRealClientIP_FallsBackToPeerWithoutXRealIP(t *testing.T) {
	got := captureRemoteAddr(t, map[string]string{
		"True-Client-IP":  "1.2.3.4",
		"X-Forwarded-For": "5.6.7.8",
	}, "198.51.100.7:4444")
	if got != "198.51.100.7:4444" {
		t.Fatalf("RemoteAddr = %q; want the TCP peer 198.51.100.7:4444 when X-Real-IP is absent", got)
	}
}

// A malformed X-Real-IP must not clobber the real peer.
func TestRealClientIP_IgnoresMalformedXRealIP(t *testing.T) {
	got := captureRemoteAddr(t, map[string]string{"X-Real-IP": "not-an-ip"}, "198.51.100.7:4444")
	if got != "198.51.100.7:4444" {
		t.Fatalf("RemoteAddr = %q; want the TCP peer when X-Real-IP is malformed", got)
	}
}
