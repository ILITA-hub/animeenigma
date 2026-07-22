package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/domain"
)

type fakeSource struct {
	mu       sync.Mutex
	verdicts []domain.Verdict
	errs     []error
	i        int
}

func (f *fakeSource) FetchVerdict(context.Context) (domain.Verdict, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := f.i
	if idx >= len(f.verdicts) {
		idx = len(f.verdicts) - 1
	}
	f.i++
	if idx < len(f.errs) && f.errs[idx] != nil {
		return domain.Verdict{}, f.errs[idx]
	}
	return f.verdicts[idx], nil
}

type fakeStore struct {
	mu        sync.Mutex
	level     *domain.Level
	score     float64
	reasons   []domain.Reason
	ttl       time.Duration
	override  *domain.Level
	pubErr    error
	publishes int
}

func (f *fakeStore) PublishLevel(_ context.Context, l domain.Level, score float64, r []domain.Reason, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.level, f.score, f.reasons, f.ttl = &l, score, r, ttl
	f.publishes++
	return f.pubErr
}

func (f *fakeStore) Override(context.Context) (*domain.Level, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.override, nil
}

type fakeSink struct {
	mu          sync.Mutex
	transitions []domain.Transition
}

func (f *fakeSink) Report(_ context.Context, t domain.Transition) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transitions = append(f.transitions, t)
}

// testTuning is the production-default tuning at a 1s tick, staleness disabled
// unless a test sets it. failTicks parameterizes the prom-failure fail-open.
func testTuning(failTicks int) Tuning {
	return Tuning{
		Tick: time.Second, LevelTTL: time.Minute, PromFailTicks: failTicks,
		AlphaUp: 0.5, AlphaDown: 0.05,
		EnterElevated: 0.45, ExitElevated: 0.20, EnterCritical: 0.90, ExitCritical: 0.55,
		StalenessMax: 0,
	}
}

func newTestGovernor(src *fakeSource, store *fakeStore, sink *fakeSink, failTicks int) *Governor {
	return New(src, store, sink, logger.Default(), testTuning(failTicks))
}

// breach builds a verdict whose raw Score matches the severity (0.5 elevated,
// 1.0 critical) so it drives the score-quantizer. SampleAgeSeconds 0 = fresh.
func breach(target domain.Level, sig, sev string) domain.Verdict {
	score := 0.5
	if sev == domain.SeverityCritical {
		score = 1.0
	}
	return domain.Verdict{
		Target:           target,
		Score:            score,
		Reasons:          []domain.Reason{{Signal: sig, Severity: sev}},
		Signals:          map[string]float64{"ae:host_psi_cpu_some:ratio": 0.5},
		SampleAgeSeconds: 0,
	}
}

func TestGovernor_RaisesFromSustainedScoreAndLogsTransition(t *testing.T) {
	// Elevated raw score 0.5 smooths 0.25, 0.375, 0.4375, 0.469 — crosses
	// enterElevated (0.45) on the 4th tick (~enter-fast).
	src := &fakeSource{verdicts: []domain.Verdict{breach(1, "psi_cpu_some", "elevated")}}
	store := &fakeStore{}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 3)

	for i := 0; i < 3; i++ {
		g.RunTick(context.Background())
		assert.Equal(t, domain.LevelNormal, g.Snapshot().Level, "score still below enter threshold")
	}
	g.RunTick(context.Background()) // 4th tick crosses 0.45

	snap := g.Snapshot()
	assert.Equal(t, domain.LevelElevated, snap.Level)
	assert.Equal(t, domain.LevelElevated, *store.level)
	require.Len(t, sink.transitions, 1)
	assert.Equal(t, domain.LevelNormal, sink.transitions[0].FromLevel)
	assert.Equal(t, domain.LevelElevated, sink.transitions[0].ToLevel)
	assert.Equal(t, []string{"psi_cpu_some:elevated"}, sink.transitions[0].Reasons)
	assert.Equal(t, 0.5, sink.transitions[0].SignalValues["ae:host_psi_cpu_some:ratio"])
}

