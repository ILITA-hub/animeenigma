# Phase 3 — Plan 01 — Summary

**Completed (code):** 2026-04-28
**Plan:** 03-PLAN.md (5 tasks: repo split, backfill, frontend fix, tests, doc updates)
**Status:** ✓ Implementation complete; ⏳ Wave 1 batch deploy + production verification pending

## One-liner

Split `ProgressRepository.Upsert` into `UpsertProgress` (heartbeat, doesn't touch `completed`) + `MarkCompleted` (idempotent set-to-true) so the `completed=true` flag becomes sticky against the player heartbeat that previously overwrote it to false on every save. Wired `MarkCompleted` into `MarkEpisodeWatched` so both the 20-min auto-mark and the manual mark button flip canonical truth in one step. Added an idempotent backfill on player-api startup that synthesizes `watch_progress.completed=true` rows from existing `anime_list.episodes` data — closes the gap between 1956 list-completed entries and 0/385 watch_progress-completed rows. Fixed a latent bug in AnimeLib + Consumet players that read a non-existent `entry?.episodes_watched` field; both now read `entry?.episodes` matching Kodik + HiAnime. 5 repo + 3 service tests cover the new write paths and regression-protect the heartbeat sticky-true invariant.

## What changed

| Layer | File | Change |
|---|---|---|
| Backend repo | `services/player/internal/repo/progress.go` | `Upsert` renamed to `UpsertProgress`; `completed` removed from its DoUpdates clause. New `MarkCompleted(ctx, userID, animeID, episodeNumber)` method — idempotent upsert that flips `completed=true`, preserves existing `progress`/`duration`. |
| Backend service | `services/player/internal/service/progress.go` | `UpdateProgress` no longer hardcodes `Completed: false`; calls renamed to `UpsertProgress`. |
| Backend service | `services/player/internal/service/list.go` | `MarkEpisodeWatched` now invokes `progressRepo.MarkCompleted` after `IncrementEpisodes` succeeds, after the auto-create branch, and after the no-op already-marked branch — best-effort with logged failures (mirrors existing `WatchHistory` pattern). |
| Backend startup | `services/player/cmd/player-api/main.go` | After `AutoMigrate`, runs idempotent backfill SQL with early-exit guard (`SELECT 1 FROM watch_progress WHERE completed=true LIMIT 1`). On first deploy, synthesizes `watch_progress.completed=true` rows for every `(user, anime, ep ≤ anime_list.episodes)` missing or false. Subsequent restarts short-circuit. |
| Frontend | `frontend/web/src/components/player/AnimeLibPlayer.vue` | `entry?.episodes_watched` → `entry?.episodes` (line 763). |
| Frontend | `frontend/web/src/components/player/ConsumetPlayer.vue` | `entry?.episodes_watched` → `entry?.episodes` (line 1224). |
| Tests | `services/player/internal/repo/progress_test.go` | NEW — 5 tests including the regression-prevention `UpsertProgress_PreservesCompletedTrue`. Registers a SQLite GREATEST UDF so production-shape SQL executes against the in-memory test DB. |
| Tests | `services/player/internal/service/list_mark_completed_test.go` | NEW — 3 sub-tests covering all branches of `MarkEpisodeWatched` (auto-create, increment, idempotent re-mark). Registers GREATEST + NOW UDFs for SQLite. |
| Planning | `.planning/REQUIREMENTS.md` | A-01, A-02, D-02 marked Complete (2026-04-28) with closure cites. |
| Planning | `.planning/ROADMAP.md` | Phase 3 ✓ in summary list; Plans / Deploy / Verification populated in detail section. |
| Planning | `.planning/STATE.md` | Wave 1 deploy pending; Phase 3 implementation complete; Wave Plan table updated. |
| Planning | `.planning/phases/03-single-source-of-truth-for-watched/{03-CONTEXT,03-PLAN,03-01-SUMMARY}.md` | NEW. |

## Test results

```
ok  github.com/ILITA-hub/animeenigma/services/player/internal/handler   0.021s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/repo      0.031s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/service   0.027s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/transport 0.007s
```

Eight new tests, all pass. Frontend `bunx tsc --noEmit` clean.

## Success criteria status

| SC | Status | Evidence |
|---|---|---|
| 1. Auto-mark flips `watch_progress.completed=true` | ✓ Code complete | `service/list.go:233` calls `MarkCompleted` after `IncrementEpisodes`; same path used by both auto-mark and manual mark per `frontend/web/src/components/player/*Player.vue` |
| 2. Manual mark uses the same code path | ✓ Code complete | All 4 player components call `userApi.markEpisodeWatched` → `POST /users/watchlist/{animeId}/episode` → `service.MarkEpisodeWatched` |
| 3. `anime_list.episodes` is the maintained denorm; checkmarks read consistently | ✓ Code complete | `IncrementEpisodes` writes denorm; all 4 players now read `entry?.episodes` from `getWatchlistEntry` (verified via grep) |
| 4. `ui_audit_bot` shows the same count on watchlist counter, episode-list checkmarks, resume CTA | ⏳ Pending Wave 1 deploy | Verification per CONTEXT.md D-13 — runs after `/animeenigma-after-update` |
| 5. Backfill handles existing `anime_list.episodes > 0` rows | ✓ Code complete; ⏳ effect verifies on first deploy | `main.go` backfill block + early-exit guard |
| (Bug) Heartbeat overwrite of `completed=false` eliminated | ✓ Code complete + tested | `TestProgressRepository_UpsertProgress_PreservesCompletedTrue` regression-protects this |

## What's next

1. **Right now:** Commit Phase 3 (single commit: backend + frontend + tests + doc updates).
2. **Wave 1 batch deploy:** Run `/animeenigma-after-update` — redeploys `player` service, runs `make health`, updates `frontend/web/public/changelog.json`, commits and pushes.
3. **Production verification:** Per CONTEXT.md D-13. Use `ui_audit_bot` to mark an episode in each of the 4 players, then check Postgres for the `watch_progress.completed=true` row + verify episode-list checkmarks render across all 4 players.
4. **After Wave 1 verified:** Open Wave 2 — plan + execute Phase 4 (state machine) and Phase 5 (gap-fill columns) in parallel.
