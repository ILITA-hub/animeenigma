# Phase 3: DB/Cache Effects + Auto Operation Discovery - Pattern Map

**Mapped:** 2026-06-05
**Files analyzed:** 13 (4 EXTEND existing, 6 NEW Go, 1 EXTEND config, 1 NEW analytics path, 1 EXTEND scheduler)
**Analogs found:** 12 / 13 (1 net-new config has no analog by design — D-12)

> Brownfield phase. ~70% of the plumbing already ships (Phase-1 ClickHouse sink, Phase-2 `Producer`/`EffectSink`/baggage + HLS reaper). Every "NEW" file below has a production-proven analog in the repo to mirror; planner copies the analog's shape, not generic boilerplate.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `libs/tracing/effect.go` (EXTEND) | model/struct | transform | itself (add fields) | exact (self) |
| `libs/tracing/producer.go` (EXTEND) | service (transport) | batch / pub-sub | itself (`wireProducerEffect`, `post()`) | exact (self) |
| `libs/tracing/client.go` (EXTEND) | middleware (RoundTripper) | request-response | itself (`recordingTransport.RoundTrip`) | exact (self) |
| `libs/tracing/attribution.go` (NEW) | utility | transform | `libs/tracing/baggage.go` (`ReadBaggage`, fallback chain) | role-match |
| `libs/tracing/gormtrace/gorm_effect.go` (NEW) | middleware (DB hook) | event-driven (callback) | `libs/tracing/gormtrace/gorm.go` (`InstrumentGORM`) + `client.go` (Effect build) | role-match |
| `libs/cache/aggregator.go` (NEW) | service (aggregator) | event-driven + batch (flush) | `services/streaming/internal/service/hls_sessions.go` (`HLSSessions`) | **exact (byte-for-byte clone)** |
| `libs/cache/keyclass.go` (NEW) | utility (classifier) | transform | `libs/cache/ttl.go` (prefixes + key builders) | role-match |
| `libs/cache/cache.go` (EXTEND) | service (cache wrapper) | CRUD (Get/Set) | itself (`metrics.CacheOperationsTotal` metering sites) | exact (self) |
| `infra/tempo/tempo.yaml` (EXTEND) | config | — | **no analog** (net-new `metrics_generator` block, D-12) | none |
| `services/analytics` daily-P95 query + threshold exposure (NEW) | repo + handler | transform / request-response | `clickhouse_store.go` (CH conn + query) + `handler/effects.go` (`/internal/*`) | role-match |
| `services/scheduler` daily cron entry (EXTEND) | service (job) | event-driven (cron) | `services/scheduler/internal/service/job.go` (`AddFunc` blocks) + `jobs/cleanup.go` (job shape) | exact |
| `services/{7 gorm svcs}/cmd/*/main.go` (EXTEND) | config (boot wiring) | — | `catalog/cmd/catalog-api/main.go:68` + `streaming/cmd/streaming-api/main.go:82-91` | exact |
| `libs/tracing/attribution_test.go` / `gorm_effect_test.go` / `libs/cache/aggregator_test.go` (NEW) | test | — | (Phase-2 reaper tests; deterministic-clock pattern via `now func()`) | role-match |

---

## Pattern Assignments

### `libs/cache/aggregator.go` (NEW — aggregator, event-driven + flush) — D-05/D-10

**Analog:** `services/streaming/internal/service/hls_sessions.go` (the entire file — clone it). This is the single most important analog: the research mandates "clone `HLSSessions` byte-for-byte." Swap `sessKey{sess,host}` → `counterKey{keyClass,result,operation}` and `sessionTally{bytes…}` → `{requests uint32}`.

**Struct + concurrency contract** (`hls_sessions.go:22-63`):
```go
type HLSSessions struct {
	sink       tracing.EffectSink
	idleWindow time.Duration
	maxEntries int           // hard map-size cap; oldest evicted on overflow (T-02-DOS)
	mu       sync.Mutex
	sessions map[sessKey]*sessionTally
	now func() time.Time     // clock; overridable in tests for deterministic flushing
	stop   chan struct{}
	doneWG sync.WaitGroup
	once   sync.Once
}
const (
	defaultIdleWindow = 45 * time.Second
	defaultMaxEntries = 10000
	reaperInterval    = 10 * time.Second // ← matches the ~10s cache flush D-05 wants
)
```

