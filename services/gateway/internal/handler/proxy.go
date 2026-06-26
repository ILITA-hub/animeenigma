package handler

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
)

type ProxyHandler struct {
	proxyService *service.ProxyService
	log          *logger.Logger
}

func NewProxyHandler(proxyService *service.ProxyService, log *logger.Logger) *ProxyHandler {
	return &ProxyHandler{
		proxyService: proxyService,
		log:          log,
	}
}

// ProxyToAuth proxies requests to auth service
func (h *ProxyHandler) ProxyToAuth(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "auth")
}

// ProxyToAuthNoRedirect proxies requests to the auth service without following
// HTTP redirects. Used for the magic-link bridge routes (/magic-link-generate,
// /magic-link-login) so that cross-domain 302 responses reach the browser
// instead of being chased server-side.
func (h *ProxyHandler) ProxyToAuthNoRedirect(w http.ResponseWriter, r *http.Request) {
	resp, err := h.proxyService.ForwardNoRedirect(r, "auth")
	if err != nil {
		h.log.Errorw("proxy failed", "service", "auth", "error", err)
		metrics.ProxyUpstreamErrors.WithLabelValues("forward_error", "auth").Inc()
		httputil.Error(w, err)
		return
	}
	defer resp.Body.Close()

	// Copy response headers, skipping CORS headers (gateway middleware handles CORS)
	for key, values := range resp.Header {
		if isCORSHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// ProxyToCatalog proxies requests to catalog service
func (h *ProxyHandler) ProxyToCatalog(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "catalog")
}

// ProxyToPlayer proxies requests to player service
func (h *ProxyHandler) ProxyToPlayer(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "player")
}

// ProxyToRecs proxies requests to the recs service (extraction Phase 1).
func (h *ProxyHandler) ProxyToRecs(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "recs")
}

// ProxyToAnidle proxies requests to the anidle guessing game service (spec 2026-06-15).
func (h *ProxyHandler) ProxyToAnidle(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "anidle")
}

// ProxyToRooms proxies requests to rooms service
func (h *ProxyHandler) ProxyToRooms(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "rooms")
}

// ProxyToScraper proxies requests to the scraper service (Phase 17 Plan 03).
// Used for /api/admin/scraper/* admin debug endpoints; the gateway router
// gates this group with JWTValidationMiddleware + AdminRoleMiddleware so
// the handler itself does not enforce auth.
func (h *ProxyHandler) ProxyToScraper(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "scraper")
}

// ProxyToStreaming proxies requests to streaming service
func (h *ProxyHandler) ProxyToStreaming(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "streaming")
}

// ProxyToStreamingBody proxies large/long-lived streaming bodies (HLS
// playlists/segments, MP4 restream, image-proxy) to the streaming service
// WITHOUT the 15s total client timeout that truncated long playback bodies
// (audit finding L466). It also clears the per-response server write deadline
// so the gateway HTTP server's WriteTimeout (30s) does not re-cap the streamed
// body — the body is then bounded only by the request context.
func (h *ProxyHandler) ProxyToStreamingBody(w http.ResponseWriter, r *http.Request) {
	h.proxyStream(w, r, "streaming")
}

// ProxyToThemes proxies requests to themes service
func (h *ProxyHandler) ProxyToThemes(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "themes")
}

// ProxyToUpscaler proxies requests to the upscaler service (admin-gated, port 8096).
// Only /api/upscale/* is exposed; internal segment-handle endpoints are
// Docker-network-only (D-05 security model).
func (h *ProxyHandler) ProxyToUpscaler(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "upscaler")
}

// ProxyToLibrary proxies requests to the library service (workstream raw-jp / v0.2).
// Phase 1 only exposes /health passthrough; Phases 2-5 add search + jobs + episodes
// endpoints. Admin-protected routes (POST /jobs, DELETE /jobs/:id, etc.) are added
// in later phases with JWTValidationMiddleware + AdminRoleMiddleware at the
// gateway router level.
func (h *ProxyHandler) ProxyToLibrary(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "library")
}

// ProxyToNotifications proxies requests to the notifications service (workstream
// notifications, v1.0 Phase 1 — see .planning/workstreams/notifications/ROADMAP.md).
// Only /api/notifications/* is exposed; /internal/notifications is reachable solely
// from inside the Docker network because this gateway never registers a route
// under /internal/* for it (D-05 security model).
func (h *ProxyHandler) ProxyToNotifications(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "notifications")
}

// ProxyToGacha proxies requests to the gacha service (workstream gacha / Лудка,
// Phase 1). Only /api/gacha/* is exposed; /internal/gacha/credit is reachable
// solely from inside the Docker network because this gateway never registers a
// route under /internal/* for it (D-05 security model).
func (h *ProxyHandler) ProxyToGacha(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "gacha")
}

