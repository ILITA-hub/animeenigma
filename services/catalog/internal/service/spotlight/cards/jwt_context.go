// Package cards holds the per-card resolver implementations for the
// spotlight aggregator (workstream hero-spotlight v1.0 Phase 3).
//
// jwt_context.go provides typed-key helpers so login-only resolvers can read
// the caller's JWT from the request context. The Resolver interface
// (services/catalog/internal/service/spotlight/types.go) takes only
// (ctx, userID *string) — login-only resolvers need the JWT itself to
// forward it to player's /api/users/recs (which uses OptionalAuth to
// distinguish the trending row vs the personalized upNext row).
//
// Rather than widen the Resolver signature (which would break Phase 1's 4
// resolvers AND the aggregator fan-out), we stash the JWT on ctx via an
// unexported typed key. The spotlight handler attaches the JWT BEFORE
// invoking aggregator.Resolve (Plan 04 wires this); resolvers in this
// package read it via JWTFromContext.
package cards

import "context"

// jwtKey is the unexported context-value key for the caller's JWT. The empty
// struct gives a zero-allocation typed key that cannot collide with any other
// package's context keys.
type jwtKey struct{}

// ContextWithJWT returns a child ctx carrying jwt. jwt may be the empty string
// (anonymous caller) — JWTFromContext treats both "no key" and "empty string"
// uniformly as ok=false so resolvers cannot accidentally send
// "Authorization: Bearer " with no value.
func ContextWithJWT(ctx context.Context, jwt string) context.Context {
	return context.WithValue(ctx, jwtKey{}, jwt)
}

// JWTFromContext returns (jwt, ok). ok=false when no JWT was attached OR when
// the stored value is the empty string. Resolvers MUST treat ok=false as "no
// auth" and either skip the call (login-only resolver returns nil card) or
// fall back to the anonymous path (personal_pick anon path).
func JWTFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(jwtKey{}).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}
