# Phase 1: ClickHouse Foundation + EventStore Swap - Pattern Map

**Mapped:** 2026-06-04
**Files analyzed:** 9 new/modified surfaces
**Analogs found:** 8 with strong analogs / 9 (1 genuinely-new: the ClickHouse DDL bootstrap)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `services/analytics/internal/repo/clickhouse_store.go` | repo (EventStore impl) | batch insert | `services/analytics/internal/repo/postgres_store.go` | exact (same interface) |
| `services/analytics/internal/repo/clickhouse_schema.go` (DDL bootstrap) | migration | DDL | `services/analytics/internal/repo/models.go` `EnsureView` | partial (no native CH analog) |
| `services/analytics/internal/config/config.go` (extend) | config | — | same file (existing pattern) | exact |
| `services/analytics/cmd/analytics-api/main.go` (wire) | bootstrap/DI | — | same file (existing pattern) | exact |
| `services/analytics/internal/observ/metrics.go` (already has dropped counter) | metrics | — | same file + `libs/metrics/db.go` | exact |
| `services/analytics/internal/ingest/batcher.go` (reuse as-is) | service | event-driven/batch | same file (no change) | exact |
| `docker/docker-compose.yml` (add `clickhouse:` service) | config (infra) | — | `tempo:` / `postgres:` blocks | exact (stateful) |
| `docker/prometheus/prometheus.yml` (add scrape job) | config (infra) | — | `analytics` job | exact |
| `docker/grafana/provisioning/datasources/datasources.yml` (add CH datasource) | config (infra) | — | `PostgreSQL` / `Tempo` entries | exact |

---

## Pattern Assignments

### 1. EventStore interface + ClickHouse impl

**Interface (the swap seam — DO NOT change):** `services/analytics/internal/domain/store.go:11-14`

```go
type EventStore interface {
	InsertBatch(ctx context.Context, events []Event) error
	UpsertIdentity(ctx context.Context, anonymousID, userID string, ts time.Time) error
}
```

A ClickHouse impl must satisfy exactly these two methods — no additions, no signature changes. The interface takes `[]domain.Event`, so the CH store owns its own `domain.Event → CH row` mapping (mirror `toModel` at `postgres_store.go:17-39`).

**Analog impl:** `services/analytics/internal/repo/postgres_store.go:11-59`

Convention to follow (Postgres store shape):
- `type PostgresStore struct{ db *gorm.DB }` + `func NewPostgresStore(db *gorm.DB) *PostgresStore` (lines 13-15). The CH store is `type ClickHouseStore struct{ conn driver.Conn }` + `NewClickHouseStore(conn) *ClickHouseStore`.
- `InsertBatch` short-circuits empty (`if len(events) == 0 { return nil }`, lines 42-44), maps every event to a row, then bulk-writes. Postgres uses `CreateInBatches(rows, 200)` (line 49); the CH analog is `conn.PrepareBatch(ctx, "INSERT INTO ...")` → `batch.Append(...)` per row → `batch.Send()`.
- `UpsertIdentity` guards empty inputs (lines 53-55) then inserts an append-only identity row. ClickHouse has no UPSERT — the existing Postgres impl is also append-only ("latest row per anonymous_id wins" — `models.go:48-49`), so the CH version is a plain insert into an identities table, fully consistent with current semantics.
- JSON-shaped columns default to `'{}'` when empty (`toModel` lines 32-37) — preserve this.

**Backend-agnostic contract test (AR-STORE-03):** `services/analytics/internal/repo/postgres_store_test.go:1-40` constructs the store over an in-memory backend (`sqlite.Open(":memory:")`, lines 13-25) and exercises `InsertBatch`/`UpsertIdentity` against the `EventStore` interface. The planner should extract the assertions into an interface-level test helper that both `PostgresStore` (sqlite-backed) and `ClickHouseStore` (containerized — testcontainers per CLAUDE.md "Use testcontainers for database tests") run against.

**New file the planner creates:** `services/analytics/internal/repo/clickhouse_store.go`.

---

### 2. DI wiring + backend selection (config/env)

**Analog:** `services/analytics/cmd/analytics-api/main.go:44-67`

