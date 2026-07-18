// Package handler — upcoming.go: GET /api/users/recs/upcoming +
// POST /api/users/recs/upcoming/dismiss (spec 2026-07-17, relevance-hardened
// 2026-07-18).
//
// "Announce recs": scores status='announced' titles for a logged-in user with
// the signals that work for unaired content — S8 franchise (dominant), S5 rich
// attribute affinity (tags/studio/genre/demographic/source/format), S9 relative
// MAL popularity, S2 genre, and S7 dropped-penalty (negative). Behavioral
// signals (S1/S3/S4) are structurally ~0 for unaired titles and not consulted.
//
// Admission gate runs on RAW scores, not normalized ones: per-pool min-max
// normalization inflates the best of a garbage pool to 1.0, so a normalized
// floor would pass junk. The gate is CONTINUATION-AWARE (spec 2026-07-18 §2):
//   - Continuation (a later franchise entry — 2nd Season etc.): admitted ONLY
//     if raw_s8 >= MinS8 (the user scored a prior franchise entry). Never via
//     taste — recommending "Season 2" to someone who never watched S1 is the
//     defect this replaces.
//   - Standalone / first entry: admitted if raw_s8 >= MinS8 OR raw_s5 >= MinS5
//     (rich attribute affinity). Genre alone (S2) no longer admits.
// S9 popularity boosts RANKING only, never gates.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs/signals"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// UpcomingKeyPrefix/Suffix build the per-user upcoming cache key:
// recs:user:<uid>:upcoming:v1. Bump the suffix on any ranking/gate change.
const (
	UpcomingKeyPrefix = "recs:user:"
	UpcomingKeySuffix = ":upcoming:v2"
	upcomingTTL       = 6 * time.Hour

	// upcomingFranchiseReasonMinS8 is the bar for CLAIMING franchise causation
	// in the reason line (spec §3). The eligibility gate (h.cfg.MinS8,
	// default 0.2) only decides whether an item is LET IN; this higher,
	// fixed bar decides whether the reason says "franchise" or "taste" — an
	// item that passed mainly on genre (S2) with a weak S8 in
	// [MinS8, upcomingFranchiseReasonMinS8) gets the taste reason instead.
	upcomingFranchiseReasonMinS8 = 0.4
)

// UpcomingConfig carries the env-tunable knobs (config.Load wires them).
type UpcomingConfig struct {
	TopK  int     // RECS_UPCOMING_TOPK, default 3
	MinS8 float64 // RECS_UPCOMING_MIN_S8, default 0.2
	MinS2 float64 // RECS_UPCOMING_MIN_S2, default 0.3 (retained; no longer gates)
	MinS5 float64 // RECS_UPCOMING_MIN_S5, default 0.01 (standalone taste gate)
}

// UpcomingAnimePayload is the hydrated anime shape for one upcoming item.
// Superset of RecAnimePayload with the announcement-relevant fields.
type UpcomingAnimePayload struct {
	ID        string  `json:"id"`
	Name      string  `json:"name,omitempty"`
	NameRU    string  `json:"name_ru,omitempty"`
	NameJP    string  `json:"name_jp,omitempty"`
	PosterURL string  `json:"poster_url,omitempty"`
	Score     float64 `json:"score,omitempty"`
	Status    string  `json:"status,omitempty"`
	Year      int     `json:"year,omitempty"`
	Season    string  `json:"season,omitempty"`
	Kind      string  `json:"kind,omitempty"`
	Franchise string  `json:"franchise,omitempty"`
}

// UpcomingReason explains WHY a title matched. Kind is one of:
//   - "franchise"   — seed fields populated ("you rated X 9/10")
//   - "attribute"   — Attribute/AttributeName populated (shared studio/source)
//   - "anticipated" — pool-relative MAL popularity is high
//   - "taste"       — generic attribute-affinity fallback
type UpcomingReason struct {
	Kind            string `json:"kind"`
	SeedAnimeID     string `json:"seed_anime_id,omitempty"`
	SeedAnimeName   string `json:"seed_anime_name,omitempty"`
	SeedAnimeNameRU string `json:"seed_anime_name_ru,omitempty"`
	UserScore       int    `json:"user_score,omitempty"`
	// Attribute is the shared S5 dimension driving an "attribute" reason
	// ("studio" | "source"); AttributeName is its human/display value.
	Attribute     string `json:"attribute,omitempty"`
	AttributeName string `json:"attribute_name,omitempty"`
}

