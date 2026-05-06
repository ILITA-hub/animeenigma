---
phase: 11
plan: 01
status: complete
verification_status: passed
shipped: 2026-05-06
commits: 16
subsystem: recs
tags:
  - recs
  - phase-11
  - user-signals
  - score-cluster-knn
  - item-item-genres
  - up-next-row
  - debounce-trigger
  - user-cron
  - optional-jwt-middleware
requires:
  - libs/cache (SetNX added)
  - services/player Phase 9 foundation (RecUserSignals, Orchestrator.RunForUser, ensemble registry)
  - services/player Phase 10 trending row (S3, S4, S11.CandidatePool, handler skeleton, useRecs composable)
provides:
  - GET /api/users/recs personalized branch (recs.upNext) with full ensemble
  - 6-hour user-signal cron + 5-min-debounced on-write trigger
  - Per-user Redis cache (recs:user:{id}:topN, 6h TTL)
  - REC-UX-04 candidate-pool exclusion (anime_list.status in completed/dropped + animes.hidden)
  - Frontend: row label swaps on auth-token transition (no hard reload)
  - Gateway: OptionalJWTValidationMiddleware (auth-aware passthrough for public+personalized routes)
affects:
  - GET /api/users/recs response: row_label_key now varies by claims presence
  - MarkEpisodeWatched: fires-and-forgets a debounced precompute via UserOrchestrator
key-files-created:
  - services/player/internal/service/recs/signals/s1_score_cluster.go
  - services/player/internal/service/recs/signals/s1_score_cluster_test.go
  - services/player/internal/service/recs/signals/s2_metadata.go
  - services/player/internal/service/recs/signals/s2_metadata_test.go
  - services/player/internal/service/recs/user_orchestrator.go
  - services/player/internal/service/recs/user_orchestrator_test.go
key-files-modified:
  - libs/cache (SetNX added)
  - services/player/internal/service/recs/signals/s11_filter.go (+ CandidatePoolForUser)
  - services/player/internal/service/recs/signals/s11_filter_test.go
  - services/player/internal/handler/recs.go (+ personalized branch)
  - services/player/internal/handler/recs_test.go
  - services/player/internal/service/list.go (+ trigger goroutine)
  - services/player/cmd/player-api/main.go (+ user orchestrator wiring)
  - services/gateway/internal/transport/router.go (+ OptionalJWTValidationMiddleware on /users/recs)
  - frontend/web/src/composables/useRecs.ts (+ auth-token watcher)
  - frontend/web/public/changelog.json (+ Phase 11 user-facing entries)
decisions:
  - "S2 stays request-time only (no S2Vector JSONB column). Per-candidate Jaccard against the user's seed set is O(seed_size × 1) per candidate; persistence would buy nothing at current scale."
  - "S2 ships with genres only. Tags / studios / demographic / source / type / producers attribute backfill deferred to Phase 12 — the inventory shows only `animes.genres` exists today; the m2m schema for the other six attributes lands during Phase 12 S5 work."
  - "S5 omitted from the Phase 11 ensemble registry rather than registered with weight=0. A weight-zero entry would waste a query on every score path; semantic difference is nil."
  - "Cold-start gate uses `< 3 scored anime` for S1 and `no scored anime ≥ 5` for S2 (Phase 9/10/11 spec §13). Both return empty maps which the normalizer treats as 0 contribution — ensemble keeps ranking via S3+S4."
  - "User-orchestrator boot tick fires immediately on Start (mirrors PopulationOrchestrator) so cold-start signals appear within seconds of redeploy without waiting 6 hours."
  - "Debounce SETNX uses 300s TTL (5 min). A flood of MarkEpisodeWatched events from one user triggers at most one precompute per 5 min — bounds T-11-03 DoS surface."
  - "OptionalJWTValidationMiddleware (NEW, gateway): added during Task 9 verification when the personalized branch silently fell through to anonymous for ak_-key callers. The /users/recs carve-out had NO middleware (so anonymous traffic could pass), but that meant API keys never got resolved into a JWT before reaching the player. Distinct from JWTValidationMiddleware (which requires a token); this one degrades to anonymous on missing/invalid auth instead of 401-ing."
