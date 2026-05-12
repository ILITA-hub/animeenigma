package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
)

// newTestProxy constructs a ProxyService wired to a config with the given
// scraper URL. The other service URLs are left as zero-value strings — tests
// in this file only exercise the scraper case.
func newTestProxy(scraperURL string) *ProxyService {
	return NewProxyService(config.ServiceURLs{
		ScraperService: scraperURL,
	}, logger.Default())
}

// TestProxyService_GetServiceURL_Scraper asserts the "scraper" case routes to
// ServiceURLs.ScraperService.
func TestProxyService_GetServiceURL_Scraper(t *testing.T) {
	t.Parallel()
	p := newTestProxy("http://scraper:8088")
	got, err := p.getServiceURL("scraper")
	if err != nil {
		t.Fatalf("getServiceURL: %v", err)
	}
	if got != "http://scraper:8088" {
		t.Errorf("getServiceURL(scraper) = %q; want http://scraper:8088", got)
	}
}

// TestProxyService_PathRewrite_AdminHealth asserts the explicit rewrite for
// the admin health endpoint: /api/admin/scraper/health → /scraper/health/admin.
//
// We exercise this by spinning up a backend httptest.Server that records the
// inbound URL.Path; the Forward call routes through the rewrite block and
// the recorded path is what the scraper service would actually see.
func TestProxyService_PathRewrite_AdminHealth(t *testing.T) {
	t.Parallel()
	gotPath := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	resp, err := p.Forward(req, "scraper")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	if got := <-gotPath; got != "/scraper/health/admin" {
		t.Errorf("backend received path = %q; want /scraper/health/admin", got)
	}
}

// TestProxyService_PathRewrite_OtherAdminScraper asserts the generic
// fallthrough for unknown admin/scraper subpaths: the /admin segment is
// stripped but no /admin suffix is appended. Today only /health has an
// explicit rewrite; this test pins the fallthrough so a future second admin
// endpoint slots in deterministically.
func TestProxyService_PathRewrite_OtherAdminScraper(t *testing.T) {
	t.Parallel()
	gotPath := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	p := newTestProxy(backend.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/other", nil)
	resp, err := p.Forward(req, "scraper")
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	got := <-gotPath
	if got != "/scraper/other" {
		t.Errorf("backend received path = %q; want /scraper/other", got)
	}
	// Defensive: ensure no /admin segment slipped through.
	if strings.Contains(got, "/admin") {
		t.Errorf("backend received path = %q; must not contain /admin", got)
	}
}
