---
phase: 05-rawlibrary-vue-admin-ui
plan: 01
type: execute
wave: 1
workstream: raw-jp
milestone: v0.2
depends_on: []
files_modified:
  - services/library/internal/handler/health.go
  - services/library/internal/handler/health_test.go
  - services/library/internal/handler/jobs.go
  - services/library/internal/handler/jobs_test.go
  - services/library/internal/repo/job.go
  - services/library/internal/repo/job_test.go
  - services/library/internal/repo/job_integration_test.go
  - services/library/internal/minio/writer.go
  - services/library/internal/minio/writer_test.go
  - services/library/internal/service/encoder_worker.go
  - services/library/internal/service/encoder_worker_test.go
  - services/library/internal/transport/router.go
  - services/library/cmd/library-api/main.go
  - frontend/web/src/types/library.ts
  - frontend/web/src/api/client.ts
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/locales/ja.json
  - frontend/web/src/views/admin/RawLibrary.vue
  - frontend/web/src/router/index.ts
  - frontend/web/e2e/raw-library-admin.spec.ts
  - .planning/workstreams/raw-jp/phases/05-rawlibrary-vue-admin-ui/05-SUMMARY.md
autonomous: true
requirements:
  - LIB-09
user_setup: []

must_haves:
  truths:
    - "Admin navigating to /admin/raw-library sees a stats strip (disk free, active torrents, active jobs) populated from /api/library/health/extended"
    - "Admin can search Nyaa + AnimeTosho from the search panel and see provider chips (cyan/purple) per row"
    - "Admin clicks Queue on a search result and the new job appears in the jobs panel within 5s with status=queued or downloading"
    - "Admin sees per-job progress bars that update every 5s while jobs are downloading/encoding/uploading"
    - "Admin clicks Cancel on an active job and it transitions to cancelled (confirm dialog only for downloading|encoding|uploading)"
    - "Admin sees Failed jobs subsection with error_text + Retry button; clicking Retry creates a new queued row"
    - "Admin sees Pending-link subsection for done jobs with NULL shikimori_id; selecting an anime via debounced /api/anime/search dropdown links it"
    - "Linking a done job triggers server-side MinIO CopyObject from pending/{job_id}/{ep}/ to {shikimori_id}/{ep}/ and inserts library_episodes"
    - "Non-admin visiting /admin/raw-library is redirected home with admin.errors.notAdmin sessionStorage banner"
    - "bun run build passes with zero errors; bunx tsc --noEmit clean"
  artifacts:
    - path: "services/library/internal/handler/health.go"
      provides: "GET /health/extended returning disk + active-torrents + active-jobs-by-status JSON"
      contains: "HealthExtended"
    - path: "services/library/internal/handler/jobs.go"
      provides: "PATCH /jobs/{id} (Link) + POST /jobs/{id}/retry (Retry)"
      contains: "Link, Retry"
    - path: "services/library/internal/minio/writer.go"
      provides: "Move(srcPrefix, dstPrefix) server-side CopyObject helper"
      contains: "func (w *Writer) Move"
    - path: "services/library/internal/repo/job.go"
      provides: "UpdateShikimoriID + Retry repo methods"
      contains: "UpdateShikimoriID, Retry"
    - path: "frontend/web/src/types/library.ts"
      provides: "Release, Job, JobStatus, Episode, LibraryHealth, CreateJobPayload TypeScript types"
      exports: ["Release", "Job", "JobStatus", "Episode", "LibraryHealth", "CreateJobPayload"]
    - path: "frontend/web/src/api/client.ts"
      provides: "adminLibraryApi block with search/listJobs/getJob/createJob/cancelJob/linkJob/retryJob/healthExtended"
      contains: "adminLibraryApi"
    - path: "frontend/web/src/views/admin/RawLibrary.vue"
      provides: "Three-section admin view: stats strip + search panel + jobs panel"
      min_lines: 250
    - path: "frontend/web/src/router/index.ts"
      provides: "/admin/raw-library route with requiresAuth + requiresAdmin meta"
      contains: "/admin/raw-library"
    - path: "frontend/web/src/locales/en.json"
      provides: "player.adminLibrary.* i18n keys"
      contains: "adminLibrary"
    - path: "frontend/web/src/locales/ru.json"
      provides: "player.adminLibrary.* i18n keys"
      contains: "adminLibrary"
    - path: "frontend/web/src/locales/ja.json"
      provides: "player.adminLibrary.* i18n keys"
      contains: "adminLibrary"
    - path: "frontend/web/e2e/raw-library-admin.spec.ts"
      provides: "Playwright e2e: login → view → search → queue → cancel"
      min_lines: 60
  key_links:
    - from: "frontend/web/src/views/admin/RawLibrary.vue"
      to: "/api/library/health/extended"
      via: "adminLibraryApi.healthExtended in 30s setInterval"
      pattern: "healthExtended"
    - from: "frontend/web/src/views/admin/RawLibrary.vue"
      to: "/api/library/search"
      via: "adminLibraryApi.search after 300ms debounce on input"
      pattern: "adminLibraryApi\\.search"
    - from: "frontend/web/src/views/admin/RawLibrary.vue"
      to: "/api/library/jobs"
      via: "adminLibraryApi.listJobs in 5s setInterval + createJob/cancelJob/linkJob/retryJob from row buttons"
      pattern: "listJobs|createJob|cancelJob|linkJob|retryJob"
    - from: "frontend/web/src/views/admin/RawLibrary.vue"
      to: "/api/anime/search"
      via: "animeApi.search debounced 300ms in pending-link dropdown"
      pattern: "animeApi\\.search"
    - from: "services/library/internal/handler/jobs.go (Link)"
      to: "minio.Writer.Move + repo.EpisodeRepository.Create"
      via: "Link handler calls Move(pending/{job_id}/{ep}/, {shikimori_id}/{ep}/) then inserts episode row"
      pattern: "uploader\\.Move|episodeRepo\\.Create"
    - from: "services/library/internal/handler/jobs.go (Retry)"
      to: "repo.JobRepository.Retry"
      via: "Retry handler calls repo.Retry which inserts a new queued row inheriting magnet/title/uploader/shikimori_id"
      pattern: "jobRepo\\.Retry"
    - from: "frontend/web/src/router/index.ts"
      to: "frontend/web/src/views/admin/RawLibrary.vue"
      via: "lazy import + beforeEach guard (requiresAuth + requiresAdmin)"
      pattern: "admin/RawLibrary\\.vue"
---

<objective>
Ship the RawLibrary.vue admin view (LIB-09) plus the three small backend
endpoints it needs (`GET /api/library/health/extended`,
`PATCH /api/library/jobs/{id}`, `POST /api/library/jobs/{id}/retry`) and the
`minio.Writer.Move` helper that powers the post-hoc shikimori link.

Purpose: Give the operator the only operational surface for the v0.2
self-hosted library (search Nyaa + AnimeTosho, queue magnets, monitor
running jobs, retry failures, link orphan jobs to catalog anime, view
disk + peer + active-job stats). Without this view admins would `curl`
the library API by hand — not viable.

Output:
- Three backend endpoint extensions + `minio.Move` + repo methods + unit/integration tests.
- One Vue view + route entry + types module + `adminLibraryApi` block + i18n in en/ru/ja.
- One Playwright e2e spec.
- One `05-SUMMARY.md` documenting commits, deviations, smoke results.

