# Spotlight v3 — carousel controls + 7-card design review (PROPOSAL)

> Status: **AWAITING USER CHOICES** (artifact: `.brainstorm/content/spotlight-ds-v3.html`,
> served at localhost:3000 via the SSH tunnel). Follow-up to the shipped v2 DS alignment
> (`2026-06-10-spotlight-ds-alignment-design.md`, commit ccc596b8).
> **Nothing here deploys until the user picks variants.**

## Shipped fixes (in code, NOT deployed)

1. **Kicker faux-bold ("мыльный" season text)** — JetBrains Mono is loaded at weights
   400/500 only; the shell kicker set `font-semibold` (600) → browser faux-bold synthesis
   → blurry rendering. `SpotlightCardShell.vue` kicker reverted to `font-medium` +
   `tracking-[0.12em]` (the pre-migration values). Fixes the season suffix AND every
   card's kicker at once.
2. **PlatformStats "only 2 tiles"** — backend, not frontend: the resolver picked ONE
   random window per metric and dropped the whole tile on error/zero, then cached the
   shrunken card for the UTC day. `platform_stats.go` now falls back to the metric's
   other windows before dropping. 4 tiles guaranteed whenever ≥4 of 5 metrics have any
   working window. **Deploy step: flush `spotlight:stats:<date>` Redis key.**

## User decisions still open (answer by letter, e.g. `A2 B1 C1 D1 E1 F2 G1 H1`)

| Q | Surface | Variants (first = recommended) |
|---|---|---|
| A | Carousel switching + skeleton | **A-1** icon-menu, active expands to icon+label pill (skeleton reserves the row with shimmer circles) · A-2 stories-style progress segments overlaid INSIDE the frame (zero external layout → zero shift) · A-3 always-visible `‹ 3/9 ›` glass cluster in the frame corner (dots removed) |
| B | RandomTail | **B-1** card-deck poster stack · B-2 full-bleed hero teaser (violet) · B-3 mystery "?" blur-reveal on hover |
| C | PersonalPick | **C-1** podium 1-2-3 (three posters, center elevated, reason under each) · C-2 refine current (title into body, scores + rank numbers on secondary) · C-3 big quote-style reason + mini poster row |
| D | TelegramNews | **D-1** Telegram chat bubbles (channel avatar, bubble tail, time) · D-2 hero post + compact tail · D-3 refine tiles (darker, taller photo, date overlay badge, ✈ vignette on photoless posts) |
| E | LatestNews | **E-1** terminal (`$ animeenigma --updates`, [FEAT]/[FIX]/[PERF] colored prefixes, blink cursor) · E-2 vertical timeline with typed glow dots · E-3 hero update (latest entry big, two small) |
| F | NowWatching | **F-1** poster trio with avatar overlap + LIVE overlay badge on posters · F-2 big "N человек смотрят прямо сейчас" counter + compact session list (works with 1 session) · F-3 refine rows (LIVE text badge, per-row episode progress bar) |
| G | NotTimeYet | **G-1** time capsule (big "ждёт N дней" counter, grayscale→color poster on hover, CTA «Время пришло») · G-2 time thread (added→today progress line) · G-3 pinned-note sticker (tilted) |
| H | ContinueWatchingNew | **H-1** episode stepper chips `эп.4 ✓ → эп.5 ✓ → эп.6 NEW` (ribbon removed from poster) · H-2 giant display episode number with pink glow · H-3 refine (corner badge instead of ribbon + season progress bar) |

## Locked (not revisited)

- A-1 brand triad, SpotlightCardShell anatomy, CTA bottom-left, overlay badges on
  posters, lucide score glyphs (v2, approved).
- FeaturedCard + PlatformStats designs (approved by user 2026-06-11; only the two
  fixes above touch them).
- Carousel mechanics (7s autoplay, stop-on-manual-nav, transition lock).

## Metrics (full package at recommended picks)

- **UXΔ = +3 (Better)** — per-card identity, non-anonymous menu, CLS shift removed.
- **CDI = 0.03 * 21** — spread stays inside `components/home/spotlight/`; mechanics untouched.
- **MVQ = Kraken 80%/85%** — many tentacles (8 surfaces), one body of primitives.
