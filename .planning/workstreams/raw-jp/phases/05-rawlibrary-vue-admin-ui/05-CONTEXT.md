# Phase 5: RawLibrary.vue Admin UI - Context

**Gathered:** 2026-05-18
**Status:** Ready for planning
**Mode:** Auto-generated (SPEC pre-written, ambiguity_score 0.20)

<domain>
## Phase Boundary

Admin-only Vue view at `/admin/raw-library` with three vertically-stacked sections:
1. **Stats strip** — disk free, active torrents, active jobs (30s refresh)
2. **Search panel** — Nyaa + AnimeTosho search, queue button per row
3. **Jobs panel** — active jobs with progress + cancel; failed jobs with retry; pending-link sub-panel for `done` jobs without `shikimori_id`

Plus three small backend additions:
- `GET /api/library/health/extended` — extended stats
- `PATCH /api/library/jobs/{id}` — retroactive `shikimori_id` link (triggers MinIO move + episode insert)
- `POST /api/library/jobs/{id}/retry` — re-enqueue a failed job

**Out of scope:** Hybrid resolver (Phase 6), user-facing library listing, bulk queue, per-uploader pattern editor UI, storage cleanup.

</domain>

<decisions>
## Implementation Decisions

### Locked from SPEC (`milestones/v0.2-phases/05-rawlibrary-admin-ui/05-SPEC.md`)

- **Route:** `/admin/raw-library` with `beforeEnter: requireAdmin` guard (existing helper).
- **Polling:** 5s for jobs, 30s for stats. No WebSockets in v0.2 (admin-only, low-traffic).
- **Provider chip:** `Badge.vue` with cyan for AnimeTosho, purple for Nyaa.
- **Cancel confirmation:** Only for `downloading|encoding|uploading` states. `queued` cancels go through without prompt.
- **Retry semantics:** Inherits magnet + title + uploader + shikimori_id from failed row. New row references old in `error_text`.
- **Link semantics:** PATCH on job resource. Side-effects (MinIO move + episode insert) fire when `shikimori_id` transitions NULL → non-NULL.
- **MinIO move:** Server-side `CopyObject` (no re-encode, no re-download). Delete old `pending/` path after copy succeeds.
- **Search debounce:** 300ms before triggering request; Enter also submits.
- **Anime search dropdown:** Reuse `SearchAutocomplete.vue` if it supports async data source; otherwise inline minimal version with debounced `GET /api/anime/search?q=`.
- **Job progress bar:** Reuse existing Tailwind progress-bar pattern (e.g., from `LastUpdates.vue` or other admin view); no new dependency.

### API Module (locked)

```ts
export const adminLibraryApi = {
  search: (q, malId?, limit=50) => apiClient.get('/library/search', { params: { q, mal_id: malId, limit } }),
  listJobs: (status?, limit=50) => apiClient.get('/library/jobs', { params: { status, limit } }),
  getJob: (id) => apiClient.get(`/library/jobs/${id}`),
  createJob: (payload) => apiClient.post('/library/jobs', payload),
  cancelJob: (id) => apiClient.delete(`/library/jobs/${id}`),
  linkJob: (id, shikimoriId) => apiClient.patch(`/library/jobs/${id}`, { shikimori_id: shikimoriId }),
  retryJob: (id) => apiClient.post(`/library/jobs/${id}/retry`),
  healthExtended: () => apiClient.get('/library/health/extended'),
}
```

### i18n Keys (locked)

Under `player.adminLibrary.*`:
- `title`
- `stats.{diskFree,activeTorrents,activeJobs}`
- `search.{title,placeholder,submit,empty,providers.nyaa,providers.animetosho}`
- `jobs.{title,empty,cancel,retry}`
- `jobs.status.{queued,downloading,encoding,uploading,done,failed,cancelled}`
- `jobs.failed.{title,errorText}`
- `jobs.pendingLink.{title,linkButton,searchPlaceholder}`

