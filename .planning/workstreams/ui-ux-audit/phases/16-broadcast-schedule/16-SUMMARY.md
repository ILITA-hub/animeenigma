---
phase: 16
plan: 1
subsystem: frontend/web
tags: [home, schedule, ui, i18n]
requirements: [UX-32]
dependency-graph:
  requires: [phase-8-continue-watching, phase-11-status-banner]
  provides: [home-this-week-row]
  affects: [Home.vue layout]
tech-stack:
  added: []
  patterns: [self-gating-row, deep-link-to-episode, client-side-day-filter]
key-files:
  created:
    - frontend/web/src/components/home/ThisWeekRow.vue
  modified:
    - frontend/web/src/views/Home.vue
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
decisions:
  - Filter today+tomorrow in user's local time zone (Date#getDate/getMonth/getFullYear projects to local TZ)
  - Skip already-aired episodes for today so the time chip never reads "Today 14:00" when the wall clock is 18:00
  - 24-hour HH:MM format consistently across all 3 locales (matches Schedule.vue's pattern but uses browser local TZ instead of forced Europe/Moscow)
  - Self-gate via items.length > 0 — no error/empty state UI (component disappears, no degraded affordance)
  - Click deep-link to /anime/{id}?episode={episodes_aired + 1} (Phase 9 UX-17 pattern)
  - Reuse home.continueWatchingEpisode for the episode badge — same "Episode {n}" / "Серия {n}" / "第{n}話" formatting
metrics:
  duration: ~12 min
  completed: 2026-05-13
  tasks_completed: 3
  files_touched: 5
---

# Phase 16 Plan 1: Broadcast schedule view — Home "This Week" row Summary

Adds a today + tomorrow horizontal-scroll row at the top of the main Home content (directly above Continue-Watching), sourced from the existing `fetchSchedule` endpoint that already powers `/schedule`. Closes UX-32.

## What shipped

1. **`ThisWeekRow.vue`** (new, 158 lines) — reuses `useAnime().fetchSchedule()`, filters client-side to items whose `next_episode_at` falls on today or tomorrow in the user's local time zone, skips today's items whose airtime has already passed, sorts ascending by airtime. Renders horizontal-scroll cards with poster, Today/Tomorrow day chip (cyan when today), episode badge (`Episode N+1`), and 24h time label. Self-gates on `items.length > 0`. Click routes to `/anime/{id}?episode={episodes_aired + 1}`.

2. **`Home.vue` mount** — imports `ThisWeekRow` and mounts it directly above `<ContinueWatchingRow />` (after the Three-Columns Layout block but before the in-progress row). Component is self-gating so unconditional mount is safe.

3. **i18n** — 3 keys × 3 locales = 9 entries:
   - `home.thisWeek` — EN "This Week" / RU "На этой неделе" / JA "今週"
   - `home.thisWeekToday` — EN "Today" / RU "Сегодня" / JA "今日"
   - `home.thisWeekTomorrow` — EN "Tomorrow" / RU "Завтра" / JA "明日"

## Commits

| Commit | Message |
|---|---|
| `25a7a34` | feat(16): ThisWeekRow component |
| `f2f0b59` | feat(16): mount ThisWeekRow in Home above Continue-Watching |
| `a48a0d0` | feat(16): home.thisWeek i18n keys |

## Decisions Made

- **Local time zone, not Europe/Moscow.** `Schedule.vue` forces `Europe/Moscow` for the `formatTime` output. For the Home row we want the user's actual wall-clock hour so "Today 18:00" maps to *their* 18:00. This is the only behavioral divergence from the `/schedule` view's time rendering.
- **Skip already-aired today entries.** When the user opens Home at 21:00 and an episode aired at 14:00 today, it's no longer "upcoming". Filter drops it. Tomorrow entries are never filtered by hour (the entire day is in the future).
- **Reuse `home.continueWatchingEpisode` for the episode badge.** Same "Episode {n}" / "Серия {n}" / "第{n}話" formatting as the in-progress row directly below — visually consistent, no new key needed.
- **No empty/loading state UI.** Component renders nothing when the schedule fetch fails or yields no today/tomorrow entries. This is a discovery row, not a primary affordance — failing silently is preferable to a degraded skeleton.

## Deviations from Plan

None — plan executed exactly as written. The three commits map 1:1 to the three plan tasks (Component / Mount / i18n).

## Verification

All gates passed — see `16-VERIFICATION.md`.

## Self-Check: PASSED

- `frontend/web/src/components/home/ThisWeekRow.vue` exists (FOUND)
- `frontend/web/src/views/Home.vue` modified — `<ThisWeekRow />` mount + import (FOUND via grep, 2 matches)
- `frontend/web/src/locales/{en,ru,ja}.json` updated — 3 keys per locale (FOUND via grep)
- Commit `25a7a34` (feat(16): ThisWeekRow component) — exists in `git log`
- Commit `f2f0b59` (feat(16): mount ThisWeekRow in Home) — exists in `git log`
- Commit `a48a0d0` (feat(16): home.thisWeek i18n keys) — exists in `git log`
- `bunx vue-tsc --noEmit` — exit 0
- `bash scripts/i18n-lint.sh` — 0 missing keys, 0 syntax errors
- `make redeploy-web` — succeeded, new bundle `index-DusZ-b9O.js` shipped
