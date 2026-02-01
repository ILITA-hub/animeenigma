package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Collector holds all Prometheus metrics for a service
type Collector struct {
	serviceName string

	requestsTotal    *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
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
					0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
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
		path := normalizePath(r.URL.Path)

		c.requestsTotal.WithLabelValues(c.serviceName, r.Method, path, status).Inc()
		c.requestDuration.WithLabelValues(c.serviceName, r.Method, path, status).Observe(duration)
		c.responseSizeBytes.WithLabelValues(c.serviceName, r.Method, path, status).Observe(float64(rw.size))
	})
}

// Handler returns the Prometheus HTTP handler for the /metrics endpoint
func Handler() http.Handler {
	return promhttp.Handler()
}

// normalizePath normalizes URL paths to reduce cardinality
// Replaces dynamic segments (UUIDs, numbers) with placeholders
func normalizePath(path string) string {
	if len(path) == 0 {
		return "/"
	}

	// Common patterns to normalize
	// We'll keep paths simple and group by first two segments
	segments := splitPath(path)
	if len(segments) == 0 {
		return "/"
	}

	// Keep first two segments, replace rest with placeholder
	normalized := ""
	for i, seg := range segments {
		if i >= 2 {
			// Check if segment looks like an ID (UUID or numeric)
			if isID(seg) {
				normalized += "/:id"
			} else {
				normalized += "/" + seg
			}
		} else {
			normalized += "/" + seg
		}
		if i >= 3 {
			break
		}
	}

	return normalized
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

func isID(s string) bool {
	if len(s) == 0 {
		return false
	}
	// Check if it's a UUID (36 chars with dashes)
	if len(s) == 36 {
		return true
	}
	// Check if it's numeric
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