This is the LAST admin-UI phase of v0.2; Phase 6 is hybrid resolver
(backend wiring only, no UI changes).
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/raw-jp/phases/05-rawlibrary-vue-admin-ui/05-CONTEXT.md
@.planning/workstreams/raw-jp/milestones/v0.2-phases/05-rawlibrary-admin-ui/05-SPEC.md
@.planning/workstreams/raw-jp/milestones/v0.2-REQUIREMENTS.md
@.planning/workstreams/raw-jp/phases/03-torrent-client-job-queue-metrics/03-SUMMARY.md
@.planning/workstreams/raw-jp/phases/04-ffmpeg-hls-transcoder-minio-writer/04-SUMMARY.md

# Frontend reuse targets
@frontend/web/src/router/index.ts
@frontend/web/src/stores/auth.ts
@frontend/web/src/api/client.ts
@frontend/web/src/components/ui/Badge.vue
@frontend/web/src/views/admin/AdminRecs.vue
@frontend/web/src/views/admin/AdminCollections.vue

# Backend extension targets
@services/library/internal/handler/health.go
@services/library/internal/handler/jobs.go
@services/library/internal/repo/job.go
@services/library/internal/minio/writer.go
@services/library/internal/service/encoder_worker.go
@services/library/internal/transport/router.go
@services/library/internal/domain/job.go
@services/library/internal/domain/episode.go

<interfaces>
<!-- Key Go types/contracts the executor needs. Extracted from codebase. -->

From `services/library/internal/domain/job.go`:
- `type JobStatus string` with constants `JobStatusQueued`, `JobStatusDownloading`, `JobStatusEncoding`, `JobStatusUploading`, `JobStatusDone`, `JobStatusFailed`, `JobStatusCancelled`.
- `type Job struct { ID, Source, Magnet, Title, Uploader, Quality, SizeBytes, ShikimoriID, Status, ProgressPct, ErrorText, CreatedAt, UpdatedAt, CompletedAt }` — `TableName()` returns `"library_jobs"`.

From `services/library/internal/domain/episode.go`:
- `type Episode struct { ID, ShikimoriID, EpisodeNumber, JobID *string, MinioPath, DurationSec *int, SizeBytes *int64, CreatedAt }` — `TableName()` returns `"library_episodes"`.

From `services/library/internal/handler/jobs.go`:
- `type JobStoreAPI interface { Create, GetByID, List }` — extend with `UpdateShikimoriID(ctx, id, shikimoriID) error` and `Retry(ctx, oldID) (*domain.Job, error)`.
- `type JobsHandler` already wires `jobRepo`, `diskGuard`, `canceller`, `metrics`, `minFreePct`, `log`. Extend constructor to also accept `minioMover MinioMover`, `episodeStore EpisodeStore`, and `detector EpisodeDetector` (defined in `internal/service/encoder_worker.go` as `EpisodeDetector`).
- `httputil.OK(w, body)`, `httputil.Created(w, body)`, `httputil.NoContent(w)`, `httputil.BadRequest(w, msg)`, `httputil.Error(w, err)`, `httputil.JSON(w, status, body)`.
- `chi.URLParam(r, "id")` reads path params.

From `services/library/internal/handler/health.go`:
- `type HealthHandler struct{}`. Extend to `HealthHandler struct { diskGuard, torrentCounter, jobLister }`; add `HealthExtended` method returning `{disk_free_bytes, disk_total_bytes, active_torrents, active_jobs_by_status: map[string]int}`.

From `services/library/internal/service/disk_guard.go`:
- `DiskGuard.Check() (freeBytes, totalBytes int64, freePct int, err error)` — already exists; reuse for `health/extended`.

From `services/library/internal/service/download_worker.go`:
- `WorkerPool.ActiveCount() int` — exposes active in-memory torrent handle count.

From `services/library/internal/repo/job.go`:
- `JobRepository.Create / GetByID / List / Claim / UpdateProgress / UpdateStatus / Cancel / ResumeInterruptedDownloads / ResumeInterruptedEncodes` — extend with `UpdateShikimoriID(ctx, id, shikimoriID) error` and `Retry(ctx, oldID) (*domain.Job, error)`.
- `JobFilter{ Statuses, Limit, Offset }`.
- `liberrors.NotFound("job")`, `liberrors.AlreadyExists(...)`, `liberrors.InvalidInput(...)`, `liberrors.Wrap(err, liberrors.CodeInternal, msg)`.

From `services/library/internal/minio/writer.go`:
- `Writer.Upload(ctx, prefix, filePaths) (int64, error)` exists. Extend with `Move(ctx, srcPrefix, dstPrefix) error` that LIST objects under `srcPrefix`, `CopyObject` each to `dstPrefix/<basename>`, then `RemoveObject` the source. Add to the `Uploader` interface: `ListObjects`, `CopyObject`, `RemoveObject`.
- minio-go/v7 SDK: `minio.Client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix, Recursive: true})` returns `<-chan minio.ObjectInfo`; `minio.Client.CopyObject(ctx, dst minio.CopyDestOptions, src minio.CopySrcOptions)` returns `minio.UploadInfo, error`; `minio.Client.RemoveObject(ctx, bucket, object, minio.RemoveObjectOptions{})`.

From `services/library/internal/parser/filename/detector.go`:
- `Detector.DetectEpisode(filename, uploader) (int, bool)` — needed when linking a `done` job: re-detect episode number from the original source filename. The encoder worker already wrote files to `pending/{job_id}/{ep}/` using a detected episode; the Link handler reuses the SAME episode number from the existing MinIO path (parse from `pending/{job_id}/{ep}/`), NOT from filename re-detection. Simpler and avoids dependency injection.

From `services/library/internal/transport/router.go`:
- `NewRouter(healthHandler, searchHandler, jobsHandler, episodesHandler, jwtConfig, log, metricsCollector)`. Extend: `jobsHandler.Link` + `jobsHandler.Retry`; `healthHandler.HealthExtended`.

From `frontend/web/src/router/index.ts`:
- Existing admin routes use `meta: { titleKey, requiresAuth: true, requiresAdmin: true }`. The global `beforeEach` guard handles the redirect. NO `beforeEnter` is needed — use the existing meta-flag pattern (matches `admin-recs`, `admin-collections`).

From `frontend/web/src/stores/auth.ts`:
- `authStore.isAdmin` is `computed(() => user.value?.role === 'admin')`.

From `frontend/web/src/api/client.ts`:
- Existing pattern: each domain has its own `export const xxxApi = { ... }` block. `apiClient` is the central axios instance; baseURL is `/api`. Use `apiClient.get/post/patch/delete`.
- Existing `animeApi.search(query, source?, pageSize?, signal?)` is the anime-search endpoint to use in the pending-link dropdown.

From `frontend/web/src/components/ui/Badge.vue`:
- `Props: { variant?: 'default'|'primary'|'secondary'|'success'|'warning'|'rating', size?: 'sm'|'md'|'lg' }`. `primary` is cyan, `secondary` is pink. The SPEC asks for **cyan for AnimeTosho, purple for Nyaa** — `primary` (cyan) maps to AnimeTosho; for Nyaa add a new `info` variant (`bg-purple-500/20 text-purple-400`) OR pick the closest existing variant. Add the `info` variant to Badge.vue's `variants` map and TypeScript union; Claude's discretion per CONTEXT.

From `frontend/web/src/locales/en.json`:
- `player` is an existing top-level namespace (line ~165). Add `adminLibrary` as a child key under `player`.
</interfaces>

