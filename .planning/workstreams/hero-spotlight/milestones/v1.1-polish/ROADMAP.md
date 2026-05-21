---
workstream: hero-spotlight
milestone: v1.1-polish
status: planned
created: 2026-05-21
source_proposal: ./REFACTOR-PROPOSAL.md
parent_uat: ../v1.0-phases/03-dynamic-cards-migration/03-UAT.md
total_phases: 10
total_requirements: 30
direction: cinematic-backdrops-distinct-templates
---

# v1.1-polish ROADMAP — hero-spotlight visual refactor

> One-line: take the functional 9-card spotlight shipped in v1.0 and give
> each card a distinct visual identity, while fixing the rapid-click
> transition bug surfaced during Phase 03 UAT.

## Why this milestone

Phase 03 UAT (2026-05-21) flagged all 9 cards as visually below bar.
Cross-cutting issues:

1. 4 cards share a cloned poster-left/text-right layout → no variety.
2. No backdrop imagery → hero feels empty.
3. No card-type theming → everything uses the same cyan accent.
4. CTAs all identical → no hierarchy.
5. Rapid carousel clicks leave the article stuck mid-transition (opacity:0 / blank screen).

This milestone addresses all five, card-by-card.

## Phases

| # | Phase | Files Touched | Goal |
|---|---|---|---|
| 01 | Foundation | tokens.ts, SpotlightBackdrop.vue, SpotlightIcon.vue, main.css, HeroSpotlightBlock.vue, CarouselControls.vue | Shared backdrop + tokens + transition lock + labeled-pill dots. Blocking dep for 02-10. |
| 02 | AnimeOfDayCard refactor | cards/AnimeOfDayCard.vue (+ spec) | Cinematic backdrop, larger poster, drop disabled CTA, color-coded genre tags. |
| 03 | RandomTailCard refactor | cards/RandomTailCard.vue (+ spec) | Purple "discovery" accent, shuffle icon, rotating taglines. Distinct from AnimeOfDay. |
| 04 | PersonalPickCard refactor | cards/PersonalPickCard.vue (+ spec) | Featured + 2 secondary layout. Fixes truncated-title bug. Username in personal title. |
| 05 | NowWatchingCard refactor | cards/NowWatchingCard.vue (+ spec) | Larger thumbs (56×84), hashed avatar circles, animated cyan→green mesh bg. |
| 06 | TelegramNewsCard refactor | cards/TelegramNewsCard.vue (+ spec), services/catalog/.../telegram_news.go (pass-through image url) | Telegram blue branding, post thumbnails when available, channel attribution. |
| 07 | LatestNewsCard refactor | cards/LatestNewsCard.vue (+ spec) | Type-icon per entry (feat/fix/perf), type pill, relative dates, drop fragile title-split regex. |
| 08 | PlatformStatsCard refactor | cards/PlatformStatsCard.vue (+ spec), services/catalog/.../platform_stats.go (extend with previous_value + series[7]) | Hero stat + 2×2 micro-grid + sparkline + delta chip. Fills N=1 dead space. |
| 09 | NotTimeYetCard refactor | cards/NotTimeYetCard.vue (+ spec), services/catalog/.../not_time_yet.go (pass-through added_at) | Amber clock accent, status pill, "Last added X ago", direct-to-watch CTA. |
| 10 | ContinueWatchingNewCard refactor | cards/ContinueWatchingNewCard.vue (+ spec) | Hero ribbon ("НОВАЯ СЕРИЯ N!"), 2-row episode stack, deep-link CTA to `/watch?episode=N`. |

## Dependencies

Phase 01 blocks 02-10 (all card phases consume new tokens + SpotlightBackdrop).
Phases 02-10 are mutually independent — can ship in any order or parallel
once Phase 01 lands.

```
01 ──┬─→ 02 (AnimeOfDay)
     ├─→ 03 (RandomTail)
     ├─→ 04 (PersonalPick)
     ├─→ 05 (NowWatching)
     ├─→ 06 (TelegramNews)        [+ small backend touch]
     ├─→ 07 (LatestNews)
     ├─→ 08 (PlatformStats)       [+ backend extension]
     ├─→ 09 (NotTimeYet)          [+ backend pass-through]
     └─→ 10 (ContinueWatchingNew)
```

## Success criteria (milestone-level)

1. All 9 cards visually distinct — a user cycling through the carousel can
   identify which card type they're on within 1 second by visual cues alone
   (color, layout, iconography).
2. Hero block fills its ~520k px² area on every card — no dead space.
3. Rapid clicks (10× ArrowRight in <1s) settle to exactly one card per
   ~500ms transition window, no opacity-0 blank states.
4. All existing Phase 03 verification gates remain green (`spotlight-full.spec.ts`
   passes, `spotlight-phase3-smoke.sh` passes, 10/10 Phase 03 truths still hold).
5. UI-REVIEW score ≥ 20/24 (current: 13/24 per `03-UI-REVIEW.md`).

## Out of scope (deferred to v1.2)

- Slide-order personalization (originally v1.1 candidate).
- Opt-out toggles per card type.
- Editorial admin-curated card.
- WebSocket-driven now_watching (Phase 05 still uses 10s Redis polling).
- Feature flag cleanup (`SPOTLIGHT_ENABLED` / `VITE_HERO_SPOTLIGHT_ENABLED`).

## Metrics — milestone roll-up

| Metric | Value |
|---|---|
| **UXΔ (signed)** | +4 (Better) |
| **CDI** | 0.06 * 89 (Spread × Shift = 0.06; Effort Fibonacci = 89 across 10 phases) |
| **MVQ** | Phoenix 88%/85% (visual rebirth; high match to user's "refactor each" directive; slop-resistance high because each phase has explicit spec assertions and an existing 9-card baseline to regress against) |
