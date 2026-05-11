package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
)

// TestRouter_NewScraperRoutesRegistered exercises the four /scraper/*
// routes through a real chi router constructed the same way
// transport.NewRouter does, but with only the scraper-endpoint handler
// wired. This verifies routes resolve (no 404) and the scraper handler
// is reachable end-to-end through the chi tree.
//
// We can't construct the full transport.NewRouter() here because it
// requires a full *handler.CatalogHandler (which needs a real
// *service.CatalogService backed by GORM). The /scraper/* routes are
// independent of the rest of the catalog routes, so testing them via a
// scoped chi router is functionally equivalent — the same chi.URLParam
// extraction + r.Get registration shape is used.
func TestRouter_NewScraperRoutesRegistered(t *testing.T) {
	// Build a stub scraper service that always answers 503 with the
	// canonical phase-15 body. The actual reply doesn't matter — the test
	// is whether the route reaches the handler at all.
	svc := &stubScraperSvc{status: http.StatusServiceUnavailable, body: []byte(`{"error":"not-yet-implemented","phase":15}`)}
	h := &handler.ScraperEndpointsHandler{}
	handler.WireScraperEndpoints(h, svc, logger.Default())

	// Mirror the catalog router's r.Route("/anime", ...) shape using the
	// same chi calls the production NewRouter uses.
	r := buildScraperOnlyRouter(h)

	cases := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"episodes", "/api/anime/uuid-1/scraper/episodes?prefer=animepahe", 503},
		{"servers", "/api/anime/uuid-1/scraper/servers?episode=ep-1", 503},
		{"stream", "/api/anime/uuid-1/scraper/stream?episode=ep-1&server=srv-1", 503},
		{"health", "/api/anime/uuid-1/scraper/health", 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Health endpoint must reply 200 (svc returns OK for it).
			if tc.name == "health" {
				svc.status = http.StatusOK
				svc.body = []byte(`{"success":true,"data":{"providers":{}}}`)
			} else {
				svc.status = http.StatusServiceUnavailable
				svc.body = []byte(`{"error":"not-yet-implemented","phase":15}`)
			}

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound {
				t.Fatalf("route %s returned 404 — not registered", tc.path)
			}
			if rec.Code != tc.wantStatus {
				t.Errorf("route %s: status = %d, want %d (body=%q)", tc.path, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
				t.Errorf("route %s: content-type = %q, want application/json", tc.path, rec.Header().Get("Content-Type"))
			}
		})
	}
}
