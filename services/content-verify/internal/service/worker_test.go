package service

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/cvmetrics"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
)

// --- fakes for the four Worker dependencies ---

type fakeShed struct{ shed bool }

func (f fakeShed) ShouldShed(int) bool { return f.shed }

// fakeLeveledShed mirrors cache.DegradationWatcher.ShouldShed's real
// level>=min semantics — fakeShed above is a static per-tick override,
// this one models the governor's actual graduated-shedding contract so
// TestTickGraduatedShedByWorkerIndex can exercise different shedMin values
// against one fixed level.
type fakeLeveledShed struct{ level int }

func (f fakeLeveledShed) ShouldShed(min int) bool { return f.level >= min }

type fakeClaimer struct {
	unit    *queue.Unit
	task    *queue.SkipTask
	release func()
	err     error
}

func (f fakeClaimer) Claim(context.Context) (*queue.Unit, *queue.SkipTask, func(), error) {
	return f.unit, f.task, f.release, f.err
}

type fakeProber struct {
	calls     int
	gotUnit   queue.Unit
	prevFails int
	verdict   domain.UnitVerdict
}

func (f *fakeProber) Probe(_ context.Context, u queue.Unit, prevFails int) domain.UnitVerdict {
	f.calls++
	f.gotUnit = u
	f.prevFails = prevFails
	return f.verdict
}

type fakeStore struct {
	getRow  *domain.ContentVerification
	getErr  error
	upserts []domain.UnitVerdict
	order   *[]string // optional: appends "persist" here — release-ordering test
}

func (f *fakeStore) Get(context.Context, string, string) (*domain.ContentVerification, error) {
	return f.getRow, f.getErr
}

func (f *fakeStore) UpsertUnit(_ context.Context, _ string, _ string, v domain.UnitVerdict) error {
	f.upserts = append(f.upserts, v)
	if f.order != nil {
		*f.order = append(*f.order, "persist")
	}
	return nil
}

type fakeSkipProber struct {
	calls     int
	gotTask   queue.SkipTask
	prevFails int
	rows      []domain.SkipTiming
}

func (f *fakeSkipProber) Probe(_ context.Context, t queue.SkipTask, prevFails int) []domain.SkipTiming {
	f.calls++
	f.gotTask = t
	f.prevFails = prevFails
	return f.rows
}

type fakeSkipStore struct {
	rows    []domain.SkipTiming
	getErr  error
	upserts []domain.SkipTiming
}

func (f *fakeSkipStore) SkipByAnime(context.Context, string) ([]domain.SkipTiming, error) {
	return f.rows, f.getErr
}

func (f *fakeSkipStore) UpsertSkip(_ context.Context, t domain.SkipTiming) error {
	f.upserts = append(f.upserts, t)
	return nil
}

// --- cases ---

func TestTickShedGateSkipsProbe(t *testing.T) {
	before := testutil.ToFloat64(cvmetrics.TicksSkippedTotal.WithLabelValues("degraded"))

	claimer := &fakeClaimer{unit: &queue.Unit{AnimeID: "a-1", Provider: "gogoanime"}}
	prober := &fakeProber{}
	store := &fakeStore{}
	w := NewWorker(time.Minute, 1, 10*time.Second, fakeShed{shed: true}, claimer, prober, store, nil, nil, 0, nil)

	w.tick(context.Background(), 1)

	if prober.calls != 0 {
		t.Fatalf("prober must not be called on a shed tick, calls=%d", prober.calls)
	}
	if len(store.upserts) != 0 {
		t.Fatalf("store must not be touched on a shed tick, upserts=%v", store.upserts)
	}
	after := testutil.ToFloat64(cvmetrics.TicksSkippedTotal.WithLabelValues("degraded"))
	if after != before+1 {
		t.Fatalf("degraded skip counter: before=%v after=%v, want +1", before, after)
	}
}

