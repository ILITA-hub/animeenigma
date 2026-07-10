package sidecar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSIDFromProxyURL(t *testing.T) {
	sid, ok := SIDFromProxyURL("http://stealth-scraper:3000/hls?sid=abc123def456abc123def456abc12345&url=https%3A%2F%2Fcdn.mewstream.buzz%2Fm.m3u8")
	if !ok || sid != "abc123def456abc123def456abc12345" {
		t.Fatalf("want sid extracted, got %q ok=%v", sid, ok)
	}
	if _, ok := SIDFromProxyURL("https://vault-99.owocdn.top/stream/uwu.m3u8"); ok {
		t.Fatal("non-sidecar URL must not yield a sid")
	}
	if _, ok := SIDFromProxyURL("http://stealth-scraper:3000/hls?url=x"); ok {
		t.Fatal("missing sid must not yield a sid")
	}
}

func TestSessionAlive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/deadbeef/alive" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"state":"gone"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, 2*time.Second)
	if got := c.SessionAlive(context.Background(), "deadbeef"); got != "gone" {
		t.Fatalf("want gone, got %q", got)
	}
}

func TestSessionAliveFailsOpen(t *testing.T) {
	c := New("http://127.0.0.1:1", 200*time.Millisecond) // nothing listening
	if got := c.SessionAlive(context.Background(), "deadbeef"); got != "alive" {
		t.Fatalf("errors must fail open to alive, got %q", got)
	}
}

func TestSessionAliveHonorsContextDeadline(t *testing.T) {
	// Verify that SessionAlive respects a tight deadline and fails open to "alive"
	// rather than waiting for the full 90s sidecar timeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond) // Ensure context has expired before calling

	c := New("http://127.0.0.1:1", 90*time.Second) // generous sidecar timeout, but our 2s bound should fire first
	if got := c.SessionAlive(ctx, "deadbeef"); got != "alive" {
		t.Fatalf("deadline expiry must fail open to alive, got %q", got)
	}
}