**Lock-only-on-map Observe** (`hls_sessions.go:123-143`) — mirror exactly; measure result OUTSIDE the lock (Pitfall 3), take the lock only for the map increment:
```go
func (s *HLSSessions) Observe(sess, host string, bytesIn, bytesOut uint64) {
	if sess == "" || s.sink == nil { return }
	now := s.now()
	key := sessKey{sess: sess, host: host}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.sessions[key]
	if t == nil { s.evictIfFullLocked(); t = &sessionTally{firstSeen: now, host: host}; s.sessions[key] = t }
	t.bytesIn += bytesIn; t.bytesOut += bytesOut; t.segments++; t.lastSeen = now
}
```

**Bounded-map oldest-eviction** (`hls_sessions.go:147-162`) — copy `evictIfFullLocked` verbatim (T-02-DOS protection against a key-flood).

**Record-time attribution capture (D-10)** — the `Mint` method (`hls_sessions.go:91-116`) is the template for capturing `operation` at record-time (cache has no live stack at flush). Per research D-10 recommendation: **cache uses the COARSE baggage operation read from ctx in `Observe`, NOT a per-op stack-walk** (a stack-walk on every Get/Set would violate D-11). So the cache aggregator's `Observe(ctx, keyClass, result)` reads `_, op := tracing.ReadBaggage(ctx)` once (cheap ctx read), and keys the counter by `(keyClass, result, op)`.

**Reaper Start / graceful flushAll Stop** (`hls_sessions.go:191-249`) — copy `flushIdle(now)`, `flushAll()`, `Start()` (ticker on `reaperInterval`), `Stop()` (`once.Do(close)` + `doneWG.Wait()` + `flushAll()`). The split between `flushIdle(now)` and the reaper loop exists specifically so tests drive flushing with an injected clock — preserve it.

**recordLocked → Effect** (`hls_sessions.go:167-186`): the cache version emits `EffectKind: "cache"`, `Target: keyClass`, `TargetKind: "key_class"`, `Requests: int(tally.requests)`, `Operation: op`, NO trace_id (D-06). Note the analog hard-codes `EffectKind: "egress"`/`Target: t.host` — change these for cache.

---

### `libs/cache/keyclass.go` (NEW — classifier, transform) — D-07

**Analog:** `libs/cache/ttl.go` (the prefix consts + key builders are the source-of-truth taxonomy).

**Source taxonomy to derive ~20-30 classes from** (`ttl.go:40-117`). The classifier strips variable IDs and maps a raw key → stable class. Prefixes + sub-namespaces present today:
```go
const (
	PrefixAnime = "anime:"   PrefixEpisode = "episode:"   PrefixUser = "user:"
	PrefixSession = "session:"   PrefixSearch = "search:"   PrefixProgress = "progress:"
	PrefixVideo = "video:"   PrefixGenre = "genre:"   PrefixStudio = "studio:"
	PrefixExternalID = "extid:"   PrefixRateLimit = "ratelimit:"   PrefixRoom = "room:"
	PrefixTelegramAuth = "tgauth:"
)
// Sub-namespaces baked into the builders (these become distinct key-classes):
//   anime:<id>                → "anime:detail"   (KeyAnime)
//   anime:list:<filters>:p:l  → "anime:list"     (KeyAnimeList)
//   anime:top:trending        → "anime:top"      (KeyTopAnime)
//   anime:related:<id>        → "anime:related"  (KeyRelatedAnime)
//   anime:similar:<id>        → "anime:similar"  (KeySimilarAnime)
//   user:profile:<id>         → "user:profile"   (KeyUserProfile)
//   search:<q>:<p>            → "search"         (KeySearchResults)
//   progress:<uid>:<aid>      → "progress"       (KeyWatchProgress)
//   video:manifest:<id>       → "video:manifest" (KeyVideoManifest)
//   extid:<src>:<id>          → "extid"          (KeyExternalID)
```
**Classifier rule:** split on the first 1-2 `:` segments, keep prefix + sub-namespace, drop everything after the first variable ID. `anime:related:` vs `anime:similar:` MUST stay separate (D-07 calls out their very different hit rates). Unknown prefixes → an `other` bucket (never let the class set grow unbounded — it keys the aggregator map).

---

