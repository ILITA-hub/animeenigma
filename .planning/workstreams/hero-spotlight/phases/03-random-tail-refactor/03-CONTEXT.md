# Phase 03: RandomTailCard refactor - Context

**Gathered:** 2026-05-22
**Status:** Ready for planning
**Mode:** Auto-generated from approved REFACTOR-PROPOSAL.md

<domain>
## Phase Boundary

Make RandomTailCard distinct from AnimeOfDayCard via:
- Purple "discovery" accent (overlay on top of poster-blur backdrop).
- `<SpotlightIcon name="shuffle">` in the header.
- Rotating taglines (4 candidates per locale, randomized at mount).
- Mount-time shuffle-deck animation, gated on `prefers-reduced-motion`.
- Purple `cta-hero` (`data-accent="purple"`).

In scope: `cards/RandomTailCard.vue` + its spec, i18n updates (en/ru/ja), CSS keyframes in main.css.
</domain>

<decisions>
- Purple secondary overlay layered on top of `SpotlightBackdrop variant="poster-blur"` to distinguish from AnimeOfDay's pure cyan.
- 4 taglines per locale; randomized on each mount (so two RandomTail views in a session don't read identically).
- Shuffle-deck animation: 5 cards fanning, 800ms cubic-bezier, gated on reduced-motion. Skipped entirely when matchMedia reports reduce.
</decisions>

<code_context>
- AnimeOfDayCard shipped in Phase 02 with cinematic backdrop; copy its `relative + z-10` structure.
- `SpotlightIcon name="shuffle"` exists from Phase 01.
- `.cta-hero[data-accent="purple"]` exists from Phase 01.
- i18n files: `frontend/web/src/locales/{en,ru,ja}.json` under `spotlight.randomTail.*`.
</code_context>

<specifics>
- Taglines i18n key: `spotlight.randomTail.taglines: string[]` (array).
- Shuffle animation classes go in `main.css` under `/* Spotlight RandomTail */` section.
- Reduced-motion check: `useMediaQuery('(prefers-reduced-motion: reduce)')` from @vueuse/core (already used elsewhere).
</specifics>

<deferred>
- Shared discovery-style helpers (if v1.2 adds more "discovery" cards).
</deferred>
