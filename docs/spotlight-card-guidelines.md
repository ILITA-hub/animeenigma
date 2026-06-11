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
| `violet` | meta / service | random_tail, latest_news, telegram_news, not_time_yet |

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
  transition lock) from card code.

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
