# Browse — `/browse` — per-view findings

**View:** `https://animeenigma.ru/browse`
**Account:** `ui_audit_bot` logged in
**Viewports:** desktop 1280×800 ✓ probed
**axe-core:** 4.10.2 — Viol 2 (1 serious color-contrast, 1 moderate heading-order)

## Desktop findings (1280×800)

### Carry-over verification — three items STILL OPEN

| Prior ID | Status | Evidence |
|---|---|---|
| **UA-046** Genre placeholder contrast | ✗ **Open** | axe `color-contrast` on `.text-white/30` — fg `#626166` on bg `#1e1e24` = 2.7:1 (fails WCAG AA 4.5:1) |
| **UA-047** GenreFilterPopup aria-haspopup | ✗ **Open** | "Жанр" button has `aria-haspopup: null`, `aria-expanded: null` |
| **UA-048** Browse heading-order (sr-only h2) | ✗ **Open** | axe `heading-order` flags `.group.block .card-hover h3` — page jumps h1 → h3 with no h2 |

These three were called out in 2026-04-20 as Batch H targets. Confirming: Batch H did NOT ship.

### Other observations (no new severity-3 findings)

- `<h1>Каталог</h1>` present and not sr-only (visible) — good info-scent.
- `aria-current="page"` correctly on the active Caталог nav link.
- Cards use h3 with full anime titles (consistent with Onging/Top rows on Home).
- No new violations beyond the three carry-over items.

## Mobile findings (500×723) — pending in mobile sweep
