# Episode Selector Redesign — variant exploration (2026-06-11)

Status: **awaiting variant pick** · Demo artifact: `docs/superpowers/demos/2026-06-11-episode-selector/index.html` (served on :58363)

## Goal

The unified-player episode selector must surface user data (watched marks,
partial progress, episode titles) without tooltips, scale 1 → 1000+ episodes,
and open from where the viewer's attention lives.

## Decision history

- **2026-06-05** — trigger locked to "Grid icon top-right" (AskUserQuestion);
  panel = floating 5-col grid reusing source-panel geometry.
- **2026-06-11 (am)** — user data mapped in: watched check (anime-list +
  completed rows), partial-progress underline, title tooltips; player header
  shows current ep number + title.
- **2026-06-11** — "Mark ep. N as watched" added inside the panel (Kodik
  parity); "Resume from MM:SS" chip (no auto-seek — user decides).
- **Today's question** — revisit trigger + interior now that the selector
  carries much more data.

## Variants

| Criterion | V1 Grid+ | V2 Bottom Sheet | V3 Side Drawer | V4 Hybrid |
|---|---|---|---|---|
| Titles visible | tooltip | yes | yes | hover |
| 1000+ eps | ranges+jump | virtual+jump | virtual | ranges+jump |
| Mobile | cramped | great (swipe) | fullscreen sheet | = V1 |
| Video occlusion | small | bottom third | right 30% | medium |
| UX shift | minimal | medium | large | small |
| CDI | 0.02 × 3 | 0.04 × 5 | 0.05 × 8 | 0.03 × 5 |
| UXΔ | +1 (Better) | +3 (Better) | +2 (Better) | +2 (Ambiguous) |
| MVQ | Sprite 70%/85% | Griffin 85%/80% | Kraken 75%/70% | Basilisk 65%/75% |

- **V1 Dense Grid+** — evolution of current: range pager (>50 eps), jump
  input, "next unwatched" shortcut, mark-watched footer.
- **V2 Bottom Sheet (recommended)** — "EP N ▴" pill in the control bar next
  to the time pill opens a horizontal episode-card strip above the controls
  (number + title + progress + watched), scroll-snap, opens centered on the
  current episode. Grid icon top-right stays as a second entry to the same
  sheet.
- **V3 Side Drawer** — Netflix-style full-height right drawer with vertical
  rows (number, title, duration, progress bar, check).
- **V4 Grid + Hover-detail** — V1 grid; hovering a cell shows a mini detail
  card (title, progress, play). Degrades to V1 on touch.

## Invariants (any variant)

- Reuse the ① primitives (ep-cell states, mark-watched button, resume chip).
- Opens centered/scrolled to the current episode.
- Esc / click-outside closes; data refreshes after mark without reload.
- Anonymous users: plain numbers, mark button hidden.
