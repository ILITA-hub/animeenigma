---
phase: 09-per-card-progress-subdub
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - services/catalog/internal/domain/anime.go
  - services/catalog/internal/parser/kodik/client.go
  - services/player/internal/domain/watch.go
  - services/player/internal/repo/progress.go
  - services/player/internal/repo/progress_test.go
  - services/player/internal/service/progress.go
  - services/player/internal/handler/progress.go
  - services/player/internal/transport/router.go
  - frontend/web/src/api/client.ts
  - frontend/web/src/composables/useAnimeProgress.ts
  - frontend/web/src/components/anime/AnimeCardNew.vue
  - frontend/web/src/views/Home.vue
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/locales/ja.json
autonomous: true
requirements:
  - UX-16
  - UX-17
  - UX-18
must_haves:
  truths:
    - "Logged-in user with in-progress watch_progress rows sees a small purple progress badge on AnimeCardNew (Browse + Search) and on the trending-row RecItem cards for animes they have progress on."
    - "Cards backed by an Anime row with has_dub=true show a small amber DUB badge in the top-right (paired with the existing quality badge)."
    - "The Ongoing column router-link on Home points to /anime/{id}?episode={N+1} when next_episode_at + episodes_aired are set; bare /anime/{id} otherwise."
    - "GET /api/users/anime-progress?ids=a,b,c returns a JSON map keyed by anime_id containing the per-anime latest episode, episodes_count, episodes_aired, completed, dropped flags; max 50 IDs per request; JWT-required."
    - "Kodik parser writes has_dub=true onto the Anime row when at least one returned translation has type==\"voice\"."
    - "i18n keys card.dubBadge + card.episodeProgress render correctly in en/ru/ja (no untranslated key fallback)."
    - "Anonymous users do not trigger anime-progress fetches; cards render without progress badges and with DUB badges where applicable."
  artifacts:
    - path: "services/catalog/internal/domain/anime.go"
      provides: "Anime.HasDub bool field with GORM default+index"
      contains: "HasDub"
    - path: "services/catalog/internal/parser/kodik/client.go"
      provides: "Kodik parser sets has_dub from translation type==\"voice\""
      contains: "has_dub"
    - path: "services/player/internal/domain/watch.go"
      provides: "BulkAnimeProgressEntry + BulkAnimeProgressMap DTO"
      contains: "type BulkAnimeProgressEntry struct"
    - path: "services/player/internal/repo/progress.go"
      provides: "ProgressRepository.GetBulkProgress method"
      contains: "func (r *ProgressRepository) GetBulkProgress"
    - path: "services/player/internal/repo/progress_test.go"
      provides: "GetBulkProgress unit test (happy + empty + cross-user isolation)"
      contains: "TestProgressRepository_GetBulkProgress"
    - path: "services/player/internal/service/progress.go"
      provides: "ProgressService.GetBulkProgress delegate"
      contains: "func (s *ProgressService) GetBulkProgress"
    - path: "services/player/internal/handler/progress.go"
      provides: "ProgressHandler.GetBulkProgress HTTP handler (parses ?ids=, max 50, JWT-required)"
      contains: "func (h *ProgressHandler) GetBulkProgress"
    - path: "services/player/internal/transport/router.go"
      provides: "GET /users/anime-progress route inside the JWT-protected /users group"
      contains: "anime-progress"
    - path: "frontend/web/src/api/client.ts"
      provides: "userApi.getAnimeProgress(ids: string[])"
      contains: "getAnimeProgress"
    - path: "frontend/web/src/composables/useAnimeProgress.ts"
      provides: "useAnimeProgress composable (debounced bulk fetch keyed by ids ref)"
      contains: "export function useAnimeProgress"
    - path: "frontend/web/src/components/anime/AnimeCardNew.vue"
      provides: "Anime.hasDub prop + DUB badge top-right + progress badge bottom-left stacked with watchlist badge"
      contains: "dubBadge"
    - path: "frontend/web/src/views/Home.vue"
      provides: "RecItem template renders progress badge over poster + Ongoing column ?episode={N+1} link"
      contains: "?episode="
    - path: "frontend/web/src/locales/en.json"
      provides: "card.dubBadge + card.episodeProgress keys (en)"
      contains: "dubBadge"
    - path: "frontend/web/src/locales/ru.json"
      provides: "card.dubBadge + card.episodeProgress keys (ru)"
      contains: "dubBadge"
    - path: "frontend/web/src/locales/ja.json"
      provides: "card.dubBadge + card.episodeProgress keys (ja)"
      contains: "dubBadge"
  key_links:
    - from: "frontend/web/src/components/anime/AnimeCardNew.vue"
      to: "/api/users/anime-progress"
      via: "useAnimeProgress composable -> userApi.getAnimeProgress -> apiClient.get"
      pattern: "getAnimeProgress"
    - from: "services/player/internal/handler/progress.go"
      to: "services/player/internal/repo/progress.go"
      via: "ProgressService.GetBulkProgress delegate"
      pattern: "GetBulkProgress"
    - from: "services/catalog/internal/parser/kodik/client.go"
      to: "services/catalog/internal/domain/anime.go (Anime.HasDub)"
      via: "parser sets has_dub when any translation.type==\"voice\""
      pattern: "HasDub"
    - from: "frontend/web/src/views/Home.vue (Ongoing column router-link)"
      to: "Anime.vue ?episode reader (Phase 8)"
      via: "URL query string ?episode={episodes_aired+1}"
      pattern: "\\?episode="
