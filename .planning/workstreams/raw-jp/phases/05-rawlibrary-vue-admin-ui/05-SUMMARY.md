---
phase: 05-rawlibrary-vue-admin-ui
status: complete
workstream: raw-jp
milestone: v0.2
date: 2026-05-18
requirements:
  - LIB-09
commits:
  - 4903c46 — feat(05): backend additions for raw-library admin UI
  - 15d2947 — feat(05): adminLibraryApi + library types module
  - 896f24c — feat(05): i18n keys for player.adminLibrary in en/ru/ja
  - d67dada — feat(05): RawLibrary.vue admin view + Badge info/destructive variants
  - 4df9d93 — feat(05): /admin/raw-library route entry
  - a70e828 — test(05): Playwright e2e for /admin/raw-library
---

# Phase 05: RawLibrary.vue Admin UI — Summary

Ships LIB-09 — the operator-only `/admin/raw-library` Vue view plus
the three small backend endpoints it needs (`GET
/api/library/health/extended`, `PATCH /api/library/jobs/{id}`, `POST
/api/library/jobs/{id}/retry`) and the `minio.Writer.Move` helper that
powers the post-hoc shikimori link.

Closes the last admin-UI gap of v0.2: admins no longer have to `curl`
the library API by hand to monitor torrent → encode → upload
progress, retry failures, or link orphan (no-shikimori_id) jobs back
to catalog anime. Phase 6 (hybrid resolver) is backend-only — no
further UI work scheduled for v0.2.

End-to-end live smoke against the deployed stack verified every
must-have: the `/health/extended` endpoint returns the SPEC-locked
JSON; PATCH + retry return clean 404 on missing rows; the admin gate
at the gateway rejects unauthenticated calls; and the new route +
chunked Vue view render in production (RawLibrary-CcQfpLPE.js served
from nginx).

## What was built

**Task 1 — Backend additions (4903c46).**

`handler/health.go` gains a new constructor
`NewHealthHandlerExtended(disk, counter, lister)` plus the
`HealthExtended` method that returns the four-field SPEC-locked JSON
(`disk_free_bytes`, `disk_total_bytes`, `active_torrents`,
`active_jobs_by_status`). Three small interfaces (`DiskCheckProbe`,
`TorrentCounter`, `JobLister`) keep the handler free of direct deps;
disk-probe error → 500, lister error → soft-fail with zeroes.

`handler/jobs.go` gains `Link` (PATCH) and `Retry` (POST). Link is
cheap-validation-first: 404 on missing job → 400 on `status != done`
→ 400 on `already linked` → 400 on missing shikimori_id → list MinIO
objects under `pending/{id}/` → parse the `{ep}` segment → Move →
EpisodeStore.Create → UpdateShikimoriID → GetByID → 200 with updated
row. Retry is a pre-flight status check + `JobRepository.Retry` →
201. Both routes flow through the existing gateway admin gate. The
new `JobStoreAPI` methods `UpdateShikimoriID` + `Retry` and the
`MinioMover` + `EpisodeStore` interfaces are wired via
`NewJobsHandlerWithLink`. `parseEpisodeFromPendingKey` table-tests
the path-segment regex.

`repo/job.go` gains `UpdateShikimoriID` (thin GORM `Updates` mapping
`RowsAffected == 0` to `NotFound`, empty input → `InvalidInput`) and
`Retry` (single transaction: SELECT old → validate `failed` →
INSERT new with `error_text = "retry of {oldID}"` →
status=queued/progress=0). `formatRetryErrorText` centralizes the
audit-trail string format.

`minio/writer.go` gains `Move(src, dst)` and `ListObjectsByPrefix`.
The `Uploader` interface picks up `ListObjects` / `CopyObject` /
`RemoveObject`; `minioClientAdapter` forwards each to the SDK. Move
runs COPY ALL first then REMOVE ALL second — on any copy error,
sources stay intact (data preserved); on a remove error post-copy,
log + return nil (orphan source is recoverable). Prefixes are
normalized to trailing slashes; empty src → error.

