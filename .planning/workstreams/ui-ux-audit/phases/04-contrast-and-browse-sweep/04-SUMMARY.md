# Phase 4 Summary: Color-contrast + Browse heading sweep

**Completed:** 2026-05-13
**Plan:** 04-PLAN.md
**Outcome:** All audit-cited contrast surfaces moved to `text-white/60`. Browse a11y triple-fix shipped (placeholder contrast + GenreFilterPopup ARIA + sr-only h2).

## Changes shipped

### UX-10 — `text-white/40` → `/60` sweep

Per-file `replace_all` in seven views: `Anime.vue`, `Themes.vue`, `Schedule.vue`, `Game.vue`, `Navbar.vue`, `Auth.vue`, `Profile.vue`. After-state grep returns zero `text-white/40` occurrences in these files.

Closed audit findings:
- UA-052 / UA-121 — Anime detail residual (multiple lines: 88, 99, 601, 698, 731 + others)
- UA-066 — Profile import-help description + all Profile subtitle text
- UA-072 — Auth Telegram-Web summary
- UA-074 — Themes empty-state "noThemes"
- UA-076 — Schedule hint text
- UA-077 — Game leaderboard rank (ranks ≥ 2)
- UA-086 — Navbar search subtitle (autocomplete dropdown)

Player components (KodikPlayer, AnimeLib, HiAnime, Consumet, English, Hanime, AnimeContextMenu, ReportButton, EnglishPlayer) intentionally NOT swept — fullscreen player UI may rely on dim contrast and the audit didn't cite contrast violations there.

### UX-11 — Browse a11y

- `GenreFilterPopup.vue` — trigger button gains `aria-haspopup="listbox"` + `:aria-expanded="isOpen"`. Placeholder span class changed `text-white/30` → `text-white/60`. Closes UA-046 + UA-047.
- `Browse.vue` — inserted `<h2 class="sr-only">{{ $t('browse.resultsHeading') }}</h2>` immediately before the results grid. Closes UA-048 (heading order h1 → h3 → fixes to h1 → h2 → h3).
- `locales/{en,ru,ja}.json` — added `browse.resultsHeading`: "Results" / "Результаты" / "結果".

## Verification

See `04-VERIFICATION.md` for the success-criteria scorecard.

## Files touched

```
frontend/web/src/views/Anime.vue                          # text-white/40 → /60 (8 occurrences)
frontend/web/src/views/Themes.vue                         # text-white/40 → /60 (1 occurrence)
frontend/web/src/views/Schedule.vue                       # text-white/40 → /60 (1 occurrence)
frontend/web/src/views/Game.vue                           # text-white/40 → /60 (2 occurrences)
frontend/web/src/views/Auth.vue                           # text-white/40 → /60 (4 occurrences)
frontend/web/src/views/Profile.vue                        # text-white/40 → /60 (16 occurrences)
frontend/web/src/views/Browse.vue                         # sr-only h2 inserted
frontend/web/src/components/layout/Navbar.vue             # text-white/40 → /60 (5 occurrences)
frontend/web/src/components/ui/GenreFilterPopup.vue       # placeholder /30 → /60 + aria-haspopup + aria-expanded
frontend/web/src/locales/en.json                          # +1 (browse.resultsHeading)
frontend/web/src/locales/ru.json                          # +1
frontend/web/src/locales/ja.json                          # +1
.planning/workstreams/ui-ux-audit/phases/04-contrast-and-browse-sweep/
  04-CONTEXT.md      (new)
  04-PLAN.md         (new)
  04-SUMMARY.md      (this file)
  04-VERIFICATION.md (new)
```

## Notes for downstream phases

- The `aria-haspopup="listbox"` + `:aria-expanded` pattern on the GenreFilterPopup trigger is the reference for Phase 15 (multi-axis catalog filter sidebar) when other genre/format/status filters are added.
- The `<h2 class="sr-only">` heading-order pattern is reusable on any grid view that has h1 + h3 cards without an intervening h2 (Schedule.vue + Themes.vue + Game.vue should be audited for the same pattern in a future phase if axe flags them).
