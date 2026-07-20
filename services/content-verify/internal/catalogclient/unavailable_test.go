// Typed-503 handling (owner directive 2026-07-20): a scraper 503 becomes an
// *UnavailableError carrying the upstream negative-cache window, so callers
// can defer exactly until the entry expires instead of re-asking blindly.
package catalogclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetJSON_503BecomesUnavailableWithRetryAfter(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/a1/scraper/episodes", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"success":false,"error":{"code":"PROVIDER_DOWN","message":"down","retry_after_seconds":900},"meta":{"tried":["miruro"]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := New(srv.URL, srv.URL, srv.Client())

	_, err := c.ScraperEpisodes(context.Background(), "a1", "miruro")
	ue, ok := AsUnavailable(err)
	if !ok {
		t.Fatalf("error = %v; want *UnavailableError", err)
	}
	if ue.RetryAfter != 900*time.Second {
		t.Errorf("RetryAfter = %s; want 15m0s", ue.RetryAfter)
	}
}

func TestGetJSON_503WithoutHintDefaultsToNegTTL(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/a1/scraper/episodes", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // e.g. proxy-generated 503, no JSON body
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := New(srv.URL, srv.URL, srv.Client())

	_, err := c.ScraperEpisodes(context.Background(), "a1", "miruro")
	ue, ok := AsUnavailable(err)
	if !ok {
		t.Fatalf("error = %v; want *UnavailableError", err)
	}
	if ue.RetryAfter != unavailableDefaultRetry {
		t.Errorf("RetryAfter = %s; want default %s", ue.RetryAfter, unavailableDefaultRetry)
	}
}

func TestScraperRoster(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/scraper/providers", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"providers":[
			{"name":"gogoanime","health":"up","policy":"auto","scraper_operated":true},
			{"name":"miruro","health":"down","policy":"manual","scraper_operated":true},
			{"name":"kodik","health":"","policy":"","scraper_operated":false}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := New(srv.URL, srv.URL, srv.Client())

	rows, err := c.ScraperRoster(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d; want 3", len(rows))
	}
	if rows[1].Name != "miruro" || rows[1].Health != "down" || !rows[1].ScraperOperated {
		t.Errorf("miruro row = %+v; want health=down scraper_operated=true", rows[1])
	}
}
