---
workstream: hero-spotlight
milestone: v1.1-polish
status: planned
created: 2026-05-21
source_proposal: ./milestones/v1.1-polish/REFACTOR-PROPOSAL.md
parent_uat: ./milestones/v1.0-phases/03-dynamic-cards-migration/03-UAT.md
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

## Phase Summary

- [ ] **Phase 01: Foundation** — Shared backdrop + tokens + transition lock + labeled-pill dots
- [ ] **Phase 02: AnimeOfDayCard refactor** — Cinematic backdrop, larger poster, drop disabled CTA
- [ ] **Phase 03: RandomTailCard refactor** — Purple discovery accent, shuffle icon, rotating taglines
- [ ] **Phase 04: PersonalPickCard refactor** — Featured + secondary layout, username personalization
- [ ] **Phase 05: NowWatchingCard refactor** — Larger thumbs, hashed avatars, animated mesh backdrop
- [ ] **Phase 06: TelegramNewsCard refactor** — Telegram branding, post thumbnails, channel attribution
- [ ] **Phase 07: LatestNewsCard refactor** — Type icons, type pills, relative dates
- [ ] **Phase 08: PlatformStatsCard refactor** — Hero stat + 2×2 micro-grid + sparkline + delta chip
- [ ] **Phase 09: NotTimeYetCard refactor** — Amber clock accent, status pill, direct-to-watch CTA
- [ ] **Phase 10: ContinueWatchingNewCard refactor** — Hero ribbon, episode meta hierarchy, deep-link CTA

## Dependencies

Phase 01 blocks 02-10 (all card phases consume new tokens + SpotlightBackdrop).
Phases 02-10 are mutually independent — can ship in any order or parallel
once Phase 01 lands.

---

## Phase Details

### Phase 01: Foundation

**Goal:** Ship the shared primitives every card phase will consume — `cardTokens` map, `<SpotlightBackdrop>` (2 variants), `<SpotlightIcon>` (9-icon sprite), 3-tier CTA classes, transition lock with watchdog, labeled-pill dots. Also fix the rapid-click blank-card bug surfaced during Phase 03 UAT inspection.

**Success Criteria:**
1. `cardTokens` map has exactly 9 entries (one per `SpotlightCardType`); parity test enforced.
2. `SpotlightBackdrop` renders both `poster-blur` and `gradient-mesh` variants without console errors.
3. 10 rapid ArrowRight keypresses produce ≤ `ceil(time/duration)` settled cards; zero `spotlight-fade-leave-active` stuck states.
4. Labeled-pill dots render with per-card-type icons and accent color on active state.
5. All existing Phase 03 spec tests + `spotlight-full.spec.ts` remain green.

**Plan:** `phases/01-foundation/01-PLAN.md`

### Phase 02: AnimeOfDayCard refactor

**Goal:** Give AnimeOfDayCard a cinematic feel — blurred poster backdrop, larger foreground poster, single oversized CTA (drop the dead disabled "Add to list" button), and color-coded genre tags.

**Success Criteria:**
1. `<SpotlightBackdrop variant="poster-blur">` renders with the anime's poster URL.
2. Zero `aria-disabled="true"` or `<button disabled>` elements remain.
3. Exactly one `.cta-hero` element rendered.
4. Genre tags use color classes from `cardTokens.anime_of_day.genreColors` map.
5. `AnimeOfDayCard.spec.ts` passes.

**Plan:** `phases/02-anime-of-day-refactor/02-PLAN.md`

### Phase 03: RandomTailCard refactor

**Goal:** Make RandomTailCard distinct from AnimeOfDayCard via a purple "discovery" accent, shuffle iconography, rotating taglines, and a mount-time shuffle-deck animation gated on reduced-motion.

**Success Criteria:**
1. `<SpotlightIcon name="shuffle">` rendered in the header.
2. CTA has `data-accent="purple"` attribute.
3. Shuffle-deck animation skipped when `prefers-reduced-motion: reduce`.
4. Tagline is one of the 4 candidates from i18n `spotlight.randomTail.taglines[]`.
5. `RandomTailCard.spec.ts` passes.

**Plan:** `phases/03-random-tail-refactor/03-PLAN.md`

### Phase 04: PersonalPickCard refactor

**Goal:** Replace the 3-equal-posters grid with a featured-pick (60% width) + 2 secondary picks (40% width) layout. Fix the truncated-title bug, add per-item reason copy, surface the username in the personalized title, and make the mobile "+ N more" link a proper full-width footer button.

**Success Criteria:**
1. Featured-pick container present with `aria-label` referencing the featured anime title.
2. Secondary picks count = `data.items.length - 1` (max 2).
3. Username appears in title when `data.source === 'personal'`.
4. Mobile (<768px): "+ N more →" rendered as `block w-full` button (not corner link).
5. `PersonalPickCard.spec.ts` passes.

