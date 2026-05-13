# Phase 8: Continue-Watching home row - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, new feature spanning Go backend + Vue frontend)

<domain>
## Phase Boundary

Logged-in Home view renders a **Continue-Watching** row above the existing trending row when the user has uncompleted episodes in `watch_progress`. Closes audit finding UA-061 and addresses Tier-E #1 (the single largest UX delta in the audit).

**Backend deliverables (`services/player/`):**
- New repo method: `ProgressRepository.ListContinueWatching(userID, limit)` returning the user's most-recent in-progress episode per anime, joined with anime info, ordered by `last_watched_at DESC`.
- "In-progress" definition: the latest row in `watch_progress` per (user, anime) where:
  - `completed = false`, OR
  - `completed = true` AND there is a higher-numbered episode for the anime that the user has not completed (i.e. the user finished E1 but hasn't started E2).
  - For MVP, restrict to the simpler form: `completed = false` (drop-off). The "completed E1, E2 exists and not started" case is best derived from `anime.episodes_count` + `MAX(episode_number) WHERE completed=true`. We include this in MVP because it's the Crunchyroll-grade pattern — drop a denormalized field on the response: `next_episode_number = latest_completed + 1` if `< episodes_count`, else equal to the dropped-off episode.
- New service method: `ProgressService.ListContinueWatching(userID, limit)`.
- New handler: `GET /users/continue-watching?limit=20` — JWT-protected (uses existing `/users/*` group in gateway).
- DTO: `ContinueWatchingItem { anime: AnimeInfo, episode_number: int, progress: int, duration: int, last_watched_at: timestamp, dropped_off_at: *int }`.
- Limit: max 20, default 10.

**Frontend deliverables (`frontend/web/`):**
- New composable: `useContinueWatching()` in `frontend/web/src/composables/` — fetches `/api/users/continue-watching`, exposes `{ items, loading, error, refetch }`.
- New row component: `frontend/web/src/components/home/ContinueWatchingRow.vue` — horizontal-scroll card row matching the trending-row visual pattern (lines 32-88 of Home.vue). Each card shows: poster, anime title, episode badge ("Серия N" / "Episode N" / "第N話"), thin progress bar at the bottom of the poster representing `progress / duration`.
- Mount in `Home.vue` **above** the existing trending row block but **only when `authStore.isAuthenticated && items.length > 0`**. Anonymous users never see it.
- Empty state: hidden (don't render the row at all if no items — matches the existing trending-row pattern).
- Loading state: shimmer skeleton matching the existing trending-row loading skeleton at lines 89-97.
- Click behavior: each card routes to `/anime/{id}?episode={N}` so the player resumes on the right episode. Use the existing `:to` pattern but with query string.
- i18n: new keys `home.continueWatching` (row label), `home.continueWatching.episode` (badge label with `{n}` placeholder).

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

- **Backend query strategy**: single SQL query using a window function (or subquery) to grab the LATEST row per (user, anime) — `ROW_NUMBER() OVER (PARTITION BY anime_id ORDER BY last_watched_at DESC) AS rn` then filter `rn = 1`. JOIN to `animes` table for poster/name/episodes_count. WHERE clause: `user_id = ? AND completed = false`. Order: `last_watched_at DESC`. Limit: param-bound. This is the Crunchyroll-grade Continue-Watching shape.
- **Completed-but-next-episode-available case**: deferred to a follow-up. For MVP, only show titles where the latest row is `completed=false` (genuine in-progress / dropped-off). Otherwise we'd need a subquery joining `episodes_count` from animes and `MAX(episode_number) WHERE completed=true` from watch_progress — adds complexity for the 15% of cases. Phase 9 (per-card progress) revisits this.
- **Auth on the endpoint**: JWT-required (no anonymous), matches the audit's spec ("logged-in Home view"). Routes through the existing protected `/users/*` chi group in `services/gateway/internal/transport/router.go` (line 230-233).
- **Component structure**: extract `ContinueWatchingRow.vue` to `frontend/web/src/components/home/` rather than inlining in Home.vue. Phase 16 ("На этой неделе" schedule row) will follow the same pattern — pre-creating the `components/home/` directory pays off twice.
- **Progress visual**: a thin 2px `bg-cyan-400` bar at the bottom of the poster, width = `(progress / duration) * 100%`. Reuses Tailwind `bg-white/10` for the empty track. Cap to 100% in case `progress > duration` (clock skew).
- **Card sizing**: same as trending row (`w-32 md:w-40 lg:w-48 aspect-[2/3]`) for visual consistency.
- **Pagination**: not in MVP. The endpoint takes `limit` and the row scrolls horizontally — that's sufficient for v0.1. If users have >10 in-progress titles, the row scrolls; >20 is rare enough that a "See all" → `/profile?tab=history` link covers it (deferred).
- **Caching**: the API response is per-user dynamic data. No Redis caching at the player layer — `watch_progress` writes are frequent and a cache would either show stale data or invalidate constantly. Rely on browser-level dedup via the composable's promise.
- **i18n strings**:
  - en: `Continue Watching` / `Episode {n}`
  - ru: `Продолжить просмотр` / `Серия {n}`
  - ja: `続きから見る` / `第{n}話`

### Locked from ROADMAP

- Phase 8 depends on Phase 3 (seed-data sync) — already complete. Test user `ui_audit_bot` will have rendered row populated from seeded `watch_progress` rows.
- Phase 8 is BEFORE Phase 9 (per-card progress) — Phase 9 extends the per-card progress pattern to ALL rows. The row component built here uses progress bar already; Phase 9 generalizes that to RecItem / Browse / Search cards.
- Cannot piggyback Phase 16 (schedule row) here — different data source (Shikimori `nextEpisodeAt`), different component, different empty-state semantics.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `services/player/internal/repo/progress.go:120` — `GetByUserAndAnime` is the closest pattern. Existing GORM idioms in this file use `.Where(...).Order(...).Find(...)`.
- `services/player/internal/service/progress.go:51` — `GetProgress` shape (delegate to repo, return slice). New method follows identical pattern.
- `services/player/internal/handler/progress.go:58` — `GetProgress` handler pattern (extract user from context, call service, JSON-encode response). New handler is shorter — no path params, just query `limit`.
- `services/player/internal/transport/router.go:73-75` — existing route group `/progress/*` is under JWT auth. New route `r.Get("/continue-watching", progressHandler.ListContinueWatching)` goes in the same group.
- `services/gateway/internal/transport/router.go:230-233` — existing `/users/*` JWT-protected proxy catches `/api/users/continue-watching` automatically. No gateway change needed.
- Frontend: `frontend/web/src/composables/useRecs.ts` is the canonical pattern for fetch-on-mount composables returning `{ items, loading, error, refetch }`. `useContinueWatching` mirrors it.
- Frontend: `frontend/web/src/views/Home.vue:32-88` is the trending row visual reference. New row uses the same Tailwind classes for layout consistency.

### Established Patterns

- Go services use `gorm` + `chi` + `domain/service/repo/handler` layering — already documented in CLAUDE.md.
- Frontend: Pinia stores for auth (`useAuthStore`), `vue-router` for navigation, `vue-i18n` for strings, Tailwind for styling. No new libraries.
- API client: `frontend/web/src/api/client.ts` — there's likely a `userApi` or `playerApi` object to extend. Verify and add `getContinueWatching()` there.

### Integration Points

- Database migration: GORM auto-migrates new model fields. No new tables — reuses `watch_progress` + JOIN `animes`. No migration needed.
- Backend test: existing `services/player/internal/repo/progress_test.go` is the unit-test pattern. Add `TestProgressRepository_ListContinueWatching`. (Note: this workstream historically doesn't enforce test additions; existing patterns suggest happy-path + empty case as minimum.)
- Frontend test: no Playwright test required for v0.1 — manual smoke against `ui_audit_bot` seeded data is sufficient (audit framework reuses the seeded account).

</code_context>

<specifics>
## Specific Ideas

- The audit explicitly calls Continue-Watching "the single largest UX delta" — visual prominence matters. Place the row at the very top of Home (above trending) for logged-in users. Anonymous users continue to see trending at top (no change).
- The thin progress bar on the poster is a "Crunchyroll-grade" detail; do not skip it. It's 80% of the perceived value of the row.
- Empty state hidden by design — no "You haven't watched anything yet" CTA in v0.1. The row simply doesn't render, and the existing trending row remains the top-of-home affordance.
- If a user is logged in but has zero in-progress watch_progress rows (brand new account), the row is hidden — that's the empty state. Acceptable for v0.1 since logged-in users without progress see the trending row directly above other content.

</specifics>

<deferred>
## Deferred Ideas

- "Next episode" CTA (completed E1 with E2 unwatched) — Phase 9 / per-card progress phase will revisit. Adds backend query complexity.
- "See all in-progress titles" link to a dedicated `/profile?tab=in-progress` view — Phase 11 (Quick-Nav + status banner phase) or Phase 20 polish.
- Skeleton screen with placeholder cards — current loading state is a row of shimmer rectangles; could later transition to actual card silhouettes with a subtle gradient sweep. Polish.
- Surface "dropped-off" titles distinctly from "in-progress" titles (red badge?) — not in v0.1 scope; the `dropped_off_at` field is exposed on the DTO so a follow-up can render it.

</deferred>
