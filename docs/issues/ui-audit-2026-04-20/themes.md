# Themes mobile audit — 2026-04-20 — viewport 500x723

## State verification — ALL green

- **UA-034 ✓** — ThemeCard `text-white/30` count = 0 (was 82 pre-fix). `text-white/60` = 42, `text-white/90` = 2. Batch D fix landed, zero residual contrast flags.
- **UA-035 ✓** — both selects carry `aria-label` ("Сортировка", "Сезон")
- **UA-040 ✓** — `document.title === "Опенинги и Эндинги - AnimeEnigma"`
- **UA-041 ✓** — heading tree h1 → h2 → h2, no order violation

## axe-core (mobile 500x723)

- **34 passes, 0 violations.** Down from 3 on pre-fix desktop.

## NEW findings

None. Themes is in excellent mobile shape.
