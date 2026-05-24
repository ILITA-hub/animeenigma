# Phase 02: AnimeOfDayCard refactor - Context

**Gathered:** 2026-05-22
**Status:** Ready for planning
**Mode:** Auto-generated from approved REFACTOR-PROPOSAL.md (autonomous mode)

<domain>
## Phase Boundary

Give AnimeOfDayCard a cinematic feel using the primitives shipped in
Phase 01:
- Blurred poster backdrop via `<SpotlightBackdrop variant="poster-blur">`.
- Larger foreground poster (`w-32 md:w-44 lg:w-56`).
- Drop the disabled "Add to list" CTA — single oversized `cta-hero`.
- Color-coded genre tags via `cardTokens.anime_of_day.genreColors`.
- Score badge moves from poster overlay to a meta-row pill.

In scope: `frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.vue` + its spec.

Out of scope: anything outside this card.

</domain>

<decisions>
- Backdrop variant: `poster-blur` (anime card direction per proposal §1).
- Drop disabled "Add to list" CTA entirely until wired (proposal direction; reduces visual noise).
- Genre color map keyed by genre.id; fallback to `bg-white/10` for unmapped IDs (10-15 common genres mapped).
</decisions>

<code_context>
- `cardTokens.anime_of_day` already exists from Phase 01 with `accent: 'cyan'`, `kickerKey`, `icon: 'sparkles'`.
- `SpotlightBackdrop.vue` exists; takes `variant` + `posterUrl` props.
- `SpotlightIcon.vue` exists; supports name="sparkles", "play", etc.
- `.cta-hero` class exists in main.css (default cyan; `data-accent` variants for other colors).
- Tailwind genre colors (red/yellow/pink/purple/etc.) — use `bg-{color}-500/20 text-{color}-200` pattern consistently.
</code_context>

<specifics>
- Score badge styling: meta-row pill, `bg-yellow-500/20 text-yellow-200`, includes the star SVG inline (keep existing inline SVG; don't move to SpotlightIcon).
- Cinematic backdrop opacity: 0.4 as shipped in SpotlightBackdrop (Phase 01 default).
</specifics>

<deferred>
- Per-genre color map for OTHER cards (lives in this phase's tokens for now; can be hoisted to shared utility if v1.2 needs cross-card consistency).
</deferred>