### `libs/cache/cache.go` (EXTEND — cache wrapper, CRUD) — AR-EFFECT-02

**Analog:** itself — the existing `metrics.CacheOperationsTotal` call sites are exactly where the aggregator hook goes.

**Hook points** — every place that already classifies hit/miss/error gets an `aggregator.Observe(ctx, keyClass(key), result)` call alongside the existing metric. `Get` (`cache.go:55-78`) has three terminal sites:
```go
metrics.CacheOperationsTotal.WithLabelValues("get", "miss").Inc()   // line 61 → Observe(ctx, kc, "miss")
metrics.CacheOperationsTotal.WithLabelValues("get", "error").Inc()  // lines 65,71
metrics.CacheOperationsTotal.WithLabelValues("get", "hit").Inc()    // line 76 → Observe(ctx, kc, "hit")
```
`Set` (`cache.go:80-98`) meters `"success"`/`"error"`. The aggregator is an optional field on `RedisCache` (nil → no-op, like `HLSSessions` guards `sink == nil`) so cache-less call paths and tests need no aggregator. Mirror the existing nil-guard discipline; do NOT make the aggregator mandatory in `New()`.

---

### `libs/tracing/gormtrace/gorm_effect.go` (NEW — DB callback hook, event-driven) — D-01/D-02

**Analog:** `libs/tracing/gormtrace/gorm.go` (`InstrumentGORM` — where the hook registers, alongside the otelgorm plugin) + `client.go:137-152` (the `build` closure → `Effect` shape).

**Existing registration seam** (`gorm.go:18-20`) — the new `RegisterEffectCallbacks(db, sink, gate)` is called right after this in each service's `main.go`:
```go
func InstrumentGORM(db *gorm.DB) error {
	return db.Use(otelgorm.NewPlugin(otelgorm.WithoutMetrics()))
}
```

**Effect build shape to mirror** (`client.go:137-152`) — the egress `build` closure is the template; the DB hook builds the same struct with DB dimensions:
```go
Effect{
	Origin: origin, Operation: operation, UserID: userID,
	EffectKind: "egress",    // ← DB: "db_write" | "db_read"
	Host: host, Provider: provider,
	Target: host,            // ← DB: db.Statement.Table
	Status: status, BytesIn: bytesIn, BytesOut: bytesOut,
	DurationMS: ..., Requests: 1,   // ← DB also sets Rows: int(db.Statement.RowsAffected)
}
```

**Callback registration (from RESEARCH Pattern 1):** `cb.Create().After("gorm:create").Register(...)` for writes (always fact-rowed, D-01); `cb.Query().After("gorm:query").Register(...)` for reads (P95-gated via `gate.ShouldRecord(op, table, durMS)`, D-02). Timing via a Before/After callback pair stashing start in `Statement.Context`.