func TestGovernor_OverridePinsPublishedLevel(t *testing.T) {
	src := &fakeSource{verdicts: []domain.Verdict{{Target: domain.LevelNormal, SampleAgeSeconds: 0}}}
	pin := domain.LevelCritical
	store := &fakeStore{override: &pin}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 3)

	g.RunTick(context.Background())
	snap := g.Snapshot()
	assert.Equal(t, domain.LevelCritical, snap.Level)
	require.NotNil(t, snap.Override)
	assert.Equal(t, domain.LevelCritical, *snap.Override)
	assert.Equal(t, domain.ReasonManualOverride, snap.Reasons[0].Signal)
	require.Len(t, sink.transitions, 1)
	assert.Contains(t, sink.transitions[0].Reasons, domain.ReasonManualOverride)

	// Clearing the override snaps back next tick (computed level is Normal).
	store.mu.Lock()
	store.override = nil
	store.mu.Unlock()
	g.RunTick(context.Background())
	assert.Equal(t, domain.LevelNormal, g.Snapshot().Level)
	assert.Len(t, sink.transitions, 2)
}

func TestGovernor_PromFailureGraceThenFailOpen(t *testing.T) {
	boom := errors.New("connection refused")
	// Score 1.0 => first healthy tick smooths to 0.5, crossing enterElevated.
	healthy := breach(1, "psi_cpu_some", "elevated")
	healthy.Score = 1.0
	src := &fakeSource{
		verdicts: []domain.Verdict{healthy, {}, {}, {}, {}},
		errs:     []error{nil, boom, boom, boom},
	}
	store := &fakeStore{}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 3)

	g.RunTick(context.Background()) // healthy: score 1.0 -> 0.5 -> Elevated
	assert.Equal(t, domain.LevelElevated, g.Snapshot().Level)
	assert.Equal(t, 0.5, store.score, "raw 1.0 smooths to 0.5 on the first healthy tick")

	g.RunTick(context.Background()) // fail 1/3 — grace: level held, score decays
	assert.Equal(t, domain.LevelElevated, g.Snapshot().Level)
	wantDecay := 0.5
	wantDecay += 0.05 * (0 - wantDecay) // Smoother.Tick(0): 0.475
	assert.Equal(t, wantDecay, store.score,
		"grace tick runs Tick(0) (decay by alphaDown), proving it's not a freeze")

	g.RunTick(context.Background()) // fail 2/3
	assert.Equal(t, domain.LevelElevated, g.Snapshot().Level)
	assert.True(t, g.Snapshot().PromHealthy, "grace window keeps last snapshot")

	g.RunTick(context.Background()) // fail 3/3 — fail-open to Normal
	snap := g.Snapshot()
	assert.Equal(t, domain.LevelNormal, snap.Level)
	assert.False(t, snap.PromHealthy)
	assert.Equal(t, domain.ReasonPrometheusUnreachable, snap.Reasons[0].Signal)
	assert.Equal(t, 0.0, store.score, "sustained loss resets both smoothers and fails open to score 0")
}

func TestGovernor_PublishesSmoothedScore(t *testing.T) {
	src := &fakeSource{verdicts: []domain.Verdict{{Target: domain.LevelNormal, Score: 1.0, SampleAgeSeconds: 0}}}
	store := &fakeStore{}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 3)

	g.RunTick(context.Background())
	assert.Equal(t, 0.5, store.score, "first tick, alphaUp 0.5 halves the 0->1 gap")
	assert.Equal(t, 0.5, g.Snapshot().Score)
}

