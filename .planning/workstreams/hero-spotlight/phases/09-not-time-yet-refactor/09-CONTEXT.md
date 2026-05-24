# Phase 09: NotTimeYetCard refactor - Context

**Gathered:** 2026-05-22
**Mode:** Auto-generated from approved REFACTOR-PROPOSAL.md

<domain>
Differentiate NotTimeYetCard from AnimeOfDayCard:
- `SpotlightBackdrop variant="poster-blur"` + amber/30 overlay (nostalgic).
- Prominent clock icon header.
- Status pill (yellow "В планах" / slate "Отложено").
- "Last added X ago" timestamp.
- Direct-to-watch CTA (`/watch` not `/anime/{id}`).
- Backend pass-through: `added_at` from `anime_list.updated_at`.

In scope: `cards/NotTimeYetCard.vue` + spec, `services/catalog/internal/service/spotlight/cards/not_time_yet.go` (struct extension), i18n updates.
</domain>

<decisions>
- Amber accent reads as "warm reminder / nostalgia" — distinct from AnimeOfDay's cyan and ContinueWatchingNew's purple.
- Status pill colors: planned → `bg-yellow-500/20 text-yellow-200`, postponed → `bg-slate-500/20 text-slate-300`.
- Direct-to-watch CTA respects intent: user already bookmarked the anime; deep-link skips the detail page.
- `added_at` already available in `PlayerClient.FetchListByStatuses` response — just plumb it through to NotTimeYetData.
</decisions>

<code_context>
- `SpotlightIcon name="clock"` exists from Phase 01.
- `.cta-hero[data-accent="amber"]` exists from Phase 01.
- `formatAgo` helper from Phase 07 — extract to `frontend/web/src/utils/time.ts` so both NotTimeYet and LatestNews share. (If not extracted by Phase 07, this phase does the extract.)
- Backend `NotTimeYetData` already has `Anime`, `Status` — add `AddedAt *time.Time`.
- `InternalListItem` from player service already includes `updated_at`.
</code_context>

<specifics>
- New i18n keys: `statusPlanned` / `statusPostponed` / `addedAt` with `{ago}` interpolation.
- CTA href: `/anime/{data.anime.id}/watch` (verify Watch.vue handles no-episode-param defaulting to ep 1).
</specifics>

<deferred>
- "Mark as watched" inline CTA (would need write-path; out of v1.1 scope).
</deferred>
