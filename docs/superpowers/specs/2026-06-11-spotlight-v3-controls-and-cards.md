# Spotlight v3→v4 — carousel controls + 7-card design review

> Status: **v4 LOCKED & IMPLEMENTED 2026-06-11** (all variants chosen and shipped)
> (artifact: `.brainstorm/content/spotlight-ds-v4.html`, served at localhost:3000).
> Follow-up to the shipped v2 DS alignment (`2026-06-10-spotlight-ds-alignment-design.md`).

## Deployed during this review (2026-06-11)

1. **Kicker faux-bold fix** — JetBrains Mono ships 400/500 only; shell kicker used
   `font-semibold` (600) → faux-bold blur ("мыльный" season text). Reverted to
   `font-medium` + `tracking-[0.12em]`. LIVE.
2. **PlatformStats 4-tiles fix (two layers)** — (a) window fallback: failed/zero
   window no longer drops the tile; (b) **parallel tile queries**: the resolver runs
   under the aggregator's 800ms per-card deadline and sequential `increase[7d]`
   queries starved the tail into `context deadline exceeded` — the real cause of the
   2-3-tile days. One goroutine per metric; rng stays on the resolver goroutine
   (deterministic, race-free). Verified live: 4 tiles. Snapshot + stats keys flushed
   (`spotlight:snapshot:*`, `spotlight:stats:<date>`).

## Locked (user, 2026-06-11)

- **A-1** icon-menu below frame, active item expands to accent icon+label pill;
  skeleton reserves the row with shimmer circles (zero CLS). **A-2** (in-frame
  progress segments) kept in reserve "для разнообразия потом". A-3 rejected.
- **E-1** terminal changelog (`$ animeenigma --updates`, [FEAT]/[FIX]/[PERF]).
- **F-2** "N человек смотрят прямо сейчас" counter + compact session list.
  **TODO (future session):** Watch Together integration — "join" badge + CTA for
  invite-open rooms; needs `wt_room_id?` on NowWatching sessions.
- **G-3** pinned-note sticker NotTimeYet.
- FeaturedCard + PlatformStats designs locked (v3); v2 base (triad, shell, CTA
  bottom-left, overlay badges, lucide scores) locked.
- Practice locked: every design review shows a "current prod" screenshot per card.

## v4 — locked choices (user, 2026-06-11) — ALL IMPLEMENTED

| Q | Lock | Implementation notes |
|---|---|---|
| B | RandomTail B-1 v2 | deal-in animation replacing the buggy 5-card overlay (poster + 2 ghost deck cards fly into the resting stack, 550ms, no content occlusion) + density: year/status pill/description clamp-3/«из N тайтлов». Sub-question: keep ghost «⤮ Ещё разок» re-roll button (needs a tiny reroll endpoint)? |
| C | PersonalPick C-2 v2 | scrollable right list up to 6 recs (desktop, fade mask + thin cyan scrollbar; resolver cap 3→6 is a one-liner), horizontal poster swipe-row on mobile. Bug surfaced: «+N ещё» links to `/recs` which DOES NOT EXIST (only `/admin/recs/:user_id`) → 404. Options: (а) link `/browse?sort=recommended` now · (б) build /recs page · (в) drop the button. |
| D | TelegramNews — new round | **D-4** hero post (photo + overlay date) + telegram bubbles right + «Подписаться» ghost (recommended) · D-5 "phone feed" framed channel mock. |
| H | ContinueWatchingNew | **H-4** stepper + context (status/genres/description/season progress bar/release date; needs `next_episode_at`+`season_episodes` in payload) (recommended) · H-5 stepper × giant EP number. |
| 📱 | Mobile layouts | 390px mockups for A-1/B-1/C-2/D-4/E-1/F-2/G-3/H-4 in the artifact — confirm or annotate. |
| PS | DS PosterCard reuse | Verdict: point-wise yes, total no. Plan: add `chrome: 'full'\|'bare'` prop to PosterCard, adopt in PersonalPick podium/recs (≥96px, real catalog items, context menu useful); keep bare `<img>`+proxy for decorative posters (deck, sticker, thumbs, backgrounds). |

