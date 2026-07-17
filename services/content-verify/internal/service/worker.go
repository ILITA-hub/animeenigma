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
	Claim(ctx context.Context) (*queue.Unit, bool, error)
}

// Worker throttles content-verify probing to one unit at a time (one Claim +
// Probe per tick). That throttle is enforced entirely in-process — Claimer
// has no distributed lease/lock, so a second replica ticking concurrently
// would happily Claim and Probe its own unit at the same time as this one.
// The k8s deployment MUST stay at replicas: 1 (see
// deploy/kustomize/base/services/content-verify.yaml) or units get
// double-probed.
type Worker struct {
	interval time.Duration
	budget   time.Duration
	shed     ShedChecker
	claimer  Claimer
	prober   UnitProber
	store    VerdictStore
	log      *logger.Logger
}

func NewWorker(interval, budget time.Duration, shed ShedChecker, claimer Claimer, prober UnitProber, store VerdictStore, log *logger.Logger) *Worker {
	return &Worker{interval: interval, budget: budget, shed: shed, claimer: claimer, prober: prober, store: store, log: log}
}

func (w *Worker) Start(ctx context.Context) {
	go func() {
		// Timer (not ticker): the interval pause runs AFTER each probe
		// completes — spec: "перерыв между пробами - 1 минута". This lets
		// the unit budget exceed the interval (browser-engine resolves alone
		// take 45-90s) without ticks stacking up behind a slow probe.
		timer := time.NewTimer(w.interval)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				w.tick(ctx)
				timer.Reset(w.interval)
			}
		}
	}()
}

func (w *Worker) tick(ctx context.Context) {
	if w.shed != nil && w.shed.ShouldShed(1) {
		cvmetrics.TicksSkippedTotal.WithLabelValues("degraded").Inc()
		return
	}
	unit, _, err := w.claimer.Claim(ctx)
	if err != nil {
		cvmetrics.TicksSkippedTotal.WithLabelValues("claim_error").Inc()
		if w.log != nil {
			w.log.Errorw("claim failed", "error", err)
		}
		return
	}
	if unit == nil {
		cvmetrics.TicksSkippedTotal.WithLabelValues("idle").Inc()
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
