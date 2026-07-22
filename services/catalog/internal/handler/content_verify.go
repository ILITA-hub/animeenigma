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
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/capability"
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

// verifyReportSource is the slice of *capability.Service this handler needs: the
// capability report supplies the ae/kodik verdicts synthesized at read time
// (content-verify no longer stores them). May be nil (then no synth is merged).
type verifyReportSource interface {
	Report(ctx context.Context, animeID string) (domain.CapabilityReport, error)
}

// ContentVerifyHandler serves GET /api/anime/{animeId}/content-verify — the
// aePlayer's dynamic verdict feed. Every request IS the +1 visit signal
// (spec §1): the hint fires before the cache check, deduped downstream. The
// content-verify store's per-provider verdicts are merged with the ae/kodik
// verdicts synthesized from the capability report (known truth, independent of
// content-verify availability).
type ContentVerifyHandler struct {
	src     verifyProxySource
	reports verifyReportSource
	cache   cache.Cache
	log     *logger.Logger
}

// NewContentVerifyHandler constructs the handler. reports, cache and log may be nil.
func NewContentVerifyHandler(src verifyProxySource, reports verifyReportSource, c cache.Cache, log *logger.Logger) *ContentVerifyHandler {
	return &ContentVerifyHandler{src: src, reports: reports, cache: c, log: log}
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

	// ae/kodik verdicts are known truth synthesized from the capability report,
	// independent of content-verify availability. Best-effort: a nil source or a
	// missing/slow report just yields no synth, and the store's providers proxy
	// through unchanged.
	var report domain.CapabilityReport
	if h.reports != nil {
		rctx, rcancel := context.WithTimeout(r.Context(), contentVerifyCtxTimeout)
		if rep, rerr := h.reports.Report(rctx, animeID); rerr == nil {
			report = rep
		} else if h.log != nil {
			h.log.Warnw("content-verify report fetch failed; ae/kodik synth skipped", "anime_id", animeID, "error", rerr)
		}
		rcancel()
	}

	ctx, cancel := context.WithTimeout(r.Context(), contentVerifyCtxTimeout)
	defer cancel()
	raw, err := h.src.RawVerdicts(ctx, animeID)
	// On a content-verify failure (down / non-200 / kill switch) raw is empty;
	// MergeSynthVerdicts still returns the ae/kodik synth so known truth surfaces
	// (and degrades to {providers:[]} when the report is empty too).
	merged := capability.MergeSynthVerdicts(animeID, raw, report)
	if err == nil && h.cache != nil {
		// Cache only healthy responses — never pin a degraded (content-verify
		// down) payload for the full TTL.
		if serr := h.cache.Set(r.Context(), key, merged, contentVerifyTTL); serr != nil && h.log != nil {
			h.log.Warnw("content-verify cache set failed", "anime_id", animeID, "error", serr)
		}
	}
	httputil.OK(w, merged)
}