func TestTickIdleQueueSkipsProbe(t *testing.T) {
	before := testutil.ToFloat64(cvmetrics.TicksSkippedTotal.WithLabelValues("idle"))

	claimer := &fakeClaimer{unit: nil}
	prober := &fakeProber{}
	store := &fakeStore{}
	w := NewWorker(time.Minute, 1, 10*time.Second, fakeShed{shed: false}, claimer, prober, store, nil, nil, 0, nil)

	w.tick(context.Background(), 1)

	if prober.calls != 0 {
		t.Fatalf("prober must not be called on an idle claim, calls=%d", prober.calls)
	}
	after := testutil.ToFloat64(cvmetrics.TicksSkippedTotal.WithLabelValues("idle"))
	if after != before+1 {
		t.Fatalf("idle skip counter: before=%v after=%v, want +1", before, after)
	}
}

func TestTickAeSynthUnitSkipsProbe(t *testing.T) {
	unit := &queue.Unit{
		AnimeID: "a-1", Provider: "ae-firstparty", Episode: 3,
		Key: domain.UnitKey{Track: "default"},
		Synth: &domain.UnitVerdict{
			Key: domain.UnitKey{Track: "default"}, Episode: 3, Status: domain.StatusVerified,
			Audio: &domain.AudioVerdict{Lang: "en", Confidence: 1.0, Verified: true},
		},
	}
	claimer := &fakeClaimer{unit: unit}
	prober := &fakeProber{}
	store := &fakeStore{}
	w := NewWorker(time.Minute, 1, 10*time.Second, fakeShed{shed: false}, claimer, prober, store, nil, nil, 0, nil)

	w.tick(context.Background(), 1)

	if prober.calls != 0 {
		t.Fatalf("ae first-party unit must synthesize, not probe; calls=%d", prober.calls)
	}
	if len(store.upserts) != 1 {
		t.Fatalf("expected exactly 1 upsert, got %d", len(store.upserts))
	}
	v := store.upserts[0]
	if v.Status != domain.StatusVerified {
		t.Fatalf("synth status = %q, want verified", v.Status)
	}
	if v.Audio == nil || v.Audio.Lang != "en" || v.Audio.Confidence != 1.0 || !v.Audio.Verified {
		t.Fatalf("synth audio verdict wrong: %+v", v.Audio)
	}
	if v.Episode != unit.Episode || v.Key != unit.Key {
		t.Fatalf("synth verdict must carry through key/episode: %+v", v)
	}
}

func TestTickNormalUnitProbesWithPrevFailsAndPersists(t *testing.T) {
	unit := &queue.Unit{
		AnimeID: "a-1", Provider: "gogoanime",
		Key: domain.UnitKey{Server: "hd-1", Category: "sub"}, Episode: 5, Episodes: 12,
	}
	claimer := &fakeClaimer{unit: unit}
	verdict := domain.UnitVerdict{Key: unit.Key, Episode: 5, Status: domain.StatusVerified}
	prober := &fakeProber{verdict: verdict}

	// Store already has a prior verdict for this exact unit key with 2 fails
	// recorded — tick must read it BEFORE probing and pass it through.
	prevRow := &domain.ContentVerification{
		AnimeID: "a-1", Provider: "gogoanime",
		Units: domain.UnitList{{Key: unit.Key, Fails: 2, Status: domain.StatusUnreachable}},
	}
	store := &fakeStore{getRow: prevRow}
	w := NewWorker(time.Minute, 1, 10*time.Second, fakeShed{shed: false}, claimer, prober, store, nil, nil, 0, nil)

	w.tick(context.Background(), 1)

	if prober.calls != 1 {
		t.Fatalf("prober must be called exactly once, calls=%d", prober.calls)
	}
	if prober.prevFails != 2 {
		t.Fatalf("prevFails passed to prober = %d, want 2 (read from store)", prober.prevFails)
	}
	if prober.gotUnit.Key != unit.Key || prober.gotUnit.AnimeID != unit.AnimeID {
		t.Fatalf("prober called with wrong unit: %+v", prober.gotUnit)
	}
	if len(store.upserts) != 1 || store.upserts[0].Status != domain.StatusVerified {
		t.Fatalf("probed verdict not persisted: %+v", store.upserts)
	}
	// Episodes-ready is enumeration truth stamped at persist time — the prober
	// never sees or sets it.
	if store.upserts[0].Episodes != 12 {
		t.Fatalf("persisted episodes = %d, want 12 (from unit)", store.upserts[0].Episodes)
	}
}

