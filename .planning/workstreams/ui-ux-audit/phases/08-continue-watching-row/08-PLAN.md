---
phase: 08-continue-watching-row
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - services/player/internal/domain/watch.go
  - services/player/internal/repo/progress.go
  - services/player/internal/repo/progress_test.go
  - services/player/internal/service/progress.go
  - services/player/internal/handler/progress.go
  - services/player/internal/transport/router.go
  - frontend/web/src/api/client.ts
  - frontend/web/src/composables/useContinueWatching.ts
  - frontend/web/src/components/home/ContinueWatchingRow.vue
  - frontend/web/src/views/Home.vue
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/locales/ja.json
autonomous: true
requirements:
  - UX-15
must_haves:
  truths:
    - "Logged-in user with in-progress watch_progress rows sees a Continue-Watching row at the top of Home, above the trending row."
    - "Each Continue-Watching card shows the anime poster, localized title, episode badge, and a thin progress bar reflecting progress/duration."
    - "Clicking a card routes to /anime/{id}?episode={N} so the player resumes on the correct episode."
    - "Anonymous users do not see the row; logged-in users with zero in-progress rows do not see the row."
    - "GET /api/users/continue-watching returns the latest in-progress episode per anime (one row per anime), ordered by last_watched_at DESC, capped at limit (default 10, max 20)."
    - "Row strings render correctly in en / ru / ja (no untranslated keys)."
  artifacts:
    - path: "services/player/internal/domain/watch.go"
      provides: "ContinueWatchingItem DTO"
      contains: "type ContinueWatchingItem struct"
    - path: "services/player/internal/repo/progress.go"
      provides: "ProgressRepository.ListContinueWatching method"
      contains: "func (r *ProgressRepository) ListContinueWatching"
    - path: "services/player/internal/repo/progress_test.go"
      provides: "ListContinueWatching unit test (happy + empty)"
      contains: "TestProgressRepository_ListContinueWatching"
    - path: "services/player/internal/service/progress.go"
      provides: "ProgressService.ListContinueWatching method"
      contains: "func (s *ProgressService) ListContinueWatching"
    - path: "services/player/internal/handler/progress.go"
      provides: "ProgressHandler.ListContinueWatching HTTP handler"
      contains: "func (h *ProgressHandler) ListContinueWatching"
    - path: "services/player/internal/transport/router.go"
      provides: "GET /users/continue-watching route inside the JWT-protected /users group"
      contains: "continue-watching"
    - path: "frontend/web/src/api/client.ts"
      provides: "userApi.getContinueWatching(limit?)"
      contains: "getContinueWatching"
    - path: "frontend/web/src/composables/useContinueWatching.ts"
      provides: "useContinueWatching composable mirroring useRecs shape"
      contains: "export function useContinueWatching"
    - path: "frontend/web/src/components/home/ContinueWatchingRow.vue"
      provides: "Continue-Watching horizontal row component"
      min_lines: 40
    - path: "frontend/web/src/views/Home.vue"
      provides: "ContinueWatchingRow mounted above trending row"
      contains: "ContinueWatchingRow"
    - path: "frontend/web/src/locales/en.json"
      provides: "home.continueWatching + home.continueWatchingEpisode keys"
      contains: "continueWatching"
    - path: "frontend/web/src/locales/ru.json"
      provides: "Russian copy for Continue-Watching row"
      contains: "continueWatching"
    - path: "frontend/web/src/locales/ja.json"
      provides: "Japanese copy for Continue-Watching row"
      contains: "continueWatching"
  key_links:
    - from: "frontend/web/src/views/Home.vue"
      to: "/api/users/continue-watching"
      via: "useContinueWatching composable -> userApi.getContinueWatching -> apiClient.get"
      pattern: "getContinueWatching"
    - from: "services/player/internal/handler/progress.go"
      to: "services/player/internal/repo/progress.go"
      via: "ProgressService.ListContinueWatching delegate"
      pattern: "ListContinueWatching"
    - from: "services/player/internal/repo/progress.go (window-function SQL)"
      to: "watch_progress + animes tables"
      via: "GORM .Raw(...).Scan(...) returning AnimeInfo + episode_number + progress + duration + last_watched_at + dropped_off_at"
      pattern: "ROW_NUMBER\\(\\) OVER \\(PARTITION BY anime_id ORDER BY last_watched_at DESC\\)"
