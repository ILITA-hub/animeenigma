// Package handler — scraper endpoints (Phase 15 plan 04).
//
// The four /scraper/* endpoints (episodes/servers/stream/health) are
// implemented on a dedicated ScraperEndpointsHandler that the public
// *CatalogHandler embeds. The split keeps the handler logic testable
// against a small scraperServiceAPI interface without requiring the full
// *service.CatalogService dependency tree (which needs real GORM repos).
package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/streamsign"
	"github.com/go-chi/chi/v5"
)

// scraperServiceAPI is the minimal interface the scraper handlers need
// from the catalog service. *service.CatalogService satisfies it.
type scraperServiceAPI interface {
	GetScraperEpisodes(ctx context.Context, animeID, prefer string, exclusive bool) (int, []byte, error)
	GetScraperServers(ctx context.Context, animeID, episodeID, prefer string, exclusive bool) (int, []byte, error)
	GetScraperStream(ctx context.Context, animeID, episodeID, serverID, category, prefer string, exclusive bool, userKey string) (int, []byte, error)
	GetScraperHealth(ctx context.Context) (int, []byte, error)
}

// ScraperServiceAPI is the public alias for cross-package consumers
// (the transport-layer tests use this to build stub services).
type ScraperServiceAPI = scraperServiceAPI

// ScraperEndpointsHandler holds the four /scraper/* handler methods. It
// is embedded inside *CatalogHandler so /scraper/* routes hang off the
// same chi mount as the rest of the catalog routes.
type ScraperEndpointsHandler struct {
	scraperSvc scraperServiceAPI
	log        *logger.Logger
}

// WireScraperEndpoints sets the dependencies on a ScraperEndpointsHandler.
// Exposed so transport-layer route tests can wire a stub service without
// the constructor exporting a setter pair.
func WireScraperEndpoints(h *ScraperEndpointsHandler, svc scraperServiceAPI, log *logger.Logger) {
	h.scraperSvc = svc
	h.log = log
}

// GetScraperEpisodes handles GET /api/anime/{animeId}/scraper/episodes.
// Resolves the UUID to MAL ID via the service layer and forwards to the
// scraper microservice. Returns status + body verbatim.
func (h *ScraperEndpointsHandler) GetScraperEpisodes(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}
	prefer := r.URL.Query().Get("prefer")
	exclusive := r.URL.Query().Get("exclusive") == "true"

	status, body, err := h.scraperSvc.GetScraperEpisodes(r.Context(), animeID, prefer, exclusive)
	if err != nil {
		h.writeScraperError(w, err)
		return
	}
	writePassthrough(w, status, body)
}

// GetScraperServers handles GET /api/anime/{animeId}/scraper/servers?episode=...
func (h *ScraperEndpointsHandler) GetScraperServers(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}
	episodeID := r.URL.Query().Get("episode")
	if episodeID == "" {
		httputil.BadRequest(w, "episode ID is required")
		return
	}
	prefer := r.URL.Query().Get("prefer")
	exclusive := r.URL.Query().Get("exclusive") == "true"

	status, body, err := h.scraperSvc.GetScraperServers(r.Context(), animeID, episodeID, prefer, exclusive)
	if err != nil {
		h.writeScraperError(w, err)
		return
	}
	writePassthrough(w, status, body)
}

