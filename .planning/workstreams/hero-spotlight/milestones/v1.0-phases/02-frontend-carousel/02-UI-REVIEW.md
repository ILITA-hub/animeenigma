# Phase 02 — UI Review (HeroSpotlightBlock + Carousel)

**Audited:** 2026-05-21
**Baseline:** `02-UI-SPEC.md` (Neon Tokyo design contract)
**Screenshots:** not captured (no dev server on :3000/:5173/:8080; live SPA is client-rendered so the public HTML at `https://animeenigma.ru/` is the empty Vue shell). Audit is **code-only against UI-SPEC + the running `/api/home/spotlight` payload**.
**Audit stance:** adversarial — assumed every pillar has failures until the SFCs proved otherwise. The user explicitly said "the implementation was still bad" until two rounds of CSS fixes; this audit treats Round-2 fixes as the surface to score, not as exoneration.

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | **2**/4 | `LatestNewsCard` "Read full changelog →" links to `/` (visible arrow promises navigation it does not deliver); `formatEntryDate()` is a no-op passthrough that renders raw `"2026-05-21"` ISO strings; `pauseAutoplay` i18n leaf is dead. |
| 2. Visuals | **3**/4 | Layout works after Round-2 fixes, but `h-[420px]` (hard height) replaced UI-SPEC's `min-h-[400px]` (flexible) — CLS-safe but silently clips overlong content. `<transition>` wraps a 9-branch `v-if/v-else-if` chain (Phase 3 bleed). |
| 3. Color | **3**/4 | 60/30/10 cyan-only rule respected in Phase-2 cards. The cards under `cards/` collectively introduce `purple-*` + `green-*` (Phase-3 ContinueWatchingNew/NowWatching) which UI-SPEC §Color forbids ("not used in Phase 2 — reserved for Phase 3"). Phase-2 cards stay within budget. |
| 4. Typography | **3**/4 | `font-bold` / `font-normal` correctly absent. PlatformStatsCard's 1-metric mode introduces `text-5xl md:text-6xl` (Round-2 sparse-state fix) — not declared in UI-SPEC's 4-size table; spec drift. |
| 5. Spacing | **2**/4 | UI-SPEC declared `min-h-[400px] md:min-h-[340px] lg:min-h-[320px] lg:max-h-[360px]`. Implemented `h-[420px] md:h-[320px] lg:h-[320px]` — lost the `md` 340px tier, lost the `lg` 360px ceiling, swapped flexible `min-h` for rigid `h`. Mobile is now 20px taller than spec. |
| 6. Experience Design | **3**/4 | State machine is solid (useIntervalFn + rAF-deferred focusout + reduced-motion + random init guard). Missing the `aria-live` pause announcement the UI-SPEC promised. Single-fetch no-retry path is acceptable per spec. |

**Overall: 16/24**

---

## Top 3 Priority Fixes

1. **BLOCKER — `LatestNewsCard` "Read full changelog →" link routes nowhere useful** (`frontend/web/src/components/home/spotlight/cards/LatestNewsCard.vue:9-14`).
   The visible arrow + EN "Read full changelog →" / RU "Все обновления →" promises a destination; the `to="/"` makes it a no-op router-link when the user is already on `/`. Plan 02-03's executor knew this and chose `/` because no `/changelog` route exists — but the user-facing copy was never softened to match.
   **Fix:** either (a) add a `/changelog` route mounting `LastUpdates.vue`, change `to="/changelog"`, OR (b) change the copy to `spotlight.latestNews.readMore` → "See more on the home page" / drop the arrow, OR (c) make the link scroll to the `#changelog-section` anchor on Home. Option (a) is the cheapest and matches the user's mental model.

2. **WARNING — `formatEntryDate()` is a no-op passthrough rendering raw ISO strings** (`LatestNewsCard.vue:53-55`).
   Users see `"2026-05-21"` instead of "2 hours ago" / "yesterday" / "May 21". UI-SPEC §Copywriting Contract explicitly specifies `formatRelative()` from `@vueuse/core` (already in deps); Plan 02-05 ships `spotlight.latestNews.entryDate` for this purpose; the key is referenced in the parity test but never in the component.
   **Fix:** import `formatTimeAgo` or `formatRelative` from `@vueuse/core` and feed `new Date(entry.date)`. Or at minimum use `Intl.DateTimeFormat(locale, { dateStyle: 'medium' })` so the locale-aware short date renders ("May 21" / "21 мая").

