package shikimori

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
)

func newTestClient(srvURL string) *Client {
	return NewClient(config.ShikimoriConfig{
		BaseURL:    srvURL,
		GraphQLURL: srvURL + "/api/graphql",
		UserAgent:  "test-agent",
		Timeout:    5 * time.Second,
		RateLimit:  100,
	}, nil)
}

func TestGetAnimeFranchise_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/animes/52991" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":52991,"russian":"Фрирен","franchise":"frieren"}`))
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL).GetAnimeFranchise(context.Background(), "52991")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "frieren" {
		t.Fatalf("want frieren, got %q", got)
	}
}

func TestGetAnimeFranchise_EmptyWhenMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"russian":"X","franchise":null}`))
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL).GetAnimeFranchise(context.Background(), "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestGetAnimeFranchise_404IsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL).GetAnimeFranchise(context.Background(), "999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("want empty on 404, got %q", got)
	}
}
