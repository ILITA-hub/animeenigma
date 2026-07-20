// Worker handling of the deferred sentinel (owner directive 2026-07-20): a
// StatusDeferred verdict must become an engine deferral and NEVER a store
// row (no Fails++, no verdict churn).
package service

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
)

// deferringClaimer is fakeClaimer + the ProviderDeferrer extension.
type deferringClaimer struct {
	fakeClaimer
	gotAnime    string
	gotProvider string
	gotRetry    time.Duration
	deferCalls  int
}

func (d *deferringClaimer) Defer(animeID, provider string, retryAfter time.Duration) {
	d.deferCalls++
	d.gotAnime, d.gotProvider, d.gotRetry = animeID, provider, retryAfter
}

func TestTickDeferredVerdictDefersInsteadOfPersisting(t *testing.T) {
	claimer := &deferringClaimer{fakeClaimer: fakeClaimer{
		unit: &queue.Unit{AnimeID: "a-1", Provider: "miruro"},
	}}
	prober := &fakeProber{verdict: domain.UnitVerdict{
		Status: domain.StatusDeferred, RetryAfter: 42 * time.Minute,
	}}
	store := &fakeStore{}
	w := NewWorker(time.Minute, 1, 10*time.Second, fakeShed{shed: false}, claimer, prober, store, nil, nil, 0, nil)

	w.tick(context.Background(), 1)

	if len(store.upserts) != 0 {
		t.Fatalf("deferred verdict must not be persisted; upserts=%v", store.upserts)
	}
	if claimer.deferCalls != 1 {
		t.Fatalf("Defer calls = %d; want 1", claimer.deferCalls)
	}
	if claimer.gotAnime != "a-1" || claimer.gotProvider != "miruro" || claimer.gotRetry != 42*time.Minute {
		t.Errorf("Defer(%q, %q, %s); want (a-1, miruro, 42m0s)",
			claimer.gotAnime, claimer.gotProvider, claimer.gotRetry)
	}
}

// A claimer WITHOUT the deferrer extension (older fakes, alternative
// implementations) must not panic — the verdict is just dropped.
func TestTickDeferredVerdictWithPlainClaimer(t *testing.T) {
	claimer := &fakeClaimer{unit: &queue.Unit{AnimeID: "a-1", Provider: "miruro"}}
	prober := &fakeProber{verdict: domain.UnitVerdict{Status: domain.StatusDeferred, RetryAfter: time.Hour}}
	store := &fakeStore{}
	w := NewWorker(time.Minute, 1, 10*time.Second, fakeShed{shed: false}, claimer, prober, store, nil, nil, 0, nil)

	w.tick(context.Background(), 1)

	if len(store.upserts) != 0 {
		t.Fatalf("deferred verdict must not be persisted; upserts=%v", store.upserts)
	}
}
