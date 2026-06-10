# Hero Spotlight × Design System — alignment design (DRAFT v1)

> Status: **DRAFT — awaiting user notes on artifact v1** (`.brainstorm/content/spotlight-ds-v1.html`,
> served at localhost:3000 via the brainstorm companion server).
> Implementation is HARD-GATED until the user approves this spec.

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

## Open questions (artifact §④)

1. Accent option A-1 (triad) vs A-2 (semantic)?
2. FeaturedCard meta row: full overlay-badge treatment vs badges only for score+status?
3. Shell rollout: all 9 cards at once vs worst-3 first (Featured, PlatformStats, RandomTail)?
4. Which current solutions does the user dislike MOST (block refs A-0, B-0, D-1…)?

## Metrics

- **UXΔ = +2 (Better)** — consistency with the rest of the site; zero new patterns.
- **CDI = 0.03 * 13** — spread: `components/home/spotlight/` + `tokens.ts` only; shift low (mechanics untouched).
- **MVQ = Griffin 85%/85%** — stitches existing primitives into one body; invents nothing new.
