package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/baggage"
)

// TestBaggageSeedRead: SeedBaggage then ReadBaggage round-trips origin+operation;
// empty values are skipped without failing the whole set.
func TestBaggageSeedRead(t *testing.T) {
	ctx := SeedBaggage(context.Background(), "api", "catalog GET /api/anime/{id}")
	origin, operation := ReadBaggage(ctx)
	if origin != "api" {
		t.Fatalf("origin = %q, want api", origin)
	}
	if operation != "catalog GET /api/anime/{id}" {
		t.Fatalf("operation = %q", operation)
	}

	// Empty origin must be skipped but operation should still be set.
	ctx2 := SeedBaggage(context.Background(), "", "scraper GET stream")
	o2, op2 := ReadBaggage(ctx2)
	if o2 != "" {
		t.Fatalf("empty origin should not be set, got %q", o2)
	}
	if op2 != "scraper GET stream" {
		t.Fatalf("operation should still be set, got %q", op2)
	}
}

// TestUserIDCtxValue: user_id round-trips via a PRIVATE ctx key, never W3C
// baggage. baggage.FromContext(ctx) must NOT contain user_id.
func TestUserIDCtxValue(t *testing.T) {
	ctx := WithUserID(context.Background(), "u-123")
	if got := UserIDFromContext(ctx); got != "u-123" {
		t.Fatalf("UserIDFromContext = %q, want u-123", got)
	}

	// user_id must NOT leak into wire baggage.
	bg := baggage.FromContext(ctx)
	if m := bg.Member("user_id"); m.Value() != "" {
		t.Fatalf("user_id leaked into W3C baggage: %q", m.Value())
	}

	// Empty user_id is a no-op (does not create an empty entry).
	if got := UserIDFromContext(WithUserID(context.Background(), "")); got != "" {
		t.Fatalf("empty user_id should read back empty, got %q", got)
	}

	// Combined: seeding baggage and user_id keeps user_id off the wire.
	ctx2 := WithUserID(SeedBaggage(context.Background(), "api", "x GET /y"), "u-9")
	if UserIDFromContext(ctx2) != "u-9" {
		t.Fatal("user_id not readable after combined seed")
	}
	if baggage.FromContext(ctx2).Member("user_id").Value() != "" {
		t.Fatal("user_id leaked into baggage after combined seed")
	}
}
