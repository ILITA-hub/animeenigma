package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/service"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	imageProxyRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "image_proxy_requests_total",
		Help: "Total image proxy requests by source",
	}, []string{"source"})

	imageProxyUpstreamDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "image_proxy_upstream_duration_seconds",
		Help:    "Upstream image fetch latency",
		Buckets: prometheus.DefBuckets,
	}, []string{"upstream"})

	imageProxyUpstreamErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "image_proxy_upstream_errors_total",
		Help: "Upstream image fetch errors by reason",
	}, []string{"upstream", "reason"})

	imageProxyCacheWriteErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "image_proxy_cache_write_errors_total",
		Help: "Cache write failures to MinIO",
	})
)

type ImageProxyHandler struct {
	imageProxyService *service.ImageProxyService
	log               *logger.Logger
}

func NewImageProxyHandler(imageProxyService *service.ImageProxyService, log *logger.Logger) *ImageProxyHandler {
	return &ImageProxyHandler{
		imageProxyService: imageProxyService,
		log:               log,
	}
}

func (h *ImageProxyHandler) ProxyImage(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		httputil.BadRequest(w, "url parameter is required")
		return
	}

	result, err := h.imageProxyService.GetImage(r.Context(), rawURL)
	if err != nil {
		if err.Error() == "domain not allowed" {
			httputil.BadRequest(w, "domain not allowed")
			return
		}
		h.log.Errorw("image proxy error", "url", rawURL, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	imageProxyRequestsTotal.WithLabelValues(string(result.Source)).Inc()

	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.Header().Set("X-Image-Source", string(result.Source))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write(result.Data)
}
