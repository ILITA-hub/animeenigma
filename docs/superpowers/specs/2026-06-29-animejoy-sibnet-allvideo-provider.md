# AnimeJoy (Sibnet + AllVideo) RU-Sub Video Provider — Design

**Date:** 2026-06-29
**Status:** Approved design; Phase 1 built. **Revised 2026-06-29: two providers, not one** (see Locked decisions).
**Origin:** AUTO-084 "Research: animejoy.ru player architecture" (resolved 2026-06-29, `docs/issues/auto-084-animejoy-player-architecture.md`). Live-probed all animejoy players through our `streamprobe` logic across 6 titles; Sibnet + AllVideo are the reliable legs. User directive: integrate them as **one catalog-side RU provider**, **RU-sub only**, simpler single-provider shape.

---

## 1. Problem

We have exactly one RU video provider (Kodik) plus OK.ru. AUTO-084 found animejoy.ru is a healthy, **non-IP-blocked**, **non-ad-substituted** source from our datacenter egress, with two legs that probe reliably across titles:

| Leg | Present (6 titles) | UP | Format |
|-----|:-:|:-:|--------|
| **Sibnet** (`video.sibnet.ru`) | 6/6 | 6 | direct MP4 + `e=` token + Referer |
| **AllVideo** (`fsst.online`→`filevideo1.com`) | 5/6 | 5 | direct MP4 + Referer |

The other animejoy legs are excluded: `cdnjoyka` (Наш плеер) is marquee-only (One Piece only); Mail/Dzen/CDA/Matreshka rot per-episode; Kodik we already integrate.

## 2. Goal

Add **`animejoy`** as a single catalog-side RU provider that resolves a title → animejoy `news_id` → episode → **Sibnet (primary), AllVideo (fallback)** → tokened MP4, surfaced as a selectable **RU-sub** source in aePlayer, gated by a `stream_providers` row and the existing policy/health machinery.

### Locked decisions

- **RU SUB only.** animejoy serves original (JP) audio + Russian subtitles, **hardsubbed** into the Sibnet/AllVideo mirror MP4s. → audio kind `sub` only; **no dub, no raw**. `SubDelivery="hard"`. (Verify burned-in subs once in Phase 2; the probe only checked playability.)
- **Two providers + a shared discovery base (REVISED 2026-06-29).** `animejoy` is **NOT a user-facing provider** — it is the shared **discovery/reference base** (`parser/animejoy/`: title→news_id→playlist, cached once). **`animejoy-sibnet`** and **`animejoy-allvideo`** are the two real provider rows (DisplayName "Sibnet" / "AllVideo") that reuse that discovery and each resolve their own leg. Mirrors how `okru` reuses AllAnime's GraphQL discovery. Independent policy/health per provider (the sweep showed Sibnet 6/6 vs AllVideo 5/6 — worth separating). Wire-ids namespaced `animejoy-*` to avoid the generic `sibnet` host-name colliding.
- **Catalog-side (Kodik pattern), NOT scraper.** RU providers live in the catalog service; the scraper's `Provider` interface, `candidateProviders`, and `EmbedExtractor` registry are EN-only and would crash-loop boot if touched.
- **No Camoufox.** animejoy.ru serves search, detail pages, and the playlist AJAX to plain `curl` (HTTP 200, no CF challenge) from our egress — discovery is plain HTTP inside catalog.

> **Tension acknowledged:** the 2026-06-24 anime365 spec rejected "AnimeJoy-direct (RKN-blocked, bespoke DLE scraping, fragile)" — but that was for *subtitles*. This is for *video streams*, where AUTO-084's probe evidence (no IP block, Sibnet 6/6) changes the calculus. The fragility caveat still stands (§6).

## 3. Architecture

One new catalog parser package + the standard Kodik-family surfacing edits. Mirrors Kodik end-to-end.

