package groq

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStream_AccumulatesDeltasAndUsage(t *testing.T) {
	// Fake Groq SSE: two content deltas, then a usage-only chunk, then [DONE].
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("missing auth header, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"# Title\\n\\nHello\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"total_tokens\":42}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "llama-3.1-8b-instant", 5*time.Second)
	var deltas []string
	text, usage, err := c.Stream(context.Background(), "sys", "usr", 100, 0.9, func(d string) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("Stream err: %v", err)
	}
	if !strings.Contains(text, "Hello world") {
		t.Errorf("text = %q, want it to contain 'Hello world'", text)
	}
	if usage != 42 {
		t.Errorf("usage = %d, want 42", usage)
	}
	if len(deltas) != 2 {
		t.Errorf("onDelta called %d times, want 2", len(deltas))
	}
}

func TestStream_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New("k", srv.URL, "m", time.Second)
	if _, _, err := c.Stream(context.Background(), "s", "u", 10, 0.5, func(string) {}); err == nil {
		t.Fatal("expected error on 429, got nil")
	}
}
