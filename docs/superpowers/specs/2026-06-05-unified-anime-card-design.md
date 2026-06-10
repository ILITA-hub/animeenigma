# Unified Anime Card — Design Spec (final)

**Date:** 2026-06-05
**Status:** Design FROZEN — ready for implementation planning
**Hosted gallery:** `design-v17-gallery.html` (brainstorm companion; iteration history v7→v16 linked in-page)
**Scope:** 3 card shapes (PosterCard, PosterRow, MediaTile). Hero Spotlight deferred to a separate session.

## Goal

Standardize the anime card on the Neon-Tokyo design system and **de-fragilize** the home-page CSS.
Today: 14 card surfaces, ~12 bespoke inline, 5+ aspect ratios, badges built 3 ways. Replace with
**3 layout components over shared primitives + one data contract**.

## Architecture (Approach 1)

```
toCardModel(src) ─► AnimeCardModel ─► PosterCard | PosterRow | MediaTile
                                          └── PosterImage, Badge, AnimeKebab + AnimeContextMenu, Skeleton
```

### Data contract

```ts
interface AnimeCardModel {
  id: string | number
  href: string                 // /anime/:id
  title: string                // localized
  coverImage: string
  year?: number
  episodes?: number
  primaryGenre?: string        // localized
  malScore?: number            // ★ amber → overlay ac-amber / chip.score
  siteScore?: number           // ◆ diamond cyan → overlay ac-cyan / chip.site
  listStatus?: 'watching' | 'plan_to_watch' | 'completed' | 'on_hold' | 'dropped'
  progress?: { current: number; total: number }
  airing?: boolean             // → ONGOING (green + pulse), only while true
  nextEpisode?: { ep: number; when: string }  // Row / MediaTile
}
```

## Locked decisions

| Area | Decision |
|------|----------|
| Architecture | 3 layout SFCs + `toCardModel()` + `PosterImage` |
| Badges | **Inline** (tinted) on surfaces · **Overlay** (dark-glass `bg-black/.62` + blur + accent text) on posters, with top/bottom scrims. One `Badge` + `overlay` prop. |
| Scores | ★ amber = MAL · ◆ diamond = AnimeEnigma · snug width + `tabular-nums` (icons align) · **stay visible on hover** |
| Controls | Play + kebab **centered, equal size**, opacity reveal. **No zoom, no GPU hacks.** |
| Skeleton | **Drift** shimmer — `background-image` gradient + animated `background-position` (NOT the `background` shorthand, NO `!important` — both break the animation in the cascade). Base posters use `background-color` only so the gradient survives. Same frame + shared containers as loaded (no size jump). |
| Context menu | C-top layout · **Open in new tab** pinned at top, own separator group |
| PosterRow | Hero-rail info set (ongoing chip · ★/◆ scores · next-ep line · season chip · rank numeral). **1-line title + fixed row height → info aligns across columns.** **Centered-glass kebab** (vertically centered on right edge, hover reveal). |
| MediaTile | Variant ② — kicker + title + next-ep, overlaid on a 16/9 poster · overlay skeleton (placeholder bars over a drifting poster) |

## Components

| Item | Status |
|------|--------|
| `toCardModel()` + `AnimeCardModel` type | NEW |
| `PosterCard.vue` (2/3) · `PosterRow.vue` (hero) · `MediaTile.vue` (16/9) | NEW |
| `PosterImage.vue` — lazy + fallback + skeleton + scrims | NEW (tiny) |
| `Badge` + new `overlay` prop | REUSE + extend |
| `AnimeKebab` + `AnimeContextMenu` (+ Open-in-new-tab item) | REUSE + 1 item |
| `Skeleton` + drift variant | REUSE + extend |

## Token units (reference)

Radius = px (`--r-sm/md/lg` = 8/12/16) · padding/type = rem (Tailwind, 1 = 0.25rem) · color = hex (solid) +
rgba (tints/scrims/dark-glass) · glow/shadow = px+rgba · motion = s/ms · border = always 1px (token carries
color). Source: `frontend/web/src/styles/main.css`. Must pass `design-system-lint.sh`.

## Rollout — 14 surfaces

1. **Browse + Anime** (already `AnimeCardNew`) → `PosterCard` first (lowest risk, proves parity).
2. Home rails → `PosterRow`.
3. Continue-watching / next-ep → `MediaTile`.
4. Delete dead `AnimeCard.vue` + `AnimeCardSkeleton.vue` (verified zero usages).

## Metrics

- **UXΔ = +3 (Better)** — one consistent card, fewer aspect ratios, fragility removed.
- **CDI = 0.04 * 21** — 3 SFCs + normalizer + `PosterImage`; 14 call-site migrations across packages.
- **MVQ = Griffin 88%/85%** — composed of proven shipping parts, low slop.

## Deferred

- Hero Spotlight 9-card redesign — separate session.