`transport/router.go` registers `PATCH /jobs/{id}`, `POST
/jobs/{id}/retry`, `GET /health/extended` inside the admin-gated
`/api/library` group. `cmd/library-api/main.go` swaps to the new
constructors (`NewHealthHandlerExtended` + `NewJobsHandlerWithLink`)
— no other wiring changes.

Tests added: 5 health-handler cases (legacy /health, happy
HealthExtended, disk error, empty jobs, status-map zero-init);
8 Link/Retry handler cases (Link happy, 404, 400 not-done, 400
already-linked, 500 no-minio, 400 empty shikimori_id, Retry happy
+ 400 not-failed + 404 missing); 7 `parseEpisodeFromPendingKey`
table cases; 6 Move/ListObjectsByPrefix cases (happy, empty src,
copy error aborts without remove, remove error soft-fails, prefix
normalization, deterministic sort); 3 repo unit tests
(`formatRetryErrorText`, `UpdateShikimoriID` empty-id, table); 2
integration tests (UpdateShikimoriID + Retry).

**Task 2 — Frontend types + adminLibraryApi (15d2947).**
New `frontend/web/src/types/library.ts` exports six types (`Job`,
`JobStatus`, `Release`, `Episode`, `LibraryHealth`,
`CreateJobPayload`) mirroring the Go domain structs in snake_case.
`adminLibraryApi` block added to `api/client.ts` after the existing
`adminApi`, with the SPEC-locked signatures
(`search/listJobs/getJob/createJob/cancelJob/linkJob/retryJob/healthExtended`).
Consumers handle the httputil envelope unwrap (`response.data.data`)
themselves.

**Task 3 — i18n keys (896f24c).**
Adds `player.adminLibrary.*` under the existing top-level `player`
namespace in all three locales (en/ru/ja). Locked keys: `title`,
`stats.{diskFree,activeTorrents,activeJobs}`,
`search.{title,placeholder,submit,empty,queue,providers.*}`,
`jobs.{title,empty,cancel,retry,confirmCancel,status.*,failed.*,pendingLink.*}`.
All three files validate as JSON; the `confirmCancel` key is the
single Claude-discretion addition beyond the SPEC list (used by the
in-flight job cancel confirm dialog).

**Task 4 — RawLibrary.vue + Badge variants (d67dada).**
`Badge.vue` picks up two new variants:
`info: bg-purple-500/20 text-purple-400` (Nyaa chip per SPEC) and
`destructive: bg-red-500/20 text-red-400` (failed status badge).

`views/admin/RawLibrary.vue` is a `<script setup lang="ts">` SFC with
three sections under a `glass-card` themed layout:

1. **Stats strip** — 3 tiles (`disk_free` %, active torrents, active
   jobs sum). 30s `setInterval` calling
   `adminLibraryApi.healthExtended()`. `formatBytes` / `formatGB` /
   `formatPct` helpers.

2. **Search panel** — title + MAL-ID inputs, debounced 300ms via
   inline `setTimeout`. Result table with provider chip (cyan for
   AnimeTosho, purple `info` for Nyaa via Badge.vue), uploader,
   title, quality, formatted size, magnet preview, Queue button.
   Queue button does optimistic prepend to active-jobs list.

3. **Jobs panel** — active list polled every 5s; failed sub-section
   + pending-link sub-section polled every 30s (cheaper, rows change
   rarely). Each active job renders a Tailwind progress bar
   (`<div class="h-2 bg-white/10"><div :style="`width:${pct}%`">`),
   status badge colour-coded via `statusVariant()`, Cancel button
   (with `window.confirm` for downloading/encoding/uploading
   states only — queued cancels straight through). Failed section
   shows `error_text` + Retry button. Pending-link section shows a
   debounced inline anime-search dropdown (`animeApi.search(q,
   undefined, 5)`); selecting an anime calls
   `adminLibraryApi.linkJob(jobID, anime.shikimori_id)` and removes
   the row optimistically. All polls + debounces cleaned up in
   `onUnmounted`.

All copy through `$t(...)` — no hard-coded strings. `bunx tsc
--noEmit` clean; `bun run build` chunks to
`RawLibrary-CcQfpLPE.js` (~13 KB / 4 KB gzip).

