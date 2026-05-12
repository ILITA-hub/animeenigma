// probe.go — per-provider liveness probe goroutine.
//
// One ProbeRunner instance per registered provider. Started from main.go
// after orchestrator.Register() and before ListenAndServe(). The probe
// exercises the full 5-stage pipeline (search → episodes → servers →
// stream → stream_segment) against a randomly-picked AnimeRef from the
// golden pool every probeBaseInterval (15 min) ± probeJitterPct (20%).
//
// Per-tick semantics:
//   1. Pick a random AnimeRef from the pool.
//   2. Call provider.FindID            → record (search) success/failure.
//   3. On success: provider.ListEpisodes → record (episodes).
//   4. On success: provider.ListServers  → record (servers).
//   5. On success: provider.GetStream    → record (stream).
//   6. On success: HTTP GET first 4 KiB of Sources[0].URL → record (stream_segment).
//   7. cache.Update(name, ProviderHealth{...}) — writes the per-stage map.
//   8. Emit provider_health_up{provider, stage} for all five canonical
//      stages — stages not exercised this tick keep their last-known gauge
//      via the windowSet's persisted up/down state.
//   9. Heartbeat: provider_probe_last_tick_timestamp{provider} = now.Unix().
//
// On any failure the probe SHORT-CIRCUITS — subsequent stages are NOT
// exercised, because (a) running them is wasted load on a broken upstream
// and (b) their failures would be caused by the earlier stage's break,
// not real downstream brokenness. The skipped stages keep their last gauge.
//
// Panic safety: Start() wraps the loop in `defer recover()`; if a provider
// panics mid-call the goroutine logs, emits a metric, and re-spawns itself
// via `go r.Start(ctx)`. A single bad tick MUST NOT take down observability.
package health

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

const (
	// probeBaseInterval is the nominal cadence between ticks. The actual
	// sleep is probeBaseInterval ± (probeJitterPct * probeBaseInterval).
	probeBaseInterval = 15 * time.Minute

	// probeJitterPct is the ± jitter applied to probeBaseInterval. Prevents
	// thundering-herd behaviour against upstream if N providers all hit the
	// 15-min boundary at once.
	probeJitterPct = 0.20

	// segmentTimeout caps the stream_segment HTTP fetch. The 4 KiB body
	// read should complete in <100 ms on a healthy upstream; 10 s is a
	// generous outer bound that also matches BaseHTTPClient's own timeout.
	segmentTimeout = 10 * time.Second

	// initialDelayEnvVar lets tests / fast-verify shrink the boot-time
	// initial delay. Unset = production-default jittered 0..interval/4.
	initialDelayEnvVar = "SCRAPER_PROBE_INITIAL_DELAY_OVERRIDE_SECONDS"
)

// ProbeRunner is a per-provider liveness probe goroutine. Construct one
// instance per registered provider via NewProbeRunner; run it via
// `go runner.Start(ctx)`. The goroutine ends when ctx is cancelled.
type ProbeRunner struct {
	provider domain.Provider
	pool     []domain.AnimeRef
	cache    *InMemoryHealthCache
	log      *logger.Logger
	windows  *windowSet
	http     *http.Client // bounded-timeout client for the segment-fetch stage
	now      func() time.Time
	rng      *rand.Rand
}

// ProbeOption is a functional option for NewProbeRunner. Used by tests to
// inject a fake clock / seeded RNG / custom HTTP client.
type ProbeOption func(*ProbeRunner)

// WithNow overrides time.Now for deterministic timestamps in tests.
func WithNow(fn func() time.Time) ProbeOption { return func(r *ProbeRunner) { r.now = fn } }

// WithRNG overrides the random source. Tests use rand.NewPCG(...) for
// reproducible Pick + jitter behaviour.
func WithRNG(rng *rand.Rand) ProbeOption { return func(r *ProbeRunner) { r.rng = rng } }

// WithHTTPClient overrides the segment-fetch HTTP client. Tests inject a
// client that talks to an httptest.Server.
func WithHTTPClient(c *http.Client) ProbeOption { return func(r *ProbeRunner) { r.http = c } }

