# Anime Card Inventory & Standardization Audit

> Date: 2026-06-04 · Scope: `frontend/web/src/**` · Purpose: pre-work inventory for standardizing the anime card on the Neon-Tokyo design system.

## TL;DR

- There is **one real reusable anime card** in active use: `AnimeCardNew.vue` — but it is wired into only **2 surfaces** (Browse grid, Anime-detail "related" rail).
- **Every other surface that shows an anime poster reinvents the card inline** (Home rails, Continue-Watching, Collections, Schedule, Profile table+grid, Activity feed, Admin recs) → ~10 divergent implementations.
- The "old" `AnimeCard.vue` and `AnimeCardSkeleton.vue` are **dead code** (exported from `components/anime/index.ts`, zero consumers).
- The shadcn-vue `ui/Card*` primitives exist but **no anime card uses them**.
- **5+ different poster aspect ratios** are in play (`2/3`, `140%`, `16/9`, `13:18`, `16:22`, `w-16 h-24`).
- Badges, hover overlays, play buttons, and status pills are each implemented 2–3 different ways, mixing `Badge` component vs bespoke spans, and semantic tokens vs hardcoded `cyan-500 / brand-violet / white/NN`.

---

## 1. Dedicated components (`components/anime/`)

| Component | Status | Used by | Aspect | DS primitives | Notes |
|---|---|---|---|---|---|
| **AnimeCardNew.vue** | ✅ **CANONICAL (active)** | `views/Browse.vue`, `views/Anime.vue` | `aspect-[2/3]` | `Badge` (quality/ratings), `AnimeKebab` | Richest card: dual rating badges (Shikimori + site), list-status pill, episode-progress pill, quality + DUB badges, kebab menu, localized title/genre. Mixes tokens with hardcoded `cyan-500`, `brand-violet`, `white/NN`. |
| **AnimeCard.vue** | ⚠️ **DEAD CODE** | none (only `index.ts` export) | `padding-top:140%` | none (scoped CSS) | Legacy. Red `--destructive` play button, emoji `▶`/`★`, scoped-CSS approach. Safe to delete after confirming. |
| **AnimeCardSkeleton.vue** | ⚠️ **DEAD CODE** | none (only `index.ts` export) | `aspect-[2/3]` | none | Matches AnimeCardNew dims but is **not** actually rendered anywhere — each surface rolls its own loading state. |
| **EpisodeCard.vue** | ✅ active (episodes, not anime) | player/season views | `aspect-video` (16:9) | `Badge` (ep #) | Not an "anime card" but shares the visual language (hover play btn, progress bar, gradient overlay). Listed for design-language consistency. |
| **AnimeKebab.vue** / **AnimeContextMenu.vue** | ✅ shared | AnimeCardNew, ColumnItem, Profile, Schedule | — | — | The ⋮ action affordance + context menu. Already shared; good model for how the card itself should be shared. |

---

## 2. Where anime are rendered as cards (by surface)

Legend — **Impl**: `Component` = uses AnimeCardNew · `Inline` = bespoke markup.

| # | Surface | File | Impl | Layout | Poster aspect | Badges / extras shown |
|---|---|---|---|---|---|---|
| 1 | **Browse – results grid** | `views/Browse.vue` | **Component** (AnimeCardNew) | grid 2→5 cols | `2/3` | full set (status, ratings, progress, kebab) |
| 2 | **Anime detail – related rail** | `views/Anime.vue` | **Component** (AnimeCardNew) in `Carousel` | horizontal scroll rail | `2/3` | full set + relation label below |
| 3 | **Home – Ongoing rail** | `views/Home.vue` → `home/ColumnItem.vue` | Inline (ColumnItem) | vertical list (56px poster) | `56px` thumb | title, next-episode line, rating, kebab |
| 4 | **Home – Top anime rail** | `views/Home.vue` → `ColumnItem.vue` | Inline (ColumnItem) | vertical list + rank | `56px` thumb | rank numeral, rating |
| 5 | **Home – Announced rail** | `views/Home.vue` → `ColumnItem.vue` | Inline (ColumnItem) | vertical list | `56px` thumb | season/announce variant |
| 6 | **Home – Continue Watching** | `home/ContinueWatchingRow.vue` | Inline | horizontal scroll | **`16/9`** cinematic | poster bg, play-on-hover, progress bar |
| 7 | **Home – Collections rail** | `home/CollectionsRow.vue` | Inline | horizontal scroll | `2/3` | collection title overlay |
| 8 | **Home – Hero Spotlight** | `home/spotlight/cards/*.vue` (9 cards) | Inline (bespoke family) | rotating hero carousel | full-bleed / mixed | see §3 |
| 9 | **Schedule – by-day grid** | `views/Schedule.vue` | Inline `router-link`+`img` | grid 1→4 cols | **`w-16 h-24`** | ep #, next-air time, kebab |
| 10 | **Profile – watchlist table** | `views/Profile.vue` (~L224) | Inline `img` | table row | **`w-12 h-16`** | poster thumb only |
| 11 | **Profile – watchlist grid** | `views/Profile.vue` (~L356) | Inline `div`+`img` | grid 2→5 cols | `2/3` | score badge, status badge, score-edit popover, kebab |
| 12 | **Collections – items grid** | `views/Collections.vue` (~L45) | Inline `router-link`+`img` | grid `auto-fill minmax(160px)` | `2/3` | title below, no badges/kebab |
| 13 | **Activity feed** | `components/ActivityFeed.vue` | Inline `img` (decorative) | list row | `~w-14 h-20` | decorative poster (aria-hidden) |
| 14 | **Admin – recommendations** | `views/admin/AdminRecs.vue` (~L83) | Inline `img` | table row | **`w-10 h-14`** | poster thumb in ranking table |

**Count:** 14 surfaces · **2 use the shared component**, **12 are bespoke**.

---

## 3. Hero Spotlight card family (`home/spotlight/cards/`)

Nine cinematic cards in a rotating carousel. Shared infra: `SpotlightBackdrop.vue`, `SpotlightIcon.vue`, `tokens.ts`. **None use `ui/Card`.** Colors are mixed tokens + hardcoded literals.

| Card | Shows anime poster? | Accent | Build |
|---|---|---|---|
| FeaturedCard | blurred backdrop only | brand-cyan | bespoke, custom hero buttons (not `ui/Button`) |
| ContinueWatchingNewCard | yes (2/3, left) | brand-violet/fuchsia | bespoke + gradient ribbon |
| NotTimeYetCard | yes (2/3, left) | warning | bespoke status pill |
| NowWatchingCard | yes (inline 56×84) | multi (token palette) | bespoke live grid |
| PersonalPickCard | yes (featured + secondary grid) | cyan | bespoke two-zone |
| RandomTailCard | yes (2/3, left) | brand-violet | bespoke + shuffle anim |
| LatestNewsCard | no | warning | bespoke 3-col news tiles |
| TelegramNewsCard | optional thumb | sky/telegram | bespoke post tiles |
| PlatformStatsCard | no | teal | bespoke stat hero |

> Spotlight cards are a distinct "hero" design language and likely **out of scope** for a grid-card standardization (full-bleed + backdrop is incompatible with a compact poster card). Worth standardizing their *tokens/accents* separately, not their layout.

---

## 4. Adjacent anime-content cards

| Component | File | Poster | DS primitives | Resembles anime card? |
|---|---|---|---|---|
| NewEpisodeCard | `notifications/NewEpisodeCard.vue` | 52×72 (13:18) | none | partially (poster+meta, but 1-D row) |
| ThemeCard (OP/ED) | `themes/ThemeCard.vue` | 64px (16:22) | `glass-card` only | loosely (expandable, squarer poster) |
| ActiveSessionsCard | `profile/ActiveSessionsCard.vue` | n/a | uses `ui/Button` | no (not anime) |

---

## 5. Design-system primitives available but unused

`components/ui/Card.vue`, `CardHeader/Content/Footer/Title.vue` — shadcn-vue card API (`variant`, `padding`, `rounded`, `glass-card`). **Zero anime cards consume them.** A standardized `AnimeCard` could be composed on `Card` or remain a dedicated primitive — TBD in the design phase.

---

## 6. Key inconsistencies to resolve (standardization targets)

1. **Adoption gap** — 12 of 14 surfaces bypass the shared component. Standardization = build one canonical card + variants, then migrate inline surfaces to it.
2. **Aspect-ratio sprawl** — `2/3`, `140%`, `16/9`, `13:18`, `16:22`, `w-16 h-24`, `w-12 h-16`, `w-10 h-14`. Need a defined size scale (e.g. `xs/sm/md` poster sizes, all `2/3`) + a separate landscape (16:9) variant for Continue-Watching.
3. **Badge implementation split** — `Badge` component (quality/ratings) vs bespoke `<span>` pills (DUB, progress, status). Consolidate all onto `Badge` variants.
4. **Color/token drift** — hardcoded `cyan-400/500`, `brand-violet`, `fuchsia-500`, `white/NN`, raw `rgba()` shadows alongside semantic tokens. Note: `cyan/pink/rose/violet` are lint-EXEMPT brand hues per DESIGN-SYSTEM.md, but usage is ad-hoc. Define card-scoped semantic tokens (`--card-accent`, status colors) and apply uniformly.
5. **Dead code** — delete `AnimeCard.vue` + `AnimeCardSkeleton.vue` (or repurpose the skeleton as the shared loading state).
6. **Loading states** — every grid rolls its own skeleton; none use `AnimeCardSkeleton`. Standardize.
7. **Layout containers** — grids (`2→5`), `auto-fill minmax(160px)`, vertical lists, horizontal rails, carousels — varied. The card should be container-agnostic; containers can stay per-surface but should share column/gap conventions.

---

## 7. Proposed card "shapes" (for the design phase)

Three distinct shapes emerge — a standardized system likely needs **3 variants**, not one:

- **A. Poster grid card** (vertical 2/3) — Browse, Profile grid, Collections, related rail, Schedule. → unify on `AnimeCardNew` + a `size` prop + slots for footer meta.
- **B. Compact list row** (small thumb + meta) — Home ColumnItem rails, Profile table, Activity, Admin, NewEpisodeCard. → a shared `AnimeListItem` / `PosterThumb` primitive.
- **C. Cinematic landscape** (16:9) — Continue-Watching, EpisodeCard. → a `MediaTile` variant.

Spotlight hero cards (§3) stay their own family.

---

## Appendix A — All 14 surfaces with code

### Shared component (`AnimeCardNew`)

**1. Browse grid** · `views/Browse.vue:129` — grid 2→5, passes full prop set (`anime`, `list-status`, `site-rating`, `progress`, `menu-open`, touch handlers). Richest fill.

**2. Anime-detail related rail** · `views/Anime.vue:1007` — same component inside `Carousel`, but **only `anime` + `menu-open`** are passed (no rating/status/progress), so it renders sparse. Relation label `<p>` sits below the card.

### Inline / bespoke

**3–5. Home rails** · `views/Home.vue` → `home/ColumnItem.vue` — `grid-cols: 56px 1fr`, poster 56×84 (2/3), scoped-CSS `.chip` badges. Variants: `ongoing` (● airing + score + site-score + next-ep line), `top` (rank watermark + score), `announced` (announce + season chips, no score). Has `AnimeKebab`.

**6. Continue Watching** · `home/ContinueWatchingRow.vue` — `.cw-card { aspect-ratio:16/9 }`, poster as `background-image`, play-on-hover 52px, bottom info overlay (ep N · total + title), 3px cyan progress bar. No kebab/rating/status.

**7. Collections rail** · `home/CollectionsRow.vue` — represents a *collection* (not one anime): cover img + title overlay + item-count. No kebab.

**8. Hero Spotlight** · `home/spotlight/cards/*.vue` — 9 bespoke cinematic cards (see §3).

**9. Schedule grid** · `views/Schedule.vue:21` — inline `router-link.flex gap-3 p-3 rounded-lg bg-white/5`, `img w-16 h-24` (64×96), title truncate + episode line + cyan air-time. `AnimeKebab`.

**10. Profile watchlist table** · `views/Profile.vue:224` — inline `img w-12 h-16` (48×64) in `<td>`, title link in separate cell, inline score-edit.

**11. Profile watchlist grid** · `views/Profile.vue:356` — ⚠️ hand-rolled near-clone of AnimeCardNew: `aspect-[2/3] rounded-xl`, score badge top-right (`bg-black/60 text-warning`), status pill bottom (gradient), `AnimeKebab`, status `<Select>` on hover, title + "N/M ep" + date range below.

**12. Collections detail grid** · `views/Collections.vue:47` — `auto-fill minmax(160px)`, inline `router-link`, `aspect-[2/3] rounded-lg bg-white/5` (note `rounded-lg` vs `xl`), poster + title. No badges/kebab.

**13. Activity feed** · `components/ActivityFeed.vue:63` — inline decorative poster (`tabindex=-1 aria-hidden`), `.feed-poster-img`.

**14. Admin recs** · `views/admin/AdminRecs.vue:83` — inline `img w-10 h-14` (40×56) in ranking table + title link.

## Appendix B — Visual diff matrix

| # | Surface | Poster | Container | Play btn | Rating | Status | Progress | Kebab | Build |
|---|---|---|---|---|---|---|---|---|---|
| 1 | Browse | 2/3 | grid 2→5 | ✅56px | ✅✅ | ✅pill | ✅pill | ✅ | Component |
| 2 | Related | 2/3 | carousel | ✅56px | ❌ | ❌ | ❌ | ✅ | Component (sparse) |
| 3 | Home Ongoing | 56×84 | 2-col list | ❌ | ✅chip | ●airing | next-ep | ✅ | CSS comp |
| 4 | Home Top | 56×84 | 2-col list | ❌ | ✅chip | rank# | ❌ | ✅ | CSS comp |
| 5 | Home Announced | 56×84 | 2-col list | ❌ | ❌ | announce+season | ❌ | ✅ | CSS comp |
| 6 | Continue Watching | 16/9 | h-scroll | ✅52px | ❌ | ❌ | ✅3px bar | ❌ | inline |
| 7 | Collections rail | cover | h-scroll | ❌ | ❌ | count | ❌ | ❌ | inline |
| 9 | Schedule | 64×96 | grid 1→4 | ❌ | ❌ | ep+time | ❌ | ✅ | inline |
| 10 | Profile table | 48×64 | table | ❌ | inline-edit | ❌ | N/M col | ❌ | inline |
| 11 | Profile grid | 2/3 | grid 2→5 | ❌ | score badge | ✅pill | N/M | ✅ | inline ⚠️dup |
| 12 | Collections grid | 2/3 | auto-fill 160 | ❌ | ❌ | ❌ | ❌ | ❌ | inline |
| 13 | Activity | thumb | list row | ❌ | ❌ | ❌ | ❌ | ❌ | inline (decor) |
| 14 | Admin recs | 40×56 | table | ❌ | ❌ | ❌ | ❌ | ❌ | inline |

**Sub-element divergence:** rating, status, progress, title-hover, and radius are each implemented ~3 ways (Badge component / scoped-CSS chip / ad-hoc div) across surfaces.

## Appendix C — Three target shapes

### Shape A — `PosterCard` (vertical 2/3)
```
┌─────────────┐
│ [HD][DUB]  ⋮│  top: quality/dub (L) · rating + kebab (R)
│   POSTER    │  aspect-[2/3] rounded-xl; hover scale-110 + scrim + center play
│   2 / 3     │
│ [status]    │  bottom-left: status pill + progress pill
└─────────────┘
  Title (2-line)
  2024 · 12 ep · Action   (optional meta)
```
Covers: Browse (1), Related (2), Profile grid (11), Collections grid (12). Skeleton → real `AnimeCardSkeleton`.
API sketch: `<PosterCard :anime size :rating :status :progress :show-kebab :footer>` — fill level via props so sparse (related) and rich (browse) share one component.

### Shape B — `PosterRow` (compact horizontal thumb + meta)
```
┌────┐ Title (1–2 line)            ⋮
│ ▓▓ │ 2024 · 12 ep
│ ▓▓ │ ●airing / ★8.2 / next ep…   (chips slot)
└────┘  2/3 thumb (xs/sm)
```
Covers: Home rails 3–5 (ColumnItem `variant`), Schedule (9), Profile table (10), Admin recs (14), Activity (13 decorative), NewEpisodeCard.
API sketch: `<PosterRow :anime size :chips :rank :show-kebab>`.

### Shape C — `MediaTile` (16:9 landscape)
```
┌───────────────────┐
│        ▶          │ aspect-video, bg cover
│  Ep 5 · 12        │ bottom overlay: ep/title
│  Title            │
│▓▓▓▓▓░░░░░░░░░░░░░░░│ 3px progress bar
└───────────────────┘
```
Covers: Continue Watching (6), EpisodeCard. Collections rail (7) = title+count overlay variant.

### Out of scope
Spotlight hero family (9 cards) — unify accent tokens only, not layout.
