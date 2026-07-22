// Package service runs the probe worker: N loops claiming back-to-back,
// governor-gated, results upserted into the verdict store.
package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	gometrics "github.com/ILITA-hub/animeenigma/libs/metrics"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/cvmetrics"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
)

// ScoreSource is satisfied by *cache.DegradationWatcher.
type ScoreSource interface {
	Score() float64
}

// DemandSource reports the pending probe backlog (satisfied by *queue.Engine).
type DemandSource interface {
	PendingCount() int
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

// ProviderDeferrer is the optional Claimer extension the worker uses to
// report an upstream 503 (see queue.Engine.Defer). Asserted dynamically so
// test fakes that only implement Claim keep compiling.
type ProviderDeferrer interface {
	Defer(animeID, provider string, retryAfter time.Duration)
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

// Worker runs content-verify probing via N in-process goroutines (see
// workers/NewWorker), each claiming independently and probing back-to-back
// while work is available. The throttle — one probe per unit and a bounded
// number per provider at a time — is enforced entirely by the Engine's
// in-flight leases (see
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
	score      ScoreSource
	claimer    Claimer
	prober     UnitProber
	store      VerdictStore
	skipProber SkipUnitProber
	skipStore  SkipStore
	skipBudget time.Duration
	log        *logger.Logger
	curve      Curve
	demandPer  int
	demand     DemandSource
}

func NewWorker(interval time.Duration, workers int, budget time.Duration, score ScoreSource, claimer Claimer, prober UnitProber, store VerdictStore,
	skipProber SkipUnitProber, skipStore SkipStore, skipBudget time.Duration, log *logger.Logger,
	curve Curve, demandPer int, demand DemandSource) *Worker {
	if workers < 1 {
		workers = 1
	}
	if demandPer < 1 {
		demandPer = 1
	}
	return &Worker{interval: interval, workers: workers, budget: budget, score: score, claimer: claimer, prober: prober, store: store,
		skipProber: skipProber, skipStore: skipStore, skipBudget: skipBudget, log: log,
		curve: curve, demandPer: demandPer, demand: demand}
}

// Start launches w.workers in-process probe loops.
func (w *Worker) Start(ctx context.Context) {
	for i := 0; i < w.workers; i++ {
		go w.runLoop(ctx, i)
	}
}

// runLoop is one probe loop. i (0-based, < w.workers) sets the initial
// stagger, so w.workers loops don't all fire in lockstep, and is passed to
// tick unchanged as the loop's identity — the admission decision (whether i
// participates this tick) is made fresh in tick from the current score and
// demand, not fixed at loop-start.
//
// Probes run back-to-back: as long as tick reports it did work, the loop
// claims again immediately. Upstream politeness is enforced by the engine's
// per-provider lease limit and load by the pressure curve — not by pacing.
// The interval is only the park/idle backoff: how long a loop sleeps when it
// is pressure/demand-parked, the queue has nothing claimable, or claiming
// errored.
func (w *Worker) runLoop(ctx context.Context, i int) {
	initial := w.interval * time.Duration(i+1) / time.Duration(w.workers)
	timer := time.NewTimer(initial)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		for w.tick(ctx, i) {
			if ctx.Err() != nil {
				return
			}
		}
		timer.Reset(w.interval)
	}
}