---

# Phase 8 Plan: Continue-Watching home row (Phoenix new feature)

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

New backend endpoint + new frontend row. Closes audit finding UA-061 (the
single largest UX delta in the 2026-05-12 audit) and addresses Tier-E #1
from the competitive benchmark. Backend ships first as an independent unit;
frontend lands on top of the verified API. No new database tables — reuses
`watch_progress` + JOIN `animes`. No new gateway route — the existing
`/users/*` JWT-protected proxy at `services/gateway/internal/transport/router.go:229`
catches `/api/users/continue-watching` automatically.

## Tasks

### Backend — Wave 1

#### B1. `ContinueWatchingItem` DTO (`services/player/internal/domain/watch.go`)

- [ ] Append a new struct to `services/player/internal/domain/watch.go`
  immediately after the existing `WatchlistStats` block (around line 226).
  Shape:
  ```go
  // ContinueWatchingItem is the per-row payload of GET /users/continue-watching.
  // One item per anime — the user's most-recent in-progress episode for that
  // anime, with the AnimeInfo projection inlined for poster/title rendering.
  // Phase 8 (UX-15 / UA-061).
  type ContinueWatchingItem struct {
      Anime         AnimeInfo `json:"anime"`
      EpisodeNumber int       `json:"episode_number"`
      Progress      int       `json:"progress"`
      Duration      int       `json:"duration"`
      LastWatchedAt time.Time `json:"last_watched_at"`
      DroppedOffAt  *int      `json:"dropped_off_at,omitempty"`
  }
  ```
- [ ] This DTO is **not** a GORM model — no `TableName()`, no
  `gorm:"primaryKey"` tags. The repo populates it via `.Raw(...).Scan(...)`
  into a flat scan struct then maps to this nested shape. Keep the nested
  `Anime AnimeInfo` field so the frontend gets a single coherent shape
  (mirrors the existing `AnimeListEntry.Anime *AnimeInfo` pattern at line 62).

#### B2. `ProgressRepository.ListContinueWatching` (`services/player/internal/repo/progress.go`)

- [ ] Append a new exported method to `ProgressRepository` after
  `GetByUserAnimeEpisode` (current end-of-file). Signature:
  ```go
  func (r *ProgressRepository) ListContinueWatching(
      ctx context.Context, userID string, limit int,
  ) ([]*domain.ContinueWatchingItem, error)
  ```
- [ ] Default + clamp the limit at the top of the method:
  ```go
  if limit <= 0 {
      limit = 10
  }
  if limit > 20 {
      limit = 20
  }
  ```
- [ ] Query strategy — single SQL statement using a window function. Use a
  CTE that ranks rows per anime by `last_watched_at DESC`, then filters
  `rn = 1`. The outer SELECT joins the latest-row CTE to `animes`. **Use
  GORM's `.Raw(...).Scan(...)` with a flat scan struct** because GORM's
  query builder cannot express `ROW_NUMBER() OVER (...)` cleanly and we
  want the SQL readable at the call site. Pattern:
  ```go
  type scanRow struct {
      AnimeID         string    `gorm:"column:anime_id"`
      EpisodeNumber   int       `gorm:"column:episode_number"`
      Progress        int       `gorm:"column:progress"`
      Duration        int       `gorm:"column:duration"`
      LastWatchedAt   time.Time `gorm:"column:last_watched_at"`
      DroppedOffAt    *int      `gorm:"column:dropped_off_at"`
      AnimeName       string    `gorm:"column:anime_name"`
      AnimeNameRU     string    `gorm:"column:anime_name_ru"`
      AnimeNameJP     string    `gorm:"column:anime_name_jp"`
      AnimePoster     string    `gorm:"column:anime_poster"`
      AnimeEpisodes   int       `gorm:"column:anime_episodes"`
  }

  sqlStr := `
      WITH ranked AS (
          SELECT
              wp.anime_id,
              wp.episode_number,
              wp.progress,
              wp.duration,
              wp.last_watched_at,
              wp.dropped_off_at,
              ROW_NUMBER() OVER (
                  PARTITION BY wp.anime_id
                  ORDER BY wp.last_watched_at DESC
              ) AS rn
          FROM watch_progress wp
          WHERE wp.user_id = ?
            AND wp.completed = false
      )
      SELECT
          r.anime_id,
          r.episode_number,
          r.progress,
          r.duration,
          r.last_watched_at,
          r.dropped_off_at,
          a.name           AS anime_name,
          a.name_ru        AS anime_name_ru,
          a.name_jp        AS anime_name_jp,
          a.poster_url     AS anime_poster,
          a.episodes_count AS anime_episodes
      FROM ranked r
      JOIN animes a ON a.id = r.anime_id
      WHERE r.rn = 1
        AND a.deleted_at IS NULL
      ORDER BY r.last_watched_at DESC
      LIMIT ?`

  var rows []scanRow
  if err := r.db.WithContext(ctx).
      Raw(sqlStr, userID, limit).
      Scan(&rows).Error; err != nil {
      return nil, err
  }
  ```
