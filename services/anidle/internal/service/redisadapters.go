package service

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisTokenStore implements tokenStore over go-redis (endless secrets, TTL).
type RedisTokenStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisTokenStore(client *redis.Client, ttl time.Duration) *RedisTokenStore {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &RedisTokenStore{client: client, ttl: ttl}
}

func (s *RedisTokenStore) PutToken(ctx context.Context, token, animeID string) error {
	return s.client.Set(ctx, "anidle:endless:"+token, animeID, s.ttl).Err()
}

func (s *RedisTokenStore) GetToken(ctx context.Context, token string) (string, bool, error) {
	v, err := s.client.Get(ctx, "anidle:endless:"+token).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// RedisZSet implements zsetStore over go-redis.
type RedisZSet struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisZSet(client *redis.Client, ttl time.Duration) *RedisZSet {
	if ttl <= 0 {
		ttl = 48 * time.Hour
	}
	return &RedisZSet{client: client, ttl: ttl}
}

func (s *RedisZSet) ZAdd(ctx context.Context, key, member string, score float64) error {
	if err := s.client.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err(); err != nil {
		return err
	}
	return s.client.Expire(ctx, key, s.ttl).Err()
}

func (s *RedisZSet) ZRangeAsc(ctx context.Context, key string, n int) ([]ZEntry, error) {
	zs, err := s.client.ZRangeWithScores(ctx, key, 0, int64(n-1)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]ZEntry, 0, len(zs))
	for _, z := range zs {
		member, _ := z.Member.(string)
		out = append(out, ZEntry{Member: member, Score: z.Score})
	}
	return out, nil
}
