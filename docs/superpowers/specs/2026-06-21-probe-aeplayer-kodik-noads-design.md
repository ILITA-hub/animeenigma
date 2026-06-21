# Probe Coverage: aePlayer + no-ads Kodik — Design

**Date:** 2026-06-21
**Status:** Approved (design + decisions locked), pending plan → execution
**Owner:** @0neymik0

## 1. Problem

The unified playback probe (analytics, "Playability B") covers only the EN scraper
chain (`gogoanime, miruro, allanime, nineanime, animefever`). On the merged
roster/playability dashboard, `ae` (self-hosted library) and `kodik` show
"— not probed". The owner wants real playback verdicts for:

1. **aePlayer (`ae`)** — with a custom rule: probe the **3 latest episodes uploaded
   to the library** (NOT the spotlight anime-set the EN providers use).
2. **no-ads Kodik** — split the single `kodik` provider into the **iframe** Kodik
   (embed, not probeable) and the **no-ads/scraped** Kodik (direct ad-free HLS via
   `kodikextract`, which IS probeable), and probe the latter.

## 2. What already exists (ground-truthed on origin/main)

- **No-ads Kodik stream**: `libs/kodikextract/` + catalog `GET /api/anime/{uuid}/kodik/stream?episode=&translation=` → `domain.KodikStreamSource{StreamURL, Referer, ...}` (ad-free `.m3u8` on `solodcdn.com`). The CDN `solodcdn.com`/`cloud.solodcdn.com` is **already allowlisted** in `libs/videoutils/proxy.go` → the probe validator can fetch it through the HLS proxy with the `referer`, no signing needed. Translations come from `GET /api/anime/{uuid}/kodik/translations` → `[]KodikTranslation{ID, Title, Type, EpisodesCount, Pinned}`.
- **ae stream**: catalog `GET /api/anime/{uuid}/ae/episodes` → `{episodes:[{id,number}], available, source}`; `GET /api/anime/{uuid}/ae/stream?episode=N` → `{url, exp, sig, ...}` (MinIO `.m3u8`, provenance-signed; streaming proxy presigns the bucket). Validator fetches via hls-proxy unchanged.
- **Library**: separate DB, table `library_episodes(shikimori_id, episode_number, minio_path, created_at, ...)`. No "latest N uploaded" endpoint yet. `EpisodeRepository` exists.
- **Catalog**: `AnimeRepository.GetByShikimoriID(shikimoriID) → *Anime{ID, Name…}` maps shikimori_id → catalog UUID + title. Internal-endpoint convention already used (`/internal/anime/{shikimoriId}/episodes`, `/internal/scraper/providers`).
- **Roster** `stream_providers` already holds `ae` (group `firstparty`) and `kodik` (group `ru`). `provider_info` is auto-emitted per non-scraper row by `scraperprovider.EmitCatalogSideRoster`. The merged dashboard roster table is `SELECT … FROM stream_providers`, so any new row auto-appears. **No functional code looks up the roster row by name** — capability families (`capability/families_ru.go`), parsers, `has_kodik`, FE, watch-together all use a hardcoded data-source key `"kodik"` independent of the roster row.
- **Probe engine**: `Engine` holds ONE shared `AnimeSetResolver` + ONE shared `Resolver` + ONE shared `Validator`; `RunOnce` resolves the anime-set once and probes every provider with the same refs via the same resolver. `Resolver.Resolve(ctx, animeUUID, animeName, slot, provider)`. `AnimeRef{UUID, Name, Slot}`. Config: `PROBE_PROVIDERS` (csv), `PROBE_ANCHOR_UUID`, `CATALOG_URL`, `STREAMING_URL`, `FFPROBE_PATH`.

## 3. Decisions (locked)