```
capability feed (catalog) ── buildFamilies ── animejoyFamily ──► CatalogService.GetAnimejoyTeams
                                                                    └─ parser/animejoy.Client
FE plays /api/anime/{id}/animejoy/* ── GetAnimejoyStream ───────────┐  ├─ ResolveNewsID (search + JaroWinkler + Cyrillic norm)   [cache 24h ±]
                                                                    │  ├─ FetchPlaylist (team→player→episode tree)             [cache 3-6h]
                                                                    │  └─ ResolveLeg: Sibnet → AllVideo                        [cache ≤5min]
                                                                    └─ streamsign.SignScraperStreamBody (exp/sig)
                                                                       → FE: /api/streaming/hls-proxy?url=&referer=&exp=&sig=
```

### 3.1 New package: `services/catalog/internal/parser/animejoy/`

- `client.go` — `Client{ baseURL, http, cache }`; ports JaroWinkler scoring (`internal/fuzzy/jarowinkler.go`) + `computeStreamTTL` (`scraper/.../animepahe/cache.go:43`, adapted to Sibnet's `e=<unix>` token) into catalog.
- `search.go` — **`ResolveNewsID(ctx, anime)`**: `GET /index.php?do=search&subaction=search&story=<urlenc>` over `anime.Title`+synonyms; parse result links `/<section>/<news_id>-<slug>.html`; score `fuzzy.JaroWinkler(normalize(q), normalize(cand)) ≥ 0.85`; tiebreak on **year + season** (catalog has both). **Cyrillic pre-normalizer** folds "N сезон / ТВ-N / Восставший… 2 сезон / часть N" before `fuzzy.NormalizeTitle` (English-only). Cache `animejoy:newsid:<anime_id>` 24h positive **and** negative.
- `playlist.go` — **`FetchPlaylist(ctx, newsID)`**: `GET /engine/ajax/playlists.php?news_id=<id>&xfield=playlist` → `{"response":"<html>"}`, JSON-unescape, parse the `<li data-id="team_player_episode" data-file>` tree → `[]Team{Name, Episodes[]{Num, legs{sibnet,allvideo URL}}}`. Filter players to Sibnet + AllVideo only. Cache `animejoy:playlist:<news_id>` 3-6h. Handle absolute-vs-per-series episode numbering.
- `sibnet.go` — **`ResolveSibnet(ctx, shellURL)`**: `GET iv.sibnet.ru/shell.php?videoid=` → regex player `/v/<hash>/<id>.mp4` → follow 302 → `dvNN.sibnet.ru/...mp4?e=<exp>`; return `{URL, Referer:"https://video.sibnet.ru/"}`. (Port `embeds/okru.go` shape: 2 MiB cap, host-equality match, absolute-URL guard.)
- `allvideo.go` — **`ResolveAllVideo(ctx, embedURL)`**: `fsst.online/embed/<id>` → 301 `incvideo1.online` → playerjs `file=` → `get_file` → 302 → `filevideo1.com/...mp4`; return `{URL, Referer:"https://fsst.online/"}`.

### 3.2 Seed rows (TWO): `services/catalog/internal/service/scraperprovider/seed.go`

```go
// defaultProviders (~line 19), after the kodik rows — animejoy itself is NOT a row
// (it is the discovery base); the two providers are:
{Name: "animejoy-sibnet", Status: domain.StatusDegraded /* soak first */,
 SupportsSub: true, SupportsDub: false, SupportsRaw: false,
 SubDelivery: "hard", QualityCeiling: "1080p", PreferenceWeight: 25,
 Reason: "Soaking — animejoy.ru via Sibnet", Description: "Sibnet (AnimeJoy, RU-sub)"},
{Name: "animejoy-allvideo", Status: domain.StatusDegraded,
 SupportsSub: true, SupportsDub: false, SupportsRaw: false,
 SubDelivery: "hard", QualityCeiling: "1080p", PreferenceWeight: 20,
 Reason: "Soaking — animejoy.ru via AllVideo", Description: "AllVideo (AnimeJoy, RU-sub)"},
```
- `intrinsicGroups` (seed.go:187): add `"animejoy-sibnet": "ru"` and `"animejoy-allvideo": "ru"`. **Do NOT** add either to `scraperOperatedNames` (seed.go:207) — that pulls them into the EN scraper-failover chain and crash-loops boot.
- **Prod migration** (`scraperprovider/migrate.go` + ordering in `catalog-api/main.go:173-259`, after `SeedDefaults`): one-time guarded INSERT of **both** rows mirroring `AnimepaheBrowserRevival`/`SplitKodik` (server IS prod; seed is insert-if-absent only).

### 3.3 Capability families (TWO, shared builder): `services/catalog/internal/service/capability/families_ru.go`

- One parameterized builder `animejoyLegFamily(ctx, animeID, provider, leg)` (copy `kodikFamily`, :88), invoked twice — `("animejoy-sibnet","sibnet")` and `("animejoy-allvideo","allvideo")`. Both call the **shared** `GetAnimejoyTeams` (discovery resolved once, cached), build `[]domain.Variant{Category:"sub", Team:{ID,Name}, SubDelivery:"hard"}`, load `providerRow(ctx, provider)`, `applyFeedFields`. A leg with no episode for a title yields an empty/absent family (no_content), not an error.
- `CatalogSource` iface (families_ru.go:11): add `GetAnimejoyTeams(ctx, animeID)([]domain.AnimejoyTeam, error)` (shared); implement on `*CatalogService`; add stub to `fakeCatalog` (families_ru_test.go:22) or the package won't compile.
- `buildFamilies` (service.go:99-122, inside the `s.catalog != nil` guard): declare both slots, `wg.Add(2)` + two goroutines, append in stable order (:118).
- DisplayName "Sibnet" / "AllVideo" in the labels map (`capability/rank.go:82`).

### 3.4 Resolution endpoint + streaming

- `GetAnimejoyStream(ctx, animeID, ep, teamID)`: `ResolveNewsID` → `FetchPlaylist` → pick episode → **`ResolveSibnet` → on error `ResolveAllVideo`** → `domain.Stream{Sources:[{URL, Type:"mp4"}], Headers:{"Referer": leg.Referer}}`. Type **must** be `"mp4"` (progressive, not HLS).
- Routes `/api/anime/{id}/animejoy/*` (mirror kodik in `transport/router.go:176`). Catalog signs sources via `streamsign.SignScraperStreamBody` (exp/sig); FE/probe call `/api/streaming/hls-proxy?url=&referer=&exp=&sig=`. The **provenance token authorizes** the unlisted Sibnet/AllVideo host (no static allowlist entry required); proxy forces IPv4 + derives Origin from Referer.
- Stream cache `animejoy:stream:<news_id>:<ep>:<leg>:sub` TTL = `computeStreamTTL(url)` (≤5min; `e=`/`expires=` parse; 0⇒don't cache).

### 3.5 Frontend: `frontend/web/src/composables/aePlayer/useProviderResolver.ts`

- `makeAnimejoyAdapter(api)` (`listEpisodes`/`resolveStream`/`listTeams`) + `if (provider === 'animejoy')` branch in `getAdapter`; add `animejoyApi` to `ResolverDeps`. **FE resolver is a hard gate** — a feed entry with no adapter throws `NotAvailableError` and fails at 0:00.
- `providerGroups.ts` — **no change** (the `ru` facet already covers it).
- All variants are `sub` → no sub/dub toggle reload; team chips appear automatically if teams are surfaced.

## 4. Data flow (concrete — Frieren ep 1)

1. `ResolveNewsID(Frieren)` → search "Frieren"/"Провожающая…" → JaroWinkler match → `news_id=3647` (cache 24h).
2. `FetchPlaylist(3647)` → teams; pick best RU-sub team; ep1 legs `{sibnet: iv.sibnet.ru/shell.php?videoid=…, allvideo: fsst.online/embed/…}` (cache 6h).
3. `ResolveSibnet` → `dvNN.sibnet.ru/…mp4?e=…` + `Referer: video.sibnet.ru` (UP).
4. Catalog signs → FE plays `/api/streaming/hls-proxy?url=<mp4>&referer=…&exp=…&sig=…` (cache ≤5min).
5. If Sibnet errors → `ResolveAllVideo` → `filevideo1.com/…mp4` + `Referer: fsst.online`.

## 5. Mechanisms (explicit)

- **Search**: DLE `do=search` + JaroWinkler ≥0.85 + year/season tiebreak + Cyrillic normalizer + 24h pos/neg cache.
- **Playlist**: `playlists.php?news_id=&xfield=playlist`, JSON-escaped nested tree, Sibnet/AllVideo filter, episode-number reconciliation (absolute vs per-series).
- **Leg resolution**: Sibnet shell→mp4+`e=`; AllVideo fsst→incvideo1→filevideo1 mp4; both Referer-gated, 2 MiB cap, absolute-URL guard.
- **Streaming**: tokened MP4 + Referer → catalog exp/sig sign → hls-proxy (provenance bypass, force-IPv4, Origin-from-Referer).
- **Failover**: Sibnet primary → AllVideo fallback per (team, episode).
- **Caching tiers**: newsid 24h · playlist 3-6h · stream ≤5min.
- **Policy/health**: seed DEGRADED → soak → auto-promote (existing self-healing); per-leg health for diagnostics.

## 6. Error handling & resilience / caveats

- **Mirror rot** — episodes get deleted (Frieren's Dzen/CDA were dead); Sibnet→AllVideo failover, then surface "episode unavailable" (don't hard-error the family).
- **Short/IP-bound tokens** — resolve near play-time, never cache signed URLs long, always stream via our proxy.
- **Title/season mis-match** — Russian titles + DLE search; mitigated by catalog year/season + JaroWinkler + Cyrillic fold; expect a tuning pass.
- **Domain volatility** — animejoy.ru is RKN-flagged + a DLE moving target; configurable base URL + mirror fallback (`animejoya.ru`); fail-soft.
- **Low marginal value** — duplicates Kodik/OK.ru coverage; net gain = redundancy + extra RU-sub teams. Acknowledged; ship behind DEGRADED.
- **Wire-id discipline** — `animejoy` must be identical across DB row, `ProviderCap.Provider`, FE `getAdapter`, family string.

## 7. Testing

- **Phase 1-2 unit tests (offline, no network)** — fixtures already captured: the One Piece/Frieren/etc. playlist JSON, Sibnet `shell.php`, AllVideo embed HTML. Test: search-match (incl. season disambiguation: Code Geass R2 vs S1; Slime S2 vs "часть 2"), playlist parse, Sibnet/AllVideo URL extraction, `computeStreamTTL` on `e=`.
- **Integration** — `GetAnimejoyStream` end-to-end through hls-proxy on 5 sweep titles; confirm 206 + `video/mp4` + **burned-in RU subs present**.
- **Probe** — add `animejoy` to `PROBE_PROVIDERS` roster; streamprobe already validated the legs.

## 8. Out of scope (YAGNI)

- The other animejoy legs (cdnjoyka/Наш плеер, Mail, Dzen, CDA, Matreshka).
- RU **dub** (animejoy is sub-only per directive).
- Surfacing `animejoy` itself as a selectable provider (it is the discovery base only, never a source chip).
- Soft-subtitle extraction (mirrors are hardsubbed).
- Camoufox/browser transport (discovery is plain HTTP).

## 9. Effort / impact (project conventions)

- **UXΔ = +1 (Slightly better)** — adds a redundant RU-sub source; real but marginal (Kodik/OK.ru already cover RU). Value is failover breadth + extra fansub teams.
- **CDI = 0.02 * 12** — one new isolated parser package (5 files) + the standard Kodik-family surfacing edits (seed row + migration + family + buildFamilies + CatalogSource + FE adapter); medium spread, low shift (every touchpoint has a Kodik analog).
- **MVQ = Griffin 84%/82%** — clean fit to the Kodik seam; soft spots: dependence on animejoy's DLE/search shape + Sibnet/AllVideo URL shapes (mitigated by fixtures, configurable base URL, fail-soft), and season-disambiguation tuning.