// GetScraperStream handles GET /api/anime/{animeId}/scraper/stream?episode=...&server=...&category=...
// category defaults to "sub".
func (h *ScraperEndpointsHandler) GetScraperStream(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}
	episodeID := r.URL.Query().Get("episode")
	if episodeID == "" {
		httputil.BadRequest(w, "episode ID is required")
		return
	}
	serverID := r.URL.Query().Get("server")
	if serverID == "" {
		httputil.BadRequest(w, "server ID is required")
		return
	}
	category := r.URL.Query().Get("category")
	if category == "" {
		category = "sub"
	}
	prefer := r.URL.Query().Get("prefer")
	exclusive := r.URL.Query().Get("exclusive") == "true"

	userKey := scraperUserKey(r)
	status, body, err := h.scraperSvc.GetScraperStream(r.Context(), animeID, episodeID, serverID, category, prefer, exclusive, userKey)
	if err != nil {
		h.writeScraperError(w, err)
		return
	}
	// Sign external stream/subtitle URLs so the HLS proxy trusts them without a
	// host allowlist. No-op on error/non-200 bodies; preserves the envelope shape.
	body = streamsign.SignScraperStreamBody(status, body)
	writePassthrough(w, status, body)
}

// GetScraperHealth handles GET /api/anime/{animeId}/scraper/health.
// The path-level animeId is structural symmetry only — the service-wide
// scraper-health endpoint doesn't actually look up the anime.
func (h *ScraperEndpointsHandler) GetScraperHealth(w http.ResponseWriter, r *http.Request) {
	status, body, err := h.scraperSvc.GetScraperHealth(r.Context())
	if err != nil {
		h.writeScraperError(w, err)
		return
	}
	writePassthrough(w, status, body)
}

// writePassthrough writes the scraper's status + body verbatim. This
// preserves the exact JSON shape the scraper produces, which catalog
// + frontend match against contractually.
func writePassthrough(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeScraperError maps service-layer errors to HTTP responses:
//   - liberrors.NotFound (anime UUID not in DB)        -> 404
//   - service.ErrMalIDUnavailable (no mal_id on row)   -> 422
//   - anything else                                    -> httputil.Error
//     (which renders 500 unless the error already carries an AppError
//     status code).
//
// REVIEW.md WR-03: the per-handler `log` field is now used for breadcrumb
// logging on unexpected error paths so operators have observability when
// the catalog↔scraper hop starts emitting 500s in production.
func (h *ScraperEndpointsHandler) writeScraperError(w http.ResponseWriter, err error) {
	// Anime not found in the catalog → 404 NotFound.
	if appErr, ok := liberrors.IsAppError(err); ok && appErr.Code == liberrors.CodeNotFound {
		httputil.Error(w, err)
		return
	}
	// mal_id missing → 422 Unprocessable Entity with the canonical body.
	if errors.Is(err, service.ErrMalIDUnavailable) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "mal_id unavailable for this anime"})
		return
	}
	// Unexpected error path — log a breadcrumb so the operator can correlate.
	if h != nil && h.log != nil {
		h.log.Errorw("scraper endpoint error", "error", err)
	}
	httputil.Error(w, err)
}

// scraperUserKey derives the opaque per-user quota key forwarded to the
// stealth-scraper sidecar (via the scraper service's X-AE-User header):
//   - authenticated → "u:" + the JWT user id (OptionalAuthMiddleware populated
//     claims when a Bearer token was sent);
//   - anonymous → "ip:" + sha256(clientIP | salt | UTC-day) so anon traffic is
//     still bounded per source, never globally shared, and the raw IP is never
//     forwarded or logged. Salt = CATALOG_IP_SALT (empty salt still hashes).
//
// Returns "" only when there is neither an authed user nor a usable client IP.
func scraperUserKey(r *http.Request) string {
	if uid := authz.UserIDFromContext(r.Context()); uid != "" {
		return "u:" + uid
	}
	ip := clientIP(r)
	if ip == "" {
		return ""
	}
	day := time.Now().UTC().Format("2006-01-02")
	sum := sha256.Sum256([]byte(ip + "|" + os.Getenv("CATALOG_IP_SALT") + "|" + day))
	return "ip:" + hex.EncodeToString(sum[:])
}

// clientIP extracts the best-effort client IP. The catalog router runs
// chi middleware.RealIP, which rewrites r.RemoteAddr from X-Forwarded-For /
// X-Real-IP, so RemoteAddr is the trusted source here.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