---

# Phase 9 Plan: Per-card progress + Sub/Dub + Episode-granular row

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Three Tier-E items batched because they share the card-render surface
(`AnimeCardNew.vue` + RecItem template in `Home.vue` + Ongoing column).
Closes UX-16 (per-card progress badge), UX-17 (Ongoing row links to the
next-aired episode), UX-18 (Sub/Dub indicator badge). No new database
tables — one new column on `animes` via GORM auto-migrate, one new
bulk endpoint reusing `watch_progress` + JOIN `animes`. No new gateway
route — the existing `/users/*` JWT-protected proxy catches
`/api/users/anime-progress` automatically.

## Tasks

### Backend — Wave 1 (catalog: UX-18)

#### B1. `Anime.HasDub` field (`services/catalog/internal/domain/anime.go`)

- [ ] Add a new bool field to the `Anime` struct (after `HasVideo` at
  line 34, before `Hidden` at line 35). GORM auto-migrate adds the column
  on next service start. Pattern:
  ```go
  HasDub          bool           `gorm:"default:false;index" json:"has_dub"`
  ```
- [ ] `default:false` keeps existing rows valid without manual backfill;
  `index` is cheap because `has_dub` is a low-cardinality bool the
  frontend may filter on in a follow-up phase.
- [ ] The JSON tag uses snake_case `has_dub` to match the existing
  `has_video` convention on the same struct.

#### B2. Kodik parser sets `has_dub` (`services/catalog/internal/parser/kodik/client.go`)

- [ ] Locate the call site(s) in `services/catalog/internal/service/`
  (run `grep -rn "kodik\." services/catalog/internal/service/ | grep -i "search\|translation"` during execution) where Kodik search results
  are mapped into `domain.Anime` rows before save. The mapping logic
  may live in the catalog service rather than the parser file itself
  — but the spec calls for the *signal source* (parser) to expose a
  helper. Add a small exported helper to the kodik client package
  (this file) immediately after `GetTranslations`:
  ```go
  // ResultsHaveDub returns true when at least one Kodik search result has
  // a translation with Type == "voice" (Kodik's dub indicator). Used by
  // the catalog service when persisting an Anime row to set
  // Anime.HasDub. Phase 9 (UX-18).
  func ResultsHaveDub(results []SearchResult) bool {
      for _, r := range results {
          if r.Translation != nil && r.Translation.Type == "voice" {
              return true
          }
      }
      return false
  }
  ```
- [ ] In the catalog service mapping site (file determined at exec time
  via grep above — typical candidates: `services/catalog/internal/service/anime.go`
  or `services/catalog/internal/service/kodik.go`), find the spot where
  a Shikimori-derived `domain.Anime` is enriched with Kodik data before
  the GORM save. Set:
  ```go
  anime.HasDub = kodik.ResultsHaveDub(kodikResults)
  ```
  The flag is computed once per ingest and stored. No per-request
  recomputation — UX-18 explicitly defers backfill of existing rows to
  a follow-up; search-driven re-ingest naturally repopulates over time.
- [ ] If the catalog service does NOT currently map Kodik results into
  the `Anime` row at write time (only into a separate
  `KodikVideoSource` / translation cache), then the simplest write
  path is the existing translation-cache write — wherever
  `kodik.GetTranslations` is called, branch on the result and update
  the `animes.has_dub` column via a small GORM `.Model(&domain.Anime{}).Where("id = ?", animeID).Update("has_dub", hasDub)` call
  in the same code path. Implementer's discretion: pick the lowest-
  churn write site; the constraint is "every time we touch Kodik for
  an anime, has_dub is recomputed and saved if it changed".
- [ ] No KodikVideoSource shape change. No new ingest job. The flag is
  best-effort and lazily backfilled.

#### B3. Catalog tests + redeploy

- [ ] `cd services/catalog && go test ./...` — all existing tests still
  pass. No new unit test required for `ResultsHaveDub` itself (3-line
  helper); the dub flag is verified manually via curl smoke after
  redeploy.
- [ ] `cd services/catalog && go vet ./...` — no new findings.
- [ ] `make redeploy-catalog` — catalog service restarted; GORM
  auto-migrate adds the `has_dub` column to `animes`. Verify column
  exists:
  ```bash
  docker compose -f docker/docker-compose.yml exec -T postgres psql \
    -U postgres -d animeenigma \
    -c "\d animes" | grep has_dub
  ```
  Expected: a row `has_dub | boolean | ... default false` is shown.
- [ ] Touch one anime to verify the flag populates. Re-trigger the
  Kodik search for an anime known to have a dub (e.g. a popular RU
  release). Confirm:
  ```bash
  docker compose -f docker/docker-compose.yml exec -T postgres psql \
    -U postgres -d animeenigma \
    -c "SELECT id, name, has_dub FROM animes WHERE has_dub = true LIMIT 5;"
  ```
  Expected: at least one row returned after the search-driven re-ingest.

### Backend — Wave 2 (player: UX-16)

#### B4. `BulkAnimeProgressEntry` DTO (`services/player/internal/domain/watch.go`)

