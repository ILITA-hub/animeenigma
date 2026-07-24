package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/redis/go-redis/v9"
)

type Cache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fn func() (interface{}, error)) error
	Invalidate(ctx context.Context, pattern string) error
	// SetNX atomically sets key to value with the given TTL only when the
	// key does not exist. Returns acquired=true if the key was set, false
	// if it already existed. Used as a distributed lock primitive (e.g.
	// the recs:debounce:{user_id} debounce lock from REC-INFRA-02).
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
	// GetDel atomically reads key into dest and deletes it in a single
	// round trip, so two concurrent callers can never both observe the
	// value — the second sees ErrNotFound. Used for one-time tokens (e.g.
	// the cert-login handshake token) where Get-then-Delete would race.
	GetDel(ctx context.Context, key string, dest interface{}) error
}

type Config struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Password string `json:"password" yaml:"password"`
	DB       int    `json:"db" yaml:"db"`
}

type RedisCache struct {
	client *redis.Client
	// agg is an OPTIONAL cache hit/miss aggregator. When nil (the default), all
	// Observe hooks below are no-ops, so cache-less call paths and tests need no
	// aggregator. Wire one with WithAggregator at service boot (plan 06).
	agg *CacheAggregator
}

// WithAggregator attaches an optional cache hit/miss aggregator so Get/Set
// outcomes are recorded as summed `cache` effect rows (AR-EFFECT-02). It returns
// the receiver for fluent boot wiring. A nil agg is accepted and leaves hooks as
// no-ops.
func (c *RedisCache) WithAggregator(agg *CacheAggregator) *RedisCache {
	c.agg = agg
	return c
}

func New(cfg Config) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisCache{client: client}, nil
}

func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	start := time.Now()
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			metrics.CacheOperationDuration.WithLabelValues("get").Observe(time.Since(start).Seconds())
			metrics.CacheOperationsTotal.WithLabelValues("get", "miss").Inc()
			if c.agg != nil {
				c.agg.Observe(ctx, KeyClass(key), "miss")
			}
			return ErrNotFound
		}
		metrics.CacheOperationDuration.WithLabelValues("get").Observe(time.Since(start).Seconds())
		metrics.CacheOperationsTotal.WithLabelValues("get", "error").Inc()
		if c.agg != nil {
			c.agg.Observe(ctx, KeyClass(key), "error")
		}
		return fmt.Errorf("cache get: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		metrics.CacheOperationDuration.WithLabelValues("get").Observe(time.Since(start).Seconds())
		metrics.CacheOperationsTotal.WithLabelValues("get", "error").Inc()
		if c.agg != nil {
			c.agg.Observe(ctx, KeyClass(key), "error")
		}
		return fmt.Errorf("cache unmarshal: %w", err)
	}

	metrics.CacheOperationDuration.WithLabelValues("get").Observe(time.Since(start).Seconds())
	metrics.CacheOperationsTotal.WithLabelValues("get", "hit").Inc()
	if c.agg != nil {
		c.agg.Observe(ctx, KeyClass(key), "hit")
	}
	return nil
}

