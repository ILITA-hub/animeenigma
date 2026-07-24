# Spotlight Card Guidelines

How to add (or redesign) a hero-spotlight card without breaking the carousel,
the DS, or the design-review process. The **mechanical 5-anchor recipe**
(resolver → Data type → DI → SFC → dispatch/i18n/types) lives in
`CLAUDE.md §Adding a Spotlight Card Type` — read it first; this doc is the
design + quality contract layered on top.

## 1. Anatomy — always build on `SpotlightCardShell`

Every card SFC wraps `SpotlightCardShell.vue`. Never hand-roll the frame.

```vue
<SpotlightCardShell
  accent="cyan|pink|violet"     <!-- triad, see §2 -->
  icon="<SpotlightIconName>"    <!-- kicker icon -->
  :kicker="t('spotlight.<type>.title')"
  backdrop="gradient-mesh|poster-blur|none"
  :poster-url="..."             <!-- for poster-blur -->
>
  <template #background-extra>…accent wash…</template>
  …body (default slot)…
  <template #cta>…Button-variant router-links…</template>
</SpotlightCardShell>
```

- **SINGLE-ROOT**: the shell's `<article>` must be the only root node. No
  top-level `v-if`, no leading template comments, no sibling nodes — the
  parent `<Transition mode="out-in">` silently wedges the whole carousel on
  fragment roots (Phase 04 e2e regression).
- **CTA pinned bottom-left** via the `#cta` slot. Full-bleed hero cards use
  `justify="end"`.
- **Kicker weight is `font-medium` (500), NEVER semibold** — JetBrains Mono is
  loaded at 400/500 only; 600 triggers blurry faux-bold synthesis (2026-06-11
  fix). Don't override the shell's kicker classes.

## 2. Accent — pick from the brand triad by card category

| Accent | Category | Existing cards |
|---|---|---|
| `cyan` | content core | featured, personal_pick, platform_stats |
| `pink` | live / personal activity | now_watching, continue_watching_new |
| `violet` | meta / service | random_tail, latest_news, telegram_news, not_time_yet, gacha_promo |

Cards are differentiated by **kicker icon + layout character**, not by new
hues. Never add a 4th accent; never use raw Tailwind palette colors (DS lint
will fail the build).

## 3. Badges, buttons, posters

- On **poster/photo imagery** → `<Badge overlay>` (dark glass). On **glass
  tiles/mesh** → tinted badges (`bg-success/15 text-success`, …). This is the
  locked inline-vs-overlay rule (2026-06-05).
- Score glyphs: amber lucide `Star` = Shikimori/MAL, cyan `ScoreDiamond` ◆ =
  site score. Plain text (no pill) for year/episode counts.
- CTAs are `router-link`s with `buttonVariants({variant, size})` classes —
  never the `Button` `href` prop (full page reload) and never bespoke
  `.btn-*` CSS.
- Posters load via `cardPosterUrl(url, width)` (image-proxy + width bucket)
  with `loading="lazy" decoding="async"`. Decorative posters are plain
  `<img>`; use `PosterCard` only for real catalog items ≥96px wide where the
  context menu makes sense (see the v4 PS analysis in the 2026-06-11 spec).
- **Every image shows a DS shimmer until it decodes** (2026-06-11 lock):
  prefer `SpotlightPoster` (built-in `skeleton-shimmer` + 300ms fade-in);
  raw `<img>`s replicate the pattern by hand (`@load`/`@error` → fade) —
  see FeaturedCard / TelegramNewsCard. Never ship a bare empty box.
- New image surfaces must be added to `cardImageUrls()` in
  HeroSpotlightBlock — it idle-prefetches every slide's images at the SAME
  proxy buckets after the cards arrive, so slide flips are cache hits. A
  bucket mismatch silently breaks the prefetch for your card.
- Typography: `font-medium`/`font-semibold` only; titles `font-display`;
  long text gets explicit line-clamps (TW4 may not emit arbitrary
  `line-clamp-[N]` — verify or use scoped CSS like LatestNews).

## 4. Carousel chrome integration

- Add the card's entry to `cardTokens` in `tokens.ts` (accent + `kickerKey` +
  icon). The menu (A-1 icon-menu) and a11y labels read from it — a missing
  entry falls back to the generic sparkles token.
- The skeleton must keep reserving the menu row height — if you change menu
  geometry, change the skeleton in the same PR (zero-CLS rule).
- Don't touch carousel mechanics (7s autoplay, stop-on-manual-nav,
  transition lock, touch swipe) from card code.
- **No overlays inside the frame** (ARR-1 lock, 2026-06-11): card content
  may run to the frame edges (terminal, deck, rec column) — nav chevrons
  live in the CarouselDots menu row BELOW the frame, never on top of the
  card. Don't reintroduce in-frame floating controls.
- The active menu pill animates open via the `grid-template-columns
  0fr→1fr` wrapper in CarouselDots — keep kicker labels short (≤ ~24 chars)
  so the expansion doesn't wrap the row on 390px.

## 5. Data & caching

- Reuse `SpotlightAnime` fields (description/year/season/status/genres/score/
  episodes) before adding new payload fields.