// NewProbeRunner constructs a ProbeRunner with production defaults. Apply
// ProbeOptions to override for tests.
func NewProbeRunner(p domain.Provider, pool []domain.AnimeRef, cache *InMemoryHealthCache, log *logger.Logger, opts ...ProbeOption) *ProbeRunner {
	r := &ProbeRunner{
		provider: p,
		pool:     pool,
		cache:    cache,
		log:      log,
		windows:  newWindowSet(),
		http:     &http.Client{Timeout: segmentTimeout},
		now:      time.Now,
		rng:      rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Start blocks until ctx is cancelled. Designed for `go r.Start(ctx)`.
//
// Wraps the per-tick loop in `defer recover()`; on panic, logs and
// re-spawns the goroutine. The first tick fires after a randomized
// initial delay (0 to interval/4) to avoid boot-time stampede and
// give the cookie-jar a moment to warm up (RESEARCH P-06).
//
// Test/CI fast-path: set initialDelayEnvVar to a non-negative integer
// to override the random delay (use 0 for "tick immediately"). Production
// leaves it unset.
func (r *ProbeRunner) Start(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			r.log.Errorw("scraper.probe: panicked, restarting",
				"provider", r.provider.Name(),
				"panic", rec,
			)
			// Re-spawn so a single bad tick doesn't kill observability.
			go r.Start(ctx)
		}
	}()
	r.log.Infow("scraper.probe: started",
		"provider", r.provider.Name(),
		"pool_size", len(r.pool),
		"base_interval", probeBaseInterval.String(),
	)

	initialDelay := r.computeInitialDelay()
	select {
	case <-ctx.Done():
		r.log.Infow("scraper.probe: stopped", "provider", r.provider.Name())
		return
	case <-time.After(initialDelay):
	}

	for {
		r.runOneTickSafely(ctx)
		sleep := nextSleep(r.rng)
		select {
		case <-ctx.Done():
			r.log.Infow("scraper.probe: stopped", "provider", r.provider.Name())
			return
		case <-time.After(sleep):
		}
	}
}

// computeInitialDelay returns the duration to wait before the FIRST tick.
// Honours initialDelayEnvVar if set; otherwise randomized 0..interval/4.
func (r *ProbeRunner) computeInitialDelay() time.Duration {
	if envVal := os.Getenv(initialDelayEnvVar); envVal != "" {
		if secs, err := strconv.Atoi(envVal); err == nil && secs >= 0 {
			r.log.Infow("scraper.probe: using initial-delay override",
				"provider", r.provider.Name(),
				"delay_seconds", secs,
			)
			return time.Duration(secs) * time.Second
		}
		// Malformed env value — fall through to default randomized delay.
		r.log.Warnw("scraper.probe: malformed initial-delay env, using random",
			"provider", r.provider.Name(),
			"env_val", envVal,
		)
	}
	return time.Duration(r.rng.Int64N(int64(probeBaseInterval / 4)))
}

// nextSleep returns probeBaseInterval ± probeJitterPct.
func nextSleep(rng *rand.Rand) time.Duration {
	delta := (rng.Float64()*2 - 1) * probeJitterPct
	return time.Duration(float64(probeBaseInterval) * (1 + delta))
}

// RunOnce exercises exactly one tick. Exposed for tests; production callers
// use Start. Does NOT wrap in defer-recover (tests want to see panics).
func (r *ProbeRunner) RunOnce(ctx context.Context) {
	r.runOneTick(ctx)
}

// runOneTickSafely is the panic-isolated tick used by Start. If the tick
// panics, we log and continue (the outer defer-recover handles fatal
// goroutine death; this inner one keeps the loop alive without re-spawning).
func (r *ProbeRunner) runOneTickSafely(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			r.log.Errorw("scraper.probe: tick panicked, continuing",
				"provider", r.provider.Name(),
				"panic", rec,
			)
		}
	}()
	r.runOneTick(ctx)
}

