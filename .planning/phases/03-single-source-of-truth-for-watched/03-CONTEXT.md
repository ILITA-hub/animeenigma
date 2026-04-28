# Phase 3: Single Source of Truth for "Watched" - Context

**Gathered:** 2026-04-28
**Status:** Ready for execution (Wave 1 with Phase 2)

<domain>
## Phase Boundary

Make `watch_progress.completed = true` the canonical signal for "user has finished
this episode" across all entry points (auto-mark at 20 min, manual mark button)
and across all 4 players. Eliminate the disagreement between
`watch_progress.completed` (currently 0/385 rows true) and `anime_list.episodes`
(maintained, accurate per-user, but a denorm). Episode-list checkmarks in the UI
must read from a consistent source across all 4 players.

**Strictly in scope:**
- Backend: split `ProgressRepository` upsert so `UpdateProgress` (heartbeat) does NOT touch `completed`, and add `MarkCompleted` for the discrete completion event called from `MarkEpisodeWatched`.
- Backend: `MarkEpisodeWatched` (called by both auto-mark and manual mark paths) writes BOTH `anime_list.episodes++` (existing) AND `watch_progress.completed=true` (new) for the marked episode.
- Backend: idempotent backfill on startup that creates `watch_progress` rows with `completed=true` for every (user, anime, ep) where `ep <= anime_list.episodes` and the row is missing/false.
- Frontend: fix `AnimeLibPlayer.vue` and `ConsumetPlayer.vue` to read `entry?.episodes` (matches Kodik / HiAnime) instead of the non-existent `entry?.episodes_watched`.
- Tests: repo tests for `UpsertProgress` (preserves existing `completed=true`) and `MarkCompleted` (idempotent set-to-true); service test confirming `MarkEpisodeWatched` triggers `MarkCompleted`.

**Strictly out of scope:**
- Reading `watch_progress.completed` server-side for resume CTAs (Phase 7 wires `GET /progress/{animeId}` into the resume flow; Phase 3 only ensures the data is correct on the server).
- Per-episode rewatch detection / `watch_count` column (Phase 5 G-02).
- Removing `anime_list.episodes` as a stored field (kept as a maintained denorm; "either read or recomputed" wording in ROADMAP SC-3 lets us keep it).
- The pre-player state machine (Phase 4 — A-03, A-04).
- Cross-device freshness / `prefs_version` / cache invalidation (Phase 7 — D-03).

</domain>

<decisions>
## Implementation Decisions

### Repository split

- **D-01:** `ProgressRepository.Upsert` is split into TWO methods:
  - `UpsertProgress(ctx, *WatchProgress)` — for heartbeat saves. Writes `progress`, `duration`, `last_watched_at`, `updated_at`. NEVER touches `completed`. Replaces today's `Upsert` (which always overwrites `completed=false`).
  - `MarkCompleted(ctx, userID, animeID, episodeNumber)` — discrete completion event. Writes `completed=true`, plus `last_watched_at`/`updated_at` bookkeeping. Idempotent (`ON CONFLICT DO UPDATE SET completed=true`). Creates the row if missing.
- **D-02:** Why split? Today the upsert in `repo/progress.go:34` always assigns `completed: progress.Completed`, and `service/progress.go:34` hardcodes `Completed: false`. Result: every heartbeat resets `completed` to false, so any future "completed=true" row would be flipped back. The split makes `completed=true` sticky.

### Service-layer dual write

- **D-03:** `service/list.go MarkEpisodeWatched` is the canonical entry point for both auto-mark (20-min, after `KodikPlayer.vue:643` etc.) AND manual mark (button click). It already increments `anime_list.episodes` via `IncrementEpisodes`. Phase 3 adds: after `IncrementEpisodes` succeeds (or returns no-op for already-marked episodes), unconditionally call `progressRepo.MarkCompleted(ctx, userID, animeID, req.Episode)`. The `MarkCompleted` call is idempotent — safe to retry, safe to call when already true.
- **D-04:** The two writes (`anime_list.episodes++` and `watch_progress.completed=true`) are NOT wrapped in a single transaction. Rationale: `MarkEpisodeWatched` already has best-effort writes (it logs `WatchHistory` failures via `s.log.Errorw` rather than rolling back). Phase 3 follows the existing pattern: log a `MarkCompleted` failure but do not fail the request. The downside is a partial-write window; this is acceptable because the next mark on the same episode will recover (idempotent `MarkCompleted`), and the worst case (no `watch_progress` row) means Phase 4's state machine falls back to `anime_list.episodes`.
- **D-05:** `service/progress.go UpdateProgress` removes the `Completed: false` line. The struct's zero value is false anyway, but the new `UpsertProgress` doesn't read it. Removing the explicit assignment removes the "User marks manually" comment that contradicts the auto-mark reality.