- [ ] Append a new DTO immediately after `ContinueWatchingItem` (line
  227 onward). Shape:
  ```go
  // BulkAnimeProgressEntry is the per-anime payload of GET
  // /users/anime-progress. Aggregates the user's watch state for one
  // anime so the frontend can render a single progress badge per card.
  // Phase 9 (UX-16).
  type BulkAnimeProgressEntry struct {
      LatestEpisode int  `json:"latest_episode"`  // highest episode_number with any row for this user+anime
      EpisodesCount int  `json:"episodes_count"`  // from animes.episodes_count
      EpisodesAired int  `json:"episodes_aired"`  // from animes.episodes_aired
      Completed     bool `json:"completed"`       // true when LatestEpisode is completed=true AND >= EpisodesCount (or no further aired)
      Dropped       bool `json:"dropped"`         // true when the latest row has dropped_off_at != NULL and is not completed
  }

  // BulkAnimeProgressMap is the response shape — a JSON object keyed by
  // anime_id (string) -> BulkAnimeProgressEntry. Animes the user has no
  // progress on are omitted from the map; the frontend treats absence as
  // "no badge".
  type BulkAnimeProgressMap map[string]BulkAnimeProgressEntry
  ```
- [ ] These are not GORM models — no `TableName()`, no `gorm:"primaryKey"`
  tags. The repo populates the map via `.Raw(...).Scan(...)` into a
  flat scan struct then folds into the map.

#### B5. `ProgressRepository.GetBulkProgress` (`services/player/internal/repo/progress.go`)

- [ ] Append a new method to `ProgressRepository` after
  `ListContinueWatching` (current end-of-file). Signature:
  ```go
  func (r *ProgressRepository) GetBulkProgress(
      ctx context.Context, userID string, animeIDs []string,
  ) (domain.BulkAnimeProgressMap, error)
  ```
- [ ] Empty-input guard at the top:
  ```go
  if len(animeIDs) == 0 {
      return domain.BulkAnimeProgressMap{}, nil
  }
  ```
- [ ] Single SQL statement. For each (user_id, anime_id), select the row
  with the highest `episode_number` (NOT highest `last_watched_at` —
  Continue-Watching cares about recency, per-card progress cares about
  furthest position). JOIN `animes` for `episodes_count` +
  `episodes_aired`. Use `.Raw(...).Scan(...)` (same pattern as
  ListContinueWatching). Pattern:
  ```go
  type scanRow struct {
      AnimeID       string `gorm:"column:anime_id"`
      LatestEpisode int    `gorm:"column:latest_episode"`
      Completed     bool   `gorm:"column:completed"`
      DroppedOffAt  *int   `gorm:"column:dropped_off_at"`
      EpisodesCount int    `gorm:"column:episodes_count"`
      EpisodesAired int    `gorm:"column:episodes_aired"`
  }

  sqlStr := `
      WITH ranked AS (
          SELECT
              wp.anime_id,
              wp.episode_number,
              wp.completed,
              wp.dropped_off_at,
              ROW_NUMBER() OVER (
                  PARTITION BY wp.anime_id
                  ORDER BY wp.episode_number DESC, wp.last_watched_at DESC
              ) AS rn
          FROM watch_progress wp
          WHERE wp.user_id = ?
            AND wp.anime_id IN (?)
      )
      SELECT
          r.anime_id              AS anime_id,
          r.episode_number        AS latest_episode,
          r.completed             AS completed,
          r.dropped_off_at        AS dropped_off_at,
          a.episodes_count        AS episodes_count,
          COALESCE(a.episodes_aired, 0) AS episodes_aired
      FROM ranked r
      JOIN animes a ON a.id = r.anime_id
      WHERE r.rn = 1
        AND a.deleted_at IS NULL`

  var rows []scanRow
  if err := r.db.WithContext(ctx).
      Raw(sqlStr, userID, animeIDs).
      Scan(&rows).Error; err != nil {
      return nil, err
  }
  ```
- [ ] Fold scan rows into the map:
  ```go
  out := make(domain.BulkAnimeProgressMap, len(rows))
  for _, row := range rows {
      // "Completed" semantics for the badge: the latest row is marked
      // completed AND the user has reached at least the last aired
      // episode (or full episodes_count if known). This is a stricter
      // bar than just watch_progress.completed=true because a user who
      // completed only E1 of a 12-ep show is still "in progress" for
      // badge purposes.
      reachedAll := false
      if row.EpisodesCount > 0 && row.LatestEpisode >= row.EpisodesCount {
          reachedAll = true
      } else if row.EpisodesCount == 0 && row.EpisodesAired > 0 && row.LatestEpisode >= row.EpisodesAired {
          reachedAll = true
      }
      entry := domain.BulkAnimeProgressEntry{
          LatestEpisode: row.LatestEpisode,
          EpisodesCount: row.EpisodesCount,
          EpisodesAired: row.EpisodesAired,
          Completed:     row.Completed && reachedAll,
          Dropped:       row.DroppedOffAt != nil && !row.Completed,
      }
      out[row.AnimeID] = entry
  }
  return out, nil
  ```
- [ ] Notes:
  - The `IN (?)` clause requires GORM to expand the slice — pass
    `animeIDs` directly; GORM expands it via `Raw(...)` placeholder
    substitution. Validated by the existing `WHERE id IN (?)` pattern
    elsewhere in this codebase (see `services/player/internal/repo/list.go`).
  - The `a.deleted_at IS NULL` filter matches the soft-delete posture
    in `ListContinueWatching`. Orphaned watch_progress rows whose
    anime row was soft-deleted are silently dropped (consistent with
    the rest of the codebase).
  - `ROW_NUMBER() OVER (PARTITION BY anime_id ORDER BY episode_number
    DESC, last_watched_at DESC)` picks the furthest episode the user
    has touched, breaking ties on recency. This is the right shape
    for the "Серия N / Y" badge.

