package animelib

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestSearch_ContextCancellation asserts that Search honors a cancelled context
// and aborts the in-flight upstream request promptly, rather than running to the
// http.Client's 10s timeout. Regression guard for the catalog findAnimeLibID
// fan-out where losing goroutines kept hitting AnimeLib after a match was found
// because Search dropped ctx (doRequest used http.NewRequest, not
// http.NewRequestWithContext).
func TestSearch_ContextCancellation(t *testing.T) {
	var serverSawCancel atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hold the response; resolve when the request context is cancelled
		// (client aborted) or after a 2s ceiling so Close doesn't hang.
		select {
		case <-r.Context().Done():
			serverSawCancel.Store(true)
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	c := &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    srv.URL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := c.Search(ctx, "frieren")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled Search, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("Search ignored ctx cancellation: took %v (want < 1s)", elapsed)
	}
}
