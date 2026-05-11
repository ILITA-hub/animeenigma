package domain

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// TestSentinelsAreNonNil verifies that each sentinel error is allocated.
// If any of these come back nil, calling errors.Is on them would panic at runtime
// and downstream provider implementations would silently misbehave.
func TestSentinelsAreNonNil(t *testing.T) {
	t.Parallel()
	if ErrNotFound == nil {
		t.Fatal("ErrNotFound is nil; sentinels must be allocated via errors.New")
	}
	if ErrProviderDown == nil {
		t.Fatal("ErrProviderDown is nil; sentinels must be allocated via errors.New")
	}
	if ErrExtractFailed == nil {
		t.Fatal("ErrExtractFailed is nil; sentinels must be allocated via errors.New")
	}
}

// TestSentinelsAreDistinct verifies that the three sentinels are pairwise distinct
// under errors.Is — i.e. distinguishing "not found" from "provider down" from
// "extract failed" works in orchestrator failover logic.
func TestSentinelsAreDistinct(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a, b error
	}{
		{"NotFound vs ProviderDown", ErrNotFound, ErrProviderDown},
		{"NotFound vs ExtractFailed", ErrNotFound, ErrExtractFailed},
		{"ProviderDown vs ExtractFailed", ErrProviderDown, ErrExtractFailed},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if errors.Is(tc.a, tc.b) {
				t.Errorf("errors.Is(%v, %v) = true; sentinels must be distinct", tc.a, tc.b)
			}
			if errors.Is(tc.b, tc.a) {
				t.Errorf("errors.Is(%v, %v) = true; sentinels must be distinct", tc.b, tc.a)
			}
		})
	}
}

// TestWrapNotFoundPreservesIs verifies the multi-%w wrap technique: errors.Is
// must match both the sentinel AND the underlying cause. This is what lets
// orchestrator code distinguish "kodik upstream returned 404" (NotFound + io.EOF
// cause) from "kodik upstream timed out" (ProviderDown + context.DeadlineExceeded).
func TestWrapNotFoundPreservesIs(t *testing.T) {
	t.Parallel()
	wrapped := WrapNotFound(io.ErrUnexpectedEOF, "context msg")
	if wrapped == nil {
		t.Fatal("WrapNotFound returned nil")
	}
	if !errors.Is(wrapped, ErrNotFound) {
		t.Errorf("errors.Is(wrapped, ErrNotFound) = false; sentinel not preserved through wrap")
	}
	if !errors.Is(wrapped, io.ErrUnexpectedEOF) {
		t.Errorf("errors.Is(wrapped, io.ErrUnexpectedEOF) = false; cause not preserved through wrap")
	}
	if !strings.Contains(wrapped.Error(), "context msg") {
		t.Errorf("wrapped.Error() = %q; expected to contain caller context %q", wrapped.Error(), "context msg")
	}
}

// TestWrapProviderDownPreservesIs mirrors TestWrapNotFoundPreservesIs for the
// ProviderDown sentinel.
func TestWrapProviderDownPreservesIs(t *testing.T) {
	t.Parallel()
	wrapped := WrapProviderDown(io.ErrClosedPipe, "upstream timeout")
	if !errors.Is(wrapped, ErrProviderDown) {
		t.Errorf("errors.Is(wrapped, ErrProviderDown) = false")
	}
	if !errors.Is(wrapped, io.ErrClosedPipe) {
		t.Errorf("errors.Is(wrapped, io.ErrClosedPipe) = false")
	}
}

// TestWrapExtractFailedPreservesIs mirrors the others for ExtractFailed.
func TestWrapExtractFailedPreservesIs(t *testing.T) {
	t.Parallel()
	wrapped := WrapExtractFailed(io.ErrShortBuffer, "decryption failed")
	if !errors.Is(wrapped, ErrExtractFailed) {
		t.Errorf("errors.Is(wrapped, ErrExtractFailed) = false")
	}
	if !errors.Is(wrapped, io.ErrShortBuffer) {
		t.Errorf("errors.Is(wrapped, io.ErrShortBuffer) = false")
	}
}

// TestErrorMessagesAreInformative catches accidental string drift. Downstream
// log parsers / Grafana dashboards may key on these substrings, so we lock them
// in at the unit-test layer.
func TestErrorMessagesAreInformative(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err      error
		substr   string
		sentinel string
	}{
		{ErrNotFound, "not found", "ErrNotFound"},
		{ErrProviderDown, "provider down", "ErrProviderDown"},
		{ErrExtractFailed, "extract failed", "ErrExtractFailed"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.sentinel, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(tc.err.Error(), tc.substr) {
				t.Errorf("%s.Error() = %q; expected substring %q", tc.sentinel, tc.err.Error(), tc.substr)
			}
		})
	}
}
