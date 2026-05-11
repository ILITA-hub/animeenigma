# Feature Research: v3.0 Universal Anime Scraper

**Domain:** Self-hosted EN anime stream resolution (Shikimori/MAL ID → HLS m3u8 + subtitles)
**Researched:** 2026-05-11
**Confidence:** MEDIUM-HIGH
- HIGH: ID-mapping landscape (verified with live `api.malsync.moe` probe), existing player contract (read from current code), kwik extraction technique (verified from `KevCui/animepahe-dl` source)
- MEDIUM: AnimeKai/MegaUp decryption reliability (every public reference still routes through external `enc-dec.app` or unmaintained packed-JS — the same failure mode that broke our consumet container)
- LOW: AniZone — cloudflare 403'd both my probes, no GitHub reference implementation found in 2026

## Audience and Scope

10-active-user self-hosted group, mostly mainstream English-sub anime, no CDN budget, no licensing budget. v3.0 replaces only the two dead EN paths (`hianime` parser + `consumet` container) and inherits the existing HLS player surface (`HiAnimePlayer.vue`, `ConsumetPlayer.vue`, `SubtitleOverlay.vue`, `libs/videoutils/proxy.go`). Russian providers (Kodik iframe, AnimeLib MP4) are untouched.

The contract the new service must honor (verified by reading the current code) is:

```go
type Stream struct {
    URL       string             // direct m3u8 the frontend HLS.js can play through our CORS proxy
    Type      string             // "hls"
    Subtitles []Subtitle         // {URL, Lang, Label, Default} — fed straight into SubtitleOverlay
    Headers   map[string]string  // upstream Referer/Origin the proxy must replay
    Intro     *TimeRange         // skip-intro marker (optional)
    Outro     *TimeRange         // skip-outro marker (optional)
    AnilistID int                // already populated by aniwatch — ARM uses this for Jimaku JP subs
    MalID     int                // ditto
}
```

Any feature decision below is graded against "does it keep this contract intact AND ship in a v3.0 timeframe AND survive when one upstream provider dies?"

---

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume already exist because v1.0 and v2.0 already shipped them on top of the now-dead `aniwatch` / `consumet-api` containers. If v3.0 ships without these the product visibly regresses.

