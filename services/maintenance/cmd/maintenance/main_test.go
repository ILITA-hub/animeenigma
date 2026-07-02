package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/config"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
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

func TestIsScraperAlert(t *testing.T) {
	for _, a := range []domain.AlertInfo{
		{Name: "High Error Rate", Service: "scraper"},
		{Name: "Parser Failure Rate", Service: ""},
		{Name: "Scraper unplayable spike", Service: "scraper"},
	} {
		if !isScraperAlert(a) {
			t.Errorf("expected scraper alert: %+v", a)
		}
	}
	for _, a := range []domain.AlertInfo{
		{Name: "High Error Rate", Service: "web"},
		{Name: "Service Unreachable", Service: "catalog"},
	} {
		if isScraperAlert(a) {
			t.Errorf("expected non-scraper alert: %+v", a)
		}
	}
}

func TestShortReason(t *testing.T) {
	cases := map[string]string{
		"cdn_unreachable on ":       "cdn_unreachable",
		"empty_response on tserver": "empty_response",
		"empty_response on 1anime":  "empty_response",
		"browser provider_down":     "browser provider_down",
		"  spaced  ":                "spaced",
		"":                          "",
	}
	for in, want := range cases {
		if got := shortReason(in); got != want {
			t.Errorf("shortReason(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatProviderFaultLine(t *testing.T) {
	rows := []scraperProviderRow{
		// healthy auto providers — excluded
		{Name: "gogoanime", Group: "en", Status: "enabled", Health: "up", Reason: "Revived via gogoanimes.fi mirror"},
		{Name: "miruro", Group: "en", Status: "enabled", Health: "up", Reason: "ok"},
		// degraded/down EN providers — included with short reason
		{Name: "allanime", Group: "en", Status: "degraded", Health: "down", Reason: "cdn_unreachable on "},
		{Name: "animefever", Group: "en", Status: "degraded", Health: "down", Reason: "empty_response on tserver"},
		// admin-disabled — excluded (intentionally off, not a fault)
		{Name: "animekai", Group: "en", Status: "disabled", Health: "down", Reason: "Stub — unimplemented"},
		// non-EN groups — excluded even when down
		{Name: "ae", Group: "firstparty", Status: "enabled", Health: "up", Reason: "first party"},
		{Name: "kodik-noads", Group: "ru", Status: "enabled", Health: "down", Reason: "ru thing"},
	}
	got := formatProviderFaultLine(rows)
	want := "⚠️ Unhealthy: allanime (cdn_unreachable), animefever (empty_response)"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestFormatProviderFaultLine_EnabledButDown(t *testing.T) {
	// An in-chain (enabled) provider failing but not yet auto-demoted must
	// still surface — its health is the live authority, not the status column.
	rows := []scraperProviderRow{
		{Name: "gogoanime", Group: "en", Status: "enabled", Health: "down", Reason: "browser provider_down"},
	}
	if got := formatProviderFaultLine(rows); got != "⚠️ Unhealthy: gogoanime (browser provider_down)" {
		t.Fatalf("enabled-but-down provider must surface, got %q", got)
	}
}

func TestFormatProviderFaultLine_AllHealthy(t *testing.T) {
	rows := []scraperProviderRow{
		{Name: "gogoanime", Group: "en", Status: "enabled", Health: "up"},
		{Name: "miruro", Group: "en", Status: "enabled", Health: "up"},
	}
	if got := formatProviderFaultLine(rows); got != "" {
		t.Fatalf("all-healthy roster must yield empty line, got %q", got)
	}
}

func TestScraperProviderFaultLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"providers":[
			{"name":"gogoanime","group":"en","status":"enabled","health":"up","reason":"ok"},
			{"name":"okru","group":"en","status":"degraded","health":"down","reason":"cdn_unreachable on "}
		]}}`))
	}))
	defer srv.Close()

	s := newTestServiceWithHTTP(t, srv.URL, srv.Client())
	if got := s.scraperProviderFaultLine(); got != "⚠️ Unhealthy: okru (cdn_unreachable)" {
		t.Fatalf("unexpected fault line: %q", got)
	}
}

func TestScraperProviderFaultLine_FailOpen(t *testing.T) {
	// Unreachable catalog must fail open (return "") so a catalog blip never
	// strips the firing alert.
	s := newTestServiceWithHTTP(t, "http://127.0.0.1:0", &http.Client{})
	if got := s.scraperProviderFaultLine(); got != "" {
		t.Fatalf("unreachable catalog must fail open, got %q", got)
	}
}

func TestIsSuppressed_StreamingGatewayKeys(t *testing.T) {
	s := newTestServiceWithHTTP(t, "http://127.0.0.1:0", &http.Client{})
	s.cfg.SuppressedAlerts = []string{"High Error Rate:streaming", "High Error Rate:gateway"}

	cases := []struct {
		key  string
		want bool
	}{
		{"High Error Rate:streaming", true},
		{"High Error Rate:gateway", true},
		{"high error rate:STREAMING", true}, // EqualFold is case-insensitive
		{"High Error Rate:catalog", false},  // catalog still pages
		{"Parser Failure Rate:gogoanime", false},
	}
	for _, c := range cases {
		if got := s.isSuppressed(c.key); got != c.want {
			t.Errorf("isSuppressed(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}
