package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/baggage"

	"github.com/ILITA-hub/animeenigma/libs/authz"
)

// wireUserID reads a "user_id" member straight out of W3C baggage — used only
// to assert that the seed path never puts user_id there.
func wireUserID(ctx context.Context) string {
	return baggage.FromContext(ctx).Member("user_id").Value()
}

// TestSeedMiddleware: a request through the seed middleware (mounted AFTER chi
// routing) yields ctx baggage with operation = "service METHOD routePattern"
// and user_id present in the private ctx value when claims exist — but user_id
// is NOT in wire baggage.
func TestSeedMiddleware(t *testing.T) {
	var gotOrigin, gotOp, gotUID, gotWireUID string

	r := chi.NewRouter()
	r.Use(SeedMiddleware("catalog"))
	r.Get("/api/anime/{id}", func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		gotOrigin, gotOp = ReadBaggage(ctx)
		gotUID = UserIDFromContext(ctx)
		// Confirm user_id is not on the wire.
		gotWireUID = wireUserID(ctx)
		w.WriteHeader(http.StatusOK)
	})

	// Inject claims so ClaimsFromContext returns a user.
	req := httptest.NewRequest(http.MethodGet, "/api/anime/42", nil)
	req = req.WithContext(authz.ContextWithClaims(req.Context(), &authz.Claims{UserID: "u-77"}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if gotOrigin != "api" {
		t.Fatalf("origin = %q, want api", gotOrigin)
	}
	if gotOp != "catalog GET /api/anime/{id}" {
		t.Fatalf("operation = %q, want coarse route pattern", gotOp)
	}
	if gotUID != "u-77" {
		t.Fatalf("user_id = %q, want u-77", gotUID)
	}
	if gotWireUID != "" {
		t.Fatalf("user_id leaked into wire baggage: %q", gotWireUID)
	}
}