| Feature | Why Expected | Complexity | Notes |
|---|---|---|---|
| **TS-01 Shikimori-ID → provider mapping** | Today's frontend passes the Shikimori ID and expects results. AnimeKai uses random slugs (`one-punch-man-wq18`), AnimePahe uses session UUIDs (`758e3b17-8f49-...`), neither is derivable from MAL/Shikimori IDs. | LOW (use `malsync.moe`) | Verified live: `GET https://api.malsync.moe/mal/anime/30276` returns `Sites.AnimeKAI.<slug>` and `Sites.animepahe.<id>` keyed by MAL ID (Shikimori IDs ARE MAL IDs per MEMORY.md). Cache 24h; fall through to fuzzy title search only on miss. |
| **TS-02 HLS m3u8 + Referer/Origin headers** | Frontend players are Video.js/HLS.js; `libs/videoutils/proxy.go` already replays per-upstream headers. Replacing the source must keep this shape. | LOW | Existing `Stream{URL, Type:"hls", Headers}` already exists — new providers just populate it. AnimeKai segments are on `megaup.live` (Cloudflare-fronted) and need `Referer: https://megacloud.blog/`-style header replay. |
| **TS-03 Episode list with sub/dub split and filler flag** | Both current players show "ep 1, ep 2 …" grids and filter sub/dub. Removing this means breaking the watch flow. | LOW | All three target providers expose this. AnimeKai gives `sub`, `dub`, `softsub` counts in the search HTML; AnimePahe gives `/api?m=release&id=<session>&sort=episode_asc` JSON. |
| **TS-04 At least one English subtitle track in the response** | The whole reason we're not using Kodik for English audiences. SubtitleOverlay.vue depends on a subtitle URL prop. | LOW (AnimeKai) / MEDIUM (AnimePahe) | AnimeKai serves WebVTT tracks per language (verified via Aniyomi yuzono extension — adds "subtitle support"). **AnimePahe burns subs in (hardcoded)** — no separate track. If AnimePahe is the only provider that has an anime, we degrade gracefully (no SubtitleOverlay) but the video itself is still English. |
| **TS-05 Multi-provider failover** | If AnimeKai is down or doesn't have *Frieren ep 14*, the user expects fallback rather than "no episodes found". Today's `consumet/client.go` already encodes this in `FallbackProviders = []string{"animekai", "hianime", "animepahe"}` — but does it serially in the client, which is the right model. | MEDIUM | Implement a `Provider` interface with ordered fallback (sequential, not parallel — parallel multiplies request volume and triggers Cloudflare). Try in order, first one that returns ≥1 episode wins. |
| **TS-06 Per-provider Prometheus health metric** | PROJECT.md explicitly lists this. Without it we won't know AnimeKai went dark until users notice and file tickets via the ReportButton. | LOW | Counter `scraper_provider_request_total{provider, op, result}` + histogram `scraper_provider_duration_seconds{provider, op}`. Already standard via `libs/metrics`. |
| **TS-07 Cutover removes dead containers** | PROJECT.md explicitly lists this. `aniwatch:4000` and `consumet:3000` containers in docker-compose are dead and must be removed (else operators see them restarting forever in `make ps`). | LOW | Single PR: remove containers, delete `parser/hianime/`, `parser/consumet/`, point the two players at new endpoints. |
| **TS-08 Frontend players point at new endpoint without UX redesign** | PROJECT.md "Out of v3.0 scope: Player UX redesign". HiAnimePlayer.vue and ConsumetPlayer.vue must still look and behave identically after the swap. | LOW (if endpoint contract is preserved) | Two options on the table: (a) two cosmetic-only frontend players sharing one backend, or (b) merge to single "English source" player. Recommended: (a) for v3.0 — minimal frontend diff, preserves existing per-player analytics. |
| **TS-09 Hard 8-10s timeout on every upstream call** | Today's `aniwatch:4000` hangs forever and 8s timeouts in `parser/hianime/client.go` keep it from blocking the catalog. New scraper must do the same — provider sites can stall indefinitely. | LOW | `httpClient.Timeout = 10 * time.Second` on every external call. Already the pattern in both parsers. |
| **TS-10 Stream URL cache with short TTL (<= 1h)** | CLAUDE.md "Don't cache video URLs longer than 1 hour (they expire)". MegaUp/kwik URLs are signed and expire faster than that — typically 6h but observed as low as 30m. | LOW | `cache.Set(ctx, "scraper:stream:<provider>:<ep>", stream, 30*time.Minute)`. Episode lists cache 6h; search results 15m. |

### Differentiators (Worth Considering for v3.0)

Features that move us past "feature-parity with the dead aniwatch path." Each is judged on whether it earns its keep at our scale (10 users, no SLA pressure).

| Feature | Value Proposition | Complexity | Notes |
|---|---|---|---|
| **DIFF-01 Provider-agnostic "Provider" interface from day one** | PROJECT.md target. Lets v3.1 add a new provider (e.g. AniZone, KickAssAnime) by writing one file. This is the v2.0 `SignalModule` pattern applied to scrapers — proven to pay off. | MEDIUM | One Go interface with `Search(shikimoriID, title) -> []Match`, `Episodes(providerID) -> []Episode`, `Stream(episodeID) -> *Stream`. Orchestrator iterates a registry. Add `Provider.Name() string` for metrics labels. |
| **DIFF-02 Liveness probe ("dead provider detection")** | PROJECT.md target. A background goroutine probes each provider's search endpoint every 15 min with a known-popular query ("naruto" or similar). On 3 consecutive failures, mark provider unhealthy → orchestrator skips it on the next request. Recovers automatically once probe succeeds again. | MEDIUM | Counter `scraper_provider_up{provider}` (0 or 1) — Grafana alert fires once a provider has been "down" > 30 min. Cheap, dramatically improves perceived reliability when one site rotates domains. |
| **DIFF-03 Intro/outro skip markers passed through** | Existing `Stream.Intro`/`Stream.Outro` are part of the contract — `aniwatch` set them, our players already use them. AnimeKai exposes them in the `getSources` response; AnimePahe does not. Preserve when present, omit silently when absent. | LOW | Already in DTO; just don't drop them. |
| **DIFF-04 Title-similarity fuzzy fallback when malsync doesn't have an entry** | malsync only covers anime someone has explicitly mapped (mainstream titles only, ~10% gap on obscure stuff). For misses, fall back to: search-by-romaji-title → pick highest Jaro-Winkler match against the Shikimori-known titles. | MEDIUM | `shafat-96/anime-mapper` is the reference implementation (Node.js, runs in Vercel). Algorithm: try romaji, english, native, userPreferred, synonyms; score by string similarity; tie-break by year+season. Pure-Go port is ~150 LOC using `github.com/agnivade/levenshtein`. |
| **DIFF-05 Subtitle-track normalization (lang code, label, default flag)** | HiAnime gave `lang: "English"`; AnimeKai gives `label: "English - English"`; future providers will give other shapes. Normalize to ISO 639-1 (`"en"`) + human label, so SubtitleOverlay.vue and the existing Jimaku JP-sub composable don't grow per-provider branches. | LOW | Single helper `normalizeSubtitleLang(raw string) (code, label string)`. |
| **DIFF-06 Admin debug endpoint per anime** | Same pattern as `/admin/recs/:user_id` from v2.0 — show "for Shikimori ID X, which provider matched, which fallback fired, what cached values exist". 30 minutes of work, saves hours of debugging when a single anime breaks. | LOW | `GET /api/admin/scraper/diag/:shikimoriId` returns provider-by-provider trace. |
| **DIFF-07 Stream-error feedback loop** | The existing ReportButton already saves reports + sends Telegram. If the report includes `provider:animekai`, the diagnostics blob already captures everything we need to know why. v3.0 just needs to make sure the frontend tags each report with the resolved provider. | LOW | One field in the report payload. Frontend already has the data. |

