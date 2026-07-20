# Catalog filter: English dub

**Date:** 2026-07-20
**Status:** approved, awaiting implementation plan

## Problem

There is no way to browse for titles that have an English dub.

Worse, the catalog already ships a chip that claims to be exactly that and is not.
`BrowseSidebar.vue` offers a `dub` value labelled `browse.filters.provider.dub` —
"English (Dub)" / "Английский (озвучка)" / 「英語（吹替）」 in all three locales. That
value maps to `animes.has_dub` (`repo/anime.go` `colsByKey`), and `has_dub` is written
in exactly one place: `service/catalog_kodik.go`, from
`kodik.TranslationsHaveDub` — Kodik translations with `Type == "voice"`. Kodik is the
RU family. The chip filters **Russian** voiceover under an **English** label.

## Data reality (production, 4973 anime)

| signal | rows | meaning |
|---|---|---|
| `has_dub` | 162 | Kodik RU voiceover, lazily backfilled on title open |
| `has_english` | 149 | EN source resolvable via the scraper chain — **sub OR dub**, undifferentiated |
| `has_video` | 0 | first-party library; no rows yet |
| `content_verifications` with verified `audio.lang='en'` | 5 (of 37 probed) | the only audio-verified EN-dub truth |

content-verify is the accurate source and is far too sparse to back a catalog filter.
The scraper chain, however, already returns a **per-episode `has_dub`** flag —
`service/anime_level_episodes.go` `latestEnglish` parses it to compute "latest EN dub
episode" — and that verdict is simply never persisted at title level.

## Decisions

1. **Honest EN-dub flag, and fix the mislabel.** New title-level column fed from the
   scraper's per-episode `has_dub`. The existing `dub` chip is relabelled to what it
   actually filters (Kodik RU voiceover).
2. **Lazy hook plus a background goroutine** in catalog (not a scheduler job), so the
   filter is not empty at launch and does not go stale.
3. **`bool` + `checked_at`**, not a bare bool — the loop must distinguish "probed, no
   dub" from "never probed", or it re-probes negatives forever and hammers providers.
4. **Chip set reorganised around dub language**, keeping existing URL keys so bookmarks
   survive.

## Design

### Schema — `services/catalog/internal/domain/anime.go`

```go
// HasEnglishDub — the EN scraper chain reported at least one episode carrying a
// dub track. Distinct from HasDub, which is Kodik RU voiceover.
HasEnglishDub bool `gorm:"default:false;index;column:has_english_dub" json:"has_english_dub"`
// EnglishDubCheckedAt — when the verdict was last established. NULL = never
// probed. Drives the background re-check cadence.
EnglishDubCheckedAt *time.Time `gorm:"index;column:english_dub_checked_at" json:"-"`
```

`AutoMigrate` adds both columns on restart.

`animeMetadataColumns` (`repo/anime.go`) deliberately excludes provider-availability
flags, so the Shikimori refresh path will not clobber the new column and **the
allowlist needs no change** — only its doc comment, to name the new flag.

Four repo tests hand-write `CREATE TABLE animes` DDL and must gain both columns or they
break: `browse_filter_test.go`, `anime_studios_test.go`, `anime_update_test.go`,
`raw_resolver_test.go`.

### Write path 1 — lazy hook

`scraperOps.GetScraperEpisodes` (`service/scraper.go`) currently detects a non-empty
episode list with `strings.Contains(body, "episodes":[{")`. Replace with a real JSON
decode of the envelope already modelled in `anime_level_episodes.go`, and set both
flags in one pass: `has_english` when the list is non-empty, `has_english_dub` when any
episode has `has_dub`.

**Honesty rule:** write `false` only when the call was not pinned to one provider
(`prefer == ""`). A pinned call may only promote to `true`. Otherwise a sub-only
gogoanime response would overwrite a true verdict from miruro, which is DUB-only.

### Write path 2 — background goroutine

New `services/catalog/internal/service/english_dub_backfill.go`, mirroring
`PlayerHealthChecker`: a `Start(ctx)` ticker loop launched from
`cmd/catalog-api/main.go` alongside the health checker.

- Default tick 60s (`CATALOG_ENDUB_BACKFILL_INTERVAL`), **one title per tick**
  (~1440/day). Provider load stays negligible.
- Candidates drawn only from `has_english = true`. No EN source implies no EN dub, and
  the restriction avoids ~4800 pointless provider calls.
- Priority: `english_dub_checked_at IS NULL` → `status='ongoing'` older than 7 days →
  anything older than 30 days. Ongoing titles are re-checked because dubs ship after
  subs.
- Degradation-aware: `cache.NewDegradationWatcher` (already in `libs/cache`; catalog
  has Redis) — skip the tick at Elevated+.
- A network-free SQL pass over `content_verifications` (jsonb: `audio.lang='en'` and
  verified) promotes those titles wholesale. It runs once at startup and hourly
  thereafter — not every tick, since content-verify moves far slower than 60s. Five
  rows today, but it is the only audio-verified truth and it outranks provider
  metadata, so it may also promote a title the scraper pass concluded `false` on.
- Metrics: `catalog_english_dub_backfill_total{result=hit|miss|error}` plus a gauge of
  titles still unchecked.

### Filter plumbing

New provider key `endub` in three places:

- handler whitelist — `handler/catalog.go`, the `case "kodik", "dub", "ae":` switch
- repo column map — `repo/anime.go`, `colsByKey["endub"] = "has_english_dub"`
- frontend `Provider` union and `PROVIDER_VALUES` — `composables/useBrowseFilters.ts`

`SearchFilters.CacheKey` already sorts and folds in `Providers`; no cache change.

### Frontend and i18n

Section retitled from "Available on" to "Dub and sources". Chip order and labels:

| key | column | ru | en | ja |
|---|---|---|---|---|
| `dub` | `has_dub` | Русская озвучка (Kodik) | Russian dub (Kodik) | ロシア語吹替（Kodik） |
| `endub` | `has_english_dub` | Английская озвучка | English dub | 英語吹替 |
| `kodik` | `has_kodik` | Kodik (RU) | Kodik (RU) | Kodik (RU) |
| `ae` | `has_video` | AnimeEnigma | AnimeEnigma | AnimeEnigma |

Existing URL keys are unchanged, so shared links and bookmarks keep working. The new
chip takes a `text-teal-400` accent — teal is on the DS brand-hue exemption list. Run
`/frontend-verify` before finishing.

### Tests

- repo: DDL columns in the four tests above; an `endub` case in `browse_filter_test.go`
- service: table test for the hook — dub present, dub absent, and a pinned `prefer`
  call proving `false` is not written
- service: candidate selection never returns a `has_english = false` row
- frontend: `endub` URL round-trip in the `useBrowseFilters` spec

## Out of scope

- Hiding the `ae` chip while `has_video` matches zero rows — considered and declined.
- A scheduler job; the goroutine lives in catalog, which owns the column.
- Widening content-verify coverage.

## Metrics

UXΔ = +2 (Better) · CDI = 0.05 * 13 · MVQ = Griffin 80%/85%
