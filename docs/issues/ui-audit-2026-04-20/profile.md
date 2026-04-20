# Profile mobile audit — 2026-04-20 — viewport 500x723

Test user: `ui_audit_bot` (own profile, logged in)

## State verification — ALL green

- **UA-018 ✓** — 6 watchlist filter pills, all expose `aria-pressed` (1 true, 5 false). "Все (8)", "Смотрю (3)", "Запланировано (2)", "Просмотрено (2)", "Отложено (0)", "Брошено (1)"
- **UA-037 ✓** — count spans no longer flagged (previously `opacity-60`, Batch B bumped to `opacity-80`)
- **UA-038 ✓** — avatar upload button has `aria-label="Загрузить аватар"` (RU, i18n'd)
- **UA-041 ✓** — heading tree is h1 → h2 → h2 (card titles promoted to h2 in Batch F). No heading-order violation.
- **UA-040 ✓** — `document.title === "Профиль - AnimeEnigma"`
- `<h1>` text = username, locale RU

## axe-core (mobile 500x723)

- **41 passes, 0 violations.** Cleanest scan of the whole audit.
- Down from 3 violations pre-Batch-B.

## NEW findings

None on Profile. This view is in excellent mobile shape.

## Deferred

- Didn't drive the watchlist status-change scenario L2 (watching → completed) — deferred to next audit if needed
- Didn't probe sharing / report buttons on other users' profiles
