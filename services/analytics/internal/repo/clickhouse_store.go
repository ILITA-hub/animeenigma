package repo

import (
	"context"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/probe"
)

// ClickHouseStore implements domain.EventStore against ClickHouse via the
// native protocol (PrepareBatch + Send). It mirrors PostgresStore exactly in
// shape (struct/constructor/empty-guard/row-mapping) — the only difference is
// the columnar batch write replacing the GORM bulk insert.
type ClickHouseStore struct{ conn driver.Conn }

// Compile-time assertion that ClickHouseStore satisfies the swap seam.
var _ domain.EventStore = (*ClickHouseStore)(nil)

// NewClickHouseStore wraps an open native ClickHouse connection.
func NewClickHouseStore(conn driver.Conn) *ClickHouseStore {
	return &ClickHouseStore{conn: conn}
}

// CHConfig holds the ClickHouse connection parameters. Mirrors the env-driven
// config shape used elsewhere in the service (host/port/db/user/password).
type CHConfig struct {
	Addr     string // host:port for the native protocol, e.g. "clickhouse:9000"
	Database string
	Username string
	Password string
}

// OpenClickHouse opens a native ClickHouse connection with sane single-host
// settings (LZ4 compression, bounded pool, 5s dial timeout). It does NOT run
// the schema — call EnsureSchema after opening, mirroring the AutoMigrateAll +
// EnsureView boot sequence.
func OpenClickHouse(cfg CHConfig) (driver.Conn, error) {
	return clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings:        clickhouse.Settings{"max_execution_time": 60},
		Compression:     &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
		DialTimeout:     5 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
	})
}

// nullableString returns nil for an empty string so it maps to a CH
// Nullable(String) NULL, else a pointer to the value.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

// defaultJSON preserves the toModel behavior: empty JSON-shaped columns default
// to "{}".
func defaultJSON(s string) string {
	if strings.TrimSpace(s) == "" {
		return "{}"
	}
	return s
}

// defaultStr returns def when s is empty. Used so clickstream rows (which leave
// Event's effect dimensions unset) keep the historical column defaults while
// effect rows carry the value the producer supplied.
func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// InsertBatch persists a batch of events via the native columnar batch API.
// Empty batches are a no-op (matching PostgresStore). Each event is appended in
// exact events-table column order; clickstream rows leave the effect
// dimensions/measures at their zero/default values (effect_kind='').
func (s *ClickHouseStore) InsertBatch(ctx context.Context, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO events")
	if err != nil {
		return err
	}
	for _, e := range events {
		// Column order MUST match createEventsTableDDL exactly.
		if err := batch.Append(
			// correlation / time
			e.Timestamp,            // timestamp
			e.ReceivedAt,           // received_at
			e.EventID,              // event_id
			e.TraceID,              // trace_id
			e.SessionID,            // session_id
			e.AnonymousID,          // anonymous_id
			nullableString(e.UserID), // user_id (Nullable)

			// register dimensions — effect rows populate these; clickstream
			// rows leave Event's effect fields empty/zero so the historical
			// defaults (origin="api", source="be", accuracy="exact") apply.
			defaultStr(e.Origin, "api"),     // origin
			e.Operation,                     // operation
			e.EffectKind,                    // effect_kind
			e.TargetKind,                    // target_kind
			e.Target,                        // target
			defaultStr(e.Source, "be"),      // source
			defaultStr(e.Accuracy, "exact"), // accuracy
			nullableString(e.AnimeID),       // anime_id (Nullable)

			// reconciled clickstream dimensions
			string(e.EventType),     // event_type
			e.EventName,             // event_name
			e.URL,                   // url
			e.Path,                  // path
			e.Referrer,              // referrer
			e.Title,                 // title
			e.ElSelector,            // el_selector
			e.ElText,                // el_text
			e.ElTag,                 // el_tag
			defaultJSON(e.ElAttrs),  // el_attrs
			e.UserAgent,             // user_agent
			e.DeviceType,            // device_type
			uint16(e.ScreenW),       // screen_w
			uint16(e.ScreenH),       // screen_h
			e.IPHash,                // ip_hash
			defaultJSON(e.Properties), // properties

			// register measures — effect rows populate these; clickstream rows
			// leave Event's measure fields at 0.
			uint32(e.Requests),   // requests
			uint64(e.BytesIn),    // bytes_in
			uint64(e.BytesOut),   // bytes_out
			uint32(e.DurationMS), // duration_ms
			uint32(e.RowCount),   // row_count
			uint32(e.ActiveMS),   // active_ms
		); err != nil {
			return err
		}
	}
	return batch.Send()
}

