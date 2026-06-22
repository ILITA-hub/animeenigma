package subprobe

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
)

type fakePinger struct {
	lat time.Duration
	err error
}

func (f fakePinger) Ping(ctx context.Context) (time.Duration, error) { return f.lat, f.err }

func TestRunOnce_ClassifiesAndRecords(t *testing.T) {
	store := NewStore(time.Minute)
	p := New(store, map[string]Pinger{
		"jimaku":        fakePinger{lat: 100 * time.Millisecond},                 // fast → up
		"opensubtitles": fakePinger{lat: 5 * time.Second},                        // slow → degraded
	}, 2*time.Second, 8*time.Second, nil)
	p.RunOnce(context.Background())

	snap := store.Snapshot()
	if snap["jimaku"].Status != StatusUp {
		t.Errorf("jimaku = %q; want up", snap["jimaku"].Status)
	}
	if snap["opensubtitles"].Status != StatusDegraded {
		t.Errorf("opensubtitles = %q; want degraded", snap["opensubtitles"].Status)
	}
}

func TestRunOnce_TransportErrorIsDown(t *testing.T) {
	store := NewStore(time.Minute)
	p := New(store, map[string]Pinger{
		"jimaku": fakePinger{err: context.DeadlineExceeded},
	}, 2*time.Second, 8*time.Second, nil)
	p.RunOnce(context.Background())
	if store.Snapshot()["jimaku"].Status != StatusDown {
		t.Fatalf("transport error: want down, got %q", store.Snapshot()["jimaku"].Status)
	}
}

func TestRunOnce_RateLimitedIsDegraded(t *testing.T) {
	store := NewStore(time.Minute)
	p := New(store, map[string]Pinger{
		"opensubtitles": fakePinger{lat: 50 * time.Millisecond, err: opensubtitles.ErrRateLimited},
	}, 2*time.Second, 8*time.Second, nil)
	p.RunOnce(context.Background())
	if store.Snapshot()["opensubtitles"].Status != StatusDegraded {
		t.Fatalf("rate-limited: want degraded, got %q", store.Snapshot()["opensubtitles"].Status)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		lat  time.Duration
		err  error
		want Status
	}{
		{"fast ok", 100 * time.Millisecond, nil, StatusUp},
		{"slow ok", 3 * time.Second, nil, StatusDegraded},
		{"rate limited", 0, opensubtitles.ErrRateLimited, StatusDegraded},
		{"unauthorized", 0, opensubtitles.ErrUnauthorized, StatusDown},
		{"deadline", 0, context.DeadlineExceeded, StatusDown},
	}
	for _, c := range cases {
		if got := classify(c.lat, c.err, 2*time.Second); got != c.want {
			t.Errorf("%s: classify = %q; want %q", c.name, got, c.want)
		}
	}
}
