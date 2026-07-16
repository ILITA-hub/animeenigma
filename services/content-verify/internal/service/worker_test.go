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

type fakeClaimer struct {
	unit    *queue.Unit
	ongoing bool
	err     error
}

func (f fakeClaimer) Claim(context.Context) (*queue.Unit, bool, error) {
	return f.unit, f.ongoing, f.err
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
}

func (f *fakeStore) Get(context.Context, string, string) (*domain.ContentVerification, error) {
	return f.getRow, f.getErr
}

func (f *fakeStore) UpsertUnit(_ context.Context, _ string, _ string, v domain.UnitVerdict) error {
	f.upserts = append(f.upserts, v)
	return nil
}

// --- cases ---

func TestTickShedGateSkipsProbe(t *testing.T) {
	before := testutil.ToFloat64(cvmetrics.TicksSkippedTotal.WithLabelValues("degraded"))

	claimer := &fakeClaimer{unit: &queue.Unit{AnimeID: "a-1", Provider: "gogoanime"}}
	prober := &fakeProber{}
	store := &fakeStore{}
	w := NewWorker(time.Minute, 10*time.Second, fakeShed{shed: true}, claimer, prober, store, nil)

	w.tick(context.Background())

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
	w := NewWorker(time.Minute, 10*time.Second, fakeShed{shed: false}, claimer, prober, store, nil)

	w.tick(context.Background())

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
		AnimeID: "a-1", Provider: "ae-firstparty", AeLang: "en", Episode: 3,
		Key: domain.UnitKey{Track: "default"},
	}
	claimer := &fakeClaimer{unit: unit}
	prober := &fakeProber{}
	store := &fakeStore{}
	w := NewWorker(time.Minute, 10*time.Second, fakeShed{shed: false}, claimer, prober, store, nil)

	w.tick(context.Background())

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
		Key: domain.UnitKey{Server: "hd-1", Category: "sub"}, Episode: 5,
	}
	claimer := &fakeClaimer{unit: unit, ongoing: true}
	verdict := domain.UnitVerdict{Key: unit.Key, Episode: 5, Status: domain.StatusVerified}
	prober := &fakeProber{verdict: verdict}

	// Store already has a prior verdict for this exact unit key with 2 fails
	// recorded — tick must read it BEFORE probing and pass it through.
	prevRow := &domain.ContentVerification{
		AnimeID: "a-1", Provider: "gogoanime",
		Units: domain.UnitList{{Key: unit.Key, Fails: 2, Status: domain.StatusUnreachable}},
	}
	store := &fakeStore{getRow: prevRow}
	w := NewWorker(time.Minute, 10*time.Second, fakeShed{shed: false}, claimer, prober, store, nil)

	w.tick(context.Background())

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
}
