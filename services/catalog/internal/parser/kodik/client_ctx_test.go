package kodik

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestGetTranslations_ContextCancellation asserts that GetTranslations honors a
// cancelled context promptly instead of running to the HTTP client's 30s
// timeout. Regression guard for the fan-out / capabilities path where the Kodik
// leg previously dropped ctx (kodik/client.go GetTranslations took no context
// and SearchByShikimoriID used PostForm with no request context).
func TestGetTranslations_ContextCancellation(t *testing.T) {
	// Slow server: holds the response for ~2s (far longer than the 50ms cancel
	// window below, far shorter than the client's 30s timeout). A client that
	// honors ctx aborts its request at ~50ms; one that ignores it would block on
	// Do until the server responds. The handler returns on its own so
	// httptest.Server.Close does not hang on an indefinitely-blocked handler.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	c := &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		apiEndpoint: srv.URL,
		token:       "deadbeefdeadbeef", // valid-looking token; skip token fetch
	}
	// Pre-set expiry so refreshTokenIfNeeded does no network I/O.
	c.tokenExpires = time.Now().Add(time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel almost immediately so the in-flight request aborts.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := c.GetTranslations(ctx, "20")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error from cancelled GetTranslations, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// The client aborts the in-flight request the moment ctx is cancelled (~50ms),
	// well before the 2s server hold. A client that ignored ctx would block on Do
	// until the server responded (~2s).
	if elapsed > time.Second {
		t.Fatalf("GetTranslations ignored ctx cancellation: took %v (want < 1s)", elapsed)
	}
}
