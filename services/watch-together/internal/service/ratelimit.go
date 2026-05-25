// Package service — ratelimit.go is the per-user token-bucket rate limiter
// for inbound WebSocket messages.
//
// Scope (WT-NF-02):
//
//	playback:seek  → 1/sec/user, burst 1   (rate.Every(time.Second), 1)
//	chat:message   → 5/sec/user, burst 5   (rate.Limit(5), 5)
//
// In-process only (single-instance v1.0). Multi-instance scale-out is deferred
// to v2; when that lands, the obvious upgrade is to lift this onto Redis
// (similar to gateway's user_rate_limit which uses redis_rate). The interface
// (AllowSeek/AllowChat/Forget) is small enough that the swap is a one-file
// change in 01.6.3's InboundRouter.
//
// Per-user identification: the user_id from the JWT claim attached to the WS
// connection. A multi-tab user (same JWT subject across two browser tabs)
// shares a single bucket — abusive seek-spam from one tab can't be hidden
// by opening another tab.
//
// Garbage collection: the InboundRouter calls Forget(userID) from the WS
// OnClose hook in 01.6.3 wiring. This drops the bucket entry so a long-running
// service doesn't accumulate unbounded state. A user who reconnects gets a
// fresh bucket (intentional — disconnect is a clean break for rate-limit
// purposes).
package service

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// seekRate is the seek limiter's refill rate — one token per second.
// rate.Every(time.Second) is the canonical "1/sec" expression and avoids
// the floating-point tax of rate.Limit(1.0).
var seekRate = rate.Every(time.Second)

// seekBurst is the seek limiter's capacity (and starting tokens). Setting
// burst=1 means a user can seek immediately on connect, but the next seek
// has to wait a full second.
const seekBurst = 1

// chatRate is the chat limiter's refill rate — five tokens per second.
// rate.Limit(5) creates a steady stream so a user can hold a sustainable
// 5 msg/s rate without ever being rejected.
const chatRate = rate.Limit(5)

// chatBurst is the chat limiter's capacity. Setting burst=5 lets a user
// fire 5 messages back-to-back at connect time (e.g. quickly typing
// "hi", "hello", "anyone here?", "?", "??") then enforces 5/s for
// sustained traffic.
const chatBurst = 5

// RateLimiter holds a per-user token bucket for each rate-limited inbound
// message type. The two maps are separate so a chatty user can't exhaust
// their seek budget by sending chat messages, and vice versa.
//
// Concurrency: mu protects both maps. AllowSeek / AllowChat / Forget are
// all O(1) with one bucket lookup or insert. The rate.Limiter values
// themselves are individually safe for concurrent use, so once we've
// fetched the pointer under the lock we can release it before calling
// Allow().
type RateLimiter struct {
	mu   sync.Mutex
	seek map[string]*rate.Limiter
	chat map[string]*rate.Limiter
}

// NewRateLimiter constructs an empty limiter. The maps are lazily populated
// on first AllowSeek / AllowChat per user.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		seek: make(map[string]*rate.Limiter),
		chat: make(map[string]*rate.Limiter),
	}
}

// AllowSeek returns true if userID has tokens available for a seek; false
// if rate-limited. The first call for a user always succeeds (burst=1 →
// starts with 1 token); the second call within 1 second is rejected.
//
// Concurrency: the lookup-or-create is gated by mu; the actual Allow()
// call happens outside the critical section so two concurrent AllowSeek
// calls for the same user are evaluated against the same bucket without
// holding mu for the duration of the bucket's internal time arithmetic.
func (r *RateLimiter) AllowSeek(userID string) bool {
	if userID == "" {
		// Defensive — the WS handler always provides a userID from JWT
		// claims, but if a future caller passes "" we don't want to
		// share a global anonymous bucket.
		return true
	}
	r.mu.Lock()
	limiter, ok := r.seek[userID]
	if !ok {
		limiter = rate.NewLimiter(seekRate, seekBurst)
		r.seek[userID] = limiter
	}
	r.mu.Unlock()
	return limiter.Allow()
}

// AllowChat returns true if userID has tokens available for a chat message;
// false if rate-limited. Burst=5 lets a normal user fire several messages
// in a row without hitting the limit; sustained > 5/s gets rejected.
func (r *RateLimiter) AllowChat(userID string) bool {
	if userID == "" {
		return true
	}
	r.mu.Lock()
	limiter, ok := r.chat[userID]
	if !ok {
		limiter = rate.NewLimiter(chatRate, chatBurst)
		r.chat[userID] = limiter
	}
	r.mu.Unlock()
	return limiter.Allow()
}

// Forget drops both buckets for userID. Called from the WS OnClose hook
// in 01.6.3 wiring so a disconnected user's state doesn't linger forever.
// No-op if userID has no buckets yet.
//
// A reconnecting user gets a fresh bucket — this is intentional (a
// disconnect is a clean break for limiting purposes; abusing reconnect to
// reset the limit is a non-issue because each connect involves an HTTP
// upgrade through the gateway's IP-level rate limit).
func (r *RateLimiter) Forget(userID string) {
	if userID == "" {
		return
	}
	r.mu.Lock()
	delete(r.seek, userID)
	delete(r.chat, userID)
	r.mu.Unlock()
}
