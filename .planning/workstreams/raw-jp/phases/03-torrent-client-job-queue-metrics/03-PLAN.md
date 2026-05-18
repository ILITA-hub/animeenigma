---
phase: 03-torrent-client-job-queue-metrics
plan: 01
type: execute
wave: 1
workstream: raw-jp
milestone: v0.2
depends_on: []
files_modified:
  - services/library/migrations/001_library_jobs.sql
  - services/library/internal/domain/job.go
  - services/library/internal/repo/job.go
  - services/library/internal/repo/job_test.go
  - services/library/internal/torrent/client.go
  - services/library/internal/torrent/client_test.go
  - services/library/internal/metrics/library_metrics.go
  - services/library/internal/metrics/library_metrics_test.go
  - services/library/internal/service/disk_guard.go
  - services/library/internal/service/disk_guard_test.go
  - services/library/internal/service/download_worker.go
  - services/library/internal/service/download_worker_test.go
  - services/library/internal/handler/jobs.go
  - services/library/internal/handler/jobs_test.go
  - services/library/internal/transport/router.go
  - services/library/internal/config/config.go
  - services/library/cmd/library-api/main.go
  - services/library/go.mod
  - services/library/go.sum
  - infra/grafana/dashboards/library.json
  - docker/.env.example
  - .planning/workstreams/raw-jp/phases/03-torrent-client-job-queue-metrics/03-SUMMARY.md
autonomous: true
requirements:
  - LIB-05
  - LIB-06
  - LIB-NF-01
  - LIB-NF-02
  - LIB-NF-03

must_haves:
  truths:
    - "Admin can POST a magnet URI to /api/library/jobs and receive 201 with the created row (id, status='queued', timestamps server-filled)"
    - "Invalid magnet → 400; insufficient disk (freePct < LIBRARY_DISK_FREE_MIN_PCT) → 507 with body {\"error\":\"disk_full\"}"
    - "Two concurrent workers each claim a different queued job via FOR UPDATE SKIP LOCKED (no double-claim)"
    - "A claimed job progresses queued → downloading and the file lands under LIBRARY_TORRENT_DOWNLOAD_DIR"
    - "DELETE /api/library/jobs/{id} transitions a running job to 'cancelled' within ~2s; in-memory handle.Cancel() releases peers"
    - "Workers stop at the encoding boundary — Phase 3 sets status='encoding' but never invokes ffmpeg/upload"
    - "Stall detection flips a zero-peer download to 'failed' with error_text after LIBRARY_TORRENT_STALL_TIMEOUT"
    - "On service restart, any status='downloading' row is rewritten to 'queued' once (resumption)"
    - "Prometheus /metrics exposes library_jobs_total{status}, library_download_bytes_total, library_active_torrents, library_disk_free_bytes, library_enqueue_rejected_total{reason}, library_torrent_seed_count"
    - "Grafana auto-loads infra/grafana/dashboards/library.json at deploy time"
  artifacts:
    - path: services/library/migrations/001_library_jobs.sql
      provides: "library_jobs schema (uuid PK, source/status enums, partial index on active statuses)"
      contains: "CREATE TABLE library_jobs"
    - path: services/library/internal/domain/job.go
      provides: "Job struct with GORM tags + JobStatus / JobSource constants"
      exports: ["Job", "JobStatus", "JobSource", "JobStatusQueued", "JobStatusDownloading", "JobStatusEncoding", "JobStatusFailed", "JobStatusCancelled"]
    - path: services/library/internal/repo/job.go
      provides: "Job repository with Create, Get, List, Claim (FOR UPDATE SKIP LOCKED), UpdateProgress, UpdateStatus, Cancel, ResumeInterruptedDownloads"
      exports: ["JobRepository", "NewJobRepository", "JobFilter"]
    - path: services/library/internal/torrent/client.go
      provides: "anacrolix/torrent facade — Client.Add, Client.Close, DownloadHandle interface"
      exports: ["Client", "NewClient", "Config", "DownloadHandle"]
    - path: services/library/internal/service/download_worker.go
      provides: "WorkerPool that claims queued jobs, drives torrent download, emits metrics, stops at encoding boundary"
      exports: ["WorkerPool", "NewWorkerPool"]
    - path: services/library/internal/service/disk_guard.go
      provides: "Unix Statfs-based free-space probe + polling goroutine that updates library_disk_free_bytes"
      exports: ["DiskGuard", "NewDiskGuard"]
    - path: services/library/internal/metrics/library_metrics.go
      provides: "Library-specific Prometheus collectors (jobs counter, download bytes, gauges)"
      exports: ["LibraryMetrics", "NewLibraryMetrics"]
    - path: services/library/internal/handler/jobs.go
      provides: "REST handlers — POST/GET/DELETE /api/library/jobs"
      exports: ["JobsHandler", "NewJobsHandler"]
    - path: infra/grafana/dashboards/library.json
      provides: "6-panel Grafana dashboard auto-provisioned by existing infra/grafana provider"
      contains: "library_jobs_total"
  key_links:
    - from: "services/library/internal/handler/jobs.go (POST)"
      to: "services/library/internal/repo/job.go Create + disk_guard.Check"
      via: "JobsHandler.Create"
      pattern: "diskGuard\\.Check\\(\\)|jobRepo\\.Create"
    - from: "services/library/internal/service/download_worker.go"
      to: "services/library/internal/repo/job.go Claim (FOR UPDATE SKIP LOCKED) + torrent.Client.Add"
      via: "WorkerPool.runOne"
      pattern: "jobRepo\\.Claim|torrentClient\\.Add"
    - from: "services/library/internal/handler/jobs.go (DELETE)"
      to: "in-memory map[jobID]DownloadHandle in download_worker.go"
      via: "WorkerPool.CancelJob(id)"
      pattern: "workerPool\\.CancelJob|handle\\.Cancel"
    - from: "services/library/cmd/library-api/main.go"
      to: "ResumeInterruptedDownloads at startup (UPDATE status='queued' WHERE status='downloading')"
      via: "jobRepo.ResumeInterruptedDownloads(ctx)"
      pattern: "ResumeInterruptedDownloads"
    - from: "services/library/internal/service/download_worker.go"
      to: "LibraryMetrics.IncJobsTotal, AddDownloadBytes, SetActiveTorrents"
      via: "metrics.<…>"
      pattern: "libMetrics\\.(Inc|Add|Set)"

user_setup: []
---

