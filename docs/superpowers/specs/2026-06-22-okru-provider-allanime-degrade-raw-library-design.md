# okru provider + allanime degrade + raw library-only — Design

**Date:** 2026-06-22
**Status:** Approved (owner, 2026-06-22)

## Goal

Restore an AllAnime-sourced EN streaming path that does **not** depend on AllAnime's Cloudflare-Turnstile-walled clock endpoint, by adding a distinct **`okru`** provider that resolves AllAnime's `Ok` (ok.ru) sources. Concurrently **degrade** the now-broken `allanime` provider and make the JP-original-audio **`raw`** provider **library-only** (dropping its redundant AllAnime backend).

## Background / R&D (verified 2026-06-22)

- AllAnime **discovery** (`api.allanime.day/api` GraphQL: search/episodes/sourceUrls) works live from our datacenter Go client. No fix needed there.
- AllAnime **stream** is broken: the primary `Luf-Mp4`/`Default` sources decode to `/apivtwo/clock.json`, which is behind a **Cloudflare managed/Turnstile challenge** on `api.allanime.day` and a tarpit/down state on the bare `allanime.day` host. Unsolvable from our egress (plain Go, curl, Camoufox, WARP all fail). No clean mirror exists (`.co/.ai/.site` are parked squats; Consumet dropped AllAnime; ani-cli's clock host is down). This is **not** an IP-reputation issue — it requires a browser-issued `cf_clearance` we can't reliably obtain.
- **ok.ru ("Ok") sources avoid the clock entirely and work from our egress (proven end-to-end):** `ok.ru/videoembed/<id>` returns static HTML with a `data-options` attribute → JSON → `flashvars.metadata.hlsManifestUrl` = `okcdn.ru/video.m3u8` → fetched HTTP 200, valid `#EXTM3U` master with quality variants, IP-locked to our egress.

## Taxonomy (locked)

**RAW = Japanese audio with NO burned-in subtitles.** Subs, if any, are soft-overlaid at runtime (SubtitleOverlay / Jimaku) — never in the pixels. `sub_delivery=soft` is the marker.

- `raw` provider → `group=jp`, `sub_delivery=soft`, JP audio, library-backed.
- `okru` provider → `group=en`, `sub_delivery=hard` (ok.ru is a generic host with no soft-sub track, so AllAnime hardsubs its uploads). EN sub/dub.
- **They never cross:** ok.ru / AllAnime `Ok` content (EN hardsub) is never routed into `raw`. (Decision: okru does **not** pull AllAnime's `raw` translation category, to keep `raw` strictly library-only.)

## Component 1 — `okru` scraper provider

**Discovery is reused, not duplicated.** `okru.Provider` holds an internal `allanime.Provider` (constructed with okru's own deps) and delegates `FindID` / `ListEpisodes` to it (same working GraphQL). Episode IDs stay AllAnime's `<showID>:<episodeString>` format.

New files:
- `services/scraper/internal/providers/okru/doc.go`
- `services/scraper/internal/providers/okru/client.go` — `okru.Provider` + `New(Deps{HTTP, Cache, Log})` implementing `domain.Provider`:
  - `Name() → "okru"`
  - `FindID`, `ListEpisodes` → delegate to internal `allanime.Provider`.
  - `ListServers` → read the episode's source list, keep only `strings.EqualFold(name,"ok")`, return as servers (`ID/Name` = `Ok`, `Type` = category).
  - `GetStream` → read the source list, filter to `Ok`, resolve each via the okru extractor until one yields a playable stream; foreign/unknown episode-id → `ErrNotFound`; no Ok source → `ErrNotFound` (so the orchestrator moves on cleanly).
  - `HealthCheck` → per-stage snapshot like allanime.
- `services/scraper/internal/providers/okru/cache.go` — `scraper:okru:*` keys for servers/stream (TTL scheme mirrors allanime; discovery may reuse allanime's shared cache).
- `services/scraper/internal/embeds/okru.go` — `OkruExtractor` implementing `domain.EmbedExtractor`:
  - `Name() → "okru"`; `Matches(url)` → host is `ok.ru`/`*.ok.ru`/`okcdn.ru`.
  - `Extract(ctx, embedURL, headers)` → **static parse** (NOT JS): GET `ok.ru/videoembed/<id>`, read `data-options="..."` attribute, HTML-unescape, JSON-parse, read `flashvars.metadata` (string→re-parse or object), take `hlsManifestUrl`/`ondemandHls` (preferred, `Type=hls`) with `videos[]` MP4 fallbacks. Return `domain.Stream{Sources, Headers:{Referer:"https://ok.ru/"}}`. Fallback method: `POST https://ok.ru/dk?cmd=videoPlayerMetadata&mid=<id>` (yt-dlp Odnoklassniki algorithm) if `data-options` is absent.

Minimal allanime change to enable reuse: export a per-episode source accessor, e.g. `func (p *allanime.Provider) EpisodeSourceURLs(ctx, episodeID, category) ([]SourceURL, error)` wrapping the existing `fetchSources`, plus a public `SourceURL{Name, URL}` view. This is the only allanime code touched besides the degrade.

Registration:
- `services/scraper/cmd/scraper-api/main.go` — build `okruBaseHTTP` (`WithPerHostRPS("ok.ru",1,2)`, `WithPerHostRPS("okcdn.ru",2,4)`, `WithProvider("okru")`, `WithTransport(egressTransport)`); register the okru extractor; construct `okru.New(...)`; add `"okru"` to the orchestrator provider slice **after** allanime.
- `services/scraper/internal/config/providers.go` — add `"okru"` to `KnownProviders`.

**Playback / signing:** okcdn.ru is IP-locked to our egress; catalog's `GetScraperStream` signs the resolved URL via `streamsign.SignScraperStreamBody` (HMAC `exp`/`sig`), so the streaming HLS proxy trusts it with **no allowlist entry**. The `Referer: https://ok.ru/` header must be carried through to segment fetches.

## Component 2 — allanime → degraded

- `services/catalog/internal/service/scraperprovider/seed.go` — set the `allanime` seed row `Status: domain.StatusDegraded` with a `Reason`/`Description` explaining the CF-Turnstile clock wall and that `okru` carries the ok.ru sources.
- `services/catalog/internal/service/scraperprovider/migrate.go` — add a guarded one-time `AllAnimeDegrade(db)` migration (mirror `AnimefeverDeclaim`): guard key `allanime_degrade`; `UPDATE stream_providers SET status='degraded', reason=…, description=… WHERE name='allanime'`; write guard only on `RowsAffected > 0`; never clobber an operator re-enable.
- `services/catalog/cmd/catalog-api/main.go` — call `AllAnimeDegrade` after `AnimefeverDeclaim`.
- The orchestrator already excludes `degraded` from auto-failover while keeping it manually selectable (hacker mode) — exactly like animefever. allanime's Go code stays registered (okru reuses its discovery internally, independent of roster status).
- Frontend `providerRegistry.ts` — update the `allanime` `blurb` to note degraded status.

## Component 3 — raw → library-only

- `services/catalog/internal/service/raw_resolver.go`:
  - Drop the `allanime.Client` field + the `parser/allanime` import.
  - `NewRawResolver(libraryClient, animeRepo, redisCache, log)` (no allanime arg).
  - `GetEpisodes` → `library.ListEpisodes(anime.ShikimoriID)` directly; empty/no-shikimori → `{Episodes:[], Available:false, Source:"library"}`.
  - `GetStream` → `library.GetEpisode(...)`; 200 → signed MinIO stream; 404 → NotFound; 5xx/error → wrapped unavailable. Delete `resolveShowID`/`doSearch`/AllAnime fallback/source-decision cache.
  - Keep `GetLibraryEpisodes`/`GetLibraryStream`/`newLibraryStream` (also used by `ae`).
- `services/catalog/cmd/catalog-api/main.go` — drop the `allanimeClient` construction; update the `NewRawResolver` call.
- Delete the now-unused `services/catalog/internal/parser/allanime/` package **after confirming no other importer** (grep gate); remove dead `AllAnimeConfig` env wiring.
- Roster `raw` row + frontend `raw` chip stay unchanged (`group=jp`, `sub_delivery=soft`).
- **Accepted tradeoff:** raw then only has episodes present in the library (self-hosted). Until the v4.1 autocache lands, raw is empty for most titles — but AllAnime-raw is already clock-broken, so this is not a practical regression, only an explicit one. (Library 5xx now surfaces as unavailable with no fallback.)

## Roster row (okru)

`name=okru, group=en, status=enabled, engine=http, scraper_operated=true, supports_sub=true, supports_dub=true, supports_raw=false, sub_delivery=hard, quality_ceiling=1080p, preference_weight=35`. Must be added to **both** `scraperprovider/seed.go` (incl. `scraperOperatedNames`) **and** scraper `KnownProviders`, or it is skipped/rejected.

Frontend `providerRegistry.ts`: `{ id:'okru', name:'OK.ru', hue:'#ee8208', group:'en', audios:['sub','dub'], langs:['en'], content:['common'], scraper:true, blurb:'EN — AllAnime index, OK.ru CDN' }` + add to the EN curated tier.

## Testing

- okru extractor: unit test against a captured `data-options` fixture → asserts HLS manifest + MP4 fallback extraction; malformed/missing → error.
- okru provider: fake discovery + fake extractor → asserts `Ok`-only filtering, foreign episode-id → NotFound, no-Ok-source → NotFound.
- raw resolver: rewrite `raw_resolver_test.go` to library-only (no AllAnime calls; library 404 → NotFound; library down → unavailable; empty when unconfigured).
- roster: `AllAnimeDegrade` idempotent-guard test; seed includes okru; `scraper_operated` true for okru.
- Live e2e (post-deploy): `prefer=okru` on a title with an `Ok` source → episodes → servers → stream → HLS proxy 200 valid playlist/segment.

## Rollout

Deploy from a clean origin/main worktree: redeploy **catalog** (seed + AllAnimeDegrade migration + raw change), **scraper** (okru provider), **web** (registry). Migration runs on catalog startup (idempotent guard). Verify roster (`allanime=degraded`, `okru=enabled`, `scraper_operated=true`), then live e2e. Changelog (Trump-mode): "OK.ru restored an AllAnime EN source / allanime degraded".

## Resolved knobs

- okru `preference_weight = 35` (late failover — partial coverage).
- Delete the dead catalog `parser/allanime` package (if no other importer).
- okru `sub_delivery = hard`.
