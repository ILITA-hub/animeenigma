package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPPopularPool_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/anime/popular" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("page_size") != "100" {
			t.Errorf("expected page_size=100, got %s", r.URL.Query().Get("page_size"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":[{"id":"u1","name":"A"},{"id":"u2","name":"B"}],"meta":{}}`))
	}))
	defer srv.Close()

	pool := NewHTTPPopularPool(srv.URL, srv.Client())
	got, err := pool.Pool(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].UUID != "u1" || got[0].Name != "A" {
		t.Errorf("first item = %+v, want {UUID:u1 Name:A}", got[0])
	}
	if got[1].UUID != "u2" || got[1].Name != "B" {
		t.Errorf("second item = %+v, want {UUID:u2 Name:B}", got[1])
	}
}
