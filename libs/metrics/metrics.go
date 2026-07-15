package metrics

import (
	"bufio"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Collector holds all Prometheus metrics for a service
type Collector struct {
	serviceName string

	requestsTotal     *prometheus.CounterVec
	requestDuration   *prometheus.HistogramVec
	responseSizeBytes *prometheus.HistogramVec
}

// NewCollector creates a new metrics collector for a service
func NewCollector(serviceName string) *Collector {
	c := &Collector{
		serviceName: serviceName,

		requestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"service", "method", "path", "status"},
		),

		requestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "http_request_duration_seconds",
				Help: "HTTP request latency in seconds",
				Buckets: []float64{
					0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 15, 30,
				},
			},
			[]string{"service", "method", "path", "status"},
		),

		responseSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "http_response_size_bytes",
				Help: "HTTP response size in bytes",
				Buckets: []float64{
					100, 1000, 10000, 100000, 1000000, 10000000,
				},
			},
			[]string{"service", "method", "path", "status"},
		),
	}

	return c
}

// responseWriter wraps http.ResponseWriter to capture status code and size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Hijack implements http.Hijacker by delegating to the underlying
// ResponseWriter when it supports hijacking. This is REQUIRED for WebSocket
// reverse proxies (httputil.ReverseProxy.ServeHTTP) which call Hijack on the
// writer chain to take ownership of the TCP socket after the 101 handshake.
// Without this, every middleware that wraps the writer (including this
// metrics middleware, applied globally at gateway boot) blocks WS upgrades
// with "websocket: response does not implement http.Hijacker".
//
// Discovered during Phase 1 plan 01.9 of workstream watch-together (the WS
// proxy added in plan 01.7 silently returned HTTP 500 on every upgrade).
// All shared response-writer wrappers across libs/* should forward Hijacker;
// this fix lives in libs/metrics because the metrics middleware is the one
// every service mounts globally.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Flush implements http.Flusher by delegating to the underlying writer when
// supported. The reverse proxy uses Flush on long-lived streams (SSE, WS
// pre-upgrade) so middleware that hides Flusher breaks streaming. Cheap to
// forward; no-op when the inner writer doesn't support it.
func (rw *responseWriter) Flush() {
	if fl, ok := rw.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

// Middleware returns an HTTP middleware that records metrics
func (c *Collector) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip metrics endpoint
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rw := newResponseWriter(w)

		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(rw.statusCode)
		path := metricPath(r)

		c.requestsTotal.WithLabelValues(c.serviceName, r.Method, path, status).Inc()
		c.requestDuration.WithLabelValues(c.serviceName, r.Method, path, status).Observe(duration)
		c.responseSizeBytes.WithLabelValues(c.serviceName, r.Method, path, status).Observe(float64(rw.size))
	})
}

// Handler returns the Prometheus HTTP handler for the /metrics endpoint
func Handler() http.Handler {
	return promhttp.Handler()
}

// metricPath returns a bounded-cardinality label after chi has completed route
// matching. For 404s and non-chi handlers, it keeps only a small operational
// namespace instead of retaining attacker-controlled path segments.
func metricPath(r *http.Request) string {
	if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
		if pattern := routeContext.RoutePattern(); pattern != "" {
			return canonicalRoutePattern(pattern)
		}
	}
	return unmatchedPathCategory(r.URL.Path)
}

func canonicalRoutePattern(pattern string) string {
	segments := splitPath(pattern)
	if len(segments) == 0 {
		return "/"
	}
	for i, segment := range segments {
		switch {
		case segment == "*":
			segments[i] = ":wildcard"
		case strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}"):
			name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
			if colon := strings.IndexByte(name, ':'); colon >= 0 {
				name = name[:colon]
			}
			if name == "" {
				name = "param"
			}
			segments[i] = ":" + name
		}
	}
	return "/" + strings.Join(segments, "/")
}

func unmatchedPathCategory(path string) string {
	segments := splitPath(path)
	if len(segments) == 0 {
		return "/other"
	}

	switch segments[0] {
	case "api", "internal", "admin", "worker":
		return "/" + segments[0] + "/:other"
	case ".well-known":
		return "/.well-known/:other"
	case "assets", "static", "favicon.ico", "robots.txt", "sitemap.xml":
		return "/static/:other"
	case "health", "healthz", "ready", "readyz", "live", "livez", "status":
		return "/health/:other"
	default:
		return "/other"
	}
}

func splitPath(path string) []string {
	var segments []string
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				segments = append(segments, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		segments = append(segments, current)
	}
	return segments
}
