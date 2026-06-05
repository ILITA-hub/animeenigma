# Unified Anime Card — UI Design Spec

> Date: 2026-06-05
> Status: APPROVED design, ready for implementation planning
> Scope: `frontend/web/src/**` (Vue 3 + Tailwind v4 + shadcn-vue, "Neon Tokyo" design system)
> Companion inventory: [`docs/anime-card-inventory.md`](../../anime-card-inventory.md)

This document is self-contained and intended to be handed to an implementing AI/engineer. It defines a standardized anime-card component system that replaces ~12 bespoke, per-surface card implementations with three shared layout components over a small set of shared primitives and one normalized data model. The explicit goal is **de-fragilization** (one poster impl, one badge system, one data shape, zero scoped CSS) — not merely a visual refresh.

---

## 1. Background & problem

The frontend renders anime as cards in **14 surfaces**. Today:

- Only `AnimeCardNew.vue` is a reusable card, and it's used in just **2** surfaces (Browse grid, Anime-detail related rail).
- The other **12** surfaces hand-roll card markup inline (Home rails via `ColumnItem.vue` with ~140 lines of scoped CSS, Continue-Watching, Collections, Schedule, Profile table+grid, Activity, Admin recs).
- `AnimeCard.vue` (legacy) and `AnimeCardSkeleton.vue` are **dead code** (exported, zero consumers).
- The same sub-elements (poster+lazy+fallback, rating badge, status pill, play overlay) are reimplemented ~3 ways each.
- **Biggest fragility source:** every surface feeds a *different data shape* — `AnimeCardNew` wants `coverImage`/`nameRu`, `ColumnItem` wants `poster_url`/`name_ru`, `CollectionsRow` wants `cover_image_url`.
- 5+ poster aspect ratios and inconsistent radii (`rounded-lg` vs `rounded-xl`).

Full per-surface breakdown: see the companion inventory doc.

---

## 2. Goal & non-goals

**Goal:** Three standardized, token-bound, utility-only card components + shared primitives + one `toCardModel()` normalizer, migrated across all in-scope surfaces, eliminating bespoke per-card CSS.

**In scope (all three shapes):**
- **Shape A — `PosterCard`** (vertical 2/3)
- **Shape B — `PosterRow`** (compact horizontal thumb + meta)
- **Shape C — `MediaTile`** (16:9 landscape)

**Out of scope (separate session):**
- The 9 **Hero Spotlight** cards in `components/home/spotlight/cards/*` — they are a distinct full-bleed "hero" design language. Only their *accent tokens* may be unified later; their layout is untouched here.

---

## 3. Architecture (Approach 1: layout components over shared primitives)

```
toCardModel(raw)  ──►  AnimeCardModel  ──►  PosterCard | PosterRow | MediaTile
                                              │
                                              ├── PosterImage      (lazy + fallback + skeleton)
                                              ├── Badge (overlay variants)   ← extends existing ui/Badge
                                              ├── PlayOverlay
                                              ├── AnimeKebab        (REUSE existing)
                                              └── AnimeContextMenu  (REUSE existing)
```

New files (all under `frontend/web/src/components/anime/`):
- `card/PosterCard.vue`
- `card/PosterRow.vue`
- `card/MediaTile.vue`
- `card/PosterImage.vue`
- `card/PlayOverlay.vue`
- `card/cardModel.ts` (type `AnimeCardModel` + `toCardModel()`)
- co-located `*.spec.ts` for each component + `cardModel.spec.ts`

Modified files:
- `frontend/web/src/components/ui/badge-variants.ts` — add overlay variants (see §6). **This is a `.ts` file, exempt from the design-system lint**, so color utilities here are fine.
- `frontend/web/src/components/ui/Skeleton.vue` — add a `shimmer` variant.
- `frontend/web/src/components/anime/index.ts` — export new components; remove dead `AnimeCard` export.

