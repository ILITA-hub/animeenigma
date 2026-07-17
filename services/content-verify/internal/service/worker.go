// Package service runs the throttled probe worker: one unit per tick,
// governor-gated, results upserted into the verdict store.
package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/cvmetrics"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
)

// ShedChecker is satisfied by *cache.DegradationWatcher.
type ShedChecker interface {
	ShouldShed(min int) bool
}

type UnitProber interface {
	Probe(ctx context.Context, u queue.Unit, prevFails int) domain.UnitVerdict
}

type VerdictStore interface {
	Get(ctx context.Context, animeID, provider string) (*domain.ContentVerification, error)
	UpsertUnit(ctx context.Context, animeID, provider string, v domain.UnitVerdict) error
}

type Claimer interface {
	Claim(ctx context.Context) (*queue.Unit, *queue.SkipTask, func(), error)
}

// SkipUnitProber is satisfied by *prober.SkipProber. Named to avoid
// colliding with UnitProber above while keeping the method itself named
// Probe (matching the concrete type) — no adapter needed at the wiring
// site in main.go.
type SkipUnitProber interface {
	Probe(ctx context.Context, t queue.SkipTask, prevFails int) []domain.SkipTiming
}

// SkipStore is the skip-timing persistence surface the worker needs —
// satisfied by *repo.Store.
type SkipStore interface {
	UpsertSkip(ctx context.Context, t domain.SkipTiming) error
	SkipByAnime(ctx context.Context, animeID string) ([]domain.SkipTiming, error)
}

// Worker throttles content-verify probing via N in-process goroutines (see
// workers/NewWorker), each running its own staggered timer loop and claiming
// independently. The throttle — one probe per (unit, provider) at a time —
// is enforced entirely by the Engine's in-flight leases (see
// queue.Engine.Claim), which are in-process-only state: there is no
// distributed lease/lock, so a second REPLICA ticking concurrently would
// happily Claim and Probe its own unit at the same time as this one. The
// k8s deployment MUST stay at replicas: 1 (see
// deploy/kustomize/base/services/content-verify.yaml) or units get
// double-probed — CV_WORKERS is how this service scales concurrency, NOT
// the replica count.
type Worker struct {
	interval   time.Duration
	workers    int
	budget     time.Duration
	shed       ShedChecker
	claimer    Claimer
	prober     UnitProber
	store      VerdictStore
	skipProber SkipUnitProber
	skipStore  SkipStore
	skipBudget time.Duration
	log        *logger.Logger
}

func NewWorker(interval time.Duration, workers int, budget time.Duration, shed ShedChecker, claimer Claimer, prober UnitProber, store VerdictStore,
	skipProber SkipUnitProber, skipStore SkipStore, skipBudget time.Duration, log *logger.Logger) *Worker {
	if workers < 1 {
		workers = 1
	}
	return &Worker{interval: interval, workers: workers, budget: budget, shed: shed, claimer: claimer, prober: prober, store: store,
		skipProber: skipProber, skipStore: skipStore, skipBudget: skipBudget, log: log}
}

// Start launches w.workers in-process probe loops.
func (w *Worker) Start(ctx context.Context) {
	for i := 0; i < w.workers; i++ {
		go w.runLoop(ctx, i)
	}
}

// runLoop is one probe loop. i (0-based) sets two things: the initial-tick
// stagger, so w.workers loops don't all fire in lockstep, and shedMin =
// w.workers-i, the governor level at which THIS loop stops claiming —
// higher-indexed loops shed first as pressure rises, so degradation removes
// workers one at a time rather than all-or-nothing. With workers=1, i=0
// gives shedMin=1: today's single-loop semantics, unchanged.
func (w *Worker) runLoop(ctx context.Context, i int) {
	shedMin := w.workers - i
	// Timer (not ticker): the interval pause runs AFTER each probe
	// completes — spec: "перерыв между пробами - 1 минута". This lets
	// the unit budget exceed the interval (browser-engine resolves alone
	// take 45-90s) without ticks stacking up behind a slow probe.
	initial := w.interval * time.Duration(i+1) / time.Duration(w.workers)
	timer := time.NewTimer(initial)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			w.tick(ctx, shedMin)
			timer.Reset(w.interval)
		}
	}
}

