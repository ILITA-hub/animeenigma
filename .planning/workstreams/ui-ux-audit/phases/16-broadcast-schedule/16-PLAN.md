# Phase 16 Plan: Broadcast schedule view — Home "This Week" row

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Scope: 1 new component + Home.vue mount + 3 locale files. The `/schedule` route view already exists (`frontend/web/src/views/Schedule.vue`); no backend work. Closes UX-32.

## Tasks

### Component

- [ ] Create `frontend/web/src/components/home/ThisWeekRow.vue`:
  - `<script setup>`: import `useAnime`, fetch schedule on mount.
  - Filter the schedule client-side to items with `next_episode_at` falling on today OR tomorrow (local time zone).
  - Sort by `next_episode_at` ascending.
  - Render row with horizontal scroll. Each card: poster, day chip ("Today"/"Tomorrow"), title (truncated, h3 for heading order), episode badge (`Серия N+1`), time badge (`18:00`).
  - Self-gate: `v-if="items.length > 0"`.
  - Click: `:to="`/anime/${item.id}?episode=${(item.episodes_aired || 0) + 1}`"`.

### Mount in Home.vue

- [ ] Import `<ThisWeekRow />` and mount ABOVE `<ContinueWatchingRow />` near the top of Home's main content. Component is self-gating, so unconditional mount is fine.

### i18n (en/ru/ja)

- [ ] Add to each locale (3 keys × 3 locales = 9 entries):
  - `home.thisWeek` (row label):
    - EN "This Week" / RU "На этой неделе" / JA "今週"
  - `home.thisWeekToday`:
    - EN "Today" / RU "Сегодня" / JA "今日"
  - `home.thisWeekTomorrow`:
    - EN "Tomorrow" / RU "Завтра" / JA "明日"

### Verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` clean.
- [ ] `bash scripts/i18n-lint.sh` clean.
- [ ] `make redeploy-web` succeeds.
- [ ] grep `ThisWeekRow` in components/home/ + Home.vue confirms wiring (2+ matches).
- [ ] grep `home.thisWeek\|home.thisWeekToday\|home.thisWeekTomorrow` in en/ru/ja confirms 3 keys per locale.
- [ ] Manual: load `/`. If any anime has `next_episode_at` for today or tomorrow, row renders. Click a card → routes to `/anime/{id}?episode={N+1}`.

## Files touched

```
frontend/web/src/components/home/ThisWeekRow.vue     (new)
frontend/web/src/views/Home.vue                       (mount)
frontend/web/src/locales/en.json                      (+3 keys)
frontend/web/src/locales/ru.json                      (+3 keys)
frontend/web/src/locales/ja.json                      (+3 keys)
.planning/workstreams/ui-ux-audit/phases/16-broadcast-schedule/
  16-CONTEXT.md
  16-PLAN.md
  16-SUMMARY.md       (written at execute end)
  16-VERIFICATION.md  (written at execute end)
```

## Closes

| Req | Surface | Mechanism |
|---|---|---|
| UX-32 | Home | ThisWeekRow component sourced from existing fetchSchedule, today+tomorrow filter, deep-link to episode |
