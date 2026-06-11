// Package handler — recs.go: GET /api/users/recs.
//
// Phase 10 surfaced the anonymous trending row. Phase 11 branches on auth
// state: anonymous callers still get the single shared trending top-N
// (row_label_key=recs.trending), while logged-in callers get a personalized
// "Up Next for you" row (row_label_key=recs.upNext) computed from the full
// 0.30·S1 + 0.20·S2 + 0.20·S3 + 0.10·S4 + 0.20·S5 Phase-12 ensemble, with
// any anime already in the user's anime_list (any status) excluded by
// S11.CandidatePoolForUser — signals still read the list to score affinity,
// they just don't recommend it back at the user.
package handler

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs/signals"
	"gorm.io/gorm"
)

// recsCache is the cache surface RecsHandler depends on. *libs/cache.RedisCache
// satisfies this interface. Tests inject a fake.
type recsCache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
}

// PublicTrendingKey is the Redis key for the anonymous shared trending top-N.
// Single shared key — anonymous personalization is deferred to v2.1.
const PublicTrendingKey = "recs:public:trending:topN"

// publicTrendingTTL — 6h per CONTEXT.md decisions §Redis cache. The 60-min
// population cron updates the underlying signals more frequently than this,
// so the cache TTL is the dominant freshness window.
const publicTrendingTTL = 6 * time.Hour

// anonRowSliceSize — Phase 10 anonymous returns top-20.
const anonRowSliceSize = 20

// userTopNTTL — Phase 11 logged-in cache window. 6h matches the user-signal
// cron cadence; the on-write debounce trigger busts the cache early when a
// new watch_history row lands.
const userTopNTTL = 6 * time.Hour

// userRowSliceSize — Phase 11 server returns top-50 for logged-in (the
// frontend slices to 20 per spec §13).
const userRowSliceSize = 50

// userTopNKey returns the per-user topN cache key in canonical shape so the
// handler / cron / trigger paths all agree. Delegates to recs.UserTopNKey so
// the cache-key version (:v2 suffix; see UserTopNKeySuffix) stays in lockstep
// across handler, orchestrator, and admin paths.
func userTopNKey(userID string) string {
	return recs.UserTopNKey(recs.UserID(userID))
}

// RecAnimePayload is the anime fields the frontend AnimeCard needs to render.
// Mirrors the home store payload shape so AnimeCard can render without changes.
type RecAnimePayload struct {
	ID            string  `json:"id"`
	Name          string  `json:"name,omitempty"`
	NameRU        string  `json:"name_ru,omitempty"`
	NameJP        string  `json:"name_jp,omitempty"`
	PosterURL     string  `json:"poster_url,omitempty"`
	Score         float64 `json:"score,omitempty"`
	EpisodesCount int     `json:"episodes_count,omitempty"`
	Status        string  `json:"status,omitempty"`
	Year          int     `json:"year,omitempty"`
}

// RecItem is one row in the response array.
//
// Phase 13 (REC-UX-03) extension: when Pinned=true (only the recs[0] item
// when S6 fires), PinReason / PinSeedAnimeID / PinSource carry the
// "Because you finished {name}" copy + admin-debug context. Final is 0 for
// the pinned row (the JSON-zero approximation of "null" since RecItem.Final
// is float64; the frontend gates display on Pinned, NOT Final, so this is
// the spec §B7 deviation).
//
// Phase 14 (REC-EVAL-01) extension: TopContributor surfaces the click-time
// signal_id so the frontend can tag rec_click events without a separate
// fetch. For pinned rows this stays empty — the frontend uses the literal
// "s6_pin" string per locked Phase 13 hand-off.
type RecItem struct {
	Anime          RecAnimePayload `json:"anime"`
	Final          float64         `json:"final"`
	Pinned         bool            `json:"pinned"`
	PinReason      string          `json:"pin_reason,omitempty"`        // Phase 13 (REC-UX-03) — legacy English fallback
	PinReasonKey   string          `json:"pin_reason_key,omitempty"`    // UX-09 (Phase 3) — i18n key
	PinReasonData  map[string]any  `json:"pin_reason_data,omitempty"`   // UX-09 (Phase 3) — interpolation values for PinReasonKey
	PinSeedAnimeID string          `json:"pin_seed_anime_id,omitempty"` // Phase 13 (REC-UX-03)
	PinSource      string          `json:"pin_source,omitempty"`        // Phase 13 (REC-UX-03)
	Rank           int             `json:"rank"`
	TopContributor string          `json:"top_contributor,omitempty"` // Phase 14 (REC-EVAL-01)
}

