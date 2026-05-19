# Plan 26-02 — has_english Column + Browse Filter

**Status:** COMPLETED
**Requirement:** SCRAPER-HEAL-25 (browse filter activation)
**Wave:** 1 (autonomous; depends on 26-01)
**Date:** 2026-05-19

## What shipped

The browse filter activation that CONTEXT.md D5 explicitly deferred from
Phase 24 to Phase 26. Backend: a `has_english` GORM column on Anime, repo
SetHasEnglish setter, filter switch case for `english`, handler whitelist
entry, opportunistic setter call from the scraper-episodes resolver.
Frontend: Provider union widened, sidebar row with emerald accent, i18n
keys in en/ru/ja.

## Files modified

### Backend
- `services/catalog/internal/domain/anime.go` — Added `HasEnglish bool`
  field with GORM tag `gorm:"default:false;index;column:has_english"`.
- `services/catalog/internal/repo/anime.go` — Added `"english": "has_english"`
  to colsByKey map + `SetHasEnglish` method.
- `services/catalog/internal/handler/catalog.go` — Added `"english"` to
  the providers whitelist switch.
- `services/catalog/internal/service/scraper.go` — Widened animeFetcher
  interface with SetHasEnglish + added opportunistic call in
  GetScraperEpisodes gated on `status==200 && contains "episodes":[{"`.
- `services/catalog/internal/service/scraper_test.go` — Updated
  fakeAnimeFetcher to implement SetHasEnglish.

### Frontend
- `frontend/web/src/composables/useBrowseFilters.ts` — Widened Provider
  type and PROVIDER_VALUES to include `'english'`.
- `frontend/web/src/components/browse/BrowseSidebar.vue` — Added english
  row to providerOptions with `text-emerald-500 focus:ring-emerald-500`.
- `frontend/web/src/locales/en.json` — `"english": "English"`
- `frontend/web/src/locales/ru.json` — `"english": "Английский"`
- `frontend/web/src/locales/ja.json` — `"english": "英語"`

## Verification

See `26-02-VERIFICATION.md`. Highlights:

- `make redeploy-catalog` → exit 0; container healthy.
- GORM auto-migrated `has_english boolean DEFAULT false` + index
  `idx_animes_has_english`.
- `bunx tsc --noEmit` exits 0; `bun run build` exits 0.
- `make redeploy-web` exits 0.
- One curl to scraper-episodes for Frieren → has_english flipped to `t`.
- `/api/anime?providers=english` returns the now-tagged anime.

## Decisions taken in flight

1. Used `strings.Contains(body, "\"episodes\":[{")` as the cheap success
   marker rather than parsing the JSON. The catalog forwards verbatim
   from the scraper; a full unmarshal would be wasted work. The marker
   distinguishes "non-empty array" from "empty array `[]`" cleanly.
2. Widened the animeFetcher interface rather than adding a separate
   `englishBackfiller` interface, because all production code already
   has both methods on `*repo.AnimeRepository` (via the SetHasEnglish
   method added in this plan). The single-interface approach matches
   how SetHasKodik / SetHasAnimeLib / SetHasRaw are wired elsewhere.
3. Emerald (text-emerald-500) accent for the english filter row —
   distinct from kodik (cyan) and animelib (orange), reads as "English"
   without explicit text mnemonic.

## Not done

Nothing — plan completed per acceptance criteria.