// UpsertIdentity appends an identity row. ClickHouse has no UPSERT; the
// "latest row per anonymous_id wins" semantic is resolved at query time by the
// argMax events_resolved view (consistent with the append-only Postgres impl).
func (s *ClickHouseStore) UpsertIdentity(ctx context.Context, anonymousID, userID string, ts time.Time) error {
	if anonymousID == "" || userID == "" {
		return nil
	}
	return s.conn.Exec(ctx,
		"INSERT INTO identities (anonymous_id, user_id, timestamp) VALUES (?, ?, ?)",
		anonymousID, userID, ts)
}

// EraseByUserIDCH is the ClickHouse analog of repo.EraseByUserID: it removes
// all events for the user_id AND all events for anonymous_ids stitched to the
// user via identities, plus the identity rows. ClickHouse deletes are
// ALTER TABLE ... DELETE mutations (admin-triggered GDPR erase only). Uses
// parameterized placeholders (T-01-04). resolve.go itself stays Postgres-only.
func EraseByUserIDCH(ctx context.Context, conn driver.Conn, userID string) error {
	// Resolve the anonymous ids mapped to this user first.
	rows, err := conn.Query(ctx,
		"SELECT DISTINCT anonymous_id FROM identities WHERE user_id = ?", userID)
	if err != nil {
		return err
	}
	var anonIDs []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			return err
		}
		anonIDs = append(anonIDs, a)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if err := conn.Exec(ctx,
		"ALTER TABLE events DELETE WHERE user_id = ?", userID); err != nil {
		return err
	}
	if len(anonIDs) > 0 {
		if err := conn.Exec(ctx,
			"ALTER TABLE events DELETE WHERE anonymous_id IN (?)", anonIDs); err != nil {
			return err
		}
	}
	return conn.Exec(ctx,
		"ALTER TABLE identities DELETE WHERE user_id = ?", userID)
}

// EraseByAnonymousIDCH is the ClickHouse analog of repo.EraseByAnonymousID.
func EraseByAnonymousIDCH(ctx context.Context, conn driver.Conn, anonymousID string) error {
	if err := conn.Exec(ctx,
		"ALTER TABLE events DELETE WHERE anonymous_id = ?", anonymousID); err != nil {
		return err
	}
	return conn.Exec(ctx,
		"ALTER TABLE identities DELETE WHERE anonymous_id = ?", anonymousID)
}

// ReadThresholdRow is one (operation, target) -> p95 duration row produced by
// the daily db_read P95 query. p95_ms is the 95th-percentile of duration_ms for
// that key over the lookback window.
type ReadThresholdRow struct {
	Operation string
	Target    string
	P95MS     float64
}