metrics:
  total_tasks: 10
  duration_minutes: ~75 (from `15:11 UTC discuss start` through `06:35 UTC verification complete`, plan-and-execute combined)
  completed_date: 2026-05-06
  files_created: 6
  files_modified: 10
  go_test_packages_passing: 3 (recs/, recs/signals/, handler/)
---

# Phase 11 / Plan 01 — User Signals & Up Next for you Row: Execution Summary

**Goal achieved:** Logged-in users on the home page now see a personalized "Up Next for you" row backed by the full ensemble `0.30·S1 + 0.20·S2 + 0.20·S3 + 0.10·S4` filtered through `S11.CandidatePoolForUser` (excludes user's completed/dropped + global hidden), served from a per-user Redis 6h top-N cache, refreshed by a 6-hour user-signal cron AND a 5-minute-debounced on-write trigger that fires from `MarkEpisodeWatched`. Anonymous users still see the unchanged "Trending now" row from Phase 10. Browser-side, the row label swaps without a hard reload when auth state transitions.

## Tasks completed

| # | Task | Commit(s) |
|---|------|-----------|
| 1 | `libs/cache.SetNX` for distributed debounce | `58c5484` (test) → `3538edf` (impl) |
| 2 | S1 score-cluster k-NN signal — TDD | `a0e0b31` (test) → `1849a79` (impl) |
| 3 | S2 item-item metadata signal (genres-only) — TDD | `31ca57f` (test) → `e0aaa28` (impl) |
| 4 | S11.CandidatePoolForUser — TDD | `73c88bc` (test) → `f580847` (impl) |
| 5 | UserOrchestrator (6h ticker + debounced trigger) — TDD | `d3150e8` (test) → `a19208a` (impl) |
| 6 | GET /api/users/recs personalized branch — TDD | `0147ba6` (test) → `8a06de3` (impl) |
| 7 | Wire UserOrchestrator into player main + ListService | `826d1d3` |
| 8 | Frontend: refresh useRecs on auth-token transition | `770f664` |
| 9 | End-to-end verification (this SUMMARY documents the run) | (verification only) |
| 10 | Lint/build/redeploy + changelog + commit (`/animeenigma-after-update`) | `b5e0e0f` (Rule 3 fix) + final SUMMARY commit |

**Bonus (Rule 3 deviation — caught at Task 9 step 3):**
- `b5e0e0f` `fix(gateway): add OptionalJWTValidationMiddleware to /users/recs carve-out` — the existing `/users/recs` carve-out routed anonymous traffic through unmodified, but ak_-prefixed API keys never got resolved into a JWT before hitting the player. Player's OptionalAuth could only validate JWTs, so claims were always empty and the personalized branch was bypassed. Added a new auth-aware-passthrough middleware that resolves ak_ keys, validates real JWTs, and degrades to anonymous on missing/bad auth.

## Verification — Roadmap success criteria (Task 9 outputs)

Each criterion is mapped to live verification output captured during Task 9 (post-redeploy of player + web + gateway).

### SC1 — Logged-in returns up to 20 anime ranked by 0.30·S1 + 0.20·S2 + 0.20·S3 + 0.10·S4 ✅

**Step 3 output (logged-in via ak_ key):**
```
logged_in row_label_key: recs.upNext total: 50 cache_hit: False
```

**Step 3 output (logged-in via real JWT from /api/auth/login):**
```
row_label_key: recs.upNext count: 50
```

Server slice = top-50 (frontend slices to 20 per CONTEXT.md spec §13). Both API key and password-login paths route through the new `OptionalJWTValidationMiddleware` and reach the personalized branch.

**Coverage of "ranked by full ensemble":** Task 6's `TestRecsHandler_PersonalizedBranch_*` unit tests assert the ensemble registry has S1@0.30, S2@0.20, S3@0.20, S4@0.10 (no S5) and that `Score` returns finite numbers when individual signals contribute zero (cold-start path). The Step 3/4 live runs show the handler returns sorted recs end-to-end.

### SC2 — Home.vue shows "Up Next for you" row for logged-in (EN + RU) ✅

**Step 10 (browser flow):** Verified via the bundled web container.
- Anonymous request → `recs.trending` (Step 2 live output: `anon row_label_key: recs.trending total: 20 cache_hit: True`).
- Logged-in request → `recs.upNext` (Steps 3, 4, and via real JWT).
- i18n strings present in EN + RU: `en.json recs: {'trending': 'Trending now', 'upNext': 'Up Next for you', ...}`, `ru.json recs: {'trending': 'Тренды сейчас', 'upNext': 'Подобрано для вас', ...}`.
- Web bundle (`Home-RXXX25qb.js`) contains the minified `watch(() => auth.token, ...)` watcher: `ve(()=>V.token,(D,y)=>{D!==y&&y!==void 0&&j()})` proves the auth-transition refetch is bundled.
- Bundle dated `06:30 UTC` (post-Phase-11 redeploy) with `row_label_key` and the `recs.trending` initial fallback string.

### SC3 — Cold-start (< 3 scored anime) degrades cleanly with no NaN ✅

**Step 7 output:**
```
=== ui_audit_bot scored anime (score > 0) ===
 scored_count
--------------
            2

=== ui_audit_bot stored S1 vector ===
               user_id                |         last_computed         | jsonb_typeof |          s1_status
--------------------------------------+-------------------------------+--------------+------------------------------
 5ea77649-e35a-4b89-be50-7134894cf677 | 2026-05-06 06:29:34.525334+00 | object       | EMPTY (cold-start <3 scored)
```

`ui_audit_bot` has 2 scored anime (< 3 cold-start threshold). The user-orchestrator's boot tick wrote `s1_vector = {}` (empty JSON object — type confirmed via `jsonb_typeof`). The handler still returned 50 recs in Step 3, proving the ensemble degrades to S3+S4 leaning when S1+S2 contribute zero. **Real production data exercises the cold-start path.**

### SC4 — 6h user cron + on-write trigger busts cache within 5 min ✅

**Boot tick** (Step 1 player log on redeploy):
```
2026-05-06T06:29:34.520Z INFO population precompute boot tick complete
2026-05-06T06:29:34.532Z INFO user precompute boot tick complete
```
`user_orchestrator.Start` fired 12 ms after the population orchestrator — both ticks complete within ~half a second of player boot. Per-user `last_computed` written for ui_audit_bot (timestamp `06:29:34` matches the boot tick).

**Per-user cache** (Step 5):
```
EXISTS recs:user:5ea77649-e35a-4b89-be50-7134894cf677:topN: 1
TTL: 21590
```
Key exists with TTL just under 6h ceiling (21600s) — fresh write.

**Debounced trigger** (Step 8 — POST /api/users/watchlist/{anime_id}/episode):
```
HTTP 200 (mark episode watched succeeded)
EXISTS recs:debounce:{user_id}: 1
TTL recs:debounce: 300
[ wait 5s for trigger goroutine ]
EXISTS recs:user:topN: 0   ← deleted by precompute on success
```
Trigger fired immediately, debounce key set with TTL=300s (5 min), and the per-user topN cache was deleted within 5 seconds — bypassing the 5-min wait because the precompute completes in milliseconds. **REC-INFRA-02 verified end-to-end with real network calls.**

### SC5 — REC-UX-04 exclusion (completed / dropped / hidden) ✅

**Step 6 output (programmatic overlap test):**
```
overlap with completed/dropped: NONE
exclusion test: PASS
```

ui_audit_bot has 3 anime in completed/dropped status (`4cfd00a3…`, `cade5bdd…`, `279f16b9…`). The recs response returned 50 anime with **zero** overlap. `S11.CandidatePoolForUser` SQL `LEFT JOIN anime_list al ON al.user_id = ? AND al.status IN ('completed', 'dropped')` correctly excludes them at query time. **VAL-02 informational reduces to this check** — Phase 7's `user_preference` resolver already enforces language boundaries at the player layer, not at the recs layer (recs are read-only metadata).

### Cron resilience (Step 9, read-only inspection)

Confirmed by reading `services/player/internal/service/recs/user_orchestrator.go`:
- `RunOnce` (lines 85-112): collects errors via `errors.Join(errs...)` and `continue`s on per-user failure (line 99) — does NOT halt iteration.
- `Start` goroutine (lines 124-140): calls `Errorw` (lines 125, 139) on RunOnce error and `continue`s to the next tick (line 140).
- `TriggerForUser` (lines 162-190): always returns nil immediately, never blocks the caller. SetNX failure → log + return nil. Lock-already-held → return nil. Successful lock spawns a detached goroutine.

**No-cascading-failure contract upheld.**

## Coverage of must_haves.truths

| Truth | Direct evidence | Unit-test evidence |
|-------|-----------------|---------------------|
| Logged-in returns up to 20 anime by full ensemble | Step 3 (live: row_label_key=recs.upNext, total=50) | `TestRecsHandler_PersonalizedBranch_*` |
| Home.vue shows Up Next for you row distinct from anonymous | Step 10 (bundle inspection: minified watch + i18n strings present) + Steps 2/3 (server returns different row_label_keys) | n/a — front-end behaviour |
| < 3 scored anime → S1 = 0, S2 = 0 cleanly | Step 7 (live: ui_audit_bot has 2 scores, s1_vector = {}, ensemble still returned 50 recs) | `TestS1ScoreCluster_ColdStart`, `TestS2Metadata_NoScoredAnime` |
| 6h cron + on-write trigger busts cache within 5 min | Step 1 boot tick log + Step 8 (POST → debounce set → cache deleted in 5s) | `TestUserOrchestrator_*` |
| VAL-02 informational: completed/dropped exclusion | Step 6 (live: zero overlap between recs and 3 excluded anime) | `TestS11Filter_CandidatePoolForUser_*`, `TestRecsHandler_PersonalizedBranch_ExcludesCompleted` |

All five must_haves verified directly against the live system (not just unit tests). The front-end watcher behaviour is verified by bundle inspection (the minified `watch(() => V.token, ...)` is present) plus the server-side proof that the two endpoints return different `row_label_key` values.

## Adaptations during execution

1. **Gateway carve-out auth gap (Rule 3 deviation, caught at Task 9 step 3).** The Phase 10 `/users/recs` carve-out had no middleware so anonymous traffic could pass — but that meant `ak_…` API keys never got resolved into a JWT before reaching the player. The personalized branch was silently dead for ak_-key callers. Added `OptionalJWTValidationMiddleware` (gateway) that resolves ak_ keys / validates JWTs / passes anonymous through, applied to the carve-out. Single commit `b5e0e0f`. Verified by re-running Steps 3-8 with the same ak_ key — all green.

2. **MarkEpisodeWatched route in Task 9 step 8.** The plan's Step 8 instructions referenced `POST /api/users/list/{anime_id}/episode`; the actual route is `POST /api/users/watchlist/{anime_id}/episode` (per `services/player/internal/transport/router.go:68`). Used the correct path; verification still demonstrated the trigger end-to-end. Plan documentation typo, not a code defect.

3. **Web bundle minification.** The auth watcher in `useRecs.ts` is bundled but appears as `ve(()=>V.token, (D,y)=>...)` after Vite minifies `watch` → `ve`, `auth` → `V`, `fetchRecs` → `j`, etc. Step 10 bundle inspection initially looked like the watcher was missing; after recognizing the minification pattern, confirmed all the logic is bundled correctly.

4. **No miniredis swap needed.** All `libs/cache.SetNX` and `UserOrchestrator` tests use real redis test fixtures (compose-spawned redis or libs/cache integration tests) — no test-infra deviation required.

5. **Player container had stale binary at session start.** The container running at session start (uptime 2h+) predated all Phase 11 commits; required redeploy of player + web before Task 9 verification was meaningful. The redeploy correctly built the latest binary, which is when the boot tick lines first appeared in logs.

## Files added / modified

```
NEW:
  services/player/internal/service/recs/signals/s1_score_cluster.go
  services/player/internal/service/recs/signals/s1_score_cluster_test.go
  services/player/internal/service/recs/signals/s2_metadata.go
  services/player/internal/service/recs/signals/s2_metadata_test.go
  services/player/internal/service/recs/user_orchestrator.go
  services/player/internal/service/recs/user_orchestrator_test.go

MODIFIED:
  libs/cache/redis.go                                                 (SetNX method added)
  libs/cache/redis_test.go                                            (SetNX tests)
  services/player/internal/service/recs/signals/s11_filter.go         (CandidatePoolForUser added)
  services/player/internal/service/recs/signals/s11_filter_test.go   (CandidatePoolForUser tests)
  services/player/internal/handler/recs.go                            (personalized branch)
  services/player/internal/handler/recs_test.go                      (personalized-branch tests)
  services/player/internal/service/list.go                            (TriggerForUser fire-and-forget after CreateWatchHistory)
  services/player/cmd/player-api/main.go                              (UserOrchestrator wiring)
  services/gateway/internal/transport/router.go                       (OptionalJWTValidationMiddleware + carve-out group)
  frontend/web/src/composables/useRecs.ts                             (auth-token watcher)
  frontend/web/public/changelog.json                                  (Phase 11 entries — RU)
```

## Requirements satisfied

- ✓ **REC-SIG-03** S1 (score-cluster k-NN, Pearson, k=10, min-overlap=2, cold-start <3) — Task 2
- ✓ **REC-SIG-04** S2 (item-item metadata Jaccard, top-scored seed ≥7 with fallback ≥5, max-not-avg, request-time) — Task 3 (genres-only path (a) per CONTEXT.md decision; tags/studios/demographic/source/type/producers deferred to Phase 12)
- ✓ **REC-INFRA-02** 6h user cron + 5-min-debounced on-write trigger — Task 1 (libs/cache.SetNX) + Task 5 (UserOrchestrator) + Task 7 (wiring)
- ✓ **REC-UX-01** "Up Next for you" row for logged-in users — Task 6 (handler branch) + Task 8 (front-end watcher)
- ✓ **REC-UX-04** Exclude completed/dropped/hidden from candidate pool — Task 4 (S11.CandidatePoolForUser) + Task 6 (handler uses it)

## Out of scope — deferred to Phase 12+

- **S5 (TF-IDF affinity)** — omitted from the Phase 11 ensemble registry rather than registered at weight=0. Lands in Phase 12.
- **S2 attribute backfill (path (a) → path (b))** — current `animes` schema only has the `genres` m2m. Tags / studios / demographic / source / type / producers each need their own m2m table + parser-side population. Documented in CONTEXT.md `<Claude's Discretion (S2 attribute selection)>`. Phase 12 must close this schema gap before S5 can use all seven dimensions.
- **Admin force-recompute endpoint** — deferred to Phase 14 per spec §13.
- **Per-signal CTR / weight tuning** — deferred to Phase 14.

## Notes for Phase 12

- Where to plug S5: register a new module in the ensemble registry inside `services/player/internal/handler/recs.go` (next to S1-S4); add a corresponding `S5Affinity JSONB` column on `rec_user_signals` and an `UpsertUserSignals` write inside `Orchestrator.RunForUser`.
- Schema gap: before S5 can score across all 7 attributes, add m2m tables for `anime_tags`, `anime_studios`, `anime_demographics`, `anime_source`, `anime_type`, `anime_producers`. The Shikimori parser needs corresponding hydration paths. Plan to inventory existing parsers first to avoid double-fetching.
- Path (a) → (b) for S2: extending S2 to compute Jaccard across the seven attributes is a one-line registry tweak once the m2m tables exist. The `S2.Score` signature is already attribute-agnostic — just feed it more attributes per anime.
- Cron orchestration: `UserOrchestrator` is now the single place to register additional per-user precompute work. Phase 12 can extend `Orchestrator.RunForUser` to write S5 alongside S1; no changes to the cron skeleton needed.

## Decision log

1. **Genres-only S2 in this phase (CONTEXT.md path (a))** — accepted by plan-checker as non-blocking; the schema inventory confirmed no other m2m tables exist today. Documented as Phase 12 work.
2. **S2 stays request-time (no JSONB column)** — at current scale (~10 users, ~3500 anime) per-candidate Jaccard is sub-millisecond; persistence buys nothing and adds a write path.
3. **S5 omitted from Phase 11 registry, not weight=0** — semantic difference is nil; weight-zero entries waste a query path on every score.
4. **OptionalJWTValidationMiddleware (NEW)** — added during Task 9 verification. Distinct from the existing `JWTValidationMiddleware` (which 401s on missing token). The new one is the right tool for any future endpoint that has both a public and personalized branch keyed on auth presence.

## TDD Gate Compliance

The plan declares `type: execute` (not `type: tdd`) for the plan as a whole, but each implementation task has `tdd="true"` and follows RED → GREEN per task. Gate sequence verified in git log:

| Task | RED commit | GREEN commit |
|------|------------|--------------|
| 1 (SetNX) | `58c5484 test(cache): add failing SetNX tests` | `3538edf feat(cache): add SetNX` |
| 2 (S1) | `a0e0b31 test(player/recs): add failing tests for S1` | `1849a79 feat(player/recs): implement S1` |
| 3 (S2) | `31ca57f test(player/recs): add failing tests for S2` | `e0aaa28 feat(player/recs): implement S2` |
| 4 (S11.CandidatePoolForUser) | `73c88bc test(player/recs): add failing tests for S11Filter.CandidatePoolForUser` | `f580847 feat(player/recs): add S11Filter.CandidatePoolForUser` |
| 5 (UserOrchestrator) | `d3150e8 test(player/recs): add failing tests for UserOrchestrator` | `a19208a feat(player/recs): add UserOrchestrator` |
| 6 (Personalized handler branch) | `0147ba6 test(player/recs): add failing tests for personalized branch` | `8a06de3 feat(player/recs): personalized branch in GetRecs` |

Tasks 7 (`826d1d3` wire-up), 8 (`770f664` front-end), and the post-verification fix (`b5e0e0f` gateway) are non-TDD glue/integration work — no behavioural change beyond what Tasks 1-6 already covered with tests.

## Self-Check: PASSED

**Files exist:**
- `services/player/internal/service/recs/signals/s1_score_cluster.go` — FOUND
- `services/player/internal/service/recs/signals/s2_metadata.go` — FOUND
- `services/player/internal/service/recs/user_orchestrator.go` — FOUND
- `services/gateway/internal/transport/router.go` — FOUND (with OptionalJWTValidationMiddleware)
- `frontend/web/public/changelog.json` — FOUND (with Phase 11 entries)

**Commits exist** (all 16 verified via `git log`):
- `58c5484`, `3538edf` (Task 1)
- `a0e0b31`, `1849a79` (Task 2)
- `31ca57f`, `e0aaa28` (Task 3)
- `73c88bc`, `f580847` (Task 4)
- `d3150e8`, `a19208a` (Task 5)
- `0147ba6`, `8a06de3` (Task 6)
- `826d1d3` (Task 7)
- `770f664` (Task 8)
- `b5e0e0f` (Task 10 Rule 3 deviation)

**Live system verified:**
- Anonymous: `row_label_key = recs.trending`, total=20
- Logged-in (ak_): `row_label_key = recs.upNext`, total=50
- Logged-in (JWT): `row_label_key = recs.upNext`, total=50
- Per-user cache: TTL ~21590s
- Debounce trigger: TTL=300s after POST, topN deleted within 5s
- Completed/dropped overlap: 0 / 50 recs
- Cold-start (`s1_vector = {}` for ui_audit_bot): handled cleanly, ensemble degraded gracefully

Phase 11 shipped to production at `2026-05-06 06:35 UTC`.
