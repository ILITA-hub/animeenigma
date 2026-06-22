package opensubtitles

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPing_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/infos/formats" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Api-Key") != "key" {
			t.Errorf("missing Api-Key header")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()
	c := NewClient(Config{APIKey: "key", BaseURL: srv.URL})
	if _, err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping OK: unexpected error %v", err)
	}
}

func TestPing_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := NewClient(Config{APIKey: "key", BaseURL: srv.URL})
	if _, err := c.Ping(context.Background()); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Ping 429: want ErrRateLimited, got %v", err)
	}
}

func TestPing_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewClient(Config{APIKey: "bad", BaseURL: srv.URL})
	if _, err := c.Ping(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Ping 401: want ErrUnauthorized, got %v", err)
	}
}
