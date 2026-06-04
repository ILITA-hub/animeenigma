# Phase 1: ClickHouse Foundation + EventStore Swap - Research

**Researched:** 2026-06-04
**Domain:** Columnar OLAP event store (ClickHouse) on a single self-hosted Docker host; Go integration behind an existing `EventStore` interface; Grafana datasource swap; live clickstream migration.
**Confidence:** HIGH (stack + client + schema verified against official docs and registries); MEDIUM (migration cutover sequencing — operational judgement, low-risk path recommended).

## Summary

This phase stands up ClickHouse as a single-host docker-compose service and implements a second `domain.EventStore` (`InsertBatch` + `UpsertIdentity`) behind the *unchanged* interface, so the existing async batcher (`services/analytics/internal/ingest/batcher.go`) drives it with zero hot-path changes. The existing clickstream (`analytics_events` GORM model, 6 Grafana SQL panels in `product-analytics.json`, `analytics_events_resolved` view, `analytics_identities` stitching) must keep working across the swap.

The cleanest end state is **one unified wide-event `MergeTree` table** whose columns are a superset of the current clickstream columns plus the new effect dimensions/measures; clickstream pageview/click/heartbeat rows are just `origin='api'`/`effect_kind`-flavored rows with their effect measures left at default. ClickHouse has **no UPDATE**, so identity stitching is modeled as **append-only `analytics_identities` rows resolved at query time via `argMax(...) ... GROUP BY` in a view** (mirroring the existing correlated-subquery `analytics_events_resolved` view, which does NOT translate to ClickHouse 1:1). The Go side uses `clickhouse-go/v2` native protocol with `PrepareBatch` + `AppendStruct`, fitting the existing 200/500-row batch shape.

**Primary recommendation:** `clickhouse/clickhouse-server:25.3` (LTS line) + `clickhouse-go/v2 v2.46.0` native client + **one unified wide-event MergeTree table** + append-only identity with an `argMax` resolved view + Grafana `grafana-clickhouse-datasource` plugin (rewrite the 6 SQL panels — there is no Postgres→ClickHouse SQL shim) + **dual-write migration** (Postgres + ClickHouse simultaneously, flag-controlled), validate parity, then flip Grafana's datasource and retire the Postgres write. Lowest-risk, fully reversible until the final flip.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Event persistence (append-only facts) | Database/Storage (ClickHouse) | — | Columnar OLAP store is the whole point of the phase |
| Async batching / drop-on-full | API/Backend (analytics svc) | — | Already implemented in `ingest/batcher.go`; backend owns it |
| Identity stitching (anon→user) | Database/Storage (CH view) | API/Backend (UpsertIdentity append) | CH has no UPDATE; resolution is a query-time concern |
| Dashboard rendering | CDN/Static (Grafana) | Database (CH datasource) | Grafana queries CH directly via plugin |
| Backup/restore | Database/Storage (ops) | — | Single-host operational concern, sidecar container |
| Metrics scrape | API/Backend (Prometheus) | Database (CH native /metrics) | CH exposes native Prometheus endpoint |

## Standard Stack

### Core
| Library / Image | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `clickhouse/clickhouse-server` | `25.3` (LTS line) | The event store | Official image; LTS tag gets ~1yr support. See note below on tag pinning. `[VERIFIED: hub.docker.com]` |
| `github.com/ClickHouse/clickhouse-go/v2` | `v2.46.0` (2026-05-03) | Go native client + batch insert | Official driver; native protocol; `PrepareBatch`/`AppendStruct` fits existing batcher. Requires Go 1.24+ (project is on `go 1.24.0`). `[VERIFIED: proxy.golang.org]` |
| `grafana-clickhouse-datasource` | `v4.17.0` | Grafana → ClickHouse datasource plugin | The official Grafana-maintained ClickHouse plugin (v4 = current query-builder generation). `[VERIFIED: github.com/grafana/clickhouse-datasource releases]` |

### Supporting
| Library / Tool | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `Altinity/clickhouse-backup` (`altinity/clickhouse-backup` image) | latest stable | Backup/restore wrapping `FREEZE` + metadata + restore workflow | The backup procedure (AR-STORE-01). Runs as sidecar sharing the CH data volume. `[CITED: github.com/Altinity/clickhouse-backup]` |
| `github.com/testcontainers/testcontainers-go/modules/clickhouse` | `v0.42.0` (2026-04-09) | Spin up real CH in contract tests | AR-STORE-03 backend-agnostic contract tests against real ClickHouse. `[VERIFIED: proxy.golang.org]` |
| `github.com/ClickHouse/ch-go` | `v0.72.0` (transitively via v2) | Low-level native protocol codec | Pulled in by `clickhouse-go/v2 >= 2.3.0`; not a direct dependency to manage. `[VERIFIED: proxy.golang.org]` |