</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Backend additions — health/extended + repo UpdateShikimoriID/Retry + minio.Move + handler Link/Retry + router wiring</name>
  <files>
    services/library/internal/handler/health.go,
    services/library/internal/handler/health_test.go,
    services/library/internal/handler/jobs.go,
    services/library/internal/handler/jobs_test.go,
    services/library/internal/repo/job.go,
    services/library/internal/repo/job_test.go,
    services/library/internal/repo/job_integration_test.go,
    services/library/internal/minio/writer.go,
    services/library/internal/minio/writer_test.go,
    services/library/internal/transport/router.go,
    services/library/cmd/library-api/main.go
  </files>
  <behavior>
    - `GET /api/library/health/extended` returns 200 with JSON `{disk_free_bytes:int64, disk_total_bytes:int64, active_torrents:int, active_jobs_by_status:{queued:int, downloading:int, encoding:int, uploading:int}}`. Counts derived from a single `repo.List(JobFilter{Statuses:[queued,downloading,encoding,uploading], Limit:500})` call grouped by status. DiskGuard.Check supplies free/total. WorkerPool.ActiveCount() supplies active_torrents. Auth: gateway-gated admin (existing).
    - `PATCH /api/library/jobs/{id}` body `{"shikimori_id":"57466"}` →
      • If row not found → 404.
      • If row status != "done" → 400 `{"error":"job must be done to link"}`.
      • If row.shikimori_id != "" → 400 `{"error":"job already linked"}` (idempotent re-link is OUT OF SCOPE).
      • Parse episode number from the existing MinIO prefix `pending/{job_id}/{ep}/` — find it by listing under `pending/{job_id}/` and extracting the `{ep}` segment (single integer dir). When listing finds zero objects → 500 `{"error":"orphan job has no minio objects"}`.
      • Call `minio.Writer.Move(ctx, src="pending/{job_id}/{ep}/", dst="{shikimori_id}/{ep}/")`. On error → 500 wrapped.
      • Insert `library_episodes` row keyed `(shikimori_id, episode_number)` with `minio_path="{shikimori_id}/{ep}/"`, `job_id`, copy `duration_sec`+`size_bytes` from the old episode row if one exists for the original pending path, otherwise leave NULL. On unique-violation → 409 `{"error":"episode already linked"}`.
      • Call `repo.UpdateShikimoriID(ctx, jobID, shikimoriID)` to flip the column on the job row.
      • Return 200 with the updated job row.
      • `UpdateShikimoriID` itself is a thin GORM `Updates(map{"shikimori_id": id, "updated_at": now()})` with `WHERE id = ?`; returns `NotFound` on `RowsAffected == 0`.
    - `POST /api/library/jobs/{id}/retry` → re-enqueues a failed job.
      • Lookup old row; if not found → 404; if status != "failed" → 400 `{"error":"only failed jobs can be retried"}`.
      • Call `repo.Retry(ctx, oldID)` which atomically (single transaction) inserts a NEW row with: same `source`, `magnet`, `title`, `uploader`, `quality`, `size_bytes`, `shikimori_id`; `status="queued"`; `error_text` = `"retry of " + oldID`; `progress_pct=0`. Returns the new row.
      • Old row stays in `failed` for audit.
      • Bump `library_jobs_total{status="queued"}` once.
      • Return 201 + new job row.
    - `minio.Writer.Move(ctx, srcPrefix, dstPrefix string) error`:
      • Both prefixes must end with `/` (normalize).
      • `ListObjects(bucket, ListObjectsOptions{Prefix: srcPrefix, Recursive: true})` → channel of ObjectInfos. For each: derive `dstObject = dstPrefix + strings.TrimPrefix(obj.Key, srcPrefix)`. Call `CopyObject(dst{Bucket, Object:dstObject}, src{Bucket, Object:obj.Key})`. Collect names.
      • After all copies succeed, RemoveObject each source. On any copy error: abort, do NOT remove sources, return wrapped error. On any remove error after successful copies: log warning, continue (object orphan is recoverable; data integrity preserved).
      • Extend the `Uploader` interface with `ListObjects(ctx, bucket, opts) <-chan minio.ObjectInfo`, `CopyObject(ctx, dst minio.CopyDestOptions, src minio.CopySrcOptions) (minio.UploadInfo, error)`, `RemoveObject(ctx, bucket, object string, opts minio.RemoveObjectOptions) error`. Update `minioClientAdapter` to forward to the SDK.
      • Add `ListObjectsByPrefix(ctx, prefix) ([]string, error)` helper on `Writer` — used by the Link handler to detect the episode number from the existing pending/ path.
    - Router (`transport/router.go`):
      • `r.Patch("/jobs/{id}", jobsHandler.Link)`
      • `r.Post("/jobs/{id}/retry", jobsHandler.Retry)`
      • `r.Get("/health/extended", healthHandler.HealthExtended)` — inside `/api/library` group (admin-gated by gateway).
    - main.go: pass `WorkerPool` (already constructed) into `HealthHandler` for ActiveCount; pass `minio.Writer` + `EpisodeRepository` + `JobRepository` into the new `JobsHandler` constructor signature.
    - Tests:
      • Handler tests (jobs_test.go): Link happy path (done + NULL shikimori), Link 404 not found, Link 400 not done, Link 400 already linked, Link 500 no minio objects, Retry happy, Retry 404, Retry 400 not failed.
      • Handler tests (health_test.go): HealthExtended returns expected fields with stubbed deps; verify `active_jobs_by_status` map keys and counts.
      • Repo unit tests (job_test.go): UpdateShikimoriID + Retry argument validation.
      • Repo integration test (job_integration_test.go, build tag `integration`): Retry inherits all fields, error_text contains old ID; UpdateShikimoriID updates column + NotFound on missing id.
      • Minio writer test (writer_test.go): Move success path (list + copy + remove, fake Uploader), Move copy-error aborts without remove, Move remove-error logs but returns nil, ListObjectsByPrefix sorts deterministically.
    - Acceptance: `cd services/library && go build ./... && go vet ./... && go test ./... -count=1` passes. `INTEGRATION=1 DB_HOST=127.0.0.1 ... go test -tags=integration ./internal/repo -count=1` passes.
  </behavior>
  <action>
Backend feature work that lights up the Link + Retry + HealthExtended endpoints plus the MinIO Move helper.

Order of work (RED first per `tdd="true"`):

1. **Write failing tests** for every behavior listed above before any implementation. Put each test in the listed `_test.go` file. For repo tests, mirror the Phase-3 `job_test.go` style (unit) and `job_integration_test.go` style (per-test DB via `INTEGRATION=1` env). For handler tests, mirror Phase-3 `jobs_test.go` — inject `JobStoreAPI` stubs + a new `MinioMover` stub + `EpisodeStore` stub. For minio writer tests, mirror Phase-4 `writer_test.go` — use the `Uploader` test seam (`newWriterWithUploader`).

2. **Implement `repo.UpdateShikimoriID`** in `internal/repo/job.go` — thin GORM `Updates` with `WHERE id = ?`; map `RowsAffected == 0` to `liberrors.NotFound("job")`.

3. **Implement `repo.Retry`** in `internal/repo/job.go` — single transaction: SELECT old by id, validate `status == "failed"` (otherwise `liberrors.InvalidInput("only failed jobs can be retried")`), INSERT new row inheriting magnet/title/uploader/quality/size_bytes/shikimori_id/source with `status='queued'`, `error_text='retry of '+oldID`, `progress_pct=0`. GORM `Create(&newJob)` writes server-generated ID + timestamps back. Return new job.

