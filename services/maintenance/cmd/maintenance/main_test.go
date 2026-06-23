package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/config"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/state"
)

func newTestServiceWithHTTP(t *testing.T, catalogURL string, httpClient *http.Client) *service {
	t.Helper()
	dir := t.TempDir()
	m := state.NewManager(filepath.Join(dir, "state.json"), filepath.Join(dir, "issues.json"))
	if err := m.Load(); err != nil {
		t.Fatalf("state load: %v", err)
	}
	return &service{
		state: m,
		cfg:   &config.Config{CatalogURL: catalogURL},
		http:  httpClient,
	}
}

func TestShouldSuppressForProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"providers":[{"name":"allanime","policy":"manual"},{"name":"gogoanime","policy":"auto"}]}}`))
	}))
	defer srv.Close()

	s := newTestServiceWithHTTP(t, srv.URL, srv.Client())

	if !s.shouldSuppressForProvider("allanime") {
		t.Fatal("manual provider should be suppressed")
	}
	if s.shouldSuppressForProvider("gogoanime") {
		t.Fatal("auto provider must NOT be suppressed")
	}
	// Unknown provider: not in list → must not suppress
	if s.shouldSuppressForProvider("nineanime") {
		t.Fatal("unknown provider must NOT be suppressed")
	}
	// Empty provider string: must not suppress
	if s.shouldSuppressForProvider("") {
		t.Fatal("empty provider must NOT be suppressed")
	}
}

func TestShouldSuppressForProvider_FailOpen(t *testing.T) {
	// Unreachable URL must fail open (return false)
	s := newTestServiceWithHTTP(t, "http://127.0.0.1:0", &http.Client{})
	if s.shouldSuppressForProvider("allanime") {
		t.Fatal("unreachable catalog must fail open (return false)")
	}
}

func TestShouldSuppressForProvider_Non200(t *testing.T) {
	// Catalog returning a non-200 status must fail open so a catalog outage
	// never silently blocks escalation of a real provider incident.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	s := newTestServiceWithHTTP(t, srv.URL, srv.Client())
	if s.shouldSuppressForProvider("allanime") {
		t.Fatal("catalog 503 must fail open (return false)")
	}
}