## Standing TODO

- Spotlight card authoring guidelines — `docs/spotlight-card-guidelines.md`
  (written this session; linked from CLAUDE.md §Adding a Spotlight Card Type).
- F-2 × Watch Together (above).

## Metrics (locks + recommended v4 picks)

- **UXΔ = +3 (Better)** · **CDI = 0.03 * 21** · **MVQ = Kraken 82%/85%**

## v4 final locks (this review round)

- **B-1 v2** — deck deal-in (no overlay) + density; «Ещё разок» KEPT — backed by
  `GET /api/home/spotlight/reroll?exclude=<id>` (catalog handler `GetReroll` +
  gateway passthrough; bypasses the daily cache both ways).
- **C-2 v2** — scrollable 6-rec column (desktop) / poster swipe-row (mobile);
  «Все рекомендации» → `/browse?sort=recommended`.
  **TODO (recorded, task for later): recs service must properly serve
  `/browse?sort=recommended`** — until then browse falls back to its default
  ordering for the unknown sort value.
- **D-4** — hero post + telegram bubbles (user: «ну такой +- но давай ЛОК»).
- **H-4** — stepper + context (status/genres/description/season progress).
- Mobile layouts — approved as mocked.
- **PosterCard question resolved differently**: instead of reusing PosterCard,
  a DEDICATED spotlight primitive set was built on the same DS token base —
  `components/home/spotlight/ui/`: SpotlightTile, SpotlightPoster,
  SpotlightChatBubble, SpotlightStepper, SpotlightProgress (cva variants,
  tokens only, co-located spec).
- personal_pick resolver: AdaptiveSlice → flat cap 6 (1 featured + 5 secondary).
- CarouselDots → A-1 icon menu (32px circles, active expands to accent
  icon+label pill via tokens.accentMenuPill); skeleton reserves the row.

## v4.1 follow-up locks (2026-06-11, same day)

User feedback round on the shipped v4:

- **Cyrillic font subsets** (fix, ce9a0fbc) — all webfonts were latin-only;
  JP-locale systems rendered Cyrillic full-width. cyrillic+cyrillic-ext
  variable woff2 with unicode-range added for Inter/Manrope/JetBrains Mono.
- **Reroll polish** (fix, ce9a0fbc) — deck shuffle loop while fetch+preload
  (256+128 buckets) run; swap on warm cache only (650ms floor);
  SpotlightBackdrop crossfades via keyed-wrapper Transition.
- **ARR-1 LOCKED** (of 4 artifact variants) — nav chevrons moved INTO the
  CarouselDots menu row (flanking the anchors, always visible);
  CarouselControls.vue deleted; in-frame overlays banned (guidelines §4).
  Skeleton row = 7 shimmer slots (chevron + 5 anchors w/ pill + chevron).
- **Touch swipe** — horizontal swipe ≥48px (and >1.2× the vertical travel)
  on the frame navigates; passive listeners, vertical scroll untouched;
  counts as manual nav (stops autoplay).
- **Smooth menu pill** — the active label expands via grid-template-columns
  0fr→1fr (+ padding-left) so activation animates instead of reflowing the
  row; label is always in the DOM (aria-hidden, aria-label carries a11y).
- **Image skeletons** — SpotlightPoster gained a built-in skeleton-shimmer +
  300ms fade-in (complete-check on mount for cache hits, reset on URL swap);
  FeaturedCard 640 hero + TelegramNews hero photo replicate the pattern.
- **Slide prefetch** — HeroSpotlightBlock.cardImageUrls() maps every card
  type to its exact proxy buckets; prefetchSlides() warms them at browser
  idle (2 lanes, starting from current+1) once per mount.
- **Cache flush op** — today's spotlight:personal/trending/snapshot keys
  flushed so the cap-6 recs payload (minted pre-redeploy with 3) refreshed.
