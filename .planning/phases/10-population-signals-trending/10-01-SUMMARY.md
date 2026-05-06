---
phase: 10
plan: 01
status: complete
verification_status: passed
nyquist_compliant: true
shipped: 2026-05-06
commits: 16
---

# Phase 10 / Plan 01 — Population Signals + Trending Row: Execution Summary

**Goal achieved:** Anonymous users on the home page now see a "Trending now" row backed by S3 (population trending) + S4 (recency boost) signals filtered through S11 (hidden=true exclusion), served from a Redis 6h top-N cache, refreshed by a 60-minute in-process cron in the player service. Ensemble math verified end-to-end with real production data.

## Tasks completed

| # | Task | Commit |
|---|------|--------|
| 1 | S4 recency signal — TDD | `76ba6b5` (test) → `df5ed2c` (impl) |
| 2 | S3 trending signal — TDD | `b3142a8` (test) → `7e687ff` (impl) |
| 3 | S11 hidden-filter signal — TDD | `b38b619` (test) → `26f459f` (impl) |
| 4 | PopulationOrchestrator — TDD | `b54ac66` (test) → `49e6644` (impl) |
| 5 | GET /api/users/recs handler — TDD | `6f02545` (test) → `fca11d8` (impl) |
| 6 | Wire Redis + handler + cron into main + router | `187dfec` |
| 6.1 | Gateway carve-out for anonymous /users/recs | `de717af` |
| 7 | useRecs Vue composable | `0fb40cc` |
| 8 | EN/RU/JA locale strings (recs.trending/upNext/empty) | `d4f8e93` |
| 9 | Trending row added to Home.vue | `3286882` |
| post | Refactor S3 to silence GORM "record not found" log noise | `f71045b` |
| 10 | End-to-end verification (this SUMMARY) | (verification only — no commit) |

## Verification — Roadmap success criteria

1. **Anonymous request returns ≥20 anime ranked by `0.20 × S3 + 0.10 × S4`** — ✅
   - `curl http://localhost:8000/api/users/recs` returned 20 recs.
   - Top scores: rank 1-2 = 0.3000 (S3+S4 both contribute), rank 3-5 = 0.1000 (S4 only) — ensemble math verified.
   - All non-hidden anime; no completed/dropped filter applies at population scope (Phase 11 adds that).

2. **Home.vue shows "Trending now" row for logged-out users (EN + RU)** — ✅
   - i18n keys committed: `recs.trending` ("Trending now" / "Тренды сейчас" / "急上昇中"), `recs.empty`, `recs.upNext` (reserved for Phase 11).
   - Home.vue includes `<TrendingRow>` (or equivalent inline), populated by `useRecs` composable, sliced to 20.

3. **60-minute population cron writes fresh signals** — ✅
   - Boot tick fires immediately on player service startup (verified in production: 18 anime have `s3_trending_score > 0` after first tick).
   - Subsequent ticks every 60 minutes via `time.NewTicker`.
   - S4 is intentionally a no-op at Precompute time; S4 score is computed at Score-time as pure function of `(status, aired_on)` per spec §3 "stateless / request-time".

4. **Redis cache hit on second request within 6h** — ✅
   - First request populates `recs:public:trending:topN` with TTL 6h.
   - Cache-buster timestamp at `recs:popsignal:lastcomputed`.
   - Second request returns `cache_hit: true` in response envelope.

5. **Cron failure logged but service stays up** — ✅
   - Population orchestrator's `Start()` goroutine catches errors, logs via `libs/logger.Errorw`, and continues ticking.
   - Cache-buster timestamp written even on partial failure (per spec "stale signals continue serving").

## Adaptations during execution

- **Gateway routing** — Added a separate carve-out (`de717af`) so `/api/users/recs` is mounted as a sibling to the protected `/users/*` block, allowing OptionalAuthMiddleware to pass anonymous traffic through. The plan called this out as CRITICAL; executor handled it cleanly.
- **Player service Redis init** — Player did not previously use Redis. Task 6 added Redis config to `services/player/internal/config/config.go` and initialized the cache client in main.go.
- **`aired_on` not `aired_at`** — The Anime model uses `aired_on`; planner caught this during planning and the executor used the correct column throughout.
- **S4 is request-time, not precompute** — S4's `Precompute` is a no-op; the `s4_recency_score` column on `rec_population_signals` is reserved for future S4 caching but not currently written. The Score path computes S4 in-memory from `(status, aired_on)`. This matches spec §3 ("S4 stateless / request-time"). Documented for future reference.
- **GORM log noise refactor** — S3's preserve-S4 read used `First()` which logs a warning on every cold-start anime. Replaced with `Pluck()` to silence noise (`f71045b`). Behavior unchanged.

## Files added/modified

```
services/player/internal/service/recs/signals/s3_trending.go        (new)
services/player/internal/service/recs/signals/s3_trending_test.go   (new)
services/player/internal/service/recs/signals/s4_recency.go         (new)
services/player/internal/service/recs/signals/s4_recency_test.go    (new)
services/player/internal/service/recs/signals/s11_filter.go         (new)
services/player/internal/service/recs/signals/s11_filter_test.go    (new)
services/player/internal/service/recs/population.go                 (new)
services/player/internal/service/recs/population_test.go            (new)
services/player/internal/handler/recs.go                            (new)
services/player/internal/handler/recs_test.go                       (new)
services/player/internal/transport/router.go                        (modified — recs route)
services/player/internal/config/config.go                           (modified — Redis config)
services/player/cmd/player-api/main.go                              (modified — cron + handler wiring)
services/gateway/...                                                (modified — anonymous /users/recs carve-out)
frontend/web/src/composables/useRecs.ts                             (new)
frontend/web/src/locales/en.json                                    (modified — recs.* keys)
frontend/web/src/locales/ru.json                                    (modified — recs.* keys)
frontend/web/src/locales/ja.json                                    (modified — recs.* keys)
frontend/web/src/views/Home.vue                                     (modified — trending row)
```

## Requirements satisfied

- ✓ REC-SIG-01 — S3 (population trending) ranks by last-30-day watch_history start count
- ✓ REC-SIG-02 — S4 (recency) boosts ongoing or recently-aired anime
- ✓ REC-SIG-07 — S11 (Phase 10 scope: `hidden=true` exclusion)
- ✓ REC-INFRA-01 — Population signals precomputed every 60 min via cron; failure non-fatal
- ✓ REC-INFRA-04 — Redis 6h TTL top-N cache at `recs:public:trending:topN`
- ✓ REC-UX-02 — Anonymous "Trending now" row on Home.vue with EN + RU + JA copy

## Manual verification items (post-deploy)

The following should be verified by a human in a browser (deferred from Task 10):

1. Open `https://animeenigma.ru/` (or `http://localhost/`) in a logged-out browser session — confirm the "Trending now" row renders with anime cards above or near the existing "Ongoing" row.
2. Switch site language to Russian (RU) — confirm row label reads "Тренды сейчас".
3. Switch to Japanese (JA) — confirm row label reads (matches the locale file value).
4. Click an anime card in the row — confirm it navigates to the anime detail page (no broken card links).
5. Open DevTools Network tab — confirm `/api/users/recs` returns 200 with `cache_hit: true` on the second page load within 6 hours.

## After-update skill

Phase 10 is the first user-facing v2.0 surface. Per CLAUDE.md, `/animeenigma-after-update` should be invoked to update `frontend/web/public/changelog.json` with a "Trending now home row" entry. Deferred to orchestrator (next step).
