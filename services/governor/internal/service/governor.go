// Package service hosts the governor evaluation loop: poll Prometheus, smooth
// the raw pressure score through ONE asymmetric-EWMA Smoother, quantize it into
// the discrete level with per-boundary hysteresis, apply the owner override,
// publish (Redis + gauges), and report transitions to the analytics history
// sink.
//
// One smoothed state (the score) drives both published values: the score is
// published directly and the level is Quantizer(score), so level and score can
// never disagree (the old design ran a separate streak Machine for the level).
package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/govmetrics"
)

// VerdictSource yields the instantaneous pressure verdict (promquery.Client).
type VerdictSource interface {
	FetchVerdict(ctx context.Context) (domain.Verdict, error)
}

// LevelStore publishes the level for consumers and reads the owner override
// (repo.RedisStore).
type LevelStore interface {
	PublishLevel(ctx context.Context, level domain.Level, score float64, reasons []domain.Reason, ttl time.Duration) error
	Override(ctx context.Context) (*domain.Level, error)
}

// TransitionSink persists level transitions (repo.AnalyticsSink). Best-effort:
// errors are logged by the implementation, never bubble into the loop.
type TransitionSink interface {
	Report(ctx context.Context, t domain.Transition)
}

// Tuning carries the governor's timing/hysteresis/staleness knobs. Grouped into
// a struct so New stays readable as the parameter set grows.
type Tuning struct {
	Tick          time.Duration
	LevelTTL      time.Duration
	PromFailTicks int
	AlphaUp       float64
	AlphaDown     float64
	EnterElevated float64
	ExitElevated  float64
	EnterCritical float64
	ExitCritical  float64
	// StalenessMax is the freshest-sample age above which the governor holds the
	// level rather than trusting the (lagging) signal. 0 disables the guard.
	StalenessMax time.Duration
}

// Governor owns the tick loop and the published snapshot.
type Governor struct {
	source VerdictSource
	store  LevelStore
	sink   TransitionSink
	log    *logger.Logger

	tick          time.Duration
	levelTTL      time.Duration
	promFailTicks int
	stalenessMax  time.Duration

	quantizer *Quantizer
	smoother  *Smoother

	mu        sync.RWMutex
	snapshot  domain.Snapshot
	failCount int
	published domain.Level
	now       func() time.Time // injectable for tests
}

// New builds a Governor from the injected IO edges and the Tuning knobs.
func New(source VerdictSource, store LevelStore, sink TransitionSink, log *logger.Logger, t Tuning) *Governor {
	return &Governor{
		source:        source,
		store:         store,
		sink:          sink,
		log:           log,
		tick:          t.Tick,
		levelTTL:      t.LevelTTL,
		promFailTicks: t.PromFailTicks,
		stalenessMax:  t.StalenessMax,
		quantizer:     NewQuantizer(t.EnterElevated, t.ExitElevated, t.EnterCritical, t.ExitCritical),
		smoother:      NewSmoother(t.AlphaUp, t.AlphaDown),
		snapshot:      domain.Snapshot{Reasons: []domain.Reason{}, PromHealthy: true},
		now:           time.Now,
	}
}

// Start runs the loop until ctx is done. The first tick fires immediately.
func (g *Governor) Start(ctx context.Context) {
	go func() {
		g.RunTick(ctx)
		t := time.NewTicker(g.tick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				g.RunTick(ctx)
			}
		}
	}()
}

// Snapshot returns the current published state (status endpoint).
func (g *Governor) Snapshot() domain.Snapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.snapshot
}

