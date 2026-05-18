package handler

// Phase 06 (workstream raw-jp / v0.2). The library service POSTs to
// /internal/cache/invalidate/raw/{shikimoriId} after every successful
// encode so the catalog's hybrid resolver re-fetches the source
// decision instead of waiting out the 1h TTL.
//
// The route is mounted OUTSIDE /api with no auth middleware — it is
// reachable only from within the docker network because nginx/gateway
// does NOT proxy /internal/*. Same model as
// services/auth/internal/transport/router.go's /internal/resolve-api-key.

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

// AnimeRepoLike is the slice of *repo.AnimeRepository the handler
// needs. Exposed as an interface so tests can inject a fake without
// spinning up a real GORM DB.
type AnimeRepoLike interface {
	GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error)
}

// InternalCacheHandler implements POST /internal/cache/invalidate/raw/
// {shikimoriId}. Looks up the anime row by shikimori_id, then deletes
// the three raw:* cache families for that animeID. Idempotent: an
// unknown shikimori_id returns 200 with an empty status (the encoder
// may finish before the anime row exists in the catalog DB).
type InternalCacheHandler struct {
	cache     *cache.RedisCache
	animeRepo AnimeRepoLike
	log       *logger.Logger
}

// NewInternalCacheHandler constructs the handler. animeRepo must
// implement GetByShikimoriID — the production *repo.AnimeRepository
// satisfies it by structural typing.
func NewInternalCacheHandler(c *cache.RedisCache, animeRepo AnimeRepoLike, log *logger.Logger) *InternalCacheHandler {
	return &InternalCacheHandler{
		cache:     c,
		animeRepo: animeRepo,
		log:       log,
	}
}

// shikimoriIDPattern matches the canonical shikimori_id shape used
// across the catalog (numeric + occasional alpha prefix). The regex
// is deliberately strict to reject any path-traversal or
// injection-shaped input long before the value reaches Redis.
var shikimoriIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// InvalidateRaw handles POST /internal/cache/invalidate/raw/{shikimoriId}.
//
// Behavior:
//   - Validate the path param against shikimoriIDPattern. Bad input → 400.
//   - Look up the anime row by shikimori_id.
//     - Row missing → 200 idempotent. The encoder may have finished
//       before the catalog row was created.
//     - Repo error → 500.
//   - DELETE the three raw:* families keyed by the resolved animeID:
//       raw:source-decision:{animeID}:*   (SCAN + DEL via Invalidate)
//       raw:stream:{animeID}:*            (SCAN + DEL via Invalidate)
//       raw:episodes:{animeID}            (exact key, Delete)
//   - libs/cache.Invalidate does not return a count today; the
//     response body therefore omits the count. (Decision documented
//     in 06-PLAN.md Task 3.)
//
// Logs an info line on success so the operator can correlate a
// library encode with the catalog cache bust in the live logs.
func (h *InternalCacheHandler) InvalidateRaw(w http.ResponseWriter, r *http.Request) {
	shikimoriID := chi.URLParam(r, "shikimoriId")
	if shikimoriID == "" {
		httputil.BadRequest(w, "shikimoriId is required")
		return
	}
	if !shikimoriIDPattern.MatchString(shikimoriID) {
		httputil.BadRequest(w, "shikimoriId has invalid characters")
		return
	}

	anime, err := h.animeRepo.GetByShikimoriID(r.Context(), shikimoriID)
	if err != nil {
		// Repo error (NOT not-found — GetByShikimoriID returns nil,
		// nil for not-found per the existing convention).
		httputil.Error(w, err)
		return
	}
	if anime == nil {
		// Idempotent: encoder finished before catalog learned about
		// the anime. Nothing to invalidate.
		if h.log != nil {
			h.log.Infow("raw: cache invalidate — anime row missing; idempotent ack",
				"shikimori_id", shikimoriID)
		}
		httputil.OK(w, map[string]any{"status": "ok", "found": false})
		return
	}

	ctx := r.Context()

	// 1. SCAN + DEL raw:source-decision:{animeID}:*
	srcPattern := fmt.Sprintf("%s:%s:*", service.CacheKeySourceDecision, anime.ID)
	if err := h.cache.Invalidate(ctx, srcPattern); err != nil && h.log != nil {
		h.log.Warnw("raw: source-decision invalidate failed", "pattern", srcPattern, "error", err)
	}

	// 2. SCAN + DEL raw:stream:{animeID}:*
	streamPattern := fmt.Sprintf("%s:%s:*", service.CacheKeyStream, anime.ID)
	if err := h.cache.Invalidate(ctx, streamPattern); err != nil && h.log != nil {
		h.log.Warnw("raw: stream invalidate failed", "pattern", streamPattern, "error", err)
	}

	// 3. DEL raw:episodes:{animeID}  (exact)
	episodesKey := fmt.Sprintf("%s:%s", service.CacheKeyEpisodes, anime.ID)
	if err := h.cache.Delete(ctx, episodesKey); err != nil && h.log != nil {
		h.log.Warnw("raw: episodes delete failed", "key", episodesKey, "error", err)
	}

	if h.log != nil {
		h.log.Infow("raw: cache invalidated",
			"shikimori_id", shikimoriID,
			"anime_id", anime.ID,
		)
	}
	httputil.OK(w, map[string]any{"status": "ok", "found": true})
}
