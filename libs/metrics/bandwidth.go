package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ProxyBytesTransferredTotal counts bytes written to clients through proxy/streaming.
	ProxyBytesTransferredTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxy_bytes_transferred_total",
			Help: "Total bytes transferred to clients through proxy",
		},
		[]string{"type"},
	)
)

// CountingResponseWriter wraps an http.ResponseWriter and increments a
// Prometheus counter with the number of bytes written.
type CountingResponseWriter struct {
	http.ResponseWriter
	Counter prometheus.Counter
}

func (crw *CountingResponseWriter) Write(p []byte) (int, error) {
	n, err := crw.ResponseWriter.Write(p)
	crw.Counter.Add(float64(n))
	return n, err
}