// ProxyToAnalytics proxies clickstream ingestion to the analytics service
// (Plan 1). Only POST /api/analytics/collect is exposed — it is PUBLIC (no
// JWT) so anonymous visitors are tracked. The internal erasure endpoint
// (/internal/erase) is Docker-network-only and never routed here.
func (h *ProxyHandler) ProxyToAnalytics(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "analytics")
}

// ProxyToWatchTogether proxies REST requests to the watch-together service
// (workstream watch-together, v1.0 Phase 1). HTTP-only — the WebSocket
// endpoint at /api/watch-together/ws is served by a dedicated WS reverse
// proxy in transport/ws_proxy.go, NOT this handler. ProxyService.Forward
// strips RFC 7230 §6.1 hop-by-hop headers (including Upgrade/Connection)
// which is correct for normal HTTP but would break the WS handshake.
// Internal forward-compat route /internal/watch-together/* is NOT exposed
// at the gateway (Docker-network-only, mirroring the notifications D-05
// security model).
func (h *ProxyHandler) ProxyToWatchTogether(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "watch-together")
}

// ProxyToGrafana proxies requests to Grafana
func (h *ProxyHandler) ProxyToGrafana(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "grafana")
}

// ProxyToPrometheus proxies requests to Prometheus
func (h *ProxyHandler) ProxyToPrometheus(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "prometheus")
}

// ProxyToWeb proxies requests to the web service (Vue SPA via nginx).
// Used by /admin/recs/* so the SPA's admin debug page is reachable through
// the same /admin/ URL prefix as Grafana/Prometheus.
func (h *ProxyHandler) ProxyToWeb(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "web")
}

func (h *ProxyHandler) proxy(w http.ResponseWriter, r *http.Request, service string) {
	resp, err := h.proxyService.Forward(r, service)
	if err != nil {
		h.log.Errorw("proxy failed", "service", service, "error", err)
		// Make upstream failures observable per backend domain. This counter
		// had zero call sites, which is why a dropped-rotation refresh
		// (domain="auth") that logged users out was previously invisible.
		metrics.ProxyUpstreamErrors.WithLabelValues("forward_error", service).Inc()
		httputil.Error(w, err)
		return
	}
	defer resp.Body.Close()

	// An upstream 5xx that still produced a response — count it too, so the
	// auth-refresh failure mode is queryable as proxy_upstream_errors_total{domain="auth"}.
	if resp.StatusCode >= 500 {
		metrics.ProxyUpstreamErrors.WithLabelValues(strconv.Itoa(resp.StatusCode), service).Inc()
	}

	// Copy response headers, skipping CORS headers (gateway middleware handles CORS)
	for key, values := range resp.Header {
		if isCORSHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	_, _ = io.Copy(w, resp.Body)
}

// proxyStream is the streaming-body variant of proxy: it forwards via the
// no-total-timeout streamClient and clears the per-response write deadline so
// the gateway server's WriteTimeout does not truncate a long playback body
// (audit finding L466). On a backend with a short WriteTimeout the body would
// otherwise be cut off regardless of the client-side timeout split.
func (h *ProxyHandler) proxyStream(w http.ResponseWriter, r *http.Request, service string) {
	// Disable the server-level write deadline for this response. The gateway's
	// http.Server sets WriteTimeout (30s) which would otherwise re-cap the
	// streamed body. A zero time means "no deadline". SetWriteDeadline is
	// best-effort — if the ResponseWriter doesn't support it (e.g. some test
	// recorders) we simply proceed; the streamClient split still removes the
	// 15s client cap.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	resp, err := h.proxyService.ForwardStream(r, service)
	if err != nil {
		h.log.Errorw("stream proxy failed", "service", service, "error", err)
		metrics.ProxyUpstreamErrors.WithLabelValues("forward_error", service).Inc()
		httputil.Error(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		metrics.ProxyUpstreamErrors.WithLabelValues(strconv.Itoa(resp.StatusCode), service).Inc()
	}

	// Copy response headers, skipping CORS headers (gateway middleware handles CORS)
	for key, values := range resp.Header {
		if isCORSHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// isCORSHeader checks if a header is a CORS-related header
func isCORSHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Methods",
		"Access-Control-Max-Age",
		"Access-Control-Expose-Headers":
		return true
	}
	return false
}

// GetOpenAPISpec aggregates OpenAPI specs from all services
func (h *ProxyHandler) GetOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "AnimeEnigma API",
			"description": "API Gateway for AnimeEnigma Platform",
			"version":     "1.0.0",
		},
		"servers": []map[string]string{
			{"url": "/api/v1"},
		},
		"paths": map[string]interface{}{
			"/auth/*":      map[string]string{"description": "Authentication endpoints"},
			"/catalog/*":   map[string]string{"description": "Anime catalog endpoints"},
			"/player/*":    map[string]string{"description": "Player and watch history endpoints"},
			"/rooms/*":     map[string]string{"description": "Game rooms and leaderboard endpoints"},
			"/streaming/*": map[string]string{"description": "Video streaming endpoints"},
		},
	}

	httputil.OK(w, spec)
}