func TestGovernor_OverridePinsScore(t *testing.T) {
	src := &fakeSource{verdicts: []domain.Verdict{
		{Target: domain.LevelNormal, Score: 1.0, SampleAgeSeconds: 0},
		{Target: domain.LevelNormal, Score: 1.0, SampleAgeSeconds: 0},
	}}
	pin := domain.LevelElevated
	store := &fakeStore{override: &pin}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 3)

	g.RunTick(context.Background())
	assert.Equal(t, 0.5, store.score, "tick1: pin (0.5) happens to match natural smoothing here")

	g.RunTick(context.Background())
	assert.Equal(t, 0.5, store.score, "tick2: natural smoothing would now be 0.75 — pin must still force 0.5")

	// A Critical override pins exactly 1.0 even though raw 0.0 stays at 0.0.
	src2 := &fakeSource{verdicts: []domain.Verdict{{Target: domain.LevelNormal, Score: 0.0, SampleAgeSeconds: 0}}}
	critPin := domain.LevelCritical
	store2 := &fakeStore{override: &critPin}
	g2 := newTestGovernor(src2, store2, &fakeSink{}, 3)

	g2.RunTick(context.Background())
	assert.Equal(t, 1.0, store2.score, "override level 2 pins the score to 1.0")
}

func TestGovernor_NoTransitionSpamWhenStable(t *testing.T) {
	src := &fakeSource{verdicts: []domain.Verdict{{Target: domain.LevelNormal, SampleAgeSeconds: 0}}}
	store := &fakeStore{}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 3)

	for i := 0; i < 5; i++ {
		g.RunTick(context.Background())
	}
	assert.Empty(t, sink.transitions)
	assert.Equal(t, 5, store.publishes, "every tick refreshes the TTL")
}

func TestGovernor_HeldByHysteresisReasonWhenLevelOutlivesBreach(t *testing.T) {
	// Tick 1 raises to Elevated (score 1.0 -> 0.5). Tick 2+ feed a Normal verdict
	// with NO reasons; the smoothed score decays slowly so the level stays
	// Elevated — that held level must carry the held_by_hysteresis reason.
	high := breach(1, "psi_cpu_some", "elevated")
	high.Score = 1.0
	normal := domain.Verdict{Target: domain.LevelNormal, Score: 0, SampleAgeSeconds: 0}
	src := &fakeSource{verdicts: []domain.Verdict{high, normal}}
	g := newTestGovernor(src, &fakeStore{}, &fakeSink{}, 3)

	g.RunTick(context.Background())
	require.Equal(t, domain.LevelElevated, g.Snapshot().Level)

	g.RunTick(context.Background())
	snap := g.Snapshot()
	assert.Equal(t, domain.LevelElevated, snap.Level, "level held by slow decay")
	require.Len(t, snap.Reasons, 1)
	assert.Equal(t, domain.ReasonHeldByHysteresis, snap.Reasons[0].Signal)
	assert.Equal(t, domain.SeverityInfo, snap.Reasons[0].Severity)
}

func TestGovernor_StaleSignalHoldsLevelAndReasons(t *testing.T) {
	tuning := testTuning(3)
	tuning.StalenessMax = 45 * time.Second
	// Tick1 fresh & high -> Elevated. Tick2 is stale (age 999s) AND carries a
	// Normal/zero score that WOULD lower the level — the hold must ignore it.
	high := breach(1, "psi_cpu_some", "elevated")
	high.Score = 1.0
	stale := domain.Verdict{Target: domain.LevelNormal, Score: 0, SampleAgeSeconds: 999}
	src := &fakeSource{verdicts: []domain.Verdict{high, stale}}
	store := &fakeStore{}
	g := New(src, store, &fakeSink{}, logger.Default(), tuning)

	g.RunTick(context.Background())
	require.Equal(t, domain.LevelElevated, g.Snapshot().Level)
	held := store.score

	g.RunTick(context.Background())
	snap := g.Snapshot()
	assert.Equal(t, domain.LevelElevated, snap.Level, "stale tick must not lower the level")
	assert.Equal(t, held, store.score, "stale tick must not advance the smoother")
	require.Len(t, snap.Reasons, 1)
	assert.Equal(t, domain.ReasonSignalStale, snap.Reasons[0].Signal)
	assert.Equal(t, float64(999), snap.SampleAgeSeconds)
}
