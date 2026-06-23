package userkey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	ctx := WithUserKey(context.Background(), "alice")
	if got := FromContext(ctx); got != "alice" {
		t.Errorf("FromContext = %q; want alice", got)
	}
}

func TestFromContextEmptyWhenUnset(t *testing.T) {
	if got := FromContext(context.Background()); got != "" {
		t.Errorf("FromContext on bare ctx = %q; want empty", got)
	}
}

func TestMiddlewareSeedsHeaderIntoContext(t *testing.T) {
	var seen string
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = FromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/scraper/stream", nil)
	req.Header.Set(HeaderName, "u-123")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "u-123" {
		t.Errorf("middleware seeded %q; want u-123", seen)
	}
}

func TestMiddlewareNoHeaderLeavesEmpty(t *testing.T) {
	var seen = "sentinel"
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = FromContext(r.Context())
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	if seen != "" {
		t.Errorf("no-header middleware seeded %q; want empty", seen)
	}
}