<objective>
Phase 3 turns the library service from a search-only façade (Phase 2) into a
worker that pulls bytes. We embed `github.com/anacrolix/torrent` behind a
clean facade, add a Postgres-backed `library_jobs` queue with FOR UPDATE
SKIP LOCKED concurrent-worker semantics, expose admin-gated CRUD on jobs,
emit Prometheus metrics, and ship the first library Grafana dashboard. The
worker progresses each job `queued → downloading`, sets the next status to
`encoding`, and **stops there** — the actual encode + upload code lives in
Phase 4. Disk-free, stall, and cancel paths are wired now.

Purpose: deliver the LIB-05, LIB-06, LIB-NF-01, LIB-NF-02, LIB-NF-03
requirements verbatim per `03-SPEC.md` and `v0.2-REQUIREMENTS.md`. Honors the
locked Phase-3 decisions in `03-CONTEXT.md` (single `*torrent.Client` per
process, GORM `db.Transaction(...)` + `clause.Locking{Strength:"UPDATE",
Options:"SKIP LOCKED"}` for Claim, default worker pool of 2 via
`LIBRARY_DOWNLOAD_WORKERS`, startup resume `UPDATE library_jobs SET
status='queued' WHERE status='downloading'`, cancel-flip-then-handle order,
`metainfo.ParseMagnetUri` validation at enqueue).

Output: a library service that you can POST a magnet to, watch the file
appear under `/data/torrents`, scrape Prometheus metrics from, and observe
on a working Grafana dashboard. Phase 4 picks up at `status='encoding'`.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/raw-jp/ROADMAP.md
@.planning/workstreams/raw-jp/STATE.md
@.planning/workstreams/raw-jp/phases/03-torrent-client-job-queue-metrics/03-CONTEXT.md
@.planning/workstreams/raw-jp/milestones/v0.2-phases/03-torrent-client-job-queue/03-SPEC.md
@.planning/workstreams/raw-jp/milestones/v0.2-REQUIREMENTS.md
@.planning/workstreams/raw-jp/phases/02-nyaa-animetosho-search-clients/02-SUMMARY.md
@.planning/workstreams/raw-jp/phases/01-library-service-scaffold/01-SUMMARY.md

# Existing scaffolding the executor extends — read once, reference often.
@services/library/cmd/library-api/main.go
@services/library/internal/config/config.go
@services/library/internal/transport/router.go
@services/library/internal/handler/search.go
@services/library/internal/handler/health.go
@services/library/internal/service/search.go
@services/library/internal/domain/release.go

# Reference patterns (do not modify — read for shape):
@services/scheduler/internal/repo/task.go
@services/catalog/internal/repo/anime.go
@libs/database/database.go
@libs/metrics/metrics.go
@libs/httputil/response.go
@libs/errors/errors.go
@infra/grafana/dashboards/scraper-provider-health.json

<interfaces>
<!-- Key types/exports the executor needs without re-discovery. -->

## libs/database
```go
type DB struct{ *gorm.DB }
func New(cfg Config) (*DB, error)               // auto-creates DB if missing
func (db *DB) AutoMigrate(models ...interface{}) error
func (db *DB) Close() error
type BaseModel struct {
    ID        string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    CreatedAt time.Time
    UpdatedAt time.Time
    DeletedAt gorm.DeletedAt `gorm:"index"`
}
```
Note: `db.DB.Exec("CREATE TYPE …")` is the right place to run raw SQL migrations BEFORE `AutoMigrate`. Wrap enum CREATEs in `DO $$ BEGIN … EXCEPTION WHEN duplicate_object THEN NULL; END $$;` so the migration is idempotent across restarts.

## libs/httputil/response.go (exported helpers)
- `OK(w, data)` — 200 envelope `{success:true, data:…}`
- `Created(w, data)` — 201 envelope
- `NoContent(w)` — 204
- `BadRequest(w, msg)` — 400
- `NotFound(w, resource)` — 404
- `Error(w, err)` — maps `*errors.AppError` → status code; raw error → 500
- `Bind(r, &v)` — decode JSON body

For HTTP 507 (Insufficient Storage), use `httputil.JSON(w, 507, map[string]string{"error": "disk_full"})` since there is no helper.

## libs/errors AppError codes used here
- `errors.InvalidInput(message)` → 400
- `errors.NotFound(resource)` → 404
- `errors.Internal(message)` → 500
- `errors.ExternalAPI(provider, err)` → 502

## anacrolix/torrent v1.61.0 API surface (pinned in Phase 2 go.mod)
```go
import (
    "github.com/anacrolix/torrent"
    "github.com/anacrolix/torrent/metainfo"
)

cfg := torrent.NewDefaultClientConfig()
cfg.DataDir = "/data/torrents"
cfg.Seed = true                                // continue to seed after completion
cfg.EstablishedConnsPerTorrent = 80            // ~ MaxPeers
cfg.UploadRateLimiter = rate.NewLimiter(rate.Limit(uploadRateBps), uploadRateBps)
c, err := torrent.NewClient(cfg)
defer c.Close()

m, err := metainfo.ParseMagnetUri(magnetURI)   // returns metainfo.Magnet
t, err := c.AddMagnet(magnetURI)               // returns *torrent.Torrent
<-t.GotInfo()                                  // wait for metadata
t.DownloadAll()
t.BytesCompleted() / t.Info().TotalLength()    // progress
len(t.PeerConns())                             // peer count
t.Drop()                                       // cancel + release peers
```
ALIAS NOTE: the package exports both `ParseMagnetUri` (canonical) and
`ParseMagnetURI` (alias var). Use `ParseMagnetUri` to match Phase 2's
codebase precedent.

## golang.org/x/sys/unix (add to go.mod for disk guard)
```go
import "golang.org/x/sys/unix"
var st unix.Statfs_t
if err := unix.Statfs("/data/torrents", &st); err != nil { … }
totalBytes := st.Blocks * uint64(st.Bsize)
freeBytes  := st.Bavail * uint64(st.Bsize)
freePct    := int(float64(freeBytes) / float64(totalBytes) * 100)
```

