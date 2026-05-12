package health

import "time"

// allowPrivateHostsForTest disables the SSRF host-allowlist check inside
// fetchSegment. REVIEW.md iter-2 WR-NEW-02: this helper replaces the
// previously-exported `WithAllowPrivateHosts` functional option. Because
// it lives in a `_test.go` file it is only linked into the test binary —
// non-test callers in any package cannot reach it.
//
// Tests that hit an httptest.Server (which binds to 127.0.0.1:randomport,
// rejected by the production guard from BLK-01) must call this helper
// AFTER constructing the runner:
//
//	r := NewProbeRunner(...)
//	allowPrivateHostsForTest(r)
//
// Production callers cannot opt out of the SSRF guard at all.
func allowPrivateHostsForTest(r *ProbeRunner) {
	r.allowPrivateHosts = true
}

// withComputeInitialDelayForTest injects a custom computeInitialDelay
// implementation. REVIEW.md iter-2 WR-NEW-01: the
// TestProbe_FatalPanicDoesNotRespawn regression drives the production
// outer defer-recover deterministically by making the injected function
// panic. The seam exists only in the test binary; production callers
// cannot opt in.
func withComputeInitialDelayForTest(fn func() time.Duration) ProbeOption {
	return func(r *ProbeRunner) { r.computeInitialDelayFn = fn }
}