// runOneTick exercises the 5-stage pipeline against one randomly-picked
// AnimeRef from the pool. Short-circuits on the first failure.
func (r *ProbeRunner) runOneTick(ctx context.Context) {
	name := r.provider.Name()
	ref := Pick(r.pool, r.rng)
	stages := make(map[string]StageStatus, len(AllStages))
	now := r.now()

	// Stage 1: search (FindID)
	providerID, err := r.provider.FindID(ctx, ref)
	if !r.record(stages, StageSearch, now, err) {
		r.commit(name, stages, now)
		return
	}

	// Stage 2: episodes
	episodes, err := r.provider.ListEpisodes(ctx, providerID)
	if err == nil && len(episodes) == 0 {
		err = fmt.Errorf("list_episodes returned 0 items")
	}
	if !r.record(stages, StageEpisodes, now, err) {
		r.commit(name, stages, now)
		return
	}

	// Stage 3: servers
	servers, err := r.provider.ListServers(ctx, providerID, episodes[0].ID)
	if err == nil && len(servers) == 0 {
		err = fmt.Errorf("list_servers returned 0 items")
	}
	if !r.record(stages, StageServers, now, err) {
		r.commit(name, stages, now)
		return
	}

	// Stage 4: stream
	stream, err := r.provider.GetStream(ctx, providerID, episodes[0].ID, servers[0].ID, domain.CategorySub)
	if err == nil && (stream == nil || len(stream.Sources) == 0) {
		err = fmt.Errorf("get_stream returned no sources")
	}
	if !r.record(stages, StageStream, now, err) {
		r.commit(name, stages, now)
		return
	}

	// Stage 5: stream_segment — bounded GET of the first 4 KiB.
	segErr := r.fetchSegment(ctx, stream.Sources[0].URL)
	r.record(stages, StageStreamSegment, now, segErr)
	r.commit(name, stages, now)
}

// record updates the per-stage window + the in-flight stages map. Returns
// whether the stage succeeded (caller short-circuits if false).
// Truncates the error message to MaxLastErrChars BEFORE storage (P-05).
//
// Note: StageStatus.Up reflects the WINDOW state (post-threshold), NOT
// the raw single-tick outcome — a stage that failed once but is still
// within threshold reports Up=true. This is the contract for SCRAPER-OBS-02.
func (r *ProbeRunner) record(stages map[string]StageStatus, stage string, now time.Time, err error) bool {
	ok := (err == nil)
	isDown := r.windows.Record(stage, now, ok)
	var lastErr string
	if err != nil {
		msg := err.Error()
		if len(msg) > MaxLastErrChars {
			msg = msg[:MaxLastErrChars]
		}
		lastErr = msg
	}
	s := StageStatus{
		Up:      !isDown,
		LastErr: lastErr,
	}
	if ok {
		s.LastOK = now
	}
	stages[stage] = s
	return ok
}

// commit writes the per-stage map to the cache, emits gauges for ALL
// canonical stages, and bumps the heartbeat. Stages not exercised this
// tick (due to short-circuit) get their gauge value from the windowSet's
// persisted state — i.e. they keep their last-known up/down value.
func (r *ProbeRunner) commit(name string, stages map[string]StageStatus, now time.Time) {
	r.cache.Update(name, ProviderHealth{
		Stages:      stages,
		LastUpdated: now,
	})
	// Emit gauges for every canonical stage. Stages exercised this tick use
	// the value just written into `stages`; stages skipped due to short-
	// circuit fall through to the windowSet's persisted up/down state.
	for _, s := range AllStages {
		up := 1.0
		if status, ok := stages[s]; ok {
			if !status.Up {
				up = 0.0
			}
		} else if r.windows.IsDown(s) {
			up = 0.0
		}
		metrics.ProviderHealthUp.WithLabelValues(name, s).Set(up)
	}
	metrics.ProviderProbeLastTick.WithLabelValues(name).Set(float64(now.Unix()))
}

// fetchSegment issues a bounded GET of the first 4 KiB of the source URL.
// Counts as success only if HTTP 2xx + at least one non-empty byte.
// Empty URL is treated as failure (the upstream pipeline returned a stream
// with no playable source — that's a broken stage, not "nothing to test").
func (r *ProbeRunner) fetchSegment(ctx context.Context, urlStr string) error {
	if urlStr == "" {
		return errors.New("stream_segment: empty source URL")
	}
	ctx, cancel := context.WithTimeout(ctx, segmentTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return fmt.Errorf("stream_segment: build request: %w", err)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return fmt.Errorf("stream_segment: do request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("stream_segment: status %d", resp.StatusCode)
	}
	buf := make([]byte, 4096)
	n, _ := io.ReadFull(resp.Body, buf)
	if n == 0 {
		return errors.New("stream_segment: empty body")
	}
	return nil
}