- [ ] Map scan rows to `[]*domain.ContinueWatchingItem`:
  ```go
  items := make([]*domain.ContinueWatchingItem, 0, len(rows))
  for _, row := range rows {
      items = append(items, &domain.ContinueWatchingItem{
          Anime: domain.AnimeInfo{
              ID:            row.AnimeID,
              Name:          row.AnimeName,
              NameRU:        row.AnimeNameRU,
              NameJP:        row.AnimeNameJP,
              PosterURL:     row.AnimePoster,
              EpisodesCount: row.AnimeEpisodes,
          },
          EpisodeNumber: row.EpisodeNumber,
          Progress:      row.Progress,
          Duration:      row.Duration,
          LastWatchedAt: row.LastWatchedAt,
          DroppedOffAt:  row.DroppedOffAt,
      })
  }
  return items, nil
  ```
- [ ] Notes:
  - `a.deleted_at IS NULL` matches the existing soft-delete posture for
    `animes`. The `AnimeInfo` projection in `domain/watch.go` line 17 omits
    `DeletedAt` for entries, but the **animes table itself** has a
    `deleted_at` column managed by GORM at the source; explicit filter
    keeps the row hidden once an admin soft-deletes the anime.
  - `Genres` is intentionally **not** loaded — the row component does not
    render genre chips, and avoiding the many-to-many join keeps this
    query single-statement. If a future card needs genres, the frontend
    can fetch from `/anime/{id}`.
  - `ROW_NUMBER() OVER (PARTITION BY ...)` is supported by Postgres 8.4+
    (we run 14+) and is the standard SQL window-function form. GORM's
    `.Raw(...)` does not interpret it — the driver passes it straight
    through.
  - No GREATEST / Postgres-only operators in this query, so the same SQL
    runs against the SQLite test DB (window functions are supported by
    `mattn/go-sqlite3` v1.14+ which the existing `progress_test.go`
    already imports — verify by reading `services/player/go.sum` for
    `mattn/go-sqlite3` major version; v1.14.x is post-3.25 SQLite which
    has window functions).

#### B3. `ProgressService.ListContinueWatching` (`services/player/internal/service/progress.go`)

- [ ] Append a thin delegate method after `MarkDropOff` (current end-of-file):
  ```go
  // ListContinueWatching returns the user's most-recent in-progress episodes,
  // one row per anime, ordered by last_watched_at DESC. Phase 8 (UX-15).
  func (s *ProgressService) ListContinueWatching(
      ctx context.Context, userID string, limit int,
  ) ([]*domain.ContinueWatchingItem, error) {
      return s.progressRepo.ListContinueWatching(ctx, userID, limit)
  }
  ```
- [ ] No preference-service interaction here — Continue-Watching is purely
  driven by `watch_progress`, not the prefs engine. The combo fields on
  `UpdateProgressRequest` belong to the heartbeat path.

#### B4. `ProgressHandler.ListContinueWatching` (`services/player/internal/handler/progress.go`)