4. **Extend `minio.Uploader` interface + `minioClientAdapter`** with ListObjects, CopyObject, RemoveObject. For ListObjects, since the SDK returns `<-chan minio.ObjectInfo`, the interface returns the same channel type. The adapter forwards directly. For CopyObject, accept `minio.CopyDestOptions` + `minio.CopySrcOptions`, forward to `c.CopyObject`. For RemoveObject, accept `minio.RemoveObjectOptions`.

5. **Implement `Writer.Move`** + `Writer.ListObjectsByPrefix`. Move semantics per behavior block above. ListObjectsByPrefix drains the channel into a `[]string` of keys for the handler's episode-number detection.

6. **Implement `JobsHandler.Link`** (PATCH handler):
   - Parse body `{shikimori_id: string}`. Trim. Validate non-empty.
   - GetByID → 404 on NotFound.
   - Status guard: must be `done`.
   - Already-linked guard: `ShikimoriID != ""` → 400.
   - List MinIO objects under `pending/{job_id}/`. If empty → 500. Otherwise the first key segment after `pending/{job_id}/` is the episode number directory — parse to int (`strconv.Atoi`). If parse fails → 500 `{"error":"could not parse episode number from minio path"}`.
   - Call `mover.Move(ctx, "pending/{id}/{ep}/", "{shikimori_id}/{ep}/")`.
   - Insert `library_episodes` row via the injected `EpisodeStore.Create`. Catch `liberrors.AlreadyExists` → 409.
   - Call `repo.UpdateShikimoriID`.
   - GetByID again to return the fresh row → 200.

7. **Implement `JobsHandler.Retry`** (POST handler):
   - GetByID → 404 on NotFound.
   - Status guard: must be `failed`.
   - Call `repo.Retry(oldID)` — handler does NOT recompute fields itself.
   - Bump `metrics.IncJobsTotal("queued")`.
   - `httputil.Created(w, newJob)`.

8. **Implement `HealthHandler.HealthExtended`** in `internal/handler/health.go`:
   - Inject `DiskCheckProbe { Check() (free, total int64, freePct int, err error) }`, `TorrentCounter { ActiveCount() int }`, `JobLister { List(ctx, JobFilter) ([]Job, error) }` interfaces.
   - In handler: `diskFree, diskTotal, _, derr := probe.Check()` — on err log+return 500 wrapped.
   - `active := counter.ActiveCount()`.
   - `jobs, _ := lister.List(ctx, JobFilter{Statuses: [queued, downloading, encoding, uploading], Limit: 500})`.
   - Build `byStatus := map[string]int{"queued":0,"downloading":0,"encoding":0,"uploading":0}` and increment per job.
   - `httputil.OK(w, {disk_free_bytes, disk_total_bytes, active_torrents, active_jobs_by_status})`.

9. **Wire routes** in `transport/router.go` per behavior block. Add the new handlers to the `jobsHandler != nil` chi.Router group.

10. **main.go**: extend `NewHealthHandler` call to pass diskGuard + workerPool + jobRepo. Extend `NewJobsHandler` call to pass the new `minioWriter`, `episodeRepo`, etc. Verify graceful shutdown is unaffected.

11. **Run tests + verify** per acceptance block. Commit atomically: `feat(05): backend additions for raw-library admin UI — health/extended + Link/Retry handlers + minio Move + repo UpdateShikimoriID/Retry`.

When implementing, follow Phase-3 + Phase-4 conventions: cheap-validation-first ordering, structured logging via `h.log.Warnw`, `liberrors.Wrap` for DB errors, `httputil.JSON(w, status, body)` when no helper for the status code exists. Do NOT run `go mod tidy` (per Phase-4 open item — workspace genproto ambiguity).
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/library && go build ./... && go vet ./... && go test ./... -count=1 -short</automated>
  </verify>
  <done>All new unit tests pass; backend builds clean; INTEGRATION=1 repo tests pass; router registers the three new routes; one atomic commit on `feat(05): backend additions...`.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Frontend types + adminLibraryApi block in api/client.ts</name>
  <files>
    frontend/web/src/types/library.ts,
    frontend/web/src/api/client.ts
  </files>
  <behavior>
    - `frontend/web/src/types/library.ts` exports:
      • `type JobStatus = 'queued' | 'downloading' | 'encoding' | 'uploading' | 'done' | 'failed' | 'cancelled'`
      • `interface Job { id, source: 'nyaa'|'animetosho'|'manual', magnet, title, uploader?, quality?, size_bytes, shikimori_id?, status: JobStatus, progress_pct, error_text?, created_at, updated_at, completed_at? }`
      • `interface Release { title, magnet, uploader?, quality?, size_bytes, source: 'nyaa'|'animetosho', mal_id?, found_at }`
      • `interface Episode { id, shikimori_id, episode_number, job_id?, minio_path, duration_sec?, size_bytes?, created_at }`
      • `interface LibraryHealth { disk_free_bytes, disk_total_bytes, active_torrents, active_jobs_by_status: Record<string, number> }`
      • `interface CreateJobPayload { magnet, title, source: 'nyaa'|'animetosho'|'manual', uploader?, quality?, size_bytes?, shikimori_id? }`
    - `frontend/web/src/api/client.ts` adds `export const adminLibraryApi = { search, listJobs, getJob, createJob, cancelJob, linkJob, retryJob, healthExtended }` matching the locked signatures in CONTEXT D-API.
    - Unwrapping: backend responses come through `httputil.OK` envelope `{success, data}` — the existing axios interceptor pattern (see other API blocks) does NOT auto-unwrap; consumers handle `.data.data`. Mirror the rest of `client.ts` — return the raw axios response, let the consumer destructure.
    - No TypeScript errors: `bunx tsc --noEmit` passes.
  </behavior>
  <action>
1. Create `frontend/web/src/types/library.ts` with the six exported types. Mirror snake_case from Go domain structs (Job + Episode + Release fields above).
2. Open `frontend/web/src/api/client.ts`. Find the `adminApi` block (around line 468) and after it add `import type { CreateJobPayload } from '@/types/library'` at the top of the file (next to the existing `import type { WatchCombo, ... } from '@/types/preference'`).
3. Add the `adminLibraryApi` block exactly per the CONTEXT D-API spec. Endpoints are relative to `/api` (baseURL) — so e.g. `/library/search`, `/library/jobs`, `/library/jobs/${id}`, `/library/jobs/${id}/retry`, `/library/health/extended`. Use `apiClient.get/post/patch/delete` (mirror existing blocks).
4. Run `bunx tsc --noEmit` from `frontend/web` to verify types compile cleanly.
5. Commit atomically: `feat(05): adminLibraryApi + library types module`.
  </action>
  <verify>
    <automated>cd /data/animeenigma/frontend/web && bunx tsc --noEmit</automated>
  </verify>
  <done>Both files exist; `bunx tsc --noEmit` clean; types match backend domain shapes; one atomic commit.</done>
</task>