// UpcomingItem is one matched announcement.
type UpcomingItem struct {
	Anime      UpcomingAnimePayload `json:"anime"`
	MatchScore float64              `json:"match_score"`
	Reason     UpcomingReason       `json:"reason"`
}

// UpcomingEnvelope is the data field of the response.
type UpcomingEnvelope struct {
	Items       []UpcomingItem `json:"items"`
	GeneratedAt string         `json:"generated_at"`
	CacheHit    bool           `json:"cache_hit"`
}

// UpcomingHandler serves the two upcoming endpoints.
type UpcomingHandler struct {
	db         *gorm.DB
	dismissals *repo.AnnouncementDismissalsRepository
	cache      recsCache
	log        *logger.Logger
	cfg        UpcomingConfig
	sf         singleflight.Group

	s2 *signals.S2Metadata
	s5 *signals.S5Attribute
	s7 *signals.S7DroppedPenalty
	s8 *signals.S8Franchise
	s9 *signals.S9MalPopularity
}

// NewUpcomingHandler wires the handler. Signals are constructed here (cheap
// struct literals over the shared DB handle). NOTE: S5 needs the recs repo —
// mirror NewRecsHandler's construction exactly.
func NewUpcomingHandler(db *gorm.DB, dismissals *repo.AnnouncementDismissalsRepository, cache recsCache, log *logger.Logger, cfg UpcomingConfig) *UpcomingHandler {
	if cfg.TopK <= 0 {
		cfg.TopK = 3
	}
	// Guard the standalone gate: MinS5 <= 0 would make `rawS5 < MinS5` never
	// fire, admitting every rawS5 >= 0 title (i.e. the whole pool). A positive
	// floor is required for the gate to mean anything.
	if cfg.MinS5 <= 0 {
		cfg.MinS5 = 0.01
	}
	return &UpcomingHandler{
		db:         db,
		dismissals: dismissals,
		cache:      cache,
		log:        log,
		cfg:        cfg,
		s2:         signals.NewS2Metadata(db),
		s5:         signals.NewS5Attribute(db, repo.NewRecsRepository(db)),
		s7:         signals.NewS7DroppedPenalty(db),
		s8:         signals.NewS8Franchise(db),
		s9:         signals.NewS9MalPopularity(db),
	}
}

// upcomingKey returns the per-user cache key.
func upcomingKey(userID string) string {
	return UpcomingKeyPrefix + userID + UpcomingKeySuffix
}