### Backfill

- **D-06:** Backfill is one-shot SQL run after `AutoMigrate` in `cmd/player-api/main.go`, idempotent across restarts. The SQL:
  ```sql
  INSERT INTO watch_progress (id, user_id, anime_id, episode_number, progress, duration, completed, last_watched_at, created_at, updated_at)
  SELECT gen_random_uuid(), al.user_id, al.anime_id, ep, 0, 0, true,
         COALESCE(al.completed_at, al.updated_at), NOW(), NOW()
  FROM anime_list al
  CROSS JOIN LATERAL generate_series(1, al.episodes) ep
  WHERE al.episodes > 0
  ON CONFLICT (user_id, anime_id, episode_number) DO UPDATE
  SET completed = true, updated_at = NOW()
  WHERE watch_progress.completed = false;
  ```
- **D-07:** Backfill rows have `progress=0, duration=0`. This is honest — we don't know the original session lengths for legacy data. Phase 5 G-02 (rewatch detection via `watch_count` increment on transition false→true) and Phase 6 (Tier 2 weighting via `watch_history.duration_watched`) read different sources, so the synthetic `progress=0` does not poison those phases.
- **D-08:** Why backfill instead of "consciously accepted gap"? Phase 4's state machine (A-03) reads "what episode is the user resuming on" — if `watch_progress.completed` is empty for a (user, anime), it would think the user has watched nothing and start ep 1, contradicting `anime_list.episodes`. Backfilling avoids a permanent fallback path through Phase 4/7 code.
- **D-09:** Backfill performance: ~2569 rows × avg ~12 episodes ≈ ~30k inserts, single transaction, expected < 5s on first deploy. Subsequent restarts: scan finds all rows already exist, the `ON CONFLICT DO UPDATE WHERE completed=false` is a no-op for already-completed rows. Cost on warm restart: a single `INSERT … ON CONFLICT` plan that scans `anime_list` × generate_series; gated by an early-exit check before the INSERT (see D-10).
- **D-10:** Early-exit guard. Before running the backfill SQL, check `SELECT 1 FROM watch_progress WHERE completed = true LIMIT 1`. If a completed row already exists, skip the backfill entirely. After first deploy, this short-circuits in microseconds. We do NOT use a `migration_log` table — overkill for one-shot backfill.

### Frontend checkmark consistency

- **D-11:** Checkmark source for all 4 players: read `entry?.episodes` from `getWatchlistEntry` response (which returns `AnimeListEntry` with the `episodes` field). Today: Kodik (`KodikPlayer.vue:606`) and HiAnime (`HiAnimePlayer.vue:1249`) read `entry?.episodes` correctly. AnimeLib (`AnimeLibPlayer.vue:763`) and Consumet (`ConsumetPlayer.vue:1224`) read `entry?.episodes_watched`, a non-existent field — currently always falsy → checkmarks never render. Fix: change to `entry?.episodes`.
- **D-12:** Why use `anime_list.episodes` (a denorm) instead of a fresh server query against `watch_progress.completed`? `anime_list.episodes` is already loaded by the existing `getWatchlistEntry` call, no new endpoint or extra round-trip. Phase 3's invariant is that `anime_list.episodes == COUNT(watch_progress.completed=true)` going forward, so reading either is equivalent. Phase 7 may reconsider if cross-device staleness pushes us to fetch from the server endpoint.

### Verification

- **D-13:** After Wave 1 deploy, verify on production using `ui_audit_bot`'s seeded data:
  1. Mark episode 6 of an existing anime as watched via the manual button on each of the 4 players.
  2. Confirm `watch_progress` shows `completed=true` for `(ui_audit_bot.user_id, that anime_id, ep 6)`.
  3. Confirm `anime_list.episodes >= 6` for that entry.
  4. Reload the player; confirm episode-list checkmarks render for episodes 1-6 in all 4 players.
  5. Confirm watchlist counter on Profile shows the same number.