// QueryReadThresholdP95 runs the daily per-(operation, target) db_read P95
// query against the ClickHouse events table over the last `lookbackDays` days.
// It keeps only keys with at least minSamples rows (HAVING count() >=
// minSamples) so tiny-sample keys are dropped — those fall back to the gate's
// static cold-start default (D-03 / Pitfall 5). This READS the events table
// only; it records NO effect (D-16 — analytics is excluded from recording).
//
// The query is parameterized (no string interpolation of the bounds) so it is
// injection-safe even though both args are int-typed.
func QueryReadThresholdP95(ctx context.Context, conn driver.Conn, lookbackDays, minSamples int) ([]ReadThresholdRow, error) {
	const q = `SELECT operation, target, quantile(0.95)(duration_ms) AS p95_ms
FROM events
WHERE effect_kind = 'db_read' AND timestamp >= now() - INTERVAL ? DAY
GROUP BY operation, target
HAVING count() >= ?`
	rows, err := conn.Query(ctx, q, lookbackDays, minSamples)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ReadThresholdRow
	for rows.Next() {
		var r ReadThresholdRow
		// quantile(0.95)(UInt32) returns Float64 in ClickHouse.
		if err := rows.Scan(&r.Operation, &r.Target, &r.P95MS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ProviderReliabilityRow is one (anime_id, provider) reliability aggregate over
// the player_resolve / player_stall telemetry. AnimeID is "" for global rows.
type ProviderReliabilityRow struct {
	AnimeID  string
	Provider string
	Resolves uint64
	Reached  uint64
	OK       uint64
	Stalls   uint64
	P95MS    float64
}

// QueryProviderReliability aggregates Stage 2a player telemetry over the last
// lookbackDays. When perAnime is false it groups by provider only (anime_id="")
// for the global ranking; when true it groups by (anime_id, provider). Keys with
// fewer than minResolves player_resolve rows are dropped (HAVING) so a handful of
// plays can't manufacture a misleading score. READS the events table only.
//
// Injection-safety: the two ? bounds are parameterized (same pattern as
// QueryReadThresholdP95). The only string-concatenated fragments (groupCols,
// selectAnime) are derived solely from the perAnime bool — never from external or
// user input — so the concatenation is injection-safe.
//
// Per-anime NULL semantics: when perAnime is false, AnimeID=="" signals a global
// aggregate row. When perAnime is true, AnimeID=="" means the underlying event had
// a NULL anime_id (collapsed by ifNull); treat such rows as uncorrelated rather
// than as belonging to any specific anime.
//
// Stall correlation caveat: Stalls counts all player_stall events for the provider
// in the window regardless of anime_id. In per-anime mode, stall events with a
// NULL anime_id won't share the anime grouping key, so per-anime stall counts may
// be conservative (understated).
func QueryProviderReliability(ctx context.Context, conn driver.Conn, lookbackDays, minResolves int, perAnime bool) ([]ProviderReliabilityRow, error) {
	groupCols := "target"
	selectAnime := "'' AS anime_id"
	if perAnime {
		groupCols = "anime_id, target"
		selectAnime = "ifNull(anime_id, '') AS anime_id"
	}
	q := `SELECT ` + selectAnime + `, target AS provider,
       countIf(effect_kind='player_resolve') AS resolves,
       countIf(effect_kind='player_resolve' AND JSONExtractBool(properties,'reached_playback')) AS reached,
       countIf(effect_kind='player_resolve' AND JSONExtractString(properties,'outcome')='ok') AS ok,
       countIf(effect_kind='player_stall') AS stalls,
       quantileIf(0.95)(duration_ms, effect_kind='player_resolve') AS p95_ms
FROM events
WHERE effect_kind IN ('player_resolve','player_stall')
  AND timestamp >= now() - INTERVAL ? DAY
  AND target != ''
GROUP BY ` + groupCols + `
HAVING resolves >= ?`
	rows, err := conn.Query(ctx, q, lookbackDays, minResolves)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProviderReliabilityRow
	for rows.Next() {
		var r ProviderReliabilityRow
		if err := rows.Scan(&r.AnimeID, &r.Provider, &r.Resolves, &r.Reached, &r.OK, &r.Stalls, &r.P95MS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PurgeOlderThanCH is a no-op: ClickHouse retention is handled declaratively by
// the events table's native `TTL ... DELETE`, so the Go purge cron is retired
// for the ClickHouse backend (RESEARCH §Don't Hand-Roll). cutoff is accepted
// only to mirror repo.PurgeOlderThan's signature shape.
func PurgeOlderThanCH(_ context.Context, _ driver.Conn, _ time.Time) (int64, error) {
	return 0, nil
}

// InsertProbeRows persists a batch of playback-probe verdicts to the probe_runs
// table. Empty batches are a no-op (matching InsertBatch). Each row maps
// ProbeRow.RunTS (Unix seconds) to a DateTime and ProbeRow.Playable to UInt8,
// mirroring the InsertBatch PrepareBatch/Append/Send idiom exactly.
func (s *ClickHouseStore) InsertProbeRows(ctx context.Context, rows []probe.ProbeRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO probe_runs")
	if err != nil {
		return err
	}
	for _, r := range rows {
		var pl uint8
		if r.Playable {
			pl = 1
		}
		if err := batch.Append(
			time.Unix(r.RunTS, 0), // run_ts  DateTime
			r.Provider,            // provider
			r.AnimeUUID,           // anime_uuid
			r.AnimeName,           // anime_name
			r.Slot,                // slot
			r.Server,              // server
			r.Stage,               // stage
			r.Reason,              // reason
			pl,                    // playable UInt8
		); err != nil {
			return err
		}
	}
	return batch.Send()
}

// Compile-time assertion: ClickHouseStore must satisfy probe.CHWriter.
var _ probe.CHWriter = (*ClickHouseStore)(nil)

// UpscaleTelemetryRow is one per-worker GPU telemetry sample forwarded from
// the upscaler service. JSON tags match the wire shape the upscaler's
// analyticsclient sends, in DDL column order (CD-15).
type UpscaleTelemetryRow struct {
	TS           time.Time `json:"ts"`
	WorkerID     string    `json:"worker_id"`
	GPUModel     string    `json:"gpu_model"`
	ImageVersion string    `json:"image_version"`
	JobID        string    `json:"job_id"`
	SegmentIdx   int32     `json:"segment_idx"`
	GPUUtil      float32   `json:"gpu_util"`
	VRAMUsedB    uint64    `json:"vram_used_b"`
	VRAMTotalB   uint64    `json:"vram_total_b"`
	GPUTempC     float32   `json:"gpu_temp_c"`
	GPUPowerW    float32   `json:"gpu_power_w"`
	DecodeFPS    float32   `json:"decode_fps"`
	InferenceFPS float32   `json:"inference_fps"`
	EncodeFPS    float32   `json:"encode_fps"`
}

// InsertUpscaleTelemetry persists a batch of GPU telemetry rows to the
// upscale_worker_telemetry table. Empty batches are a no-op. Each row is
// appended in exact DDL column order using the PrepareBatch/Append/Send
// idiom (mirrors InsertBatch and InsertProbeRows).
func (s *ClickHouseStore) InsertUpscaleTelemetry(ctx context.Context, rows []UpscaleTelemetryRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO upscale_worker_telemetry")
	if err != nil {
		return err
	}
	for _, r := range rows {
		// Column order MUST match createUpscaleTelemetryTableDDL exactly.
		if err := batch.Append(
			r.TS,           // ts             DateTime64(3)
			r.WorkerID,     // worker_id      String
			r.GPUModel,     // gpu_model      LowCardinality(String)
			r.ImageVersion, // image_version  LowCardinality(String)
			r.JobID,        // job_id         String
			r.SegmentIdx,   // segment_idx    Int32
			r.GPUUtil,      // gpu_util       Float32
			r.VRAMUsedB,    // vram_used_b    UInt64
			r.VRAMTotalB,   // vram_total_b   UInt64
			r.GPUTempC,     // gpu_temp_c     Float32
			r.GPUPowerW,    // gpu_power_w    Float32
			r.DecodeFPS,    // decode_fps     Float32
			r.InferenceFPS, // inference_fps  Float32
			r.EncodeFPS,    // encode_fps     Float32
		); err != nil {
			return err
		}
	}
	return batch.Send()
}
