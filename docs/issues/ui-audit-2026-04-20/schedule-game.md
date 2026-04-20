# Schedule + Game mobile audit — 2026-04-20 — viewport 500x723

## /schedule

- `document.title === "Расписание выхода серий - AnimeEnigma"` — UA-040 ✓
- `<h1>Расписание выхода серий</h1>` ✓
- **axe: 30 passes, 0 violations** — down from 1 on pre-Batch-B desktop (footer contrast)

## /game

- `document.title === "Игровые комнаты - AnimeEnigma"` — UA-040 ✓
- `<h1>Игровые комнаты</h1>` ✓
- **axe: 28 passes, 0 violations** — down from 1 on pre-Batch-B desktop (footer contrast)

## NEW findings

None on either view. Both are in excellent mobile shape.

## Notes

Footer copyright (UA-007) previously failing at 3.83:1 on every view now passes axe-core across all 7 probed views (Home, Browse, Anime, Profile, Themes, Schedule, Game). Batch B's `text-white/40` → `text-white/60` promotion landed cleanly and survives at mobile widths.
