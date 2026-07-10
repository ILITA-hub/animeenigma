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

func TestSessionAliveBoundedBySlowSidecar(t *testing.T) {
	// Discriminating proof of the internal 2s bound: the server answers
	// {"state":"gone"} only after a 3s sleep, and the caller imposes NO deadline
	// of its own. Un-fixed code (inheriting the 90s client timeout) would wait
	// out the sleep and return "gone" in ~3s; fixed code trips its own 2s bound
	// and fails open to "alive" in ~2s.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done(): // client hung up — unblock so srv.Close() is fast
			return
		case <-time.After(3 * time.Second):
		}
		w.Write([]byte(`{"state":"gone"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, 90*time.Second) // generous client timeout; only the internal bound can fire
	start := time.Now()
	got := c.SessionAlive(context.Background(), "deadbeef")
	elapsed := time.Since(start)
	if got != "alive" {
		t.Fatalf("slow sidecar must fail open to alive, got %q", got)
	}
	if elapsed >= 2900*time.Millisecond {
		t.Fatalf("SessionAlive not bounded: took %v (want < 2.9s)", elapsed)
	}
}