#### B6. `ProgressService.GetBulkProgress` (`services/player/internal/service/progress.go`)

- [ ] Append a thin delegate method after `ListContinueWatching` (current
  end-of-file):
  ```go
  // GetBulkProgress returns a map keyed by anime_id with the user's
  // furthest episode reached + completion flags. Used by AnimeCardNew
  // to render a per-card progress badge. Phase 9 (UX-16).
  func (s *ProgressService) GetBulkProgress(
      ctx context.Context, userID string, animeIDs []string,
  ) (domain.BulkAnimeProgressMap, error) {
      return s.progressRepo.GetBulkProgress(ctx, userID, animeIDs)
  }
  ```
- [ ] No preference-service interaction — pure read-through.

#### B7. `ProgressHandler.GetBulkProgress` (`services/player/internal/handler/progress.go`)

- [ ] Append a new handler method after `ListContinueWatching` (current
  end-of-file):
  ```go
  // GetBulkProgress returns the bulk per-anime progress map for the
  // authenticated user, scoped to the comma-separated `ids` query
  // param. Caps at 50 IDs per request — the AnimeCardNew composable
  // batches per visible grid page. Phase 9 (UX-16).
  func (h *ProgressHandler) GetBulkProgress(w http.ResponseWriter, r *http.Request) {
      const maxIDs = 50

      claims, ok := authz.ClaimsFromContext(r.Context())
      if !ok || claims == nil {
          httputil.Unauthorized(w)
          return
      }

      raw := r.URL.Query().Get("ids")
      if raw == "" {
          httputil.OK(w, domain.BulkAnimeProgressMap{})
          return
      }
      parts := strings.Split(raw, ",")
      ids := make([]string, 0, len(parts))
      for _, p := range parts {
          p = strings.TrimSpace(p)
          if p == "" {
              continue
          }
          ids = append(ids, p)
      }
      if len(ids) == 0 {
          httputil.OK(w, domain.BulkAnimeProgressMap{})
          return
      }
      if len(ids) > maxIDs {
          httputil.BadRequest(w, "ids must contain at most 50 entries")
          return
      }

      out, err := h.progressService.GetBulkProgress(r.Context(), claims.UserID, ids)
      if err != nil {
          h.log.Errorw("failed to bulk-load anime progress",
              "user_id", claims.UserID, "count", len(ids), "error", err)
          httputil.Error(w, err)
          return
      }
      if out == nil {
          out = domain.BulkAnimeProgressMap{}
      }
      httputil.OK(w, out)
  }
  ```
- [ ] Add the `"strings"` import to the existing import block if not
  already present.

#### B8. Route registration (`services/player/internal/transport/router.go`)

- [ ] Inside the existing JWT-protected `r.Route("/users", ...)` block,
  add the new route immediately after the existing `/continue-watching`
  route (line 83):
  ```go
  // Bulk per-card anime-progress (Phase 9 / UX-16). Comma-separated
  // ?ids=a,b,c (max 50). Returns a JSON object keyed by anime_id with
  // the user's furthest episode reached + completion flags. The
  // AnimeCardNew composable batches per visible grid page.
  r.Get("/anime-progress", progressHandler.GetBulkProgress)
  ```
- [ ] No gateway change needed. The existing `/users/*` JWT-protected
  proxy in `services/gateway/internal/transport/router.go` catches
  this path the same way it catches `/users/continue-watching`.

#### B9. Unit test (`services/player/internal/repo/progress_test.go`)

- [ ] Append a new test function reusing the existing
  `setupProgressTestDB` helper. Pattern:
  ```go
  // TestProgressRepository_GetBulkProgress covers the happy path
  // (mixed in-progress + completed across multiple anime, one row per
  // anime returned with the highest episode_number), empty IDs slice,
  // and cross-user isolation. Phase 9 (UX-16).
  func TestProgressRepository_GetBulkProgress(t *testing.T) {
      r, db := setupProgressTestDB(t)
      ctx := context.Background()

      // Create the animes table the JOIN needs.
      err := db.Exec(`CREATE TABLE animes (
          id TEXT PRIMARY KEY,
          name TEXT,
          episodes_count INTEGER DEFAULT 0,
          episodes_aired INTEGER DEFAULT 0,
          deleted_at DATETIME
      )`).Error
      require.NoError(t, err)

      require.NoError(t, db.Exec(
          `INSERT INTO animes (id, name, episodes_count, episodes_aired) VALUES (?, ?, ?, ?)`,
          "anime-A", "Anime A", 12, 12).Error)
      require.NoError(t, db.Exec(
          `INSERT INTO animes (id, name, episodes_count, episodes_aired) VALUES (?, ?, ?, ?)`,
          "anime-B", "Anime B", 24, 12).Error)
      require.NoError(t, db.Exec(
          `INSERT INTO animes (id, name, episodes_count, episodes_aired) VALUES (?, ?, ?, ?)`,
          "anime-C", "Anime C", 12, 12).Error)

      // Anime A: E1 in progress, E3 completed. Latest = E3 (highest
      // episode_number, completed=true).
      seedProgressRow(t, db, "user-1", "anime-A", 1, 600, 1400, false)
      seedProgressRow(t, db, "user-1", "anime-A", 3, 1400, 1400, true)

      // Anime B: E5 in progress only. Latest = E5.
      seedProgressRow(t, db, "user-1", "anime-B", 5, 800, 1500, false)

      // Anime C: every episode completed (E1..E12). Latest = E12,
      // completed=true, reached EpisodesCount.
      for ep := 1; ep <= 12; ep++ {
          seedProgressRow(t, db, "user-1", "anime-C", ep, 1400, 1400, true)
      }

      // Different user — must NOT leak into user-1's map.
      seedProgressRow(t, db, "user-2", "anime-A", 9, 100, 1400, false)

      // --- Happy path ---
      m, err := r.GetBulkProgress(ctx, "user-1",
          []string{"anime-A", "anime-B", "anime-C", "anime-missing"})
      require.NoError(t, err)
      require.Len(t, m, 3, "missing anime omitted from map")

      entryA, ok := m["anime-A"]
      require.True(t, ok)
      assert.Equal(t, 3, entryA.LatestEpisode, "highest episode_number")
      assert.False(t, entryA.Completed, "E3 of 12 — completed flag true on row but reachedAll false")

      entryB, ok := m["anime-B"]
      require.True(t, ok)
      assert.Equal(t, 5, entryB.LatestEpisode)
      assert.False(t, entryB.Completed)

      entryC, ok := m["anime-C"]
      require.True(t, ok)
      assert.Equal(t, 12, entryC.LatestEpisode)
      assert.True(t, entryC.Completed, "E12 of 12 with completed=true → reachedAll")

      // --- Empty IDs slice ---
      empty, err := r.GetBulkProgress(ctx, "user-1", []string{})
      require.NoError(t, err)
      assert.Empty(t, empty)

      // --- Cross-user isolation ---
      otherUser, err := r.GetBulkProgress(ctx, "user-no-rows",
          []string{"anime-A", "anime-B", "anime-C"})
      require.NoError(t, err)
      assert.Empty(t, otherUser)
  }
  ```
