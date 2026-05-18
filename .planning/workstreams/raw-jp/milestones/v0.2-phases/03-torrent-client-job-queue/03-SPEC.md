---
id: LIB-torrent-job-queue
title: Embedded torrent client + Postgres job queue + Prometheus metrics
workstream: raw-jp
milestone: v0.2
phase: 03
created_at: 2026-05-18
status: SPEC-ready
ambiguity_score: 0.20
mode: --auto
---

# Phase 03 (workstream `raw-jp`, milestone v0.2): Torrent Client + Job Queue + Metrics — Specification

**Workstream:** `raw-jp`
**Milestone:** v0.2 Self-Hosted Library
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
**Requirements:** LIB-05, LIB-06, LIB-NF-01, LIB-NF-02, LIB-NF-03
**Depends on:** Phase 1 (scaffold)
**Mode:** `--auto`

## Goal

Embed `github.com/anacrolix/torrent` behind a clean interface; persist a job queue in Postgres with `FOR UPDATE SKIP LOCKED` concurrent-worker semantics; emit Prometheus metrics that power a new Grafana dashboard. Expose enqueue + status REST endpoints. Workers progress jobs through `queued → downloading` and stop at `encoding` (encoder lives in Phase 4).

## Background

**Today, three things are true and need to change:**

1. **The library service has no way to actually pull bytes.** Phase 2 ships search; Phase 3 has to turn a chosen magnet into a downloaded file on disk under `LIBRARY_TORRENT_DOWNLOAD_DIR`. The embedded `anacrolix/torrent` Go library is the existing-pure-Go choice (no daemon, supports magnet/DHT/PEX/UDP/uTP, BSD-3 licensed).

2. **Long-running work needs a queue with concurrency-safe checkout.** A naïve "fetch and process" loop would race when we eventually have N>1 workers. Postgres `FOR UPDATE SKIP LOCKED` is the standard pattern; the codebase has no other job-queue dependency, so adding Redis Streams or RabbitMQ for one service is overkill.

3. **Observability is a v0.2 hard requirement.** Without metrics, we can't see when a torrent stalls or when disk fills up. The library service ships its own Grafana dashboard from day one.

**The implementation:**
- `internal/torrent/client.go` — thin facade over `anacrolix/torrent`.
- `internal/domain/job.go` + `internal/repo/job.go` — the queue.
- `internal/service/{download_worker,disk_guard}.go` — the worker loop + the disk-free probe.
- `internal/metrics/library_metrics.go` — Prometheus counters/gauges/histograms.
- `internal/handler/jobs.go` — CRUD on jobs.
- `migrations/001_library_jobs.sql` — initial schema.
- `infra/grafana/dashboards/library.json` — new dashboard.

## Requirements

### LIB-05: Embedded torrent client

- **Current:** No `services/library/internal/torrent/`.
- **Target:**
  - `client.go` with:
    ```go
    type Client struct { /* anacrolix client + config + metrics hooks */ }
    func NewClient(cfg Config) (*Client, error)
    type Config struct {
        DownloadDir    string
        MaxPeers       int
        UploadRateKBPS int
        SeedDuration   time.Duration
    }
    type DownloadHandle interface {
        ID() string           // anacrolix's torrent infohash hex
        Progress() (downloaded, total int64, peers int)
        Cancel()
        Done() <-chan struct{}
    }
    func (c *Client) Add(ctx context.Context, magnetURI string) (DownloadHandle, error)
    func (c *Client) Close() error
    ```
  - On `Add`, parse magnet via `metainfo.ParseMagnetURI`, then `Client.AddMagnet`. Wait for `<-t.GotInfo()` then start downloading. Return the handle to the worker.
  - Configurable via env: `LIBRARY_TORRENT_DOWNLOAD_DIR` (default `/data/torrents`), `LIBRARY_TORRENT_MAX_PEERS` (default 80), `LIBRARY_TORRENT_MAX_UPLOAD_RATE_KBPS` (default 1024), `LIBRARY_TORRENT_SEED_DURATION` (default `24h`).
