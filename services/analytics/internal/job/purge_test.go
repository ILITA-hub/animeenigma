package job

import (
	"context"
	"testing"
	"time"
)

type fakePurger struct {
	cutoff time.Time
	called int
}

func (f *fakePurger) PurgeOlderThan(_ context.Context, cutoff time.Time) (int64, error) {
	f.called++
	f.cutoff = cutoff
	return 3, nil
}

func TestPurgeJob_RunOnce(t *testing.T) {
	p := &fakePurger{}
	j := NewPurgeJob(p, 90, nil)
	n, err := j.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 purged, got %d", n)
	}
	// cutoff must be ~90 days ago.
	wantApprox := time.Now().Add(-90 * 24 * time.Hour)
	if diff := p.cutoff.Sub(wantApprox); diff > time.Minute || diff < -time.Minute {
		t.Fatalf("cutoff off by %v", diff)
	}
}