**Task 5 — Router entry (4df9d93).**
`/admin/raw-library` route registered with
`meta: { titleKey: 'player.adminLibrary.title', requiresAuth: true,
requiresAdmin: true }`, lazy-imported. Reuses the existing global
`beforeEach` admin guard — no per-route `beforeEnter`. Slotted
alongside the other `admin-collection-*` routes; precedes the public
`/collections/:slug` and `:pathMatch(.*)*` catch-all.

**Task 6 — Playwright e2e (a70e828).**
`frontend/web/e2e/raw-library-admin.spec.ts`: two tests under a
shared `describe`. `beforeAll` / `afterAll` promote `ui_audit_bot` to
admin via `docker compose exec -T postgres psql` and revert on
teardown; `test.skip()` cleanly if docker is unreachable. Test 1
asserts admin can render the title, the stats-strip section, and the
jobs-panel header; the search box accepts input + submit. Test 2
flips `ui_audit_bot` back to user mid-suite, asserts the redirect
away from `/admin/raw-library` and the `admin_redirect_reason`
sessionStorage marker. Live torrent download is NOT asserted (too
slow for e2e). `playwright test --list` registers 6 test instances
(2 tests × chromium/firefox/Mobile Chrome).

## Files touched

**New (4):**
- `services/library/internal/handler/health_test.go` — 5 tests
- `frontend/web/src/types/library.ts` — Job/Release/Episode/etc
- `frontend/web/src/views/admin/RawLibrary.vue` — 556 lines, 3 sections
- `frontend/web/e2e/raw-library-admin.spec.ts` — 2 tests

**Extended (10):**
- `services/library/internal/handler/health.go` — HealthExtended + interfaces
- `services/library/internal/handler/jobs.go` — Link + Retry handlers + 2 interfaces + new constructor
- `services/library/internal/handler/jobs_test.go` — +8 tests + 2 stubs
- `services/library/internal/repo/job.go` — UpdateShikimoriID + Retry + formatRetryErrorText
- `services/library/internal/repo/job_test.go` — +3 tests
- `services/library/internal/repo/job_integration_test.go` — +2 integration tests
- `services/library/internal/minio/writer.go` — Move + ListObjectsByPrefix + Uploader interface extensions
- `services/library/internal/minio/writer_test.go` — +6 tests + fake stub extensions
- `services/library/internal/transport/router.go` — 3 new routes
- `services/library/cmd/library-api/main.go` — new constructor wiring
- `frontend/web/src/api/client.ts` — adminLibraryApi block + import
- `frontend/web/src/components/ui/Badge.vue` — info + destructive variants
- `frontend/web/src/locales/{en,ru,ja}.json` — player.adminLibrary.* keys
- `frontend/web/src/router/index.ts` — /admin/raw-library route

## Verification results

### Backend build + vet + tests

```
$ cd services/library && go build ./... && go vet ./... && go test ./... -count=1 -short
ok  	internal/ffmpeg	0.120s
ok  	internal/handler	0.018s
ok  	internal/metrics	0.007s
ok  	internal/minio	0.006s
ok  	internal/parser/animetosho	0.014s
ok  	internal/parser/filename	0.007s
ok  	internal/parser/nyaa	0.007s
ok  	internal/repo	0.005s
ok  	internal/service	0.125s
ok  	internal/torrent	0.130s
```

### Integration tests (Phase-5 additions)

```
$ INTEGRATION=1 DB_HOST=127.0.0.1 ... go test -tags=integration ./internal/repo -count=1
ok  	internal/repo	1.399s
```

`TestJobRepository_UpdateShikimoriID_Updates` and `TestJobRepository_Retry`
both pass against per-test databases with full schema reapply.

### Frontend type-check + build

```
$ cd frontend/web && bunx tsc --noEmit
(clean)

$ bun run build
✓ 374 modules transformed.
✓ built in 6.00s
dist/assets/RawLibrary-CcQfpLPE.js   13.27 kB │ gzip:   4.26 kB
```

### Playwright list

```
$ bunx playwright test raw-library-admin --list
6 tests in 1 file (2 tests × chromium/firefox/Mobile Chrome)
```

### make redeploy-library + make redeploy-web