// RecsEnvelope is the data field of the API response (wrapped by httputil.OK).
type RecsEnvelope struct {
	Recs        []RecItem `json:"recs"`
	GeneratedAt string    `json:"generated_at"`
	CacheHit    bool      `json:"cache_hit"`
	Total       int       `json:"total"`
	RowLabelKey string    `json:"row_label_key"`
}

// RecsHandler serves GET /api/users/recs.
type RecsHandler struct {
	db    *gorm.DB
	repo  *repo.RecsRepository
	cache recsCache
	log   *logger.Logger

	// Composed signals — built once at construction. The DB handle is enough
	// to wire them.
	s11 *signals.S11Filter
	s3  *signals.S3Trending
	s4  *signals.S4Recency
	s1  *signals.S1ScoreCluster
	s2  *signals.S2Metadata
	s5  *signals.S5Attribute // Phase 12 (REC-SIG-05) — TF-IDF attribute affinity
	s6  *signals.S6ComboPin  // Phase 13 (REC-SIG-06) — combo-watched-after pin resolver; may be nil
}

// NewRecsHandler wires the handler with its dependencies. The signal modules
// are constructed once here (cheap; just struct literals + the DB handle).
//
// The s6 argument may be nil when the caller doesn't want the S6 pin
// surface (e.g. an ensemble-only test fixture); computeFreshForUser
// nil-guards before invoking.
func NewRecsHandler(db *gorm.DB, recsRepo *repo.RecsRepository, cache recsCache, s6 *signals.S6ComboPin, log *logger.Logger) *RecsHandler {
	return &RecsHandler{
		db:    db,
		repo:  recsRepo,
		cache: cache,
		log:   log,
		s11:   signals.NewS11Filter(db),
		s3:    signals.NewS3Trending(db, recsRepo),
		s4:    signals.NewS4Recency(db),
		s1:    signals.NewS1ScoreCluster(db, recsRepo),
		s2:    signals.NewS2Metadata(db),
		s5:    signals.NewS5Attribute(db, recsRepo), // Phase 12 (REC-SIG-05)
		s6:    s6,                                   // Phase 13 (REC-SIG-06)
	}
}

// GetRecs serves the recs row. Branches on JWT presence:
//
//   - Logged-in (claims.UserID != "") -> personalized "Up Next for you" row
//     via serveLoggedIn (REC-UX-01 / REC-UX-04).
//   - Anonymous -> shared "Trending now" row via serveAnonymous (Phase 10
//     contract preserved).
func (h *RecsHandler) GetRecs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if claims, ok := authz.ClaimsFromContext(ctx); ok && claims != nil && claims.UserID != "" {
		h.serveLoggedIn(ctx, w, claims.UserID)
		return
	}
	h.serveAnonymous(ctx, w)
}

// serveAnonymous serves the Phase 10 trending row. Cache flow:
//  1. Try recsCache.Get(PublicTrendingKey).
//  2. Hit  -> set CacheHit=true and return.
//  3. Miss -> compute via S11.CandidatePool + Ensemble(S3=0.20, S4=0.10),
//     sort with S4-tiebreak backfill, slice to top-20, hydrate anime info,
//     write to cache with 6h TTL, return.
func (h *RecsHandler) serveAnonymous(ctx context.Context, w http.ResponseWriter) {
	// 1. Cache read-through
	var cached RecsEnvelope
	err := h.cache.Get(ctx, PublicTrendingKey, &cached)
	if err == nil {
		cached.CacheHit = true
		httputil.OK(w, cached)
		return
	}
	if err != nil && !isCacheMiss(err) {
		h.log.Warnw("recs cache read failed; recomputing", "error", err)
	}

	// 2. Compute fresh
	envelope, err := h.computeFresh(ctx)
	if err != nil {
		h.log.Errorw("recs compute failed", "error", err)
		httputil.OK(w, RecsEnvelope{
			Recs:        []RecItem{},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			CacheHit:    false,
			Total:       0,
			RowLabelKey: "recs.trending",
		})
		return
	}

	// 3. Cache the result
	if setErr := h.cache.Set(ctx, PublicTrendingKey, envelope, publicTrendingTTL); setErr != nil {
		h.log.Warnw("recs cache write failed", "error", setErr)
	}

	httputil.OK(w, envelope)
}

