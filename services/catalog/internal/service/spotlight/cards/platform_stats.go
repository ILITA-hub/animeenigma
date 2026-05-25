package cards

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// promQuerier is the subset of the Prometheus client this resolver needs.
// Defined here so tests inject a handwritten fake (no testify/mock).
type promQuerier interface {
	Query(ctx context.Context, promql string) (float64, error)
	Health(ctx context.Context) (allUp bool, uptimePct float64, err error)
}

// statsServices is the fixed roster of real backend services the daily
// "vibe" line can name. Order is irrelevant — the pick is RNG-driven.
var statsServices = []string{
	"auth", "catalog", "streaming", "player", "rooms",
	"scheduler", "themes", "notifications", "gateway",
}

const (
	defaultTagline = "Лучшая платформа для аниме. Поверьте."
	defaultQuip    = "ОЧЕНЬ МНОГО"
)

var defaultVibe = vibe{UXDelta: "+5 (Tremendous)", CDI: "0.00 * 99", MVQ: "Dragon 99%/99%"}

// PlatformStatsResolver implements spotlight.Resolver for the bombastic
// platform_stats card. It draws real health + tile values from Prometheus
// and joke copy from the embedded pools. Always eligible: the pool-backed
// hero renders even when Prometheus is fully down.
type PlatformStatsResolver struct {
	prom  promQuerier
	cache cache.Cache
	log   *logger.Logger
}

// NewPlatformStatsResolver constructs the resolver.
func NewPlatformStatsResolver(prom promQuerier, c cache.Cache, log *logger.Logger) *PlatformStatsResolver {
	return &PlatformStatsResolver{prom: prom, cache: c, log: log}
}

// Type returns the card discriminator string.
func (r *PlatformStatsResolver) Type() string { return "platform_stats" }

// Resolve assembles the daily card. userID is ignored — everyone sees the
// same global daily pick. The payload is cached once per UTC day.
func (r *PlatformStatsResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	dateKey := spotlight.DateKeyUTC(time.Now())
	key := "spotlight:stats:" + dateKey

	var cached spotlight.PlatformStatsData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	rng := dateSeededRng(dateKey)

	// --- Hero joke content (RNG order is fixed for determinism) ---------
	quip := pickString(rng, parsedJokes.UptimeQuips, defaultQuip)
	service := statsServices[rng.Intn(len(statsServices))]
	tagline := pickString(rng, parsedJokes.Taglines, defaultTagline)
	v := pickVibe(rng, parsedJokes.Vibes)

	hero := spotlight.StatsHero{
		UptimeQuip: quip,
		Service:    service,
		Tagline:    tagline,
		UXDelta:    v.UXDelta,
		CDI:        v.CDI,
		MVQ:        v.MVQ,
	}

	// --- Real health ----------------------------------------------------
	if allUp, pct, err := r.prom.Health(ctx); err != nil {
		r.log.Warnw("spotlight.stats_health_failed", "error", err)
		hero.WorkingOK = false
		hero.UptimePercent = nil
	} else {
		hero.WorkingOK = allUp
		p := math.Round(pct*10) / 10
		hero.UptimePercent = &p
	}

	// --- Tiles: shuffle allowlist, pick a random window each, keep > 0 --
	tiles := make([]spotlight.StatsTile, 0, 4)
	order := make([]promTile, len(parsedTiles))
	copy(order, parsedTiles)
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	for _, t := range order {
		if len(tiles) >= 4 {
			break
		}
		if len(t.Windows) == 0 {
			continue
		}
		window := t.Windows[rng.Intn(len(t.Windows))]
		val, err := r.prom.Query(ctx, windowPromQL(t.Metric, window))
		if err != nil {
			r.log.Warnw("spotlight.stats_tile_failed", "metric", t.Metric, "window", window, "error", err)
			continue
		}
		if val <= 0 {
			continue
		}
		tiles = append(tiles, spotlight.StatsTile{
			Label:  t.Label,
			Value:  val,
			Window: window,
			Format: t.Format,
		})
	}

	data := spotlight.PlatformStatsData{Hero: hero, Tiles: tiles}

	if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}

// dateSeededRng returns an RNG seeded from the UTC date key, so the daily
// pick is stable within the day and reproducible in tests.
func dateSeededRng(dateKey string) *rand.Rand {
	h := fnv.New64a()
	_, _ = h.Write([]byte(dateKey))
	return rand.New(rand.NewSource(int64(h.Sum64())))
}

// windowPromQL builds the sum()-aggregated PromQL for a metric + window.
func windowPromQL(metric, window string) string {
	switch window {
	case "day":
		return fmt.Sprintf("sum(increase(%s[1d]))", metric)
	case "week":
		return fmt.Sprintf("sum(increase(%s[7d]))", metric)
	default: // "all"
		return fmt.Sprintf("sum(%s)", metric)
	}
}

func pickString(rng *rand.Rand, pool []string, fallback string) string {
	if len(pool) == 0 {
		return fallback
	}
	return pool[rng.Intn(len(pool))]
}

func pickVibe(rng *rand.Rand, pool []vibe) vibe {
	if len(pool) == 0 {
		return defaultVibe
	}
	return pool[rng.Intn(len(pool))]
}