- [ ] Append a new handler method after `MarkDropOff` (current end-of-file):
  ```go
  // ListContinueWatching returns the Continue-Watching row for the
  // authenticated user — at most `limit` (default 10, max 20) anime, one
  // row per anime, ordered by last_watched_at DESC. Phase 8 (UX-15 / UA-061).
  func (h *ProgressHandler) ListContinueWatching(w http.ResponseWriter, r *http.Request) {
      claims, ok := authz.ClaimsFromContext(r.Context())
      if !ok || claims == nil {
          httputil.Unauthorized(w)
          return
      }

      limit := 10
      if v := r.URL.Query().Get("limit"); v != "" {
          if n, err := strconv.Atoi(v); err == nil && n > 0 {
              limit = n
          }
      }

      items, err := h.progressService.ListContinueWatching(r.Context(), claims.UserID, limit)
      if err != nil {
          h.log.Errorw("failed to list continue-watching",
              "user_id", claims.UserID, "error", err)
          httputil.Error(w, err)
          return
      }
      // Always return a non-nil slice so the frontend can safely call .length.
      if items == nil {
          items = []*domain.ContinueWatchingItem{}
      }
      httputil.OK(w, items)
  }
  ```
- [ ] Add the `"strconv"` import to the existing import block.
- [ ] The repo already clamps `limit` to `[1, 20]` so no additional handler-
  side bounds check is needed; the handler only defaults to 10 when the
  query param is missing or unparseable.

#### B5. Route registration (`services/player/internal/transport/router.go`)

- [ ] Inside the existing JWT-protected `r.Route("/users", ...)` block
  (line 59 onward), add the new route immediately after the Progress
  routes group (after line 75, before line 77 "History routes"):
  ```go
  // Continue-Watching row (Phase 8 / UX-15). One row per anime, most
  // recent in-progress episode, ordered by last_watched_at DESC.
  r.Get("/continue-watching", progressHandler.ListContinueWatching)
  ```
- [ ] Verify chi's route table: `/users/continue-watching` is a leaf path
  with no `{animeId}` ambiguity against existing `/users/watchlist/{animeId}`
  or `/users/progress/{animeId}` because chi requires the literal
  `watchlist` / `progress` segment first. No conflict.
- [ ] **No gateway change needed.** The existing
  `r.HandleFunc("/users/*", proxyHandler.ProxyToPlayer)` at
  `services/gateway/internal/transport/router.go:229` already catches this
  path inside the JWT-protected group there.

#### B6. Unit test (`services/player/internal/repo/progress_test.go`)

