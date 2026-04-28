# Phase 3: Single Source of Truth for "Watched" - Plan

**Created:** 2026-04-28
**Wave:** 1 (with Phase 2)
**Deploy gate:** Batched at end of Wave 1 (after Phase 3 lands)

<objective>
Make `watch_progress.completed = true` the canonical signal for "user has finished
this episode" — set by both auto-mark (20-min) and manual mark, sticky against
heartbeat saves, backfilled from existing `anime_list.episodes` data, with all 4
players' episode-list checkmarks reading from a consistent source.
</objective>

<scope>
**In:**
- Repo split: `UpsertProgress` (heartbeat, doesn't touch `completed`) + `MarkCompleted` (idempotent set-to-true)
- Service: `MarkEpisodeWatched` calls `MarkCompleted` after `IncrementEpisodes`
- Backfill: idempotent SQL on startup with early-exit guard
- Frontend: AnimeLib + Consumet checkmark field-name fix
- Tests: repo + service unit tests

**Out:** Phase 4 state machine, Phase 5 watch_count, Phase 7 cross-device freshness, removing `anime_list.episodes` as a stored field.
</scope>

<success_criteria>
1. **A-01:** Auto-mark (20-min) sets `watch_progress.completed = true` for the marked episode. Verified via repo test + manual integration test.
2. **A-02:** Manual mark button sets `watch_progress.completed = true` via the same code path (`MarkEpisodeWatched` → `MarkCompleted`). Verified via service test.
3. **D-02 (SC-3):** `anime_list.episodes` is the maintained denorm count; episode-list checkmarks in all 4 players read `entry?.episodes` (consistent source). Verified via grep.
4. **SC-4:** After Wave 1 deploys, `ui_audit_bot`'s seeded data shows the same watched-episode count on watchlist counter, episode-list checkmarks (all 4 players), and resume CTA.
5. **SC-5:** Backfill handles the existing 1956 rows where `anime_list.episodes > 0` — every (user, anime, ep ≤ episodes) gets `watch_progress.completed=true`.
6. `Completed=false` heartbeat overwrites are eliminated (the bug at `repo/progress.go:34`).
</success_criteria>

<task id="03-01">
## Task 03-01: Backend repo split + service dual-write

**Files:**
- `services/player/internal/repo/progress.go`
- `services/player/internal/service/progress.go`
- `services/player/internal/service/list.go`

**What:**
1. In `repo/progress.go`:
   - Rename existing `Upsert` → `UpsertProgress`. Remove `"completed":` from the `DoUpdates` map (heartbeat does NOT touch completed). Keep `progress`, `duration` (with GREATEST), `last_watched_at`, `updated_at`.
   - Add `MarkCompleted(ctx, userID, animeID string, episodeNumber int) error` — idempotent upsert that sets `completed=true`, `last_watched_at=now`, `updated_at=now`. Creates row if missing with `progress=0, duration=0, completed=true`. ON CONFLICT updates `completed=true, last_watched_at, updated_at`.
2. In `service/progress.go UpdateProgress`:
   - Remove the `Completed: false` line from the `WatchProgress` struct literal (zero value works; comment was misleading anyway).
   - Call `progressRepo.UpsertProgress(ctx, progress)` (rename of `Upsert`).
3. In `service/list.go MarkEpisodeWatched`:
   - After `IncrementEpisodes` returns (whether it updated or not), call `s.progressRepo.MarkCompleted(ctx, userID, animeID, req.Episode)` unconditionally.
   - Wrap in best-effort `if err := ...; err != nil { s.log.Errorw("failed to mark watch_progress completed", ...) }` — don't fail the request.
   - Place the call AFTER the auto-create-watchlist-entry branch and BEFORE the watch_history create — completion semantics belong with anime_list updates, history is observability.

**Done when:**
- `progress.go` has `UpsertProgress` (no `completed` in DoUpdates) and `MarkCompleted`
- `service/progress.go` UpdateProgress no longer hardcodes `Completed: false`
- `service/list.go` MarkEpisodeWatched calls `MarkCompleted` after IncrementEpisodes
- `go build ./...` clean from the repo root

**Verification:**
- `grep "Completed: false" services/player/internal/service/progress.go` → empty
- `grep "completed.*progress.Completed" services/player/internal/repo/progress.go` → empty (heartbeat doesn't touch completed)
- `grep "MarkCompleted" services/player/internal/{repo,service}/` → 3+ matches (definition + service call + tests)

</task>

<task id="03-02">
## Task 03-02: Backfill on startup

**Files:**
- `services/player/cmd/player-api/main.go`

**What:**
After `db.AutoMigrate(...)` succeeds (around line 57), insert a backfill block:

```go
// Phase 3 backfill: synthesize watch_progress.completed=true rows for legacy data.
// Idempotent (ON CONFLICT DO UPDATE), short-circuits after first deploy.
var anyCompleted int
db.DB.Raw("SELECT 1 FROM watch_progress WHERE completed = true LIMIT 1").Scan(&anyCompleted)
if anyCompleted == 0 {
    log.Infow("phase 3 backfill: synthesizing watch_progress.completed=true rows from anime_list.episodes")
    if err := db.DB.Exec(`
        INSERT INTO watch_progress (id, user_id, anime_id, episode_number, progress, duration, completed, last_watched_at, created_at, updated_at)
        SELECT gen_random_uuid(), al.user_id, al.anime_id, ep, 0, 0, true,
               COALESCE(al.completed_at, al.updated_at), NOW(), NOW()
        FROM anime_list al
        CROSS JOIN LATERAL generate_series(1, al.episodes) ep
        WHERE al.episodes > 0
        ON CONFLICT (user_id, anime_id, episode_number) DO UPDATE
        SET completed = true, updated_at = NOW()
        WHERE watch_progress.completed = false
    `).Error; err != nil {
        log.Errorw("phase 3 backfill failed (non-fatal)", "error", err)
    } else {
        log.Infow("phase 3 backfill complete")
    }
}
```

**Done when:**
- main.go has the backfill block guarded by the early-exit check
- `go build services/player/cmd/player-api/...` clean
- Idempotency manually traced: re-running the SQL in Postgres against fresh data flips no rows on second pass

**Verification:**
- `grep "phase 3 backfill" services/player/cmd/player-api/main.go` → 2-3 matches
- After Wave 1 deploy, `docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "SELECT COUNT(*) FROM watch_progress WHERE completed=true"` returns > 0 (was 0 before)

</task>

<task id="03-03">
## Task 03-03: Frontend checkmark consistency fix

**Files:**
- `frontend/web/src/components/player/AnimeLibPlayer.vue` (line 763)
- `frontend/web/src/components/player/ConsumetPlayer.vue` (line 1224)

**What:**
Change `if (entry?.episodes_watched) { watchedEpisodes.value = entry.episodes_watched }` →
`if (entry?.episodes) { watchedEpisodes.value = entry.episodes }`. Mirrors what
Kodik (`KodikPlayer.vue:606`) and HiAnime (`HiAnimePlayer.vue:1249`) already do.

**Done when:**
- Both files read `entry?.episodes` (not the non-existent `episodes_watched`)
- `bunx tsc --noEmit` (in `frontend/web/`) clean

**Verification:**
- `grep -n "episodes_watched" frontend/web/src/components/player/` → empty (no more references)
- All 4 players read `entry?.episodes` — `grep -n "entry?.episodes\b" frontend/web/src/components/player/` → 4 matches

</task>

<task id="03-04">
## Task 03-04: Tests

**Files (new):**
- `services/player/internal/repo/progress_test.go`
- `services/player/internal/service/list_mark_completed_test.go`

**What:**
1. **Repo tests** (in-memory SQLite, follow `sync_test.go:15` pattern):
   - `TestProgressRepository_UpsertProgress_PreservesCompletedTrue` — insert a row with `completed=true`, then call `UpsertProgress` with the heartbeat-shape struct (no completed assignment). Assert: `completed` stays `true`. **This is the regression-prevention test for the heartbeat bug.**
   - `TestProgressRepository_MarkCompleted_CreatesRowIfMissing` — call MarkCompleted on a fresh DB. Assert: row exists with `completed=true`, `progress=0`, `duration=0`.
   - `TestProgressRepository_MarkCompleted_Idempotent` — call MarkCompleted twice. Assert: still one row, still `completed=true`, no error.
   - `TestProgressRepository_MarkCompleted_FlipsExistingFalseRow` — insert a row with `completed=false, progress=500, duration=1440`. Call MarkCompleted. Assert: `completed=true`, `progress` and `duration` unchanged.

2. **Service test** (mock or in-memory):
   - `TestListService_MarkEpisodeWatched_TriggersMarkCompleted` — call MarkEpisodeWatched. Assert: `progressRepo.MarkCompleted` was invoked with the correct (userID, animeID, episode).

**Done when:**
- 4 repo tests pass
- 1 service test passes
- `go test ./services/player/...` clean

**Verification:**
- `cd services/player && go test ./internal/repo/... -run TestProgressRepository -v` shows all 4 PASS
- `cd services/player && go test ./internal/service/... -run TestListService_MarkEpisodeWatched_TriggersMarkCompleted -v` shows PASS

</task>

<task id="03-05">
## Task 03-05: Update REQUIREMENTS, ROADMAP, STATE; commit

**What:**
1. `.planning/REQUIREMENTS.md` — mark A-01, A-02, D-02 status `Complete (Phase 3 — 2026-04-28)`. Update the requirement bullets with closure cites.
2. `.planning/ROADMAP.md` — mark `[x] Phase 3` ✓ in summary list; populate Plans/Deliverable in detail section.
3. `.planning/STATE.md` — bump `completed_phases: 3`, update Wave 1 status row, current focus → "Wave 1 deploy".
4. Write `03-01-SUMMARY.md` recording outcomes per task.
5. Single commit covering all of Phase 3 (one-shot since the wave batches at end).

**Done when:**
- Single git commit lands all Phase 3 changes (backend + frontend + tests + docs)
- `git status` clean

</task>

<dependencies>
- **Hard dependency:** Phase 1 (override-rate baseline observable). Already complete ✓.
- **Recommended:** Phase 2 lands first (it just did) so the hygiene context is on disk.
- **No dependency:** Phase 4 (waits on Phase 3).

</dependencies>

<risks>
- **Backfill performance on first deploy.** ~30k inserts in one transaction. Mitigation: tested manually; expected < 5s on production hardware. If it exceeds 30s, split into batches by user_id.
- **Best-effort dual-write divergence.** A `MarkCompleted` failure after a successful `IncrementEpisodes` leaves the data with `anime_list.episodes++` but no `watch_progress.completed=true` for that episode. Mitigation: next mark on the same episode recovers (idempotent); Phase 4 state machine falls back to `anime_list.episodes` if no progress data exists; the operator can re-run the backfill SQL ad-hoc to repair drift.
- **Sticky-true edge case.** A user marks an episode watched, then "unmarks" — Phase 3 has no unmark path. Mitigation: there is no current unmark UI on any of the 4 players (verified via grep). If Phase 7 Advanced Settings adds one, it'll need an explicit `MarkUncompleted` repo method.

</risks>

<deployment>
- **Code lands on main:** Per task 03-05.
- **Deploy:** **Wave 1 batch deploy** runs `/animeenigma-after-update` once both Phase 2 and Phase 3 are committed. Affected service: `player`. The backfill runs as the new container starts.
- **Verification on production:** Per CONTEXT.md D-13, executed AFTER Wave 1 deploy via `ui_audit_bot` — not gating Phase 3 commit, gating Wave 1 closeout.
</deployment>

---

*Phase 3 — Single Source of Truth for "Watched"*
*Plan created: 2026-04-28*