Required in en, ru, ja.

### Backend Endpoints (additions)

- `GET /api/library/health/extended` → `{disk_free_bytes, disk_total_bytes, active_torrents, active_jobs_by_status}`.
- `PATCH /api/library/jobs/{id}` body `{shikimori_id}` — if transitioning NULL → non-NULL: server-side MinIO CopyObject from `pending/{job_id}/...` to `{shikimori_id}/{episode}/...`, insert `library_episodes`, delete old path.
- `POST /api/library/jobs/{id}/retry` — creates new queued row with same magnet (admin must have permission).

### Claude's Discretion (autonomous mode)

- Glass-card component composition (reuse existing `Card.vue` or admin-specific).
- Exact debounce implementation (use `lodash-es/debounce` or a small util).
- Job card layout details (icons, spacing).
- Provider chip visual exact shade if `Badge.vue` doesn't already have cyan/purple variants — add them or pick closest.
- Whether to memoize the search results client-side beyond the request itself.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `frontend/web/src/router/index.ts` — has `beforeEnter` admin guards (e.g., for `/admin/grafana`-style routes or existing admin views).
- `frontend/web/src/stores/auth.ts` — `authStore.isAdmin` flag.
- `frontend/web/src/api/client.ts` — central axios-based `apiClient`; add `adminLibraryApi` block.
- `frontend/web/src/components/ui/Badge.vue` — provider chip target.
- `frontend/web/src/views/admin/` — directory exists with admin views (e.g., AdminRecs).
- `frontend/web/src/i18n/` or `src/locales/{en,ru,ja}.json` — i18n message catalog.
- `frontend/web/src/components/ui/Card.vue` (or similar) — glass-card pattern.
- `frontend/web/src/views/LastUpdates.vue` — progress bar reference.
- `services/library/internal/handler/jobs.go` — Phase 3 handler; extend with PATCH + retry.
- `services/library/internal/handler/health.go` — Phase 1 handler; extend with `/health/extended`.
- `services/library/internal/minio/writer.go` — Phase 4 writer; add `Move(src, dst)` server-side CopyObject helper.
- `services/library/internal/repo/job.go` — extend with `UpdateShikimoriID`, `Retry`.

### Established Patterns

- Vue 3 SFC with `<script setup lang="ts">`.
- Pinia store usage via `useAuthStore()`.
- Vue Router beforeEnter guards.
- i18n via `vue-i18n` `useI18n().t(...)`.
- Polling via `setInterval` cleaned up `onUnmounted`.
- API responses unwrapped from `httputil.OK` envelope (`{success, data}`).
- Tailwind utility classes for layout.

### Integration Points

- `frontend/web/src/router/index.ts` — add `/admin/raw-library` route.
- `frontend/web/src/api/client.ts` — add `adminLibraryApi` block (named export).
- `frontend/web/src/locales/{en,ru,ja}.json` — add `player.adminLibrary.*` keys (note SPEC says under `player.*` namespace).
- `services/library/internal/handler/router.go` — wire PATCH + retry routes.
- `services/library/internal/handler/health.go` — add `/health/extended`.

</code_context>

<specifics>
## Specific Ideas

- SPEC reference at `milestones/v0.2-phases/05-rawlibrary-admin-ui/05-SPEC.md` is authoritative.
- Playwright e2e at `frontend/web/e2e/raw-library-admin.spec.ts` covers: admin login, view loads, search, queue job, see in jobs panel, cancel. Live torrent download not asserted.
- The view should follow existing admin view shape (look at `frontend/web/src/views/admin/` for sibling examples).

</specifics>

<deferred>
## Deferred Ideas

- WebSockets for live job progress (overkill for v0.2).
- Bulk queue (queueing season packs) — v0.2.1+.
- Pattern editor UI for filename_patterns — admins use psql.
- Storage cleanup (delete old library episodes) — v0.2.1.
- User-facing library listing — out of scope; v0.3+.

</deferred>
