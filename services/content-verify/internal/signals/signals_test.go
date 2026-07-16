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
