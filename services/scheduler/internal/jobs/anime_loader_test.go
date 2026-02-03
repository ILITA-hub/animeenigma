package jobs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiter(t *testing.T) {
	rl := &rateLimiter{
		tokens:     3,
		maxTokens:  3,
		lastRefill: time.Now(),
		interval:   time.Second,
	}

	// First 3 requests should be immediate
	start := time.Now()
	for i := 0; i < 3; i++ {
		rl.acquire()
	}
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 100*time.Millisecond, "first 3 requests should be immediate")
	assert.Equal(t, 0, rl.tokens, "tokens should be depleted")

	// 4th request should wait for refill
	start = time.Now()
	rl.acquire()
	elapsed = time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 800*time.Millisecond, "4th request should wait for refill")
	assert.Equal(t, 2, rl.tokens, "tokens should be refilled minus one")
}

func TestRateLimiter_RefillAfterInterval(t *testing.T) {
	rl := &rateLimiter{
		tokens:     0,
		maxTokens:  3,
		lastRefill: time.Now().Add(-2 * time.Second), // 2 seconds ago
		interval:   time.Second,
	}

	// Should refill immediately since interval has passed
	start := time.Now()
	rl.acquire()
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 100*time.Millisecond, "should refill immediately")
	assert.Equal(t, 2, rl.tokens, "tokens should be refilled minus one")
}
