package videoutils

import (
	"os"
	"testing"
)

// TestMain provides a provenance signing secret for the whole package test
// run. loadProvenanceSecret now fails closed (token mechanism disabled) when
// neither STREAM_TOKEN_SECRET nor JWT_SECRET is set, so the existing token
// round-trip tests — which don't configure their own secret — would otherwise
// exercise the disabled path. Production always sets STREAM_TOKEN_SECRET.
func TestMain(m *testing.M) {
	if os.Getenv("STREAM_TOKEN_SECRET") == "" && os.Getenv("JWT_SECRET") == "" {
		_ = os.Setenv("STREAM_TOKEN_SECRET", "test-provenance-secret-0123456789abcdef")
	}
	// The whole suite signs and fetches httptest fixtures on 127.0.0.1, which the
	// SSRF guards (provenance URL check + dial-time private-IP block, findings
	// #64/#65) would otherwise reject. Relax them package-wide; the guard's own
	// tests flip this back off for their cases. Tests in this package run serially
	// (no t.Parallel), so the shared flag is safe.
	allowLoopbackForTest = true
	os.Exit(m.Run())
}
