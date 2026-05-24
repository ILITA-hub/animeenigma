# Phase 01: Foundation - Context

**Gathered:** 2026-05-21
**Status:** Ready for planning
**Mode:** Auto-generated from approved REFACTOR-PROPOSAL.md (autonomous mode)

<domain>
## Phase Boundary

Ship the shared primitives every card phase will consume:

1. `cardTokens.ts` — per-card-type token map (accent, kickerKey, icon).
2. `<SpotlightBackdrop>` — shared backdrop component, 2 variants
   (`poster-blur`, `gradient-mesh`).
3. `<SpotlightIcon>` — inline-SVG sprite, 9 named icons (telegram,
   sparkles, chart, pulse, clock, play, shuffle, wrench, lightning).
4. 3-tier CTA classes in main.css (`cta-hero`, `cta-card`, `cta-text`)
   with `data-accent` variants.
5. Transition lock — fixes the rapid-click blank-card bug surfaced
   during Phase 03 UAT (Vue `<transition mode="out-in">` gets stuck in
   `leave-active + enter-from` → opacity:0 when navigation fires
   mid-fade). Add `isTransitioning` ref + `@before-leave`/`@after-enter`
   listeners + 600ms watchdog.
6. Carousel chrome polish — labeled-pill dots with per-card-type icons,
   hover tooltips, active-state accent color.

In scope:
- All primitives are shared infrastructure for cards 02-10.
- No card behavior changes in this phase — additive only.
- Existing Phase 03 spec tests + `spotlight-full.spec.ts` MUST stay green.

Out of scope (deferred to v1.2):
- Slide-order personalization.
- Opt-out toggles.
- Editorial card.
- WebSocket now_watching.
- Feature flag cleanup.

</domain>

<decisions>
## Implementation Decisions

### Cinematic backdrop direction
**Decision:** Two variants — `poster-blur` (anime cards) and `gradient-mesh`
(news/stats/social cards). Backdrop sits in an absolute-positioned layer
behind the existing card content, controlled via slots.
**Why:** User signed off on "cinematic backdrop + distinct templates"
direction in proposal-review questions (2026-05-21).
**Reference:** REFACTOR-PROPOSAL.md §1.

### Backdrop opacity = 0.4
**Decision:** Start at 0.4 for poster-blur. A/B at 0.3 / 0.4 / 0.5 during
Phase 02 visual smoke; lock in then.
**Why:** Foreground text legibility vs background presence trade-off;
0.4 is the safe midpoint.

### Transition lock with 600ms watchdog
**Decision:** Use `isTransitioning: Ref<boolean>` gated by Vue transition
hooks. Add a `setTimeout` watchdog that force-resets the flag after 600ms
in case `@after-enter` never fires (defensive).
**Why:** Phase 03 UAT inspection found rapid clicks leave the article in
`spotlight-fade-leave-active + spotlight-fade-enter-from` simultaneously,
opacity:0. Lock prevents this race; watchdog prevents permanent deadlock.

### Token map enforced by parity test
**Decision:** Vitest test iterates the `SpotlightCardType` union and
asserts every member has a `cardTokens` entry.
**Why:** Adding a 10th card type later (v1.2 editorial card) shouldn't
silently skip a token; the parity test trips immediately.

### Dot polish includes labels + tooltips
**Decision:** Replace 6 grey circles with pill buttons showing the
per-card icon and a hover tooltip with the kicker text. Active dot uses
the card's accent color.
**Why:** Anonymized grey dots give the user no signal about what they're
about to navigate to. The 7-not-9 dot UX surprise from UAT was partly
driven by this — users couldn't tell which card types were present.

### CTA hierarchy (3 sizes, accent variants via data-attr)
**Decision:** `cta-hero` (oversized, anime cards), `cta-card` (default,
internal CTAs), `cta-text` (link-style for secondary actions). Accent
variants via `data-accent="purple|amber|green|sky|teal"`.
**Why:** Today's `btn btn-primary text-sm md:text-base` is uniform across
all 9 cards — no hierarchy. Each card needs to signal its action priority
relative to siblings.

</decisions>

<code_context>
## Existing Code Insights

- `HeroSpotlightBlock.vue` (`frontend/web/src/components/home/spotlight/`) —
  hosts the carousel state machine. Current `<transition>` has no lock.
  Need to add `isTransitioning` ref + hooks; gate `next/prev/goTo` calls.
- `CarouselControls.vue` (same dir) — renders the 6 grey dots today.
  Replace dot template with labeled-pill buttons referencing cardTokens.
- `frontend/web/src/styles/main.css` — currently has `spotlight-fade`
  CSS rules hardcoded to `0.4s`. Refactor to CSS var `--spotlight-fade-ms`
  so the JS lock window matches the actual fade duration.
- `frontend/web/src/types/spotlight.ts` — exports `SpotlightCardType` union
  used by every card. cardTokens map keys must match this union.
- `@vueuse/core` already in deps — used for `useMediaQuery` in
  HeroSpotlightBlock and PersonalPickCard. Same lib for any new utilities.
- `vue-i18n` already wired — tokens map references i18n keys (no need to
  pull in another i18n lib).

</code_context>

<specifics>
## Specific Ideas

- **No icon library dependency.** All 9 icons rendered as inline SVGs
  inside `<SpotlightIcon>` so we don't add lucide/heroicons. Each icon
  is ~200-400 bytes; total sprite < 3 KB gzipped.
- **Backdrop has zero JS.** Pure CSS via Tailwind utility composition +
  a few custom utility classes in main.css for the mesh gradients.
- **Watchdog uses setTimeout, not requestAnimationFrame.** 600ms is too
  long for rAF and we want a deterministic time-based fallback.

</specifics>

<deferred>
## Deferred Ideas

- **Sparkline + DeltaChip** components are Phase 08's responsibility.
  Not added in Phase 01.
- **Per-genre color map** is Phase 02's responsibility (lives in
  `cardTokens.anime_of_day.genreColors`). Phase 01 just provides the
  cardTokens infrastructure.
- **Telegram SVG channel attribution** (e.g. @anime_enigma string) is
  Phase 06.

</deferred>
