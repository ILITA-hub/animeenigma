// Package service hosts the governor evaluation loop: poll Prometheus,
// smooth through the hysteresis Machine, apply the owner override, publish
// (Redis + gauges), and report transitions to the analytics history sink.
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
	PublishLevel(ctx context.Context, level domain.Level, reasons []domain.Reason, ttl time.Duration) error
	Override(ctx context.Context) (*domain.Level, error)
}

// TransitionSink persists level transitions (repo.AnalyticsSink). Best-effort:
// errors are logged by the implementation, never bubble into the loop.
type TransitionSink interface {
	Report(ctx context.Context, t domain.Transition)
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

	machine *Machine

	mu        sync.RWMutex
	snapshot  domain.Snapshot
	failCount int
	published domain.Level
	now       func() time.Time // injectable for tests
}

// New builds a Governor. enterTicks/exitTicks parameterize the Machine.
func New(source VerdictSource, store LevelStore, sink TransitionSink, log *logger.Logger,
	tick, levelTTL time.Duration, enterTicks, exitTicks, promFailTicks int) *Governor {
	return &Governor{
		source:        source,
		store:         store,
		sink:          sink,
		log:           log,
		tick:          tick,
		levelTTL:      levelTTL,
		promFailTicks: promFailTicks,
		machine:       NewMachine(enterTicks, exitTicks),
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
			// Grace window: keep the last published level alive (TTL refresh).
			g.publish(ctx, g.currentPublished(), g.Snapshot().Reasons)
			return
		}
		// Fail-open: never shed on missing data.
		verdict = domain.Verdict{Target: domain.LevelNormal}
	} else {
		g.mu.Lock()
		g.failCount = 0
		g.mu.Unlock()
	}

	computed := domain.LevelNormal
	if promHealthy {
		computed, _ = g.machine.Tick(verdict.Target)
	}

	override, oerr := g.store.Override(ctx)
	if oerr != nil {
		g.log.Warnw("override read failed; ignoring override", "error", oerr)
		override = nil
	}

	published := computed
	reasons := verdict.Reasons
	if !promHealthy {
		reasons = []domain.Reason{{Signal: domain.ReasonPrometheusUnreachable, Severity: domain.SeverityInfo}}
	}
	if override != nil {
		published = *override
		reasons = append([]domain.Reason{{Signal: domain.ReasonManualOverride, Severity: domain.SeverityInfo}}, reasons...)
	}

	g.publish(ctx, published, reasons)

	g.mu.Lock()
	prev := g.published
	g.published = published
	g.snapshot = domain.Snapshot{
		Level:       published,
		Reasons:     reasons,
		Signals:     verdict.Signals,
		Override:    override,
		Target:      verdict.Target,
		UpdatedAt:   g.now(),
		PromHealthy: promHealthy,
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
func (g *Governor) publish(ctx context.Context, level domain.Level, reasons []domain.Reason) {
	if err := g.store.PublishLevel(ctx, level, reasons, g.levelTTL); err != nil {
		g.log.Warnw("redis publish failed (consumers fail open)", "error", err)
	}
	govmetrics.DegradationLevel.Set(float64(level))

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
	for _, syn := range []string{domain.ReasonManualOverride, domain.ReasonPrometheusUnreachable} {
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
