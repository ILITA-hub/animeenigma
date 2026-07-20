package signals

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testSignals(t *testing.T) (*Signals, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	s := New(rdb)
	s.now = func() time.Time { return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC) }
	return s, mr
}

func TestVisitDedupAndWindow(t *testing.T) {
	s, _ := testSignals(t)
	ctx := context.Background()
	_ = s.RecordVisit(ctx, "a-1", "u:alice")
	_ = s.RecordVisit(ctx, "a-1", "u:alice") // same day dedup
	_ = s.RecordVisit(ctx, "a-1", "u:bob")
	if n := s.UniqueVisitors(ctx, "a-1"); n != 2 {
		t.Fatalf("visitors = %d, want 2", n)
	}
	// Same visitor on another day still counts ONCE across the window.
	s.now = func() time.Time { return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC) }
	_ = s.RecordVisit(ctx, "a-1", "u:alice")
	if n := s.UniqueVisitors(ctx, "a-1"); n != 2 {
		t.Fatalf("cross-day visitors = %d, want 2 (union, not sum)", n)
	}
	visited := s.VisitedAnime(ctx)
	if len(visited) != 1 || visited[0] != "a-1" {
		t.Fatalf("visited = %v", visited)
	}
}

func TestCooldown(t *testing.T) {
	s, mr := testSignals(t)
	ctx := context.Background()
	if s.InCooldown(ctx, "a-1") {
		t.Fatal("fresh anime must not be cooling")
	}
	s.SetCooldown(ctx, "a-1", time.Hour)
	if !s.InCooldown(ctx, "a-1") {
		t.Fatal("cooldown not set")
	}
	mr.FastForward(2 * time.Hour)
	if s.InCooldown(ctx, "a-1") {
		t.Fatal("cooldown must expire")
	}
}

func TestIdleCursorAdvanceAndWrap(t *testing.T) {
	s, _ := testSignals(t)
	ctx := context.Background()

	if got := s.IdleCursor(ctx); got != 0 {
		t.Fatalf("cold cursor = %d, want 0", got)
	}
	if got := s.AdvanceIdleCursor(ctx, 100, 250); got != 100 {
		t.Fatalf("advance = %d, want 100", got)
	}
	if got := s.AdvanceIdleCursor(ctx, 100, 250); got != 200 {
		t.Fatalf("advance = %d, want 200", got)
	}
	// 200+100 = 300 wraps past total 250 → 300 % 250 = 50.
	if got := s.AdvanceIdleCursor(ctx, 100, 250); got != 50 {
		t.Fatalf("wrap = %d, want 50", got)
	}
	if got := s.IdleCursor(ctx); got != 50 {
		t.Fatalf("read back = %d, want 50", got)
	}
	// total 0 (empty tail) → cursor pinned at 0, no divide-by-zero.
	if got := s.AdvanceIdleCursor(ctx, 100, 0); got != 0 {
		t.Fatalf("total 0 = %d, want 0", got)
	}
}

// TestFailOpenOnRedisDown pins the "reads fail open" contract: when Redis is
// unreachable, every read must degrade to a zero signal rather than error out
// or panic — the queue keeps working through a Redis blip.
func TestFailOpenOnRedisDown(t *testing.T) {
	s, mr := testSignals(t)
	ctx := context.Background()
	mr.Close()

	if n := s.UniqueVisitors(ctx, "a-1"); n != 0 {
		t.Fatalf("UniqueVisitors on down Redis = %d, want 0", n)
	}
	if v := s.VisitedAnime(ctx); v != nil {
		t.Fatalf("VisitedAnime on down Redis = %v, want nil", v)
	}
	if s.InCooldown(ctx, "a-1") {
		t.Fatal("InCooldown on down Redis = true, want false")
	}
	s.SetCooldown(ctx, "a-1", time.Hour) // must not panic
}
