package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

const (
	contentVerifyTTL        = 30 * time.Second // shorter than the FE poll (45s)
	contentVerifyCtxTimeout = 3 * time.Second
)

// verifyProxySource is the slice of capability.VerifyClient this handler needs.
type verifyProxySource interface {
	RawVerdicts(ctx context.Context, animeID string) (json.RawMessage, error)
	Hint(animeID, visitor, source string)
}

// ContentVerifyHandler serves GET /api/anime/{animeId}/content-verify — the
// aePlayer's dynamic verdict feed. Every request IS the +1 visit signal
// (spec §1): the hint fires before the cache check, deduped downstream.
type ContentVerifyHandler struct {
	src   verifyProxySource
	cache cache.Cache
	log   *logger.Logger
}

// NewContentVerifyHandler constructs the handler. cache and log may be nil.
func NewContentVerifyHandler(src verifyProxySource, c cache.Cache, log *logger.Logger) *ContentVerifyHandler {
	return &ContentVerifyHandler{src: src, cache: c, log: log}
}

// Get handles GET /api/anime/{animeId}/content-verify.
func (h *ContentVerifyHandler) Get(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		http.Error(w, "animeId required", http.StatusBadRequest)
		return
	}
	h.src.Hint(animeID, scraperUserKey(r), "visit")

	key := "contentverify:" + animeID
	var cached json.RawMessage
	if h.cache != nil {
		if err := h.cache.Get(r.Context(), key, &cached); err == nil && len(cached) > 0 {
			httputil.OK(w, cached)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), contentVerifyCtxTimeout)
	defer cancel()
	raw, err := h.src.RawVerdicts(ctx, animeID)
	if err != nil {
		// Degrade to an empty report — the FE treats it as all-unverified.
		httputil.OK(w, map[string]any{"anime_id": animeID, "providers": []any{}})
		return
	}
	if h.cache != nil {
		if err := h.cache.Set(r.Context(), key, json.RawMessage(raw), contentVerifyTTL); err != nil && h.log != nil {
			h.log.Warnw("content-verify cache set failed", "anime_id", animeID, "error", err)
		}
	}
	httputil.OK(w, json.RawMessage(raw))
}
