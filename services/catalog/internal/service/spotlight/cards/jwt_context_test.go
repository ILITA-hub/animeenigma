// Workstream hero-spotlight v1.0 Phase 3 — Plan 02 Task 1.
//
// JWT context propagation tests. The Resolver interface (spotlight/types.go)
// takes only (ctx, userID *string); login-only resolvers need the JWT itself
// to forward it to player's /api/users/recs. We stash the JWT on ctx via a
// typed key so the Resolver interface signature stays unchanged.

package cards

import (
	"context"
	"testing"
)

func TestContextWithJWT_RoundTrip(t *testing.T) {
	ctx := ContextWithJWT(context.Background(), "abc")
	got, ok := JWTFromContext(ctx)
	if !ok {
		t.Fatal("expected ok=true for stored jwt, got false")
	}
	if got != "abc" {
		t.Fatalf("expected jwt=abc, got %q", got)
	}
}

func TestJWTFromContext_NoKey_ReturnsFalse(t *testing.T) {
	got, ok := JWTFromContext(context.Background())
	if ok {
		t.Fatalf("expected ok=false on empty ctx, got ok=true (jwt=%q)", got)
	}
	if got != "" {
		t.Fatalf("expected empty jwt on miss, got %q", got)
	}
}

func TestJWTFromContext_EmptyString_ReturnsFalse(t *testing.T) {
	// Empty string is treated as "no auth" — callers must not see ok=true with
	// an empty token (which would then be sent as "Authorization: Bearer "
	// in player_client.GO and confuse player's OptionalAuth).
	ctx := ContextWithJWT(context.Background(), "")
	got, ok := JWTFromContext(ctx)
	if ok {
		t.Fatalf("expected ok=false for empty-string jwt, got ok=true (jwt=%q)", got)
	}
	if got != "" {
		t.Fatalf("expected empty jwt on empty-stored, got %q", got)
	}
}

func TestJWTFromContext_DistinctKeysDoNotCollide(t *testing.T) {
	// Sanity-check that ContextWithJWT uses an unexported typed key — adding
	// an unrelated string value to ctx must not collide. (The typed key is
	// already a compile-time guarantee; this test exists to document the
	// invariant.)
	type otherKey struct{}
	ctx := context.WithValue(context.Background(), otherKey{}, "decoy")
	ctx = ContextWithJWT(ctx, "real-jwt")

	got, ok := JWTFromContext(ctx)
	if !ok || got != "real-jwt" {
		t.Fatalf("expected real-jwt found, got (%q, ok=%v)", got, ok)
	}
}
