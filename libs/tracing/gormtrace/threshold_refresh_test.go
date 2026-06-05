package gormtrace

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeHashReader is an injectable HGetAll source backed by a swappable map, so
// the refresher's tick can be driven without a live Redis. Handwritten — no
// testify/mock (matches readgate_test.go style).
type fakeHashReader struct {
	mu   sync.Mutex
	hash map[string]string
	err  error
}

func (f *fakeHashReader) HGetAll(_ context.Context, _ string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	// Return a copy so the refresher can't mutate our backing map.
	out := make(map[string]string, len(f.hash))
	for k, v := range f.hash {
		out[k] = v
	}
	return out, nil
}

func TestThresholdRefresh(t *testing.T) {
	t.Run("one tick feeds the gate from the hash", func(t *testing.T) {
		gate := NewReadGate(50)
		reader := &fakeHashReader{hash: map[string]string{"catalog.X|animes": "80"}}
		rf := NewThresholdRefresher(reader, gate, time.Hour)

		// refreshOnce is the synchronous tick body — assert it directly so the
		// test is deterministic (no ticker timing).
		rf.refreshOnce(context.Background())

		if !gate.ShouldRecord("catalog.X", "animes", 90) {
			t.Fatalf("90ms > p95=80 should record after refresh")
		}
		if gate.ShouldRecord("catalog.X", "animes", 70) {
			t.Fatalf("70ms < p95=80 should NOT record after refresh")
		}
	})

	t.Run("empty hash leaves the static default", func(t *testing.T) {
		gate := NewReadGate(50)
		reader := &fakeHashReader{hash: map[string]string{}}
		rf := NewThresholdRefresher(reader, gate, time.Hour)
		rf.refreshOnce(context.Background())
		// No key present -> static default of 50ms applies.
		if !gate.ShouldRecord("any.Op", "tbl", 60) {
			t.Fatalf("60ms > static 50ms should record")
		}
		if gate.ShouldRecord("any.Op", "tbl", 40) {
			t.Fatalf("40ms < static 50ms should NOT record")
		}
	})

	t.Run("read error does not poison the snapshot or panic", func(t *testing.T) {
		gate := NewReadGate(50)
		gate.SetSnapshot(map[string]float64{"keep|me": 200})
		reader := &fakeHashReader{err: errors.New("redis down")}
		rf := NewThresholdRefresher(reader, gate, time.Hour)
		// Must not panic and must not overwrite the existing snapshot.
		rf.refreshOnce(context.Background())
		if gate.ShouldRecord("keep", "me", 150) {
			t.Fatalf("prior snapshot (p95=200) must survive a read error; 150<200")
		}
		if !gate.ShouldRecord("keep", "me", 250) {
			t.Fatalf("prior snapshot (p95=200) must survive a read error; 250>200")
		}
	})

	t.Run("malformed/oversized fields are skipped, valid ones kept", func(t *testing.T) {
		gate := NewReadGate(50)
		reader := &fakeHashReader{hash: map[string]string{
			"good|key":     "80",
			"bad|key":      "not-a-number",
			"negative|key": "-5",
			"oversized|k":  strings.Repeat("9", 400), // absurd value -> skip
			"":             "10",                     // empty field -> skip
		}}
		rf := NewThresholdRefresher(reader, gate, time.Hour)
		rf.refreshOnce(context.Background())

		// good|key parsed.
		if !gate.ShouldRecord("good", "key", 90) {
			t.Fatalf("good|key p95=80 should apply")
		}
		// bad|key skipped -> falls back to static default 50.
		if gate.ShouldRecord("bad", "key", 40) {
			t.Fatalf("bad|key should fall back to static default (40<50)")
		}
		if !gate.ShouldRecord("bad", "key", 60) {
			t.Fatalf("bad|key should fall back to static default (60>50)")
		}
	})

	t.Run("Start then Stop halts the ticker cleanly", func(t *testing.T) {
		gate := NewReadGate(50)
		reader := &fakeHashReader{hash: map[string]string{"a|b": "10"}}
		rf := NewThresholdRefresher(reader, gate, 5*time.Millisecond)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rf.Start(ctx)
		time.Sleep(20 * time.Millisecond)
		rf.Stop()
		// After Stop, a second Stop must be safe (idempotent, no panic).
		rf.Stop()
		// The gate should reflect at least one tick.
		if !gate.ShouldRecord("a", "b", 20) {
			t.Fatalf("ticker should have applied a|b p95=10 (20>10)")
		}
	})
}
