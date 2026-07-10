package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchSynopsis_ByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/anime/abc" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"name":"Frieren","description":"A mage journeys..."}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 2*time.Second, nil)
	title, synopsis, err := c.FetchSynopsis(context.Background(), "abc", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if title != "Frieren" || synopsis != "A mage journeys..." {
		t.Fatalf("got title=%q synopsis=%q", title, synopsis)
	}
}

func TestFetchSynopsis_ShikimoriFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/anime/shikimori/52991" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"name":"Frieren","description":"desc"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 2*time.Second, nil)
	_, synopsis, err := c.FetchSynopsis(context.Background(), "", "52991")
	if err != nil || synopsis != "desc" {
		t.Fatalf("fallback failed: synopsis=%q err=%v", synopsis, err)
	}
}

func TestFetchSynopsis_ErrorIsGraceful(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 2*time.Second, nil)
	_, synopsis, err := c.FetchSynopsis(context.Background(), "abc", "")
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if synopsis != "" {
		t.Fatalf("synopsis should be empty on error, got %q", synopsis)
	}
}