- [ ] Append a new test function reusing the existing `setupProgressTestDB`
  helper. Pattern (full test, append to end of file):
  ```go
  // TestProgressRepository_ListContinueWatching covers the happy path
  // (multiple anime, latest in-progress row per anime returned) and the
  // empty path (no rows for the user). Phase 8 (UX-15).
  func TestProgressRepository_ListContinueWatching(t *testing.T) {
      r, db := setupProgressTestDB(t)
      ctx := context.Background()

      // Create the animes table the JOIN needs. Minimal columns so the
      // repo's SELECT list lines up — no FK constraint, this is SQLite.
      err := db.Exec(`CREATE TABLE animes (
          id TEXT PRIMARY KEY,
          name TEXT, name_ru TEXT, name_jp TEXT,
          poster_url TEXT,
          episodes_count INTEGER DEFAULT 0,
          deleted_at DATETIME
      )`).Error
      require.NoError(t, err)

      // Seed two anime.
      require.NoError(t, db.Exec(
          `INSERT INTO animes (id, name, poster_url, episodes_count) VALUES (?, ?, ?, ?)`,
          "anime-A", "Anime A", "/a.jpg", 12).Error)
      require.NoError(t, db.Exec(
          `INSERT INTO animes (id, name, poster_url, episodes_count) VALUES (?, ?, ?, ?)`,
          "anime-B", "Anime B", "/b.jpg", 24).Error)

      // Anime A: two in-progress rows (E1 completed, E2 in-progress at
      // later last_watched_at). Latest non-completed row = E2.
      seedProgressRow(t, db, "user-1", "anime-A", 1, 1200, 1400, true)  // completed
      // Bump last_watched_at by overriding the seed manually so the latest
      // non-completed row is the one we expect to surface.
      require.NoError(t, db.Exec(
          `INSERT INTO watch_progress (id, user_id, anime_id, episode_number,
              progress, duration, completed, watch_count, last_watched_at,
              created_at, updated_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
          "seed-A2", "user-1", "anime-A", 2, 600, 1400, false, 1,
          time.Now(), time.Now(), time.Now()).Error)

      // Anime B: single in-progress row, older last_watched_at than A.
      older := time.Now().Add(-1 * time.Hour)
      require.NoError(t, db.Exec(
          `INSERT INTO watch_progress (id, user_id, anime_id, episode_number,
              progress, duration, completed, watch_count, last_watched_at,
              created_at, updated_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
          "seed-B5", "user-1", "anime-B", 5, 300, 1500, false, 1,
          older, older, older).Error)

      // Different user — must NOT leak into user-1's row.
      require.NoError(t, db.Exec(
          `INSERT INTO watch_progress (id, user_id, anime_id, episode_number,
              progress, duration, completed, watch_count, last_watched_at,
              created_at, updated_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
          "seed-X", "user-2", "anime-A", 3, 100, 1400, false, 1,
          time.Now(), time.Now(), time.Now()).Error)

      // --- Happy path ---
      items, err := r.ListContinueWatching(ctx, "user-1", 10)
      require.NoError(t, err)
      require.Len(t, items, 2, "expected one row per anime (A, B)")
      // Anime A first — most recent last_watched_at.
      assert.Equal(t, "anime-A", items[0].Anime.ID)
      assert.Equal(t, 2, items[0].EpisodeNumber, "should be latest in-progress episode (E2), not completed E1")
      assert.Equal(t, 600, items[0].Progress)
      assert.Equal(t, 1400, items[0].Duration)
      assert.Equal(t, "Anime A", items[0].Anime.Name)
      assert.Equal(t, "/a.jpg", items[0].Anime.PosterURL)
      assert.Equal(t, 12, items[0].Anime.EpisodesCount)
      // Anime B second — older last_watched_at.
      assert.Equal(t, "anime-B", items[1].Anime.ID)
      assert.Equal(t, 5, items[1].EpisodeNumber)

      // --- Empty path ---
      empty, err := r.ListContinueWatching(ctx, "user-no-rows", 10)
      require.NoError(t, err)
      assert.Empty(t, empty)

      // --- Limit clamp ---
      // limit=0 -> default 10, limit=999 -> clamp to 20. Both should still
      // return the same two rows here; this is a smoke test of the bounds
      // branches, not of pagination.
      itemsZero, err := r.ListContinueWatching(ctx, "user-1", 0)
      require.NoError(t, err)
      assert.Len(t, itemsZero, 2)
      itemsHuge, err := r.ListContinueWatching(ctx, "user-1", 999)
      require.NoError(t, err)
      assert.Len(t, itemsHuge, 2)
  }
  ```
- [ ] Run from repo root to confirm: `cd services/player && go test ./internal/repo -run TestProgressRepository_ListContinueWatching -v`.

### Backend verification gate — finish Wave 1 before Wave 2

- [ ] `cd services/player && go test ./...` — all existing tests still pass
  AND the new `TestProgressRepository_ListContinueWatching` passes.
- [ ] `cd services/player && go vet ./...` — no new vet findings.
- [ ] `make redeploy-player` — player service rebuilt and the new endpoint
  is live.
- [ ] Smoke against production (this server IS production per
  `project_deployment.md`):
  ```bash
  source docker/.env
  curl -s -H "Authorization: Bearer $UI_AUDIT_API_KEY" \
    http://localhost:8000/api/users/continue-watching | jq '.data | length, .data[0]'
  ```
  Expected: `length` > 0 (Phase 3 seeded `watch_progress` rows for
  `ui_audit_bot`); first item has `anime.id`, `anime.poster_url`,
  `episode_number`, `progress`, `duration` populated.
- [ ] Smoke unauthenticated: `curl -s http://localhost:8000/api/users/continue-watching`
  returns HTTP 401 (no token).

### Frontend — Wave 2

#### F1. API client extension (`frontend/web/src/api/client.ts`)

- [ ] Add a method to the existing `userApi` object (after
  `getProgress` at line 249):
  ```typescript
  getContinueWatching: (limit?: number) =>
    apiClient.get('/users/continue-watching', {
      params: typeof limit === 'number' ? { limit } : undefined,
    }),
  ```
- [ ] No new top-level export — `userApi` is already imported by composables.

#### F2. New composable (`frontend/web/src/composables/useContinueWatching.ts`)