- [ ] Run from repo root to confirm: `cd services/player && go test ./internal/repo -run TestProgressRepository_GetBulkProgress -v`.

#### B10. Backend Wave 2 verification gate

- [ ] `cd services/player && go test ./...` — all existing tests still
  pass AND the new `TestProgressRepository_GetBulkProgress` passes.
- [ ] `cd services/player && go vet ./...` — no new findings.
- [ ] `make redeploy-player` — player service rebuilt and the new
  endpoint is live.
- [ ] Smoke against production (`project_deployment.md` says this
  server IS production). Pick 3 real anime IDs the `ui_audit_bot`
  user has progress on — first query the DB to get them:
  ```bash
  docker compose -f docker/docker-compose.yml exec -T postgres psql \
    -U postgres -d animeenigma -tA \
    -c "SELECT DISTINCT anime_id FROM watch_progress wp
        JOIN users u ON u.id = wp.user_id
        WHERE u.username = 'ui_audit_bot' LIMIT 3;"
  ```
  Then:
  ```bash
  source docker/.env
  IDS=$(echo "$(...IDs above as a,b,c...)")
  curl -s -H "Authorization: Bearer $UI_AUDIT_API_KEY" \
    "http://localhost:8000/api/users/anime-progress?ids=$IDS" | jq
  ```
  Expected: JSON object keyed by anime_id with `latest_episode`,
  `episodes_count`, `episodes_aired`, `completed`, `dropped` populated.
- [ ] Smoke unauthenticated:
  `curl -s http://localhost:8000/api/users/anime-progress?ids=foo`
  returns HTTP 401.
- [ ] Smoke oversize: 51 comma-separated IDs returns HTTP 400 with body
  `"ids must contain at most 50 entries"`.

### Frontend — Wave 3 (UX-16 + UX-17 + UX-18)

#### F1. API client extension (`frontend/web/src/api/client.ts`)

- [ ] Add a method to the existing `userApi` object immediately after
  `getContinueWatching` (around line 252):
  ```typescript
  getAnimeProgress: (ids: string[]) =>
    apiClient.get('/users/anime-progress', {
      params: { ids: ids.join(',') },
    }),
  ```
- [ ] No top-level export change — `userApi` is already imported by
  composables.

#### F2. New composable (`frontend/web/src/composables/useAnimeProgress.ts`)