3. **WARNING — Carousel slide height regression: spec `min-h` → implementation `h`** (`HeroSpotlightBlock.vue:27, 50`).
   Round-2 fix turned `min-h-[400px] md:min-h-[340px] lg:min-h-[320px] lg:max-h-[360px]` (UI-SPEC §Spacing Scale) into `h-[420px] md:h-[320px] lg:h-[320px]`. Three consequences:
   - Mobile is 20px taller than declared (420 vs 400) — not a defect, but undocumented.
   - Tablet shrank from 340 → 320 — Now-Watching with 5 session rows on tablet has nowhere to grow.
   - Desktop dropped `lg:max-h-[360px]` ceiling and the `min-h` floor — hard pin at 320. PlatformStatsCard with 3 metrics in `text-3xl md:text-4xl` value text + headline + uppercase label + delta line per chip is tight; any future i18n that lengthens the metric label will push to overflow rather than expand.

   **Fix:** restore the UI-SPEC contract: `min-h-[400px] md:min-h-[340px] lg:min-h-[320px] lg:max-h-[360px]`. Combined with the existing `flex flex-col` + `flex-1 min-h-0 pb-10` on the slide container, this gives growth room without breaking CLS prevention (the `min-h` ensures the skeleton matches the loaded floor).

---

## Detailed Findings

### Pillar 1: Copywriting (2/4)

**BLOCKER findings:**

- **F1.1 — LatestNews readMore CTA is a dead arrow.** `frontend/web/src/components/home/spotlight/cards/LatestNewsCard.vue:9-14` — `<router-link to="/">{{ t('spotlight.latestNews.readMore') }}</router-link>`. EN copy `"Read full changelog →"`, RU `"Все обновления →"`, JA `"全ての更新 →"`. The arrow glyph implies forward navigation; the destination is the page already on. Plan 02-03 SUMMARY documents this as a deliberate temporary choice ("no /changelog route exists"). Three months in, this is still ship-quality regression.
  - 02-01-SUMMARY.md item-5 in "Follow-up Work" called for backend follow-up (add `title?:` field); the more pressing fix is router-level (add `/changelog` route mounting `LastUpdates.vue`).

**WARNING findings:**

- **F1.2 — Entry date renders raw ISO strings.** `LatestNewsCard.vue:53-55` — `function formatEntryDate(iso: string): string { return iso }`. UI-SPEC §Copywriting Contract: `spotlight.latestNews.entryDate` is supposed to use `formatRelative()` from `@vueuse/core` (a dep already in `package.json`). The i18n key was shipped by Plan 02-05; the consumer was never wired up. Users currently see `"2026-05-21"` which is fine in EN/RU but unlocalized and unfriendly.

- **F1.3 — `pauseAutoplay` i18n key is dead.** EN `"Autoplay paused"`, RU `"Автопрокрутка приостановлена"`, JA `"自動再生を一時停止"` — shipped in all three locales (`en.json:994`, `ru.json:994`, `ja.json:994`). UI-SPEC §Copywriting Contract explicitly states it is "sr-only live announcement when hover/focus pauses cycle". Grep `pauseAutoplay` finds 4 hits: 3 locale files + 1 parity spec. Zero usage in any Vue component. Sighted users get visual feedback (cycle stops, dot stays put); screen-reader users get nothing.