- [ ] Create the file with the same shape as `useRecs.ts` (onMounted fetch
  + auth-state watcher + `{ items, isLoading, error, refresh }`). Full
  contents:
  ```typescript
  import { ref, onMounted, watch } from 'vue'
  import { userApi } from '@/api/client'
  import { useAuthStore } from '@/stores/auth'

  // Phase 8 (UX-15 / UA-061): Continue-Watching row for the logged-in Home
  // view. Anonymous users are never even fetched — the composable returns
  // `items: []` and the row is hidden by the parent. We still mount the
  // auth watcher so a login transition triggers the first fetch without
  // a page reload.
  //
  // Backend contract (services/player/internal/handler/progress.go):
  //   GET /api/users/continue-watching?limit=N
  //   -> { success, data: ContinueWatchingItem[] }

  export interface ContinueWatchingAnime {
    id: string
    name?: string
    name_ru?: string
    name_jp?: string
    poster_url?: string
    episodes_count?: number
  }

  export interface ContinueWatchingItem {
    anime: ContinueWatchingAnime
    episode_number: number
    progress: number
    duration: number
    last_watched_at: string
    dropped_off_at?: number | null
  }

  export function useContinueWatching(limit = 10) {
    const items = ref<ContinueWatchingItem[]>([])
    const isLoading = ref(false)
    const error = ref<string | null>(null)
    const auth = useAuthStore()

    async function fetchItems() {
      // Anonymous users skip the fetch entirely — the endpoint is JWT-
      // protected and would return 401.
      if (!auth.token) {
        items.value = []
        return
      }
      isLoading.value = true
      error.value = null
      try {
        const res = await userApi.getContinueWatching(limit)
        const data = (res.data?.data ?? res.data) as ContinueWatchingItem[]
        items.value = Array.isArray(data) ? data : []
      } catch (e) {
        error.value = e instanceof Error ? e.message : 'failed to load continue-watching'
        items.value = []
      } finally {
        isLoading.value = false
      }
    }

    onMounted(fetchItems)

    // Re-fetch on auth transitions (login from anonymous, logout to anonymous).
    watch(
      () => auth.token,
      (newToken, oldToken) => {
        if (newToken !== oldToken && oldToken !== undefined) {
          fetchItems()
        }
      },
    )

    return { items, isLoading, error, refresh: fetchItems }
  }
  ```

#### F3. New row component (`frontend/web/src/components/home/ContinueWatchingRow.vue`)

- [ ] Create the `frontend/web/src/components/home/` directory if it does
  not already exist (it currently does not — see `ls components/`). This
  directory is intentionally pre-created for Phase 16 ("На этой неделе"
  schedule row) which will follow the same pattern.
- [ ] Create the component file with this contents:
  ```vue
  <template>
    <!-- Phase 8 (UX-15 / UA-061). Hidden when no items so logged-in users
         with zero in-progress rows see no degraded affordance — the
         trending row below remains the top-of-home anchor in that case. -->
    <div v-if="items.length > 0" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
      <div class="flex items-center justify-between mb-4">
        <h2 class="text-xl md:text-2xl font-bold text-white">
          {{ $t('home.continueWatching') }}
        </h2>
      </div>
      <div class="flex gap-3 overflow-x-auto scrollbar-hide pb-2 -mx-1 px-1">
        <router-link
          v-for="item in items"
          :key="item.anime.id + ':' + item.episode_number"
          :to="`/anime/${item.anime.id}?episode=${item.episode_number}`"
          class="flex-shrink-0 w-32 md:w-40 lg:w-48 group"
        >
          <div class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] mb-2">
            <img
              :src="item.anime.poster_url || '/placeholder.svg'"
              alt=""
              class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
              loading="lazy"
            />
            <!-- Episode badge -->
            <div class="absolute top-2 right-2 px-2 py-1 rounded-md bg-black/70 backdrop-blur-sm text-xs font-semibold text-white">
              {{ $t('home.continueWatchingEpisode', { n: item.episode_number }) }}
            </div>
            <!-- Thin progress bar at the bottom of the poster -->
            <div class="absolute bottom-0 left-0 right-0 h-[2px] bg-white/10">
              <div
                class="h-full bg-cyan-400 transition-all"
                :style="{ width: progressPct(item) + '%' }"
              />
            </div>
          </div>
          <h3 class="text-sm font-medium text-white truncate group-hover:text-cyan-400 transition-colors">
            {{ getLocalizedTitle(item.anime.name, item.anime.name_ru, item.anime.name_jp) }}
          </h3>
        </router-link>
      </div>
    </div>
    <!-- Loading skeleton — matches the trending-row loading skeleton at
         Home.vue lines 89-97 for visual consistency. -->
    <div v-else-if="isLoading" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
      <div class="h-8 w-48 bg-white/10 rounded animate-pulse mb-4" />
      <div class="flex gap-3 overflow-hidden">
        <div
          v-for="i in 6"
          :key="i"
          class="flex-shrink-0 w-32 md:w-40 lg:w-48 aspect-[2/3] bg-white/10 rounded-xl animate-pulse"
        />
      </div>
    </div>
  </template>

  <script setup lang="ts">
  import { getLocalizedTitle } from '@/utils/title'
  import { useContinueWatching, type ContinueWatchingItem } from '@/composables/useContinueWatching'

  const { items, isLoading } = useContinueWatching(10)

  function progressPct(item: ContinueWatchingItem): number {
    if (!item.duration || item.duration <= 0) return 0
    const pct = (item.progress / item.duration) * 100
    // Cap at 100 in case progress > duration (clock skew between heartbeats).
    return Math.max(0, Math.min(100, pct))
  }
  </script>
  ```
