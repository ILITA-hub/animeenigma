// Package service holds analytics business logic. read_threshold.go closes the
// D-03 daily feedback loop that makes the plan-03 GORM ReadGate dynamic: it
// computes a per-(operation, target) db_read P95 from the ClickHouse events
// register and publishes a compact `read_thresholds` Redis hash that every GORM
// service snapshots on a ticker (libs/tracing/gormtrace ThresholdRefresher).
//
// D-16: this path READS the events table only — it registers NO effect callback
// (analytics is excluded from recording, enforced in boot wiring).
package service

import (
	"context"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
)

// ReadThresholdsHashKey is the Redis hash that carries the dynamic db_read P95
// thresholds. Field = "operation|target" (matching the plan-03 ReadGate
// snapshot key format), value = p95_ms as a decimal string. The 7 GORM services
// HGETALL this hash on a ticker and feed it into their in-memory ReadGate.
const ReadThresholdsHashKey = "read_thresholds"

// Tuning defaults for the daily P95 compute. Exposed as fields on
// ReadThresholdService so they can be overridden without touching the query.
const (
	defaultLookbackDays = 1
	// minSamples drops tiny-sample (operation, target) keys so a handful of
	// slow reads can't manufacture a misleading P95 — those keys fall back to
	// the gate's static cold-start default (Pitfall 5 / cold-start safety).
	defaultMinSamples = 20
	// defaultHashTTL is generous (48h) so a single missed daily run does NOT
	// blank the threshold table (T-03-17 — a blanked table would make the
	// static default fire everywhere). The next successful run refreshes it.
	defaultHashTTL = 48 * time.Hour
)

// redisHashWriter is the narrow slice of *redis.Client the publisher needs. It
// lets tests inject a fake without a live Redis (the concrete *redis.Client
// satisfies it via HSet/Expire returning *redis.*Cmd whose .Err() is checked by
// the adapter in main.go). The publisher takes already-extracted errors so this
// package does not import go-redis.
type redisHashWriter interface {
	// HSetThresholds replaces the read_thresholds hash with fields and sets the
	// expiry atomically. An empty map is a no-op (it must NOT delete the hash —
	// a transient empty compute should leave the last good thresholds in place
	// until they expire).
	HSetThresholds(ctx context.Context, key string, fields map[string]string, ttl time.Duration) error
}

// ReadThresholdService computes the daily db_read P95 from ClickHouse and
// publishes it to the read_thresholds Redis hash.
type ReadThresholdService struct {
	conn         driver.Conn
	redis        redisHashWriter
	lookbackDays int
	minSamples   int
	hashTTL      time.Duration
}

// NewReadThresholdService builds the service over an open ClickHouse connection
// and a Redis hash writer. conn or redis may be nil only in degraded boots
// (Recompute guards against a nil conn); production wiring supplies both.
func NewReadThresholdService(conn driver.Conn, rd redisHashWriter) *ReadThresholdService {
	return &ReadThresholdService{
		conn:         conn,
		redis:        rd,
		lookbackDays: defaultLookbackDays,
		minSamples:   defaultMinSamples,
		hashTTL:      defaultHashTTL,
	}
}

// ComputeReadThresholds runs the daily P95 query against the ClickHouse events
// table and returns a map keyed "operation|target" -> p95_ms. The underlying
// SQL (in repo.QueryReadThresholdP95, where the driver.Conn lives) is:
//
//	SELECT operation, target, quantile(0.95)(duration_ms) AS p95_ms
//	FROM events
//	WHERE effect_kind = 'db_read' AND timestamp >= now() - INTERVAL ? DAY
//	GROUP BY operation, target HAVING count() >= ?
//
// effect_kind='db_read', last lookbackDays days, grouped by operation+target,
// HAVING count() >= minSamples drops tiny-sample keys. The key format matches
// the plan-03 ReadGate snapshot exactly so the refresher feeds it through
// unchanged.
func (s *ReadThresholdService) ComputeReadThresholds(ctx context.Context) (map[string]float64, error) {
	rows, err := repo.QueryReadThresholdP95(ctx, s.conn, s.lookbackDays, s.minSamples)
	if err != nil {
		return nil, err
	}
	m := make(map[string]float64, len(rows))
	for _, r := range rows {
		// Skip degenerate keys defensively (empty operation/target would
		// produce an ambiguous "|"-only field).
		if r.Operation == "" && r.Target == "" {
			continue
		}
		m[r.Operation+"|"+r.Target] = r.P95MS
	}
	return m, nil
}

// PublishReadThresholds writes the computed P95 map to the read_thresholds Redis
// hash (field="op|table", value=p95_ms) with a 48h expiry. An empty map is a
// no-op so a transient empty compute never blanks the live thresholds.
func (s *ReadThresholdService) PublishReadThresholds(ctx context.Context, m map[string]float64) error {
	if len(m) == 0 {
		return nil
	}
	fields := make(map[string]string, len(m))
	for k, v := range m {
		fields[k] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return s.redis.HSetThresholds(ctx, ReadThresholdsHashKey, fields, s.hashTTL)
}

// Recompute is the daily entry point: compute then publish. It is the single
// method the analytics /internal recompute endpoint and any internal cron call.
// A nil ClickHouse connection (degraded boot, e.g. dualwrite fell back to
// postgres-only) is a no-op success — there is nothing to query, and the
// existing 48h hash carries the GORM services until CH returns.
func (s *ReadThresholdService) Recompute(ctx context.Context) error {
	if s.conn == nil || s.redis == nil {
		return nil
	}
	m, err := s.ComputeReadThresholds(ctx)
	if err != nil {
		return err
	}
	return s.PublishReadThresholds(ctx, m)
}