- [ ] Create the file. Same auth-gating posture as `useContinueWatching`,
  but keyed by a reactive `ids` ref and debounced so a rapidly-
  changing grid (e.g. user scrolling through paginated Browse) does
  not fire per-card requests. Full contents:
  ```typescript
  import { ref, watch, type Ref } from 'vue'
  import { userApi } from '@/api/client'
  import { useAuthStore } from '@/stores/auth'

  // Phase 9 (UX-16): bulk per-card anime-progress fetch. Anonymous users
  // skip the fetch entirely — the endpoint is JWT-protected and would
  // return 401. The composable batches all IDs passed in `ids` into a
  // single network call and exposes a Map<animeId, ProgressEntry> the
  // card can look up by ID.
  //
  // Backend contract (services/player/internal/handler/progress.go):
  //   GET /api/users/anime-progress?ids=a,b,c
  //   -> { success, data: { [animeId]: ProgressEntry } }

  export interface ProgressEntry {
    latest_episode: number
    episodes_count: number
    episodes_aired: number
    completed: boolean
    dropped: boolean
  }

  export function useAnimeProgress(ids: Ref<string[]>, debounceMs = 200) {
    const progressMap = ref<Map<string, ProgressEntry>>(new Map())
    const loading = ref(false)
    const error = ref<string | null>(null)
    const auth = useAuthStore()

    let debounceHandle: ReturnType<typeof setTimeout> | null = null

    async function fetchProgress(currentIds: string[]) {
      if (!auth.token || currentIds.length === 0) {
        progressMap.value = new Map()
        return
      }
      // Cap at 50 to match backend; if caller passes more we slice.
      const trimmed = currentIds.slice(0, 50)
      loading.value = true
      error.value = null
      try {
        const res = await userApi.getAnimeProgress(trimmed)
        const data = (res.data?.data ?? res.data) as Record<string, ProgressEntry>
        const next = new Map<string, ProgressEntry>()
        if (data && typeof data === 'object') {
          for (const [k, v] of Object.entries(data)) {
            next.set(k, v)
          }
        }
        progressMap.value = next
      } catch (e) {
        error.value = e instanceof Error ? e.message : 'failed to load anime-progress'
        progressMap.value = new Map()
      } finally {
        loading.value = false
      }
    }

    // Debounced watcher — re-fetch when the ids list changes or the
    // auth token transitions. Skipping the initial empty-ids case
    // avoids a wasted 0-length request on mount.
    watch(
      [ids, () => auth.token],
      ([newIds]) => {
        if (debounceHandle) clearTimeout(debounceHandle)
        const snapshot = [...newIds]
        debounceHandle = setTimeout(() => fetchProgress(snapshot), debounceMs)
      },
      { immediate: true },
    )

    return { progressMap, loading, error, refresh: () => fetchProgress(ids.value) }
  }
  ```

#### F3. AnimeCardNew — DUB badge + progress badge (`frontend/web/src/components/anime/AnimeCardNew.vue`)

- [ ] Extend the `Anime` interface (currently at line 117-131) with
  the new optional fields:
  ```typescript
  interface Anime {
    id: string | number
    title: string
    name?: string
    nameRu?: string
    nameJp?: string
    coverImage: string
    rating?: number
    releaseYear?: number
    episodes?: number
    status?: string
    genres?: string[]
    rawGenres?: { name?: string; nameRu?: string }[]
    quality?: string
    hasDub?: boolean         // UX-18 (Phase 9)
  }
  ```
- [ ] Extend `defineProps` to accept an optional `progress` entry that
  the parent (Browse/Search/Home) wires up via `useAnimeProgress`:
  ```typescript
  const props = defineProps<{
    anime: Anime
    listStatus?: string | null
    siteRating?: { average_score: number; total_reviews: number } | null
    menuOpen?: boolean
    // Phase 9 (UX-16): optional per-card progress entry. When supplied
    // the card renders a progress badge in the bottom-left, stacked
    // above the watchlist-status badge.
    progress?: {
      latest_episode: number
      episodes_count: number
      episodes_aired: number
      completed: boolean
      dropped: boolean
    } | null
  }>()
  ```
- [ ] Add the DUB badge inside the top-badges container. Currently
  line 33-38 renders the quality badge on the left and the rating
  stack on the right. Replace the left-corner block (line 34-38) with
  a left-column flex stack that renders quality on top and DUB
  beneath it (DUB appears in the top-left area, top-right is reserved
  for the rating stack):
  ```vue
  <!-- Top-left column: Quality + DUB badges -->
  <div class="flex flex-col items-start gap-1">
    <Badge v-if="anime.quality" variant="default" size="sm">
      {{ anime.quality }}
    </Badge>
    <span
      v-if="anime.hasDub"
      class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-bold rounded bg-amber-500/90 text-white"
    >
      {{ $t('card.dubBadge') }}
    </span>
  </div>
  ```
- [ ] Replace the bottom-left watchlist-status block (currently line
  71-75) with a vertical stack that pairs the watchlist badge with
  the new progress badge below it:
  ```vue
  <!-- Bottom-left: watchlist status + progress (Phase 9 / UX-16) -->
  <div class="absolute bottom-2 left-2 flex flex-col gap-1 items-start">
    <span v-if="listStatus" :class="listBadgeClasses">
      {{ listStatusLabel }}
    </span>
    <span
      v-if="progressBadgeText"
      class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-medium rounded bg-purple-500/80 text-white"
    >
      {{ progressBadgeText }}
    </span>
  </div>
  ```
- [ ] Compute `progressBadgeText` in `<script setup>` near the other
  computeds (after `listBadgeClasses`):
  ```typescript
  // Phase 9 (UX-16): renders "Серия N" / "Episode N" / "第N話" when in
  // progress; "N / Y" when completed-but-more-aired; hidden when
  // fully complete or no progress.
  const progressBadgeText = computed(() => {
    const p = props.progress
    if (!p) return ''
    // Fully complete and caught up — no badge needed (the watchlist
    // "completed" status badge already signals this).
    if (p.completed) return ''
    // In progress — show the *current* episode (latest_episode is the
    // furthest reached; the next-to-watch is latest_episode + 1 when
    // the latest row is itself completed; otherwise it's
    // latest_episode itself). For badge purposes we show the latest
    // reached episode number — matches how Crunchyroll surfaces it.
    if (p.latest_episode > 0) {
      return t('card.episodeProgress', {
        n: p.latest_episode,
        total: p.episodes_count || p.episodes_aired || '?',
      })
    }
    return ''
  })
  ```

#### F4. Home.vue — RecItem progress badge + Ongoing ?episode link (UX-16 + UX-17)