Today the backend is hard-wired: `db, _ := database.New(cfg.Database)` (line 44) → `store := repo.NewPostgresStore(db.DB)` (line 61) → `batcher := ingest.New(store, …)` (line 62). The batcher takes a `domain.EventStore`, so swapping the store is a one-line constructor change.

Convention to follow for backend selection (mirror `config.Load()` env pattern at `config/config.go:30-51`):
- Add an env-driven selector, e.g. `ANALYTICS_STORE_BACKEND` (default `postgres` for reversibility) read via the existing `getEnv` helper (`config.go:53-58`).
- Add a `ClickHouse` connection sub-config block to `Config` (mirror the `Database database.Config` field at `config.go:13-21`, populated from `CLICKHOUSE_HOST`/`CLICKHOUSE_PORT`/`CLICKHOUSE_DB`/`CLICKHOUSE_USER`/`CLICKHOUSE_PASSWORD` via `getEnv`/`getEnvInt`).
- In `main.go`, branch on the selector to build either `repo.NewPostgresStore(db.DB)` or `repo.NewClickHouseStore(chConn)` and pass the chosen `domain.EventStore` into `ingest.New(...)` unchanged (line 62).

Note for the **migration strategy** (Claude's discretion per CONTEXT): a dual-write store wrapper is trivial here — wrap both stores behind one `domain.EventStore` whose `InsertBatch` fans out to both. This is the lowest-risk reversible path and needs zero changes to the batcher or handlers.

**Drop-hook wiring already exists:** `main.go:66` — `.WithDropHook(func() { observ.EventsDropped.Inc() })`. No change needed; the dropped-event metric (AR-STORE-05) is already plumbed (see §7).

---

### 3. Async batcher (reuse unchanged)

**Analog (and the file itself — reuse as-is):** `services/analytics/internal/ingest/batcher.go`

The batcher is store-agnostic (depends only on `domain.EventStore`, line 24) — it works with the ClickHouse store with **zero changes**. Document its contract so the planner doesn't reinvent it:

- **Batch size / flush triggers:** `Config{MaxBatch, FlushInterval, BufferSize}` (lines 15-19). Defaults: `MaxBatch=500`, `FlushInterval=1s`, `BufferSize=10000` (lines 33-41). Env overrides via `ANALYTICS_MAX_BATCH` / `ANALYTICS_FLUSH_INTERVAL` / `ANALYTICS_BUFFER_SIZE` (`config.go:47-49`).
- **Flush logic:** `run()` (lines 72-108) flushes on `len(buf) >= MaxBatch` (lines 89-91) OR the `FlushInterval` ticker (lines 93-94), and drains on `Stop()` (lines 95-105).
- **Drop-on-full:** `Enqueue` is non-blocking — `select { case b.ch <- e: ... default: onDrop(); return false }` (lines 56-66). A full channel drops the event and fires the metrics hook. This is the AR-STORE ingestion contract; preserve it.
- **How it calls InsertBatch:** `flush()` (lines 110-127) copies the buffer, calls `b.store.InsertBatch(ctx, batch)` with a 5s timeout context (line 111), and separately calls `UpsertIdentity` for `EventTypeIdentify` events (lines 120-126). The ClickHouse store must tolerate this same call shape.

**No new file.** The CH store slots in behind this batcher untouched.

---

### 4. New stateful docker service

**Closest analog:** `tempo:` block at `docker/docker-compose.yml:274-291` (stateful, named volume, config-file mount, host-bound port, healthcheck) — and `postgres:` at `:6-22` for the auth/env/healthcheck shape. Tempo is the best analog because, like ClickHouse, it is a 3rd-party data store mounting a config file with a named data volume.

Convention to mirror (compose the CH service from these):
- **Image pinning — pin a concrete tag** (NOT `latest`). All stateful services pin: `postgres:16-alpine` (`:7`), `grafana/tempo:2.4.1` (`:275`), `grafana/loki:2.9.4` (`:231`). Use e.g. `clickhouse/clickhouse-server:24.x` (exact patch).
- **`container_name: animeenigma-clickhouse`** + `restart: unless-stopped` (every block).
- **Config mount + named volume:** Tempo mounts `../infra/tempo/tempo.yaml:/etc/tempo/tempo.yaml:ro` + `tempo_data:/var/tempo` (lines 280-281). CH mirrors: optional `:ro` config XML mount + `clickhouse_data:/var/lib/clickhouse`.
- **Host-bound port:** always `127.0.0.1:HOST:CONTAINER` (e.g. `:15` postgres, `:283` tempo). CH native `127.0.0.1:9000:9000` collides with MinIO (`:48`); CH HTTP `8123` is free — bind `127.0.0.1:8123:8123` (and remap native if exposed, e.g. `127.0.0.1:9100:9000`, like Tempo's `3201:3200` remap-on-collision precedent at line 283).
- **Healthcheck:** mirror Tempo's wget spider (`:287-291`): `wget --quiet --tries=1 --spider http://localhost:8123/ping`, `interval: 15s / timeout: 5s / retries: 5`.
- **depends_on:** the `analytics` service must add `clickhouse: { condition: service_healthy }` (mirror analytics→postgres at `:781-783` and promtail→loki at `:255-257`).
- **Named volume registration:** add `clickhouse_data:` under the top-level `volumes:` list (`:861-877`).
- **Networks:** none needed explicitly — all services share the implicit `default` network `animeenigma-network` (`:879-881`).
- **Resource limits:** the repo does NOT use compose `deploy.resources` limits on any stateful service (postgres/redis/tempo/loki have none). Follow suit — do not invent a limits block; CH tuning goes in its config XML if needed. (The only RSS cap in the project is the animepahe-resolver sidecar, out of scope.)

Add the `analytics` env block (`:769-778`) keys for CH connection (`CLICKHOUSE_HOST: clickhouse`, etc.) when wiring §2.

**Modified file:** `docker/docker-compose.yml`.

---

### 5. Prometheus scrape config

**Analog:** `analytics` job at `docker/prometheus/prometheus.yml:68-71`

```yaml
  - job_name: 'analytics'
    static_configs:
      - targets: ['analytics:8092']
    metrics_path: /metrics
```

Convention: every AnimeEnigma target is a `job_name` + `static_configs.targets: ['<service>:<port>']` + `metrics_path: /metrics`, appended to `scrape_configs` (global `scrape_interval: 15s`, lines 1-3). The Prometheus self-scrape (lines 7-10) shows the override pattern for a non-`/metrics` path.

For ClickHouse: add a job targeting CH's built-in Prometheus endpoint (CH exposes `/metrics` on its HTTP port when `<prometheus>` is enabled in its server config) — e.g.:
```yaml
  - job_name: 'clickhouse'
    static_configs:
      - targets: ['clickhouse:8123']   # or the dedicated prometheus port if configured
    metrics_path: /metrics
```
This requires enabling CH's `<prometheus>` block in its config XML (note this dependency for the planner). No separate exporter container is needed — CH self-metrics are sufficient for Phase 1.

**Modified file:** `docker/prometheus/prometheus.yml`.

---

### 6. Grafana datasource provisioning

**Analog:** `PostgreSQL` (`:31-44`) and `Tempo` (`:46-60`) entries in `docker/grafana/provisioning/datasources/datasources.yml`

Exact shape to add a ClickHouse datasource:
```yaml
  - name: ClickHouse
    uid: aenigma-clickhouse          # stable uid; dashboards bind to it (see Prometheus uid warning, lines 5-9)
    type: grafana-clickhouse-datasource   # the official CH plugin type
    access: proxy
    url: http://clickhouse:8123
    jsonData:
      defaultDatabase: animeenigma   # or the CH db name
      # server/port/protocol fields per the grafana-clickhouse-datasource plugin schema
    secureJsonData:
      password: ...                  # mirror PostgreSQL secureJsonData.password at :42-43
    editable: false
```

Conventions to follow:
- **Stable `uid` prefixed `aenigma-`** (`aenigma-postgres`, `aenigma-loki`, `aenigma-tempo`). The header comment at lines 5-9 is a hard warning: dashboards + alert rules bind to exact UIDs — pick `aenigma-clickhouse` once and never rename.
- **`access: proxy`**, **`editable: false`** on every datasource.
- **In-network URL** (`postgres:5432`, `http://tempo:3200`) — use `http://clickhouse:8123`.
- Credentials go in `secureJsonData` (Postgres pattern, `:42-43`).
- The CH datasource requires the `grafana-clickhouse-datasource` plugin — Grafana must install it. The `grafana:` compose block uses a custom entrypoint (`:312-319`); the planner should add `GF_INSTALL_PLUGINS: grafana-clickhouse-datasource` to the grafana `environment` block (around `:320`) OR pre-bake it. Flag this as the one extra step beyond the datasources file.

**Modified file:** `docker/grafana/provisioning/datasources/datasources.yml` (+ grafana compose env).

---

### 7. Backend metrics (`*_total` counters via promauto)

**Analog:** `services/analytics/internal/observ/metrics.go:10-19` (and the canonical promauto pattern at `libs/metrics/db.go:8-40`).

The dropped-event counter (AR-STORE-05) **already exists** — no new code:
```go
EventsDropped = promauto.NewCounter(prometheus.CounterOpts{
	Name: "analytics_events_dropped_total",
	Help: "Clickstream events dropped because the in-process buffer was full.",
})
```
It is wired to the batcher drop-hook at `main.go:66`. Convention for any *new* counter: `promauto.NewCounter` with a `_total` suffix + `Help`, declared in a package `var` block, auto-registered to the default registry (`observ/metrics.go:1-3` comment). If the CH store needs its own counters (e.g. insert failures), add them to `observ/metrics.go` following this exact shape; no labels unless required (mirror `libs/metrics/db.go`).

**Likely no new file** — extend `observ/metrics.go` only if CH-specific counters are wanted.

---

### 8. Migrations / schema bootstrap

**Analog:** `services/analytics/internal/repo/models.go:59-88` — `AutoMigrateAll` (GORM `db.AutoMigrate(&Event{}, &Identity{})`) + `EnsureView` (idempotent `DROP VIEW IF EXISTS` + `CREATE VIEW`, run from `main.go:54-59`).

The convention is **schema-on-boot, idempotently** — the service creates its own tables/views at startup (`main.go:54-59`), no external migration tool. `libs/database` also auto-creates the Postgres database itself (`database.go:88-118 ensureDatabaseExists`).

**ClickHouse cannot use GORM AutoMigrate** (GORM here is Postgres/sqlite-only — see §6 below). So the CH DDL is genuinely new: the planner creates a `clickhouse_schema.go` with an idempotent `EnsureSchema(ctx, conn)` that runs `CREATE DATABASE IF NOT EXISTS` + `CREATE TABLE IF NOT EXISTS ... ENGINE = MergeTree() ...` (CH supports native `IF NOT EXISTS`, simpler than the EnsureView DROP+CREATE dance). Call it from `main.go` right after opening the CH connection, mirroring the `repo.AutoMigrateAll` + `repo.EnsureView` call site (`main.go:54-59`). Engine per CONTEXT: `MergeTree` family, `ORDER BY (timestamp, operation, target)`-style tuple, partition by month/`toYYYYMM(timestamp)`; AggregatingMergeTree rollups are deferred (AR-V2-01).

The CH DDL (`.sql` or Go string constant) should live co-located in the repo package: `services/analytics/internal/repo/clickhouse_schema.go`.

**New file:** `services/analytics/internal/repo/clickhouse_schema.go`.

---

## Shared Patterns

### DB connection lib — does NOT generalize to ClickHouse
**Source:** `libs/database/database.go:1-118`
`libs/database` is GORM+Postgres-specific: it imports `gorm.io/driver/postgres` (line 7), its `New` opens a `postgres.Open(cfg.DSN())` GORM connection (line 55), `DSN()` builds a `host=… port=… sslmode=…` Postgres string (lines 35-40), and `ensureDatabaseExists` issues Postgres `CREATE DATABASE` (lines 88-118). **It cannot back ClickHouse.**

Therefore the ClickHouse client is **standalone**: use the official `github.com/ClickHouse/clickhouse-go/v2` driver directly inside `services/analytics/internal/repo/clickhouse_store.go`, opening `clickhouse.Open(&clickhouse.Options{...})`. Do NOT extend `libs/database` and do NOT (for Phase 1) create a shared `libs/clickhouse` module — keep it local to the analytics service, matching how `libs/database` stays GORM-only.

**`libs/` module-addition checklist (only if you reconsider a shared lib — per project memory):**
Adding any new `libs/{name}` module touches ALL of: (1) `go.work` add `./libs/{name}`; (2) the importing service `go.mod` require+replace (analytics `go.mod` `replace` block shows the pattern); (3) **EVERY** go.work service Dockerfile (all 13) needs `COPY libs/{name}/go.mod libs/{name}/go.sum* ./libs/{name}/` or every service build breaks; (4) `go work sync`. This is a heavy 4-point change — **avoid it this phase**; add `clickhouse-go/v2` as a direct dependency of `services/analytics/go.mod` only (no service besides analytics imports CH in Phase 1). The analytics `go.mod` currently has NO clickhouse dep (verified — none anywhere in the repo), so this is a fresh `go get`.

### Promauto metric pattern
**Source:** `services/analytics/internal/observ/metrics.go` + `libs/metrics/db.go`
**Apply to:** any new counter/gauge. `_total` suffix for counters, `Help` string, package `var` block, default-registry auto-register.

### Idempotent schema-on-boot
**Source:** `main.go:54-59` + `models.go:60-88`
**Apply to:** the CH schema bootstrap — call `EnsureSchema` at boot, make it re-runnable (CH `IF NOT EXISTS`).

---

## No Clean Analog Found (genuinely new)

| Item | Role | Reason |
|------|------|--------|
| `clickhouse_schema.go` (CH DDL via native driver) | migration | No native ClickHouse DDL exists in the repo. The Postgres path uses GORM AutoMigrate + a SQL view; CH needs raw `MergeTree` DDL over `clickhouse-go/v2`. Pattern (idempotent schema-on-boot) is borrowed, but the SQL itself and the driver are new. |
| ClickHouse client connection | repo | `libs/database` is Postgres-only and does not generalize; the `clickhouse-go/v2` driver + `clickhouse.Open` connection is a first-in-repo dependency (no `clickhouse` import anywhere today). |
| Grafana `grafana-clickhouse-datasource` plugin install | infra | The datasource YAML shape is analogous to Postgres/Tempo, but Grafana must additionally install the CH plugin (`GF_INSTALL_PLUGINS`) — no existing datasource requires a plugin install, so this compose change has no analog. |

Everything else (EventStore impl shape, DI wiring, env config, batcher reuse, dropped-event metric, compose stateful-service shape, Prometheus job, datasource YAML) has an exact in-repo analog the planner should copy rather than reinvent.

---

## Summary

Phase 1 is overwhelmingly a **copy-the-Postgres-path-for-ClickHouse** exercise: the `EventStore` interface (`store.go:11-14`) is a clean two-method swap seam, the async batcher (`batcher.go`) is fully store-agnostic and reused unchanged, the dropped-event metric (AR-STORE-05) already exists and is already wired to the batcher drop-hook (`main.go:66`), and config/DI follow the established `getEnv` + constructor-injection pattern. Infra additions mirror the `tempo:`/`postgres:` compose blocks (pin image, named volume, host-bound port, wget healthcheck, `depends_on: service_healthy`), the `analytics` Prometheus scrape job, and the `aenigma-`prefixed Postgres/Tempo Grafana datasources. The three genuinely-new pieces — all flagged above — are (1) the ClickHouse `MergeTree` DDL bootstrap (`clickhouse_schema.go`), because `libs/database` is GORM/Postgres-only and does not generalize, (2) the standalone `clickhouse-go/v2` driver connection (first CH dependency in the repo, added to `services/analytics/go.mod` only — NOT a shared `libs/` module, to avoid the 13-Dockerfile checklist), and (3) the Grafana ClickHouse datasource plugin install. The strong recommendation, consistent with project conventions, is to keep ClickHouse local to the analytics service and select the backend via an env flag (defaulting to Postgres) so the cutover is reversible on the single host.