**Plan:** `phases/04-personal-pick-refactor/04-PLAN.md`

### Phase 05: NowWatchingCard refactor

**Goal:** Make NowWatchingCard feel alive — bigger poster thumbs (56×84, 3.5× current), hashed avatar circles per user, animated cyan→green gradient backdrop, and a pulsing LIVE micro-element next to each avatar.

**Success Criteria:**
1. Each row poster sized `w-14 h-21` (56×84).
2. Avatar circle has deterministic color class (same username → same color across mounts).
3. `<SpotlightBackdrop variant="gradient-mesh" accent="green">` rendered.
4. Pulsing LIVE dot rendered next to avatar (not text "LIVE" on right).
5. `NowWatchingCard.spec.ts` passes.

**Plan:** `phases/05-now-watching-refactor/05-PLAN.md`

### Phase 06: TelegramNewsCard refactor

**Goal:** Give TelegramNewsCard a clear Telegram identity (logo + brand-blue accent) and surface post thumbnails when available. Backend touch: small pass-through change so the existing `news:telegram` cache delivers any image URL it already holds.

**Success Criteria:**
1. `<SpotlightIcon name="telegram">` rendered with `aria-label="Telegram"`.
2. When `post.image_url` is present, thumbnail `<img>` rendered; falls back to text-only otherwise.
3. External link uses `target="_blank" rel="noopener noreferrer"` (T-03-18 pin held).
4. `<SpotlightBackdrop variant="gradient-mesh" accent="sky">` rendered.
5. Backend resolver test confirms `ImageURL` flows through when cache provides it.

**Plan:** `phases/06-telegram-news-refactor/06-PLAN.md`

### Phase 07: LatestNewsCard refactor

**Goal:** Give changelog entries visual hierarchy via type-icons (feat/fix/perf), type pills, relative dates, and drop the fragile title-split regex in favor of a simple character-count fallback.

**Success Criteria:**
1. `<SpotlightIcon>` renders with type-specific name for each entry (sparkles/wrench/lightning).
2. Type pill renders with type-coded accent class.
3. Dates rendered via `Intl.RelativeTimeFormat` ("2 дня назад" rather than raw ISO).
4. Sentence-splitter regex removed from component source.
5. `LatestNewsCard.spec.ts` passes.

**Plan:** `phases/07-latest-news-refactor/07-PLAN.md`

### Phase 08: PlatformStatsCard refactor

**Goal:** Fill the N=1 dead space with a richer layout — hero stat (left, oversized number) + 2×2 micro-grid of supporting stats (right) + 7-day sparkline + delta chip. Requires a small backend extension to emit `previous_value` and `series[7]`.

**Success Criteria:**
1. Hero stat renders at `text-7xl` / `text-8xl`.
2. `Sparkline` SVG has `data-points` attribute matching the 7-day series.
3. `DeltaChip` renders `↑` when `value > previous_value`, `↓` when less, `—` when equal/null.
4. Supporting micro-grid renders 4 metrics when 5 are provided.
5. Backend test confirms `len(Series) == 7` and `PreviousValue` is populated.

**Plan:** `phases/08-platform-stats-refactor/08-PLAN.md`

### Phase 09: NotTimeYetCard refactor

**Goal:** Make NotTimeYetCard distinct from AnimeOfDayCard via amber/clock theming, a status pill (planned vs postponed), a "Last added X ago" timestamp, and a direct-to-watch CTA. Requires a small backend pass-through.

**Success Criteria:**
1. `<SpotlightIcon name="clock">` rendered in header.
2. Status pill text matches `planned` → "В планах" / `postponed` → "Отложено".
3. Status pill class is `bg-yellow-500/20` for planned, `bg-slate-500/20` for postponed.
4. CTA href ends in `/watch` (direct-to-watch).
5. Relative `added_at` rendered when provided; backend emits `AddedAt`.

**Plan:** `phases/09-not-time-yet-refactor/09-PLAN.md`

### Phase 10: ContinueWatchingNewCard refactor

**Goal:** Transform the tiny "New episode N!" corner badge into a hero ribbon across the top of the poster, stack the episode meta (last watched + new) with visual hierarchy, and deep-link the CTA into the new episode directly.

**Success Criteria:**
1. Hero ribbon spans poster top (`inset-x-0 top-0`) and contains the new episode number.
2. Two-row episode meta renders: "Вы посмотрели до серии N" (subdued) + "Новая серия N" (purple accent).
3. CTA href ends in `/watch?episode={n}` (deep-link).
4. Pre-flight verified: `Watch.vue` honors `?episode=N` query param on mount.
5. `ContinueWatchingNewCard.spec.ts` passes.

**Plan:** `phases/10-continue-watching-new-refactor/10-PLAN.md`

---

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