| # | Decision |
|---|----------|
| D1 | **ae anime-set = 3 newest library uploads, deduped to distinct anime.** Newest `library_episodes` row per distinct `shikimori_id`, take 3, each carrying its own episode number. Probes whatever was most recently ingested, with anime diversity. |
| D2 | **no-ads Kodik reuses the spotlight 4-slot set** (same anchor+featured+2 random the EN providers use). Kodik has broad RU coverage; zero extra config. |
| D3 | **Split = rename + add.** Rename the `stream_providers` row `kodik` → `kodik-iframe` (the iframe embed, not probeable) and add a new `kodik-noads` row (scraped ad-free HLS, probeable). The rename is **scoped to roster/dashboard identity only**: the `stream_providers` row + the catalog `health_checker` liveness label. The functional data-source key `"kodik"` (parsers, capability families, FE, watch-together, `has_kodik`) is **untouched** → RU playback unaffected. |
| D4 | **Engine gains per-provider (AnimeSet, Resolver) pairs** via a `ProbeTarget` registry. EN providers share the spotlight set + scraper resolver; `ae` and `kodik-noads` get custom ones. The `Validator` stays shared (the HLS proxy handles signed-scraper, allowlisted-kodik-CDN, and signed+presigned-MinIO uniformly). |
| D5 | **`AnimeRef` gains `Episode int`** (0 = default/first). The ae anime-set sets it to the uploaded episode; the scraper/kodik resolvers ignore it (use ep 1 / first listed). |
| D6 | The ae target-set join (library-recent + shikimori→uuid) lives in **catalog** (`GET /internal/probe/ae-targets?limit=N`), since catalog already owns the library client + anime repo. Library exposes `GET /internal/library/recent-episodes?limit=N`. Both are Docker-network-only (gateway never proxies `/internal/*`). |

## 4. Architecture

```
analytics ProbeEngine.RunOnce()
  for each ProbeTarget{provider, animeSet, resolver}:
      refs := target.animeSet.Resolve(ctx)          // per-provider anime/episode selection
      for each ref:
          streams := target.resolver.Resolve(ctx, ref.UUID, ref.Name, ref.Episode, ref.Slot, provider)
          for each stream: validator.Validate(stream)   // SHARED hls-proxy validator
      Rollup(provider, verdicts)

Targets:
  EN chain (gogoanime,miruro,allanime,nineanime,animefever):
      animeSet = HTTPAnimeSet (spotlight)         [shared instance]
      resolver = HTTPResolver  (/api/anime/{uuid}/scraper/...)   [shared instance]
  ae:
      animeSet = AeAnimeSet → catalog /internal/probe/ae-targets?limit=3
                   → catalog /internal/library/recent-episodes (library DB)
                   → GetByShikimoriID → [{uuid, name, episode}]
      resolver = AeResolver → catalog /api/anime/{uuid}/ae/stream?episode=N → {url,exp,sig}
  kodik-noads:
      animeSet = HTTPAnimeSet (spotlight)         [reused shared instance]
      resolver = KodikNoadsResolver → /api/anime/{uuid}/kodik/translations (pick pinned|first)
                   → /api/anime/{uuid}/kodik/stream?episode=1&translation=ID → {stream_url, referer}
```

### Components (new)

- **`AnimeRef.Episode int`** — threads the specific episode for ae; scraper/kodik ignore (0).
- **`ProbeTarget{Provider string; AnimeSet AnimeSetResolver; Resolver Resolver}`** — Engine holds `[]ProbeTarget` (ordered). `RunOnce` loops targets; each resolves its own anime-set then probes with its own resolver. Shared `Validator`. Panic isolation + resolve-error verdicts preserved per target.
- **Resolver interface gains `episode int`**: `Resolve(ctx, animeUUID, animeName string, episode int, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error)`. `HTTPResolver` ignores `episode`; `AeResolver`/`KodikNoadsResolver` use it.
- **`AeResolver`** (analytics) — calls catalog `/api/anime/{uuid}/ae/stream?episode=N`; returns one `ResolvedStream{MasterURL=url, Exp=exp, Sig=sig, Stage=StageStream}`. Empty `url`/non-200 → stage-appropriate error (down).
- **`AeAnimeSet`** (analytics) — calls catalog `/internal/probe/ae-targets?limit=3`; returns refs `{UUID, Name, Episode, Slot=SlotLibraryLatest}`. Catalog-down or empty → returns empty (ae rolls up Down, never panics the run).
- **`KodikNoadsResolver`** (analytics) — `/kodik/translations` (prefer `Pinned`, else first; none → servers-stage error), then `/kodik/stream?episode=1&translation=ID`; returns `ResolvedStream{MasterURL=stream_url, Referer=referer, Stage=StageStream}`.
- **New slot** `SlotLibraryLatest AnimeSlot = "library_latest"` for ae rows.
- **Library** `GET /internal/library/recent-episodes?limit=N` → `{episodes:[{shikimori_id, episode_number}]}` (newest distinct shikimori_id, `ORDER BY created_at DESC`, deduped, limit N). Internal-only.
- **Catalog** `GET /internal/probe/ae-targets?limit=N` → `{targets:[{uuid, name, episode}]}` — calls library-recent, maps each shikimori_id via `GetByShikimoriID` (skips ones absent from catalog), carries the episode number. Internal-only.
- **Roster**: seed.go — rename intent + add `kodik-noads`; `intrinsicGroups`: `kodik-iframe`→`ru`, `kodik-noads`→`ru`. Guarded migration (`migrate.go`, mirrors `AnimefeverDeclaim`): (a) `UPDATE stream_providers SET name='kodik-iframe' WHERE name='kodik'` (guard: row named `kodik` exists), (b) insert-if-absent `kodik-noads`. health_checker `providerKodik` label → `"kodik-iframe"`.
- **Config**: `PROBE_PROVIDERS` default appends `ae,kodik-noads`. main.go builds the `[]ProbeTarget` registry, mapping provider name → target; unknown names in `PROBE_PROVIDERS` are skipped with a logged warning.