// serveLoggedIn serves the personalized Up Next row. Per-user cache,
// CandidatePoolForUser (excluding any anime already in the user's list),
// full ensemble.
func (h *RecsHandler) serveLoggedIn(ctx context.Context, w http.ResponseWriter, userID string) {
	// 1. Cache read-through (per-user key).
	var cached RecsEnvelope
	if err := h.cache.Get(ctx, userTopNKey(userID), &cached); err == nil {
		cached.CacheHit = true
		httputil.OK(w, cached)
		return
	} else if !isCacheMiss(err) {
		h.log.Warnw("personalized recs cache read failed; recomputing",
			"user_id", userID, "error", err)
	}

	// 2. Compute fresh
	envelope, err := h.computeFreshForUser(ctx, userID)
	if err != nil {
		h.log.Errorw("personalized recs compute failed",
			"user_id", userID, "error", err)
		httputil.OK(w, RecsEnvelope{
			Recs:        []RecItem{},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			CacheHit:    false,
			Total:       0,
			RowLabelKey: "recs.upNext",
		})
		return
	}

	// 3. Cache (per-user, 6h TTL).
	if setErr := h.cache.Set(ctx, userTopNKey(userID), envelope, userTopNTTL); setErr != nil {
		h.log.Warnw("personalized recs cache write failed",
			"user_id", userID, "error", setErr)
	}

	httputil.OK(w, envelope)
}

// isCacheMiss is a soft check for the conventional ErrNotFound returned by
// libs/cache. The fake test cache returns a wrapped error with the same
// substring; production returns the cache.ErrNotFound sentinel. Either way,
// we treat it as a cache miss and recompute.
func isCacheMiss(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errCacheNotFound) || err.Error() == "cache: key not found"
}

// errCacheNotFound is a local sentinel used only for type-checking in
// isCacheMiss. The handler intentionally does NOT import libs/cache here so
// the recs package interface stays narrow; we match by error string.
var errCacheNotFound = errors.New("cache: key not found")

// computeFresh runs the full ensemble pipeline.
func (h *RecsHandler) computeFresh(ctx context.Context) (RecsEnvelope, error) {
	pool, err := h.s11.CandidatePool(ctx)
	if err != nil {
		return RecsEnvelope{}, err
	}
	if len(pool) == 0 {
		return RecsEnvelope{
			Recs:        []RecItem{},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			CacheHit:    false,
			Total:       0,
			RowLabelKey: "recs.trending",
		}, nil
	}

	ensemble := recs.NewEnsemble([]recs.WeightedSignal{
		{Module: h.s3, Weight: 0.20},
		{Module: h.s4, Weight: 0.10},
	})
	ranked, err := ensemble.Rank(ctx, "", pool)
	if err != nil {
		return RecsEnvelope{}, err
	}

	// Thin-S3 backfill: when the S3 pool is empty/sparse, the per-pool
	// normalizer returns all-zeros for S3, so Final reduces to 0.10*S4_norm.
	// To produce a stable ordering — including ties at 0 — re-sort with a
	// (Final desc, S4 breakdown desc, AnimeID asc) tiebreak chain. This is
	// always safe to do (pure-S3 ordering is preserved when S3 is dense)
	// and produces the expected backfill shape for thin pools.
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Final != ranked[j].Final {
			return ranked[i].Final > ranked[j].Final
		}
		s4i := float64(ranked[i].Breakdown[recs.SignalID("s4")])
		s4j := float64(ranked[j].Breakdown[recs.SignalID("s4")])
		if s4i != s4j {
			return s4i > s4j
		}
		return ranked[i].AnimeID < ranked[j].AnimeID
	})

	// Slice to top-20 (or fewer if pool is small)
	end := anonRowSliceSize
	if len(ranked) < end {
		end = len(ranked)
	}
	top := ranked[:end]

	// Hydrate anime metadata
	ids := make([]string, len(top))
	for i, r := range top {
		ids[i] = r.AnimeID
	}
	hydrated, err := h.hydrateAnime(ctx, ids)
	if err != nil {
		return RecsEnvelope{}, err
	}

	items := make([]RecItem, 0, len(top))
	// Phase 14 (REC-EVAL-01): trending row uses S3 + S4 only; derive
	// top_contributor for the click telemetry pipeline. Default to "s3"
	// when both signals are 0 so the click-time signal_id is never empty.
	trendingWeights := []recs.WeightedSignal{
		{Module: h.s3, Weight: 0.20},
		{Module: h.s4, Weight: 0.10},
	}
	for i, r := range top {
		anime, ok := hydrated[r.AnimeID]
		if !ok {
			// Anime row vanished between S11 and hydrate — unlikely but
			// possible under concurrent edits. Skip silently.
			continue
		}
		topSig := deriveTopContributor(r.Breakdown, trendingWeights)
		if topSig == "" {
			topSig = "s3"
		}
		items = append(items, RecItem{
			Anime:          anime,
			Final:          r.Final,
			Pinned:         false,
			Rank:           i + 1,
			TopContributor: topSig,
		})
	}

	return RecsEnvelope{
		Recs:        items,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		CacheHit:    false,
		Total:       len(items),
		RowLabelKey: "recs.trending",
	}, nil
}

