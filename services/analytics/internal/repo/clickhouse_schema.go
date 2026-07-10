package repo

import (
	"context"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// ClickHouse DDL for the unified wide-event store (AR-STORE-02).
//
// The design mirrors the schema-on-boot convention of models.go
// (AutoMigrateAll + EnsureView) but uses CH-native "IF NOT EXISTS" instead of
// the DROP+CREATE dance. Everything here is idempotent and re-runnable.
//
// ClickHouse has no row UPDATE, so:
//   - the events table carries a 90-day native TTL (replaces the Go purge cron);
//   - identity stitching is append-only + resolved at query time via argMax
//     (NOT a verbatim port of the correlated-subquery Postgres view, which is
//     invalid in ClickHouse).
//
// The measure column is named row_count, NOT rows: "rows" is reserved-ish and
// the clickhouse-go/v2 native batch binder treats it inconsistently, so we
// avoid it entirely (RESEARCH note A2). All other column names match the
// Postgres analytics_events shape one-for-one so the existing clickstream maps
// straight across; the effect dimensions/measures default to '' / 0 for
// clickstream rows (effect_kind='').
const (
	// createDatabaseDDL ensures the analytics database exists. Harmless when the
	// connection's default database is already analytics.
	createDatabaseDDL = `CREATE DATABASE IF NOT EXISTS analytics`

	// createEventsTableDDL is the unified wide-event table: one row per effect,
	// plus the reconciled clickstream dimensions. LowCardinality dictionary-
	// encodes the categorical dims; Delta+ZSTD compresses the monotonic/counter
	// columns; plain ZSTD(1) compresses free-text/high-cardinality strings.
	// Nullable user_id/anime_id are kept OUT of the ORDER BY (Pitfall 4).
	createEventsTableDDL = `CREATE TABLE IF NOT EXISTS events
(
    -- correlation / time
    timestamp     DateTime64(3)               CODEC(Delta, ZSTD(1)),
    received_at   DateTime64(3)               CODEC(Delta, ZSTD(1)),
    event_id      String                      CODEC(ZSTD(1)),
    trace_id      String                      CODEC(ZSTD(1)),
    session_id    String                      CODEC(ZSTD(1)),
    anonymous_id  String                      CODEC(ZSTD(1)),
    user_id       Nullable(String)            CODEC(ZSTD(1)),

    -- register dimensions (categorical -> LowCardinality)
    origin        LowCardinality(String)  DEFAULT 'api',
    operation     LowCardinality(String)  DEFAULT '',
    effect_kind   LowCardinality(String)  DEFAULT '',
    target_kind   LowCardinality(String)  DEFAULT '',
    target        String                  DEFAULT '' CODEC(ZSTD(1)),
    provider      LowCardinality(String)  DEFAULT '',
    source        LowCardinality(String)  DEFAULT 'be',
    accuracy      LowCardinality(String)  DEFAULT 'exact',
    anime_id      Nullable(String),

    -- reconciled clickstream dimensions
    event_type    LowCardinality(String),
    event_name    LowCardinality(String)  DEFAULT '',
    url           String                  DEFAULT '' CODEC(ZSTD(1)),
    path          String                  DEFAULT '' CODEC(ZSTD(1)),
    referrer      String                  DEFAULT '' CODEC(ZSTD(1)),
    title         String                  DEFAULT '' CODEC(ZSTD(1)),
    el_selector   String                  DEFAULT '' CODEC(ZSTD(1)),
    el_text       String                  DEFAULT '' CODEC(ZSTD(1)),
    el_tag        LowCardinality(String)  DEFAULT '',
    el_attrs      String                  DEFAULT '{}' CODEC(ZSTD(1)),
    user_agent    String                  DEFAULT '' CODEC(ZSTD(1)),
    device_type   LowCardinality(String)  DEFAULT '',
    screen_w      UInt16                  DEFAULT 0,
    screen_h      UInt16                  DEFAULT 0,
    ip_hash       String                  DEFAULT '' CODEC(ZSTD(1)),
    properties    String                  DEFAULT '{}' CODEC(ZSTD(1)),

    -- register measures
    requests      UInt32 DEFAULT 0,
    bytes_in      UInt64 DEFAULT 0 CODEC(Delta, ZSTD(1)),
    bytes_out     UInt64 DEFAULT 0 CODEC(Delta, ZSTD(1)),
    duration_ms   UInt32 DEFAULT 0 CODEC(Delta, ZSTD(1)),
    row_count     UInt32 DEFAULT 0,
    active_ms     UInt32 DEFAULT 0
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (toDate(timestamp), origin, operation, effect_kind, target, timestamp)
TTL toDateTime(timestamp) + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192`

	// createIdentitiesTableDDL is the append-only identity table. No
	// ReplacingMergeTree / FINAL: the "latest row per anonymous_id wins"
	// semantic is resolved at query time by the argMax view below (Pattern 2).
	createIdentitiesTableDDL = `CREATE TABLE IF NOT EXISTS identities
(
    anonymous_id String,
    user_id      String,
    timestamp    DateTime64(3) CODEC(Delta, ZSTD(1))
)
ENGINE = MergeTree
ORDER BY (anonymous_id, timestamp)`

	// createProbeRunsTableDDL records one row per (run, provider, anime, server)
	// probe verdict. Used by probe.PromReporter via repo.ClickHouseStore to write
	// the playback-health history consumed by the Grafana probe dashboard.
	// TTL mirrors the events table: 90 days, then ClickHouse deletes the data.
	createProbeRunsTableDDL = `CREATE TABLE IF NOT EXISTS probe_runs
(
  run_ts      DateTime,
  provider    LowCardinality(String),
  anime_uuid  String,
  anime_name  String,
  slot        LowCardinality(String),
  server      String,
  stage       LowCardinality(String),
  reason      LowCardinality(String),
  playable    UInt8
) ENGINE = MergeTree ORDER BY (run_ts, provider) TTL run_ts + INTERVAL 90 DAY`

	// alterProbeRunsAddAnimeNameDDL adds anime_name to a probe_runs table that
	// predates the column (the create-table DDL only fires on a fresh install).
	// Idempotent: IF NOT EXISTS makes re-runs on every boot a no-op. AFTER
	// anime_uuid keeps the physical column order aligned with the positional
	// INSERT in ClickHouseStore.InsertProbeRows.
	alterProbeRunsAddAnimeNameDDL = `ALTER TABLE probe_runs ADD COLUMN IF NOT EXISTS anime_name String AFTER anime_uuid`

	// alterEventsAddProviderDDL adds the provider dimension to an events table
	// that predates the column (the create-table DDL only fires on a fresh
	// install). Idempotent via IF NOT EXISTS. AFTER target keeps the physical
	// column order aligned with the positional INSERT in ClickHouseStore.InsertBatch
	// (provider is appended right after target there).
	alterEventsAddProviderDDL = `ALTER TABLE events ADD COLUMN IF NOT EXISTS provider LowCardinality(String) DEFAULT '' AFTER target`

	// createResolvedViewDDL stitches identity at query time. argMax(user_id,
	// timestamp) picks the latest identity per anonymous_id; person_id is the
	// identified user if ever known, else the anonymous id. This preserves the
	// Postgres analytics_events_resolved semantics without a correlated
	// subquery (unsupported in ClickHouse).
	createResolvedViewDDL = `CREATE VIEW IF NOT EXISTS events_resolved AS
SELECT e.*,
       coalesce(e.user_id, i.user_id)                 AS resolved_user_id,
       coalesce(e.user_id, i.user_id, e.anonymous_id) AS person_id
FROM events AS e
LEFT JOIN (
    SELECT anonymous_id, argMax(user_id, timestamp) AS user_id
    FROM identities
    GROUP BY anonymous_id
) AS i USING (anonymous_id)`

	// createUpscaleTelemetryTableDDL records one row per worker telemetry sample
	// (GPU utilisation, VRAM, temperature, power, FPS metrics). High-cardinality
	// per-worker history — kept in ClickHouse rather than Prometheus (CD-15).
	// TTL mirrors the events table: 90 days, then ClickHouse deletes the data.
	createUpscaleTelemetryTableDDL = `CREATE TABLE IF NOT EXISTS upscale_worker_telemetry (
    ts             DateTime64(3) CODEC(Delta, ZSTD(1)),
    worker_id      String        CODEC(ZSTD(1)),
    gpu_model      LowCardinality(String),
    image_version  LowCardinality(String),
    job_id         String        CODEC(ZSTD(1)),
    segment_idx    Int32,
    gpu_util       Float32,
    vram_used_b    UInt64        CODEC(Delta, ZSTD(1)),
    vram_total_b   UInt64        CODEC(Delta, ZSTD(1)),
    gpu_temp_c     Float32,
    gpu_power_w    Float32,
    decode_fps     Float32,
    inference_fps  Float32,
    encode_fps     Float32
) ENGINE = MergeTree
  PARTITION BY toYYYYMM(ts)
  ORDER BY (worker_id, ts)
  TTL toDateTime(ts) + INTERVAL 90 DAY DELETE
  SETTINGS index_granularity = 8192`

	// createDegradationTransitionsTableDDL records one row per published
	// degradation-level change from the governor service (graceful-degradation
	// Phase 2) — the durable "what changed, when, and why" history rendered as
	// annotations on the Degradation Overview dashboard. Transitions are rare
	// (a handful per pressure event), so a 365-day TTL costs nothing and keeps
	// a year of incident context.
	createDegradationTransitionsTableDDL = `CREATE TABLE IF NOT EXISTS degradation_transitions (
    ts            DateTime64(3) CODEC(Delta, ZSTD(1)),
    from_level    UInt8,
    to_level      UInt8,
    reasons       Array(String),
    signal_values Map(String, Float64)
) ENGINE = MergeTree
  PARTITION BY toYYYYMM(ts)
  ORDER BY ts
  TTL toDateTime(ts) + INTERVAL 365 DAY DELETE
  SETTINGS index_granularity = 8192`
)

// EnsureSchema idempotently creates the ClickHouse analytics schema: the
// unified wide-event events MergeTree, the append-only identities table, and
// the argMax events_resolved view. It mirrors the schema-on-boot call site of
// repo.AutoMigrateAll + repo.EnsureView and is safe to run on every boot.
func EnsureSchema(ctx context.Context, conn driver.Conn) error {
	for _, ddl := range []string{
		createDatabaseDDL,
		createEventsTableDDL,
		alterEventsAddProviderDDL,
		createIdentitiesTableDDL,
		createProbeRunsTableDDL,
		alterProbeRunsAddAnimeNameDDL,
		createResolvedViewDDL,
		createUpscaleTelemetryTableDDL,
		createDegradationTransitionsTableDDL,
	} {
		if err := conn.Exec(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}
