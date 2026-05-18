---
id: LIB-rawlibrary-admin-ui
title: RawLibrary.vue admin view — search, queue, monitor, link
workstream: raw-jp
milestone: v0.2
phase: 05
created_at: 2026-05-18
status: SPEC-ready
ambiguity_score: 0.20
mode: --auto
---

# Phase 05 (workstream `raw-jp`, milestone v0.2): RawLibrary.vue Admin UI — Specification

**Workstream:** `raw-jp`
**Milestone:** v0.2 Self-Hosted Library
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
**Requirements:** LIB-09
**Depends on:** Phases 2 (search), 3 (jobs), 4 (episodes)
**Mode:** `--auto`

## Goal

Admin-only Vue view at `/admin/raw-library` providing the operational surface for the library service: search Nyaa + AnimeTosho, queue magnets, monitor running jobs with cancel + retry, link orphan jobs (no shikimori_id) to catalog anime, and view disk/peer stats. No end-user visibility.

## Background

**Today, three things are true and need to change:**

1. **No frontend exists for the library backend.** Phases 1-4 ship the API end-to-end; without a UI, admins would `curl` jobs by hand. Not viable for day-to-day operations.

2. **The catalog already has an admin-routing pattern.** `frontend/web/src/router/index.ts` has `beforeEnter` guards against `authStore.isAdmin`. Reuse the pattern; don't invent a new auth flow.

3. **The library service exposes everything the UI needs.** The existing endpoints (`/search`, `/jobs`, `/episodes/{shikimori_id}/{episode}`) plus a small new `/health/extended` are enough. The frontend is pure rendering + polling.

**The implementation:**
- One new view `frontend/web/src/views/admin/RawLibrary.vue` with three sections.
- One new API module `adminLibraryApi` in `frontend/web/src/api/client.ts`.
- One new type file `frontend/web/src/types/library.ts`.
- Router entry + admin guard.
- i18n strings in en/ru/ja.
- Two new small backend endpoints: `GET /api/library/health/extended` (disk + active-torrent summary) and `PATCH /api/library/jobs/{id}` (post-hoc shikimori_id linker + re-link flow).

## Requirements

### LIB-09: RawLibrary.vue admin view

