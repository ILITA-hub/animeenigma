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

func newTestGovernor(src *fakeSource, store *fakeStore, sink *fakeSink, enter, exit, failTicks int) *Governor {
	return New(src, store, sink, logger.Default(), time.Second, time.Minute, enter, exit, failTicks, 0.5, 0.05)
}

func breach(target domain.Level, sig, sev string) domain.Verdict {
	return domain.Verdict{
		Target:  target,
		Reasons: []domain.Reason{{Signal: sig, Severity: sev}},
		Signals: map[string]float64{"ae:host_psi_cpu_some:ratio": 0.5},
	}
}

func TestGovernor_RaisesAfterEnterTicksAndLogsTransition(t *testing.T) {
	src := &fakeSource{verdicts: []domain.Verdict{breach(1, "psi_cpu_some", "elevated")}}
	store := &fakeStore{}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 2, 3, 3)

	g.RunTick(context.Background())
	assert.Equal(t, domain.LevelNormal, g.Snapshot().Level, "one tick is not enough")
	g.RunTick(context.Background())

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
	src := &fakeSource{verdicts: []domain.Verdict{{Target: domain.LevelNormal}}}
	pin := domain.LevelCritical
	store := &fakeStore{override: &pin}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 2, 3, 3)

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
	src := &fakeSource{
		verdicts: []domain.Verdict{breach(1, "psi_cpu_some", "elevated"), {}, {}, {}, {}},
		errs:     []error{nil, boom, boom, boom},
	}
	store := &fakeStore{}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 1, 10, 3)

	g.RunTick(context.Background()) // healthy: raises to Elevated (enterTicks=1)
	assert.Equal(t, domain.LevelElevated, g.Snapshot().Level)

	g.RunTick(context.Background()) // fail 1/3 — grace: level held, TTL refreshed
	g.RunTick(context.Background()) // fail 2/3
	assert.Equal(t, domain.LevelElevated, g.Snapshot().Level)
	assert.True(t, g.Snapshot().PromHealthy, "grace window keeps last snapshot")

	g.RunTick(context.Background()) // fail 3/3 — fail-open to Normal
	snap := g.Snapshot()
	assert.Equal(t, domain.LevelNormal, snap.Level)
	assert.False(t, snap.PromHealthy)
	assert.Equal(t, domain.ReasonPrometheusUnreachable, snap.Reasons[0].Signal)
}

func TestGovernor_PublishesSmoothedScore(t *testing.T) {
	src := &fakeSource{verdicts: []domain.Verdict{{Target: domain.LevelNormal, Score: 1.0}}}
	store := &fakeStore{}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 2, 3, 3)

	g.RunTick(context.Background())
	assert.Equal(t, 0.5, store.score, "first tick, alphaUp 0.5 halves the 0->1 gap")
	assert.Equal(t, 0.5, g.Snapshot().Score)
}

func TestGovernor_OverridePinsScore(t *testing.T) {
	src := &fakeSource{verdicts: []domain.Verdict{{Target: domain.LevelNormal, Score: 1.0}}}
	pin := domain.LevelElevated
	store := &fakeStore{override: &pin}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 2, 3, 3)

	g.RunTick(context.Background())
	assert.Equal(t, 0.5, store.score, "override level 1 pins the score to 0.5 regardless of raw")
	assert.Equal(t, 0.5, g.Snapshot().Score)
}

func TestGovernor_NoTransitionSpamWhenStable(t *testing.T) {
	src := &fakeSource{verdicts: []domain.Verdict{{Target: domain.LevelNormal}}}
	store := &fakeStore{}
	sink := &fakeSink{}
	g := newTestGovernor(src, store, sink, 2, 3, 3)

	for i := 0; i < 5; i++ {
		g.RunTick(context.Background())
	}
	assert.Empty(t, sink.transitions)
	assert.Equal(t, 5, store.publishes, "every tick refreshes the TTL")
}
