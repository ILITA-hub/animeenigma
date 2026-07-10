package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// slowThenOK sleeps, then answers 200 "done". Used to prove which routes the
// scoped timeout cuts off and which it lets run long.
func slowThenOK(d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(d):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("done"))
		case <-r.Context().Done():
			// http.TimeoutHandler cancels the request context at the deadline.
		}
	})
}

// TestScopedTimeout_ShortRoutesGet503 — any route other than the copy route
// must be cut off at the short ceiling with a 503 (http.TimeoutHandler),
// so a hung list/ingest/delete cannot hold its connection for the server's
// long WriteTimeout.
func TestScopedTimeout_ShortRoutesGet503(t *testing.T) {
	h := ScopedTimeout(slowThenOK(200*time.Millisecond), 20*time.Millisecond)

	for _, path := range []string{"/internal/storage/list", "/internal/storage/ingest-urls", "/health"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want 503 (timed out)", path, rec.Code)
		}
	}
}

// TestScopedTimeout_CopyRouteExempt — the copy route must pass through
// untouched: a handler slower than the short ceiling still completes.
func TestScopedTimeout_CopyRouteExempt(t *testing.T) {
	h := ScopedTimeout(slowThenOK(60*time.Millisecond), 20*time.Millisecond)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/storage/copy", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("copy route: status = %d, want 200 (exempt from short timeout)", rec.Code)
	}
	if got := rec.Body.String(); got != "done" {
		t.Fatalf("copy route: body = %q, want %q", got, "done")
	}
}

// TestScopedTimeout_FastRoutesUnaffected — a route answering inside the
// ceiling passes through normally.
func TestScopedTimeout_FastRoutesUnaffected(t *testing.T) {
	h := ScopedTimeout(slowThenOK(0), 100*time.Millisecond)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/storage/list", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("fast route: status = %d, want 200", rec.Code)
	}
}
