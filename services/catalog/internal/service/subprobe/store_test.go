package subprobe

import (
	"sync"
	"testing"
	"time"
)

func TestStore_RecordAndSnapshot(t *testing.T) {
	s := NewStore(15 * time.Minute)
	s.Record("jimaku", Health{Status: StatusUp, LatencyMS: 120, CheckedAt: time.Now()})
	snap := s.Snapshot()
	if snap["jimaku"].Status != StatusUp || snap["jimaku"].LatencyMS != 120 {
		t.Fatalf("snapshot = %+v; want up/120", snap["jimaku"])
	}
}

func TestStore_StaleDowngradesToUnknown(t *testing.T) {
	now := time.Unix(10_000, 0)
	s := NewStore(60 * time.Second)
	s.now = func() time.Time { return now }
	s.Record("jimaku", Health{Status: StatusUp, CheckedAt: now.Add(-120 * time.Second)})
	if got := s.Snapshot()["jimaku"].Status; got != StatusUnknown {
		t.Fatalf("stale status = %q; want unknown", got)
	}
}

func TestStore_SnapshotIsCopy(t *testing.T) {
	s := NewStore(time.Minute)
	s.Record("jimaku", Health{Status: StatusUp, CheckedAt: time.Now()})
	snap := s.Snapshot()
	snap["jimaku"] = Health{Status: StatusDown}
	if s.Snapshot()["jimaku"].Status != StatusUp {
		t.Fatal("Snapshot must return a copy; mutation leaked into the store")
	}
}

func TestStore_ConcurrentRecordSnapshot(t *testing.T) {
	s := NewStore(time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); s.Record("jimaku", Health{Status: StatusUp, CheckedAt: time.Now()}) }()
		go func() { defer wg.Done(); _ = s.Snapshot() }()
	}
	wg.Wait()
}
