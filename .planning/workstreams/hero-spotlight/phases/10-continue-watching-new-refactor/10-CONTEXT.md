# Phase 10: ContinueWatchingNewCard refactor - Context

**Gathered:** 2026-05-22
**Mode:** Auto-generated from approved REFACTOR-PROPOSAL.md

<domain>
Transform the tiny corner badge into a hero treatment:
- Hero ribbon across poster top: "🎬 НОВАЯ СЕРИЯ {n}!"
- 2-row episode meta: "Вы посмотрели до серии N" (subdued) + "Новая серия N" (purple accent, large).
- Deep-link CTA `/anime/{id}/watch?episode={n}` (jump straight to new episode).
- Backdrop poster-blur + purple/30 overlay.

In scope: `cards/ContinueWatchingNewCard.vue` + spec, i18n updates.
</domain>

<decisions>
- Hero ribbon spans `inset-x-0 top-0` of the poster — replaces the corner badge for visual emphasis.
- Deep-link respects intent: user knows about this anime AND we know which episode is new; remove the click-to-detail-then-click-to-watch friction.
- Pre-flight check: confirm `Watch.vue` honors `?episode=N` route query at mount. If absent, add a one-line `route.query.episode` handler.
</decisions>

<code_context>
- `SpotlightIcon name="play"` exists from Phase 01.
- `.cta-hero[data-accent="purple"]` exists from Phase 01.
- `ContinueWatchingNewData.last_watched_episode` + `new_episode_number` already in TS type + Go struct.
- Existing `Watch.vue` at `frontend/web/src/views/Watch.vue` — grep for `route.query.episode` to confirm support.
</code_context>

<specifics>
- New i18n keys: `lastWatched: "Вы посмотрели до серии {n}"`, `newEpisodeLine: "Новая серия {n}"`, `resumeCtaWithEp: "Смотреть серию {n} →"` (3 locales).
- Ribbon styling: `bg-gradient-to-r from-purple-600 to-fuchsia-500 text-white text-xs font-bold uppercase tracking-wider shadow-lg`.
</specifics>

<deferred>
- Thumbnail of the new episode's first frame (would need episode-thumbnail backend data — v1.2).
</deferred>