// RunTick performs one evaluate→publish cycle. Exported for tests.
func (g *Governor) RunTick(ctx context.Context) {
	verdict, err := g.source.FetchVerdict(ctx)
	promHealthy := err == nil
	if err != nil {
		g.mu.Lock()
		g.failCount++
		fails := g.failCount
		g.mu.Unlock()
		govmetrics.GovernorEvalFailuresTotal.Inc()
		g.log.Warnw("prometheus poll failed", "consecutive", fails, "error", err)
		if fails < g.promFailTicks {
			// Grace window: keep the last published level alive (TTL refresh),
			// decaying the score one tick rather than freezing it.
			g.publish(ctx, g.currentPublished(), g.smoother.Tick(0), g.Snapshot().Reasons)
			return
		}
		// Sustained loss: fail open — reset both smoothers so recovery starts
		// from a clean 0, and publish LevelNormal (never shed on missing data).
		g.smoother.Reset()
		g.quantizer.Reset()
		verdict = domain.Verdict{Target: domain.LevelNormal}
	} else {
		g.mu.Lock()
		g.failCount = 0
		g.mu.Unlock()
	}

	computed := domain.LevelNormal
	score := 0.0
	stale := false
	if promHealthy {
		if verdict.SampleAgeSeconds >= 0 {
			govmetrics.SignalStaleness.Set(verdict.SampleAgeSeconds)
		}
		govmetrics.EgressUplinkFraction.Set(verdict.EgressFraction) // 0 when disabled/idle
		stale = g.stalenessMax > 0 && verdict.SampleAgeSeconds > g.stalenessMax.Seconds()
		if stale {
			// The freshest sample is older than the budget: scrape or rule
			// evaluation is lagging (often under the very load we watch). Do NOT
			// advance the smoother/quantizer — trusting stale data could relax
			// shedding. Hold the current computed level and last score.
			govmetrics.GovernorStaleTicksTotal.Inc()
			g.log.Warnw("pressure signal stale; holding level",
				"age_s", verdict.SampleAgeSeconds, "budget_s", g.stalenessMax.Seconds())
			computed = g.quantizer.Level()
			score = g.smoother.Value()
		} else {
			score = g.smoother.Tick(verdict.Score)
			computed = g.quantizer.Tick(score)
		}
	}

	override, oerr := g.store.Override(ctx)
	if oerr != nil {
		g.log.Warnw("override read failed; ignoring override", "error", oerr)
		override = nil
	}

	published := computed
	reasons := verdict.Reasons
	switch {
	case !promHealthy:
		reasons = []domain.Reason{{Signal: domain.ReasonPrometheusUnreachable, Severity: domain.SeverityInfo}}
	case stale:
		reasons = []domain.Reason{{Signal: domain.ReasonSignalStale, Severity: domain.SeverityInfo}}
	case computed > domain.LevelNormal && len(reasons) == 0:
		// Level held up purely by the slow score decay — surface WHY so the
		// dashboard never shows "level 1, reasons: []".
		reasons = []domain.Reason{{Signal: domain.ReasonHeldByHysteresis, Severity: domain.SeverityInfo}}
	}
	if override != nil {
		published = *override
		reasons = append([]domain.Reason{{Signal: domain.ReasonManualOverride, Severity: domain.SeverityInfo}}, reasons...)
		score = map[domain.Level]float64{
			domain.LevelNormal: 0, domain.LevelElevated: 0.5, domain.LevelCritical: 1.0,
		}[*override]
	}

	g.publish(ctx, published, score, reasons)

	g.mu.Lock()
	prev := g.published
	g.published = published
	g.snapshot = domain.Snapshot{
		Level:            published,
		Score:            score,
		Reasons:          reasons,
		Signals:          verdict.Signals,
		Override:         override,
		Target:           verdict.Target,
		UpdatedAt:        g.now(),
		PromHealthy:      promHealthy,
		SampleAgeSeconds: verdict.SampleAgeSeconds,
		EgressFraction:   verdict.EgressFraction,
	}
	g.mu.Unlock()

	if prev != published {
		tr := domain.Transition{
			TS:           g.now(),
			FromLevel:    prev,
			ToLevel:      published,
			Reasons:      flattenReasons(reasons),
			SignalValues: verdict.Signals,
		}
		govmetrics.GovernorTransitionsTotal.WithLabelValues(strconv.Itoa(int(published))).Inc()
		g.log.Infow("degradation level transition",
			"from", prev, "to", published, "reasons", tr.Reasons)
		g.sink.Report(ctx, tr)
	}
}

func (g *Governor) currentPublished() domain.Level {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.published
}

// publish refreshes the Redis keys and the Prometheus gauges.
func (g *Governor) publish(ctx context.Context, level domain.Level, score float64, reasons []domain.Reason) {
	if err := g.store.PublishLevel(ctx, level, score, reasons, g.levelTTL); err != nil {
		g.log.Warnw("redis publish failed (consumers fail open)", "error", err)
	}
	govmetrics.DegradationLevel.Set(float64(level))
	govmetrics.DegradationScore.Set(score)

	active := map[string]bool{}
	for _, r := range reasons {
		active[r.Signal+"\x00"+r.Severity] = true
	}
	// Fixed label universe: absent reasons are explicitly zeroed so a stale
	// "active" can never linger after the condition clears.
	for _, sig := range domain.BreachSignals {
		for _, sev := range []string{domain.SeverityElevated, domain.SeverityCritical} {
			v := 0.0
			if active[sig+"\x00"+sev] {
				v = 1
			}
			govmetrics.DegradationReasonActive.WithLabelValues(sig, sev).Set(v)
		}
	}
	for _, syn := range []string{
		domain.ReasonManualOverride, domain.ReasonPrometheusUnreachable,
		domain.ReasonHeldByHysteresis, domain.ReasonSignalStale,
	} {
		v := 0.0
		if active[syn+"\x00"+domain.SeverityInfo] {
			v = 1
		}
		govmetrics.DegradationReasonActive.WithLabelValues(syn, domain.SeverityInfo).Set(v)
	}
}

func flattenReasons(reasons []domain.Reason) []string {
	out := make([]string, 0, len(reasons))
	for _, r := range reasons {
		if r.Severity == domain.SeverityInfo {
			out = append(out, r.Signal)
			continue
		}
		out = append(out, fmt.Sprintf("%s:%s", r.Signal, r.Severity))
	}
	return out
}