- **D-14:** Live verification is BLOCKED until Wave 1 batch deploy lands (per user's locked deploy posture). Until deploy, verification is restricted to unit/integration tests against an in-memory SQLite (the existing pattern at `services/player/internal/repo/sync_test.go:15`).

### Out-of-scope items surfaced during scoping (deferred)

- **D-15:** A `watch_progress.completed_at` timestamp column was considered to record WHEN an episode was completed. Deferred — `last_watched_at` already serves this for completion writes (the same call sets it). If a future phase needs to distinguish "last activity" vs "completion time", that's a Phase 5/8 schema-add.
- **D-16:** Per-episode-list aggregate query (e.g., `GET /users/list/:animeId/episodes-watched` returning array of episode numbers) was considered. Deferred — current `entry?.episodes` (count) is sufficient for "checkmark all eps ≤ N" UI semantics. A more granular fetch becomes valuable when episode-by-episode tracking matters (Phase 4 state machine may revisit).

</decisions>

<canonical_refs>
## Canonical References

**Downstream (Phase 4, 5, 6, 7) reads / mutations of these contracts.**

### Files modified by this phase
- `services/player/internal/repo/progress.go` — split `Upsert` → `UpsertProgress` + `MarkCompleted`
- `services/player/internal/service/progress.go` — call `UpsertProgress`; remove `Completed: false`
- `services/player/internal/service/list.go` — call `MarkCompleted` after `IncrementEpisodes`
- `services/player/cmd/player-api/main.go` — add backfill SQL + early-exit guard after `AutoMigrate`
- `frontend/web/src/components/player/AnimeLibPlayer.vue:763` — `episodes_watched` → `episodes`
- `frontend/web/src/components/player/ConsumetPlayer.vue:1224` — `episodes_watched` → `episodes`

### Files NOT modified (read-only references)
- `services/player/internal/domain/watch.go` — schema unchanged (no new columns, no removed columns)
- `services/player/internal/repo/list.go IncrementEpisodes` — unchanged
- `services/player/internal/handler/list.go MarkEpisodeWatched` — unchanged (handler is thin)
- `frontend/web/src/components/player/KodikPlayer.vue` — already reads `entry?.episodes` correctly
- `frontend/web/src/components/player/HiAnimePlayer.vue` — already reads `entry?.episodes` correctly

### Project planning
- `.planning/PROJECT.md` — A-01, A-02, D-02 active requirements
- `.planning/REQUIREMENTS.md` — close A-01, A-02, D-02 on completion
- `.planning/ROADMAP.md` §"Phase 3" — success criteria 1-5
- `.planning/phases/02-analytics-audit/02-CONTEXT.md` — Phase 5 candidate lock that will consume Phase 3's reliable `completed`
- `docs/analytics-audit-2026-04-28.md` § "Cleanup / Hygiene Items" — `watch_progress.completed = false on every row` is the headline bug Phase 3 closes

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `services/player/internal/repo/sync_test.go:15` — `setupTestDB` SQLite `:memory:` helper. New repo tests reuse this pattern.
- `services/player/internal/repo/activity_test.go:13-43` — table-creation pattern for tests where a custom `CREATE TABLE` is needed (SQLite doesn't support `gen_random_uuid()`).
- `service/progress.go` already wires `prefService.UpsertAnimePreference` from progress saves — same pattern (best-effort post-write side effect) as the new `MarkCompleted` from `MarkEpisodeWatched`.

### Established Patterns to Preserve
- **Best-effort side effects on writes.** `service/list.go MarkEpisodeWatched` already logs WatchHistory create failures with `s.log.Errorw` rather than rolling back. New `MarkCompleted` failure follows the same pattern.
- **Idempotent upserts via GORM `clause.OnConflict`.** Both `progress.go:29-37` and `list.go:117-135` use `ON CONFLICT (user_id, anime_id, episode_number) DO UPDATE`. New `MarkCompleted` follows this template.
- **AutoMigrate-driven schema evolution.** No explicit migration files. Backfill SQL runs as a one-shot `db.Exec` after `AutoMigrate` in `main.go`. Idempotent + early-exit guard.

### Integration Points
- `MarkEpisodeWatched` flows: `POST /api/users/watchlist/{animeId}/episode` → `handler/list.go:180` → `service/list.go:175` → repos. All 4 players hit this same endpoint.
- `UpdateProgress` flows: `POST /api/users/progress` → existing handler → `service/progress.go:27` → `progressRepo.UpsertProgress` (after split). All 4 players' heartbeats hit this same endpoint.

</code_context>

<deferred>
## Deferred Ideas

- `watch_progress.completed_at` timestamp column — D-15.
- Per-episode-list aggregate endpoint — D-16. May be revisited in Phase 4 state machine work.
- Audit-trail of completion events (which player, which combo at completion time) — Phase 5 G-04-lite session_id partly covers this.

</deferred>

---

*Phase 3 — Single Source of Truth for "Watched"*
*Context gathered: 2026-04-28*
