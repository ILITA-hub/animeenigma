package tracing

import (
	"context"

	"go.opentelemetry.io/otel/baggage"
)

// Baggage member keys. Only origin/operation ride W3C baggage — they are safe
// to propagate to downstream services (and, by W3C semantics, potentially to
// 3rd-party hosts on outbound requests). user_id MUST NOT go here; it rides a
// private non-propagated ctx value (WithUserID) instead — see the security
// note on userIDKey below (T-02-PII).
const (
	baggageKeyOrigin    = "origin"
	baggageKeyOperation = "operation"
)

// SeedBaggage returns a ctx whose W3C baggage carries origin + operation. Empty
// values are skipped (baggage.NewMemberRaw rejects empty values, and an empty
// dimension is meaningless), and a per-member error never aborts the whole set —
// a malformed value just omits that one member.
func SeedBaggage(ctx context.Context, origin, operation string) context.Context {
	var members []baggage.Member
	if origin != "" {
		if m, err := baggage.NewMemberRaw(baggageKeyOrigin, origin); err == nil {
			members = append(members, m)
		}
	}
	if operation != "" {
		if m, err := baggage.NewMemberRaw(baggageKeyOperation, operation); err == nil {
			members = append(members, m)
		}
	}
	if len(members) == 0 {
		return ctx
	}

	// Merge onto any baggage already in ctx so we don't clobber upstream members.
	bg := baggage.FromContext(ctx)
	for _, m := range members {
		if next, err := bg.SetMember(m); err == nil {
			bg = next
		}
	}
	return baggage.ContextWithBaggage(ctx, bg)
}

// ReadBaggage reads back the origin + operation dimensions seeded above. Missing
// members read as empty strings.
//
// The operation may have been seeded LAZILY by SeedMiddleware (chi resolves the
// route pattern only after the Use-middleware runs). When wire baggage carries
// no operation but a lazy resolver is present and the route pattern is now
// available, ReadBaggage resolves and returns it. This keeps callers — the
// handler and the recording RoundTripper — agnostic to the seeding timing.
func ReadBaggage(ctx context.Context) (origin, operation string) {
	bg := baggage.FromContext(ctx)
	origin = bg.Member(baggageKeyOrigin).Value()
	operation = bg.Member(baggageKeyOperation).Value()
	if operation == "" {
		operation = resolveOperation(ctx)
	}
	return origin, operation
}

// operationResolver carries the data needed to build the coarse operation label
// once chi has resolved the route pattern. routePattern is read lazily because a
// chi Use-middleware runs before the route match completes.
type operationResolver struct {
	service string
	method  string
	rc      patternProvider
}

// patternProvider is the minimal surface of *chi.Context ReadBaggage needs,
// kept as an interface so baggage.go takes no chi dependency.
type patternProvider interface {
	RoutePattern() string
}

type operationResolverKeyType struct{}

var operationResolverKey operationResolverKeyType

// withOperationResolver stashes the lazy operation resolver on ctx.
func withOperationResolver(ctx context.Context, service, method string, rc patternProvider) context.Context {
	return context.WithValue(ctx, operationResolverKey, operationResolver{service: service, method: method, rc: rc})
}

// resolveOperation builds "service METHOD routePattern" from a stashed resolver,
// or "" when none is present. Falls back to "service METHOD" if the pattern is
// not yet resolved.
func resolveOperation(ctx context.Context) string {
	r, ok := ctx.Value(operationResolverKey).(operationResolver)
	if !ok {
		return ""
	}
	op := r.service + " " + r.method
	if r.rc != nil {
		if p := r.rc.RoutePattern(); p != "" {
			op += " " + p
		}
	}
	return op
}

// userIDKeyType is an unexported context-key type so the user_id ctx value can
// never collide with another package's key and is unreachable outside this
// package except via UserIDFromContext.
type userIDKeyType struct{}

var userIDKey userIDKeyType

// WithUserID stashes user_id on a PRIVATE, non-propagated ctx value. This is the
// "sharpest finding" of the RESEARCH security domain: W3C baggage is injected on
// outbound requests and would leak user_id to 3rd-party hosts. user_id therefore
// rides only this in-process value and never enters baggage. Empty user_id is a
// no-op.
func WithUserID(ctx context.Context, userID string) context.Context {
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext reads the private user_id value, or "" when unset.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}
