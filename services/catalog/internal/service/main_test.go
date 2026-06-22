package service

import (
	"os"
	"testing"
)

// TestMain enables HLS-proxy provenance signing for the whole service test
// package. videoutils.SignStreamURL loads STREAM_TOKEN_SECRET lazily (once per
// process) and is DISABLED when unset — without this, library/ae streams come
// back with empty exp/sig and the raw-resolver signing assertions can't run.
// Mirrors libs/videoutils/videoutils_main_test.go.
func TestMain(m *testing.M) {
	if os.Getenv("STREAM_TOKEN_SECRET") == "" {
		_ = os.Setenv("STREAM_TOKEN_SECRET", "test-provenance-secret-0123456789abcdef")
	}
	os.Exit(m.Run())
}