// TestTickSkipTaskProbesWithPrevFailsAndPersistsAllRows covers a pair-
// bootstrap skip task: prevFails must be the max Fails recorded across
// existing rows matching the task's PRIMARY unit only (provider+team+
// episode) — a row for a different episode of the same family must not
// leak in — both rows the skip prober returns must be persisted via
// UpsertSkip, the verify-lane prober/store must be untouched, and
// SkipProbesTotal must be incremented keyed by the first returned row's
// OpStatus.
func TestTickSkipTaskProbesWithPrevFailsAndPersistsAllRows(t *testing.T) {
	unit := queue.SkipUnit{AnimeID: "a-1", Provider: "kodik", Team: "610", Episode: 1}
	pair := queue.SkipUnit{AnimeID: "a-1", Provider: "kodik", Team: "610", Episode: 2}
	task := &queue.SkipTask{Unit: unit, Pair: &pair}
	claimer := &fakeClaimer{unit: nil, task: task}
	prober := &fakeProber{}
	store := &fakeStore{}

	skipStore := &fakeSkipStore{rows: []domain.SkipTiming{
		{AnimeID: "a-1", Provider: "kodik", Team: "610", Episode: 1, Fails: 3,
			OpStatus: domain.SkipUnreachable, EdStatus: domain.SkipUnreachable},
		// Different episode of the same family — must not leak into prevFails.
		{AnimeID: "a-1", Provider: "kodik", Team: "610", Episode: 7, Fails: 9,
			OpStatus: domain.SkipUnreachable, EdStatus: domain.SkipUnreachable},
	}}
	skipProber := &fakeSkipProber{rows: []domain.SkipTiming{
		{AnimeID: "a-1", Provider: "kodik", Team: "610", Episode: 1, OpStatus: domain.SkipDetected},
		{AnimeID: "a-1", Provider: "kodik", Team: "610", Episode: 2, OpStatus: domain.SkipDetected},
	}}
	w := NewWorker(time.Minute, 1, 10*time.Second, fakeShed{shed: false}, claimer, prober, store,
		skipProber, skipStore, 20*time.Second, nil)

	before := testutil.ToFloat64(cvmetrics.SkipProbesTotal.WithLabelValues("kodik", domain.SkipDetected))

	w.tick(context.Background(), 1)

	if prober.calls != 0 {
		t.Fatalf("verify prober must NOT be called for a skip claim, calls=%d", prober.calls)
	}
	if len(store.upserts) != 0 {
		t.Fatalf("verify-lane store must not be touched by a skip claim, upserts=%v", store.upserts)
	}
	if skipProber.calls != 1 {
		t.Fatalf("skip prober must be called exactly once, calls=%d", skipProber.calls)
	}
	if skipProber.prevFails != 3 {
		t.Fatalf("prevFails passed to skip prober = %d, want 3 (max Fails from the task's own unit row)", skipProber.prevFails)
	}
	if skipProber.gotTask.Unit != unit || skipProber.gotTask.Pair == nil || *skipProber.gotTask.Pair != pair {
		t.Fatalf("skip prober called with wrong task: %+v", skipProber.gotTask)
	}
	if len(skipStore.upserts) != 2 {
		t.Fatalf("expected both returned rows persisted via UpsertSkip, got %d", len(skipStore.upserts))
	}
	after := testutil.ToFloat64(cvmetrics.SkipProbesTotal.WithLabelValues("kodik", domain.SkipDetected))
	if after != before+1 {
		t.Fatalf("SkipProbesTotal{kodik,detected}: before=%v after=%v, want +1", before, after)
	}
}

