# Hero Spotlight × Design System — alignment design (DRAFT v1)

> Status: **APPROVED & IMPLEMENTED 2026-06-10** (artifact v2 with live screenshots:
> `.brainstorm/content/spotlight-ds-v2.html`). User decisions: A-1 brand triad; FeaturedCard
> on primitives with PLAIN year/episodes and pills only for genres/status/scores; CTA row
> pinned bottom-left; lucide icons for score glyphs; all-9 rollout in one pass.

## Goal

Align the home-page hero spotlight (9-card carousel, `frontend/web/src/components/home/spotlight/`)
with the Neon Tokyo DS — **without touching carousel mechanics or the BE contract**.

## Audit findings (2026-06-10)

| # | Finding | Where |
|---|---|---|
| 1 | 6-hue accent rainbow (cyan/purple/sky/amber/teal/green) lives in `tokens.ts` — a `.ts` file, so it evades the `.vue`-only DS lint gate | `tokens.ts` |
| 2 | 12-hue genre color map (red/blue/yellow/…/fuchsia) | `tokens.ts` `featured.genreColors` |
| 3 | 10/10 SFCs ship bespoke scoped CSS; only ONE `ui/` primitive import (Avatar) across the whole dir | all cards |
| 4 | `font-weight: 800/700` ×4 (DS scale is medium/semibold only) | FeaturedCard, PlatformStatsCard |
| 5 | Hand-rolled `.btn-primary-hero` instead of Button v2.0 primitive | FeaturedCard |
| 6 | 27 raw `rgba()` lines in style blocks (evade the hex-only lint rule) | 8 files |
| 7 | Shikimori score rendered with a **cyan star** — violates DS §6 (amber ★ = Shikimori, cyan ◆ = ours) | FeaturedCard meta row |
| 8 | Locked inline-vs-overlay badge rule (2026-06-05) not applied to poster-overlaid pills | FeaturedCard, others |

## Proposal (v1 — pending user choices)

1. **Accents → brand triad** (option A-1, recommended): cyan = content core (featured, personal_pick,
   platform_stats), pink = live/personal (now_watching, continue_watching_new), violet = meta/service
   (random_tail, latest_news, telegram_news, not_time_yet). Cards are differentiated by kicker ICON,
   not hue. Alternative A-2: semantic status tokens (success=live, warning=waiting). **User to pick.**
2. **`SpotlightCardShell.vue`** — shared frame (kicker + title + body slot + CTA row), padding
   `p-4 md:p-6 lg:p-8`, background via existing `SpotlightBackdrop`. Cards migrate one-by-one
   (Strangler); typed dispatch chain in HeroSpotlightBlock unchanged.
3. **Badges:** on-poster → `Badge overlay=true` (dark glass + accent text); on-glass → tinted Badge.
   Genre tags → neutral overlay badges (genre is meta, not status).
4. **CTAs → Button primitive** (`default` / `ghost`); delete `.btn-primary-hero`.
5. **Score glyphs:** Shikimori → amber ★; AnimeEnigma → cyan ◆ `ScoreDiamond`.
6. **Typography:** only `font-medium` / `font-semibold`; titles in `font-display`.
7. **Keeps:** `SpotlightIcon` (lucide keep-list), `CarouselControls`/`CarouselDots` (bespoke-keep
   governance §5; only ensure 44px touch targets), backdrop scrims/blur, today's image lazy-loading.

## Resolved decisions (user, 2026-06-10)

1. Accents: **A-1 brand triad**.
2. FeaturedCard meta row: **plain text for year/episodes; pills for genres, status, scores**.
3. Shell rollout: **all 9 in sequence** (delivered in one pass).
4. Score glyphs: **lucide icons** (amber `Star` for Shikimori; `ScoreDiamond` stays canonical
   for site scores when they reach the payload). CTA icons also lucide (Play/Shuffle/
   ExternalLink/ArrowDown); SpotlightIcon remains for kickers (lucide keep-list).

## Implementation summary

- `SpotlightCardShell.vue` (+spec) — shared kicker/body/CTA-bottom-left frame, DS padding scale.
- `tokens.ts` — triad accents + `accentText` map; genreColors rainbow deleted; latest_news
  pills → semantic `bg-success/15 text-success` / `bg-warning/15 text-warning`.
- All 9 cards migrated; legacy `.btn-primary-hero`/`.cta-hero`/`.cta-card`/`.cta-text`
  usages in spotlight replaced with Button-variant classes; on-poster pills → `Badge overlay`;
  font weights clamped to medium/semibold; bespoke scoped CSS reduced to justified keeps
  (hero scrim, pulse dot, line-clamp:7, stats gradient, tile highlight).
- Verified: 228/228 spotlight tests, vue-tsc clean, DS lint PASS, in-browser smoke of all
  9 cards at 1440px + 390px (screenshots in artifact v2).

## Metrics

- **UXΔ = +2 (Better)** — consistency with the rest of the site; zero new patterns.
- **CDI = 0.03 * 13** — spread: `components/home/spotlight/` + `tokens.ts` only; shift low (mechanics untouched).
- **MVQ = Griffin 85%/85%** — stitches existing primitives into one body; invents nothing new.
