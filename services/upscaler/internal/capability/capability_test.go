package capability

// Test strategy for sync.Once:
//
// The public Init function uses a package-level sync.Once, which means in a
// normal test binary a second call to Init with a different secret would be
// silently ignored. To allow multi-secret testing without testify/mock or
// process isolation, we expose the unexported initWith(secret string) helper
// that always reinitialises the state (bypassing the once), and public Init
// wraps it through the once. Tests call initWith directly, which is fine
// because they live in the same package (white-box). Production code ONLY
// ever calls Init (the once-gated form).

import (
	"testing"
	"time"
)

const testSecret = "test-secret-capability-hmac"

// TestMintVerify covers the full happy path and all wrong-input rejection cases.
func TestMintVerify(t *testing.T) {
	t.Helper()

	// Use initWith to bypass sync.Once so we can set a fresh secret for this test.
	initWith(testSecret)

	// Fixed reference time so we can control expiry without sleeps.
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ttl := 15 * time.Minute

	handle, exp, sig := MintJobHandle("job-1", "segment-get", 0, ttl)
	if handle == "" || exp == "" || sig == "" {
		t.Fatalf("MintJobHandle returned empty triple: handle=%q exp=%q sig=%q", handle, exp, sig)
	}

	// Expected handle format.
	wantHandle := "job-1:segment-get:0"
	if handle != wantHandle {
		t.Errorf("MintJobHandle handle = %q, want %q", handle, wantHandle)
	}

	// Verify the real minted (exp, sig) using internal VerifyJobHandle with a
	// suitable "now" that is strictly before expiry.
	// We re-mint using a controlled now so we know exactly what exp looks like.
	_, exp2, sig2 := mintAt("job-1", "segment-get", 0, ttl, now)

	t.Run("valid_returns_true", func(t *testing.T) {
		ok := VerifyJobHandle("job-1", "segment-get", 0, exp2, sig2, now)
		if !ok {
			t.Error("VerifyJobHandle returned false for valid handle")
		}
	})

	t.Run("at_exp_boundary_valid", func(t *testing.T) {
		// At exactly the expiry unix second, the token is still valid
		// (condition: now.Unix() > expUnix, so equal is still valid).
		expTime := now.Add(ttl)
		ok := VerifyJobHandle("job-1", "segment-get", 0, exp2, sig2, expTime)
		if !ok {
			t.Error("VerifyJobHandle returned false at exact exp boundary (should still be valid)")
		}
	})

	t.Run("expired_returns_false", func(t *testing.T) {
		// One second past expiry.
		expiredNow := now.Add(ttl + time.Second)
		ok := VerifyJobHandle("job-1", "segment-get", 0, exp2, sig2, expiredNow)
		if ok {
			t.Error("VerifyJobHandle returned true for expired token")
		}
	})

	t.Run("tampered_sig_returns_false", func(t *testing.T) {
		tampered := sig2[:len(sig2)-1] + "x"
		ok := VerifyJobHandle("job-1", "segment-get", 0, exp2, tampered, now)
		if ok {
			t.Error("VerifyJobHandle returned true for tampered sig")
		}
	})

	t.Run("wrong_jobid_returns_false", func(t *testing.T) {
		ok := VerifyJobHandle("job-2", "segment-get", 0, exp2, sig2, now)
		if ok {
			t.Error("VerifyJobHandle returned true for wrong jobID")
		}
	})

	t.Run("wrong_operation_returns_false", func(t *testing.T) {
		ok := VerifyJobHandle("job-1", "segment-put", 0, exp2, sig2, now)
		if ok {
			t.Error("VerifyJobHandle returned true for wrong operation (segment-put vs segment-get)")
		}
	})

	t.Run("wrong_idx_returns_false", func(t *testing.T) {
		ok := VerifyJobHandle("job-1", "segment-get", 1, exp2, sig2, now)
		if ok {
			t.Error("VerifyJobHandle returned true for wrong idx (1 vs 0)")
		}
	})
}

// TestFailClosedWhenUnset verifies the package fails closed with an empty secret.
func TestFailClosedWhenUnset(t *testing.T) {
	// Override to empty — simulates no JOB_CAPABILITY_SECRET configured.
	initWith("")

	t.Run("enabled_is_false", func(t *testing.T) {
		if Enabled() {
			t.Error("Enabled() returned true when secret is empty")
		}
	})

	now := time.Now()
	handle, exp, sig := MintJobHandle("job-1", "segment-get", 0, 15*time.Minute)

	t.Run("mint_returns_empty_triple", func(t *testing.T) {
		if handle != "" || exp != "" || sig != "" {
			t.Errorf("MintJobHandle did not return empty triple: handle=%q exp=%q sig=%q", handle, exp, sig)
		}
	})

	t.Run("verify_returns_false", func(t *testing.T) {
		ok := VerifyJobHandle("job-1", "segment-get", 0, "9999999999", "deadbeef", now)
		if ok {
			t.Error("VerifyJobHandle returned true when secret is empty")
		}
	})

	// Restore a real secret so any subsequent tests in the same binary aren't broken.
	initWith(testSecret)
}