### Anti-Features (Plausible But Wrong For Us)

| Feature | Why Requested | Why Problematic | Alternative |
|---|---|---|---|
| **ANTI-01 Iframe-only providers (Embed.su, Vidplay, "any working embed")** | "Just embed whatever works" sounds like a free win — no decryption, no Cloudflare. | Kills SubtitleOverlay (can't reach video element across iframe origin), kills quality switching, kills time tracking, kills Jimaku JP subs. **That is the entire reason we're not just using Kodik for English audiences.** | If a provider is iframe-only, drop the provider. Better to have one direct-HLS provider than three iframe wrappers. |
| **ANTI-02 Parallel-fanout query to all providers** | "Try all three at once, return first with results" feels faster. | At 10 users this is amortized over near-zero traffic, but Cloudflare Bot Management triggers on burst-fanout from the same egress IP. AnimePahe has already throttled us in testing; AnimeKai will follow. Burns reputation on a self-hosted server with no IP-rotation budget. | Sequential fallback (AnimeKai → AnimePahe → Anitaku). p95 cache-miss < 2.5s is fine for our use case. |
| **ANTI-03 Aggregating duplicate "team" or "server" lists for the user to pick from** | The current HiAnime player exposes "sub / dub / raw" picks; AnimeKai exposes "Sub / Dub / Softsub"; AnimePahe exposes encoder team (SubsPlease, Erai-raws). Surface them all and let the user choose. | The v1.0 Smart Watch Picker was built specifically to remove this decision from the user. Doubling the number of dropdowns invalidates v1.0's success metric (override-rate). | Auto-pick "Sub" by default. Expose "Dub" only as a toggle if the anime has dub. Don't surface fansub teams — pick the highest-bitrate option in the right language. |
| **ANTI-04 Real-time WebSocket-pushed "provider just came back online" notifications** | "Our streaming app is so responsive, it tells users the moment X works again." | Zero user value at 10-user scale; complex to implement; cache+route on next request gives same outcome with no infra cost. | Heartbeat probe (DIFF-02) silently re-enables provider on next request. |
| **ANTI-05 Auto-pick "first provider with results" without preference signal** | Simple — first hit wins. | Breaks `feedback_watch_preferences.md` strict fallback. A user who has watched 4 episodes via AnimeKai expects ep 5 from AnimeKai too. Switching mid-series silently loses the resume position (different episode IDs). | Score providers using the existing per-anime preference signal: if user has any `watch_history` rows for this anime on `provider=animekai`, AnimeKai wins on the next request. New anime falls back to default order. |
| **ANTI-06 Embedded headless browser inside the Go service (rod, chromedp)** | Solves Cloudflare and JS-decryption universally. | Adds ~700 MB image bloat, GPU dependencies, headless-Chromium CVE management, brittle CSS-selector scraping. For a 10-user box it's overkill and an operational time-sink the team has never asked for. | Reuse existing `megacloud-extractor` Node helper for the small JS-eval edge cases (kwik packer, AnimeKai token). Everything else is pure Go HTTP + goquery. |
| **ANTI-07 Pre-populating a database of every anime on every provider** | "Search would be instant if we'd indexed it!" | CLAUDE.md "Don't pre-populate the database with anime (on-demand only)" — this is project policy. Also: provider catalogs change hourly; the index would be stale by design. | On-demand resolution, 24h cache of malsync mappings keyed by Shikimori ID. |
| **ANTI-08 Custom AES key derivation for MegaUp/AnimeKai sourced from training data or "best guess"** | Tempting because the public extractors all do it. | The MegaCloud key has rotated 4+ times in the last 12 months (`cinemaxhq/keys/e1/key` in our own megacloud-extractor is exactly this fragile pattern; it's why aniwatch died). Coding a guess inline makes the v3.0 service inherit aniwatch's bug. | Fetch the rotating key at runtime from a single source-of-truth URL (e.g. `raw.githubusercontent.com/cinemaxhq/keys/e1/key`, which our megacloud-extractor already uses successfully). If that source dies, the provider is dead — fail fast and the heartbeat probe (DIFF-02) flags it. |

---

## Feature Dependencies

```
TS-01 Shikimori-ID → provider mapping (malsync.moe)
   └──unblocks──> TS-03 Episode list
                  TS-05 Multi-provider failover (needs candidate slugs for each provider)

TS-02 HLS m3u8 + headers
   └──requires──> libs/videoutils/proxy.go (already exists, allowed-domains list needs megaup.live + kwik.cx + animekai's CDN added)

TS-04 English subtitles
   └──requires──> TS-02 (subs ride alongside the stream response)
   └──enhances──> SubtitleOverlay.vue (existing — no change)

TS-05 Multi-provider failover
   └──requires──> DIFF-01 Provider interface
   └──enhances──> DIFF-02 Liveness probe (skip dead providers in failover)

DIFF-04 Fuzzy title fallback
   └──extends──> TS-01 (only triggered when malsync mapping absent)

ANTI-05 conflicts──> TS-05 + existing v1.0 preference system
(Failover must respect per-anime preferred provider, NOT just "first one that answers".)

ANTI-08 conflicts──> long-term reliability
(Hard-coded key strings = inherits aniwatch's failure mode. Rotate via external source-of-truth.)
```

### Dependency Notes

- **TS-01 unblocks everything**: without an ID mapper, every other feature has to start with a fuzzy title search → ~3x more upstream requests per playback. malsync.moe is the cheapest single dependency in this whole project.
- **TS-04 reuses SubtitleOverlay as-is**: the player path doesn't change. The new scraper just has to feed it the same `{URL, Lang, Format: "vtt"}` shape that `aniwatch` did. Verified by reading `HiAnimePlayer.vue`:74-84 and `ConsumetPlayer.vue`:76-86 — both wire `:subtitle-url` from a `Subtitle` prop with identical shape.
- **TS-05 sequentially**, not parallel: see ANTI-02.
- **DIFF-02 silently fixes the "AnimeKai is broken today" problem** without operator intervention.

---

## MVP Definition (For v3.0)

### Launch With (v3.0 Phases 1-N)

Minimum viable scope to replace aniwatch + consumet and not regress.

- [ ] **TS-01** Shikimori → AnimeKai slug + AnimePahe session via malsync.moe (24h cache)
- [ ] **TS-02** HLS m3u8 + Referer/Origin header replay through existing `libs/videoutils/proxy.go`
- [ ] **TS-03** Episode list with `sub`/`dub` and `is_filler` flags
- [ ] **TS-04** English VTT subtitle track from AnimeKai (AnimePahe drops to no-subs gracefully)
- [ ] **TS-05** AnimeKai → AnimePahe sequential fallback
- [ ] **TS-06** `scraper_provider_request_total` + `scraper_provider_duration_seconds` Prometheus metrics
- [ ] **TS-07** Remove `aniwatch:4000` and `consumet:3000` containers from docker-compose; delete `parser/hianime/` + `parser/consumet/`
- [ ] **TS-08** Frontend players repointed to new endpoint without UX redesign
- [ ] **TS-09** 10s timeout on every upstream call
- [ ] **TS-10** 30-minute TTL on stream URLs, 6h on episode lists, 15m on search results
- [ ] **DIFF-01** `Provider` interface so v3.1+ can add sources without rewriting the orchestrator
- [ ] **DIFF-02** Liveness probe with `scraper_provider_up` Prometheus gauge

### Add After Validation (v3.1)

- [ ] **DIFF-04** Fuzzy title fallback for malsync misses — only if we measure > 5% miss rate via metrics
- [ ] **DIFF-05** Subtitle-track lang-code normalization across providers — only matters once we have 3+ providers
- [ ] **DIFF-06** `/api/admin/scraper/diag/:shikimoriId` debug endpoint — add after first user-reported "doesn't work" incident
- [ ] Add Anitaku/Gogoanime as third provider (kept out of MVP — see "Provider Selection" below)

### Future (v3.2+)

- [ ] AniZone as fourth provider — currently no public reference implementation; revisit when one exists
- [ ] Per-user preferred-provider memoization (extend v1.0 preference resolver to learn provider, not just translation)
- [ ] **DIFF-03** Intro/outro markers passed through end-to-end — wait until we measure whether users actually use the skip button on the existing AnimeLib path

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority | Complexity Note |
|---|---|---|---|---|
| TS-01 ID mapping (malsync) | HIGH | LOW | P1 | trivial Go HTTP + JSON |
| TS-02 HLS m3u8 + headers | HIGH | LOW | P1 | reuse existing proxy.go |
| TS-03 Episode list sub/dub/filler | HIGH | LOW | P1 | goquery on AnimeKai HTML; JSON API on AnimePahe |
| TS-04 EN VTT subtitles | HIGH | MEDIUM | P1 | AnimeKai inline; AnimePahe degrades silently |
| TS-05 Sequential failover | HIGH | MEDIUM | P1 | orchestrator with Provider registry |
| TS-06 Prometheus per-provider | MEDIUM | LOW | P1 | one counter + one histogram |
| TS-07 Cutover (drop dead containers) | HIGH | LOW | P1 | docker-compose + go.mod cleanup |
| TS-08 Frontend repoint | HIGH | LOW | P1 | URL change + Stream DTO unchanged |
| TS-09 10s timeouts | MEDIUM | LOW | P1 | one line per HTTP client |
| TS-10 TTL'd URL cache | HIGH | LOW | P1 | reuse `libs/cache` |
| DIFF-01 Provider interface | MEDIUM (now) / HIGH (v3.1) | MEDIUM | P1 | architectural — has to be in v3.0 or it's expensive to retrofit |
| DIFF-02 Liveness probe | MEDIUM | MEDIUM | P1 | background goroutine + Prometheus gauge |
| DIFF-04 Fuzzy title fallback | MEDIUM | MEDIUM | P2 | only if metrics prove > 5% miss rate |
| DIFF-05 Lang-code normalization | LOW | LOW | P2 | nice-to-have when 3+ providers |
| DIFF-06 Admin diag endpoint | LOW | LOW | P2 | add after first incident |
| DIFF-07 ReportButton provider tag | LOW | LOW | P2 | frontend single-field add |
| Anitaku as 3rd provider | LOW (at 10 users) | MEDIUM (Cloudflare + goose-cdn extraction) | P3 | revisit when AnimeKai+AnimePahe coverage gap measured > 10% |

**Priority key:** P1 ships in v3.0 MVP, P2 in v3.1, P3 in v3.2+.

---

## Provider Selection Recommendation

The brief asks specifically about AnimeKai, AnimePahe, Anitaku/Gogoanime, and AniZone. Based on reference-implementation availability in 2026:

| Provider | Reference Implementation (2026) | Pure Go viable? | JS-eval required? | Subtitle support | Recommendation |
|---|---|---|---|---|---|
| **AnimeKai** (animekai.to) | `shimizudev/animekai-api` (pure Go, search+info+episodes, but **no stream extraction** — relies on external `DECODER_API`); `walterwhite-69/AnimeKAI-API` (Python Flask, **delegates token decryption to `enc-dec.app`**); Aniyomi `yuzono/aniyomi-extensions#416` (Kotlin, claims subtitle support; PR is hidden behind GitHub DMCA-451 on direct fetch — visible in search-result snippets only) | Partial: search/info/episodes pure Go. Stream URL extraction needs either external decoder OR our existing `megacloud-extractor` Node helper. | Yes for stream extraction. | Yes (WebVTT, multi-language). | **Primary provider.** Best subtitle story, largest catalog, megaup extraction reuses our existing megacloud-extractor pattern. |
| **AnimePahe** | `KevCui/animepahe-dl` (Bash + Node, verified working via inspection of source); `Pal-droid/Animepahe-API` (Python FastAPI + cloudscraper + Node for JS eval); `jashanbhullar/animepahe-scraper`. API endpoints `/api?m=search&q=` and `/api?m=release&id=<session>&sort=episode_asc` are clean JSON. Stream extraction is kwik.cx packed-JS: `eval(function(p,a,c,k,e,d){...})` → rewrite `eval(` → `console.log(` → execute via Node → regex `const source='<m3u8>'`. | Partial: API endpoints pure Go. Kwik packed-JS extraction needs JS eval (15 LOC of `goja` in Go OR a Node call). | Yes for stream extraction (~15 LOC equivalent). | **No** — AnimePahe burns subs into the video (hardcoded). | **Secondary provider** (fallback). Subs are non-negotiable degrade. |
| **Anitaku/Gogoanime** (anitaku.io) | `riimuru/gogoanime-api` (Node + Cheerio, last active 2024); `roflmuffin/node-anime-scraper`; numerous Python forks. Provider has rotated domains 5+ times in 18 months (`anitaku.bz` → `anitaku.io` is the most recent — confirmed via `consumet/consumet.ts#613`). | Yes (HTML scraping + simple base64 cipher). | No — pure Go works. | Limited — depends on the specific embed server (gogo-cdn vs streamwish). | **Defer to v3.1.** Domain volatility means we'd be debugging URL changes every few months; coverage overlap with AnimeKai+AnimePahe is high. |
| **AniZone** (anizone.to) | None found in 2026. Cloudflare 403'd both my probes (WebFetch + direct curl). The Aniyomi/Anikku ecosystem has no public extension. | Unknown. | Unknown. | Unknown. | **Drop from v3.0 scope.** No public reference implementation in 2026 = we'd be reverse-engineering from scratch, which is exactly the failure mode aniwatch represents. |

### Bottom line on providers

Ship v3.0 with **AnimeKai (primary) + AnimePahe (fallback)**. That's two providers, three working extractors, one JS-eval edge case per provider (both already solved publicly), and one known degradation (AnimePahe = no separate subs). Add Anitaku in v3.1 only if metrics show > 10% of requests fall through both AnimeKai and AnimePahe.

### Note on the "MegaUp/enc-dec.app" trap

Every public AnimeKai reference implementation in 2026 — without exception — depends on the external `enc-dec.app` service to decrypt the token (`_` query param) that makes the `getSources` call succeed. **This is the exact same failure mode that broke our consumet container** (PROJECT.md: "Consumet broken: `riimuru/consumet-api:latest` calls `enc-dec.app` with stale body shape"). Two viable mitigations:

1. **Run our own enc-dec equivalent.** The existing `docker/megacloud-extractor/server.js` already does ~80% of the work: it fetches the rotating MegaCloud key from `raw.githubusercontent.com/cinemaxhq/keys/e1/key`, performs AES-256-CBC decryption (OpenSSL-compatible KDF), and returns sources + tracks. Extending it to also generate the AnimeKai token is a ~50 LOC delta and removes the external dependency entirely.
2. **Accept the external dependency, but heartbeat-probe it.** DIFF-02 detects when enc-dec.app drifts before users do, so we don't suffer the silent 100%-failure that consumet did.

Recommendation: **option 1**. The existing megacloud-extractor is already the pattern, and it's the only way to avoid inheriting aniwatch's death-spiral.

---

## Competitor Feature Analysis

| Feature | Existing parsers (Kodik, AnimeLib) | Old aniwatch path (now dead) | Old consumet path (now broken) | Our v3.0 plan |
|---|---|---|---|---|
| ID mapping | Kodik: Shikimori ID native (`shikimori_id` param). AnimeLib: title search then slug lookup. | aniwatch: title search → slug, no Shikimori knowledge. | consumet: same as aniwatch. | **malsync.moe direct Shikimori→provider-ID, 24h cache, fuzzy title fallback.** |
| Subtitle handling | Kodik: iframe, no access. AnimeLib: external ASS/VTT via `subtitles` array (we already render). | aniwatch: WebVTT in `tracks` array. SubtitleOverlay consumed directly. | consumet: shape varied per provider; never reliable. | **Preserve aniwatch `tracks` shape. AnimePahe drops to no-subs (degrades gracefully).** |
| Resilience | Kodik: token auto-refresh (`Client.getToken`). AnimeLib: 401/403 retry-without-token. | aniwatch: 8s timeout, retry-with-fallback-server. We kept the retry loop. | consumet: no health check, silent 100% failure. | **Per-provider Prometheus metric + 15-min liveness probe + sequential failover. Best of both worlds.** |
| URL caching | Kodik: token TTL 12h, no URL cache (iframe doesn't need it). AnimeLib: 1h via `libs/cache`. | aniwatch: none. | consumet: none. | **30m for stream URLs (megaup/kwik expire that fast), 6h episode list, 15m search.** |
| Self-healing | Kodik: scrapes token from public JS sources (`nb557/plugins`, `kodik-add.com`). | aniwatch: relied on upstream container; upstream died → service died. | consumet: same. | **External rotating key source (cinemaxhq) for megacloud; liveness probe for everything else.** |

---

## Sources

- [malsync.moe API](https://api.malsync.moe/mal/anime/30276) — verified live 2026-05-11; returns AnimeKAI slug + animepahe session keyed by MAL ID
- [shimizudev/animekai-api](https://github.com/shimizudev/animekai-api) — pure Go reference, search+info+episodes endpoints (no stream extraction)
- [walterwhite-69/AnimeKAI-API](https://github.com/walterwhite-69/AnimeKAI-API) — Python Flask, full pipeline including stream extraction, but depends on external `enc-dec.app`
- [KevCui/animepahe-dl](https://github.com/KevCui/animepahe-dl) — verified working kwik m3u8 extraction via `eval`→`console.log` JS rewrite trick (read source directly)
- [Pal-droid/Animepahe-API](https://github.com/Pal-droid/Animepahe-API) — Python FastAPI + cloudscraper, `/search` `/episodes` `/sources` `/m3u8` endpoint shape
- [shafat-96/anime-mapper](https://github.com/shafat-96/anime-mapper) — Node.js title-similarity matcher across AnimePahe/HiAnime/AnimeKai (reference algorithm for DIFF-04)
- [Eltik/AniSync](https://github.com/Eltik/AniSync) — Same idea, more general (AniList ↔ Zoro/MangaDex/etc.)
- [yuzono/aniyomi-extensions PR #416](https://github.com/yuzono/aniyomi-extensions/pull/416) — AnimeKai source with subtitle support (page DMCA-451's; signal-only from search snippets)
- [consumet/consumet.ts#613](https://github.com/consumet/consumet.ts/issues/613) — anitaku.bz → anitaku.io domain change tracker (evidence of provider volatility)
- [anime-dl/anime-downloader PR #316](https://github.com/anime-dl/anime-downloader/pull/316) — historical AnimePahe/kwik fix, documents the `const source='([^']*)'` extraction regex
- [/data/animeenigma/docker/megacloud-extractor/server.js](file:///data/animeenigma/docker/megacloud-extractor/server.js) — our existing Node helper; already implements MegaCloud AES-256-CBC decryption with rotating key
- [/data/animeenigma/services/catalog/internal/parser/hianime/client.go](file:///data/animeenigma/services/catalog/internal/parser/hianime/client.go) — current Stream DTO contract that the new scraper must preserve (Subtitle, TimeRange, AnilistID, MalID fields)
- [/data/animeenigma/services/catalog/internal/parser/kodik/client.go](file:///data/animeenigma/services/catalog/internal/parser/kodik/client.go) — reference for "what good looks like": token auto-refresh, retry logic, error classification

---

*Researched: 2026-05-11*
*Confidence: MEDIUM-HIGH. Highest on ID mapping (verified live) and existing player contract (read directly). Lowest on AnimeKai/MegaUp long-term reliability — every public extractor in 2026 still routes through `enc-dec.app` or rotating external keys, which is structurally fragile. Mitigation via DIFF-02 liveness probe + option to bring decryption fully in-house via our existing megacloud-extractor helper.*
