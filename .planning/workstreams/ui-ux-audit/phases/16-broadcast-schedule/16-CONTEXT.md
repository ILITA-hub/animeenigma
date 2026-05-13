# Phase 16: Broadcast schedule view - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, Phoenix new feature — but core view already exists)

<domain>
## Phase Boundary

The `/schedule` route already exists (`frontend/web/src/views/Schedule.vue`) and renders a weekly schedule grouped by day. Phase 16 closes the gap by:

- **UX-32** — Add a "На этой неделе" / "This Week" horizontal-scroll row to Home above the Ongoing column block, showing today's + tomorrow's airing episodes by hour. Tier E #4.

Out of scope: the `/schedule` view itself is already shipped (likely from a prior milestone). Don't refactor it. The Home row reuses the same `fetchSchedule` composable to keep the data path consistent.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**Home row:**
- New component: `frontend/web/src/components/home/ThisWeekRow.vue`. Horizontal scroll, same card pattern as the trending row.
- Card content: poster (small), localized title (truncated), episode number badge, hour-of-day label ("18:00").
- Data: reuse `useAnime().fetchSchedule()` (already powers Schedule.vue). Filter to "today" + "tomorrow" client-side. Sort by `next_episode_at` ascending.
- Empty state: hide row entirely if no titles airing today/tomorrow.
- Mount in Home.vue ABOVE the Continue-Watching row (which itself sits above trending). New row hierarchy on Home: SystemBanner > This Week > Continue-Watching (auth-gated) > Trending > Three columns > Top10 > Announcements.
- Click behavior: route to `/anime/{id}?episode={episodes_aired + 1}` (deep-link to the airing episode — same pattern as Phase 9 UX-17).

**i18n keys:**
- `home.thisWeek` (row label): EN "This Week" / RU "На этой неделе" / JA "今週"
- `home.thisWeek.today` / `.tomorrow` (small label per item): EN "Today" / "Tomorrow" / RU "Сегодня" / "Завтра" / JA "今日" / "明日"
- (3 keys × 3 locales = 9 entries)

### Locked from ROADMAP

- Phase 16 depends on Phase 8 (Continue-Watching pattern) — complete.
- Phase 16 depends on Phase 11 (status-banner infra) — complete (mostly relevant for the Home top-section spacing pattern).
- No backend work — schedule API exists.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `frontend/web/src/composables/useAnime.ts` — `fetchSchedule()` already returns the airing anime list. Use as-is.
- `frontend/web/src/views/Schedule.vue` — shape of the data is known: each item has `id`, `name`, `name_ru`, `name_jp`, `poster_url`, `next_episode_at`, `episodes_aired`.
- `frontend/web/src/components/home/ContinueWatchingRow.vue` — pattern for a new Home row component.

### Established Patterns

- Self-gating component: render nothing when `items.length === 0`.
- Click deep-link: `/anime/{id}?episode={N+1}` (Phase 9 UX-17 pattern).

### Integration Points

- Home.vue mount point: above ContinueWatchingRow (which mounts at the very top of the main content area).

</code_context>

<specifics>
## Specific Ideas

- The Home row shows TODAY + TOMORROW only — 2-day window. Schedule.vue keeps showing the full week. Different surfaces, different scopes.
- Hour label format: `HH:MM` (24-hour) consistently across locales. Internationalized via `Intl.DateTimeFormat`.
- "Today" / "Tomorrow" badge above each card (small text-cyan-400 chip) to disambiguate when scrolling.

</specifics>

<deferred>
## Deferred Ideas

- Calendar grid view on /schedule — Phase 20 polish if needed.
- Notification reminders for tracked airing anime — separate notification engine feature (see project memory `project_notifications_engine.md`).
- Time zone configuration UI — defer; use browser's local time zone for v0.1.

</deferred>
