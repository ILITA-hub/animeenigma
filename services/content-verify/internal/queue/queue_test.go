package queue

import (
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

func TestBackoff(t *testing.T) {
	if Backoff(1) != 6*time.Hour || Backoff(2) != 12*time.Hour {
		t.Fatal("backoff base wrong")
	}
	if Backoff(10) != 168*time.Hour {
		t.Fatalf("backoff cap: %s", Backoff(10))
	}
}

func TestUnitDue(t *testing.T) {
	now := time.Now()
	u := Unit{Episode: 28, Key: domain.UnitKey{Server: "hd-1", Category: "sub"}}
	if !UnitDue(u, nil, now, 720*time.Hour) {
		t.Fatal("never-probed must be due")
	}
	fresh := &domain.UnitVerdict{Episode: 28, Status: domain.StatusVerified, ProbedAt: now.Add(-time.Hour)}
	if UnitDue(u, fresh, now, 720*time.Hour) {
		t.Fatal("fresh verified must NOT be due")
	}
	oldEp := &domain.UnitVerdict{Episode: 27, Status: domain.StatusVerified, ProbedAt: now.Add(-time.Hour)}
	if !UnitDue(u, oldEp, now, 720*time.Hour) {
		t.Fatal("new episode must re-probe")
	}
	stale := &domain.UnitVerdict{Episode: 28, Status: domain.StatusVerified, ProbedAt: now.Add(-721 * time.Hour)}
	if !UnitDue(u, stale, now, 720*time.Hour) {
		t.Fatal("stale must re-probe")
	}
	failing := &domain.UnitVerdict{Episode: 28, Status: domain.StatusUnreachable, Fails: 1, ProbedAt: now.Add(-time.Hour)}
	if UnitDue(u, failing, now, 720*time.Hour) {
		t.Fatal("unreachable within backoff must wait")
	}
	failing.ProbedAt = now.Add(-7 * time.Hour)
	if !UnitDue(u, failing, now, 720*time.Hour) {
		t.Fatal("unreachable past backoff must retry")
	}
}

func TestPendingUnitsKeyedByProviderAndKey(t *testing.T) {
	now := time.Now()
	units := []Unit{
		{AnimeID: "a1", Provider: "gogoanime", Episode: 5, Key: domain.UnitKey{Server: "hd-1", Category: "sub"}},
		{AnimeID: "a1", Provider: "kodik", Episode: 5, Key: domain.UnitKey{Server: "hd-1", Category: "sub"}}, // same Key.String() as above, different provider
	}
	rows := []domain.ContentVerification{
		{AnimeID: "a1", Provider: "gogoanime", Units: domain.UnitList{
			{Key: domain.UnitKey{Server: "hd-1", Category: "sub"}, Episode: 5, Status: domain.StatusVerified, ProbedAt: now.Add(-time.Hour)},
		}},
	}
	pending := PendingUnits(units, rows, now, 720*time.Hour)
	if len(pending) != 1 || pending[0].Provider != "kodik" {
		t.Fatalf("pending should only contain the never-probed kodik unit (provider-scoped key): %+v", pending)
	}
}
