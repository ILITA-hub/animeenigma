// Package handler — recs.go: GET /api/users/recs anonymous trending row.
//
// Phase 10 surfaces the population-wide recommendations engine for the first
// time. Anonymous callers get a single shared trending top-N (no per-anon
// personalization in v2.0). Logged-in callers in this phase get the SAME
// payload — Phase 11 will branch on auth state to swap in the personalized
// "Up Next for you" row.
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
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs/signals"
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
type RecItem struct {
	Anime  RecAnimePayload `json:"anime"`
	Final  float64         `json:"final"`
	Pinned bool            `json:"pinned"`
	Rank   int             `json:"rank"`
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

	// Composed signals — built lazily-once on first request to keep the
	// constructor zero-arg-friendly. The DB handle is enough to wire them.
	s11 *signals.S11Filter
	s3  *signals.S3Trending
	s4  *signals.S4Recency
}

// NewRecsHandler wires the handler with its dependencies. The signal modules
// are constructed once here (cheap; just struct literals + the DB handle).
func NewRecsHandler(db *gorm.DB, recsRepo *repo.RecsRepository, cache recsCache, log *logger.Logger) *RecsHandler {
	return &RecsHandler{
		db:    db,
		repo:  recsRepo,
		cache: cache,
		log:   log,
		s11:   signals.NewS11Filter(db),
		s3:    signals.NewS3Trending(db, recsRepo),
		s4:    signals.NewS4Recency(db),
	}
}

// GetRecs serves the trending row.
//
// Cache flow:
//  1. Try recsCache.Get(PublicTrendingKey).
//  2. Hit  -> set CacheHit=true and return.
//  3. Miss -> compute via S11.CandidatePool + Ensemble(S3=0.20, S4=0.10),
//     sort with S4-tiebreak backfill, slice to top-20, hydrate anime info,
//     write to cache with 6h TTL, return.
//
// Phase 10 contract: logged-in callers get the SAME anonymous payload (with
// row_label_key="recs.trending"). Phase 11 will branch on claims to switch
// to "recs.upNext" and a personalized ensemble.
func (h *RecsHandler) GetRecs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Phase-11 stub: log when a logged-in caller hits the anonymous path so
	// we can audit migration when personalization lands.
	if claims, ok := authz.ClaimsFromContext(ctx); ok && claims != nil {
		// TODO(phase-11): branch on claims to serve personalized recs row
		h.log.Debugw("phase-11 personalization deferred — serving anonymous trending",
			"user_id", claims.UserID)
	}

	// 1. Cache read-through
	var cached RecsEnvelope
	err := h.cache.Get(ctx, PublicTrendingKey, &cached)
	if err == nil {
		cached.CacheHit = true
		httputil.OK(w, cached)
		return
	}
	// On any non-trivial cache error, log but continue (fail open).
	if err != nil && !isCacheMiss(err) {
		h.log.Warnw("recs cache read failed; recomputing", "error", err)
	}

	// 2. Compute fresh
	envelope, err := h.computeFresh(ctx)
	if err != nil {
		h.log.Errorw("recs compute failed", "error", err)
		// Serve an empty row instead of an error — the trending row is a
		// best-effort surface. Frontend hides on empty.
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
	for i, r := range top {
		anime, ok := hydrated[r.AnimeID]
		if !ok {
			// Anime row vanished between S11 and hydrate — unlikely but
			// possible under concurrent edits. Skip silently.
			continue
		}
		items = append(items, RecItem{
			Anime:  anime,
			Final:  r.Final,
			Pinned: false,
			Rank:   i + 1,
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