### Validator — unchanged, works for all three

`proxyURL` already: passes through `/api/streaming/` paths; else wraps `?url=&exp=&sig=&referer=`. ae sends exp/sig (provenance) → streaming presigns MinIO. kodik-noads sends referer, no exp/sig → CDN allowlisted. EN sends exp/sig. No Validator change.

## 5. Metrics, storage, dashboard

- `probe_provider_up{provider}`, `probe_provider_status`, `probe_runs_total`, and ClickHouse `probe_runs` rows gain `ae` and `kodik-noads` automatically (the reporter is provider-agnostic). `probe_runs.anime_name` already carries titles.
- **Merged dashboard panel** auto-updates: roster query shows `kodik-iframe` + `kodik-noads` rows; the ClickHouse playability/reasons join populates `ae` + `kodik-noads`. `kodik-iframe` has no probe rows → "— not probed" (correct). No JSON edit strictly required; we will verify live and only adjust if a column/mapping needs it.

## 6. Reason codes

Reuse `libs/streamprobe` reasons. ae: `empty_response`/`cdn_unreachable`/`decode_failed`/`invalid_video`/`playable`. kodik-noads: same set plus `empty_response` when no translations/stream resolve.

## 7. Testing

Table-driven, fakes only (no live APIs):
- Library recent-episodes: repo query returns newest distinct shikimori_id, limit N; handler shape.
- Catalog ae-targets: library-recent + GetByShikimoriID happy path; skips unmapped shikimori_id; carries episode.
- Engine `[]ProbeTarget`: per-target anime-set + resolver dispatch; panic isolation per target; resolve-error → Down; mixed targets in one run.
- AeResolver / AeAnimeSet: happy path, catalog-down (empty → Down), non-200.
- KodikNoadsResolver: pinned-preference, first-fallback, no-translations → error, stream happy path (referer carried).
- Roster migration: rename `kodik`→`kodik-iframe`, insert `kodik-noads`, idempotent re-run; group intrinsic.

## 8. Risks

- A season-pack ingest can make the 3 newest uploads share an anime → D1 dedupes to distinct anime, so coverage stays diverse (or fewer than 3 if the library holds <3 anime — acceptable, probe what exists).
- Spotlight anime may occasionally lack a Kodik translation → that anime's kodik-noads verdict is `empty_response`; rollup handles it (degraded/down by dominant reason). Not a false negative for the provider unless ALL slots lack kodik.
- Renaming the roster row desyncs nothing functional (verified); the Parser-timing panel keeps a single `kodik` series (parser timing is per data-source, legitimately one) — acceptable, documented.

## 9. Out of scope

- Building any new scraper/extraction (kodikextract + ae paths already exist).
- Changing the functional `"kodik"` data-source key (FE/capabilities/WT).
- Per-anime Kodik target tuning (D2 reuses spotlight).