- **Acceptance:** Unit test wraps a fake tracker fixture (e.g. file-backed tracker via anacrolix's test helpers) and verifies the `Add → Done` lifecycle completes. Integration test (gated on `INTEGRATION=1`) downloads a known small public-domain torrent (e.g. archive.org Pop Goes the Weasel test torrent) and asserts the file lands on disk.

### LIB-06: Postgres-backed job queue

- **Current:** No queue.
- **Target:**
  - Migration `001_library_jobs.sql` per the design doc verbatim (uuid PK, source enum, magnet, title, uploader, quality, size_bytes, status enum, progress_pct, error_text, created/updated/completed_at, shikimori_id nullable). Index `idx_library_jobs_status` on `status` where `status NOT IN ('done', 'cancelled')` for the queue scan.
  - `domain/job.go` — Go struct with GORM tags + `JobStatus` constants.
  - `repo/job.go` — methods:
    - `Create(ctx, job) error`
    - `Get(ctx, id) (*Job, error)`
    - `List(ctx, filter JobFilter) ([]Job, error)`
    - `Claim(ctx, statuses ...JobStatus) (*Job, error)` — runs `SELECT ... FOR UPDATE SKIP LOCKED LIMIT 1 WHERE status IN ($statuses)` in a tx + flips status to the next state. Returns `nil, nil` when nothing is available.
    - `UpdateProgress(ctx, id, downloadedBytes, totalBytes, peers) error`
    - `UpdateStatus(ctx, id, newStatus, errorText) error`
    - `Cancel(ctx, id) error` — transitions `queued|downloading → cancelled`.
  - `service/download_worker.go` — N goroutines (default 2 via `LIBRARY_DOWNLOAD_WORKERS`). Loop: `Claim(queued)` → `torrent.Add(magnet)` → poll Progress every 5s → `UpdateProgress` + emit metrics → on `Done`, `UpdateStatus(encoding)`.
  - `handler/jobs.go` — endpoints:
    - `POST /api/library/jobs` — body `{magnet, title, source, uploader?, quality?, size_bytes?, shikimori_id?}` (admin-gated). Server-side fills `id`, `status=queued`, timestamps. Returns the created row.
    - `GET /api/library/jobs?status=&limit=` — admin-gated list.
    - `GET /api/library/jobs/{id}` — single fetch.
    - `DELETE /api/library/jobs/{id}` — `Cancel()`.
- **Acceptance:**
  1. Two concurrent workers each claim a different queued job without one stomping the other (integration test).
  2. POST returns the created job in the response; GET returns it consistent with the post.
  3. Cancel transitions a running job to `cancelled` and the torrent client stops fetching peers.
  4. Progress updates every 5s while the torrent runs.

### LIB-NF-01: Disk-free guard

- **Current:** No disk guard.
- **Target:**
  - `service/disk_guard.go` exposes `Check() (freeBytes uint64, totalBytes uint64, freePct int)` using `golang.org/x/sys/unix.Statfs`.
  - Background goroutine polls every 30s and updates the `library_disk_free_bytes` gauge.
  - Enqueue handler calls `Check()` before inserting; rejects with HTTP 507 `{"error":"disk_full"}` when `freePct < LIBRARY_DISK_FREE_MIN_PCT` (default 20). Increments `library_enqueue_rejected_total{reason="disk_full"}`.
- **Acceptance:** Manually setting `LIBRARY_DISK_FREE_MIN_PCT=99` reproduces the 507 response on enqueue. Gauge populates correctly.

### LIB-NF-02: Stall detection

- **Current:** No stall detection.
- **Target:**
  - Worker tracks `lastNonZeroPeerAt` per active download.
  - When `time.Since(lastNonZeroPeerAt) > LIBRARY_TORRENT_STALL_TIMEOUT` (default `30m`), worker transitions the job to `failed` with `error_text="stalled: no peers for 30 minutes"`, releases the torrent.
- **Acceptance:** Integration test with a tracker that returns zero peers ages a queued job to `failed` within the configured window.

### LIB-NF-03: Prometheus metrics + Grafana dashboard

- **Current:** Only the standard HTTP middleware metrics from Phase 1.
- **Target:**
  - `metrics/library_metrics.go`:
    - `library_jobs_total{status}` counter (incr on transitions).
    - `library_download_bytes_total` counter (incr by bytes downloaded per progress tick).
    - `library_active_torrents` gauge (current handles count).
    - `library_disk_free_bytes` gauge (from disk_guard).
    - `library_enqueue_rejected_total{reason}` counter.
    - `library_torrent_seed_count` gauge.
  - `infra/grafana/dashboards/library.json` — new dashboard with 6 panels (one per metric) plus a job-status pie/donut.
- **Acceptance:** `curl /metrics | grep ^library_` shows the new metrics. Grafana auto-loads the new dashboard at deploy time (Grafana provisioning already auto-loads `infra/grafana/dashboards/*.json`).

## Acceptance Criteria

1. `services/library/internal/torrent/` exists with the documented interface; unit tests pass.
2. Migration `001_library_jobs.sql` applies on service start (idempotent).
3. POSTing a magnet to `/api/library/jobs` results in a row + a downloaded file appearing under `LIBRARY_TORRENT_DOWNLOAD_DIR` (smoke test against a public-domain magnet).
4. Two workers concurrently claim two queued jobs (integration test using `--parallel=2`).
5. `library_jobs_total{status="downloading"}` increments and `library_active_torrents` gauge tracks open handles.
6. Cancel API moves a running job to `cancelled` within 2s.
7. `library_disk_free_bytes` gauge populated and `LIBRARY_DISK_FREE_MIN_PCT=99` reproduces 507 enqueue.
8. Stall detection transitions a no-peer job to `failed` (integration test).
9. New Grafana dashboard `infra/grafana/dashboards/library.json` validates against the Grafana schema.

## Auto-selected implementation decisions

- **Tx semantics for `Claim()`:** Single transaction `BEGIN; SELECT ... FOR UPDATE SKIP LOCKED LIMIT 1; UPDATE library_jobs SET status='downloading', updated_at=now() WHERE id=?; COMMIT;`. GORM's `db.Transaction(...)` wraps this.
- **Worker shutdown:** Workers listen on a `context.Context` passed from `main.go`. On SIGTERM, all workers receive `ctx.Done()`, cancel active torrent handles, mark in-flight jobs back to `queued` via `Cancel`, then exit.
- **Progress emit cadence:** 5 seconds — matches the design doc.
- **Restart resumption:** Worker startup loop runs `UPDATE library_jobs SET status='queued' WHERE status='downloading'` once (no in-flight rows survive a restart; jobs re-enter the queue). Documented in the operator runbook.
- **Anacrolix client lifecycle:** Single `*torrent.Client` per process. Created at service start, closed on shutdown.
- **Cancel propagation:** `DELETE /jobs/{id}` flips status to `cancelled` THEN calls the worker's handle.Cancel() via an in-memory `map[jobID]DownloadHandle` keyed by job ID. Worker discovers the status change on the next 5s tick and stops gracefully.
- **Magnet validation:** `metainfo.ParseMagnetURI` at enqueue; reject with 400 if invalid.
- **Job creation auth:** `POST /api/library/jobs` admin-gated (gateway AdminMiddleware on `/api/library/*` from Phase 1).

## Touches

- **New:** `services/library/internal/torrent/{client.go,client_test.go}`
- **New:** `services/library/internal/domain/job.go`
- **New:** `services/library/internal/repo/job.go`
- **New:** `services/library/internal/service/{download_worker,disk_guard}.go`
- **New:** `services/library/internal/metrics/library_metrics.go`
- **New:** `services/library/internal/handler/jobs.go`
- **New:** `services/library/migrations/001_library_jobs.sql`
- **New:** `infra/grafana/dashboards/library.json`
- **Extend:** `services/library/internal/transport/router.go` (register routes)
- **Extend:** `services/library/internal/config/config.go` (new `TorrentConfig`, `WorkerConfig`)
- **Extend:** `services/library/cmd/library-api/main.go` (wire client + workers + handler)
- **Extend:** `services/library/go.mod` (lock in `github.com/anacrolix/torrent` + `golang.org/x/sys`)
- **Extend:** `docker/.env.example` (document the new torrent + worker + disk envs)

## Out of Scope (for this phase)

- ffmpeg encoding / MinIO upload (Phase 4).
- Admin UI (Phase 5).
- Hybrid resolver (Phase 6).
- Per-torrent seed-ratio targets (use the default seed duration).
- IPv6 / DHT bootstrap tuning beyond defaults.

## Citations to design doc

- Architecture → "anacrolix/torrent wrapper" + "postgres job queue" + "ffmpeg subprocess" pattern.
- Data flow → state machine `queued → downloading → encoding → uploading → done|failed|cancelled` with metric emit cadence.
- Configuration → `LIBRARY_TORRENT_*` env vars.
- Error-handling → "Torrent stalled" + "Disk free < 20%" rows.