- [ ] No emit, no props — the component owns its own data via the
  composable. Mirrors the self-contained pattern of `ActivityFeed.vue`
  and `LastUpdates.vue` already used on Home.

#### F4. Mount on Home (`frontend/web/src/views/Home.vue`)

- [ ] Add the import next to the existing component imports (around line 383,
  after `LastUpdates`):
  ```ts
  import ContinueWatchingRow from '@/components/home/ContinueWatchingRow.vue'
  ```
- [ ] Mount the component **above** the trending-row block. Insert it
  immediately after the search-bar `</div>` at line 25 and before the
  `<!-- Trending Now Row -->` comment at line 27. The new mount line:
  ```vue
  <!-- Continue-Watching row (Phase 8 / UX-15). Hidden when anonymous OR
       when the logged-in user has no in-progress watch_progress rows.
       The component itself owns the v-if so we just always mount it. -->
  <ContinueWatchingRow />
  ```
- [ ] Do **not** import `useAuthStore` here for this row — the component's
  composable already gates the fetch on `auth.token`. Anonymous users
  get `items.length === 0` and the component renders nothing.
- [ ] Verify the existing trending-row block at line 32 onward is
  unchanged — the only Home.vue edit is the one new import + the one new
  `<ContinueWatchingRow />` mount line.

#### F5. i18n keys (en / ru / ja)

- [ ] `frontend/web/src/locales/en.json` — inside the existing `home` object
  (around line 38, after `"episodeCount": "{count} ep."`), append:
  ```json
  "continueWatching": "Continue Watching",
  "continueWatchingEpisode": "Episode {n}"
  ```
  (Adjust the trailing comma on the line above so the JSON remains valid.)
- [ ] `frontend/web/src/locales/ru.json` — same keys in Russian:
  ```json
  "continueWatching": "Продолжить просмотр",
  "continueWatchingEpisode": "Серия {n}"
  ```
- [ ] `frontend/web/src/locales/ja.json` — same keys in Japanese:
  ```json
  "continueWatching": "続きから見る",
  "continueWatchingEpisode": "第{n}話"
  ```
- [ ] Key naming: **flat** keys (`home.continueWatching`, not
  `home.continueWatching.title`) because the CONTEXT spec called for
  `home.continueWatching.episode` but vue-i18n does not allow a string
  value at `home.continueWatching` AND a nested object at the same key.
  We flatten to `home.continueWatching` + `home.continueWatchingEpisode`
  to match the existing flat shape in the file (see `episodeCount`,
  `noOngoing`, etc.). The component template uses these keys verbatim.

### Frontend verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` — passes; the new
  composable + component types check cleanly (new file `useContinueWatching.ts`
  exports a typed shape; new component imports the type explicitly).