// TestTickSkipTaskLocateSingleRow covers a locate skip task (no Pair): the
// single returned row must still be persisted and prevFails defaults to 0
// when the store has no existing row for the unit.
func TestTickSkipTaskLocateSingleRow(t *testing.T) {
	unit := queue.SkipUnit{AnimeID: "a-1", Provider: "gogoanime", EpisodeID: "ep-9", Episode: 9}
	task := &queue.SkipTask{Unit: unit}
	claimer := &fakeClaimer{unit: nil, task: task}
	prober := &fakeProber{}
	store := &fakeStore{}
	skipStore := &fakeSkipStore{}
	skipProber := &fakeSkipProber{rows: []domain.SkipTiming{
		{AnimeID: "a-1", Provider: "gogoanime", Episode: 9, OpStatus: domain.SkipNoMatch, EdStatus: domain.SkipNoMatch},
	}}
	w := NewWorker(time.Minute, 1, 10*time.Second, fakeShed{shed: false}, claimer, prober, store,
		skipProber, skipStore, 20*time.Second, nil)

	w.tick(context.Background(), 1)

	if prober.calls != 0 {
		t.Fatalf("verify prober must NOT be called for a skip claim, calls=%d", prober.calls)
	}
	if skipProber.calls != 1 || skipProber.prevFails != 0 {
		t.Fatalf("skip prober calls=%d prevFails=%d, want 1/0 (no existing row)", skipProber.calls, skipProber.prevFails)
	}
	if skipProber.gotTask.Pair != nil {
		t.Fatalf("locate task must not carry a pair: %+v", skipProber.gotTask)
	}
	if len(skipStore.upserts) != 1 || skipStore.upserts[0].Episode != 9 {
		t.Fatalf("expected the single returned row persisted via UpsertSkip, got %+v", skipStore.upserts)
	}
}

// TestTickGraduatedShedByWorkerIndex covers graduated governor shedding: at
// governor level 1, the shedMin=1 worker (the "extra" one — worker index 1
// of a 2-worker pool, per runLoop's shedMin=workers-i) must shed, while the
// shedMin=2 worker (index 0, the floor worker) must keep claiming. Level 1
// thus halves probing instead of stopping it outright.
func TestTickGraduatedShedByWorkerIndex(t *testing.T) {
	shed := fakeLeveledShed{level: 1}
	unit := &queue.Unit{
		AnimeID: "a-1", Provider: "gogoanime",
		Key: domain.UnitKey{Server: "hd-1", Category: "sub"},
	}

	prober1 := &fakeProber{}
	w1 := NewWorker(time.Minute, 2, 10*time.Second, shed, &fakeClaimer{unit: unit}, prober1, &fakeStore{}, nil, nil, 0, nil)
	w1.tick(context.Background(), 1)
	if prober1.calls != 0 {
		t.Fatalf("shedMin=1 worker must shed at governor level 1, calls=%d", prober1.calls)
	}

	prober2 := &fakeProber{}
	w2 := NewWorker(time.Minute, 2, 10*time.Second, shed, &fakeClaimer{unit: unit}, prober2, &fakeStore{}, nil, nil, 0, nil)
	w2.tick(context.Background(), 2)
	if prober2.calls != 1 {
		t.Fatalf("shedMin=2 worker must keep claiming at governor level 1 (graduated floor), calls=%d", prober2.calls)
	}
}

// TestTickReleaseRunsAfterPersist covers the claim-lease release ordering
// contract: release must fire only AFTER the probed verdict is persisted,
// so a second worker can't re-claim the unit before its row is written.
func TestTickReleaseRunsAfterPersist(t *testing.T) {
	var order []string

	unit := &queue.Unit{
		AnimeID: "a-1", Provider: "gogoanime",
		Key: domain.UnitKey{Server: "hd-1", Category: "sub"}, Episode: 5,
	}
	verdict := domain.UnitVerdict{Key: unit.Key, Episode: 5, Status: domain.StatusVerified}
	prober := &fakeProber{verdict: verdict}
	store := &fakeStore{order: &order}
	claimer := &fakeClaimer{unit: unit, release: func() { order = append(order, "release") }}

	w := NewWorker(time.Minute, 1, 10*time.Second, fakeShed{shed: false}, claimer, prober, store, nil, nil, 0, nil)
	w.tick(context.Background(), 1)

	if len(order) != 2 || order[0] != "persist" || order[1] != "release" {
		t.Fatalf("expected [persist release] order, got %v", order)
	}
}