## GORM FOR UPDATE SKIP LOCKED pattern
Two acceptable styles. Spec calls for `clause.Locking`; existing
scheduler uses raw SQL. Use `clause.Locking` per CONTEXT decision:
```go
import "gorm.io/gorm/clause"

err := db.Transaction(func(tx *gorm.DB) error {
    var job domain.Job
    res := tx.WithContext(ctx).
        Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
        Where("status = ?", domain.JobStatusQueued).
        Order("created_at ASC").
        Limit(1).
        Take(&job)
    if errors.Is(res.Error, gorm.ErrRecordNotFound) {
        return nil      // nothing to claim; caller sees claimed==nil
    }
    if res.Error != nil { return res.Error }
    return tx.Model(&job).
        Updates(map[string]any{
            "status":     domain.JobStatusDownloading,
            "updated_at": time.Now(),
        }).Error
})
```
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Migration + domain.Job + repo.Job + repo tests</name>
  <files>
    services/library/migrations/001_library_jobs.sql,
    services/library/internal/domain/job.go,
    services/library/internal/repo/job.go,
    services/library/internal/repo/job_test.go
  </files>
  <action>
    Write the SQL migration verbatim from 03-SPEC (CREATE TYPE job_source / job_status,
    CREATE TABLE library_jobs with all 13 columns + the partial index
    `idx_library_jobs_status (status, created_at) WHERE status NOT IN ('done','cancelled')`).
    Wrap each `CREATE TYPE` in a `DO $$ BEGIN … EXCEPTION WHEN duplicate_object THEN NULL; END $$;`
    block so re-running the migration is idempotent (per Acceptance 1).

    Define `domain.Job` with GORM tags matching the migration columns (UUID PK + gen_random_uuid
    default, snake_case column tags, `pq.NullString` or `*string` for nullable text columns;
    use Go time.Time + `*time.Time` for completed_at). Define `JobSource` + `JobStatus`
    string types with the SPEC-locked constants (`JobSourceNyaa`, `JobSourceAnimeTosho`,
    `JobSourceManual`, `JobStatusQueued`, `JobStatusDownloading`, `JobStatusEncoding`,
    `JobStatusUploading`, `JobStatusDone`, `JobStatusFailed`, `JobStatusCancelled`).

    Implement `repo.JobRepository` over a `*gorm.DB` with: Create(ctx, *Job) → INSERT;
    GetByID(ctx, id) → SELECT or `liberrors.NotFound("job")`; List(ctx, JobFilter) where
    JobFilter holds `Statuses []JobStatus`, `Limit int` (default 100, max 500), `Offset int`;
    Claim(ctx, statuses ...JobStatus) (*Job, error) using the `clause.Locking{Strength:"UPDATE",
    Options:"SKIP LOCKED"}` pattern shown in the interfaces block (return `(nil, nil)` when
    nothing matches); UpdateProgress(ctx, id, downloadedBytes int64, totalBytes int64, peers int)
    → updates the `progress_pct` column (compute pct = downloaded * 100 / total, clamp 0..100;
    no-op when total<=0); UpdateStatus(ctx, id, newStatus, errorText string) → updates status +
    error_text + updated_at, sets completed_at when newStatus ∈ {done, failed, cancelled};
    Cancel(ctx, id) → conditional update `WHERE status IN ('queued','downloading')` setting
    status='cancelled' and updated_at=now() (no-op when already terminal — return
    `liberrors.NotFound("job")` only when row not found, otherwise nil even if status didn't match);
    ResumeInterruptedDownloads(ctx) (int64, error) → `UPDATE library_jobs SET status='queued',
    updated_at=now() WHERE status='downloading'`, returns rows affected (the startup resumption
    hook from CONTEXT).

    Repo tests in `job_test.go`: use sqlmock OR (preferred) a build-tag `//go:build integration`
    + envcheck `INTEGRATION=1` that connects to localhost:5432 with a throwaway schema. Cover
    at minimum: Create→Get roundtrip, Claim returns nil when empty, Claim returns first queued
    job and flips it to downloading, two parallel Claim() calls return DIFFERENT rows when two
    queued jobs exist (acceptance criterion 4 — use `t.Parallel()` + `sync.WaitGroup`),
    UpdateProgress clamps pct, UpdateStatus sets completed_at for terminal states, Cancel
    transitions queued→cancelled and downloading→cancelled but is a no-op on done/failed,
    ResumeInterruptedDownloads rewrites all downloading rows. Sqlmock-style tests are
    acceptable for everything EXCEPT the concurrent-claim test, which MUST run against real
    Postgres (gated by `INTEGRATION=1`) because sqlmock cannot honor `SKIP LOCKED` semantics.
    Use a uniquely-named schema per test (`library_test_$pid_$nanos`) and DROP it in
    `t.Cleanup` so reruns don't collide.

    Run the migration from `cmd/library-api/main.go` via `db.DB.Exec(migrationSQL)` where
    `migrationSQL` is the file content embedded via `//go:embed migrations/001_library_jobs.sql`
    (no separate migration framework). Wire this in Task 5; the migration file lands here.
  </action>
  <verify>
    <automated>cd services/library &amp;&amp; go test ./internal/domain/... ./internal/repo/... -count=1 &amp;&amp; INTEGRATION=1 go test -tags=integration ./internal/repo/... -count=1 -run TestJobRepository_ConcurrentClaim</automated>
  </verify>
  <done>
    `001_library_jobs.sql` re-applies cleanly twice in a row against the same database
    (no "type already exists" error). All non-integration repo unit tests pass under
    `go test ./...`. With INTEGRATION=1 and a running Postgres on localhost:5432, two
    parallel `Claim()` calls return two different job rows (no double-claim) and a third
    parallel call returns `(nil, nil)`.
  </done>
</task>

<task type="auto">
  <name>Task 2: Torrent client facade + tests</name>
  <files>
    services/library/internal/torrent/client.go,
    services/library/internal/torrent/client_test.go
  </files>
  <action>
    Create `internal/torrent/` package wrapping `github.com/anacrolix/torrent`. Define
    `Config{DownloadDir string, MaxPeers int, UploadRateKBPS int, SeedDuration time.Duration}`
    and `Client` holding a single `*torrent.Client` (per CONTEXT decision — one client per
    process). `NewClient(cfg Config) (*Client, error)` builds `torrent.NewDefaultClientConfig()`,
    sets `DataDir = cfg.DownloadDir`, `Seed = true`, `EstablishedConnsPerTorrent = cfg.MaxPeers`,
    and (if `UploadRateKBPS > 0`) `UploadRateLimiter = rate.NewLimiter(rate.Limit(KBPS*1024),
    KBPS*1024)`. Make sure `mkdir -p cfg.DownloadDir` runs before `torrent.NewClient`.

    Expose `DownloadHandle` interface per SPEC: `ID() string` (lowercase hex infohash from
    `t.InfoHash().HexString()`), `Progress() (downloaded, total int64, peers int)` (use
    `t.BytesCompleted()`, `t.Info().TotalLength()` AFTER `<-t.GotInfo()` — total is -1 until
    metadata arrives; return `total=-1, peers=len(t.PeerConns())` in that case),
    `Cancel()` (calls `t.Drop()` AND `close(doneCh)` exactly once via `sync.Once`),
    `Done() &lt;-chan struct{}` (closed when the torrent's `Complete.On()` channel signals
    OR when Cancel runs OR when seed duration elapses).

    `Client.Add(ctx, magnetURI string) (DownloadHandle, error)`: validate magnet via
    `metainfo.ParseMagnetUri(magnetURI)` (return wrapped error on parse failure — caller
    surfaces 400); then `c.anacrolix.AddMagnet(magnetURI)`. Spawn a goroutine that waits
    for `&lt;-t.GotInfo()` (respect ctx via select), calls `t.DownloadAll()`, then waits
    for `t.Complete.On()` or `ctx.Done()`, then schedules a SeedDuration timer after which
    `t.Drop()` is called. The DownloadHandle.Done() channel is closed when the wait
    exits OR when the user calls Cancel().

    `Client.Close() error`: closes underlying anacrolix client; drops all torrents.

    Tests: focus on lifecycle, not network. Use `t.Skip("requires anacrolix test fixture")`
    for any real-tracker test. Required unit tests (no network):
    1. `NewClient` creates the download dir if missing.
    2. `Add()` with malformed magnet returns a non-nil error and the magnet is not added.
    3. `DownloadHandle.Cancel()` is idempotent — calling it twice does not panic and
       Done() resolves exactly once.
    4. `Client.Close()` after Add does not panic and Done() on all outstanding handles resolves.

    For the integration-only test (`//go:build integration`, gated on `INTEGRATION=1`),
    add a smoke that adds a magnet for a known small public-domain torrent (the
    Sintel/Ubuntu test torrent magnet — pick one and document in a comment), waits up to
    120s for Done(), asserts the data dir is non-empty. This test is opt-in and skipped
    in CI by default.
  </action>
  <verify>
    <automated>cd services/library &amp;&amp; go test ./internal/torrent/... -count=1</automated>
  </verify>
  <done>
    `go test ./internal/torrent/...` is green without network. `Client.Add` rejects
    invalid magnets with a wrapped error. `DownloadHandle.Cancel` is idempotent.
    `Client.Close` cleanly tears down the underlying anacrolix client. Integration
    test compiles under the `integration` build tag.
  </done>
</task>

<task type="auto">
  <name>Task 3: Library Prometheus metrics + Disk guard + tests</name>
  <files>
    services/library/internal/metrics/library_metrics.go,
    services/library/internal/metrics/library_metrics_test.go,
    services/library/internal/service/disk_guard.go,
    services/library/internal/service/disk_guard_test.go
  </files>
  <action>
    Create `internal/metrics/library_metrics.go` exposing `LibraryMetrics` with the six
    SPEC-locked collectors (use `promauto.NewCounterVec` / `NewGauge` / `NewGaugeVec`):
    - `library_jobs_total{status}` — CounterVec, increment in `IncJobsTotal(status string)`.
    - `library_download_bytes_total` — Counter, `AddDownloadBytes(n int64)`.
    - `library_active_torrents` — Gauge, `SetActiveTorrents(n int)`.
    - `library_disk_free_bytes` — Gauge, `SetDiskFreeBytes(n uint64)`.
    - `library_enqueue_rejected_total{reason}` — CounterVec, `IncEnqueueRejected(reason string)`.
    - `library_torrent_seed_count` — Gauge, `SetSeedCount(n int)`.

    Register via `promauto` so the metrics auto-appear on the existing `/metrics` endpoint
    served by `transport/router.go`. Provide `NewLibraryMetrics() *LibraryMetrics`. Use a
    package-level `sync.Once` (or accept that promauto panics on double-registration when
    tests run twice — use `prometheus.NewRegistry()` parameter for testability via an
    optional `NewLibraryMetricsWithRegisterer(prometheus.Registerer)`).

    Tests in `library_metrics_test.go`: construct a fresh `prometheus.NewRegistry`, register
    the collectors against it, call each method, then assert via `testutil.ToFloat64(metric)`
    or `testutil.CollectAndCount`. Confirm the SPEC-required label names exist
    (e.g. `IncJobsTotal("downloading")` → exposes `library_jobs_total{status="downloading"} 1`).

    `internal/service/disk_guard.go`: `DiskGuard` struct holding `path string`, a
    `*LibraryMetrics` ref, and a logger. Public methods:
    - `Check() (freeBytes uint64, totalBytes uint64, freePct int, err error)` via
      `golang.org/x/sys/unix.Statfs`. Compute `totalBytes := st.Blocks * uint64(st.Bsize)`,
      `freeBytes := st.Bavail * uint64(st.Bsize)`, `freePct := int(freeBytes * 100 / totalBytes)`
      (guard against totalBytes==0).
    - `Run(ctx context.Context, interval time.Duration)` — blocking loop that calls Check
      every interval (default 30s — caller passes the value from config) and updates the
      `library_disk_free_bytes` gauge. Exits on `&lt;-ctx.Done()`. Logs warn but does not
      fail when Check errors.
    - `Allow(minFreePct int) (allowed bool, freePct int, err error)` — convenience wrapper
      the enqueue handler calls. `allowed == freePct >= minFreePct`. Returns the freePct
      for logging.

    Tests: stub Statfs by injecting a `func(path string, st *unix.Statfs_t) error`
    indirection on `DiskGuard` (default = `unix.Statfs`, tests overwrite with a fake that
    populates Blocks/Bavail/Bsize). Cover (a) Check returns expected freePct given known
    Blocks/Bavail, (b) Allow returns false when freePct &lt; minFreePct, (c) Run updates
    the gauge after one tick (use a 10ms interval and `time.After(50 * time.Millisecond)`
    to observe one tick).

    Add the `golang.org/x/sys` direct dependency to `services/library/go.mod` via
    `go get golang.org/x/sys/unix` — it's likely already in go.sum as an indirect via
    anacrolix.
  </action>
  <verify>
    <automated>cd services/library &amp;&amp; go test ./internal/metrics/... ./internal/service/... -count=1 -run "LibraryMetrics|DiskGuard"</automated>
  </verify>
  <done>
    All six SPEC-locked metric names exist on `/metrics` after `make redeploy-library`.
    Disk guard returns correct freePct for a fake Statfs result and emits a gauge update
    after one polling tick. `Allow()` correctly gates on `LIBRARY_DISK_FREE_MIN_PCT`.
  </done>
</task>

<task type="auto">
  <name>Task 4: Download worker + stall detection + concurrency integration test</name>
  <files>
    services/library/internal/service/download_worker.go,
    services/library/internal/service/download_worker_test.go
  </files>
  <action>
    Create `WorkerPool` in `internal/service/download_worker.go`. Constructor:
    `NewWorkerPool(workers int, jobRepo *repo.JobRepository, tc *torrent.Client,
    libMetrics *metrics.LibraryMetrics, stallTimeout time.Duration, progressTick
    time.Duration, log *logger.Logger) *WorkerPool`. Internal state:
    `handles map[string]torrent.DownloadHandle` keyed by job ID, `handlesMu sync.RWMutex`.

    Public methods:
    - `Start(ctx context.Context)`: launches `workers` goroutines. Each goroutine runs
      a loop: `Claim` → handle one job → loop. On empty queue, sleep 2s (configurable
      via a `pollInterval` field, default 2s — claim contention with N workers will
      naturally space out fetches) then retry. Exits on `&lt;-ctx.Done()`.
    - `CancelJob(ctx, jobID string) error`: status flip FIRST (`jobRepo.Cancel`) THEN
      look up the in-memory handle, call `handle.Cancel()`, remove from map. The
      worker's next progress tick observes status=='cancelled' and exits cleanly. Order
      matters per CONTEXT — flipping status first guarantees that even if the worker
      is between ticks, the next tick aborts before any further progress writes.
    - `Stop(ctx context.Context, timeout time.Duration) error`: cancels all in-memory
      handles, waits up to timeout for goroutines, then for any job still marked
      `status='downloading'` (because workers were mid-flight) writes them back to
      `queued` via UpdateStatus (resumption-on-restart semantics — but at shutdown,
      not startup). Logs the count.
    - `ActiveCount() int`: returns len(handles) — drives `library_active_torrents` gauge
      via a 5s tick goroutine started in Start().

    Per-job loop (`processJob(ctx, *Job)`):
    1. Increment `library_jobs_total{status="downloading"}`.
    2. Call `tc.Add(ctx, job.Magnet)`; on error → `UpdateStatus(failed, err.Error())`
       + `IncJobsTotal("failed")` and return.
    3. Record handle in the map (under handlesMu).
    4. Tick every `progressTick` (default 5s): call `handle.Progress()`; compute new
       downloadedBytes delta vs lastReported; `libMetrics.AddDownloadBytes(delta)`;
       `jobRepo.UpdateProgress(...)`. Track `lastNonZeroPeerAt` — if a tick sees
       `peers==0`, don't update it; otherwise set to time.Now().
    5. Stall check each tick: if `time.Since(lastNonZeroPeerAt) > stallTimeout`,
       call `handle.Cancel()`, `UpdateStatus(failed, "stalled: no peers for 30 minutes")`,
       `IncJobsTotal("failed")`, remove handle, return.
    6. On each tick, re-read the job status — if it's `cancelled`, call `handle.Cancel()`,
       remove from map, return (no UpdateStatus — the DELETE handler already wrote
       cancelled). Increment `IncJobsTotal("cancelled")`.
    7. On `&lt;-handle.Done()` (download complete OR Cancel): if status is still
       downloading, write `status='encoding'` (NOT `done` — Phase 4 picks up at encoding).
       Increment `IncJobsTotal("encoding")`. Remove handle from map.
    8. Always defer `handle.Cancel()` to release peers in defensive cases.

    Tests (most run with a stub `torrentClient` interface — extract a minimal interface
    `torrentAdder` in download_worker.go so unit tests can fake it without spinning up
    anacrolix). Cover at unit level:
    - processJob: torrentClient.Add error → UpdateStatus(failed) is called.
    - processJob: handle.Done() closes → UpdateStatus(encoding) is called once, not Done.
    - stall: lastNonZeroPeerAt is older than stallTimeout → UpdateStatus(failed) with
      the SPEC-locked error_text "stalled: no peers for 30 minutes".
    - CancelJob: status flip happens before handle.Cancel().
    - Start/Stop: graceful shutdown drops in-flight handles and rewrites
      status='downloading' rows to status='queued'.

    Integration test (`//go:build integration`, INTEGRATION=1) drives two worker
    goroutines against a real Postgres + a stub torrentAdder; seeds two queued jobs;
    asserts both transition to downloading (or beyond) within ~3s and the two job IDs
    are different. This satisfies Acceptance 2 (concurrent claim).
  </action>
  <verify>
    <automated>cd services/library &amp;&amp; go test ./internal/service/... -count=1 -run "Worker" &amp;&amp; INTEGRATION=1 go test -tags=integration ./internal/service/... -count=1 -run "TestWorkerPool_TwoWorkersClaimTwoJobs"</automated>
  </verify>
  <done>
    Unit tests cover the five behaviors above. INTEGRATION test verifies two parallel
    workers each claim a different job. Worker loop honors the SPEC's locked ordering
    (cancel status flip → handle.Cancel → next tick observes cancelled) and stops at
    `status='encoding'` (does not invoke any ffmpeg/upload — verified by absence of
    those imports).
  </done>
</task>

<task type="auto">
  <name>Task 5: Handler + router + config + main.go wiring + .env.example</name>
  <files>
    services/library/internal/handler/jobs.go,
    services/library/internal/handler/jobs_test.go,
    services/library/internal/transport/router.go,
    services/library/internal/config/config.go,
    services/library/cmd/library-api/main.go,
    services/library/go.mod,
    services/library/go.sum,
    docker/.env.example
  </files>
  <action>
    `internal/handler/jobs.go` exposes `JobsHandler` with deps:
    `jobRepo *repo.JobRepository`, `diskGuard *service.DiskGuard`, `pool *service.WorkerPool`,
    `libMetrics *metrics.LibraryMetrics`, `minFreePct int`, `log *logger.Logger`.

    Endpoints (admin-gated at the gateway, no per-handler auth):
    - `POST /api/library/jobs`:
        1. Bind body `{magnet, title, source, uploader?, quality?, size_bytes?, shikimori_id?}`.
        2. Validate `source ∈ {nyaa, animetosho, manual}` and `title != ""` and `magnet != ""`.
           Reject with `httputil.BadRequest`.
        3. `metainfo.ParseMagnetUri(magnet)` — on error → `IncEnqueueRejected("invalid_magnet")`,
           400 with `BadRequest("invalid magnet")`.
        4. `diskGuard.Allow(minFreePct)` — on `allowed==false` → `IncEnqueueRejected("disk_full")`
           and `httputil.JSON(w, http.StatusInsufficientStorage /* 507 */, map[string]string{
           "error":"disk_full"})`.
        5. Build `domain.Job{Source, Magnet, Title, Uploader, Quality, SizeBytes,
           ShikimoriID, Status: JobStatusQueued}` and call `jobRepo.Create`. Server fills
           id/timestamps via GORM defaults. Increment `IncJobsTotal("queued")`.
        6. Return `httputil.Created(w, job)` with the created row.
    - `GET /api/library/jobs?status=&limit=`:
        1. Parse status (comma-separated, optional) into `[]JobStatus`; unknown values → 400.
        2. Parse limit (default 100, clamp 1..500).
        3. `jobRepo.List(ctx, JobFilter{Statuses, Limit})` → `httputil.OK(w, map[string]any{
           "jobs": jobs})` (always non-nil slice — convert nil → `[]Job{}`).
    - `GET /api/library/jobs/{id}`:
        - `chi.URLParam(r, "id")`; `jobRepo.GetByID` → `httputil.OK(w, job)`;
          NotFound → 404 via `httputil.Error`.
    - `DELETE /api/library/jobs/{id}`:
        - Call `pool.CancelJob(ctx, id)` (which flips status FIRST, then in-memory cancel).
        - If repo returns NotFound → 404. On success → 204 `httputil.NoContent(w)`.
          Increment `IncJobsTotal("cancelled")` on the success path only.

    Handler tests (`jobs_test.go`) use `httptest.NewRecorder` + a stub jobRepo + stub
    pool + stub diskGuard. Cover:
    - POST valid → 201 + body shape.
    - POST invalid magnet → 400 and `IncEnqueueRejected("invalid_magnet")` was called.
    - POST when disk guard rejects → 507 with body `{"error":"disk_full"}` and
      `IncEnqueueRejected("disk_full")` was called.
    - POST missing title → 400.
    - POST unknown source → 400.
    - GET list with status filter → repo.List was called with the right filter.
    - GET by id NotFound → 404 envelope.
    - DELETE → 204 and `pool.CancelJob(id)` was called.
    - DELETE on unknown id → 404.

    `transport/router.go`: pass `jobsHandler *handler.JobsHandler` into NewRouter.
    Inside `r.Route("/api/library", …)` add:
    ```
    r.Post("/jobs", jobsHandler.Create)
    r.Get("/jobs", jobsHandler.List)
    r.Get("/jobs/{id}", jobsHandler.Get)
    r.Delete("/jobs/{id}", jobsHandler.Delete)
    ```
    Keep the `_ = jwtConfig` line; gateway-side admin gate from Phase 2 still covers all
    `/api/library/*` non-/health routes.

    `internal/config/config.go`: add three new sub-configs.
    - `TorrentConfig{DownloadDir string, MaxPeers int, UploadRateKBPS int,
       SeedDuration time.Duration, StallTimeout time.Duration}`
    - `WorkerConfig{Count int, ProgressTick time.Duration}`
    - `DiskConfig{MinFreePct int, PollInterval time.Duration}`
    Wire env vars per CONTEXT (locked):
    - `LIBRARY_TORRENT_DOWNLOAD_DIR` (default `/data/torrents`)
    - `LIBRARY_TORRENT_MAX_PEERS` (default 80)
    - `LIBRARY_TORRENT_MAX_UPLOAD_RATE_KBPS` (default 1024)
    - `LIBRARY_TORRENT_SEED_DURATION` (default `24h`)
    - `LIBRARY_TORRENT_STALL_TIMEOUT` (default `30m`)
    - `LIBRARY_DOWNLOAD_WORKERS` (default 2)
    - `LIBRARY_DOWNLOAD_PROGRESS_TICK` (default `5s`)
    - `LIBRARY_DISK_FREE_MIN_PCT` (default 20)
    - `LIBRARY_DISK_POLL_INTERVAL` (default `30s`)

    `cmd/library-api/main.go`: extend the existing wiring (do NOT regress Phase 2's
    search wiring) — after `database.New(...)`:
    1. Load + exec the embedded migration: `//go:embed migrations/001_library_jobs.sql`
       → `db.DB.Exec(string(migrationSQL))`. Fail-fast on error.
    2. `jobRepo := repo.NewJobRepository(db.DB)`.
    3. `resumed, _ := jobRepo.ResumeInterruptedDownloads(rootCtx)`; log the count.
    4. `libMetrics := metrics.NewLibraryMetrics()` (registered against the default
       prometheus registry so the existing `/metrics` endpoint picks them up).
    5. `tc, err := torrent.NewClient(torrent.Config{...})` from `cfg.Torrent`. Defer
       `tc.Close()`. Fail-fast on err.
    6. `diskGuard := service.NewDiskGuard(cfg.Torrent.DownloadDir, libMetrics, log)`.
       Launch `go diskGuard.Run(rootCtx, cfg.Disk.PollInterval)`.
    7. `pool := service.NewWorkerPool(cfg.Worker.Count, jobRepo, tc, libMetrics,
       cfg.Torrent.StallTimeout, cfg.Worker.ProgressTick, log)`.
       Launch `go pool.Start(rootCtx)`.
    8. `jobsHandler := handler.NewJobsHandler(jobRepo, diskGuard, pool, libMetrics,
       cfg.Disk.MinFreePct, log)`.
    9. Pass `jobsHandler` into `transport.NewRouter(...)`.
    10. On shutdown (after `srv.Shutdown`), call `pool.Stop(ctx, 30*time.Second)` and
        `tc.Close()`. The existing SIGTERM channel handles the cancel.

    `services/library/go.mod`: ensure direct requires include
    `github.com/anacrolix/torrent v1.61.0` (already there from Phase 2) and
    `golang.org/x/sys` (add). Run `go mod tidy`. Use `git checkout --` on any other
    workspace `go.mod`/`go.sum` files `go mod tidy` may touch, per Phase 2's
    Deviation #4 pattern.

    `docker/.env.example`: append a new "Library service: torrent + worker + disk"
    block below the Phase 2 search block documenting all nine new env vars with
    one-line descriptions. Do NOT touch the Phase 2 entries.
  </action>
  <verify>
    <automated>cd services/library &amp;&amp; go build ./... &amp;&amp; go vet ./... &amp;&amp; go test ./internal/handler/... -count=1</automated>
  </verify>
  <done>
    `services/library` builds clean with the new handler, router, config, main wiring.
    Handler unit tests pass for the eight scenarios above. After `make redeploy-library`:
    POST /api/library/jobs (admin JWT) with a valid magnet returns 201 + the new row;
    the same body without a JWT returns 401 at the gateway; an invalid magnet returns
    400; setting `LIBRARY_DISK_FREE_MIN_PCT=99` in compose reproduces 507; GET
    /api/library/jobs returns the queued job; DELETE flips it to cancelled.
  </done>
</task>

<task type="auto">
  <name>Task 6: Grafana dashboard JSON</name>
  <files>infra/grafana/dashboards/library.json</files>
  <action>
    Author `infra/grafana/dashboards/library.json` mirroring the shape of the existing
    `infra/grafana/dashboards/scraper-provider-health.json` so the existing Grafana
    provisioning auto-loads it on container restart. Use Grafana schema version matching
    the existing dashboards (`"schemaVersion": 39`-ish, `"pluginVersion": "10.3.3"` —
    copy from scraper-provider-health.json). Datasource UID: `${DS_PROMETHEUS}`.

    Required panels (one per SPEC-locked metric, plus a status-mix panel):
    1. **Job status counts (24h)** — `barchart`, `sum by (status) (increase(library_jobs_total[24h]))`,
       legend `{{status}}`.
    2. **Active torrents** — `timeseries`/`stat`, `library_active_torrents`.
    3. **Download throughput** — `timeseries`,
       `rate(library_download_bytes_total[5m])`, unit `Bps`.
    4. **Disk free** — `stat`/`gauge`, `library_disk_free_bytes`, unit `bytes`.
       Threshold red at &lt;20% (use absolute value if you know container disk size, else
       leave the threshold relative via a transform).
    5. **Enqueue rejects** — `barchart`, `sum by (reason) (increase(library_enqueue_rejected_total[24h]))`,
       legend `{{reason}}`.
    6. **Seeding torrents** — `stat`, `library_torrent_seed_count`.
    7. **Job-status mix (pie)** — `piechart`,
       `sum by (status) (library_jobs_total)`. Set unit `short`.

    Dashboard title: `"Library Service — Self-Hosted Torrent Pipeline"`. UID: `"library"`.
    Tags: `["library","raw-jp","v0.2"]`. Include a top-level `"description"` field
    summarizing the dashboard (see scraper dashboard for tone).

    Validate the JSON syntactically (`jq . infra/grafana/dashboards/library.json
    &gt; /dev/null`). After deploy, Grafana's auto-provisioner picks it up — verify
    via the existing Grafana URL pattern in CLAUDE.md (admin path routing, not subdomain).

    Do NOT touch grafana provisioning configs — Phase 1+2 already proved the path
    auto-loads `*.json` from `infra/grafana/dashboards/`.
  </action>
  <verify>
    <automated>jq . infra/grafana/dashboards/library.json &gt; /dev/null &amp;&amp; grep -c "library_jobs_total\|library_active_torrents\|library_disk_free_bytes\|library_download_bytes_total\|library_enqueue_rejected_total\|library_torrent_seed_count" infra/grafana/dashboards/library.json</automated>
  </verify>
  <done>
    `library.json` parses as valid JSON, contains references to all six SPEC-locked
    metric names (grep returns a count &gt;= 6), uses the same `${DS_PROMETHEUS}`
    datasource template and `pluginVersion`/`schemaVersion` shape as the existing
    scraper dashboard. After `docker compose restart grafana`, the dashboard appears
    in the Grafana UI without manual import.
  </done>
</task>

<task type="auto">
  <name>Task 7: Live smoke + admin-auth verification + SUMMARY.md</name>
  <files>
    .planning/workstreams/raw-jp/phases/03-torrent-client-job-queue-metrics/03-SUMMARY.md
  </files>
  <action>
    Run the standard ship-the-phase smoke set against the deployed library service.
    Use a temporary admin API key minted against the `tNeymik` admin user via the
    direct-DB pattern documented in `MEMORY.md` (under "API Key Authentication");
    revoke the key after smoke — Phase 2 used the same pattern. `ui_audit_bot` is
    role=user, so it cannot exercise admin-gated jobs routes (Phase 2 open item).

    Run these against the live deployment:

    1. `cd services/library && go build ./... && go vet ./... && go test ./... -count=1`
       → all green.
    2. `make redeploy-library && make redeploy-gateway && make health`
       → `✓ library:8089` and no other regressions.
    3. `curl -sI http://localhost:8000/api/library/jobs` (no auth)
       → HTTP/1.1 401 (gateway gate is intact).
    4. `curl -X POST http://localhost:8000/api/library/jobs -H "Authorization: Bearer $ADMIN_KEY"
       -H "Content-Type: application/json" -d '{"magnet":"not-a-magnet","title":"x","source":"manual"}'`
       → 400 + body mentions "invalid magnet".
    5. `curl -X POST http://localhost:8000/api/library/jobs -H "Authorization: Bearer $ADMIN_KEY"
       -H "Content-Type: application/json"
       -d '{"magnet":"&lt;a real small public-domain magnet — sintel or ubuntu&gt;","title":"smoke",
            "source":"manual"}'`
       → 201 with `id`, `status:"queued"`. Then `GET /api/library/jobs/{id}` after ~30s
       shows `status:"downloading"` and a non-zero `progress_pct`. After another ~60s the
       data dir under the library container's `/data/torrents` shows the file.
    6. `curl -X DELETE http://localhost:8000/api/library/jobs/{id} -H "Authorization: Bearer $ADMIN_KEY"`
       → 204. `GET .../{id}` shortly after shows `status:"cancelled"`.
    7. `curl -s http://localhost:8089/metrics | grep ^library_` → all six metric names
       appear with at least one labelset.
    8. Bounce the library container with `LIBRARY_DISK_FREE_MIN_PCT=99` env, repost the
       same magnet → 507 with `{"error":"disk_full"}`. Restore env after.
    9. Open the Grafana UI → confirm "Library Service — Self-Hosted Torrent Pipeline"
       dashboard is loaded automatically.

    After the smokes succeed, REVOKE the temporary admin API key:
    `docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres
     -d animeenigma -c "UPDATE users SET api_key_hash = NULL WHERE username = 'tNeymik';"`.

    Write `03-SUMMARY.md` mirroring `02-SUMMARY.md`'s shape:
    - YAML front-matter (`phase`, `status: complete`, `workstream: raw-jp`,
      `milestone: v0.2`, `date`, `requirements: [LIB-05, LIB-06, LIB-NF-01, LIB-NF-02,
      LIB-NF-03]`, commit hashes once known).
    - "What was built" section keyed by task — one paragraph each.
    - "Files touched" with NEW vs EXTEND split.
    - "Verification results" containing the actual smoke command output (the nine
      blocks above), each formatted as a code fence.
    - "Deviations from plan" — list each adjustment with severity tag per Phase 2's
      pattern. (`[Rule N — …]`)
    - "Out of scope (per SPEC)" — verbatim copy from CONTEXT's deferred ideas plus
      anything that emerged during smoke (Phase 4 owns encoding/upload).
    - "Open items" — at minimum carry forward Phase 1/2 open items (port-doc fixes,
      ui_audit_bot non-admin) plus anything new (e.g. stall-detection 30m default
      makes integration testing slow — consider env override docs for Phase 4).
    - "Self-Check: PASSED" — list every file in `files_modified` and confirm presence
      (run `ls` to verify). List the commit hashes from `git log --oneline`.

    Commit messages (use `Co-Authored-By` block per MEMORY.md):
    - Task 1: `feat(03): add library_jobs schema + Job domain/repo + repo tests`
    - Task 2: `feat(03): embed anacrolix/torrent behind clean facade`
    - Task 3: `feat(03): library Prometheus metrics + disk-free guard`
    - Task 4: `feat(03): download worker + stall detection + concurrent claim`
    - Task 5: `feat(03): wire jobs handler, router, config, main, .env`
    - Task 6: `feat(03): library Grafana dashboard JSON`
    - Task 7: `docs(03): SUMMARY for torrent client + job queue + metrics phase`

    Finally, after the SUMMARY is committed, invoke `/animeenigma-after-update` per
    CLAUDE.md's "After-Update Skill (MUST USE)" so the changelog entry, lint+build
    verification, redeploy, health checks, and push happen with full traceability.
    The changelog entry should be admin-oriented (this is invisible to end users
    until Phase 5 ships the UI) — phrase it as
    "Admin: backend now downloads JP raws via embedded torrent client with job queue,
    metrics, and Grafana dashboard. UI lands in v0.2 Phase 5."
  </action>
  <verify>
    <automated>ls .planning/workstreams/raw-jp/phases/03-torrent-client-job-queue-metrics/03-SUMMARY.md &amp;&amp; grep -c "Self-Check: PASSED" .planning/workstreams/raw-jp/phases/03-torrent-client-job-queue-metrics/03-SUMMARY.md</automated>
  </verify>
  <done>
    All nine smoke checks pass against the live deployment. `03-SUMMARY.md` exists with
    the same shape as `02-SUMMARY.md`, includes the verification command outputs, and
    Self-Check lists every file from `files_modified` as FOUND. Temporary admin API key
    is revoked. `/animeenigma-after-update` ran, the changelog entry is committed, and
    the phase is pushed to origin/main.
  </done>
</task>

</tasks>

<verification>

After all seven tasks complete, the following invariants must hold:

1. `cd services/library && go build ./... && go vet ./... && go test ./... -count=1` is green.
2. `INTEGRATION=1 go test -tags=integration ./services/library/internal/repo/... ./services/library/internal/service/... -count=1` is green (requires running Postgres).
3. `make redeploy-library && make health` reports `✓ library:8089` and no regression on other services.
4. `curl -s http://localhost:8089/metrics | grep -c '^library_'` returns at least 6 distinct metric series.
5. The end-to-end smoke (POST magnet → GET .../{id} shows downloading → file lands under `/data/torrents`) works against a real public-domain magnet.
6. DELETE flips a running job to `cancelled` within ~2s and the in-memory handle is released (`library_active_torrents` decrements).
7. `LIBRARY_DISK_FREE_MIN_PCT=99` reproduces a 507 response on enqueue and increments `library_enqueue_rejected_total{reason="disk_full"}`.
8. Grafana auto-loads `infra/grafana/dashboards/library.json` at restart with the title `"Library Service — Self-Hosted Torrent Pipeline"`.
9. On service restart, any `status='downloading'` row is rewritten to `status='queued'` exactly once and the worker re-claims it.
10. Stall detection (verified at unit level for the time-since logic; integration test optionally with a shortened `LIBRARY_TORRENT_STALL_TIMEOUT=10s` env override) transitions a no-peer job to `failed` with `error_text="stalled: no peers for 30 minutes"`.
11. The Phase 3 boundary is intact — no code under `services/library/internal/{ffmpeg,minio}/` is touched; workers stop at `status='encoding'`.
12. The temporary admin API key minted for smoke is REVOKED before the phase commits.

</verification>

<success_criteria>

Phase 3 ships when:

- All seven tasks' `<done>` blocks are satisfied (verified by their `<verify>` commands).
- All nine smoke checks in Task 7 pass against the live deployment.
- `03-SUMMARY.md` exists, follows the Phase 2 SUMMARY shape, and ends with `Self-Check: PASSED`.
- `make health` returns `✓` for all services including `library:8089`.
- The Grafana dashboard is visible at the admin Grafana URL without manual import.
- `library_jobs_total{status="downloading"}` has incremented at least once (proves the worker path executed).
- `/animeenigma-after-update` has run, the changelog is updated, and the work is pushed to origin/main.
- No file under `services/library/internal/ffmpeg/` or `services/library/internal/minio/` exists yet (Phase 4 boundary intact).

</success_criteria>

<output>
After completion, ensure
`.planning/workstreams/raw-jp/phases/03-torrent-client-job-queue-metrics/03-SUMMARY.md`
exists and is committed. Update `.planning/workstreams/raw-jp/STATE.md` to mark Phase 3
complete and Phase 4 (ffmpeg + MinIO) as the next active phase.
</output>
