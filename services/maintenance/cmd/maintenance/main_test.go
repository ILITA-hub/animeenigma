package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/config"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/state"
)

// TestMain initializes the package-level `log` var (normally set in main()
// before any handler runs) so tests exercising log-calling code paths (e.g.
// dropSuppressedAlerts) don't panic on a nil *logger.Logger.
func TestMain(m *testing.M) {
	log = logger.Default()
	os.Exit(m.Run())
}

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
	// Underscores are backslash-escaped by escTelegram — legacy Telegram
	// Markdown treats a bare "_" as a potential italic delimiter.
	want := "⚠️ Unhealthy: allanime (cdn\\_unreachable), animefever (empty\\_response)"
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
	if got := formatProviderFaultLine(rows); got != "⚠️ Unhealthy: gogoanime (browser provider\\_down)" {
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
	if got := s.scraperProviderFaultLine(); got != "⚠️ Unhealthy: okru (cdn\\_unreachable)" {
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

// TestDropSuppressedAlerts verifies the early filter — called immediately
// after batch.Relevant is assembled in processWork, before the
// multi-service-outage triage — removes suppressed firing alerts so they can
// never inflate the escalation count or reach escalateBatch/dedup/analysis,
// while non-suppressed alerts and non-alert messages pass through unchanged
// and in order.
func TestDropSuppressedAlerts(t *testing.T) {
	s := newTestServiceWithHTTP(t, "http://127.0.0.1:0", &http.Client{})
	s.cfg.SuppressedAlerts = []string{"High Error Rate:streaming", "High Error Rate:gateway"}

	suppressedStreaming := domain.ClassifiedMessage{
		MessageID: 1,
		Type:      domain.MessageAlertFiring,
		Alerts:    []domain.AlertInfo{{Name: "High Error Rate", Service: "streaming"}},
	}
	suppressedGateway := domain.ClassifiedMessage{
		MessageID: 2,
		Type:      domain.MessageAlertFiring,
		Alerts:    []domain.AlertInfo{{Name: "High Error Rate", Service: "gateway"}},
	}
	nonSuppressedAlert := domain.ClassifiedMessage{
		MessageID: 3,
		Type:      domain.MessageAlertFiring,
		Alerts:    []domain.AlertInfo{{Name: "High Error Rate", Service: "catalog"}},
	}
	adminMessage := domain.ClassifiedMessage{
		MessageID: 4,
		Type:      domain.MessageAdminMessage,
		Text:      "restart streaming please",
	}

	in := []domain.ClassifiedMessage{suppressedStreaming, suppressedGateway, nonSuppressedAlert, adminMessage}
	got := s.dropSuppressedAlerts(in)

	want := []domain.ClassifiedMessage{nonSuppressedAlert, adminMessage}
	if len(got) != len(want) {
		t.Fatalf("dropSuppressedAlerts() returned %d messages, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].MessageID != want[i].MessageID {
			t.Errorf("index %d: got MessageID %d, want %d (order/content mismatch)", i, got[i].MessageID, want[i].MessageID)
		}
	}
}