<task type="auto">
  <name>Task 3: i18n keys under player.adminLibrary.* for en + ru + ja</name>
  <files>
    frontend/web/src/locales/en.json,
    frontend/web/src/locales/ru.json,
    frontend/web/src/locales/ja.json
  </files>
  <behavior>
    Each locale gets `player.adminLibrary` added under the existing `player` top-level namespace (already present in all three files, around line ~165 in en.json):

    ```
    player: {
      adminLibrary: {
        title: <title>,
        stats: { diskFree, activeTorrents, activeJobs },
        search: {
          title, placeholder, submit, empty,
          providers: { nyaa, animetosho }
        },
        jobs: {
          title, empty, cancel, retry,
          status: { queued, downloading, encoding, uploading, done, failed, cancelled },
          failed: { title, errorText },
          pendingLink: { title, linkButton, searchPlaceholder }
        }
      }
    }
    ```

    Translations per SPEC:
    - `title`: en "Raw Library" / ru "Сырая библиотека" / ja "生ライブラリ"
    - `stats.diskFree`: en "Disk free" / ru "Свободно на диске" / ja "ディスク空き容量"
    - `stats.activeTorrents`: en "Active torrents" / ru "Активные торренты" / ja "アクティブなトレント"
    - `stats.activeJobs`: en "Active jobs" / ru "Активные задания" / ja "アクティブなジョブ"
    - `search.title`: en "Search Nyaa + AnimeTosho" / ru "Поиск Nyaa + AnimeTosho" / ja "Nyaa + AnimeTosho 検索"
    - `search.placeholder`: en "Anime title..." / ru "Название аниме..." / ja "アニメタイトル..."
    - `search.submit`: en "Search" / ru "Найти" / ja "検索"
    - `search.empty`: en "Search Nyaa + AnimeTosho to populate the library." / ru "Найдите аниме в Nyaa + AnimeTosho, чтобы пополнить библиотеку." / ja "Nyaa と AnimeTosho を検索してライブラリに追加してください。"
    - `search.providers.nyaa`: "Nyaa" / "Nyaa" / "Nyaa"
    - `search.providers.animetosho`: "AnimeTosho" / "AnimeTosho" / "AnimeTosho"
    - `jobs.title`: en "Active jobs" / ru "Активные задания" / ja "アクティブなジョブ"
    - `jobs.empty`: en "No active jobs." / ru "Нет активных заданий." / ja "アクティブなジョブはありません。"
    - `jobs.cancel`: en "Cancel" / ru "Отменить" / ja "キャンセル"
    - `jobs.retry`: en "Retry" / ru "Повторить" / ja "再試行"
    - `jobs.status.{queued,downloading,encoding,uploading,done,failed,cancelled}`:
      • en: Queued / Downloading / Encoding / Uploading / Done / Failed / Cancelled
      • ru: В очереди / Скачивание / Кодирование / Загрузка / Готово / Ошибка / Отменено
      • ja: 待機中 / ダウンロード中 / エンコード中 / アップロード中 / 完了 / 失敗 / キャンセル済み
    - `jobs.failed.title`: en "Failed jobs" / ru "Неудачные задания" / ja "失敗したジョブ"
    - `jobs.failed.errorText`: en "Error" / ru "Ошибка" / ja "エラー"
    - `jobs.pendingLink.title`: en "Pending link" / ru "Ожидают привязки" / ja "リンク待ち"
    - `jobs.pendingLink.linkButton`: en "Link" / ru "Привязать" / ja "リンク"
    - `jobs.pendingLink.searchPlaceholder`: en "Search anime..." / ru "Поиск аниме..." / ja "アニメを検索..."
  </behavior>
  <action>