// tick runs one claim+probe cycle for a single worker loop. shedMin is that
// loop's graduated-shedding floor (see runLoop) — the loop sits out this
// tick when the governor-reported degradation level is at or above it.
func (w *Worker) tick(ctx context.Context, shedMin int) {
	if w.shed != nil && w.shed.ShouldShed(shedMin) {
		cvmetrics.TicksSkippedTotal.WithLabelValues("degraded").Inc()
		return
	}
	unit, task, release, err := w.claimer.Claim(ctx)
	// release runs AFTER this tick's persist (deferred to function return,
	// past every branch below) — required so another worker can't re-claim
	// the same unit/provider before its verdict row is actually written.
	if release != nil {
		defer release()
	}
	if err != nil {
		cvmetrics.TicksSkippedTotal.WithLabelValues("claim_error").Inc()
		if w.log != nil {
			w.log.Errorw("claim failed", "error", err)
		}
		return
	}
	if unit == nil && task == nil {
		cvmetrics.TicksSkippedTotal.WithLabelValues("idle").Inc()
		return
	}
	if unit == nil {
		w.tickSkip(ctx, *task)
		return
	}

	// Synth units (ae library truth, kodik translation roster): persist the
	// pre-built verdict as-is, no probe.
	if unit.Synth != nil {
		v := *unit.Synth
		v.ProbedAt = time.Now().UTC()
		w.persist(ctx, *unit, v, "synth")
		return
	}

	prevFails := 0
	if prev, err := w.store.Get(ctx, unit.AnimeID, unit.Provider); err == nil && prev != nil {
		key := unit.Key.String()
		for _, u := range prev.Units {
			if u.Key.String() == key {
				prevFails = u.Fails
				break
			}
		}
	}

	start := time.Now()
	bctx, cancel := context.WithTimeout(ctx, w.budget)
	v := w.prober.Probe(bctx, *unit, prevFails)
	cancel()
	cvmetrics.ProbeDuration.Observe(time.Since(start).Seconds())
	w.persist(ctx, *unit, v, v.Status)
}

func (w *Worker) persist(ctx context.Context, unit queue.Unit, v domain.UnitVerdict, result string) {
	// Episodes-ready count is enumeration truth, not a probe result — stamp it
	// here so both the probe and synth paths carry it. 0 (unknown) stays 0.
	v.Episodes = unit.Episodes
	if err := w.store.UpsertUnit(ctx, unit.AnimeID, unit.Provider, v); err != nil {
		cvmetrics.ProbesTotal.WithLabelValues(unit.Provider, "error").Inc()
		if w.log != nil {
			w.log.Errorw("verdict upsert failed", "anime_id", unit.AnimeID, "provider", unit.Provider, "error", err)
		}
		return
	}
	cvmetrics.ProbesTotal.WithLabelValues(unit.Provider, result).Inc()
	cvmetrics.LastProbeTS.Set(float64(time.Now().Unix()))
	if w.log != nil {
		w.log.Infow("unit probed", "anime_id", unit.AnimeID, "provider", unit.Provider,
			"key", v.Key.String(), "status", v.Status)
	}
}

// tickSkip runs one skip-lane (OP/ED) probe: prevFails is read from the
// existing row for the task's primary unit, the probe runs under
// skipBudget, and every returned row (1 for a locate task, 2 for a pair
// task) is upserted.
func (w *Worker) tickSkip(ctx context.Context, t queue.SkipTask) {
	prevFails := w.skipPrevFails(ctx, t)

	bctx, cancel := context.WithTimeout(ctx, w.skipBudget)
	rows := w.skipProber.Probe(bctx, t, prevFails)
	cancel()

	for _, row := range rows {
		if err := w.skipStore.UpsertSkip(ctx, row); err != nil {
			if w.log != nil {
				w.log.Errorw("skip timing upsert failed", "anime_id", row.AnimeID, "provider", row.Provider,
					"team", row.Team, "episode", row.Episode, "error", err)
			}
		}
	}

	result := ""
	if len(rows) > 0 {
		result = rows[0].OpStatus
	}
	cvmetrics.SkipProbesTotal.WithLabelValues(t.Unit.Provider, result).Inc()
	if w.log != nil {
		w.log.Infow("skip unit probed", "anime_id", t.Unit.AnimeID, "provider", t.Unit.Provider,
			"episode", t.Unit.Episode, "pair", t.Pair != nil, "re_pair", t.RePair, "result", result)
	}
}

// skipPrevFails reads the max Fails recorded across existing skip rows for
// the task's primary unit (provider+team+episode) — at most one row can
// match (idx_skip_unit is unique on that tuple); "max" mirrors the spec's
// phrasing and stays correct if that ever changes.
func (w *Worker) skipPrevFails(ctx context.Context, t queue.SkipTask) int {
	rows, err := w.skipStore.SkipByAnime(ctx, t.Unit.AnimeID)
	if err != nil {
		if w.log != nil {
			w.log.Warnw("skip rows fetch failed", "anime_id", t.Unit.AnimeID, "error", err)
		}
		return 0
	}
	prevFails := 0
	for _, r := range rows {
		if r.Provider == t.Unit.Provider && r.Team == t.Unit.Team && r.Episode == t.Unit.Episode && r.Fails > prevFails {
			prevFails = r.Fails
		}
	}
	return prevFails
}