// GetDel implements the Cache interface — atomic read-and-delete via Redis
// GETDEL. It mirrors Get's unmarshal/metrics/error shaping exactly, so
// callers can swap a Get+Delete pair for a single race-free call.
func (c *RedisCache) GetDel(ctx context.Context, key string, dest interface{}) error {
	start := time.Now()
	data, err := c.client.GetDel(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			metrics.CacheOperationDuration.WithLabelValues("getdel").Observe(time.Since(start).Seconds())
			metrics.CacheOperationsTotal.WithLabelValues("getdel", "miss").Inc()
			if c.agg != nil {
				c.agg.Observe(ctx, KeyClass(key), "miss")
			}
			return ErrNotFound
		}
		metrics.CacheOperationDuration.WithLabelValues("getdel").Observe(time.Since(start).Seconds())
		metrics.CacheOperationsTotal.WithLabelValues("getdel", "error").Inc()
		if c.agg != nil {
			c.agg.Observe(ctx, KeyClass(key), "error")
		}
		return fmt.Errorf("cache getdel: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		metrics.CacheOperationDuration.WithLabelValues("getdel").Observe(time.Since(start).Seconds())
		metrics.CacheOperationsTotal.WithLabelValues("getdel", "error").Inc()
		if c.agg != nil {
			c.agg.Observe(ctx, KeyClass(key), "error")
		}
		return fmt.Errorf("cache unmarshal: %w", err)
	}

	metrics.CacheOperationDuration.WithLabelValues("getdel").Observe(time.Since(start).Seconds())
	metrics.CacheOperationsTotal.WithLabelValues("getdel", "hit").Inc()
	if c.agg != nil {
		c.agg.Observe(ctx, KeyClass(key), "hit")
	}
	return nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	start := time.Now()
	data, err := json.Marshal(value)
	if err != nil {
		metrics.CacheOperationDuration.WithLabelValues("set").Observe(time.Since(start).Seconds())
		metrics.CacheOperationsTotal.WithLabelValues("set", "error").Inc()
		if c.agg != nil {
			c.agg.Observe(ctx, KeyClass(key), "error")
		}
		return fmt.Errorf("cache marshal: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		metrics.CacheOperationDuration.WithLabelValues("set").Observe(time.Since(start).Seconds())
		metrics.CacheOperationsTotal.WithLabelValues("set", "error").Inc()
		if c.agg != nil {
			c.agg.Observe(ctx, KeyClass(key), "error")
		}
		return fmt.Errorf("cache set: %w", err)
	}

	metrics.CacheOperationDuration.WithLabelValues("set").Observe(time.Since(start).Seconds())
	metrics.CacheOperationsTotal.WithLabelValues("set", "success").Inc()
	if c.agg != nil {
		c.agg.Observe(ctx, KeyClass(key), "success")
	}
	return nil
}

// SetNX implements the Cache interface — atomic SET-if-not-exists with TTL.
// Returns acquired=true when the key was set (it didn't exist before),
// false when the key already existed (the existing value/TTL is unchanged).
// Used by UserOrchestrator.TriggerForUser as a 5-min per-user debounce lock
// (REC-INFRA-02).
func (c *RedisCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	start := time.Now()
	data, err := json.Marshal(value)
	if err != nil {
		metrics.CacheOperationDuration.WithLabelValues("setnx").Observe(time.Since(start).Seconds())
		metrics.CacheOperationsTotal.WithLabelValues("setnx", "error").Inc()
		return false, fmt.Errorf("cache marshal: %w", err)
	}

	ok, err := c.client.SetNX(ctx, key, data, ttl).Result()
	if err != nil {
		metrics.CacheOperationDuration.WithLabelValues("setnx").Observe(time.Since(start).Seconds())
		metrics.CacheOperationsTotal.WithLabelValues("setnx", "error").Inc()
		return false, fmt.Errorf("cache setnx: %w", err)
	}

	outcome := "miss"
	if ok {
		outcome = "acquired"
	}
	metrics.CacheOperationDuration.WithLabelValues("setnx").Observe(time.Since(start).Seconds())
	metrics.CacheOperationsTotal.WithLabelValues("setnx", outcome).Inc()
	return ok, nil
}

func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	err := c.client.Del(ctx, keys...).Err()
	if err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("delete", "error").Inc()
	} else {
		metrics.CacheOperationsTotal.WithLabelValues("delete", "success").Inc()
	}
	return err
}

func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (c *RedisCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fn func() (interface{}, error)) error {
	err := c.Get(ctx, key, dest)
	if err == nil {
		return nil
	}
	if err != ErrNotFound {
		return err
	}

	value, err := fn()
	if err != nil {
		return err
	}

	if err := c.Set(ctx, key, value, ttl); err != nil {
		return err
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache marshal in GetOrSet: %w", err)
	}
	return json.Unmarshal(data, dest)
}

func (c *RedisCache) Invalidate(ctx context.Context, pattern string) error {
	iter := c.client.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}
	return nil
}

func (c *RedisCache) Client() *redis.Client {
	return c.client
}

// HGetAll returns the full field->value map of a Redis hash. It exists so a
// *RedisCache satisfies the gormtrace.HashReader interface directly (the
// db_read P95 ThresholdRefresher snapshots the read_thresholds hash through it),
// keeping libs/tracing free of any go-redis import — each GORM service passes its
// existing *RedisCache as the HashReader at boot (plan 06). An empty map (no
// fields) is a valid, non-error cold-start result.
func (c *RedisCache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, key).Result()
}

// SetJSON is an alias for Set (which already handles JSON marshaling)
func (c *RedisCache) SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return c.Set(ctx, key, value, ttl)
}

// GetJSON is an alias for Get (which already handles JSON unmarshaling)
func (c *RedisCache) GetJSON(ctx context.Context, key string, dest interface{}) error {
	return c.Get(ctx, key, dest)
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

var ErrNotFound = fmt.Errorf("cache: key not found")