**CRITICAL GOTCHAs (verified, from RESEARCH Pitfalls 1 & 5):**
- The hook MUST issue **zero DB queries** — a query inside an after-callback resets `RowsAffected` to 0 (gorm issue #7044). Read `db.Statement.RowsAffected` directly, emit to the in-process sink only.
- The CH measure column is `row_count`, not `rows`. Go field `Rows`, JSON tag `row_count` (see producer extension below).
- `EffectKind` MUST be set explicitly to `db_write`/`db_read` — analytics defaults empty kind to `"egress"` (`effects.go` `if kind == ""`), so an unset kind mislabels the row.

---

### `libs/tracing/attribution.go` (NEW — resolver, transform) — D-08/D-09/D-11

**Analog:** `libs/tracing/baggage.go` (`ReadBaggage` + the lazy `resolveOperation` fallback chain — the new resolver sits *in front* of these as the new primary).

**Existing fallback chain to chain onto** (`baggage.go:57-65`) — the new resolver's `Resolve()` falls through to exactly this:
```go
func ReadBaggage(ctx context.Context) (origin, operation string) {
	bg := baggage.FromContext(ctx)
	origin = bg.Member(baggageKeyOrigin).Value()
	operation = bg.Member(baggageKeyOperation).Value()
	if operation == "" { operation = resolveOperation(ctx) }
	return origin, operation
}
```

**New resolver (from RESEARCH Pattern 2):** `CaptureOperationPCs(ctx)` runs SYNC on the record path — cheap `runtime.Callers(3, pcs[:])` PC capture only. `Operation.Resolve()` runs ASYNC on the Producer goroutine — `runtime.CallersFrames` symbol resolution (the expensive part, D-11), walking to the nearest frame whose func path contains `/internal/service/`, normalized to `catalog.UpdateAnimeInfo`. Fallback: service frame → `ReadBaggage` op → `originName(ctx)`. The `/internal/service/` substring is the robust anchor (all 7 services use `services/{name}/internal/service/...` per CLAUDE.md).

**PII guardrail (Security domain):** the new attribution path must NEVER seed `user_id` into baggage. `user_id` rides only the private ctx value (`baggage.go:120-133` `WithUserID`/`UserIDFromContext`). Add a test mirroring `TestNoUserIDOnOutboundWire`. `stripWireBaggagePII` (`client.go:29-35`) is the existing defense-in-depth.

---

### `libs/tracing/client.go` (EXTEND — RoundTripper retrofit) — D-08

**Analog:** itself, `recordingTransport.RoundTrip` (`client.go:108-171`).

**The retrofit:** today (`client.go:114-115`) egress takes `operation` from baggage as PRIMARY:
```go
origin, operation := ReadBaggage(ctx)   // ← coarse "catalog GET /api/anime/{id}"
```
D-08 switches this to stack-frame-primary so every effect in one trace shares one fine `operation`. Capture PCs here (sync) and let the async resolver fall back to `ReadBaggage` when no service frame is found. The `build` closure (`client.go:137-152`) keeps its shape; only the `operation` source changes. Note in plan: pre-Phase-3 egress rows keep the coarse op, post-Phase-3 get the fine op (append-only, no migration).

---

### `libs/tracing/effect.go` + `producer.go` (EXTEND — wire contract) — Runtime State Inventory

**Analog:** itself. The analytics RECEIVE side already has the fields; only the PRODUCER side is missing them.

**`Effect` struct extension** (`effect.go:8-24`) — add `TargetKind`, `Rows`, `AnimeID`:
```go
type Effect struct {
	Origin, Operation, UserID, EffectKind, Host, Provider, Target string
	TargetKind string // NEW: "host" | "table" | "key_class"  (post() must stop hard-coding "host")
	Status     int
	AnimeID    string // NEW
	BytesIn, BytesOut, DurationMS, Requests int
	Rows       int    // NEW: GORM RowsAffected → CH row_count
}
```

**`wireProducerEffect` drift to fix** (`producer.go:48-60`) — currently has NO `row_count` / `anime_id`. The analytics `wireEffect` (`handler/effects.go:38-51`) ALREADY has both (`AnimeID string \`json:"anime_id"\``, `RowCount int \`json:"row_count"\``). Add the two fields to `wireProducerEffect` to re-sync the contract.

**`post()` hard-coding to fix** (`producer.go:152-172`) — line 165 hard-codes `TargetKind: "host"` for EVERY effect. Change `post()` to use `e.TargetKind` (and map the new `Rows`/`AnimeID`):
```go
batch.Effects = append(batch.Effects, wireProducerEffect{
	..., TargetKind: "host",   // ← BUG for db/cache: use e.TargetKind
	// + RowCount: e.Rows, AnimeID: e.AnimeID,
})
```
No analytics-side change needed for these two fields — the receiver already accepts them. The ClickHouse column is `row_count` (`clickhouse_store.go:147` `uint32(e.RowCount)`), confirming the JSON tag.

---

### `infra/tempo/tempo.yaml` (EXTEND — config) — D-12 — NO ANALOG

Net-new `metrics_generator` block (span-metrics + service-graphs processors + `remote_write` to Prometheus + the per-tenant `overrides.defaults.metrics_generator.processors` enable). Full config in RESEARCH §Code Examples. The file today (read in full, 40 lines) has `server`/`distributor`/`ingester`/`compactor`/`storage`/`usage_report` — and NO generator block. **Pitfall 6:** the `overrides` enable list is REQUIRED or the generator silently no-ops. **Open Q1 (verify in plan):** Prometheus needs `--web.enable-remote-write-receiver` and the remote-write URL must match its `/prometheus` route-prefix. Grafana wiring goes in `docker/grafana/provisioning/datasources/datasources.yml` (add `serviceMap.datasourceUid` / `tracesToMetrics` on the Tempo datasource).

---

### `services/analytics` daily-P95 + `services/scheduler` cron (NEW + EXTEND) — D-03

**Analytics analog:** `clickhouse_store.go` (the only service with a `driver.Conn`). The daily P95 `quantile(0.95)(duration_ms)` query (RESEARCH §Code Examples) runs here. If exposed via HTTP, mirror `handler/effects.go` — `/internal/*` only (Docker-network, never gateway-proxied), 256KB body cap. Recommended distribution: a `read_thresholds` Redis hash the 7 services snapshot on a ticker (NOT a synchronous lookup in the Query callback — Pitfall 4).

**Scheduler analog:** `services/scheduler/internal/service/job.go:48-160` — each job is an `s.cron.AddFunc(cronExpr, func(){...})` block with the metrics-wrapped Run (`SchedulerJobExecutionsTotal` / `SchedulerJobDuration` / `SchedulerJobLastSuccess` + `lastXRun` timestamp). The job itself follows `jobs/cleanup.go:13-55` (`type XJob struct{db, cache, config, log}` + `NewXJob(...)` + `Run(ctx) error`). Add a `daily_read_threshold` job mirroring this exactly. Cron framework: `robfig/cron/v3` (already present).

---

## Shared Patterns

### Attribution (operation resolution) — D-08/D-09
**Source:** `libs/tracing/baggage.go` (`ReadBaggage` fallback chain) + new `attribution.go`.
**Apply to:** ALL effect kinds — egress (client.go retrofit), DB (gorm_effect.go), non-aggregated paths.
**Rule:** nearest `/internal/service/` stack frame → baggage op → origin name. Capture PCs sync, resolve async (D-11).

### Async batched drop-on-full transport — D-10
**Source:** `libs/tracing/producer.go` (`Producer.Record` lines 94-101 — drop-on-full, `effectsDropped.Inc()`).
**Apply to:** every new effect emitter (GORM hook, cache aggregator). NEVER block a request. The sink contract (`effect.go:29-31` `EffectSink.Record` MUST be non-blocking) is already satisfied by `Producer`.

### Lock-only-on-map + bounded map + deterministic clock — D-05
**Source:** `services/streaming/internal/service/hls_sessions.go` (mutex never held across IO; `evictIfFullLocked` oldest-eviction; `now func()` injectable clock; `flushIdle(now)`/`flushAll()` split).
**Apply to:** the cache aggregator (byte-for-byte clone).

### PII safety — Security domain (T-02-PII)
**Source:** `libs/tracing/baggage.go:120-133` (`WithUserID` private ctx) + `client.go:29-35` (`stripWireBaggagePII`).
**Apply to:** every new attribution/effect path. `user_id` rides private ctx only; `UserIDFromContext` is the only read path; never seed it into baggage.

### Boot wiring (per-service `main.go`)
**Source:** `catalog/cmd/catalog-api/main.go:68` (`gormtrace.InstrumentGORM(db.DB)`) + `streaming/cmd/streaming-api/main.go:82-91` (Producer `Start()`/`defer Stop()`, `SetGlobalSink`, `NewHLSSessions(...).Start()`/`defer Stop()`).
**Apply to:** the 7 gorm services (register effect callbacks after `InstrumentGORM`) + cache-using services (instantiate + `Start`/`Stop` the cache aggregator). **D-16: EXCLUDE `services/analytics`** — never wire the effect hook there (self-referential). The streaming main shows the exact graceful-shutdown ordering: flush the aggregator BEFORE the producer drains.

---

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `infra/tempo/tempo.yaml` `metrics_generator` block | config | — | Net-new Tempo config (D-12); no metrics_generator anywhere in the repo today. Config shape from official Grafana docs (RESEARCH §Code Examples). |

---

## Metadata

**Analog search scope:** `libs/tracing/`, `libs/tracing/gormtrace/`, `libs/cache/`, `services/streaming/internal/service/`, `services/analytics/internal/{repo,handler,domain}/`, `services/scheduler/internal/{service,jobs}/`, `services/catalog/cmd/`, `services/streaming/cmd/`, `infra/tempo/`
**Files scanned:** 13 (all read directly; no second reads)
**Pattern extraction date:** 2026-06-05
</content>
</invoke>
