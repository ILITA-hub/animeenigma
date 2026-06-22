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
	os.Exit(m.Run())
}
