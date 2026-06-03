package tracing

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// WrapTransport wraps an existing RoundTripper so outbound requests get a
// client span and the active trace context is injected (W3C traceparent) using
// the global propagator. Pass nil to wrap http.DefaultTransport. Use this to
// keep a custom transport's dialer/pool settings while adding propagation:
//
//	t.Transport = tracing.WrapTransport(t.Transport)
func WrapTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return otelhttp.NewTransport(base)
}

// NewHTTPClient returns an *http.Client whose transport propagates trace
// context. base may be nil (http.DefaultTransport). When tracing is disabled
// the global provider is a no-op and no header is injected.
func NewHTTPClient(base http.RoundTripper, timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: WrapTransport(base)}
}