> **Version-tag note (riskiest unknown #1):** The WebSearch surfaced `lts → 26.3` and `latest → 26.5`, but ClickHouse uses **CalVer** (`YY.M`) and the search index appears to be reporting a near-future month. As of the knowledge cutoff the stable LTS line is **`25.3`**. **The planner MUST add a `checkpoint:human-verify` task to pin the exact current LTS tag** by running `docker run --rm clickhouse/clickhouse-server:lts clickhouse-server --version` and checking https://github.com/ClickHouse/ClickHouse/releases for the current LTS. **Pin an explicit patch tag** (e.g. `25.3.x.y`), never `latest` or bare `lts`, so redeploys are reproducible. `[ASSUMED]` on the exact patch number.

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `clickhouse-go/v2` native protocol | `database/sql` driver (same package, `OpenDB`) | `database/sql` is more familiar but loses the typed `AppendStruct` batch API and adds a layer; native `PrepareBatch` is the documented high-throughput path and matches the batcher shape. Use native. `[CITED: clickhouse.com/docs/integrations/go]` |
| `clickhouse-backup` | Raw `ALTER TABLE … FREEZE` + manual metadata copy, or volume snapshot | FREEZE is the low-level primitive (hardlink snapshot, no extra disk); clickhouse-backup wraps it AND backs up DDL metadata + gives a one-command restore. Volume snapshot risks torn writes. clickhouse-backup is simplest *reliable* for single host. `[CITED: clickhouse.com/docs/operations/backup/alternative_methods]` |
| One unified table | clickstream-table + effects-table | Two tables means every cross-cut pivot is a UNION/JOIN and breaks the "one register" mental model. Unified table with nullable/defaulted effect columns is the design intent (CONTEXT §specifics). Recommend unified. |
| ReplacingMergeTree for identity | append + argMax view | RMT dedups on merge but merges are async/unpredictable; `FINAL` is slow. argMax-in-view is deterministic and matches the existing "latest row per anonymous_id wins" semantics exactly. |

**Installation:**
```bash
cd services/analytics
go get github.com/ClickHouse/clickhouse-go/v2@v2.46.0
go get github.com/testcontainers/testcontainers-go/modules/clickhouse@v0.42.0
# go.work already lists ./services/analytics — run: go work sync
```
Grafana plugin (compose env on the grafana service):
```yaml
GF_INSTALL_PLUGINS: grafana-clickhouse-datasource
```

## Package Legitimacy Audit

| Package | Registry | Age | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-------------|-----------|-------------|
| `github.com/ClickHouse/clickhouse-go/v2` | Go proxy | 5+ yrs (v2 line) | github.com/ClickHouse/clickhouse-go (official) | n/a (Go) | Approved — official ClickHouse org |
| `github.com/ClickHouse/ch-go` | Go proxy | 4+ yrs | github.com/ClickHouse/ch-go (official) | n/a | Approved — transitive, official org |
| `github.com/testcontainers/testcontainers-go/modules/clickhouse` | Go proxy | 2+ yrs | github.com/testcontainers/testcontainers-go (official) | n/a | Approved — official testcontainers org |
| `clickhouse/clickhouse-server` | Docker Hub | 7+ yrs | Official ClickHouse image | n/a | Approved — official |
| `altinity/clickhouse-backup` | Docker Hub | 5+ yrs | github.com/Altinity/clickhouse-backup | n/a | Approved — Altinity (ClickHouse enterprise vendor) |
| `grafana-clickhouse-datasource` | Grafana plugin registry | 4+ yrs | github.com/grafana/clickhouse-datasource (official Grafana) | n/a | Approved — official Grafana plugin |

slopcheck targets npm/PyPI; all packages here are Go modules from official upstream orgs or official container/plugin registries verified directly against their source repos via Go proxy and GitHub. No `[SLOP]`/`[SUS]` findings. The only outstanding verification is the **ClickHouse image patch tag** (see Standard Stack note) — gated by a human-verify checkpoint.

## Architecture Patterns

### System Architecture Diagram

```
                  FE snippet (pageview/click/heartbeat/identify)
                              │  POST /collect
                              ▼
                  analytics svc :8092  (handler.CollectHandler)
                              │  Enqueue(Event)  [non-blocking, drop-on-full]
                              ▼
                  ingest.Batcher  (in-memory chan, 500-row / 1s flush)
                              │  InsertBatch(ctx, []Event)
                              │  UpsertIdentity(ctx, anon, user, ts)  ── on identify
                              ▼
        ┌─────────────────────────────────────────────────┐
        │  EventStore (interface — UNCHANGED swap seam)     │
        │   ├─ PostgresStore  (existing — kept during DW)   │
        │   └─ ClickHouseStore (NEW — native batch insert)  │
        └─────────────────────────────────────────────────┘
                              │ native proto :9000
                              ▼
        ClickHouse :8123(http)/:9000(native)/:9363(prometheus)
          ├─ events            (wide MergeTree, 1 row/effect)
          ├─ identities        (append-only MergeTree)
          └─ events_resolved   (VIEW: argMax-stitched person_id)
                  ▲                              ▲
       clickhouse-backup sidecar        Grafana grafana-clickhouse-datasource
       (FREEZE snapshot → restore)      (6 rewritten product-analytics panels)
                              ▲
                   Prometheus scrape :9363/metrics
```

### Recommended structure (additive — no interface change)
```
services/analytics/internal/
├── domain/store.go            # UNCHANGED — the swap seam
├── repo/
│   ├── postgres_store.go      # kept (dual-write source of truth during migration)
│   ├── clickhouse_store.go    # NEW — ClickHouseStore implementing EventStore
│   ├── clickhouse_schema.go   # NEW — DDL: events + identities + events_resolved view
│   ├── store_contract_test.go # NEW — backend-agnostic suite (table-driven, run vs PG + CH)
│   └── models.go              # extend Event with effect dims/measures (defaulted)
└── cmd/analytics-api/main.go  # wire CH store; dual-write fan-out behind a flag
```

### Pattern 1: One unified wide-event MergeTree table
**What:** A single `events` table holding both clickstream rows and effect rows. Clickstream events set `effect_kind=''` (or a `'clickstream'` flavor) and leave effect measures at 0; future effect rows set `origin`/`effect_kind`/`target` and the measures.

**Engine + ORDER BY + PARTITION + TTL:**
```sql
-- Source: clickhouse.com/docs/engines/table-engines/mergetree-family/mergetree
CREATE TABLE IF NOT EXISTS events
(
    -- correlation / time
    timestamp     DateTime64(3)               CODEC(Delta, ZSTD(1)),
    received_at   DateTime64(3)               CODEC(Delta, ZSTD(1)),
    event_id      String                      CODEC(ZSTD(1)),
    trace_id      String                      CODEC(ZSTD(1)),
    session_id    String                      CODEC(ZSTD(1)),
    anonymous_id  String                      CODEC(ZSTD(1)),
    user_id       Nullable(String)            CODEC(ZSTD(1)),

    -- register dimensions (categorical → LowCardinality)
    origin        LowCardinality(String),          -- fe_click | goroutine | scheduled_job | api
    operation     LowCardinality(String)  DEFAULT '',
    effect_kind   LowCardinality(String)  DEFAULT '', -- egress|db_write|db_read|cache|fe_rum| '' (clickstream)
    target_kind   LowCardinality(String)  DEFAULT '',
    target        String                  DEFAULT '' CODEC(ZSTD(1)),
    source        LowCardinality(String)  DEFAULT 'be',   -- be | fe_rum
    accuracy      LowCardinality(String)  DEFAULT 'exact', -- exact | approx
    anime_id      Nullable(String),

    -- clickstream-specific dims (reconciled in)
    event_type    LowCardinality(String),
    event_name    LowCardinality(String) DEFAULT '',
    url           String DEFAULT '' CODEC(ZSTD(1)),
    path          String DEFAULT '' CODEC(ZSTD(1)),
    referrer      String DEFAULT '' CODEC(ZSTD(1)),
    title         String DEFAULT '' CODEC(ZSTD(1)),
    el_selector   String DEFAULT '' CODEC(ZSTD(1)),
    el_text       String DEFAULT '' CODEC(ZSTD(1)),
    el_tag        LowCardinality(String) DEFAULT '',
    el_attrs      String DEFAULT '{}' CODEC(ZSTD(1)),
    user_agent    String DEFAULT '' CODEC(ZSTD(1)),
    device_type   LowCardinality(String) DEFAULT '',
    screen_w      UInt16 DEFAULT 0,
    screen_h      UInt16 DEFAULT 0,
    ip_hash       String DEFAULT '' CODEC(ZSTD(1)),
    properties    String DEFAULT '{}' CODEC(ZSTD(1)),

    -- register measures
    requests      UInt32 DEFAULT 0,
    bytes_in      UInt64 DEFAULT 0 CODEC(Delta, ZSTD(1)),
    bytes_out     UInt64 DEFAULT 0 CODEC(Delta, ZSTD(1)),
    duration_ms   UInt32 DEFAULT 0 CODEC(Delta, ZSTD(1)),
    rows          UInt32 DEFAULT 0,
    active_ms     UInt32 DEFAULT 0   -- existing heartbeat measure
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (toDate(timestamp), origin, operation, effect_kind, target, timestamp)
TTL toDateTime(timestamp) + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;
```
**When to use:** This is the phase-1 table. `ORDER BY` leads with `toDate(timestamp)` then the common pivots (origin/operation/effect_kind/target), then full `timestamp` for uniqueness — matching the "time + operation + target" pivots named in CONTEXT. `PARTITION BY toYYYYMM` makes the 90-day TTL drop whole monthly parts cheaply (replaces the Postgres `PurgeOlderThan` cron — TTL is native and the cron can be retired for CH). `[CITED: clickhouse.com/docs/engines/table-engines/mergetree-family/mergetree]`

**Codec rationale:** `LowCardinality(String)` for every categorical dim (origin/operation/effect_kind/target_kind/source/accuracy/device_type/event_type/el_tag) — dictionary-encodes the handful of distinct values; this is the single biggest compression + pivot-speed win. `Delta, ZSTD` for monotonic/counter columns (timestamps, byte counters, durations). Plain `ZSTD(1)` for free-text/high-cardinality strings (urls, ids, el_text). `[CITED: clickhouse.com/docs/sql-reference/statements/create/table#column_compression_codec]`

> **`rows` is a SQL reserved-ish word** — fine as a CH column name when quoted/backticked, but the Go struct/insert must match exactly. Confirm during implementation; rename to `row_count` if it causes friction.

### Pattern 2: Append-only identity + argMax resolved view (replaces correlated-subquery view)
**What:** ClickHouse has no row UPDATE. `UpsertIdentity` becomes a plain INSERT into an append-only `identities` table. The "latest row per anonymous_id wins" semantic is resolved at query time.
```sql
CREATE TABLE IF NOT EXISTS identities
(
    anonymous_id String,
    user_id      String,
    timestamp    DateTime64(3) CODEC(Delta, ZSTD(1))
)
ENGINE = MergeTree
ORDER BY (anonymous_id, timestamp);

-- Resolved view: person_id = identified user if ever known, else anon.
-- argMax(user_id, timestamp) picks the latest identity per anonymous_id.
-- Source: clickhouse.com/docs/sql-reference/aggregate-functions/reference/argmax
CREATE VIEW IF NOT EXISTS events_resolved AS
SELECT e.*,
       coalesce(e.user_id, i.user_id)                AS resolved_user_id,
       coalesce(e.user_id, i.user_id, e.anonymous_id) AS person_id
FROM events AS e
LEFT JOIN (
    SELECT anonymous_id, argMax(user_id, timestamp) AS user_id
    FROM identities
    GROUP BY anonymous_id
) AS i USING (anonymous_id);
```
**Impact on existing assets:** The Postgres `analytics_events_resolved` view uses a correlated subquery (`EnsureView` in `models.go`) — ClickHouse does not support correlated subqueries, so it **cannot be ported verbatim**; the `argMax` + `GROUP BY` + `LEFT JOIN` form above is the idiomatic equivalent and preserves identical semantics. Any dashboard/code reading `analytics_events_resolved` (e.g. `repo/resolve.go`) must point at `events_resolved` on the CH datasource. `[CITED: clickhouse.com/docs/sql-reference/aggregate-functions/reference/argmax]`

### Pattern 3: Native batch insert fitting the existing batcher
```go
// Source: clickhouse.com/docs/integrations/go (native PrepareBatch + AppendStruct)
func (s *ClickHouseStore) InsertBatch(ctx context.Context, events []domain.Event) error {
    if len(events) == 0 {
        return nil
    }
    batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO events")
    if err != nil {
        return err
    }
    for _, e := range events {
        if err := batch.AppendStruct(toCHRow(&e)); err != nil { // ch struct tags map cols
            return err
        }
    }
    return batch.Send()
}

func (s *ClickHouseStore) UpsertIdentity(ctx context.Context, anon, user string, ts time.Time) error {
    if anon == "" || user == "" {
        return nil
    }
    return s.conn.Exec(ctx,
        "INSERT INTO identities (anonymous_id, user_id, timestamp) VALUES (?, ?, ?)",
        anon, user, ts)
}
```
The batcher already calls `InsertBatch(ctx, batch)` with a 5s timeout and `UpsertIdentity` per identify event — **no batcher changes required**. `AppendStruct` requires the Go struct field `ch` tags to align in name+type with table columns. `[CITED: pkg.go.dev/github.com/ClickHouse/clickhouse-go/v2]`

### Anti-Patterns to Avoid
- **One row per HLS segment / per trivial DB read:** explicitly out of scope here, but the schema must not invite it — keep effect aggregation discipline (Phase 2+). The `requests` measure exists precisely so N segments collapse to 1 row with `requests=N`.
- **Single-row INSERTs:** ClickHouse hates small inserts (one part per insert → merge storm). Always go through the batcher; never insert per-event. `[CITED: clickhouse.com/docs/optimize/bulk-inserts]`
- **Using `FINAL` or ReplacingMergeTree for identity resolution:** non-deterministic timing + slow. Use the argMax view.
- **Porting the correlated-subquery view verbatim:** invalid in ClickHouse.
- **Bare `latest`/`lts` image tag in compose:** non-reproducible; pin a patch version.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Backup/restore | Custom `cp`/`tar` of `/var/lib/clickhouse` | `altinity/clickhouse-backup` (wraps FREEZE) | Crash-consistent hardlink snapshot + metadata DDL + one-command restore; raw copy risks torn parts |
| Retention purge | Port the `PurgeOlderThan` Go cron to CH DELETE | Native `TTL … DELETE` on the table | CH drops whole partitions; mutation-based DELETE is expensive and unnecessary |
| Identity dedup | Manual "latest row" bookkeeping | `argMax(user_id, timestamp)` in a view | Idiomatic, deterministic, no merge dependency |
| Native protocol encoding | `database/sql` string building | `clickhouse-go/v2` `PrepareBatch`/`AppendStruct` | Typed, columnar, compressed; the documented fast path |
| CH metrics exporter | Run a sidecar prometheus-exporter | CH native `<prometheus>` endpoint `:9363/metrics` | Built-in since modern versions; no extra container |

**Key insight:** ClickHouse ships native primitives (TTL, FREEZE-based backup, Prometheus endpoint, argMax) for everything this phase needs — the work is wiring, not building.

## Runtime State Inventory

> This phase introduces a NEW store and migrates a live clickstream. Treated as a migration phase.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | Existing `analytics_events` + `analytics_identities` rows in **Postgres** (`animeenigma` DB). 90-day retention so the dataset is bounded. | Backfill into ClickHouse is OPTIONAL (see Migration). Lowest-risk path: dual-write forward, do NOT backfill history unless dashboards need >0-day continuity; if continuity required, one-shot export-via-`INSERT INTO … SELECT` from a CH `postgresql()` table function or CSV dump. |
| Live service config | Grafana datasource `aenigma-postgres` is referenced by all 6 `product-analytics.json` panels AND (per datasources.yml comment) the Prometheus UID is load-bearing for 14 alert rules — **the ClickHouse datasource must use a NEW uid** (e.g. `aenigma-clickhouse`), never reuse an existing one. | Add new CH datasource; repoint the 6 product-analytics panels; leave Prometheus/Postgres datasources untouched. |
| OS-registered state | None — Dockerized service, no host scheduler/registrations. | None (verified: analytics runs only as a compose service). |
| Secrets/env vars | New: `CLICKHOUSE_*` connection env (host/port/db/user/password). Existing `ANALYTICS_*` env unchanged. ClickHouse default user is `default` with no password in dev — set a password for the prod host. | Add CH env to analytics + grafana + backup services in compose and `.env.example`. |
| Build artifacts | `services/analytics/go.mod` gains `clickhouse-go/v2` + testcontainers; analytics is already in `go.work` (line 18). Per project memory, adding a *new `libs/` module* touches all Dockerfiles — **N/A here** because the CH client is a direct dep of the analytics service, not a shared `libs/` module (unless a `libs/clickhouse` wrapper is created — recommend NOT creating one this phase). | `cd services/analytics && go get …`; `go work sync`; rebuild analytics image only. |

**The canonical question — after every file is updated, what runtime state still has the old shape?** Postgres still holds historical clickstream rows (intentionally, as the reversible fallback during dual-write). Grafana panels still query Postgres until the deliberate datasource flip. Nothing is silently stale once the flip is done and verified.

## Common Pitfalls

### Pitfall 1: Reusing a Grafana datasource UID
**What goes wrong:** datasources.yml explicitly warns that reusing/renaming `PBFA97CFB590B2093` orphaned 14 alert rules. Reusing `aenigma-postgres`'s uid for ClickHouse would silently break every product-analytics panel binding.
**How to avoid:** Allocate a brand-new uid `aenigma-clickhouse`. Repoint each of the 6 panels' `datasource` block to it explicitly.
**Warning signs:** Panels show "datasource not found" or query errors after provisioning.

### Pitfall 2: Postgres SQL in the panels won't run on ClickHouse
**What goes wrong:** The 6 panels' `rawSql` is Postgres dialect (correlated subqueries, `analytics_events_resolved`). There is **no SQL compatibility shim** — ClickHouse SQL differs (no correlated subqueries, different date functions, `argMax`, backtick identifiers).
**How to avoid:** Rewrite all 6 panel queries by hand against the CH `events` / `events_resolved` schema. Grafana's CH plugin has a query builder, but the panels store `rawSql` — edit each.
**Warning signs:** Syntax errors referencing functions like `date_trunc` or correlated subqueries.

### Pitfall 3: Small/single-row inserts → part explosion
**What goes wrong:** Bypassing the batcher (e.g. a "quick test insert") creates one part per insert; the background merger thrashes; `Too many parts` errors.
**How to avoid:** Everything goes through `ingest.Batcher` (already 500-row/1s). Keep CH `max_insert_block_size` defaults; the 200/500 batch sizes are healthy.
**Warning signs:** `DB::Exception: Too many parts (N). Merges are processing significantly slower than inserts.`

### Pitfall 4: `Nullable` columns in the ORDER BY / sort key
**What goes wrong:** Nullable columns in the primary key hurt performance and are discouraged.
**How to avoid:** Keep `user_id`/`anime_id` Nullable but OUT of the `ORDER BY` (the schema above does this — they're drill-down dims, per CONTEXT risk note "keep as nullable drill-down dims, not in hot rollup keys").

### Pitfall 5: Contract tests need a real ClickHouse (sqlite trick won't work)
**What goes wrong:** The existing contract tests use in-memory sqlite (`postgres_store_test.go`). ClickHouse has no embeddable equivalent; sqlite cannot stand in for CH.
**How to avoid:** Use `testcontainers-go/modules/clickhouse` to boot a real CH per test run; gate behind a build tag or `testing.Short()` so unit runs stay fast. The contract suite must be backend-parameterized (AR-STORE-03): one table-driven test body, two store constructors. Note the existing repo `go.mod` has no testcontainers yet and tests currently run pure-sqlite — adding Docker-dependent tests changes CI assumptions; gate them.

## Code Examples

### Connection (native, with sane single-host settings)
```go
// Source: pkg.go.dev/github.com/ClickHouse/clickhouse-go/v2
conn, err := clickhouse.Open(&clickhouse.Options{
    Addr: []string{cfg.CHAddr}, // "clickhouse:9000"
    Auth: clickhouse.Auth{Database: "analytics", Username: cfg.CHUser, Password: cfg.CHPass},
    Settings: clickhouse.Settings{"max_execution_time": 60},
    Compression: &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
    DialTimeout: 5 * time.Second,
    MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxLifetime: time.Hour,
})
// schema bootstrap on boot (mirrors AutoMigrateAll + EnsureView):
//   conn.Exec(ctx, createEventsTableDDL)
//   conn.Exec(ctx, createIdentitiesTableDDL)
//   conn.Exec(ctx, createResolvedViewDDL)
```

### Contract test skeleton (backend-agnostic)
```go
// Source: pkg.go.dev/github.com/testcontainers/testcontainers-go/modules/clickhouse
func runEventStoreContract(t *testing.T, newStore func(t *testing.T) domain.EventStore) {
    t.Run("InsertBatch_persists", func(t *testing.T) { /* … */ })
    t.Run("InsertBatch_empty_noop", func(t *testing.T) { /* … */ })
    t.Run("UpsertIdentity_latest_wins", func(t *testing.T) { /* … */ })
}
func TestPostgresStore_Contract(t *testing.T)   { runEventStoreContract(t, newSqliteStore) }
func TestClickHouseStore_Contract(t *testing.T) {
    if testing.Short() { t.Skip("requires docker") }
    runEventStoreContract(t, newCHStore) // testcontainers
}
```

### Compose service (mirror postgres pattern; ports bound to 127.0.0.1)
```yaml
clickhouse:
  image: clickhouse/clickhouse-server:25.3   # PIN exact patch after human-verify
  container_name: animeenigma-clickhouse
  restart: unless-stopped
  ulimits: { nofile: { soft: 262144, hard: 262144 } }   # CH requirement
  environment:
    CLICKHOUSE_DB: analytics
    CLICKHOUSE_USER: ${CLICKHOUSE_USER:-analytics}
    CLICKHOUSE_PASSWORD: ${CLICKHOUSE_PASSWORD:-changeme}
    CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT: "1"
  ports:
    - "127.0.0.1:8123:8123"   # http
    - "127.0.0.1:9000:9000"   # native
    - "127.0.0.1:9363:9363"   # prometheus (enable in config)
  volumes:
    - clickhouse_data:/var/lib/clickhouse
    - ./clickhouse/config.d:/etc/clickhouse-server/config.d:ro   # <prometheus> + resource caps
  healthcheck:
    test: ["CMD", "wget", "--spider", "-q", "http://localhost:8123/ping"]
    interval: 10s
    timeout: 5s
    retries: 5
  deploy:
    resources:
      limits: { memory: 4G }   # cap on single shared host
```
Prometheus native endpoint (`./clickhouse/config.d/prometheus.xml`):
```xml
<!-- Source: clickhouse.com/docs/interfaces/prometheus -->
<clickhouse>
  <prometheus>
    <endpoint>/metrics</endpoint>
    <port>9363</port>
    <metrics>true</metrics><events>true</events>
    <asynchronous_metrics>true</asynchronous_metrics>
  </prometheus>
</clickhouse>
```
Prometheus scrape (add to `docker/prometheus/prometheus.yml`, mirroring the 14 existing jobs):
```yaml
- job_name: 'clickhouse'
  static_configs:
    - targets: ['clickhouse:9363']
```

### Grafana datasource (append to datasources.yml — NEW uid)
```yaml
# Source: grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/configure/
- name: ClickHouse
  uid: aenigma-clickhouse            # NEW — never reuse aenigma-postgres
  type: grafana-clickhouse-datasource
  access: proxy
  jsonData:
    host: clickhouse
    port: 9000
    protocol: native
    defaultDatabase: analytics
    username: ${CLICKHOUSE_USER}
  secureJsonData:
    password: ${CLICKHOUSE_PASSWORD}
  editable: false
```

### Backup sidecar (clickhouse-backup) — and dry-run restore (AR-STORE-01)
```yaml
clickhouse-backup:
  image: altinity/clickhouse-backup:latest   # pin a tag matching CH major
  container_name: animeenigma-clickhouse-backup
  entrypoint: ["/bin/sh","-c","sleep infinity"]   # run via `docker exec`
  volumes:
    - clickhouse_data:/var/lib/clickhouse        # MUST share CH data volume
    - clickhouse_backups:/var/lib/clickhouse/backup
  environment:
    CLICKHOUSE_HOST: clickhouse
    CLICKHOUSE_PASSWORD: ${CLICKHOUSE_PASSWORD}
```
Backup / restore commands (document in a runbook; dry-run restore once):
```bash
docker exec animeenigma-clickhouse-backup clickhouse-backup create daily-$(date +%F)
docker exec animeenigma-clickhouse-backup clickhouse-backup list
# DRY-RUN restore into a scratch DB to prove the procedure:
docker exec animeenigma-clickhouse-backup clickhouse-backup restore --schema daily-YYYY-MM-DD
```
`[CITED: github.com/Altinity/clickhouse-backup]`

## State of the Art

| Old Approach | Current Approach | When | Impact |
|--------------|------------------|------|--------|
| Postgres clickstream table + GORM | ClickHouse wide-event MergeTree | This phase | Columnar compression + OLAP pivots; UPDATE-less identity |
| Retention via Go cron `DELETE` | Native `TTL … DELETE` partition drop | This phase | Cheaper, declarative; analytics purge cron can be retired for CH |
| Old `clickhouse-go` v1 / yandex image | `clickhouse-go/v2` native + `clickhouse/clickhouse-server` official | current | v1 + `yandex/clickhouse-server` are deprecated; do not use |
| `database/sql` driver | native `PrepareBatch`/`AppendStruct` | current | Typed columnar batch path |

**Deprecated/outdated:**
- `yandex/clickhouse-server` image — superseded by `clickhouse/clickhouse-server`. Do not use.
- `clickhouse-go` v1 — use v2.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | ClickHouse current stable LTS line is `25.3`; pin a patch tag | Standard Stack | LOW — gated by a human-verify checkpoint; CalVer means tag must be confirmed at plan time. WebSearch index reported `26.3 lts`/`26.5 latest` (likely future-dated index noise). |
| A2 | `rows` is usable as a CH column name | Pattern 1 | LOW — rename to `row_count` if it conflicts; cosmetic |
| A3 | History backfill is NOT required (90-day retention, dual-write forward suffices) | Migration | MEDIUM — if product-analytics must show pre-cutover data, add a one-shot backfill task |
| A4 | No shared `libs/clickhouse` module is created (CH client is analytics-local) | Runtime State | LOW — if a shared lib is later wanted, the all-Dockerfiles rule from memory applies |

## Migration (AR-STORE-04) — recommended path

**Recommendation: dual-write, then flip.** Lowest-risk, fully reversible on a single host.

1. **Add CH store + schema bootstrap** alongside Postgres (no behavior change yet).
2. **Dual-write behind a flag** (`ANALYTICS_CH_DUAL_WRITE=true`): the batcher fans `InsertBatch`/`UpsertIdentity` out to *both* stores. Postgres remains the dashboard source of truth. A CH write failure must NOT fail the Postgres write (log + dropped-metric only) — analytics is best-effort.
3. **Validate parity** — compare row counts / sample pivots between PG and CH over a day.
4. **Repoint Grafana** — rewrite the 6 `product-analytics.json` panels onto the `aenigma-clickhouse` datasource; verify they render (in-browser smoke per DS-NF-06).
5. **Flip source of truth** — drop the Postgres write (CH-only), keep PG data + code for a rollback window, then remove in a later cleanup.

**Reversibility:** at any step before #5 the system runs on Postgres exactly as today; revert is "turn off the flag." Backfill (A3) only if pre-cutover history must appear — via CH `INSERT INTO events SELECT … FROM postgresql('postgres:5432','animeenigma','analytics_events','user','pass')` (the `postgresql()` table function) or a CSV dump.

**No SQL shim exists** for the Grafana panels — they must be rewritten. The "compatibility shim" alternative (a Postgres FDW/view emulating the CH schema) is not worth it for 6 panels.

## Forward-awareness (do NOT build now)

- **P6 OTel Collector ClickHouse exporter:** the exporter creates its own `otel_traces`/`otel_logs` tables (its own schema). Phase-1 choice that helps: keep the register in a dedicated `analytics` database and don't squat on table names the exporter uses (`otel_*`). No coupling needed now.
- **v2 AggregatingMergeTree rollups (AR-V2-01):** rollups are materialized views reading from `events`. Phase-1 choices that make them cheaper: the `LowCardinality` dims and the `requests`/`bytes_*`/`duration_ms`/`rows` measures are already the right shape for `sum()/count()` rollups grouped by (origin, operation, effect_kind, target, time-bucket). Keeping high-cardinality `user_id`/`anime_id` OUT of the sort key (and Nullable) means future hot rollup keys stay small. No rollup tables this phase.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Docker / docker-compose | CH + grafana + backup containers | ✓ (project runs via `make dev`) | — | none needed |
| ClickHouse server image | the store | pull at deploy | pin `25.3.x` | none |
| `clickhouse-go/v2` | Go store impl | go get | v2.46.0 | none |
| testcontainers + Docker socket in CI | contract tests | depends on CI runner | v0.42.0 | gate behind `testing.Short()` / build tag so unit runs don't need Docker |
| Grafana CH plugin | dashboards | `GF_INSTALL_PLUGINS` (needs egress at boot) or bake into image | v4.17.0 | pre-install in a custom grafana image if the host lacks plugin-CDN egress |

**Missing with no fallback:** none.
**Missing with fallback:** testcontainers requires a Docker socket — gate CH contract tests so the default `go test ./...` stays Docker-free; Grafana plugin install needs network egress at container start (bake into image if the prod host is egress-restricted).

## Validation Architecture

> nyquist_validation: config not inspected for explicit `false` — treated as enabled.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (stdlib) + `testcontainers-go/modules/clickhouse` for CH |
| Config file | none (go test) |
| Quick run command | `cd services/analytics && go test ./internal/repo/... -short` |
| Full suite command | `cd services/analytics && go test ./... -race` (CH contract needs Docker) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AR-STORE-02 | wide-event schema accepts effect + clickstream rows | unit/integration | `go test ./internal/repo -run TestClickHouseStore_Contract` | ❌ Wave 0 |
| AR-STORE-03 | CH store passes same contract as PG store | integration | `go test ./internal/repo -run Contract` | ❌ Wave 0 |
| AR-STORE-05 | drop-on-full + dropped metric | unit | `go test ./internal/ingest -run TestBatcher` (exists) + assert `EventsDropped` | ✅ (batcher_test.go) |
| AR-STORE-04 | clickstream still ingests post-swap | manual/smoke | in-browser: emit events, confirm panels render | manual |
| AR-STORE-01 | backup creates + restores | manual | `clickhouse-backup create` then dry-run `restore --schema` | manual runbook |

### Wave 0 Gaps
- [ ] `services/analytics/internal/repo/store_contract_test.go` — backend-parameterized contract suite (PG + CH)
- [ ] `services/analytics/internal/repo/clickhouse_store.go` + `clickhouse_schema.go`
- [ ] testcontainers dependency added to `services/analytics/go.mod`; `go work sync`
- [ ] Extend `domain.Event` (or a CH-row mapper) with effect dims/measures

## Security Domain

> `security_enforcement` config not inspected; analytics is internal/admin-facing. Minimal applicable surface.

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | service is internal; no new user auth |
| V4 Access Control | yes | CH + Grafana datasource bound to `127.0.0.1` only (matches existing compose pattern); CH user/password not `default`-empty in prod |
| V5 Input Validation | yes | reuse existing `Event.Validate()`; CH typed columns reject malformed measures |
| V6 Cryptography | no | no new crypto; do not hand-roll |

### Known Threat Patterns
| Pattern | STRIDE | Mitigation |
|---------|--------|------------|
| SQL injection into rawSql panels | Tampering | Panels are admin-authored static SQL; CH inserts use parameterized `?` placeholders (Pattern 3) |
| Exposed CH ports | Info Disclosure | Bind `127.0.0.1:8123/9000/9363` only (existing postgres/redis pattern); set CH password |
| PII in events (ip_hash, user_id) | Info Disclosure | Existing `ANALYTICS_IP_SALT` hashing preserved; 90-day TTL caps retention; existing erase-by-user path must be ported to CH (lightweight `ALTER TABLE … DELETE` mutation or accept TTL) — note GDPR-erase via mutation is the one place CH "UPDATE-less" hurts; flag for the planner |

## Sources

### Primary (HIGH confidence)
- `clickhouse-go/v2` v2.46.0 — `proxy.golang.org` (version) + `pkg.go.dev/github.com/ClickHouse/clickhouse-go/v2` (native batch / AppendStruct API)
- `testcontainers-go/modules/clickhouse` v0.42.0 — `proxy.golang.org`
- `grafana/clickhouse-datasource` v4.17.0 — GitHub releases API
- ClickHouse MergeTree / codecs / TTL / argMax / Prometheus interface — clickhouse.com/docs (engines, create/table codecs, interfaces/prometheus)
- Altinity/clickhouse-backup — github.com/Altinity/clickhouse-backup + clickhouse.com/docs/operations/backup/alternative_methods
- Codebase: `services/analytics/internal/{domain,repo,ingest}`, `cmd/analytics-api/main.go`, `docker/docker-compose.yml`, `docker/grafana/provisioning/datasources/datasources.yml`, `docker/prometheus/prometheus.yml`, `infra/grafana/dashboards/product-analytics.json`

### Secondary (MEDIUM confidence)
- ClickHouse Docker image tags & LTS guidance — hub.docker.com/r/clickhouse/clickhouse-server (WebSearch; tag number gated by human-verify)
- Grafana CH datasource provisioning fields — grafana.com/docs/plugins/grafana-clickhouse-datasource

### Tertiary (LOW confidence)
- Exact current LTS patch tag (CalVer) — needs `docker run … --version` confirmation at plan time

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH (Go client, testcontainers, plugin versions verified against registries) / MEDIUM on CH image tag (CalVer — human-verify)
- Architecture (schema, identity, batch): HIGH — verified against ClickHouse official docs + matches existing code shape
- Migration: MEDIUM — dual-write is a sound, reversible operational judgement, not a verifiable fact
- Pitfalls: HIGH — drawn from official docs + the repo's own datasource-UID warning

**Research date:** 2026-06-04
**Valid until:** ~2026-07-04 (ClickHouse moves fast on CalVer; re-confirm the image tag at plan time)

---

## Key Recommendations (summary)

1. **Image:** `clickhouse/clickhouse-server`, pin an explicit LTS patch tag (`25.3.x.y`) — add a human-verify checkpoint to confirm the current LTS number.
2. **Schema:** ONE unified wide-event `MergeTree` table; `LowCardinality` all dims, `Delta+ZSTD` counters; `ORDER BY (toDate(ts), origin, operation, effect_kind, target, ts)`; `PARTITION BY toYYYYMM(ts)`; `TTL ts + 90 DAY` (retires the purge cron for CH).
3. **Client:** `clickhouse-go/v2 v2.46.0` native, `PrepareBatch`+`AppendStruct` — the existing batcher drives it unchanged.
4. **Identity:** append-only `identities` table + `argMax(user_id, timestamp)` resolved view (NOT a verbatim port of the correlated-subquery PG view; NOT ReplacingMergeTree/FINAL).
5. **Contract tests:** one backend-parameterized suite run against sqlite/PG AND a real CH via `testcontainers-go/modules/clickhouse`, gated behind `testing.Short()`.
6. **Migration:** dual-write (flag) → validate parity → rewrite the 6 Grafana panels onto a NEW `aenigma-clickhouse` datasource uid → flip source of truth. Reversible until the flip. No SQL shim — panels are rewritten by hand. Backfill optional.
7. **Ops:** `altinity/clickhouse-backup` sidecar sharing the CH data volume (FREEZE + metadata + one-command restore); dry-run a `restore --schema` once; CH native `<prometheus>` endpoint on `:9363` added to `prometheus.yml`; healthcheck on `http://localhost:8123/ping`; bind ports to `127.0.0.1`; 4G memory cap.

**Riskiest unknowns:** (1) the exact current ClickHouse LTS CalVer tag — gate with human-verify; (2) GDPR per-user erase under an UPDATE-less store needs an `ALTER TABLE … DELETE` mutation path (flag for planner — the existing `EraseByUserID` must be ported); (3) whether dashboards require pre-cutover history (decides if a one-shot backfill task is needed).