- [ ] **UX-17 (one-line URL change).** Replace the bare
  `/anime/${anime.id}` `:to` binding on the Ongoing column router-link
  (currently line 150) with the episode-aware variant. The Ongoing
  column iterates over `ongoingAnime`; each entry already exposes
  `next_episode_at` + `episodes_aired` (used at line 190+194). Replace
  the existing `:to="`/anime/${anime.id}`"` with:
  ```vue
  :to="anime.next_episode_at && anime.episodes_aired
    ? `/anime/${anime.id}?episode=${(anime.episodes_aired || 0) + 1}`
    : `/anime/${anime.id}`"
  ```
  Phase 8 wired the `?episode={N}` reader in Anime.vue, so this URL
  directly drives episode auto-load.
- [ ] **UX-16 on the trending row (RecItem).** The trending row is
  rendered inline in Home.vue (line 57-93). Add a progress lookup
  using `useAnimeProgress` keyed by the visible `trendingRecs`
  anime IDs:

  In `<script setup lang="ts">`, after the existing imports + `const trendingRecs = ...`:
  ```typescript
  import { useAnimeProgress } from '@/composables/useAnimeProgress'

  // Phase 9 (UX-16): bulk fetch per-card progress for the trending row.
  // The composable auto-skips when anonymous (no token).
  const trendingIds = computed(() => trendingRecs.value.map((r) => String(r.anime.id)))
  const { progressMap: trendingProgress } = useAnimeProgress(trendingIds)
  ```
- [ ] In the trending-row template, add a progress badge over each
  poster. Inside the `<router-link>` poster `<div>` (lines 64-89),
  immediately before the closing `</div>` of the poster wrapper (line
  89), append:
  ```vue
  <!-- Phase 9 (UX-16): per-card progress badge -->
  <span
    v-if="trendingProgress.get(String(item.anime.id))?.latest_episode > 0
      && !trendingProgress.get(String(item.anime.id))?.completed"
    class="absolute bottom-2 left-2 inline-flex items-center px-1.5 py-0.5 text-[10px] font-medium rounded bg-purple-500/80 text-white"
  >
    {{ t('card.episodeProgress', {
      n: trendingProgress.get(String(item.anime.id))!.latest_episode,
      total: trendingProgress.get(String(item.anime.id))!.episodes_count
        || trendingProgress.get(String(item.anime.id))!.episodes_aired
        || '?',
    }) }}
  </span>
  ```
- [ ] No change to `AnimeKebab` wiring; no change to the existing
  pinned-badge / score-badge logic.

#### F5. Browse + Search wiring (out-of-scope reminder)

- [ ] AnimeCardNew is also rendered by Browse.vue + Search results.
  Wiring `useAnimeProgress` into those views requires gathering the
  visible-page anime IDs and passing each card's entry via the new
  `progress` prop. **For this phase, AnimeCardNew accepts the prop
  and renders the badge but Browse.vue + Search.vue wiring is
  optional polish** — the minimum bar is that Home's trending row
  shows the badge (most-trafficked card grid for a logged-in user).
  If Browse/Search wiring is genuinely a single-paragraph edit at
  exec time (one `useAnimeProgress` call + one `:progress="..."` on
  the card), the implementer SHOULD include it; if it cascades into
  multi-file plumbing the implementer SHOULD defer to Phase 10.
- [ ] Whichever direction the implementer takes, document the
  decision in 09-SUMMARY.md.

#### F6. i18n keys (en / ru / ja)

- [ ] `frontend/web/src/locales/en.json` — add a new top-level `card`
  namespace (currently no such namespace exists; the file uses
  per-section namespaces like `home`, `recs`, `anime`). Insert after
  the existing `recs` block (around line 50):
  ```json
  "card": {
    "dubBadge": "DUB",
    "episodeProgress": "Episode {n} / {total}"
  },
  ```
- [ ] `frontend/web/src/locales/ru.json` — same shape, Russian copy:
  ```json
  "card": {
    "dubBadge": "DUB",
    "episodeProgress": "Серия {n} / {total}"
  },
  ```
- [ ] `frontend/web/src/locales/ja.json` — same shape, Japanese copy:
  ```json
  "card": {
    "dubBadge": "DUB",
    "episodeProgress": "第{n}話 / {total}"
  },
  ```
- [ ] **`dubBadge` is locked to "DUB" across all three locales** per
  CONTEXT.md (anime fans recognize this universally; localizing the
  string would create three different identifiers for the same
  concept). The key is duplicated across locales for consistency and
  to give translators a future override point if needed.
- [ ] Adjust trailing commas on the line preceding each insertion so
  the JSON remains valid in all three files.

### Frontend verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` — passes; the new
  composable, the AnimeCardNew prop extension, and the Home.vue
  `useAnimeProgress` call all type-check cleanly.
- [ ] `cd frontend/web && bunx eslint src/composables/useAnimeProgress.ts src/components/anime/AnimeCardNew.vue src/views/Home.vue src/api/client.ts` — zero errors / zero warnings.
- [ ] JSON validity:
  ```bash
  cd frontend/web && bun -e "JSON.parse(require('fs').readFileSync('src/locales/en.json','utf8')); JSON.parse(require('fs').readFileSync('src/locales/ru.json','utf8')); JSON.parse(require('fs').readFileSync('src/locales/ja.json','utf8')); console.log('ok')"
  ```
  prints `ok`.