- [ ] `cd frontend/web && bunx eslint src/composables/useContinueWatching.ts src/components/home/ContinueWatchingRow.vue src/views/Home.vue src/api/client.ts` — zero errors / zero warnings.
- [ ] JSON validity: `cd frontend/web && bun -e "JSON.parse(require('fs').readFileSync('src/locales/en.json','utf8')); JSON.parse(require('fs').readFileSync('src/locales/ru.json','utf8')); JSON.parse(require('fs').readFileSync('src/locales/ja.json','utf8')); console.log('ok')"` prints `ok`.
- [ ] `make redeploy-web` — frontend rebuilt and shipped.
- [ ] Manual smoke as `ui_audit_bot` against
  `https://animeenigma.ru/`:
  1. Log in as `ui_audit_bot` (or inject the JWT into `localStorage.token`
     using the audit-framework pattern from CLAUDE.md).
  2. Reload `/`. The Continue-Watching row appears **above** the trending
     row, populated from the Phase 3-seeded `watch_progress` rows.
  3. Each card shows poster + episode badge (`Episode N` in en /
     `Серия N` in ru / `第N話` in ja, depending on locale) + a thin cyan
     progress bar at the bottom of the poster.
  4. Click any card. The router navigates to
     `/anime/{id}?episode=N` and the player loads the right episode.
  5. Log out. Reload `/`. The Continue-Watching row is no longer
     rendered. The trending row remains at the top. No console errors.
- [ ] Manual locale probe: switch the UI locale (en/ru/ja) via the
  Navbar locale switcher. Confirm the row label and the per-card
  episode badge swap to the right language. No untranslated key
  fallback (no raw `home.continueWatching` string visible).
- [ ] Chrome MCP axe-core re-run on `https://animeenigma.ru/` (logged-in
  as `ui_audit_bot`):
  - Zero new violations on Home — the new row uses the same `<h2>` row
    label + `<h3>` card title pattern that Phase 7 normalized.
  - Specifically: `heading-order`, `image-alt`, `color-contrast`,
    `link-name` all stay clean.

## Files touched

```
services/player/internal/domain/watch.go                          (+ ContinueWatchingItem DTO)
services/player/internal/repo/progress.go                         (+ ListContinueWatching)
services/player/internal/repo/progress_test.go                    (+ TestProgressRepository_ListContinueWatching)
services/player/internal/service/progress.go                      (+ ListContinueWatching delegate)
services/player/internal/handler/progress.go                      (+ ListContinueWatching handler + strconv import)
services/player/internal/transport/router.go                      (+ GET /users/continue-watching route)
frontend/web/src/api/client.ts                                    (+ userApi.getContinueWatching)
frontend/web/src/composables/useContinueWatching.ts               (NEW)
frontend/web/src/components/home/ContinueWatchingRow.vue          (NEW; new directory components/home/)
frontend/web/src/views/Home.vue                                   (+ import + <ContinueWatchingRow /> mount)
frontend/web/src/locales/en.json                                  (+ home.continueWatching, home.continueWatchingEpisode)
frontend/web/src/locales/ru.json                                  (+ same keys, RU copy)
frontend/web/src/locales/ja.json                                  (+ same keys, JA copy)
.planning/workstreams/ui-ux-audit/phases/08-continue-watching-row/
  08-CONTEXT.md                                                   (already exists)
  08-PLAN.md                                                      (this file)
  08-SUMMARY.md                                                   (written at execute-phase end)
  08-VERIFICATION.md                                              (written at execute-phase end)
```

No new database tables. No new gateway routes. No new external libraries.
No new GORM auto-migration step needed — the repo method runs raw SQL
against existing `watch_progress` + `animes` tables.

## Closes

| Requirement | Finding | Surface | Mechanism |
|---|---|---|---|
| UX-15 | UA-061 (Tier-E #1) | New row above trending on logged-in Home | `GET /api/users/continue-watching` window-function query against `watch_progress` joined with `animes`; new `ContinueWatchingRow.vue` mounts in `Home.vue` and gates rendering on `items.length > 0` so anonymous and zero-progress users see no degraded affordance |

## Wave outline

| Wave | Tasks | Rationale |
|---|---|---|
| 1 (Backend) | B1 DTO -> B2 repo + SQL -> B3 service -> B4 handler -> B5 route -> B6 unit test -> backend verification gate (`go test`, `make redeploy-player`, curl smoke) | Backend ships + verifies as a single atomic unit. The endpoint exists and returns valid data before any frontend code is written. |
| 2 (Frontend + i18n) | F1 api client -> F2 composable -> F3 row component -> F4 Home mount -> F5 i18n keys (en/ru/ja) -> frontend verification (vue-tsc, eslint, redeploy-web, manual smoke + axe re-run) | Frontend depends on the verified Wave 1 endpoint. i18n keys are written as part of Wave 2 because the component template references them — adding them in a later wave would break the build. |