- **F1.4 — Russian changelog message hygiene (data, not UI, but surfaces in the block).** Live API returns `"🎯 Steam-style контекст отзывов! ... Никто больше не сможет молча обосрать аниме..."`. The word `обосрать` is profanity-tier informal; it lands on every home-page render to a logged-in user. Out of Phase-2 scope (it's a content/changelog content concern, not a UI bug) but the spotlight is the surface that exposes it. **Recommend:** tighten the changelog tone-of-voice gate; or surface only sanitized entries to the spotlight.

- **F1.5 — `RandomTailCard` "Открыть" CTA navigates to anime detail page, not "discover something new" experience.** `RandomTailCard.vue:94-99` — link target is `/anime/${data.anime.id}`. The eyebrow "Случайная находка" + subtitle "Откройте что-то новое" sells discovery; the CTA delivers the standard detail page. Minor; copy mismatch with action.

### Pillar 2: Visuals (3/4)

**WARNING findings:**

- **F2.1 — Fixed-height containers replaced flexible min-height.** `HeroSpotlightBlock.vue:27` (skeleton) and `:50` (loaded). Both now use `h-[420px] md:h-[320px] lg:h-[320px]` instead of the UI-SPEC's `min-h-[400px] md:min-h-[340px] lg:min-h-[320px] lg:max-h-[360px]`. Implications:
  - Tablet height regressed from 340 → 320 (–20px). LatestNewsCard's 3-col grid + header gets tighter.
  - Desktop ceiling `lg:max-h-[360px]` lost — any 1-metric PlatformStats with `text-6xl` value text (~60px line-height) + uppercase label + delta + parent padding is right at the wall.
  - Mobile gained 20px (400 → 420) without spec.

- **F2.2 — `<transition mode="out-in">` wraps a 9-branch `v-if/v-else-if` chain.** `HeroSpotlightBlock.vue:66-115`. Vue 3 `<transition>` requires a single direct child; here it gets 9 siblings each with `v-if/v-else-if`. At runtime only one renders so the constraint isn't violated, but vue-tsc / vue-language-server lint occasionally surfaces this as a structural smell. With 9 card types the chain is also a maintenance scaling concern — adding a 10th means another branch and another `:key` invocation. Plan 02-06 SUMMARY explicitly notes this was deliberate ("vue-tsc cannot narrow the union prop in dynamic component dispatch") but a `defineComponent`-mapped dispatch table with explicit per-variant typing would be cleaner.

- **F2.3 — Skeleton state has no shimmer accent.** `HeroSpotlightBlock.vue:29` — `<div class="w-full h-full skeleton-shimmer" />` fills the whole 420/320px rectangle with a single uniform shimmer. The loaded state has poster + meta + dots — a more visually-accurate skeleton would silhouette these regions (poster rect on the left, 2 text rows on the right). Acceptable as a "Tier-1" skeleton but a clear visual downgrade vs `CollectionsRow` / `ContinueWatchingRow` skeletons that do silhouette real elements.

- **F2.4 — PlatformStatsCard 1-metric mode has no visual context for the lonely number.** `PlatformStatsCard.vue:14-50` — when `metrics.length === 1`, renders centered with `text-5xl md:text-6xl` value. Round-2 dropped the empty `—` no-change indicator. With backend currently emitting only `anime_added_7d: 3`, users see a giant "3" centered on a 320px-tall card with the uppercase label "Новых аниме" above. The visual weight implies it's the headline metric; for "3 new anime in 7 days" that's overclaim. **Recommend:** when `metrics.length === 1`, soften the value text to `text-4xl md:text-5xl` and add a `text-sm` body line explaining the timeframe in plain language ("Last 7 days").

### Pillar 3: Color (3/4)

Tailwind class distribution (top tokens used in `src/components/home/spotlight/`):

```
17 bg-white      — surface chrome (skeletons, chip backgrounds)
16 text-white    — primary text
 9 text-cyan-400 — accent (CTAs, eyebrows, focus, active dot, delta+)
 8 text-gray-400 — secondary text (meta, dates)
 5 text-cyan-300/80 — RandomTail eyebrow (dimmer cyan — within spec)
 3 text-gray-300 — genre chips
 2 text-yellow-400 — score star (within spec)
 2 bg-cyan-500/20 — chevron hover (within spec)
 1 text-green-400, 1 bg-green-400  ← Phase-3 NowWatching (LIVE badge)
 1 text-cyan-400, 1 bg-cyan-400    — within spec
[Phase 3 cards: purple-300/90, purple-500/90 in ContinueWatchingNewCard]
```

**Phase-2 cards (AnimeOfDay, RandomTail, LatestNews, PlatformStats):** Color discipline is excellent — cyan-400 reserved for the 4 declared cases (active dot, focus, primary CTA, hover-link). Score-star yellow-400 contained to the chip overlay.

**Phase-3 bleed (will affect Phase-3 UI-SPEC, not this one):**

- **F3.1 — `ContinueWatchingNewCard` uses `purple-*`, `NowWatchingCard` uses `green-*`.** UI-SPEC §Color: "**Accent secondary (rare) — pink** `#ff2d7c` — not used in Phase 2 — **Reserved for Phase 3's `now_watching` "live" indicator — DO NOT use in Phase 2**". Phase 3's actual implementation chose `green-400` for the live indicator and `purple-*` for "new episode" — two new colors with no UI-SPEC declaration. The 60/30/10 budget remains intact but the palette grew. This is a Phase-3 audit concern; flagged here for completeness because it shares the parent block.

### Pillar 4: Typography (3/4)

Tailwind font-size distribution across all 9 cards + wrapper + controls:

```
text-xs    — labels, chips, dates, deltas (12px)
text-sm    — body, meta, secondary CTAs (14px)
text-base  — primary CTA labels at md+ (16px)
text-lg    — card headings at default (18px)
text-xl    — card headings at md+ (20px)
text-2xl   — display titles at default (24px)
text-3xl   — display titles at md+ (30px)
text-4xl   — PlatformStats metric value (multi-metric, md+)
text-5xl   — PlatformStats metric value (1-metric mode, default)
text-6xl   — PlatformStats metric value (1-metric mode, md+)
```

Font weights: `font-medium` (500) and `font-semibold` (600). No `font-bold` / `font-normal`. ✓ Within the UI-SPEC §Typography "two weights" rule.

**WARNING findings:**

- **F4.1 — Round-2 single-metric mode added `text-5xl md:text-6xl` outside spec.** UI-SPEC §Typography declares four sizes × two weights with explicit roles: Display `text-2xl md:text-3xl`, Heading `text-lg md:text-xl`, Body `text-sm md:text-base`, Label `text-xs`. Round-2's "centered text-5xl/6xl" fix for the sparse 1-metric state introduces two new sizes (60-ish px). This is a Spec-Drift, not a violation — the visual is reasonable — but should be documented in `02-UI-SPEC.md` Typography section to keep the spec honest.

- **F4.2 — Tabular-nums applied correctly.** `PlatformStatsCard.vue:35` — `tabular-nums leading-none` on the value. ✓

- **F4.3 — `tracking-wider` on uppercase eyebrows.** Used consistently on `AnimeOfDayCard`, `RandomTailCard`, `NotTimeYetCard`, `ContinueWatchingNewCard`, `PlatformStatsCard`, `PersonalPickCard`. ✓ Project convention from `Home.vue` carry-over.

### Pillar 5: Spacing (2/4)

Spotlight-only spacing-class distribution (top tokens):

```
39 p-4   — card outer padding (default + md)
20 p-5   — not used in spotlight (parent Home grid)
16 p-2   — chip + score-chip
12 gap-3 — content column gap
10 gap-4 — desktop card gap
 9 p-6   — desktop card padding (lg:p-6)  ✓ matches UI-SPEC
 7 p-3   — news entry tile inner padding
 7 gap-2 — chip flow, dot indicator gap
 5 px-2  — chip horizontal
 5 gap-1 — meta inline gap
 4 gap-6 — desktop md:gap-6 row gap
 3 py-0.5 — chip vertical (0.125rem ≈ 2px — sub-4 multiple, intentional micro-pad)
```

The 4-multiple scale (4/8/16/24/32 px) is **substantially** held, with documented exceptions: 12px (`gap-3`, `space-y-3`, `bottom-3` — UI-SPEC explicit exception) and 2px (`py-0.5` on chips — convention from existing `Home.vue` `trendingRecs`).

**WARNING findings:**

- **F5.1 — Block height contract regressed (see F2.1).** `h-[420px] md:h-[320px] lg:h-[320px]` vs UI-SPEC's `min-h-[400px] md:min-h-[340px] lg:min-h-[320px] lg:max-h-[360px]`. Five values changed: mobile +20px, tablet -20px, desktop floor unchanged, desktop ceiling removed, `min-h` → `h` (constraint type changed). This is the **single most visible defect** — it's also the one Round-1 + Round-2 fixes worked hardest on, so the user's friction with this surface is high.

- **F5.2 — PlatformStatsCard delta indicator margin inconsistency.** UI-SPEC §Visual Contract specified `<p ... class="mt-1 text-xs ...">{{ deltaPositive }}</p>`. Implementation uses `mt-2`. (`PlatformStatsCard.vue:46`). 4px drift. Cosmetic, but specifically off-spec.

- **F5.3 — Tablet padding `md:p-4` instead of UI-SPEC's `p-4` (no md: override).** UI-SPEC §Spacing Scale "Card inner padding" — Desktop `p-6`, Tablet `p-4`, Mobile `p-4`. AnimeOfDayCard etc. ship `p-4 md:p-4 lg:p-6` — the explicit `md:p-4` is redundant but harmless. ✓

- **F5.4 — RandomTailCard mobile-only eyebrow `mb-1` instead of mb-2 used on desktop.** `RandomTailCard.vue:8` mobile `mb-1`, line 49 desktop `mb-2`. Two different spacings on the same visual concept. Cosmetic.

- **F5.5 — `pb-10` on slide container** (`HeroSpotlightBlock.vue:53`) — 40px bottom padding inside the slide region to clear the absolute-positioned dots at `bottom-3`. This is sensible (the dots overlap content otherwise) but the constant 40px subtracts from the available slide height across all breakpoints; combined with the 320px desktop hard pin from F5.1, this leaves cards 280px of usable vertical space. Document this in the spec.

### Pillar 6: Experience Design (3/4)

State machine: covered by 12 Vitest cases + 9 Playwright e2e cases (Plan 02-04 + Plan 02-06 SUMMARYs). The core mechanics — random init guard, useIntervalFn pause/resume, rAF-deferred focusout, reduced-motion override, runtime media-query reactivity, single-card no-cycle, wraparound — are tight.

**PASS findings (no follow-up needed):**

- ✓ Skeleton state present and matches loaded dimensions (CLS-safe).
- ✓ Error state silent self-hide per UI-SPEC §State Contract; one `console.warn` for observability.
- ✓ Empty state silent self-hide.
- ✓ Feature flag `VITE_HERO_SPOTLIGHT_ENABLED` honored at build time.
- ✓ `ArrowLeft` / `ArrowRight` keyboard nav bound on root `<section>`.
- ✓ Hover & focus pause; rAF-deferred focusout prevents Tab-between-dots flicker.
- ✓ Single-fetch-no-retry path is acceptable per UI-SPEC §State Contract; Phase 3 will introduce 30s refresh.

**WARNING findings:**

- **F6.1 — `pauseAutoplay` SR announcement is missing.** UI-SPEC §Copywriting and §Accessibility both promise an `sr-only` live region announcing pause state. The key was shipped in all three locales; **zero consumers**. Screen-reader users get no signal when the carousel they're focused inside has paused. Implementation:
  ```vue
  <span class="sr-only" aria-live="polite">
    {{ paused ? t('spotlight.pauseAutoplay') : '' }}
  </span>
  ```
  Add to `HeroSpotlightBlock.vue` template + drive from `intervalId.value === null && cards.value.length > 1`.

- **F6.2 — `aria-live="polite"` on slide region + frequent auto-advance every 7s.** `HeroSpotlightBlock.vue:63`. Auto-advance + aria-live means screen readers announce a new slide every 7 seconds. APG-correct (the polite region waits for screen-reader idle) but anecdotally aggressive. UI-SPEC §A11y "Screen reader announcement flow" describes this exact behavior, so it's spec-compliant — flag here so the team is aware and can revisit if user feedback rolls in.

- **F6.3 — `onAdd` stub silently no-ops.** `AnimeOfDayCard.vue:125-128` — `// Phase 2: stubbed handler. Phase 3 will wire to the watchlist API`. The CTA button labelled "Add to list" / "В список" / "リストに追加" looks active, has no `disabled` attribute, no aria-disabled, and emits zero feedback on click. Users will press it and silently see no result. **Recommend:** either (a) Phase 3 implements the real handler ASAP, OR (b) Phase 2 either disables the button visually (`opacity-50 cursor-not-allowed`) OR removes it entirely until Phase 3 lands.

- **F6.4 — No swipe gesture support on mobile.** UI-SPEC §Responsive "Swipe gestures: Not supported in Phase 2 — N/A — Phase 4+ enhancement if user demand arises". Spec-compliant. Mobile users have chevrons (visible at all breakpoints) + dot taps. ✓ Acceptable.

- **F6.5 — Tabindex on the 420px-tall root `<section>`.** `HeroSpotlightBlock.vue:41` — `tabindex="0"`. When a keyboard user Tabs into the block, the global `:focus-visible` rule (`main.css:91`) paints a 2px cyan-400 ring + 2px base-color spacer around the entire 420px×~1280px rectangle. Visually heavy. APG carousel pattern endorses focusing the region for arrow-key handling so this is correct. Cosmetic note only.

---

## Files Audited

**Phase 2 source (primary scope):**
- `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` (297 lines, current state — Round-2 fixed)
- `frontend/web/src/components/home/spotlight/CarouselControls.vue` (105 lines)
- `frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.vue` (129 lines)
- `frontend/web/src/components/home/spotlight/cards/RandomTailCard.vue` (119 lines)
- `frontend/web/src/components/home/spotlight/cards/LatestNewsCard.vue` (78 lines)
- `frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue` (75 lines)
- `frontend/web/src/composables/useSpotlight.ts` (68 lines)
- `frontend/web/src/styles/main.css` (lines 90-94, 110-115, 236-…, 299-331)
- `frontend/web/src/views/Home.vue` (mount point: `<HeroSpotlightBlock />` at line 38)
- `frontend/web/src/locales/en.json` (spotlight.* namespace, lines 987-1063)
- `frontend/web/src/locales/ru.json` (spotlight.* namespace, lines ~987-1063)

**Phase 3 source (bonus / cross-pillar observations):**
- `frontend/web/src/components/home/spotlight/cards/PersonalPickCard.vue` (91 lines)
- `frontend/web/src/components/home/spotlight/cards/NowWatchingCard.vue` (63 lines)
- `frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.vue` (58 lines)
- `frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.vue` (93 lines)
- `frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.vue` (92 lines)

**Phase 2 plan/spec inputs:**
- `02-UI-SPEC.md` (primary baseline — 781 lines)
- `02-CONTEXT.md`
- `02-01..06-SUMMARY.md` (all six plan summaries)

**Live deploy verification:**
- `GET https://animeenigma.ru/api/home/spotlight` — returned 6 cards (anime_of_day, platform_stats, random_tail, latest_news, telegram_news, personal_pick); `generated_at: 2026-05-21T08:43:28Z` — block is alive and serving real data.
- `GET https://animeenigma.ru/` — returned client-rendered SPA shell (1896 bytes); rendered DOM not inspectable without a browser. Code audit performed against the Vue/CSS sources committed at HEAD.

---

## Audit Notes

- Two-round-of-fixes context: commits `915da45` and `cf1ea07` are reflected in the current `HEAD`. Findings F1.1, F2.1, F5.1, F5.2 are residual issues after those fixes — the user's note that "it was still bad" is borne out by the height-contract regression (F5.1) and the dead readMore link (F1.1).
- The Round-2 PlatformStats single-metric `text-5xl/6xl` (F4.1) is a defensible UX fix but introduces a typography spec drift that should be reconciled by amending `02-UI-SPEC.md` rather than reverting the fix.
- Screenshots not captured (no local dev server, live SPA renders client-side). All findings are anchored to file paths + line numbers in the source. A future audit with Playwright-MCP available would benefit from per-breakpoint screenshots of the 4 Phase-2 cards in their loaded + single-metric + skeleton + reduced-motion states.
- Recommendation count: **3 priority fixes**, **9 warnings** documented across 6 pillars. The implementation is shippable (currently shipping live) but has measurable spec drift in 4 of 6 pillars.
