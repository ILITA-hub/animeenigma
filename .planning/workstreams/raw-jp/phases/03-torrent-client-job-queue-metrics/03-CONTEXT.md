# Phase 3: Torrent Client + Job Queue + Metrics - Context

**Gathered:** 2026-05-18
**Status:** Ready for planning
**Mode:** Auto-generated (SPEC pre-written, ambiguity_score 0.20)

<domain>
## Phase Boundary

Embed `github.com/anacrolix/torrent` behind a clean facade; persist a Postgres-backed job queue with `FOR UPDATE SKIP LOCKED` semantics; emit Prometheus metrics and ship a Grafana dashboard. Expose enqueue/list/get/cancel REST endpoints. Workers progress jobs from `queued → downloading` and stop at the `encoding` boundary — the encoder lives in Phase 4.

**Out of scope:** ffmpeg encoding, MinIO upload, admin UI, hybrid resolver, per-torrent seed-ratio targets, IPv6/DHT tuning beyond defaults.

</domain>

<decisions>
## Implementation Decisions

### Locked from SPEC (`milestones/v0.2-phases/03-torrent-client-job-queue/03-SPEC.md`)

- **Torrent library:** `github.com/anacrolix/torrent` (already imported in Phase 2 for `metainfo.ParseMagnetURI`; pin a single version here).
- **Single `*torrent.Client` per process** — created at start, closed on shutdown.
- **Queue claim semantics:** `BEGIN; SELECT ... FOR UPDATE SKIP LOCKED LIMIT 1; UPDATE status='downloading'; COMMIT;` wrapped via `db.Transaction(...)`.
- **Worker shutdown:** SIGTERM → `ctx.Done()` → cancel torrent handles → flip in-flight jobs back to `queued` → exit.
- **Restart resumption:** On startup, `UPDATE library_jobs SET status='queued' WHERE status='downloading'` once. Documented for operator.
- **Progress emit cadence:** 5 seconds.
- **Cancel propagation:** `DELETE /jobs/{id}` flips status → `cancelled`, then in-memory `map[jobID]DownloadHandle` calls `handle.Cancel()`. Worker observes status on next tick and exits gracefully.
- **Magnet validation:** `metainfo.ParseMagnetURI` at enqueue — 400 on invalid.
- **Auth:** `POST/DELETE /api/library/jobs*` admin-gated via gateway (Phase 2 admin gate already covers `/api/library/*`).
- **Job creator auth body shape:** `{magnet, title, source, uploader?, quality?, size_bytes?, shikimori_id?}` — server fills id/status/timestamps.

### State Machine (locked)

`queued → downloading → encoding → uploading → done|failed|cancelled`

Phase 3 implements transitions to `downloading` and stops at boundary `encoding`. The actual encoder + upload code is Phase 4.

### Configuration (locked envs)

- `LIBRARY_TORRENT_DOWNLOAD_DIR` (default `/data/torrents`)
- `LIBRARY_TORRENT_MAX_PEERS` (default 80)
- `LIBRARY_TORRENT_MAX_UPLOAD_RATE_KBPS` (default 1024)
- `LIBRARY_TORRENT_SEED_DURATION` (default `24h`)
- `LIBRARY_TORRENT_STALL_TIMEOUT` (default `30m`)
- `LIBRARY_DOWNLOAD_WORKERS` (default 2)
- `LIBRARY_DISK_FREE_MIN_PCT` (default 20)

### Migration (locked)

`migrations/001_library_jobs.sql` — uuid PK, source enum (`nyaa|animetosho|manual`), magnet, title, uploader, quality, size_bytes, status enum (`queued|downloading|encoding|uploading|done|failed|cancelled`), progress_pct, error_text, created_at/updated_at/completed_at, shikimori_id (nullable). Index `idx_library_jobs_status` partial on `status NOT IN ('done', 'cancelled')`.

### Metrics (locked)

- `library_jobs_total{status}` counter
- `library_download_bytes_total` counter
- `library_active_torrents` gauge
- `library_disk_free_bytes` gauge
- `library_enqueue_rejected_total{reason}` counter
- `library_torrent_seed_count` gauge

### Disk Guard (locked)

- `unix.Statfs` for free/total/percent.
- Polling goroutine, 30s cadence.
- Enqueue handler rejects with HTTP 507 when `freePct < min`.

### Claude's Discretion (autonomous mode)

- Test fixture structures (mock trackers via anacrolix test helpers or local-loopback tracker).
- Whether to use a separate `internal/metrics/` package or extend the existing `libs/metrics` (favor service-local for library-specific metrics).
- Exact retry/backoff inside the worker beyond the stall-timeout rule.
- Internal helper signatures.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `services/library/` — Phase 1 scaffold + Phase 2 search live on port 8089.
- `services/library/internal/config/config.go` — extend with `TorrentConfig`, `WorkerConfig`, `DiskConfig`.
- `services/library/cmd/library-api/main.go` — wire torrent client + workers + disk guard + handler.
- `libs/database` — Postgres connection helper with `AutoMigrate`. Use raw SQL migration file alongside.
- `libs/httputil` — response envelope.
- `libs/logger` — structured logger.
- `libs/errors` — wrap external errors.
- `services/scraper/internal/repo/` — example of GORM repo pattern.
- `libs/metrics` — Prometheus collector.
- `infra/grafana/dashboards/` — existing dashboards auto-provisioned.
- `github.com/anacrolix/torrent` — already in go.mod (Phase 2).

### Established Patterns

- Migrations: SQL files in `migrations/` + GORM AutoMigrate at startup. Library will use a hybrid — SQL migration file PLUS GORM-tagged struct. Run the SQL file via embedded fs at startup, then AutoMigrate (idempotent).
- Workers: see `services/scheduler/` for goroutine + context cancellation patterns.
- Graceful shutdown: `signal.NotifyContext` + `errgroup`.
- Tests: testcontainers for Postgres-backed unit tests, `INTEGRATION=1` gate for network/integration tests.

### Integration Points

- Router: register `/jobs` routes under existing admin-gated prefix in `transport/router.go`.
- Gateway: no changes needed (admin gate is in place from Phase 2).
- Compose: existing `library_torrents` volume already mounted (Phase 1).
- Disk guard reads from `LIBRARY_TORRENT_DOWNLOAD_DIR` path inside container.

</code_context>

<specifics>
## Specific Ideas

- SPEC reference at `milestones/v0.2-phases/03-torrent-client-job-queue/03-SPEC.md` is authoritative.
- Acceptance tests include both unit (httptest, GORM in-memory or testcontainers) and INTEGRATION-gated (real network).
- Integration test smoke can use archive.org public-domain test torrent (e.g., Pop Goes the Weasel) — but only when `INTEGRATION=1`.
- Grafana dashboard JSON validates against Grafana schema; existing dashboards in `infra/grafana/dashboards/` are the reference shape.

</specifics>

<deferred>
## Deferred Ideas

- ffmpeg encoding + MinIO upload (Phase 4).
- Admin UI (Phase 5).
- Hybrid resolver fallback (Phase 6).
- Per-torrent seed-ratio targets.
- IPv6 / DHT bootstrap tuning.

</deferred>