// tick runs one claim+probe cycle for loop i and reports whether it did any
// work (probed or ran a skip task — a probe that
// ended in a provider deferral still counts: real time was spent and the
// engine has already parked that provider). false means the loop should back
// off for an interval: it is parked by pressure/demand, the queue is idle,
// or the claim errored. The loop participates only while
// i < min(curve(score), demandCap) — pressure removes loops from the top one
// at a time; a shallow queue keeps most loops parked (demand cap floors at 1
// so a cold start can still build the first queue snapshot).
func (w *Worker) tick(ctx context.Context, i int) bool {
	pressureCap := w.curve.Cap(w.scoreValue())
	demandCap := 1
	if w.demand != nil {
		if pc := w.demand.PendingCount(); pc > 0 {
			demandCap = (pc + w.demandPer - 1) / w.demandPer
		}
	} else {
		demandCap = w.workers
	}
	effective := pressureCap
	if demandCap < effective {
		effective = demandCap
	}
	if i == 0 { // one writer for the gauges — loop 0 ticks most often
		cvmetrics.WorkerCap.WithLabelValues("pressure").Set(float64(pressureCap))
		cvmetrics.WorkerCap.WithLabelValues("demand").Set(float64(demandCap))
		cvmetrics.WorkerCap.WithLabelValues("effective").Set(float64(effective))
		// Shed-state family (dashboard "Shed state" panel): mirror the
		// library_encode graded semantics — 0 full, 1 reduced, 2 paused.
		// Demand-capped parking is not shedding, so only the PRESSURE cap
		// drives this gauge.
		shed := 0.0
		switch {
		case pressureCap <= 0:
			shed = 2
		case pressureCap < w.workers:
			shed = 1
		}
		gometrics.DegradationShed.WithLabelValues("content_verify").Set(shed)
	}
	if i >= effective {
		cvmetrics.TicksSkippedTotal.WithLabelValues("degraded").Inc()
		return false
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
		return false
	}
	if unit == nil && task == nil {
		cvmetrics.TicksSkippedTotal.WithLabelValues("idle").Inc()
		return false
	}
	if unit == nil {
		w.tickSkip(ctx, *task)
		return true
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
	// Deferred sentinel (upstream 503 — provider down / negative-cached): the
	// verdict is dropped, no Fails++ (a down provider is not a failing
	// episode), and the engine defers the (anime, provider) pair until the
	// upstream negative-cache entry expires.
	if v.Status == domain.StatusDeferred {
		if d, ok := w.claimer.(ProviderDeferrer); ok {
			d.Defer(unit.AnimeID, unit.Provider, v.RetryAfter)
		}
		cvmetrics.TicksSkippedTotal.WithLabelValues("provider_unavailable").Inc()
		return true
	}
	w.persist(ctx, *unit, v, v.Status)
	return true
}

func (w *Worker) scoreValue() float64 {
	if w.score == nil {
		return 0
	}
	return w.score.Score()
}

func (w *Worker) persist(ctx context.Context, unit queue.Unit, v domain.UnitVerdict, result string) {
	// Episodes-ready count is enumeration truth, not a probe result — stamp it
	// here so both the probe and synth paths carry it. 0 (unknown) stays 0.
	v.Episodes = unit.Episodes
	if err := w.store.UpsertUnit(ctx, unit.AnimeID, unit.Provider, v); err != nil {
		cvmetrics.ProbesTotal.WithLabelValues(unit.Provider, "error", unit.Band.Label()).Inc()
		if w.log != nil {
			w.log.Errorw("verdict upsert failed", "anime_id", unit.AnimeID, "provider", unit.Provider, "error", err)
		}
		return
	}
	cvmetrics.ProbesTotal.WithLabelValues(unit.Provider, result, unit.Band.Label()).Inc()
	if v.Audio != nil {
		cvmetrics.VerdictsTotal.WithLabelValues(orUnknown(v.Audio.Lang)).Inc()
	}
	if v.Hardsub != nil && v.Hardsub.Present {
		cvmetrics.HardsubTotal.WithLabelValues(orUnknown(v.Hardsub.Lang)).Inc()
	}
	cvmetrics.LastProbeTS.Set(float64(time.Now().Unix()))
	if w.log != nil {
		w.log.Infow("unit probed", "anime_id", unit.AnimeID, "provider", unit.Provider,
			"key", v.Key.String(), "status", v.Status)
	}
}

// orUnknown maps an empty language code to the metric label "unknown" so a
// missing detection still emits a bounded, non-empty Prometheus label.
func orUnknown(lang string) string {
	if lang == "" {
		return "unknown"
	}
	return lang
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
