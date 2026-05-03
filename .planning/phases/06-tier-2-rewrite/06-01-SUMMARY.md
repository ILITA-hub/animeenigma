# Phase 6 — Plan 01 — Summary

**Completed (code):** 2026-05-03
**Status:** ✓ Implementation complete; ⏳ deploy + production verification pending

## One-liner

Replaced the naive Tier 2 `COUNT(*) GROUP BY` with weighted, exponentially
decayed, two-signal inference and a min-confidence floor. The resolver now
locks (language, watch_type) from a coarse signal weighted by
`duration_watched` × exp-decay (default 30-day half-life), picks the team
within that lock from a fine signal, and falls through to Tier 3 when total
weighted history is below the configurable confidence floor.

## What changed

| Layer | File | Change |
|---|---|---|
| Domain | `services/player/internal/domain/preference.go` | New types: `WeightedCoarse`, `WeightedFine`, `Tier2Lock`. Comment on `ComboCount` now points at the legacy `/api/users/preferences/global` endpoint only |
| Repo | `services/player/internal/repo/preference.go` | New `GetUserHistoryForTier2(ctx, userID, maxRows)` — bounded fetch ordered by `watched_at DESC`. **Removed** dead `GetUserGlobalFavorite` (was the old Tier 2 source) |
| Service (new) | `services/player/internal/service/tier2.go` | NEW — pure functions `AggregateTier2(rows, halfLifeDays, now, durationFloor) → (coarse, fine, total)` and `ChooseTier2Lock(coarse, fine, total, minConfidence) → *Tier2Lock` |
| Service | `services/player/internal/service/resolver.go` | Tier 2 block rewritten to consume `*domain.Tier2Lock` instead of `*domain.ComboCount`. VAL-02 boundary preserved (matches only inside locked language+watch_type) |
| Service | `services/player/internal/service/preference.go` | `Resolve()` now loads history → aggregates → applies floor → calls resolver. New constructor `NewPreferenceServiceWithTier2(...)` threads tunables. `Tier2Params` struct + `DefaultTier2Params()` defaults |
| Config | `services/player/internal/config/config.go` | New `Tier2Config{ HalfLifeDays, MinConfidence, MaxHistoryRows, DurationFloor }`; reads `TIER2_HALF_LIFE_DAYS` (default 30), `TIER2_MIN_CONFIDENCE` (default 1800 ≈ 30min effective), `TIER2_MAX_HISTORY_ROWS` (default 5000), `TIER2_DURATION_FLOOR` (default 60). New `getEnvFloat` helper |
| Wiring | `services/player/cmd/player-api/main.go` | Calls `NewPreferenceServiceWithTier2` so the env-driven config reaches the resolver |
| Metrics | `libs/metrics/watch.go` | New `Tier2ThinSignalSkipTotal{anon}` counter — incremented when total weighted history > 0 but < floor; resolver falls through to Tier 3 |
| Tests | `services/player/internal/service/tier2_test.go` | NEW — 11 unit tests covering: empty history, single-row no-decay, exact half-life decay, duration floor, dimension-skip, two-signal independence, binary decay (60d = 0.25×), confidence floor (below + above), coarse-lock-with-cross-bucket-fine, latency check at 5k rows |
| Tests | `services/player/internal/service/resolver_test.go` | Tier 2 cases rewritten to use `*Tier2Lock`. New case "T2: nil lock (thin signal) → falls straight to Tier 3" verifies the floor-decline path |

## Test results

```
ok  github.com/ILITA-hub/animeenigma/services/player/internal/handler    0.021s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/repo       0.042s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/service    0.032s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/transport  (cached)
```

The 5000-row latency check passed in well under 50ms — the resolver budget for
p95 stays comfortably intact.

## Success criteria status

| SC | Status | Evidence |
|---|---|---|
| 1. Aggregation weights every row by `WatchHistory.duration_watched` instead of treating each row as 1 vote | ✓ | `AggregateTier2` multiplies `max(duration_watched, durationFloor)` by the decay factor (`tier2.go:47-66`) |
| 2. Exponential time decay with tunable half-life (default 30 days) | ✓ | `decayRate = ln(2) / (halfLifeDays * 86400)`; `TestAggregateTier2_OneHalfLifeReducesWeightByHalf` verifies; env var `TIER2_HALF_LIFE_DAYS` |
| 3. Two distinct signals — coarse `(language, watch_type)` for lock + fine `(translation_title)` for team within lock | ✓ | `AggregateTier2` returns separate maps; `ChooseTier2Lock` picks top coarse, then heaviest fine inside locked bucket; `TestChooseTier2Lock_AboveFloor_PicksTopCoarse_AndTopFineInLock` verifies cross-bucket filtering |
| 4. Below-floor weighted history → Tier 2 declines, falls to Tier 3 | ✓ | `ChooseTier2Lock` returns nil when `total < minConfidence`; service emits `tier2_thin_signal_skip_total` increment; resolver path verified by "T2: nil lock (thin signal)" test |
| 5. VAL-02 boundary preserved — never crosses language or dub/sub | ✓ | Resolver Tier 2 match filters on `a.Language == lockLang && a.WatchType == lockType`; existing boundary tests "B: never cross language" and "B: never cross type" still pass |
| 6. Resolver p95 latency < 50ms | ✓ | `TestAggregateTier2_ManyRowsLatencyCheck` asserts < 50ms for 5000-row aggregation; production uses MaxHistoryRows=5000 cap. Single SQL fetch on (user_id) index + in-Go aggregation |

## Tunables (default → env var)

| Env var | Default | Purpose |
|---|---|---|
| `TIER2_HALF_LIFE_DAYS` | 30.0 | Exponential decay half-life. Older rows lose half their weight per N days |
| `TIER2_MIN_CONFIDENCE` | 1800.0 | Total weighted history below this → fall through to Tier 3 (~30min effective duration at age 0) |
| `TIER2_MAX_HISTORY_ROWS` | 5000 | Safety cap on rows fetched per resolve — bounds latency for power users |
| `TIER2_DURATION_FLOOR` | 60 | Floor on a single row's duration in seconds — rescues legacy rows where `duration_watched=0` from being silently zeroed by the new weighting |

## What's not in this phase (out of scope, intentional)

- **Anonymous Tier 2** — Phase 7 lives in localStorage; Phase 6 service skips Tier 2 entirely when `userID == ""`.
- **Advanced Settings exposure** — viewing raw weights / overriding the lock / resetting learned prefs is Phase 7 scope (B-05).
- **Materialized view / cached aggregate** — the in-Go aggregation meets the 50ms budget, so we skip the cache complexity per "don't add abstractions beyond what the task requires."
- **`combo_override` label-misalignment fix** (`language=unknown` for resolves of known anime — flagged in PROJECT.md baseline snapshot). The label is owned by the override handler, not the resolver; left as a Phase 7 cleanup item or backlog ticket.

## What's next

1. **Right now:** Commit Phase 6 (single commit: domain + repo + service + config + main + metrics + tests + plan summary).
2. **Wave 3 deploy:** `/animeenigma-after-update` redeploys `player` (no web changes in Phase 6).
3. **Production verification:** After deploy, watch `tier2_thin_signal_skip_total` start counting. After ≥7d of post-deploy traffic, recompute the override-rate baseline (`PROJECT.md § Baseline override rate`) for Phase 7 comparison.
4. **Phase 7 dependency:** Advanced Settings will read `Tier2Lock` data and let users override; `prefs_version` cookie will need to bump on per-anime preference saves.
