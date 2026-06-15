# Season filter in catalog browse + season-data correctness

**Date:** 2026-06-15
**Status:** Approved (design)
**Scope:** Surface a "Season" filter in the existing catalog Browse, and fix the
derived-season data so the filter is accurate. No separate page.

## Problem / Goal

The owner wanted "a separate page showing next-season anime". Investigation
showed the catalog already stores everything needed and the backend already
filters by season — the only missing piece on the surface is a **season filter**.
A dedicated page would duplicate the catalog. Decision (owner-confirmed): do it
**through the existing catalog filters**, not a new page.

Resolving "next season" is then just: set `Season = Summer` + year range
`2026–2026` (+ optionally `Status = Announced`) in Browse. The season + year are
left for the user to pick; we do not auto-compute or add a preset entry point.

## Background (verified facts)

- `domain.Anime` already has `Season string` (`anime.go:18`), `Year`
  (`anime.go:17`), `AiredOn`, `NextEpisodeAt`, and `Status` (`announced`,
  `anime.go:119`).
- `Season` is derived at ingest from the aired month via
  `detectSeason(month)` in `services/catalog/internal/parser/shikimori/client.go:523`
  (called at `client.go:298` and `client.go:469`, both guarded by
  `AiredOn != nil`).
- Backend browse already applies the `season` filter
  (`repo/anime.go:120-122`) and parses it from the query
  (`handler/catalog.go:630`). **No backend route/handler/repo change is needed
  for filtering.**
- Frontend filter state lives in `composables/useBrowseFilters.ts`; the sidebar
  UI is `components/browse/BrowseSidebar.vue`. The `Status` filter is the exact
  template to copy (single-select `RadioGroup` + `FilterSection`).

### Live data snapshot (announced)

160 announced rows: 111 with a real year+season, 69 with a full aired date,
**49 with `year=0, season='fall'`** — an artifact of `detectSeason` returning
`fall` from its `default` branch when the month is missing/0. These pollute the
"Осень" (fall) season filter.

## Design

### Part A — Frontend: "Season" filter

**`frontend/web/src/composables/useBrowseFilters.ts`**
- Add `export type Season = '' | 'winter' | 'spring' | 'summer' | 'fall'` and
  `const SEASON_VALUES: Season[] = ['', 'winter', 'spring', 'summer', 'fall']`.
- Add `const season = ref<Season>('')`.
- `readUrl()`: read `?season=`, validate against `SEASON_VALUES` (mirror the
  `status` validation; unknown → `''`).
- `writeUrl()`: `next.season = season.value || undefined`.
- `apiParams`: `if (season.value) p.season = season.value`.
- `activeCount`: `if (season.value) n++`.
- `reset()`: `season.value = ''`.
- Return `season` from the composable.

**`frontend/web/src/components/browse/BrowseSidebar.vue`**
- Add a `<FilterSection :label="$t('browse.filters.section.season')"
  :count="filters.season.value ? 1 : 0">` containing a `RadioGroup` bound to
  `filters.season.value`, placed immediately **after** the Status section.
- Add `seasonOptions` computed: `{value:'',any} | winter | spring | summer | fall`.
- Add `onSeasonChange(v)` → set `filters.season.value` + `filters.writeUrl()`
  (mirror `onStatusChange`).

**i18n — both `locales/en.json` AND `locales/ru.json`** (parity test enforces both):
- `browse.filters.section.season` → "Season" / "Сезон"
- `browse.filters.season.any` → "Any" / "Любой"
- `browse.filters.season.winter` → "Winter" / "Зима"
- `browse.filters.season.spring` → "Spring" / "Весна"
- `browse.filters.season.summer` → "Summer" / "Лето"
- `browse.filters.season.fall` → "Fall" / "Осень"

### Part B — Backend: `detectSeason` correctness + data backfill

**`services/catalog/internal/parser/shikimori/client.go` — `detectSeason`:**
Replace the catch-all `default: return "fall"` so an absent/invalid month no
longer maps to fall:
```go
func detectSeason(month int) string {
    switch {
    case month >= 1 && month <= 3:
        return "winter"
    case month >= 4 && month <= 6:
        return "spring"
    case month >= 7 && month <= 9:
        return "summer"
    case month >= 10 && month <= 12:
        return "fall"
    default:
        return ""
    }
}
```

**One-off data backfill (psql against the live DB):**
```sql
UPDATE animes SET season = '' WHERE year = 0 AND season <> '';
```
Clears the 49 mislabeled TBA rows. A season without a year is meaningless for
this feature, so clearing is correct regardless of status. Going forward, the
fixed parser keeps new/re-ingested rows correct.

## Behavior notes (intentional)

- **Season without a year** returns that season across all years (e.g. every
  summer premiere in the catalog) — identical composition semantics to the
  existing `status`/`kind` filters.
- **On-demand catalog**: past seasons surface only already-ingested titles. This
  is by design (CLAUDE.md "Don't pre-populate the database"); we do NOT add a
  bulk Shikimori season fetch.

## Out of scope (explicit)

- No new page/route (`/seasons` etc.).
- No "Next season" preset link or navbar entry.
- No bulk season pre-population from Shikimori.
- No backend route/handler/repo changes for filtering (already supported).

## Testing / Verification

- `cd services/catalog && go test ./internal/parser/shikimori/...` — add
  `detectSeason` cases for month `0`, `13`, `10/11/12 → fall`.
- `cd frontend/web && bunx vitest run` — season round-trip in the
  `useBrowseFilters` spec (URL read/write, apiParams, activeCount, reset) and a
  `BrowseSidebar` assertion; `locales` parity spec must stay green.
- `bunx tsc --noEmit`, `bunx eslint src/`, `bash scripts/design-system-lint.sh`.
- Run the backfill SQL once; re-query announced season distribution to confirm
  the 49 `year=0` rows no longer carry a season.
- `/animeenigma-after-update`: redeploy `catalog` + `web`, changelog, commit, push.
