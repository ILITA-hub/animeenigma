// Package signals stores the dynamic-priority inputs in Redis: per-day
// unique-visitor sets (+15 each), a visited-anime index for queue
// membership, and per-anime claim cooldowns. All reads fail open (errors →
// zero signal) — the queue must keep working through a Redis blip.
package signals

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	dayFormat  = "20060102"
	windowDays = 7
	signalTTL  = 8 * 24 * time.Hour
)

type Signals struct {
	rdb *redis.Client
	now func() time.Time
}

func New(rdb *redis.Client) *Signals { return &Signals{rdb: rdb, now: time.Now} }

func visitKey(animeID, day string) string { return fmt.Sprintf("cv:visit:%s:%s", animeID, day) }
func visitedKey(day string) string        { return "cv:visited:" + day }
func cooldownKey(animeID string) string   { return "cv:cooldown:" + animeID }

func (s *Signals) dayKeys(build func(day string) string) []string {
	keys := make([]string, 0, windowDays)
	for i := 0; i < windowDays; i++ {
		keys = append(keys, build(s.now().UTC().AddDate(0, 0, -i).Format(dayFormat)))
	}
	return keys
}

func (s *Signals) RecordVisit(ctx context.Context, animeID, visitor string) error {
	day := s.now().UTC().Format(dayFormat)
	pipe := s.rdb.Pipeline()
	pipe.SAdd(ctx, visitKey(animeID, day), visitor)
	pipe.Expire(ctx, visitKey(animeID, day), signalTTL)
	pipe.SAdd(ctx, visitedKey(day), animeID)
	pipe.Expire(ctx, visitedKey(day), signalTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// UniqueVisitors counts DISTINCT visitors across the 7-day window (union,
// not sum — a daily returnee is one person, not seven).
func (s *Signals) UniqueVisitors(ctx context.Context, animeID string) int {
	members, err := s.rdb.SUnion(ctx, s.dayKeys(func(d string) string { return visitKey(animeID, d) })...).Result()
	if err != nil {
		return 0
	}
	return len(members)
}

func (s *Signals) VisitedAnime(ctx context.Context) []string {
	members, err := s.rdb.SUnion(ctx, s.dayKeys(visitedKey)...).Result()
	if err != nil {
		return nil
	}
	return members
}

func (s *Signals) InCooldown(ctx context.Context, animeID string) bool {
	n, err := s.rdb.Exists(ctx, cooldownKey(animeID)).Result()
	return err == nil && n > 0
}

func (s *Signals) SetCooldown(ctx context.Context, animeID string, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	_ = s.rdb.Set(ctx, cooldownKey(animeID), "1", ttl).Err()
}