- [ ] `make redeploy-web` — frontend rebuilt and shipped.
- [ ] Manual smoke as `ui_audit_bot` against `https://animeenigma.ru/`:
  1. Inject the JWT into `localStorage.token` per the audit framework
     pattern, reload `/`.
  2. The trending row renders. At least one card whose anime the user
     has progress on shows a small purple progress badge in the
     bottom-left over the poster.
  3. The Ongoing column's first entry, if it has `next_episode_at`,
     navigates to `/anime/{id}?episode={N+1}` when clicked. The
     player loads on the expected episode (Phase 8 reader).
  4. Cards backed by an anime with `has_dub=true` (verifiable via the
     DB query in B3) show a small amber `DUB` badge in the top-left
     beneath the quality badge.
  5. Switch UI locale en → ru → ja. The DUB badge stays "DUB" in all
     three. The progress badge text swaps: "Episode N / Y" → "Серия
     N / Y" → "第N話 / Y".
  6. Log out. Reload `/`. No anime-progress network request fires
     (verify in DevTools Network panel — no `/api/users/anime-progress`
     entry). Cards render without progress badges. DUB badges still
     render where applicable (DUB comes from the public anime row,
     not from user state).
- [ ] Chrome MCP axe-core re-run on `https://animeenigma.ru/`
  (logged-in as `ui_audit_bot`):
  - Zero new violations on Home — the new badges are decorative
    spans with sufficient contrast (purple-500/80 + white text =
    AA, amber-500/90 + white text = AA on the dark poster).
  - Specifically: `color-contrast`, `image-alt`, `link-name` stay
    clean.

## Files touched

```
services/catalog/internal/domain/anime.go                         (+ Anime.HasDub field)
services/catalog/internal/parser/kodik/client.go                  (+ ResultsHaveDub helper)
services/catalog/internal/service/<kodik-mapping-site>.go         (+ anime.HasDub = kodik.ResultsHaveDub(...) at write site)
services/player/internal/domain/watch.go                          (+ BulkAnimeProgressEntry + BulkAnimeProgressMap DTOs)
services/player/internal/repo/progress.go                         (+ GetBulkProgress method)
services/player/internal/repo/progress_test.go                    (+ TestProgressRepository_GetBulkProgress)
services/player/internal/service/progress.go                      (+ GetBulkProgress delegate)
services/player/internal/handler/progress.go                      (+ GetBulkProgress handler + "strings" import)
services/player/internal/transport/router.go                      (+ GET /users/anime-progress route)
frontend/web/src/api/client.ts                                    (+ userApi.getAnimeProgress)
frontend/web/src/composables/useAnimeProgress.ts                  (NEW)
frontend/web/src/components/anime/AnimeCardNew.vue                (+ hasDub on Anime, + progress prop, + DUB badge top-left, + progress badge bottom-left)
frontend/web/src/views/Home.vue                                   (+ useAnimeProgress for trending row, + progress badge in RecItem template, + ?episode={N+1} on Ongoing router-link)
frontend/web/src/locales/en.json                                  (+ card.dubBadge + card.episodeProgress)
frontend/web/src/locales/ru.json                                  (+ same keys, RU copy)
frontend/web/src/locales/ja.json                                  (+ same keys, JA copy)
.planning/workstreams/ui-ux-audit/phases/09-per-card-progress-subdub/
  09-CONTEXT.md                                                   (already exists)
  09-PLAN.md                                                      (this file)
  09-SUMMARY.md                                                   (written at execute-phase end)
  09-VERIFICATION.md                                              (written at execute-phase end)
```

No new database tables. No new gateway routes. No new external
libraries. GORM auto-migrate adds the one new `animes.has_dub`
column on next catalog start.

## Closes

| Requirement | Surface | Mechanism |
|---|---|---|
| UX-16 | All cards (AnimeCardNew + RecItem trending row) | Bulk endpoint `GET /api/users/anime-progress?ids=...` (max 50) + `useAnimeProgress` composable + purple progress badge stacked beneath the watchlist-status badge in the bottom-left |
| UX-17 | Home Ongoing column | One-line `:to` binding change: `/anime/{id}?episode={episodes_aired+1}` when `next_episode_at` is set, hitting the Phase 8 episode reader in Anime.vue |
| UX-18 | All cards (AnimeCardNew) | `Anime.HasDub` bool field on the catalog domain model + Kodik parser sets it when any translation has `type=="voice"` + amber DUB badge top-left beneath the quality badge |

## Wave outline

| Wave | Tasks | Rationale |
|---|---|---|
| 1 (Catalog) | B1 Anime.HasDub field → B2 Kodik parser helper + write-site wiring → B3 go test + redeploy-catalog (column verified via psql + has_dub=true row verified) | Catalog ships independently. The new column lives in the shared postgres DB but the player service does NOT need to know about it — the frontend reads `has_dub` straight off the existing anime API surface. |
| 2 (Player) | B4 DTOs → B5 repo + SQL → B6 service → B7 handler → B8 route → B9 unit test → B10 backend verification gate (go test, redeploy-player, curl smoke against real IDs + 401 + 400 oversize) | Endpoint ships + verifies as a single atomic unit before any frontend code is written. |
| 3 (Frontend + i18n) | F1 api client → F2 composable → F3 AnimeCardNew prop + DUB badge + progress badge → F4 Home.vue trending-row wiring + Ongoing ?episode link → F5 Browse/Search optional polish → F6 i18n keys (en/ru/ja) → frontend verification (vue-tsc, eslint, redeploy-web, manual smoke + axe re-run) | Frontend depends on the verified Wave 1 column + Wave 2 endpoint. i18n keys ship in the same wave as the component changes that reference them — adding them later would break the build. |
