package jimaku

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPing_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/entries/search" || r.URL.Query().Get("anilist_id") != "1" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()
	c := NewClient("key")
	c.baseURL = srv.URL
	if _, err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping OK: unexpected error %v", err)
	}
}

func TestPing_Non200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := NewClient("key")
	c.baseURL = srv.URL
	if _, err := c.Ping(context.Background()); err == nil {
		t.Fatal("Ping 503: expected error, got nil")
	}
}
