package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

// memLogRedis is an in-memory logRedis for tests. It is mutex-guarded so it
// matches the concurrency contract of the production *redis.Client (which is
// goroutine-safe), letting the concurrent regression test isolate the LogBuffer
// channel-close race rather than tripping on an unsynchronised fake.
type memLogRedis struct {
	mu   sync.Mutex
	data map[string][]string
}

func newMemLogRedis() *memLogRedis {
	return &memLogRedis{data: make(map[string][]string)}
}

func (m *memLogRedis) appendLog(_ context.Context, key, val string, cap int, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = append(m.data[key], val)
	if len(m.data[key]) > cap {
		m.data[key] = m.data[key][len(m.data[key])-cap:]
	}
	return nil
}

func (m *memLogRedis) rangeLogs(_ context.Context, key string, n int) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.data[key]
	if len(all) == 0 {
		return nil, nil
	}
	if n >= len(all) {
		// Return a copy so callers can't observe later mutations under the race detector.
		return append([]string(nil), all...), nil
	}
	return append([]string(nil), all[len(all)-n:]...), nil
}

func TestLogBuffer_AppendTail(t *testing.T) {
	t.Parallel()
	buf := NewLogBuffer(newMemLogRedis(), LogBufferConfig{Cap: 5})
	ctx := context.Background()

	// Append 10 lines — only last 5 should survive in ring.
	for i := 0; i < 10; i++ {
		if err := buf.Append(ctx, "job-1", LogLine{
			Source:  "orchestrator",
			Level:   "info",
			Msg:     "line",
			Segment: i,
			TS:      time.Now(),
		}); err != nil {
			t.Fatalf("Append line %d: %v", i, err)
		}
	}

	lines := buf.Tail(ctx, "job-1", 5)
	if len(lines) != 5 {
		t.Fatalf("Tail(5): got %d lines; want 5", len(lines))
	}
	// Last appended segment should be 9, first visible should be 5.
	if lines[0].Segment != 5 {
		t.Errorf("Tail[0].Segment = %d; want 5 (oldest in ring)", lines[0].Segment)
	}
	if lines[4].Segment != 9 {
		t.Errorf("Tail[4].Segment = %d; want 9 (newest)", lines[4].Segment)
	}
}

func TestLogBuffer_Subscribe(t *testing.T) {
	t.Parallel()
	buf := NewLogBuffer(newMemLogRedis(), LogBufferConfig{Cap: 50})
	ctx := context.Background()

	ch, cancel := buf.Subscribe("job-2")
	defer cancel()

	line := LogLine{Source: "worker", Level: "info", Msg: "hello", TS: time.Now()}
	if err := buf.Append(ctx, "job-2", line); err != nil {
		t.Fatalf("Append: %v", err)
	}

	select {
	case got := <-ch:
		if got.Msg != "hello" {
			t.Errorf("Subscribe: Msg = %q; want hello", got.Msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Subscribe: did not receive line within 100ms")
	}
}

// TestLogBuffer_ConcurrentAppendCancel is a regression guard for the
// send-on-closed-channel panic: the SSE handler's deferred cancel() can close a
// subscriber channel while a worker/orchestrator Append is mid-fan-out. Append
// must hold the read lock across the non-blocking sends so cancel (write lock)
// can never close a channel between the lock release and the send. Run with -race.
func TestLogBuffer_ConcurrentAppendCancel(t *testing.T) {
	t.Parallel()
	buf := NewLogBuffer(newMemLogRedis(), LogBufferConfig{Cap: 100})
	ctx := context.Background()
	var wg sync.WaitGroup

	for iter := 0; iter < 100; iter++ {
		ch, cancel := buf.Subscribe("jobX")
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = buf.Append(ctx, "jobX", LogLine{Msg: "x", TS: time.Now()})
			}
		}()
		go func() {
			defer wg.Done()
			cancel()
		}()
		go func() {
			for range ch { //nolint:revive // drain so the buffer isn't always full
			}
		}()
	}
	wg.Wait()
}

func TestLogBuffer_Flush_NoOp(t *testing.T) {
	t.Parallel()
	buf := NewLogBuffer(newMemLogRedis(), LogBufferConfig{Cap: 50})
	ctx := context.Background()
	// No flusher wired — Flush must succeed (no-op).
	if err := buf.Flush(ctx, "job-3"); err != nil {
		t.Fatalf("Flush (no flusher): expected nil error; got %v", err)
	}
}