- Redis keys carry the `spotlight:` prefix (HSB-NF-03). Remember the whole
  card-set is ALSO cached per user/day as `spotlight:snapshot:<user>:<date>`.
- **Changing a cached payload shape ⇒ same-day flush** of both the card key
  and `spotlight:snapshot:*`, then runtime-smoke the live endpoint (stale
  old-shape JSON silently deserializes into empty structs).
- Resolvers run under an **800ms per-card deadline** — parallelize multiple
  upstream calls (see `platform_stats.go` tile goroutines), never walk them
  sequentially.
- **Card `Priority` semantics**: default 1.0; values BELOW 2.0 (e.g. curated's
  1.5) only bias the carousel's weighted-random opening pick. Values `>= 2.0`
  (frontend `PINNED_PRIORITY_MIN`, weightedRandom.ts) make the card **pinned**:
  `useSpotlight` orders it first in the deck and the carousel ALWAYS opens on
  it (`openingSlideIndex`). One pinned promo at a time, please — two pinned
  cards compete by raw priority value (highest wins slide 0).

## 6. Mobile (390px, frame 470px)

- Padding `p-4`; posters drop to `w-24`; two-column layouts stack.
- Vertical inner scroll areas conflict with page scroll → convert to
  **horizontal swipe rows** (PosterRow pattern).
- CTA goes full-width at the bottom when space is tight.

## 7. Process (how changes get approved)

1. Design iterations go through the hosted-artifact loop
   (`.brainstorm/server.cjs`, port 58363, newest html in `.brainstorm/content/`
   wins; user tunnels localhost:3000). Artifact structure: ① primitives from
   source ② decision history ③ designs with edge cases ④ on-page spec + md
   mirror in `docs/superpowers/specs/`.
2. **Always show a "current prod" screenshot per card** next to proposals
   (locked practice, 2026-06-11).
3. Show desktop AND 390px mobile mockups for any layout change.
4. No implementation until the user locks variants (ЛОК).
5. Ship checklist: i18n parity en/ru (+ja where the namespace has it; the
   spotlight-keys parity test fails on en/ru drift), co-located spec ≥5
   assertions, `vitest run src/components/home/spotlight/`, `vue-tsc`, DS
   lint, then an in-browser smoke at 1440px + 390px (DS-NF-06 — jsdom can't
   catch TW4 cascade bugs).

## Recipe: add a Spotlight card type (5 anchors, ~50 lines)

`HeroSpotlightBlock` (workstream `hero-spotlight`) is a 14-card rotating carousel; adding another touches 5 anchors. Read the guidelines above alongside this recipe.

1. **BE resolver** — create `services/catalog/internal/service/spotlight/cards/{new_type}.go` implementing `spotlight.Resolver` (`Type()` + `Resolve(ctx, userID *string) (*spotlight.Card, error)`). Mirror `featured.go` («Рекомендуем сегодня»): manual `cache.Get`/`cache.Set` with `errors.Is(err, cache.ErrNotFound)`; return `(nil,nil)` ineligible, `(nil,err)` failure, `(*Card,nil)` success. Multi-item resolvers MUST apply `spotlight.AdaptiveSlice` (1-2-3 layout rule). Login-only resolvers return `(nil,nil)` when `userID==nil`. Carry the `spotlight:` Redis key prefix for new keys (HSB-NF-03). Co-locate a `_test.go` with handwritten fakes (no testify/mock).
2. **BE Data type** — add the JSON-shaped `{NewType}Data` struct to `services/catalog/internal/service/spotlight/types.go` (extends the Card union). Add a round-trip marshal/unmarshal test to `types_test.go`.
3. **BE DI** — add a `cards.New{NewType}Resolver(...)` call to the `spotlightResolvers` slice in `services/catalog/cmd/catalog-api/main.go`. Stable order = tie-break display order.
4. **FE SFC** — create `frontend/web/src/components/home/spotlight/cards/{NewType}Card.vue` with a typed `data` prop (the step-5 variant). Honor UI-SPEC: ONLY `font-medium`/`font-semibold`, `p-4 md:p-6 lg:p-8` padding, Tailwind-utility-only, `min-h-[400px] md:min-h-[340px] lg:min-h-[320px]`. Add `target="_blank"` + `rel="noopener noreferrer"` on external anchors. Co-locate a `.spec.ts` (≥5 Vitest assertions).
5. **FE dispatch + i18n + types** — extend the `SpotlightCard` discriminated union in `frontend/web/src/types/spotlight.ts` with `{ type:'{new_type}', data:{NewType}Data }`; add a `v-else-if="active.type === '{new_type}'"` branch to the dispatch chain in `HeroSpotlightBlock.vue` (DO NOT switch to `<component :is>` — keep the typed chain so vue-tsc narrows props); add a `spotlight.{newType}.*` sub-namespace to BOTH `frontend/web/src/locales/en.json` and `ru.json` (parity test `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` fails on mismatch).

Verify: `cd services/catalog && go test ./internal/service/spotlight/... -count=1 -race` and `cd frontend/web && bunx vitest run src/components/home/spotlight/ src/locales/__tests__/spotlight-keys.spec.ts && bunx tsc --noEmit`. E2E regression: `frontend/web/e2e/spotlight.spec.ts`.
