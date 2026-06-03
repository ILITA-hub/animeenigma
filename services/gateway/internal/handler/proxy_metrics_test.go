package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
)

// counterValue reads the current value of a Prometheus counter child without
// pulling in the client_golang/testutil subpackage (which would drag a newer
// client_golang + its transitive test deps into every workspace module).
func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("write counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

// An upstream 5xx that still produced a response must be counted under
// proxy_upstream_errors_total{status="<code>",domain="<service>"} so the
// auth-refresh cookie-drop failure mode is queryable. Before this fix the
// counter had zero call sites and the failure was invisible.
func TestProxyHandler_Upstream5xx_IncrementsProxyUpstreamErrors(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	proxySvc := service.NewProxyService(config.ServiceURLs{AuthService: backend.URL}, logger.Default())
	h := NewProxyHandler(proxySvc, logger.Default())

	before := counterValue(t, metrics.ProxyUpstreamErrors.WithLabelValues("500", "auth"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	h.ProxyToAuth(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; want 500 (gateway must RELAY the upstream 5xx, not synthesize a different one)", rec.Code)
	}
	if delta := counterValue(t, metrics.ProxyUpstreamErrors.WithLabelValues("500", "auth")) - before; delta != 1 {
		t.Errorf("proxy_upstream_errors_total{status=500,domain=auth} delta = %v; want 1", delta)
	}
}

// A transport-level Forward error (upstream unreachable) must increment the
// forward_error variant.
func TestProxyHandler_ForwardError_IncrementsProxyUpstreamErrors(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := backend.URL
	backend.Close() // now unreachable → Forward returns an error

	proxySvc := service.NewProxyService(config.ServiceURLs{AuthService: deadURL}, logger.Default())
	h := NewProxyHandler(proxySvc, logger.Default())

	before := counterValue(t, metrics.ProxyUpstreamErrors.WithLabelValues("forward_error", "auth"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	h.ProxyToAuth(rec, req)

	if delta := counterValue(t, metrics.ProxyUpstreamErrors.WithLabelValues("forward_error", "auth")) - before; delta != 1 {
		t.Errorf("proxy_upstream_errors_total{status=forward_error,domain=auth} delta = %v; want 1", delta)
	}
}
