package cards

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// platformStatsWindow is the rolling lookback period for each metric
// (HSB-BE-13: "7d" metrics). UTC anchored — same lookback regardless of
// server timezone.
const platformStatsWindow = 7 * 24 * time.Hour

// PlatformStatsResolver implements spotlight.Resolver for the
// `platform_stats` card.
//
// Phase 1 ships ONE real metric (anime_added_7d via GORM Count) and
// HARDCODES the other two as nil:
//
//   - episodes_added_7d: SKIPPED for Phase 1 — no per-episode event log
//     exists in this codebase (RESEARCH.md A6 / Pitfall: no `episodes`
//     table with created_at, only Anime.EpisodesAired snapshot).
//
//   - active_rooms_7d: SKIPPED for Phase 1 — rooms service is Redis-only
//     (verified in RESEARCH.md A7: `services/rooms/internal/service/room.go`
//     writes `room:<id>` to Redis with 24h TTL, no Postgres table to query).
//
// Card eligibility (HSB-BE-13): card returned iff ≥1 metric is non-nil.
// In Phase 1 that resolves to "iff anime_added_7d count succeeded".
type PlatformStatsResolver struct {
	db    *gorm.DB
	cache cache.Cache
	log   *logger.Logger
}

// NewPlatformStatsResolver constructs the resolver.
func NewPlatformStatsResolver(db *gorm.DB, c cache.Cache, log *logger.Logger) *PlatformStatsResolver {
	return &PlatformStatsResolver{db: db, cache: c, log: log}
}

// Type returns the card discriminator string.
func (r *PlatformStatsResolver) Type() string { return "platform_stats" }

// Resolve returns the platform_stats card. Phase 1 produces a single
// metric (anime_added_7d). userID is ignored — stats are global.
//
// Eligibility: zero metrics computed → (nil, nil), no cache write.
// Per-metric errors are logged and the metric is dropped; other metrics
// can still succeed.
func (r *PlatformStatsResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	key := "spotlight:stats:" + spotlight.DateKeyUTC(time.Now())

	// --- Cache GET path -------------------------------------------------
	var cached spotlight.PlatformStatsData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	// --- Cache MISS path: compute metrics -------------------------------
	cutoff := time.Now().Add(-platformStatsWindow)
	metrics := make([]spotlight.StatsMetric, 0, 1)

	// --- Metric 1: anime_added_7d (always attempted) --------------------
	if r.db != nil {
		var animeCount int64
		err := r.db.WithContext(ctx).Model(&domain.Anime{}).
			Where("created_at > ?", cutoff).
			Count(&animeCount).Error
		if err != nil {
			r.log.Warnw("spotlight.stats_count_failed",
				"metric", "anime_added_7d",
				"error", err,
			)
		} else {
			metrics = append(metrics, spotlight.StatsMetric{
				Key:   "anime_added_7d",
				Value: animeCount,
			})
		}
	} else {
		// Nil db — log and skip; eligibility branch below handles the
		// "no metrics" outcome. Tests use this path to drive the
		// ineligibility-on-empty case.
		r.log.Warnw("spotlight.stats_count_failed",
			"metric", "anime_added_7d",
			"error", "nil db",
		)
	}

	// --- Metric 2: episodes_added_7d ------------------------------------
	// SKIPPED for Phase 1: no per-episode log exists in this codebase —
	// see RESEARCH.md A6. The closest field is Anime.EpisodesAired (a
	// snapshot, not an event log). Re-introducing this metric requires
	// either an `episode_added_at` column backfill or a new event table,
	// both out of scope for Phase 1. Card stays eligible via Metric 1.

	// --- Metric 3: active_rooms_7d --------------------------------------
	// SKIPPED for Phase 1: rooms service is Redis-only (verified in
	// RESEARCH.md A7) — no Postgres table to query. The shared GORM
	// connection from catalog cannot SELECT * FROM rooms because that
	// table doesn't exist. A cross-service direct-query approach is
	// deferred to a later phase per VALIDATION.md.

	// --- Eligibility check ----------------------------------------------
	if len(metrics) == 0 {
		// All metrics failed/unavailable — card is ineligible.
		// Do NOT cache empty (Pitfall 5).
		return nil, nil
	}
	data := spotlight.PlatformStatsData{Metrics: metrics}

	// --- Cache SET (best-effort) ----------------------------------------
	if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