// GetUpcoming serves the personalized announced-title matches. JWT required
// (router mounts AuthMiddleware, but the handler double-checks claims so a
// direct call without middleware still 401s).
func (h *UpcomingHandler) GetUpcoming(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims, ok := authz.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.UserID == "" {
		httputil.Unauthorized(w)
		return
	}
	userID := claims.UserID

	var cached UpcomingEnvelope
	if err := h.cache.Get(ctx, upcomingKey(userID), &cached); err == nil {
		cached.CacheHit = true
		httputil.OK(w, cached)
		return
	} else if !isCacheMiss(err) {
		h.log.Warnw("upcoming cache read failed; recomputing", "user_id", userID, "error", err)
	}

	key := upcomingKey(userID)
	v, err, _ := h.sf.Do(key, func() (interface{}, error) {
		var warm UpcomingEnvelope
		if cerr := h.cache.Get(ctx, key, &warm); cerr == nil {
			return warm, nil
		}
		env, cerr := h.computeUpcoming(ctx, userID)
		if cerr != nil {
			return UpcomingEnvelope{}, cerr
		}
		if setErr := h.cache.Set(ctx, key, env, upcomingTTL); setErr != nil {
			h.log.Warnw("upcoming cache write failed", "user_id", userID, "error", setErr)
		}
		return env, nil
	})
	if err != nil {
		h.log.Errorw("upcoming compute failed", "user_id", userID, "error", err)
		httputil.OK(w, UpcomingEnvelope{
			Items:       []UpcomingItem{},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	httputil.OK(w, v.(UpcomingEnvelope))
}

// dismissBody is the POST /upcoming/dismiss request shape.
type dismissBody struct {
	AnimeID string `json:"anime_id"`
}

// PostDismiss persists a permanent per-user dismissal and busts the upcoming
// cache so the card advances on the next resolve.
func (h *UpcomingHandler) PostDismiss(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims, ok := authz.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.UserID == "" {
		httputil.Unauthorized(w)
		return
	}
	var body dismissBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AnimeID == "" {
		httputil.BadRequest(w, "anime_id is required")
		return
	}
	if err := h.dismissals.Insert(ctx, claims.UserID, body.AnimeID); err != nil {
		h.log.Errorw("upcoming dismiss insert failed", "user_id", claims.UserID, "anime_id", body.AnimeID, "error", err)
		httputil.Error(w, err)
		return
	}
	if err := h.cache.Delete(ctx, upcomingKey(claims.UserID)); err != nil {
		h.log.Warnw("upcoming cache bust failed", "user_id", claims.UserID, "error", err)
	}
	httputil.OK(w, map[string]bool{"dismissed": true})
}

// computeUpcoming builds the pool, scores it, gates on raw scores, and
// hydrates the top-K.
func (h *UpcomingHandler) computeUpcoming(ctx context.Context, userID string) (UpcomingEnvelope, error) {
	env := UpcomingEnvelope{
		Items:       []UpcomingItem{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// 1. Pool: announced, visible, not listed by the user, not dismissed.
	var pool []recs.AnimeID
	if err := h.db.WithContext(ctx).
		Table("animes AS a").
		Select("a.id").
		Joins("LEFT JOIN anime_list al ON al.anime_id = a.id AND al.user_id = ?", userID).
		Joins("LEFT JOIN rec_announcement_dismissals d ON d.anime_id = a.id AND d.user_id = ?", userID).
		Where("a.status = ?", "announced").
		Where("a.hidden = ?", false).
		Where("a.deleted_at IS NULL").
		Where("al.status IS NULL").
		Where("d.id IS NULL").
		Pluck("a.id", &pool).Error; err != nil {
		return env, err
	}
	if len(pool) == 0 {
		return env, nil
	}

	// 2. Score with the announcement ensemble. RankWithBreakdown exposes the
	//    raw per-signal scores for gating + reason derivation. S8 dominant, S5
	//    rich attribute affinity, S9 relative popularity, S2 genre, S7 negative.
	ensemble := recs.NewEnsemble([]recs.WeightedSignal{
		{Module: h.s8, Weight: 0.40},
		{Module: h.s5, Weight: 0.30},
		{Module: h.s9, Weight: 0.15},
		{Module: h.s2, Weight: 0.10},
		{Module: h.s7, Weight: -0.05},
	})
	ranked, err := ensemble.RankWithBreakdown(ctx, recs.UserID(userID), pool)
	if err != nil {
		return env, err
	}

	// 3. Continuation detection over the ranked pool (name + franchise-structural).
	rankedIDs := make([]string, len(ranked))
	for i, r := range ranked {
		rankedIDs[i] = r.AnimeID
	}
	names, err := h.loadNames(ctx, rankedIDs)
	if err != nil {
		return env, err
	}
	airedSibling, err := h.franchiseHasAiredSibling(ctx, rankedIDs)
	if err != nil {
		return env, err
	}

	// 4. Continuation-aware raw-score gate.
	type pick struct {
		id        string
		final     float64
		franchise bool
		rawS9     float64
	}
	picks := make([]pick, 0, len(ranked))
	var maxS9 float64
	for _, r := range ranked {
		rawS8 := float64(r.Raw[recs.SignalID("s8")])
		rawS5 := float64(r.Raw[recs.SignalID("s5")])
		rawS9 := float64(r.Raw[recs.SignalID("s9")])
		nm := names[r.AnimeID]
		continuation := looksLikeSequel(nm.Name, nm.NameRU) || airedSibling[r.AnimeID]
		if continuation {
			if rawS8 < h.cfg.MinS8 {
				continue // sequels only via franchise affinity, never taste
			}
		} else if rawS8 < h.cfg.MinS8 && rawS5 < h.cfg.MinS5 {
			continue // standalone needs rich attribute affinity (or franchise)
		}
		if rawS9 > maxS9 {
			maxS9 = rawS9
		}
		picks = append(picks, pick{
			id:        r.AnimeID,
			final:     r.Final,
			franchise: rawS8 >= upcomingFranchiseReasonMinS8,
			rawS9:     rawS9,
		})
	}
	if len(picks) == 0 {
		return env, nil
	}

	// 5. Franchise-fired items sort strictly ahead of taste items (S8 is the
	//    most valuable signal), then by Final desc. Take TopK.
	sort.SliceStable(picks, func(i, j int) bool {
		if picks[i].franchise != picks[j].franchise {
			return picks[i].franchise
		}
		return picks[i].final > picks[j].final
	})
	if len(picks) > h.cfg.TopK {
		picks = picks[:h.cfg.TopK]
	}

	// 6. Hydrate + resolve reasons.
	ids := make([]string, len(picks))
	for i, p := range picks {
		ids[i] = p.id
	}
	hydrated, err := h.hydrateUpcoming(ctx, ids)
	if err != nil {
		return env, err
	}
	for _, p := range picks {
		anime, ok := hydrated[p.id]
		if !ok {
			continue
		}
		env.Items = append(env.Items, UpcomingItem{
			Anime:      anime,
			MatchScore: p.final,
			Reason:     h.resolveReason(ctx, userID, p.id, anime, p.franchise, p.rawS9, maxS9),
		})
	}
	return env, nil
}

// hydrateUpcoming fetches the announcement card fields in one SELECT.
func (h *UpcomingHandler) hydrateUpcoming(ctx context.Context, ids []string) (map[string]UpcomingAnimePayload, error) {
	type row struct {
		ID        string
		Name      string
		NameRU    string
		NameJP    string
		PosterURL string
		Score     float64
		Status    string
		Year      int
		Season    string
		Kind      string
		Franchise string
	}
	var rows []row
	if err := h.db.WithContext(ctx).
		Table("animes").
		Select("id, name, name_ru, name_jp, poster_url, score, status, year, season, kind, franchise").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]UpcomingAnimePayload, len(rows))
	for _, r := range rows {
		out[r.ID] = UpcomingAnimePayload{
			ID: r.ID, Name: r.Name, NameRU: r.NameRU, NameJP: r.NameJP,
			PosterURL: r.PosterURL, Score: r.Score, Status: r.Status,
			Year: r.Year, Season: r.Season, Kind: r.Kind, Franchise: r.Franchise,
		}
	}
	return out, nil
}

// franchiseSeed finds the user's best-scored anime in the given franchise —
// the "you rated X 9/10" half of the why-line. Returns nil (no error) when
// the user has no scored entry in the franchise (shouldn't happen when the
// S8 gate passed, but the data can shift between scoring and hydration).
func (h *UpcomingHandler) franchiseSeed(ctx context.Context, userID, franchise string) (*UpcomingReason, error) {
	type row struct {
		AnimeID string
		Name    string
		NameRU  string
		Score   int
	}
	var rows []row
	if err := h.db.WithContext(ctx).
		Table("anime_list AS al").
		Select("al.anime_id AS anime_id, a.name AS name, a.name_ru AS name_ru, al.score AS score").
		Joins("JOIN animes a ON a.id = al.anime_id").
		Where("al.user_id = ? AND a.franchise = ? AND al.score > 5", userID, franchise).
		Order("al.score DESC").
		Limit(1).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &UpcomingReason{
		Kind:            "franchise",
		SeedAnimeID:     rows[0].AnimeID,
		SeedAnimeName:   rows[0].Name,
		SeedAnimeNameRU: rows[0].NameRU,
		UserScore:       rows[0].Score,
	}, nil
}
