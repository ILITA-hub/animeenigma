package service

import (
	"testing"
	"time"
)

// ----------------------------------------------------------------------------
// Test 1 — first AllowSeek for a fresh user returns true (bucket starts
// pre-filled to burst capacity).
// ----------------------------------------------------------------------------

func TestRateLimit_AllowSeek_FirstCall_Allowed(t *testing.T) {
	rl := NewRateLimiter()
	if !rl.AllowSeek("alice") {
		t.Fatal("first AllowSeek should be true")
	}
}

// ----------------------------------------------------------------------------
// Test 2 — two AllowSeek calls back-to-back: second returns false because
// burst=1 was consumed by the first and the refill is 1/sec.
// ----------------------------------------------------------------------------

func TestRateLimit_AllowSeek_SecondCallWithin1s_Rejected(t *testing.T) {
	rl := NewRateLimiter()
	if !rl.AllowSeek("alice") {
		t.Fatal("first AllowSeek should be true")
	}
	if rl.AllowSeek("alice") {
		t.Fatal("second AllowSeek within 1s should be false (burst=1)")
	}
}

// ----------------------------------------------------------------------------
// Test 3 — after 1.1s, AllowSeek for the same user returns true again
// (the 1/sec refill has put one token back in the bucket).
// ----------------------------------------------------------------------------

func TestRateLimit_AllowSeek_AfterRefill_Allowed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 1.1s sleep in -short mode")
	}
	rl := NewRateLimiter()
	if !rl.AllowSeek("alice") {
		t.Fatal("first AllowSeek should be true")
	}
	if rl.AllowSeek("alice") {
		t.Fatal("second AllowSeek should be false")
	}
	time.Sleep(1100 * time.Millisecond)
	if !rl.AllowSeek("alice") {
		t.Fatal("AllowSeek after 1.1s refill should be true")
	}
}

// ----------------------------------------------------------------------------
// Test 4 — buckets are per-user; alice exhausting her bucket does not
// affect bob.
// ----------------------------------------------------------------------------

func TestRateLimit_AllowSeek_IndependentBuckets(t *testing.T) {
	rl := NewRateLimiter()
	_ = rl.AllowSeek("alice") // consume alice's
	if rl.AllowSeek("alice") {
		t.Fatal("alice's second AllowSeek should be false")
	}
	if !rl.AllowSeek("bob") {
		t.Fatal("bob's first AllowSeek should be true (independent bucket)")
	}
}

// ----------------------------------------------------------------------------
// Test 5 — chat: 5 calls in a row succeed (burst=5), 6th is rejected.
// Use a fresh user so the chat bucket is at full capacity.
// ----------------------------------------------------------------------------

func TestRateLimit_AllowChat_BurstThenReject(t *testing.T) {
	rl := NewRateLimiter()
	for i := 0; i < 5; i++ {
		if !rl.AllowChat("alice") {
			t.Fatalf("AllowChat #%d should be true (burst=5)", i+1)
		}
	}
	if rl.AllowChat("alice") {
		t.Fatal("AllowChat #6 within burst window should be false")
	}
}

// ----------------------------------------------------------------------------
// Test 6 — Forget(userID) drops both seek + chat buckets. After Forget,
// AllowSeek and AllowChat behave like a fresh user (new bucket at full
// burst capacity).
// ----------------------------------------------------------------------------

func TestRateLimit_Forget_ClearsBothBuckets(t *testing.T) {
	rl := NewRateLimiter()

	// Exhaust both buckets.
	if !rl.AllowSeek("alice") {
		t.Fatal("seek setup failed")
	}
	if rl.AllowSeek("alice") {
		t.Fatal("seek exhaustion setup failed")
	}
	for i := 0; i < 5; i++ {
		if !rl.AllowChat("alice") {
			t.Fatalf("chat setup #%d failed", i+1)
		}
	}
	if rl.AllowChat("alice") {
		t.Fatal("chat exhaustion setup failed")
	}

	// Forget — drops alice's state entirely.
	rl.Forget("alice")

	// Subsequent calls succeed (fresh buckets).
	if !rl.AllowSeek("alice") {
		t.Fatal("AllowSeek after Forget should be true (fresh bucket)")
	}
	if !rl.AllowChat("alice") {
		t.Fatal("AllowChat after Forget should be true (fresh bucket)")
	}
}

// ----------------------------------------------------------------------------
// Bonus — empty userID defensively allowed (defensive guard documented in
// ratelimit.go; the WS handler never actually passes "" but the limiter
// should fail-open if it ever did so a bug can't lock everyone out via
// a shared anonymous bucket).
// ----------------------------------------------------------------------------

func TestRateLimit_EmptyUserID_AlwaysAllowed(t *testing.T) {
	rl := NewRateLimiter()
	for i := 0; i < 10; i++ {
		if !rl.AllowSeek("") {
			t.Fatalf("AllowSeek(\"\") #%d should be true (defensive fail-open)", i+1)
		}
		if !rl.AllowChat("") {
			t.Fatalf("AllowChat(\"\") #%d should be true (defensive fail-open)", i+1)
		}
	}
}
