package cache

import (
	"context"
	"math"
	"strconv"
	"sync/atomic"
	"time"
)

// DegradationLevelKey is the Redis key the governor service publishes the
// platform degradation level to ("0"|"1"|"2", TTL'd). Mirrors
// services/governor/internal/domain.RedisLevelKey — keep in sync.
const DegradationLevelKey = "ae:degradation:level"

// DegradationScoreKey is the Redis key the governor publishes the continuous
// pressure score to ("0.00".."1.00", TTL'd). Mirrors
// services/governor/internal/domain.RedisScoreKey — keep in sync.
const DegradationScoreKey = "ae:degradation:score"

// DegradationWatcher polls the governor-published degradation level and caches
// it for cheap synchronous reads on hot paths (worker claim loops, cron
// guards). FAIL-OPEN by design: a nil watcher, a nil cache, a missing/expired
// key (governor down), or any Redis error all read as level 0 — consumers must
// never shed work because the signal is missing.
//
// Graceful-degradation Phase 3 consumer side
// (docs/superpowers/specs/2026-07-10-graceful-degradation-design.md).
type DegradationWatcher struct {
	cache *RedisCache
	poll  time.Duration
	level atomic.Int32
	score atomic.Uint64 // math.Float64bits-encoded
}

// NewDegradationWatcher builds a watcher over c (nil c is allowed and pins the
// level at 0). poll <= 0 defaults to 5s.
func NewDegradationWatcher(c *RedisCache, poll time.Duration) *DegradationWatcher {
	if poll <= 0 {
		poll = 5 * time.Second
	}
	return &DegradationWatcher{cache: c, poll: poll}
}

// Start launches the poll loop until ctx is done. The first read happens
// immediately so consumers see a fresh level right after boot.
func (w *DegradationWatcher) Start(ctx context.Context) {
	if w == nil || w.cache == nil {
		return
	}
	go func() {
		w.refresh(ctx)
		t := time.NewTicker(w.poll)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				w.refresh(ctx)
			}
		}
	}()
}

// Level returns the last-read degradation level (0 Normal, 1 Elevated,
// 2 Critical). Safe on a nil receiver (returns 0).
func (w *DegradationWatcher) Level() int {
	if w == nil {
		return 0
	}
	return int(w.level.Load())
}

// ShouldShed reports whether heavy work admission should be shed at the given
// minimum level (typically 1). Safe on a nil receiver (never sheds).
func (w *DegradationWatcher) ShouldShed(min int) bool {
	return w.Level() >= min
}

// Score returns the last-read continuous pressure score (0.00..1.00).
// Safe on a nil receiver (returns 0). Fail-open: missing key, parse error,
// or Redis error all read as 0.0.
func (w *DegradationWatcher) Score() float64 {
	if w == nil {
		return 0
	}
	return math.Float64frombits(w.score.Load())
}

func (w *DegradationWatcher) refresh(ctx context.Context) {
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, err := w.cache.Client().Get(rctx, DegradationLevelKey).Result()
	if err != nil {
		// redis.Nil (no key = governor down/undeployed) and transport errors
		// alike fail open to Normal.
		w.level.Store(0)
		w.score.Store(0)
		return
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 2 {
		w.level.Store(0)
		w.score.Store(0)
		return
	}
	w.level.Store(int32(n))

	sraw, serr := w.cache.Client().Get(rctx, DegradationScoreKey).Result()
	if serr != nil {
		w.score.Store(0)
		return
	}
	f, serr := strconv.ParseFloat(sraw, 64)
	if serr != nil {
		w.score.Store(0)
		return
	}
	// strconv.ParseFloat accepts "NaN"/"nan", and math.Max/Min propagate NaN
	// rather than clamping it — a corrupted key would otherwise publish NaN,
	// which fails CLOSED downstream (Curve.Cap(NaN) hits the last-cap branch,
	// e.g. 0 workers). Guard explicitly before the clamp (review finding 3).
	if math.IsNaN(f) {
		w.score.Store(0)
		return
	}
	w.score.Store(math.Float64bits(math.Max(0, math.Min(1, f))))
}