Deleted files:
- `frontend/web/src/components/anime/AnimeCard.vue` (dead)
- `frontend/web/src/components/anime/AnimeCardSkeleton.vue` (dead; replaced by PosterImage's built-in skeleton + a `PosterCard` skeleton state)

---

## 4. Design-system constraints (MUST follow)

Read `frontend/web/src/styles/DESIGN-SYSTEM.md` before styling. The build-enforced lint (`frontend/web/scripts/design-system-lint.sh`) scans **`*.vue`** files and FAILS the build on:

1. **Off-palette Tailwind color classes** — `(text|bg|border|ring|from|to|via|fill|stroke|placeholder|divide|outline|decoration|shadow)-(red|amber|yellow|emerald|green|blue|sky|purple|violet|gray|slate|zinc)-(50…900)`. **These are forbidden in `.vue`.** Use semantic tokens instead: `text-success`, `bg-warning`, `text-destructive`, `text-info`, `text-muted-foreground`, etc.
   - **EXEMPT brand hues (allowed):** `cyan`, `pink`, `orange`, `rose`, `indigo`, `teal`, `lime`. So `bg-cyan-500/18`, `text-cyan-200` are fine directly in `.vue`.
2. **No hardcoded hex** in `.vue` outside the allowlist.
3. **No deprecated `var(--ink)`/`var(--accent)`/`var(--pink)`**.

**Implication for this spec:** status/rating colors that map to red/amber/green/emerald MUST be expressed as semantic tokens (`success`/`warning`/`destructive`/`info`) OR be defined inside `badge-variants.ts` (the `.ts` cva file, lint-exempt) and consumed in `.vue` only via `<Badge variant="…">`. Cyan/pink/rose/etc. may be used directly. **Zero scoped `<style>` blocks** in the new components — utility classes only.

Other governance rules: reuse `@/components/ui` primitives; only `font-medium`/`font-semibold` weights; verify visual changes in a real browser at desktop + mobile (DS-NF-06) — jsdom cannot catch Tailwind v4 cascade bugs.

---

## 5. Data model — `cardModel.ts`

One normalized shape consumed by all three components. The normalizer accepts the loose/varied raw objects the surfaces currently hold and maps them once.

```ts
export interface AnimeCardModel {
  id: string | number
  // Localized title is resolved by the component via getLocalizedTitle();
  // the model carries the raw locale fields so resolution stays reactive.
  name?: string
  nameRu?: string
  nameJp?: string
  titleFallback: string          // pre-resolved/raw title if locale fields absent
  poster: string                 // normalized from coverImage | poster_url | cover_image_url
  year?: number
  episodes?: number              // total episode count
  rawGenres?: { name?: string; nameRu?: string }[]
  genreFallback?: string

  rating?: number                // Shikimori 0–10
  siteRating?: { average_score: number; total_reviews: number } | null

  // Airing / schedule
  isOngoing?: boolean            // drives the conditional "Ongoing" label (Shape A)
  nextEpisodeNumber?: number
  nextEpisodeAt?: string         // ISO; rendered as "Day HH:MM"

  // Per-user (optional; null for anon)
  listStatus?: 'watching' | 'plan_to_watch' | 'completed' | 'on_hold' | 'dropped' | null
  progress?: {
    latest_episode: number
    episodes_count: number
    episodes_aired: number
    completed: boolean
    dropped: boolean
  } | null

  // Contextual extras
  rank?: number                  // Shape B "top" variant watermark
  relationLabel?: string         // related rail caption (Shape A)
  episodeNumber?: number         // Shape C (continue-watching / episode)
  progressPct?: number           // Shape C progress bar 0–100
}

export function toCardModel(raw: any): AnimeCardModel
```

`toCardModel()` rules:
- `poster` ← `raw.coverImage ?? raw.poster_url ?? raw.cover_image_url ?? raw.anime?.poster_url ?? ''`
- locale fields ← accept both camel and snake (`nameRu ?? name_ru`, etc.)
- `isOngoing` ← `raw.isOngoing ?? (!!raw.next_episode_at) ?? (raw.status === 'ongoing')`
- never throw; missing fields → undefined/null and the component hides the corresponding zone.
- DROP `quality` and `hasDub` — they are no longer rendered (see decisions §9).

Surfaces map their raw object to a model once (in the view or a small adapter), then pass `:model`.

---

## 6. The pill / badge system (one component, Hero-aligned)

All overlay chips (ongoing, watch-status, progress, ratings, airing, next) are **one** `<Badge>` component using cva variants — NOT hand-rolled spans. Visual language matches the Hero spotlight chips (`components/home/spotlight`): **pill-shaped (`rounded-full`), `line-height:1`, mono uppercase for labels, tabular mono for numbers, subtle tinted background + a matching 1px ring (legible on any poster), a glowing pulse dot for live states.**

### 6.1 Extend `ui/badge-variants.ts`

Add a `pill` size and overlay variants. (This `.ts` file is lint-exempt, so emerald/amber/etc. literals are acceptable here — but prefer semantic-token utilities where they exist so theming stays centralized.)

```ts
// size
pill: 'px-2 py-1 text-[10px] leading-none rounded-full gap-1'

// variants (overlay family) — bind to tokens; cyan is brand-exempt
ratingOverlay:   'bg-black/60 text-warning backdrop-blur-sm ring-1 ring-white/10 font-mono tabular-nums'
siteOverlay:     'bg-black/60 text-cyan-300 backdrop-blur-sm ring-1 ring-white/10 font-mono tabular-nums'
progressOverlay: 'bg-black/60 text-white/90 backdrop-blur-sm ring-1 ring-white/10 font-mono tabular-nums'
nextOverlay:     'bg-cyan-500/16 text-cyan-200 ring-1 ring-cyan-400/30 font-mono tabular-nums'

// state labels (uppercase, mono, letter-spacing applied via class on use: tracking-wider)
ongoing:    'bg-success/15 text-success ring-1 ring-success/30 uppercase font-mono'
airing:     'bg-success/15 text-success ring-1 ring-success/30 uppercase font-mono'
statusWatching:  'bg-cyan-500/18 text-cyan-200 ring-1 ring-cyan-400/34 uppercase font-mono'
statusPlanned:   'bg-white/14 text-white/90 ring-1 ring-white/22 uppercase font-mono'
statusCompleted: 'bg-success/18 text-success ring-1 ring-success/30 uppercase font-mono'
statusOnHold:    'bg-warning/18 text-warning ring-1 ring-warning/30 uppercase font-mono'
statusDropped:   'bg-destructive/18 text-destructive ring-1 ring-destructive/30 uppercase font-mono'
```

### 6.2 Glow dot

Live states (`ongoing`, `airing`) prefix a 6px pulsing dot: `<span class="w-1.5 h-1.5 rounded-full bg-current shadow-[0_0_7px_currentColor] motion-safe:animate-pulse" />`. Respect `prefers-reduced-motion` (use `motion-safe:`).

### 6.3 Status → variant mapping

| `listStatus` | Badge variant | Label i18n key |
|---|---|---|
| watching | statusWatching | profile.watchlist.watching |
| plan_to_watch | statusPlanned | profile.watchlist.planToWatch |
| completed | statusCompleted | profile.watchlist.completed |
| on_hold | statusOnHold | profile.watchlist.onHold |
| dropped | statusDropped | profile.watchlist.dropped |

### 6.4 Rating badges

- Shikimori: variant `ratingOverlay`, `★` glyph (inline SVG star), `rating.toFixed(1)`.
- Site rating: variant `siteOverlay`, `◆` glyph, `average_score.toFixed(1)`, only when `siteRating.total_reviews > 0`.
- The `◆` diamond glyph is **kept for now** but is an isolated, easily-swappable token (single source) — design may revisit later.

### 6.5 Pill centering

No fixed pixel height. Use padding + `leading-none` (line-height:1) so text is optically centered over posters. (This fixes the observed "1px off" baseline.)

---

## 7. Component specs

Shared interaction conventions:
- The outer element is `class="group"` so `group-hover:` reveals overlays (required by `AnimeKebab`).
- Full-card click navigates to `/anime/{id}` (SPA `router-link`); the kebab sits above it (`z-20`) and `@click.prevent.stop`.
- Hover effects honor `prefers-reduced-motion`.
- Utility classes only; no scoped CSS.

### 7.1 `PosterCard.vue` (Shape A — vertical 2/3)

**Props:**
```ts
{
  model: AnimeCardModel
  menuOpen?: boolean          // for kebab open state
  showKebab?: boolean         // default true
  hoverZoom?: boolean         // default true — poster scale-110 on hover
}
```
**Emits:** `openMenu: [el: HTMLElement]` (wired to AnimeKebab → parent opens AnimeContextMenu).

**Layout / zones:**
- Container: `rounded-xl overflow-hidden bg-white/5 border border-white/10`. Poster: `<PosterImage aspect="2/3">`.
- **Top-left (`tl`)** — conditional **`● ONGOING`** label (`Badge variant=ongoing size=pill` + glow dot). Shown only when `model.isOngoing`. (Replaces the removed DUB/quality badges.)
- **Top-right (`tr`)** — rating badges stacked, right-aligned. **Fades out on hover** (`group-hover:opacity-0`) so the kebab owns the corner.
- **Bottom-left (`bl`)** — `listStatus` pill (if any) then `progress` pill (if in-progress & not completed), stacked, **left-aligned at natural width** (do NOT equalize widths). Progress is the neutral `progressOverlay` glass pill rendering `EP {latest}/{total}` (no purple).
- **Next strip** — when `isOngoing` && `nextEpisodeAt`: a one-line bottom gradient strip, `font-mono`, `whitespace-nowrap overflow-hidden text-ellipsis`, format **`◷ EP {nextEpisodeNumber} - {Day HH:MM}`** (Moscow tz, matching existing `ColumnItem` formatting). One line only.
- **Center play** — `<PlayOverlay>` appears on hover (springy scale).
- **Kebab** — corner top-right, reuse `<AnimeKebab>` (appears on hover; cyan + 12° rotate on its own hover). **Decision: corner kebab + center play** (the "both controls centered" experiment was rejected).
- **Footer** — title `<h3>` `font-medium line-clamp-2` with **reserved 2-line min-height** (`min-h-[2.64em]`) so 1- and 2-line footers align; 3-line titles truncate with ellipsis. Meta row `text-xs text-muted-foreground` **single line `truncate`** = `{year} · {episodes} ep · {genre}` (long genres ellipsis-clip).
- Title turns `text-cyan-400` on `group-hover`.

**Hover effect set:** lift `-translate-y-1.5` (~6px) + shadow glow + border brighten; scrim fade-in; poster zoom 1.08 (if `hoverZoom`); play-button springy pop; kebab fade-in; ratings fade-out; title→cyan.

**Container clearance:** consuming grids MUST provide top clearance (e.g. `pt-3` on the grid or `mt-1.5` on items) so the hover-lift never clips/overlaps the row above.

### 7.2 `PosterRow.vue` (Shape B — compact thumb + meta)

**Props:**
```ts
{
  model: AnimeCardModel
  variant: 'ongoing' | 'top' | 'announced' | 'plain'   // default 'plain'
  menuOpen?: boolean
  showKebab?: boolean   // default true
}
```
**Emits:** `openMenu`.

**Layout:**
- Grid `grid-cols-[48px_1fr] gap-3 p-2.5 rounded-xl bg-white/[0.035] border border-white/10`.
- Thumb: 48×72 (2/3) `rounded-lg`.
- Body: title (`truncate`, →cyan on hover) + meta (`{year} · {episodes} ep`) + a **chips row** (slot or computed): Shikimori `★` chip, `● Airing` (variant=airing) for ongoing, `◷ {Day HH:MM}` next chip — all pills.
- **Rank watermark** — ONLY when `variant === 'top'` && `model.rank`: a large numeral behind content, `text-cyan-400/10`, `z-0`, `select-none`, `pointer-events-none`. (Use class `is-topN` / utilities — avoid the literal `top-N` Tailwind collision noted in current `ColumnItem`.)
- **Kebab** — reuse `<AnimeKebab position="top-right">`, hover-reveal on **every** variant (not just top).

### 7.3 `MediaTile.vue` (Shape C — 16:9 landscape)

**Props:**
```ts
{
  model: AnimeCardModel    // uses poster, episodeNumber, episodes, progressPct, title
  episodeNumber?: number
  total?: number
  progressPct?: number     // 0–100
}
```
**Layout:**
- Container: **explicit width** (e.g. `w-[248px]` or responsive) + `aspect-video rounded-xl overflow-hidden border border-white/10`. Poster via `background`/`<img absolute inset-0 object-cover brightness-75>`.
- **CRITICAL:** any parent rail/flex MUST use `items-start` (not the default `items-stretch`) AND the tile must keep an explicit width, so a taller sibling can't stretch the tile into a square. (This was the observed "square" bug.)
- Center `<PlayOverlay>` on hover; bottom info overlay (`Episode {n} · {total}` mono + title); 3px cyan progress bar (`shadow-[0_0_10px] text-cyan-400`) when `progressPct > 0 && < 100`.
- Collections-rail reuse = a `MediaTile` variant whose overlay shows collection title + item count instead of episode info (optional follow-up; can be a `variant` prop).

### 7.4 `PosterImage.vue` (primitive)

**Props:** `{ src: string; alt: string; aspect: '2/3' | '16/9'; rounded?: string; class?: string }`.
Behavior: animated skeleton placeholder (see Skeleton shimmer) until load; image fades in (`opacity` transition); `loading="lazy"`; on error, swap to `getImageFallbackUrl(src)` once (guard with `dataset.fallback`). Encapsulates the ~4 duplicated lazy/fallback implementations.

### 7.5 `PlayOverlay.vue` (primitive)

Centered circular play button, hidden by default, revealed by parent `group-hover`. Cyan fill (`bg-cyan-500/90`), white play SVG, springy `scale` pop, cyan glow shadow. `size` prop (default 50px; MediaTile may use 52px). Decorative (`aria-hidden`) — the card link is the real target.

### 7.6 Reused as-is

- `AnimeKebab.vue` — the round 3-dot button (hover reveal, cyan + rotate, `kebab-glow`). No changes.
- `AnimeContextMenu.vue` — `DropdownMenu`-based menu (poster + title + scores header, status actions with ✓ on current, Share). No changes. Parents open it from the `openMenu` emit, exactly as Browse/Anime do today.

### 7.7 `Skeleton.vue` — add shimmer

Add a `variant="shimmer"` (default stays `pulse`) implementing a diagonal sweep:
`bg-[linear-gradient(110deg,transparent_30%,rgba(255,255,255,0.10)_50%,transparent_70%)] bg-[length:200%_100%] animate-[shimmer_1.4s_infinite]` with a `@keyframes shimmer { to { background-position:-200% 0 } }` registered in the Tailwind/theme layer (not scoped). Used by `PosterImage` and the `PosterCard` loading state.

---

## 8. Migration map (surface → component)

| # | Surface | File | Target |
|---|---|---|---|
| 1 | Browse grid | `views/Browse.vue` | `PosterCard` (swap existing `AnimeCardNew`) |
| 2 | Anime-detail related rail | `views/Anime.vue` | `PosterCard` in `Carousel` + relationLabel |
| 3–5 | Home Ongoing/Top/Announced | `views/Home.vue` + `components/home/ColumnItem.vue` | `PosterRow variant=ongoing/top/announced`; delete ColumnItem scoped CSS |
| 6 | Continue Watching | `components/home/ContinueWatchingRow.vue` | `MediaTile` |
| 7 | Collections rail | `components/home/CollectionsRow.vue` | `MediaTile` (collection variant) |
| 9 | Schedule grid | `views/Schedule.vue` | `PosterRow variant=plain` (with next chip) |
| 10 | Profile watchlist table | `views/Profile.vue` | `PosterRow variant=plain` (or keep table cell using `PosterImage`) |
| 11 | Profile watchlist grid | `views/Profile.vue` | `PosterCard` (removes the hand-rolled duplicate) |
| 12 | Collections detail grid | `views/Collections.vue` | `PosterCard` |
| 13 | Activity feed | `components/ActivityFeed.vue` | `PosterImage` (decorative thumb) |
| 14 | Admin recs | `views/admin/AdminRecs.vue` | `PosterRow variant=plain` or `PosterImage` thumb |
| 8 | Hero Spotlight | `components/home/spotlight/*` | OUT OF SCOPE |

Each migrated surface maps its raw objects via `toCardModel()` and passes `:model`. Containers keep their own grid/rail layout but standardize on `gap-4`, `rounded-xl`, and top clearance for the lift.

---

## 9. Decisions log (authoritative)

1. **Architecture:** Approach 1 (3 layout components + shared primitives + 1 data contract). Rejected: a single mega-component with a `layout` prop (prop-soup), and building on `ui/Card` (poster overlay pattern fights it).
2. **Scope:** all 3 shapes. Hero Spotlight deferred to its own session.
3. **No HD/quality badge.** Removed entirely.
4. **No DUB badge.** Removed; replaced top-left by a **conditional `● Ongoing`** label shown only while airing.
5. **Ratings:** Shikimori `★` (amber/`warning`) + site `◆` (cyan), stacked top-right, fade on hover. `◆` glyph kept for now, isolated for easy future swap.
6. **Bottom-left pills:** status + progress, **left-aligned at natural width** (not equal-width, not merged). Progress is neutral glass `EP n/total` — **purple dropped**.
7. **Next strip:** one line, `◷ EP {n} - {Day HH:MM}`.
8. **Title:** `line-clamp-2` + reserved 2-line min-height; meta single-line ellipsis (handles long genres, e.g. "Путешествие во времени").
9. **Controls:** corner kebab (reused `AnimeKebab`) + center play. The "both controls centered" experiment was **rejected**.
10. **Poster hover-zoom:** keep, subtle (1.08), exposed as `hoverZoom` prop (default true) so it's trivially toggled off per surface.
11. **Pills:** one `Badge` system, Hero-aligned (pill shape, mono kicker, tinted bg + matching ring, glow dot, `line-height:1`). Token-bound to pass the design-system lint.
12. **Top clearance** on grids so the hover-lift never overlaps.
13. **MediaTile** must set explicit width and parents use `items-start` to avoid flex-stretch squaring.
14. **Skeleton shimmer** added to `ui/Skeleton.vue`.
15. **Zero scoped CSS** in new components; delete dead `AnimeCard.vue` + `AnimeCardSkeleton.vue`.

---

## 10. Accessibility

- Card link has an `aria-label` of the localized title; play overlay is `aria-hidden`.
- Kebab: `aria-haspopup="menu"`, `aria-expanded`, keyboard-activable (already in `AnimeKebab`).
- All hover/scale/pulse animations gated behind `motion-safe:` / `prefers-reduced-motion`.
- Decorative posters (Activity feed) keep `aria-hidden` + `tabindex="-1"`.
- Color is never the sole signal — status pills carry text labels, live states carry a dot + label.

---

## 11. Testing & acceptance

**Per-component (`*.spec.ts`, Vitest):** props render correct zones; conditional zones hide when data absent; status→variant mapping; `toCardModel` field normalization (camel/snake, poster fallbacks, missing fields → no throw); rating/progress visibility thresholds.

**Type:** `bunx tsc --noEmit` / `vue-tsc` clean (discriminated props narrow).

**Design-system lint:** `bash frontend/web/scripts/design-system-lint.sh` returns 0 errors over the new `.vue` files; prove fail-path unaffected via `--selftest`.

**i18n:** any new keys added to BOTH `locales/en.json` and `locales/ru.json`; locale-parity tests pass.

**In-browser smoke (DS-NF-06, mandatory):** verify each migrated surface at desktop + mobile widths — pill alignment, hover-lift clearance, MediaTile 16:9, truncation, kebab → context menu. jsdom/vitest cannot catch Tailwind v4 cascade bugs.

**Acceptance criteria:**
- All 13 in-scope surfaces render via the new components; no inline bespoke card markup remains in them.
- `ColumnItem.vue` scoped CSS removed (or component deleted) in favor of `PosterRow`.
- Dead `AnimeCard.vue` / `AnimeCardSkeleton.vue` deleted.
- No new scoped `<style>` in `card/*` components.
- Lint, type, unit, i18n-parity all green; browser smoke verified.

---

## 12. Project metric scoring (per `.planning/CONVENTIONS.md`)

- **UXΔ = +3 (Better)** — consistent, polished, professional cards across the app; clearer status/progress signals; fixes alignment + overlap papercuts.
- **CDI = 0.06 × 34** — touches many surfaces (high spread) but each change is mechanical swap-to-shared-component; net coherence INCREASES (removes ~12 bespoke impls + scoped CSS). Effort 34 (Fibonacci) for full migration; can be split per-surface.
- **MVQ = Griffin 88%/85%** — disciplined consolidation (Griffin: structure + craft), high slop-resistance because it deletes divergence rather than adding a layer.

---

## 13. Suggested implementation order

1. `cardModel.ts` (+ spec) — the contract everything depends on.
2. `ui/badge-variants.ts` overlay variants + `ui/Skeleton.vue` shimmer.
3. Primitives: `PosterImage`, `PlayOverlay` (+ specs).
4. `PosterCard` (+ spec) → migrate Browse, then Profile grid (kills the duplicate), Collections grid, related rail.
5. `PosterRow` (+ spec) → migrate Home rails (retire ColumnItem CSS), Schedule, Profile table, Admin.
6. `MediaTile` (+ spec) → Continue-Watching, Collections rail.
7. Delete dead files; update `anime/index.ts`.
8. Full verification pass (§11) + browser smoke; then changelog + redeploy via `/animeenigma-after-update`.
```