// deriveTopContributor returns the SignalID with the largest weighted
// contribution to a Recommendation's Final score. Mirrors the
// RankWithBreakdown TopContributor logic but operates on the narrow
// Breakdown map the public Rank API exposes. Phase 14 (REC-EVAL-01).
//
// Returns "" only when weights is empty.
func deriveTopContributor(breakdown map[recs.SignalID]recs.NormalizedScore, weights []recs.WeightedSignal) string {
	if len(weights) == 0 {
		return ""
	}
	var topSig recs.SignalID
	topVal := -1.0
	for _, ws := range weights {
		w := ws.Weight * float64(breakdown[ws.Module.ID()])
		if w > topVal {
			topVal = w
			topSig = ws.Module.ID()
		}
	}
	return string(topSig)
}

// computeFreshForUser runs the personalized ensemble for a logged-in user:
// S11.CandidatePoolForUser (excludes any anime already in the user's list) ->
// full Phase-12 ensemble 0.30·S1 + 0.20·S2 + 0.20·S3 + 0.10·S4 + 0.20·S5 ->
// stable sort -> top-50 server slice -> hydrate -> envelope with
// row_label_key=recs.upNext.
func (h *RecsHandler) computeFreshForUser(ctx context.Context, userID string) (RecsEnvelope, error) {
	pool, err := h.s11.CandidatePoolForUser(ctx, recs.UserID(userID))
	if err != nil {
		return RecsEnvelope{}, err
	}
	if len(pool) == 0 {
		return RecsEnvelope{
			Recs:        []RecItem{},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			CacheHit:    false,
			Total:       0,
			RowLabelKey: "recs.upNext",
		}, nil
	}

	ensemble := recs.NewEnsemble([]recs.WeightedSignal{
		{Module: h.s1, Weight: 0.30},
		{Module: h.s2, Weight: 0.20},
		{Module: h.s3, Weight: 0.20},
		{Module: h.s4, Weight: 0.10},
		{Module: h.s5, Weight: 0.20}, // Phase 12 (REC-SIG-05)
	})
	ranked, err := ensemble.Rank(ctx, recs.UserID(userID), pool)
	if err != nil {
		return RecsEnvelope{}, err
	}

	// Stable secondary sort matching the Phase 10 thin-pool pattern: tiebreak
	// by S4 then AnimeID so cold-start users get deterministic ordering.
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Final != ranked[j].Final {
			return ranked[i].Final > ranked[j].Final
		}
		s4i := float64(ranked[i].Breakdown[recs.SignalID("s4")])
		s4j := float64(ranked[j].Breakdown[recs.SignalID("s4")])
		if s4i != s4j {
			return s4i > s4j
		}
		return ranked[i].AnimeID < ranked[j].AnimeID
	})

	end := userRowSliceSize
	if len(ranked) < end {
		end = len(ranked)
	}
	top := ranked[:end]

	ids := make([]string, len(top))
	for i, r := range top {
		ids[i] = r.AnimeID
	}
	hydrated, err := h.hydrateAnime(ctx, ids)
	if err != nil {
		return RecsEnvelope{}, err
	}

	items := make([]RecItem, 0, len(top))
	// Phase 14 (REC-EVAL-01): derive top_contributor per item so the
	// frontend can tag rec_click events without a separate fetch.
	upNextWeights := []recs.WeightedSignal{
		{Module: h.s1, Weight: 0.30},
		{Module: h.s2, Weight: 0.20},
		{Module: h.s3, Weight: 0.20},
		{Module: h.s4, Weight: 0.10},
		{Module: h.s5, Weight: 0.20},
	}
	for i, r := range top {
		anime, ok := hydrated[r.AnimeID]
		if !ok {
			continue
		}
		items = append(items, RecItem{
			Anime:          anime,
			Final:          r.Final,
			Pinned:         false,
			Rank:           i + 1,
			TopContributor: deriveTopContributor(r.Breakdown, upNextWeights),
		})
	}

	// Phase 13 (REC-SIG-06 / REC-UX-03): try to resolve a pin candidate.
	// The S6 cascade reads the user's s6_seed_*, runs the local + Shikimori
	// fallbacks, and returns a single PinCandidate or nil. On non-nil:
	//
	//   1. Hydrate the pin's anime row so the frontend can render the card.
	//   2. Remove the pin's anime from items[] if the ensemble already
	//      ranked it (avoids the same poster appearing twice).
	//   3. Re-rank the remaining ensemble tail (rank 2..N).
	//   4. Prepend a Pinned RecItem at index 0 with rank 1.
	if h.s6 != nil {
		topIDs := make([]string, 0, len(top))
		for _, r := range top {
			topIDs = append(topIDs, string(r.AnimeID))
		}
		pin, err := h.s6.Resolve(ctx, userID, topIDs)
		if err != nil {
			h.log.Warnw("s6 resolve failed (non-fatal)", "user_id", userID, "error", err)
		} else if pin != nil {
			pinHydrated, hydrateErr := h.hydrateAnime(ctx, []string{pin.AnimeID})
			if hydrateErr == nil {
				if anime, ok := pinHydrated[pin.AnimeID]; ok {
					// Drop pin from items[] if it was already in ensemble result.
					deduped := make([]RecItem, 0, len(items))
					for _, it := range items {
						if it.Anime.ID != pin.AnimeID {
							deduped = append(deduped, it)
						}
					}
					// Re-rank the deduped tail (pin takes rank 1).
					for i := range deduped {
						deduped[i].Rank = i + 2
					}
					pinItem := RecItem{
						Anime:          anime,
						Final:          0, // spec §B7: float64 zero approximates "null"; frontend gates on Pinned
						Pinned:         true,
						PinReason:      "Because you finished " + pin.SeedName,                         // legacy fallback
						PinReasonKey:   "recs.pinReason.becauseYouFinished",                           // UX-09: i18n key
						PinReasonData:  map[string]any{"name": pin.SeedName},                          // UX-09: interpolation values
						PinSeedAnimeID: pin.SeedAnimeID,
						PinSource:      pin.Source,
						Rank:           1,
					}
					items = append([]RecItem{pinItem}, deduped...)
				}
			} else {
				h.log.Warnw("s6 pin hydrate failed (non-fatal); serving row without pin",
					"user_id", userID, "pin_anime_id", pin.AnimeID, "error", hydrateErr)
			}
		}
	}

	return RecsEnvelope{
		Recs:        items,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		CacheHit:    false,
		Total:       len(items),
		RowLabelKey: "recs.upNext",
	}, nil
}

// hydrateAnime fetches the anime fields the frontend needs in a single SELECT.
func (h *RecsHandler) hydrateAnime(ctx context.Context, ids []string) (map[string]RecAnimePayload, error) {
	if len(ids) == 0 {
		return map[string]RecAnimePayload{}, nil
	}
	type row struct {
		ID            string
		Name          string
		NameRU        string
		NameJP        string
		PosterURL     string
		Score         float64
		EpisodesCount int
		Status        string
		Year          int
	}
	var rows []row
	err := h.db.WithContext(ctx).
		Table("animes").
		Select("id, name, name_ru, name_jp, poster_url, score, episodes_count, status, year").
		Where("id IN ?", ids).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]RecAnimePayload, len(rows))
	for _, r := range rows {
		out[r.ID] = RecAnimePayload{
			ID:            r.ID,
			Name:          r.Name,
			NameRU:        r.NameRU,
			NameJP:        r.NameJP,
			PosterURL:     r.PosterURL,
			Score:         r.Score,
			EpisodesCount: r.EpisodesCount,
			Status:        r.Status,
			Year:          r.Year,
		}
	}
	return out, nil
}