- **Current:** No view, no route, no API client.
- **Target:**
  - **Route:** `/admin/raw-library` registered in `frontend/web/src/router/index.ts` with `beforeEnter: requireAdmin` guard (existing helper).
  - **Component:** `frontend/web/src/views/admin/RawLibrary.vue` with three vertically-stacked sections:

    1. **Stats strip (top, always visible).** Three glass-card tiles:
       - Disk free (% + GB) — from `GET /api/library/health/extended`.
       - Active torrents — `library_active_torrents` value.
       - Active jobs — count from `GET /api/library/jobs?status=queued,downloading,encoding,uploading`.
       - Auto-refresh every 30s.

    2. **Search panel.**
       - Input: title (text) + MAL ID (number, optional). Submit button + Enter key.
       - On submit: `GET /api/library/search?q=&mal_id=&limit=50`.
       - Result table: provider chip (`Badge.vue` with variant cyan for AnimeTosho, purple for Nyaa), uploader, title, quality, size (formatted GB/MB), magnet preview (first 60 chars), "Queue" button per row.
       - "Queue" button: `POST /api/library/jobs` with the magnet + metadata (`shikimori_id` derived from MAL ID via the catalog if available, otherwise NULL). Optimistic UI shows the new job in section 3 with `status='queued'`.
       - Empty state: "Search Nyaa + AnimeTosho to populate the library." copy with localized strings.

    3. **Jobs panel.**
       - Polls `GET /api/library/jobs?status=queued,downloading,encoding,uploading` every 5s.
       - Renders job cards: title, state badge, progress bar (`progress_pct`), bytes downloaded / total, peers, ETA (computed from rate). Cancel button per card.
       - "Failed jobs" sub-section (collapsible) renders failed jobs with their `error_text` and a "Retry" button (re-enqueue with the same magnet).
       - "Pending link" sub-section (highlighted) shows `done` jobs with `shikimori_id=NULL`. Each row has an inline anime-search dropdown (debounced `GET /api/anime/search?q=`). Selecting an anime calls `PATCH /api/library/jobs/{id}` with the new `shikimori_id` and triggers a re-link of the MinIO files into the proper path (backend Move; spec'd below).

  - **API module** `adminLibraryApi`:
    ```ts
    export const adminLibraryApi = {
      search: (q: string, malId?: number, limit = 50) => apiClient.get('/library/search', { params: { q, mal_id: malId, limit } }),
      listJobs: (status?: string, limit = 50) => apiClient.get('/library/jobs', { params: { status, limit } }),
      getJob: (id: string) => apiClient.get(`/library/jobs/${id}`),
      createJob: (payload: CreateJobPayload) => apiClient.post('/library/jobs', payload),
      cancelJob: (id: string) => apiClient.delete(`/library/jobs/${id}`),
      linkJob: (id: string, shikimoriId: string) => apiClient.patch(`/library/jobs/${id}`, { shikimori_id: shikimoriId }),
      retryJob: (id: string) => apiClient.post(`/library/jobs/${id}/retry`),
      healthExtended: () => apiClient.get('/library/health/extended'),
    }
    ```

  - **Type file** `frontend/web/src/types/library.ts`: `Release`, `Job`, `JobStatus`, `Episode`, `LibraryHealth`, `CreateJobPayload`.

  - **i18n keys** in `player.adminLibrary.*` for en/ru/ja:
    - `title` — "Raw Library" / "Сырая библиотека" / "生ライブラリ"
    - `stats.diskFree`, `stats.activeTorrents`, `stats.activeJobs`
    - `search.title`, `search.placeholder`, `search.submit`, `search.empty`, `search.providers.nyaa`, `search.providers.animetosho`
    - `jobs.title`, `jobs.empty`, `jobs.status.{queued,downloading,encoding,uploading,done,failed,cancelled}`, `jobs.cancel`, `jobs.retry`
    - `jobs.failed.title`, `jobs.failed.errorText`
    - `jobs.pendingLink.title`, `jobs.pendingLink.linkButton`, `jobs.pendingLink.searchPlaceholder`

  - **Backend additions in this phase (small):**
    - `GET /api/library/health/extended` — handler returns `{disk_free_bytes, disk_total_bytes, active_torrents, active_jobs_by_status}`.
    - `PATCH /api/library/jobs/{id}` — accepts `{shikimori_id}` to retroactively link a `done` job; triggers a MinIO `CopyObject` from `pending/{job_id}/...` to `{shikimori_id}/{episode}/...` and inserts the `library_episodes` row. The old `pending/` path is deleted after the copy succeeds.
    - `POST /api/library/jobs/{id}/retry` — re-enqueues with the same magnet (creates a new job row in `queued`; old row remains in `failed` for the audit trail).

- **Acceptance:**
  1. `/admin/raw-library` renders with stats + search + jobs sections.
  2. Search returns hits; clicking Queue creates a job that appears in section 3 within 5s.
  3. Job progress bar ticks every 5s; cancel moves the job to `cancelled` and removes it from the active list.
  4. Retry on a failed job creates a new queued row.
  5. Pending-link panel shows a `done` job with NULL shikimori_id; selecting an anime via the dropdown moves the MinIO files + inserts `library_episodes` + removes the row from pending-link.
  6. Non-admin visiting `/admin/raw-library` is redirected.
  7. `bun run build` passes with zero errors.

## Acceptance Criteria

1. View file + router entry + admin guard + API module + types + i18n strings all present.
2. Stats strip auto-refreshes every 30s; values match what `GET /metrics` shows.
3. Search input debounced (300ms) before triggering the request; Enter key also submits.
4. Job cards expand to show `error_text` when status is `failed`.
5. Pending-link flow successfully moves MinIO objects and inserts `library_episodes`.
6. Backend additions (`/health/extended`, `PATCH /jobs/{id}`, `POST /jobs/{id}/retry`) covered by unit tests.
7. `bunx tsc --noEmit` clean. `bun run build` clean.
8. Playwright e2e (`frontend/web/e2e/raw-library-admin.spec.ts`): admin can log in, see the view, queue a job, see it appear, cancel it. (Live torrent download not asserted — too long for an e2e; integration testing of the full pipeline lives in the operator runbook.)

## Auto-selected implementation decisions

- **Polling vs WebSockets:** Polling (5s for jobs, 30s for stats). The library is admin-only and low-traffic; WebSocket plumbing is overkill.
- **Anime search dropdown component:** Reuse `SearchAutocomplete.vue` from `components/ui/` if already supporting an async data source; otherwise inline a minimal version with debounced `GET /api/anime/search?q=`.
- **Job progress bar:** Reuse the existing Tailwind progress-bar pattern from `LastUpdates.vue` or other admin views; no new dependency.
- **Cancel button safety:** Confirm dialog only for jobs in `downloading|encoding|uploading` (interrupting them wastes work); queued cancels go through without prompt.
- **Retry on failed:** Inherits the original magnet + title + uploader + shikimori_id (if linked) from the failed row. The new row references the failed row in `error_text` ("retry of {old_id}") for audit.
- **PATCH for linking (vs separate /link endpoint):** PATCH on the job resource keeps the resource model coherent. The MinIO move + episode-row insert side-effects fire only when `shikimori_id` transitions from NULL to non-NULL.
- **MinIO copy granularity:** Server-side `CopyObject` (MinIO supports this) — no re-encode, no re-download.

## Touches

- **New:** `frontend/web/src/views/admin/RawLibrary.vue`
- **New:** `frontend/web/src/types/library.ts`
- **New:** `frontend/web/e2e/raw-library-admin.spec.ts`
- **Extend:** `frontend/web/src/router/index.ts` (add `/admin/raw-library` route + guard)
- **Extend:** `frontend/web/src/api/client.ts` (add `adminLibraryApi`)
- **Extend:** `frontend/web/src/locales/{en,ru,ja}.json` (add `player.adminLibrary.*` keys)
- **Extend:** `services/library/internal/handler/health.go` (add `/health/extended`)
- **Extend:** `services/library/internal/handler/jobs.go` (add `PATCH /jobs/{id}` + `POST /jobs/{id}/retry`)
- **Extend:** `services/library/internal/service/encoder_worker.go` (re-link helper for the PATCH path)
- **Extend:** `services/library/internal/minio/writer.go` (add `Move(src, dst)` helper using server-side CopyObject)
- **Extend:** `services/library/internal/repo/job.go` (add `UpdateShikimoriID`, `Retry`)

## Out of Scope (for this phase)

- Hybrid resolver (Phase 6).
- User-facing library listing.
- Bulk-queue (queueing a season pack in one click) — admins queue one job at a time for v0.2.
- Per-uploader filename-pattern editor in the UI — admins edit `library_filename_patterns` directly via psql when adding new uploaders.
- Storage cleanup (delete old library episodes) — v0.2.1.

## Citations to design doc

- Architecture → "RawLibrary.vue admin UI" + the per-section description.
- Data flow → Phase 5 in the milestone-level roadmap.
- Tech-choices → "Other-subs UI: Modal panel triggered from player toolbar" (pattern reference for modal use).
- Rollout → "v0.2 (manual library) — admin-only surface, no user-visible change until an admin queues a first job".
