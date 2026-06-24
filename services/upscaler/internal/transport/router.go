package transport

import (
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/controlplane"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/handler"
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
// when service=="upscaler" via the standard copyForwardHeaders path);
// the gateway injects X-Gateway-Internal on all /api/upscale/* proxy requests
// (injected by the gateway's admin-gated proxy); this gate is the server-side
// enforcement of that contract.
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
	segmentHandler *handler.SegmentHandler,
	adminHandler *handler.AdminHandler,
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

	// Admin API routes (/api/upscale/*) — CRUD for jobs + fleet status.
	// requireGatewayInternal gates this group so it is only reachable via
	// the gateway's admin-proxied path (X-Gateway-Internal header injected
	// by the gateway on /api/upscale/* → upscaler proxying). A direct dial
	// to upscaler:8096/api/upscale/* without the header returns 404.
	r.Route("/api/upscale", func(r chi.Router) {
		r.Use(requireGatewayInternal)

		if adminHandler != nil {
			// Job CRUD
			r.Post("/jobs", adminHandler.CreateJob)
			r.Get("/jobs", adminHandler.ListJobs)
			r.Get("/jobs/{id}", adminHandler.GetJob)
			r.Post("/jobs/{id}/cancel", adminHandler.CancelJob)
			r.Post("/jobs/{id}/retry", adminHandler.RetryJob)
			// Job log ring-buffer (REST tail + SSE stream)
			r.Get("/jobs/{id}/logs", adminHandler.GetJobLogs)
			r.Get("/jobs/{id}/logs/stream", adminHandler.StreamJobLogs)
			// Fleet status + control-plane commands
			r.Get("/workers", adminHandler.ListWorkers)
			r.Post("/workers/{id}/commands", adminHandler.PostWorkerCommand)
		}
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
			// Nil-guard: a misconfigured wiring (or a test that passes nil)
			// must return a clean 500, not panic on the EnrollTx dereference (M-3).
			if enrollStore == nil {
				log.Errorw("enroll: enrollStore is nil — service misconfigured")
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
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

		// Segment data plane (Task 11b) — the sole consumer of the capability
		// verification. GET downloads the leased INPUT segment; PUT stores the
		// upscaled OUTPUT segment. Auth is the per-segment HMAC capability handle
		// (?exp=&sig=) bound to job+operation+idx — NOT the gateway header, NOT
		// JWT. The SegmentHandler itself performs all 7 security controls
		// (capability verify, idx bound-check, traversal defense, lease ownership,
		// body cap, anti-overwrite/finalized guard, generic errors).
		//
		// Nil-guard: a misconfigured wiring must 503 cleanly, not panic on the
		// handler method dispatch.
		if segmentHandler != nil {
			r.Get("/segments/{job}/{idx}", segmentHandler.GetSegment)
			r.Put("/segments/{job}/{idx}", segmentHandler.PutSegment)
		} else {
			log.Warnw("segment data-plane handler not wired — /worker/segments/* disabled")
			segDisabled := func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "segment data plane unavailable", http.StatusServiceUnavailable)
			}
			r.Get("/segments/{job}/{idx}", segDisabled)
			r.Put("/segments/{job}/{idx}", segDisabled)
		}
	})

	return r
}