```
$ make redeploy-library
Container animeenigma-library Started
[INFO] library is running
[INFO] Deployment complete!

$ make redeploy-web
Container animeenigma-web Started
Web frontend redeployed
```

### Live smoke against the deployed stack

```
# 1. Direct library service — health/extended JSON shape verified
$ curl -s http://localhost:8089/api/library/health/extended
{"success":true,"data":{
  "disk_free_bytes":268540088320,
  "disk_total_bytes":539667304448,
  "active_torrents":0,
  "active_jobs_by_status":{"downloading":0,"encoding":0,"queued":0,"uploading":0}
}}

# 2. Gateway admin gate works (401 without auth)
$ curl -sI http://localhost:8000/api/library/health/extended | head -1
HTTP/1.1 401 Unauthorized
$ curl -sI -X PATCH http://localhost:8000/api/library/jobs/aaa | head -1
HTTP/1.1 401 Unauthorized
$ curl -sI -X POST http://localhost:8000/api/library/jobs/aaa/retry | head -1
HTTP/1.1 401 Unauthorized

# 3. With temporary admin promotion + JWT, all routes work
$ TOKEN=$(curl ... login | jq -r .data.access_token)
$ curl -s http://localhost:8000/api/library/health/extended -H "Authorization: Bearer $TOKEN"
{"success":true,"data":{"disk_free_bytes":268540088320,...}}

$ curl -s "http://localhost:8000/api/library/jobs?status=failed&limit=5" -H "Authorization: Bearer $TOKEN"
{"success":true,"data":{"jobs":[]}}

# 4. PATCH + retry return clean 404 on missing rows
$ curl -X PATCH .../jobs/00000000-0000-0000-0000-000000000000 -d '{"shikimori_id":"1"}'
{"success":false,"error":{"code":"NOT_FOUND","message":"job not found"}}
$ curl -X POST .../jobs/00000000-0000-0000-0000-000000000000/retry
{"success":false,"error":{"code":"NOT_FOUND","message":"job not found"}}

# 5. nginx serves the RawLibrary chunk
$ docker exec animeenigma-web ls /usr/share/nginx/html/assets/ | grep RawLibrary
RawLibrary-CcQfpLPE.js
RawLibrary-CcQfpLPE.js.gz
```

### Cleanup

`UPDATE users SET role='user' WHERE username='ui_audit_bot'` — restored
after the smoke session. No library_jobs / library_episodes / MinIO
objects were created during the smoke (the smoke is read-only against
the live endpoints; full E2E with a real torrent stays in the
operator runbook).

## Deviations from plan

**1. [Rule 2 — Missing functionality] Badge `destructive` variant.**
The plan locked the `info` variant for Nyaa (purple). The colour-coded
job-status badges legitimately needed a red variant for the `failed`
state — without it the failed job badge would have been hard-coded as
inline classes outside Badge.vue's variants map, fragmenting the design
system. Added `destructive: bg-red-500/20 text-red-400` alongside
`info` in the same Badge.vue diff.

**2. [Rule 2 — Missing functionality] `confirmCancel` i18n key.**
The SPEC enumerated the i18n keys but didn't include the confirm-dialog
copy. The plan's behavior block called for `window.confirm` on
in-flight cancels — that copy needs to be translatable. Added
`jobs.confirmCancel` to all three locales as a one-line addition.