1. Open each of the three locale JSON files. Find the `"player": { ... }` block. Insert a new key `"adminLibrary": { ... }` inside the player object (before its closing brace), with the nested shape and translations per the behavior block.
2. Validate JSON syntax: run `bun run build` or `node -e "require('./src/locales/en.json')"` style check — but the simplest is to run `bunx tsc --noEmit` after the keys land (vue-i18n message catalogues are JSON; tsc won't validate them but a syntactic error breaks the build).
3. Be careful with trailing commas — JSON does not allow them. Use a JSON-aware editor mode mentally; verify with `python3 -c "import json; json.load(open('src/locales/en.json'))"` or equivalent.
4. Commit atomically: `feat(05): i18n keys for player.adminLibrary in en/ru/ja`.
  </action>
  <verify>
    <automated>cd /data/animeenigma/frontend/web && node -e "['en','ru','ja'].forEach(l => JSON.parse(require('fs').readFileSync('src/locales/'+l+'.json','utf8')))" && bunx tsc --noEmit</automated>
  </verify>
  <done>All three locale files parse as valid JSON; tsc clean; all locked keys present per the SPEC; one atomic commit.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 4: RawLibrary.vue view component — stats strip + search panel + jobs panel + pending-link sub-panel</name>
  <files>
    frontend/web/src/views/admin/RawLibrary.vue,
    frontend/web/src/components/ui/Badge.vue
  </files>
  <behavior>
    - `<script setup lang="ts">` SFC under `frontend/web/src/views/admin/RawLibrary.vue`.
    - Imports: `ref`, `computed`, `onMounted`, `onUnmounted` from vue; `useI18n` from vue-i18n; `adminLibraryApi`, `animeApi` from `@/api/client`; types from `@/types/library`; `Badge` from `@/components/ui/Badge.vue`; `Button`, `Input`, `Card` if present.
    - Three sections vertically stacked under a header (`{{ $t('player.adminLibrary.title') }}`):
      1. **Stats strip** — 3 glass-card tiles in a `flex` / `grid grid-cols-1 sm:grid-cols-3 gap-4`:
         - Disk free: `{((free/total)*100).toFixed(1)}% ({formatGB(free)} / {formatGB(total)} GB)`. Label = `$t('player.adminLibrary.stats.diskFree')`.
         - Active torrents: `health.active_torrents`. Label = `stats.activeTorrents`.
         - Active jobs: sum of `Object.values(health.active_jobs_by_status)`. Label = `stats.activeJobs`.
         - Polling: `setInterval` 30000ms calling `adminLibraryApi.healthExtended()`; cleared in `onUnmounted`.
      2. **Search panel** — `<form>` with title `<input>` (placeholder = `search.placeholder`), MAL ID `<input type="number">`, Submit button (text = `search.submit`).
         - Debounced 300ms via `lodash-es/debounce` (already a peer dep — verify with `grep '"lodash-es"' frontend/web/package.json`) OR a 6-line inline debounce util. Enter key also submits.
         - Submit fires `adminLibraryApi.search(query, malId, 50)` and renders results in a table:
           • Columns: provider chip (Badge variant cyan for AnimeTosho, purple/`info` for Nyaa), uploader, title, quality, size (human-readable via `formatBytes` util), magnet preview (first 60 chars + `…`), Queue button.
           • Queue button: `adminLibraryApi.createJob({magnet, title, source: release.source, uploader, quality, size_bytes, shikimori_id: undefined})`. On success: optimistically prepend the new job to the active jobs list with `status='queued'`. On failure (e.g. 507 disk_full): show inline error red text.
         - Empty result: render `search.empty` copy.
      3. **Jobs panel**:
         - Polls `adminLibraryApi.listJobs('queued,downloading,encoding,uploading', 50)` every 5000ms.
         - Renders job cards: title, status badge (color-coded; e.g. queued=default, downloading=primary cyan, encoding=warning amber, uploading=secondary pink, done=success emerald, failed=destructive red, cancelled=default), progress bar (Tailwind `<div class="h-2 bg-white/10 rounded"><div :style="`width:${progress_pct}%`" class="h-full bg-cyan-500 rounded"></div></div>`), bytes downloaded / total when available (derived from size_bytes + progress_pct: `formatBytes(size_bytes * progress_pct / 100)` / `formatBytes(size_bytes)`), Cancel button.
         - Cancel: if status in `['downloading','encoding','uploading']`, show `window.confirm` first. If user accepts (or status is `queued`), call `adminLibraryApi.cancelJob(id)`. Remove from active list on success.
         - **Failed sub-section** (`<details>` collapsible OR always-visible):
           • Fetched via a separate `adminLibraryApi.listJobs('failed', 20)` call. Polls every 30s (less frequent — failed rows don't change).
           • Each row shows title, `error_text` (truncated with full text on hover via `title` attribute), Retry button.
           • Retry button: `adminLibraryApi.retryJob(id)` → on success show toast OR optimistically add new queued job to active list.
         - **Pending-link sub-section**:
           • Fetched via `adminLibraryApi.listJobs('done', 20)` and filtered client-side to `job.shikimori_id == null || job.shikimori_id === ''`.
           • Highlighted with a yellow/amber border.
           • Each row has title + inline anime search dropdown: a debounced text input (300ms) that calls `animeApi.search(query)`. Render top 5 results as a dropdown list (poster thumbnail + name) below the input.
           • Selecting an anime calls `adminLibraryApi.linkJob(jobID, anime.shikimori_id)`. On success remove the job from the pending-link list.
    - All copy goes through `$t(...)` keys defined in Task 3. No hard-coded strings.
    - Polling cleanup: `onUnmounted` clears all three intervals (stats, jobs, failed/pending fetches if separate).
    - Loading state: skeleton or spinner while initial data loads; mirror `AdminRecs.vue` pattern.
    - Error state: `glass-card p-4 mb-6 border border-red-500/40` red banner with the error code (`403`, `5xx`, generic) — same shape as `AdminRecs.vue`.
    - **Badge.vue extension**: add an `'info'` variant with `bg-purple-500/20 text-purple-400` for the Nyaa chip. Update the `Props.variant` union and the `variants` map. This is the ONLY change to Badge.vue — keep diff tight.
  </behavior>
  <action>
1. Open `frontend/web/src/components/ui/Badge.vue`. In the `Props` interface add `'info'` to the variant union. In the `variants` object add `info: 'bg-purple-500/20 text-purple-400'`. That is the full diff.
2. Create `frontend/web/src/views/admin/RawLibrary.vue`. Use the AdminRecs.vue header/loading/error shape as the structural reference. Implementation outline:
   - `<template>` opens with `<div class="min-h-screen bg-base pt-20"><div class="container mx-auto px-4 py-8 max-w-7xl">`. Inside: `<h1>` with `$t('player.adminLibrary.title')`, then the three sections.
   - `<script setup lang="ts">`:
     • Refs: `health: Ref<LibraryHealth | null>`, `searchQuery: Ref<string>`, `searchMalId: Ref<number | null>`, `searchResults: Ref<Release[]>`, `searching: Ref<boolean>`, `activeJobs: Ref<Job[]>`, `failedJobs: Ref<Job[]>`, `pendingLinkJobs: Ref<Job[]>`, `pendingLinkSearchQueries: Ref<Record<string, string>>` (per-job dropdown query), `pendingLinkResults: Ref<Record<string, AnimeSearchResult[]>>`, `error: Ref<string | null>`.
     • Functions: `fetchHealth()`, `fetchActiveJobs()`, `fetchFailedJobs()`, `fetchPendingLinkJobs()`, `handleSearch()` (debounced 300ms), `queueJob(release)`, `cancelJob(job)`, `retryJob(job)`, `searchAnimeForLink(jobId, query)` (debounced 300ms), `linkJob(jobId, shikimoriID)`. Helpers: `formatBytes(n)`, `formatPct(num, denom)`, `truncateMagnet(s)`.
     • `onMounted`: fire each fetch once; start three intervals (30s health, 5s active jobs, 30s failed + pending-link).
     • `onUnmounted`: clear intervals.
   - Use `<Badge :variant="release.source === 'animetosho' ? 'primary' : 'info'">{{ $t('player.adminLibrary.search.providers.' + release.source) }}</Badge>`.
   - For the anime-search dropdown, render as a relatively-positioned `<div class="relative">` containing the `<input>` and a conditional `<ul class="absolute z-10 ...">` of top-5 results.
3. Verify `bun run build` from `frontend/web` succeeds. Then `bunx tsc --noEmit`.
4. Run `make redeploy-library` (no-op if backend unchanged this task) and `cd frontend/web && bun run build` to confirm the production build.
5. Commit atomically: `feat(05): RawLibrary.vue admin view — stats + search + jobs + pending-link panels`.

**Notes on i18n nested-keys access**: vue-i18n supports `$t('player.adminLibrary.jobs.status.' + job.status)` via dot-path; this is the canonical idiom in the codebase (`AdminRecs.vue` uses `$t('admin.recs.' + key)`).

**Notes on poll cleanup**: keep interval handles in module-scope `let` variables (or refs); clear them all in `onUnmounted` to prevent memory leaks across route navigations.

**Notes on optimistic UI**: after `queueJob` returns, prepend the new job to `activeJobs.value` without waiting for the next poll tick — the operator sees their action in &lt;100ms instead of waiting up to 5s.

**Notes on accessibility**: every button needs `:aria-label` for screen readers (re-use the action label as the aria-label).
  </action>
  <verify>
    <automated>cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bun run build 2>&1 | tail -20</automated>
  </verify>
  <done>RawLibrary.vue exists with all three sections + pending-link sub-panel; `bun run build` succeeds; `bunx tsc --noEmit` clean; Badge.vue has the `info` variant; one atomic commit.</done>
</task>

<task type="auto">
  <name>Task 5: Router entry for /admin/raw-library</name>
  <files>
    frontend/web/src/router/index.ts
  </files>
  <behavior>
    - New route entry:
      • `path: '/admin/raw-library'`
      • `name: 'admin-raw-library'`
      • `component: () => import('@/views/admin/RawLibrary.vue')`
      • `meta: { titleKey: 'player.adminLibrary.title', requiresAuth: true, requiresAdmin: true }`
    - Placed alongside the other admin routes (after `admin-collection-edit`, before the `:pathMatch(.*)*` catch-all).
    - No new beforeEnter — the global `beforeEach` handles auth + admin redirect via meta flags.
  </behavior>
  <action>
1. Open `frontend/web/src/router/index.ts`. Find the existing admin routes block (around line 110 — `admin-collections`, `admin-collection-edit`). After the last admin route and before the `path: '/collections/:slug'` public route OR before the `:pathMatch(.*)*` catch-all, insert the new route entry.
2. Format: same shape as the other admin routes.
3. Run `bunx tsc --noEmit` and `bun run build` to verify.
4. Commit atomically: `feat(05): /admin/raw-library route entry`.
  </action>
  <verify>
    <automated>cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bun run build 2>&1 | tail -10</automated>
  </verify>
  <done>Route registered; visiting /admin/raw-library as a logged-in admin user renders RawLibrary.vue; one atomic commit.</done>
</task>

<task type="auto">
  <name>Task 6: Playwright e2e — frontend/web/e2e/raw-library-admin.spec.ts</name>
  <files>
    frontend/web/e2e/raw-library-admin.spec.ts
  </files>
  <behavior>
    - Test name: `RawLibrary admin view — workstream raw-jp Phase 05`.
    - Setup: `ui_audit_bot` is `role=user` per Phase-2 SUMMARY's open items — the spec must first promote the user to admin via a direct DB UPDATE OR skip itself if no admin fixture exists. Pattern: at `test.beforeAll`, execute `docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "UPDATE users SET role='admin' WHERE username='ui_audit_bot'"` via `child_process.execSync`. At `test.afterAll`, revert to `role='user'`. If `execSync` is not available in the Playwright runtime (it should be — Playwright runs under Node), wrap in try/catch and `test.skip` the suite.
    - Test flow:
      1. Promote `ui_audit_bot` to admin (beforeAll).
      2. Use the existing `loginAsUiAuditBot` helper (copy-paste from `e2e/raw-player.spec.ts:25-48`) to mint a session.
      3. Navigate to `/admin/raw-library`.
      4. Assert page title: locator with `text=/Raw Library/i` is visible.
      5. Assert stats strip: three glass-card tiles visible (locators by aria-label or text matching the three stat labels in EN).
      6. Search panel: fill the title input with `"test"`, press Enter. Wait for either the result table to appear OR the empty-state message. (Live Nyaa response may be slow — set 30s timeout. Don't fail if Nyaa is down; tolerate empty results.)
      7. Jobs panel: locator with `text=/Active jobs/i` is visible (whether or not jobs exist).
      8. Non-admin guard test (separate `test` block): navigate to `/admin/raw-library` as the original `role=user` ui_audit_bot (revert role first, log in fresh). Assert URL bounces to `/` and the `admin_redirect_reason` sessionStorage marker was set.
    - Live torrent download IS NOT asserted (too slow for e2e; integration in operator runbook).
    - Run via `cd frontend/web && bunx playwright test raw-library-admin --reporter=list`.
  </behavior>
  <action>
1. Copy the `loginAsUiAuditBot` helper from `e2e/raw-player.spec.ts` (lines 25-48) into the new spec file as a top-level async function.
2. Add a `promoteToAdmin()` / `revertToUser()` pair using `child_process.execSync`:
   ```ts
   function setUserRole(role: 'admin' | 'user') {
     execSync(`docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "UPDATE users SET role='${role}' WHERE username='ui_audit_bot'"`, { stdio: 'pipe' })
   }
   ```
3. Wire `test.beforeAll(() => setUserRole('admin'))` and `test.afterAll(() => setUserRole('user'))`. Wrap in try/catch — on failure call `test.skip()` with reason.
4. Write the two test bodies per the behavior block. Use Playwright locators (`page.getByRole`, `page.getByText`, `page.locator`).
5. Run `cd frontend/web && bunx playwright test raw-library-admin --reporter=list` once locally to confirm it passes.
6. Commit atomically: `test(05): Playwright e2e for /admin/raw-library`.

**Caveat**: if the Playwright test environment runs OUTSIDE the docker host (e.g. CI), `docker compose exec` will fail. Document the local-only nature in a top-of-file comment: `// REQUIRES: local docker compose stack running. CI runners without docker will test.skip().`

**Caveat 2**: per CONTEXT, the SPEC's Acceptance #6 says "Non-admin visiting /admin/raw-library is redirected" — that's exactly what the second test asserts.
  </action>
  <verify>
    <automated>cd /data/animeenigma/frontend/web && bunx playwright test raw-library-admin --reporter=list 2>&1 | tail -30</automated>
  </verify>
  <done>Spec file exists; runs locally with docker-stack-up; commit lands on `test(05): Playwright e2e for /admin/raw-library`.</done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Task 7: Live smoke + SUMMARY.md</name>
  <what-built>
    A complete Phase-05 deliverable: the three backend endpoints, the
    Vue view, the e2e test, types module, i18n keys. The remaining
    work is operator verification end-to-end plus the phase
    SUMMARY.md.
  </what-built>
  <how-to-verify>
    1. **Redeploy library service**: `make redeploy-library`. Confirm `make health` shows `library:8089` ✓.
    2. **Frontend dev or build**: `cd frontend/web && bun run build`. Or `bun run dev` if you want hot-reload. Verify zero errors.
    3. **Promote ui_audit_bot to admin** (temp): `docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "UPDATE users SET role='admin' WHERE username='ui_audit_bot'"`.
    4. **Open the view**: in a logged-in admin session, navigate to `https://animeenigma.ru/admin/raw-library` (production) or `http://localhost:5173/admin/raw-library` (dev). Verify:
       - Page title shows "Raw Library" (or RU/JA equivalent depending on locale).
       - Stats strip renders three tiles with non-zero values (or the SPEC-shaped defaults).
       - Search box accepts input.
    5. **Search**: type "bocchi" + press Enter. Verify the result table renders with provider chips (cyan for AnimeTosho, purple for Nyaa). If both providers are down, accept the empty-state copy.
    6. **Queue**: click Queue on a result row. Verify the new job appears in the Active Jobs panel within 5s with `status=queued` and then transitions to `downloading`.
    7. **Cancel**: click Cancel on the in-flight job. Confirm dialog appears (because status is downloading). Accept. Verify the job is removed from the active list.
    8. **Retry path**: if any failed jobs exist in the DB (or create one via `UPDATE library_jobs SET status='failed', error_text='manual test' WHERE id=...`), verify the Failed sub-section renders the row with the error text + Retry button. Click Retry. Verify a new queued job appears.
    9. **Pending-link path**: this requires a done job with `shikimori_id=NULL` plus MinIO objects under `pending/{job_id}/`. If none exist, manually seed:
       ```bash
       # 1. Insert a done job with no shikimori_id
       docker compose exec -T postgres psql -U postgres -d library -c "INSERT INTO library_jobs (source, magnet, title, status, shikimori_id) VALUES ('manual', 'magnet:?xt=urn:btih:dummy', 'pending-link-smoke', 'done', '') RETURNING id;"
       # 2. Note the returned job_id
       # 3. Upload a stub playlist + segment under pending/{job_id}/1/
       docker exec animeenigma-minio mc cp /etc/hosts local/raw-library/pending/{job_id}/1/playlist.m3u8
       docker exec animeenigma-minio mc cp /etc/hosts local/raw-library/pending/{job_id}/1/segment_000.ts
       ```
       Reload the view, scroll to Pending-link section, verify the job appears. Type "test" in the dropdown, pick any anime. Verify:
       - MinIO objects moved from `pending/{job_id}/1/` to `{shikimori_id}/1/`.
       - `library_episodes` row inserted.
       - Job's `shikimori_id` column populated.
       - Job removed from Pending-link list.
    10. **Non-admin redirect**: revert `ui_audit_bot` to `role='user'`. Re-login. Navigate to `/admin/raw-library`. Confirm redirect to home + red banner via the existing `admin.errors.notAdmin` toast/banner.
    11. **Cleanup**: revert `ui_audit_bot` to `role='user'` if not already. Delete any smoke-test rows from library_jobs / library_episodes / MinIO. (Mirror Phase-3 + Phase-4 SUMMARY cleanup notes.)
    12. **Write SUMMARY.md** at `.planning/workstreams/raw-jp/phases/05-rawlibrary-vue-admin-ui/05-SUMMARY.md` following the @$HOME/.claude/get-shit-done/templates/summary.md structure, mirroring Phase-3 + Phase-4 SUMMARYs in scope and detail:
       - Frontmatter: phase, status, workstream, milestone, date, requirements (LIB-09), commits (the 6 atomic commits from tasks 1-6).
       - Sections: What was built (per-task) / Files touched / Verification results (build + tests + smoke) / Deviations from plan / Out of scope / Open items / Self-Check.
       - Open items: `ui_audit_bot` admin promotion is manual; the e2e test does it via psql; in a hardened test env we'd seed a separate admin fixture user.
    13. **`/animeenigma-after-update` skill**: at the very end (NOT before each task — only once after the full phase ships) invoke the after-update skill to lint/build/redeploy/changelog/commit any catch-up changes.

    Verification artefacts to capture (per Phase-3 + Phase-4 style):
    - `make health` output.
    - `curl http://localhost:8000/api/library/health/extended` JSON output.
    - Screenshot of the view (optional but encouraged).
    - SQL: `SELECT id, status, shikimori_id, error_text FROM library_jobs ORDER BY created_at DESC LIMIT 5;`
    - MinIO: `docker exec animeenigma-minio mc ls --recursive local/raw-library/ | head -20`.

    Acceptance:
    - [ ] All seven SPEC Acceptance Criteria pass (view renders, stats refresh, search debounces + submits, queue creates job in &lt;5s, cancel works, retry works, pending-link moves files + inserts episode + redirects non-admin, `bun run build` clean, e2e green).
    - [ ] SUMMARY.md committed.
    - [ ] Cleanup done (no stray smoke rows / objects / admin promotions).
  </how-to-verify>
  <resume-signal>Type "approved" once the SPEC's 7 acceptance criteria are verified, the SUMMARY.md is committed, and cleanup is complete.</resume-signal>
</task>

</tasks>

<threat_model>

## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Admin user browser → /admin/raw-library | Untrusted user input (search query, MAL ID, shikimori_id, anime selection) crosses into Vue → axios → gateway |
| Gateway → library service | Admin-gated by JWT + AdminRoleMiddleware (existing). Library trusts what the gateway forwards. |
| Library service → MinIO | CopyObject/RemoveObject on the `raw-library` bucket; bucket ACL is server-side-only. |
| Library service → Postgres | All queries are parameterized via GORM; no string concat into SQL. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-05-01 | Tampering | Admin browser → `PATCH /jobs/{id}` body `shikimori_id` | mitigate | Server validates `shikimori_id` is non-empty + string; existence in catalog is NOT checked (admin discretion — they pick from dropdown of catalog anime). |
| T-05-02 | Spoofing | Non-admin attempts to call `/api/library/jobs/*` directly | mitigate | Gateway's `AdminRoleMiddleware` already gates all `/api/library/*` except `/health`. The new PATCH + retry routes inherit this gate (no per-route opt-out). |
| T-05-03 | Tampering | Admin browser submits crafted `magnet` via Queue button (XSS via title?) | mitigate | Vue auto-escapes via `{{ }}` interpolation; never use `v-html` on user-supplied title/magnet. Backend already validates magnet shape via `metainfo.ParseMagnetUri`. |
| T-05-04 | Repudiation | Admin denies retrying a destructive job | mitigate | New retry row's `error_text` field contains `"retry of <old_id>"` — full audit trail in Postgres. Old failed row never deleted. |
| T-05-05 | Information Disclosure | `error_text` from failed encodes contains ffmpeg stderr — may leak filesystem paths | accept | Stderr is server-side-tail-only, admin-gated to read, low impact (paths are within `/tmp/encode` not anything sensitive). |
| T-05-06 | Denial of Service | Admin spam-clicks Queue on many search results → fills disk | mitigate | `disk_full` 507 guard from Phase 3 still applies on each POST; existing `library_enqueue_rejected_total{reason="disk_full"}` increments. |
| T-05-07 | Elevation of Privilege | Vue admin guard bypass via direct route navigation | mitigate | Defense-in-depth: gateway is the security boundary, Vue guard is UX only. Gateway always rejects non-admin even if Vue allows render. |
| T-05-08 | Tampering | MinIO Move: incomplete copy → orphaned source objects → data loss | mitigate | Move sequence: copy ALL first, only delete sources AFTER every copy returns success. On copy error → abort + sources stay → no data loss. Documented in writer.go. |
| T-05-09 | Information Disclosure | Admin browser polls every 5s; `error_text` may flash sensitive content briefly | accept | Admin is already trusted with full job table contents. No different from the SQL view. |
| T-05-10 | Tampering | Admin manually queues a magnet from an attacker-controlled tracker URL | accept | The library service is the operator's tool; they trust their own queue. No content moderation in v0.2. |

</threat_model>

<verification>

## Phase-level checks (run after all tasks land)

```bash
# 1. Backend build + tests
cd /data/animeenigma/services/library && go build ./... && go vet ./... && go test ./... -count=1 -short

# 2. Integration tests
cd /data/animeenigma/services/library && INTEGRATION=1 DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres DB_NAME=postgres go test -tags=integration ./internal/repo -count=1

# 3. Frontend type-check + build
cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bun run build

# 4. Linter (frontend)
cd /data/animeenigma/frontend/web && bunx eslint src/views/admin/RawLibrary.vue src/types/library.ts

# 5. Playwright e2e
cd /data/animeenigma/frontend/web && bunx playwright test raw-library-admin --reporter=list

# 6. Locale parseability
cd /data/animeenigma/frontend/web && node -e "['en','ru','ja'].forEach(l => JSON.parse(require('fs').readFileSync('src/locales/'+l+'.json','utf8')))"

# 7. Routes registered in library service
curl -sI http://localhost:8000/api/library/health/extended | head -1   # 401 without auth (admin gate)
curl -sI -X PATCH http://localhost:8000/api/library/jobs/aaa | head -1 # 401 without auth
curl -sI -X POST http://localhost:8000/api/library/jobs/aaa/retry | head -1 # 401 without auth

# 8. Vue route registered
grep -c "admin-raw-library" /data/animeenigma/frontend/web/src/router/index.ts  # >= 1
```

</verification>

<success_criteria>

This phase is COMPLETE when:

- [ ] All 7 SPEC Acceptance Criteria pass (rendered view, stats refresh, search debounces, queue → job in &lt;5s, cancel works, retry works, pending-link links + redirects).
- [ ] `cd services/library && go build ./... && go vet ./... && go test ./... -count=1` returns 0.
- [ ] `cd frontend/web && bunx tsc --noEmit && bun run build` returns 0.
- [ ] `cd frontend/web && bunx playwright test raw-library-admin` returns 0 (or test.skip with documented reason in CI).
- [ ] All `must_haves.truths` verifiable manually.
- [ ] All `must_haves.artifacts` exist on disk.
- [ ] All `must_haves.key_links` greppable.
- [ ] STRIDE register entries each have a disposition (mitigate / accept / transfer).
- [ ] 6 atomic commits land (one per task 1-6) plus a final `docs(05):` for the SUMMARY.
- [ ] SUMMARY.md committed at `.planning/workstreams/raw-jp/phases/05-rawlibrary-vue-admin-ui/05-SUMMARY.md`.
- [ ] Smoke cleanup complete (no stray smoke rows / objects / admin promotions).
- [ ] `/animeenigma-after-update` skill invoked once at the end to deploy + changelog + push.

</success_criteria>

<output>
After completion, the executor MUST create `.planning/workstreams/raw-jp/phases/05-rawlibrary-vue-admin-ui/05-SUMMARY.md` mirroring the Phase-3 + Phase-4 SUMMARYs in shape (frontmatter with commits, sections for what-was-built / files-touched / verification / deviations / out-of-scope / open-items / self-check).
</output>
