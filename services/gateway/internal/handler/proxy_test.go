package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
)

// TestProxyHandler_ProxyToScraper_CallsForward exercises the one-liner shim:
// ProxyToScraper(w, r) MUST route through the proxy service with service
// name "scraper", which in turn lands at ServiceURLs.ScraperService.
//
// The spy is the backend httptest.Server — if Forward routes anywhere else,
// the recorded path channel never receives and the test times out (visible
// as a select default fall-through into a test failure).
func TestProxyHandler_ProxyToScraper_CallsForward(t *testing.T) {
	t.Parallel()
	gotMethod := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod <- r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxySvc := service.NewProxyService(config.ServiceURLs{
		ScraperService: backend.URL,
	}, logger.Default())
	h := NewProxyHandler(proxySvc, logger.Default())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	h.ProxyToScraper(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", rec.Code)
	}
	if method := <-gotMethod; method != http.MethodGet {
		t.Errorf("backend method = %q; want GET", method)
	}
}
