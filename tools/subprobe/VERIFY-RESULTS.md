# Provider sub-delivery verification — empirical vs. DB `sub_delivery`

Subject: **Sousou no Frieren** ep.12 (uuid `f0b40660…`), real streams via our pipeline.
Detector: tier-1 `text_stroke` gate over bottom-center band → Tesseract OCR (script-ID).
Method legend: **frames** = pixel-probed; **tracks** = soft `.vtt` track-list signal; **n/a** = couldn't resolve live.

| Provider | DB says | MEASURED | method | match | notes |
|---|---|---|---|---|---|
| **gogoanime** | hard | **SOFTSUB** | tracks | ❌ **MISMATCH** | 9 soft tracks (Arabic, **English**, French, German, Italian, Portuguese, **Russian**, Spanish ×2). Clean video + softsubs. (video is sid-gated so not pixel-confirmed, but 9 lang tracks ⇒ clean.) |
| **animepahe** | hard | **HARDSUB (latin/EN)** | frames | ✅ | direct resolve times out (CF), but miruro fronts the *same* animepahe streams — pixel-confirmed EN burned-in. |
| **miruro** | hard | **HARDSUB (latin/EN)** | frames | ✅ | tracks=0; 8/15 frames text-confirmed. Aggregator → serves animepahe. |
| **nineanime** | hard | **HARDSUB (latin/EN)** | frames | ✅ | direct MP4 (my.1anime.site), tracks=0; 5/15 text-confirmed. |
| **kodik-noads** | hard | **HARDSUB (cyrillic/RU)** | frames | ✅ | catalog path (`/kodik/stream`), subtitle-type translation = burned-in RU. Voice translations = DUB (clean video). |
| **18anime** | hard | **HARDSUB (latin/EN)** | frames | ✅ | catalog adult path (`/anime18/stream`, mp4upload); 5/15 text-confirmed. KEEPS hard. |
| allanime | hard → **unknown** | inconclusive | n/a | ↓ | episodes resolve, **stream stage down** (CF/clock). |
| okru | hard → **unknown** | inconclusive | n/a | ↓ | ok.ru extractor for allanime; **stream stage down**. |
| animefever | hard → **unknown** | not verifiable | n/a | ↓ | policy=disabled, health=down. |
| animekai | hard → **unknown** | not verifiable | n/a | ↓ | policy=disabled, health=down. |

Non-`hard` (for completeness): `ae`/`raw` = soft (first-party/library); `animelib` = none (disabled, supports_sub=f); `hanime`/`kodik-iframe` = none.

## Bottom line
- **5/5 testable claimed-hard providers verified**: 4 genuinely HARDSUB (animepahe, miruro, nineanime, kodik-noads), **1 stale label — gogoanime is actually SOFTSUB**.
- The detector reliably distinguishes burned-in vs soft, and **RU (cyrillic) vs EN (latin)** hardsub on real frames at both 1080p and 360p.
- 5 providers couldn't be confirmed live (down/disabled/adult). Their labels stand unverified, not validated.

## Applied (2026-06-29, direct DB; SeedDefaults is insert-if-absent so it persists)
```sql
UPDATE stream_providers SET sub_delivery='soft'    WHERE name='gogoanime';
UPDATE stream_providers SET sub_delivery='unknown' WHERE name IN ('allanime','okru','animefever','animekai');
```
- `gogoanime` → **soft** (clean video + multi-language soft `.vtt` tracks, incl. EN and RU).
- 4 unverifiable claimed-hard → **unknown** (down/disabled — couldn't confirm burned-in).
- Verified HARDSUB kept: animepahe, miruro, nineanime, kodik-noads, 18anime.

`'unknown'` is safe: catalog rank `switch` has no default (neutral), FE `capLabels` maps
non-soft/hard → null (no delivery badge claimed).

### Residual (optional follow-up)
`services/catalog/internal/service/scraperprovider/seed.go` still hard-codes `gogoanime`
`SubDelivery:"hard"` (and `""→"hard"` default). Only matters for a FRESH DB (insert-if-absent
won't touch the corrected prod rows). Fix in code if you want fresh installs correct.
