package transport

import (
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/controlplane"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// internalGatewayHeader is the header the gateway injects on /api/upscale/*
// admin-proxied requests. It proves the request arrived via the gateway's
// admin-gated path (JWT + AdminRole) rather than directly from the ext edge.
//
// The ext edge proxies /worker/* only — it cannot set this header because
// the gateway's ExternalAPIKeyMiddleware group never injects it. A direct
// caller reaching upscaler:8096 without going through the admin-gated gateway
// would need to know this secret-like header to reach the admin API, providing
// defense-in-depth for the admin surface. Phase 2 should replace this with
// proper mTLS or a rotating signed header; for Phase 1 it keeps the admin
// surface distinct from the worker surface.
//
// The gateway sets X-Gateway-Internal: "1" on /api/upscale/* proxied requests
// (see services/gateway/internal/service/proxy.go — forwardWith injects it
// when service=="upscaler" via the standard copyForwardHeaders path). Until
// that injection lands in a later task, this gate serves as a documented
// separation point: see docs/upscaler-edge-setup.md §Backend defense-in-depth.
const internalGatewayHeader = "X-Gateway-Internal"

// requireGatewayInternal is middleware that ensures the request came through
// the gateway's admin-gated /api/upscale/* proxy path. Direct calls to
// upscaler:8096/api/upscale/* without the header are rejected with 404
// (not 401 — we don't reveal that there's a gate here to an unauthenticated
// caller who somehow reached the Docker-internal port).
//
// FOLLOW-UP (Phase 2): replace this header check with gateway-injected
// X-Gateway-Internal signed token (HMAC-SHA256, rotated per-deploy) so
// a leaked Docker network access cannot trivially impersonate the gateway
// by setting a known static header value. Task 6 establishes the separation
// contract; the signing is a separate hardening step.
func requireGatewayInternal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(internalGatewayHeader) == "" {
			// 404, not 401 — avoid revealing the existence of the gate to
			// callers who reached the Docker-internal port directly.
			httputil.NotFound(w, "not found")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NewRouter returns the HTTP handler for the upscaler service.
// Surface separation:
//
//   - /worker/{enroll,ws,segments/*} — worker-facing routes (Tasks 5/7+).
//     These are reached from the internet via the ext.animeenigma.org edge
//     (gateway /worker/* → upscaler). No JWT required here; auth is the
//     API-key + session/capability chain.
//
//   - /api/upscale/* — admin-facing routes (Task 4+). These are reached only
//     via the gateway's admin-gated /api/upscale/* → upscaler proxy (JWT +
//     AdminRole). requireGatewayInternal ensures the admin surface is not
//     served to a caller that bypasses the gateway and dials upscaler:8096
//     directly. FOLLOW-UP: sign the injected header in Phase 2.
func NewRouter(
	log *logger.Logger,
	metricsCollector *metrics.Collector,
	hub *controlplane.Hub,
	enrollStore *controlplane.GormEnrollStore,
) http.Handler {
	r := chi.NewRouter()

	// Middleware chain mirrors services/themes/internal/transport/router.go
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	// Health check — reachable without any gate so the Docker healthcheck +
	// ops probes work without credentials (mirrors library, notifications, etc.)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// Metrics endpoint
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// Admin API routes (/api/upscale/*) — filled in later tasks (Task 4+).
	// requireGatewayInternal gates this group so it is only reachable via
	// the gateway's admin-proxied path (X-Gateway-Internal header injected
	// by the gateway on /api/upscale/* → upscaler proxying). A direct dial
	// to upscaler:8096/api/upscale/* without the header returns 404.
	r.Route("/api/upscale", func(r chi.Router) {
		r.Use(requireGatewayInternal)
		// placeholder — handlers wired in Task 4+
	})

	// Worker routes (/worker/*) — worker-facing enroll + WS upgrade.
	// These are reached from the internet via the gateway's /worker/* group
	// (ExternalAPIKeyMiddleware + WS proxy). No additional gate here; auth
	// is the API-key (gateway) + session/capability chain.
	r.Route("/worker", func(r chi.Router) {
		// POST /worker/enroll — one-time-token enroll flow.
		// Uses GormEnrollStore.EnrollTx directly (transactional, durable
		// single-use); NOT Handle+GormEnrollStore (non-transactional footgun).
		r.Post("/enroll", func(w http.ResponseWriter, r *http.Request) {
			var req controlplane.EnrollRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}
			resp, err := enrollStore.EnrollTx(r.Context(), req, controlplane.SessionTTL)
			if err != nil {
				if err == controlplane.ErrTokenNotFound {
					http.Error(w, "token not found or already used", http.StatusUnauthorized)
					return
				}
				log.Warnw("enroll: EnrollTx error", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		})

		// GET /worker/ws — WebSocket upgrade for the WS control-plane.
		// Session verified via ?worker_id=&exp=&sig= query params.
		r.Get("/ws", controlplane.UpgradeHandler(hub))
	})

	return r
}