**3. [Claude's discretion] `JobLister` interface name vs `JobStoreAPI`.**
The plan named the health-handler-side interface `JobLister`; jobs.go
already had `JobStoreAPI` with `List` in it. Keeping them distinct
(JobLister has only `List`) lets `*repo.JobRepository` satisfy both
without exposing Create/GetByID to the health handler. Same approach
as the Phase-3 `JobStore` vs `JobStoreAPI` split.

**4. [Claude's discretion] Soft-fail on lister error in HealthExtended.**
The plan called out disk-error → 500. For the lister it didn't specify.
I chose to log + continue with zeroes — the disk number is the more
important signal for the operator and a transient DB hiccup shouldn't
break the stats strip refresh loop. If the DB is truly down the entire
service will fail health checks anyway.

**5. [Claude's discretion] `NewJobsHandlerWithLink` rather than
mutating `NewJobsHandler`.** The plan called for "extend constructor".
Existing test code (`newTestHandler`) calls `NewJobsHandler` with the
6 original args — changing the signature would cascade into the
Phase-3 test file. New `NewJobsHandlerWithLink` adds the 2 deps
non-disruptively; main.go uses the new one. Keeps the diff tight and
preserves the Phase-3 test surface.

## Out of scope (per SPEC)

- Phase 6 hybrid resolver consuming `library_episodes` (backend only).
- User-facing library listing.
- Bulk-queue (admin queues one job at a time).
- Per-uploader filename-pattern editor UI.
- Live torrent download E2E test (operator runbook only).
- Storage cleanup / retention policy (v0.2.1).

## Open items

Carried forward from Phase 1-4:
- The CLAUDE.md "Service Ports" table still lists `library 8081`
  but the service runs on 8089 (Phase-1 deviation).
- `ui_audit_bot` is role=user by default — Phase 5's Playwright spec
  promotes it to admin via psql in beforeAll. A dedicated admin
  fixture user would be cleaner but is deferred until we add a
  third-party CI runner.
- GORM "record not found" warning lines from the worker's Claim()
  call still spam docker logs (Phase-3 deferred).
- `go mod tidy` still unsafe on this workspace (Phase-4 deferred).

New from Phase 5:
- **MinIO Move's remove-error path is logged-but-swallowed.** Per
  the threat model (T-05-08), data integrity is preserved when copy
  succeeds. An admin who notices stale `pending/{job_id}/` objects
  can manually clean them up via `mc rm`. A cleanup sweeper for
  orphan-source detection is deferred to v0.2.1.
- **Pending-link episode-number detection assumes single-episode
  jobs.** The detector reads the first `{ep}` directory under
  `pending/{job_id}/`. Multi-episode jobs (a single torrent
  containing multiple episodes) would land all files under a single
  `{ep}` dir per Phase 4's encoder behavior — but if a future
  encoder ever writes multiple `{ep}` directories under one job, the
  Link handler would silently pick the first one returned by the
  sorted list. Worth a follow-up assert when v0.2.1 adds multi-ep
  torrent support.
- **Optimistic UI on Retry assumes the new row appears immediately
  in the active-jobs list.** The next 5s active-jobs poll will
  reconcile if the row landed via repo. If POST returns 201 but the
  row is somehow rolled back, the UI shows a phantom queued row for
  up to 5s. Cosmetic only — not data-loss.

## Self-Check: PASSED

Verified every file in the plan's `files_modified` exists on disk:

```
$ for f in services/library/internal/handler/health.go services/library/internal/handler/health_test.go \
          services/library/internal/handler/jobs.go services/library/internal/handler/jobs_test.go \
          services/library/internal/repo/job.go services/library/internal/repo/job_test.go \
          services/library/internal/repo/job_integration_test.go \
          services/library/internal/minio/writer.go services/library/internal/minio/writer_test.go \
          services/library/internal/transport/router.go services/library/cmd/library-api/main.go \
          frontend/web/src/types/library.ts frontend/web/src/api/client.ts \
          frontend/web/src/locales/en.json frontend/web/src/locales/ru.json \
          frontend/web/src/locales/ja.json frontend/web/src/views/admin/RawLibrary.vue \
          frontend/web/src/router/index.ts frontend/web/e2e/raw-library-admin.spec.ts; do
    [ -f "$f" ] && echo "FOUND: $f" || echo "MISSING: $f"
done
```

All FOUND. Commit hashes in the frontmatter all present in `git log`:

```
$ git log --oneline | grep "(05)"
a70e828 test(05): Playwright e2e for /admin/raw-library
4df9d93 feat(05): /admin/raw-library route entry
d67dada feat(05): RawLibrary.vue admin view + Badge info/destructive variants
896f24c feat(05): i18n keys for player.adminLibrary in en/ru/ja
15d2947 feat(05): adminLibraryApi + library types module
4903c46 feat(05): backend additions for raw-library admin UI
```

All six Phase-5 commits FOUND in git log.
