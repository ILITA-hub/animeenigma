# Phase 28: Provider Expansion Round 2 — Research

**Researched:** 2026-05-20
**Domain:** Go microservice provider implementation (anime stream scrapers) + Vue 3 frontend i18n + Playwright e2e
**Confidence:** HIGH for AnimeFever (live probe successful), HIGH for 9anime.me.uk (live probe successful), MEDIUM for Miruro (kept obfuscation spike scope honest), HIGH for housekeeping items (existing scraper template established by Phase 26 + Phase 27).

## Summary

Phase 28 grows the EN scraper failover pool from 1 working provider (allanime) to up to 4 by lifting three new providers — AnimeFever, Miruro (conditional), 9anime.me.uk — using the established `domain.Provider` template from Phase 26 Wave 1. The architectural template is fully locked: each provider is a self-contained package under `services/scraper/internal/providers/{name}/` implementing the 6-method `domain.Provider` interface, registered in `cmd/scraper-api/main.go` in failover-chain order. New embed extractors (if any) are added as `services/scraper/internal/embeds/<host>.go`. HLS proxy allowlist entries land per-provider in `libs/videoutils/proxy.go`.

Live upstream probes confirmed Phase 28's primary-fallback (AnimeFever) and last-resort (9anime.me.uk) endpoints are reachable from the production server, and Miruro's obfuscation keys (`VITE_PROXY_OBF_KEY`, `VITE_PIPE_OBF_KEY`) remain stable at the values observed in the 2026-05-19 survival sweep. AnimeFever's data path resolves cleanly: `/search/<term>` (HTML, returns `/info/<slug>` URLs) → `/info/<slug>` (HTML, returns `/watch/<slug>?ep=<id>` URLs) → `/ajax/anime/load_episodes_v2?s=<server>` (JSON POST with `ctk` token, returns iframe to `am.vidstream.vip`) → embed page (JWPlayer-style HTML with inline `sources: [{"file":"...m3u8"}]`). 9anime.me.uk's data path resolves cleanly: `/wp-json/wp/v2/search?search=<term>&subtype=series` (JSON) → series page HTML (parses `<a class="ep-item">` for episode list) → episode WP post HTML (extracts `<iframe src="https://my.1anime.site/index.php?action=play&file=<name>.mp4">`) → embed page (parses `<source src="videos/<name>.mp4">`). Both paths are pure HTML/JSON scrape, no JS challenges, no anti-bot.

**Primary recommendation:** Execute the 4-wave plan exactly as the CONTEXT.md plan-sketch defines. AnimeFever ships as a clean HTML scrape (slot 4) reusing `BaseHTTPClient`'s cookie jar + Jaro-Winkler from `services/scraper/internal/fuzzy/`. The vidstream.vip embed needs a new extractor (one ~80-line file) using the same `packed_common.go`-style pattern but with a plain regex extractor (no Dean-Edwards-packer unpack needed — the HLS URL is in a plain `sources: [{"file":"..."}]` literal). 9anime ships as a clean HTML scrape (slot 6, last-resort) with a brand-new `my.1anime.site` extractor that emits an MP4 Source (the frontend already supports MP4 via the AnimePahe-via-Kwik precedent). Miruro's obfuscation spike (28-00) hard kill-switch at 4 agent-sessions stays as-written — convergence criteria in CONTEXT.md D3 are correct; the obfuscation keys are HMAC-SHA256/AES-CTR-shaped constants and reverse-engineering them is the spike's defining work.

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SCRAPER-HEAL-34 | Miruro obfuscation spike — produce `SPIKE-MIRURO.md` verdict + Go-port reference impl if converged | Section "Miruro" + CONTEXT.md D3 convergence gates verified upstream (env2.js keys unchanged 2026-05-19 → 2026-05-20). |
| SCRAPER-HEAL-35 | AnimeFever embed-extractor recon spike — produce `SPIKE-ANIMEFEVER.md` with ordered embed-host list | Section "AnimeFever" — live recon resolved Frieren ep28 path to `am.vidstream.vip`; spike output expected: 1 new extractor (`vidstream_vip.go`) required. |
| SCRAPER-HEAL-36 | AnimeFever provider lift — `services/scraper/internal/providers/animefever/` | Section "AnimeFever" + "Architecture Patterns" — template established in 26-01 (allanime). |
| SCRAPER-HEAL-37 | Miruro provider lift — conditional on 34 spike convergence | Section "Miruro" — same template but with deobfuscation layer. |
| SCRAPER-HEAL-38 | New embed extractors revealed by SCRAPER-HEAL-35 recon | Section "AnimeFever / Embed Strategy" — `vidstream_vip.go` skeleton + Source extraction approach. |
| SCRAPER-HEAL-39 | 9anime.me.uk provider lift + housekeeping (dropdown polish + i18n + e2e) | Sections "9anime.me.uk" + "Frontend Dropdown Polish". |

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**D1 — Ship-order by reliability ceiling, not by alphabetical or by upstream-popularity.**
AnimeFever → Miruro → 9anime. Failover chain order: `gogoanime → animepahe → allanime → animefever → miruro → nineanime → animekai`. Higher-slot positions get probed less often.

**D2 — 9anime.me.uk is explicitly accepted as a low-quality, last-resort source.**
Operator policy "as many providers as possible" overrides natural "not-worth" verdict. Failover slot 6 (LAST). Operator mitigation via `SCRAPER_DEGRADED_PROVIDERS=nineanime` when it breaks (~6-month half-life expected). Alternate test target for 9anime: Marriagetoxin episode 1 OR 7 (Frieren absent in upstream catalog **for season 1**; the upstream DOES have S2 — see Section "9anime.me.uk" upstream probe).

**D3 — Miruro spike has a hard 4-agent-session kill-switch; failure does NOT block the phase.**
Convergence criteria (gate to advance to SCRAPER-HEAL-37):
1. Transform `(endpoint, OBF_KEY)` → obfuscated URL is implementable in pure Go using only `crypto/hmac`, `crypto/sha256`, `crypto/aes`, or `encoding/base64`.
2. A test call against `pro.ultracloud.cc/<transformed-url>` from production server returns HTTP 200 with parseable HLS/JSON.
3. Obfuscation keys appear stable across at least 3 sequential `env2.js` fetches (NOT session-rotated).
4. A spot-check against Frieren AniList ID (154587) returns a non-error episode listing.
Any gate fails → spike `killed`, SCRAPER-HEAL-37 rolls to v3.2.

**D4 — AnimeFever's embed-extractor recon (SCRAPER-HEAL-35) ships its own spike artifact.**
Recon fetches one Frieren episode page, identifies the embed iframe host(s), emits ordered list of `embeds/<host>.go` files to write in SCRAPER-HEAL-38. No speculative extractor writes.

**D5 — Failover-chain ordering is locked in CONTEXT.md and enforced in `main.go` Register order.**
1. gogoanime (degraded), 2. animepahe (degraded → revived in Phase 27), 3. allanime ★ working, 4. animefever NEW, 5. miruro NEW (conditional), 6. nineanime NEW, 7. animekai (gated stub). `SCRAPER_SERVER_PRIORITY` env override allowed.

**D6 — Test targets per provider follow `feedback_verify_streams.md`.**
AnimeFever: Frieren (MAL 52991, AniList 154587). Miruro: Frieren (AniList 154587). 9anime: Marriagetoxin ep 1 OR 7 (Frieren S1 absent; S2 present — verified upstream during research). Frieren E2E gate is a hard ship gate per provider.

**D7 — HLS proxy allowlist additions land per-provider, not in a single batch.**
`libs/videoutils/proxy.go` allowlist grows in the same commit that ships the provider it serves. New entries expected: AnimeFever → `am.vidstream.vip` + `static-cdn-ca1.mofl.pro` (verified during research). Miruro → `pro.ultracloud.cc` + `pru.ultracloud.cc`. 9anime → `my.1anime.site`.

### Claude's Discretion

- **Whether to write `vidstream_vip.go` extractor using the existing `packed_common.go` base or a new lighter base** — recon shows AnimeFever's embed is JWPlayer-style with inline `sources: [{"file":"...m3u8"}]` (no Dean-Edwards-packer wrap), so a new ~80-line plain HTML-regex extractor is the right shape. NOT a `packed_common.go` reuse.
- **Whether to include MAL ID metadata in 9anime's title-fuzzy scoring** — yes (CONTEXT.md `<open_questions>` second item). Use Shikimori title + EN title + episode count + year as fuzzy-match scoring inputs.
- **Source dropdown ordering** — failover-chain order (CONTEXT.md `<open_questions>` third item). Matches user expectation of "primary first."

### Deferred Ideas (OUT OF SCOPE)

- Backfill of `has_english` column for never-touched anime (v3.2 polish).
- WARP egress sidecar (revives VibePlayer; v3.2+ separate spec).
- MinIO segment archival (v3.2+ separate spec).
- Reviving gogoanime, animepahe, animekai (separate phases: 24, 27, 26-06 respectively).
- 9anime's broken WP `?s=` search workaround — we use `/wp-json/wp/v2/search` instead; if even that returns garbage, 9anime degrades to operator-curated slug map.
- AnimeFever rebrand-resilience layer (selector-change auto-recovery beyond Pattern 7 maintenance-bot — that's already Phase 25's coverage).
</user_constraints>

## Project Constraints (from CLAUDE.md)

The following CLAUDE.md directives apply directly to Phase 28's implementation surface:

- **Use `make redeploy-scraper`** (NOT raw `docker compose up`) to redeploy the scraper service.
- **Frontend uses `bun`** (NOT npm/pnpm) — Playwright runs as `bunx playwright test player`.
- **Always invoke `/animeenigma-after-update`** after completing implementation. Wave 3 plan 28-06 has this as its final step.
- **Add to `docs/issues/README.md`** for any incidents during the phase (ISS-NNN numbering).
- **Don't add CDN-related code** — providers stream-through, no MinIO archival.
- **Don't cache video URLs longer than 1 hour** — expiry-aware caching per allanime/animepahe pattern.
- **Don't add complex abstractions for simple operations** — each provider is one self-contained package, no shared `provider-base` lib (matches Phase 26 D1 "copy-with-adaptation, NOT a move").
- **Go service conventions:** snake_case files (`client.go`, `cache.go`, `dto.go`, `doc.go`), PascalCase types, camelCase variables.
- **Use shared `libs/errors`, `libs/cache`, `libs/logger`** — never reinvent.
- **The 4-co-author commit convention** (Claude Opus 4.7 + 0neymik0 + NANDIorg). 28-06 housekeeping plan's final commit includes all 3.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| New scraper provider implementations (animefever, miruro, nineanime) | API / Backend (scraper microservice) | — | Provider scraping = backend HTTP+HTML+JSON parsing only; never touches browser. |
| Embed extractors (vidstream_vip, ultracloud, 1anime_site) | API / Backend (scraper microservice) | — | Extractors run in the scraper service; results returned to frontend as `Stream` DTOs. |
| Provider registration + failover chain | API / Backend (scraper-api main.go) | — | `domain.Provider` interface and orchestrator are scraper-internal. |
| HLS proxy allowlist additions | API / Backend (streaming service via `libs/videoutils`) | — | The allowlist is consumed at request time by streaming service's `ProxyWithReferer`. |
| Source dropdown polish (capitalizeProvider branches) | Frontend Server (Vue build) | Browser (renders dropdown) | UI label rendering happens in Vue templates; capitalization is a static string transform. |
| Provider i18n labels | Frontend Server (Vue i18n) | Browser | i18n JSON files baked into build; resolved at component render time. |
| Playwright e2e (source switching) | Browser (Playwright headless) | Frontend Server (target) | E2E test driver runs against deployed frontend. |
| Miruro obfuscation spike artifact | Decision log (planning artifact, not runtime) | — | `.planning/phases/28-provider-expansion-r2/SPIKE-MIRURO.md` is documentation; the Go port (if converged) lives in `providers/miruro/`. |
| AnimeFever recon spike artifact | Decision log (planning artifact, not runtime) | — | `.planning/phases/28-provider-expansion-r2/SPIKE-ANIMEFEVER.md` is documentation; the new extractor lives in `embeds/`. |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `services/scraper/internal/domain.BaseHTTPClient` | n/a (project lib) | Single source of truth for upstream HTTP. retryablehttp + cookie jar + per-host rate limits + baseline headers. | SCRAPER-FOUND-06 NF — providers MUST NOT hand-roll their own http.Client. Confirmed `[VERIFIED: services/scraper/internal/domain/httpclient.go]`. |
| `services/scraper/internal/domain.Provider` | n/a (interface) | Contract every provider implements: Name, FindID, ListEpisodes, ListServers, GetStream, HealthCheck. | Adding a new provider = one struct + one Register call. `[VERIFIED: services/scraper/internal/domain/provider.go:134-141]`. |
| `services/scraper/internal/domain.EmbedExtractor` + `Registry` | n/a (interface) | Per-host embed extractor contract: Name, Matches, Extract. Registry.Find returns first matching. | Established in Phase 16; Kwik + Megacloud + StreamHG + Earnvids + VibePlayer all use it. `[VERIFIED: services/scraper/internal/domain/embed.go]`. |
| `services/scraper/internal/fuzzy.JaroWinkler` + `NormalizeTitle` | n/a (project lib) | Title-search fallback when canonical ID unavailable. Threshold 0.85 used by animepahe. | Already extracted from animepahe to shared `fuzzy` package; ready for 9anime title-fuzzy. `[VERIFIED: services/scraper/internal/fuzzy/`. |
| `services/scraper/internal/health` | n/a (project lib) | 5-stage canonical health: search/episodes/servers/stream/stream_segment. ProbeRunner + InMemoryHealthCache + sliding window. | Versioned contract per `health.AllStages`. `[VERIFIED: services/scraper/internal/health/stage.go:9-14]`. |
| `libs/cache` | n/a (project lib) | Redis cache with key-family helpers + TTL constants (`TTLEpisodes`, `TTLAnimeDetails`). | All existing providers use it; required for malsync 24h positive/negative cache. `[VERIFIED: services/scraper/internal/providers/animepahe/malsync.go]`. |
| `libs/logger` | n/a (project lib) | Structured logging (`log.Infow`, `log.Errorw`). | Project-wide standard per CLAUDE.md. |
| `libs/videoutils.HLSProxyAllowedDomains` | n/a (project lib) | Allowlist for `streaming` service's HLS proxy. | New embed hosts MUST be added or stream playback returns 502 (per Phase 25 SCRAPER-HEAL-24). `[VERIFIED: libs/videoutils/proxy.go:230-290]`. |
| `github.com/PuerkitoBio/goquery` | v1.10.3 | jQuery-style DOM querying for HTML scrape providers. | `[VERIFIED: services/scraper/go.mod]` — animepahe uses it. |
| `github.com/dop251/goja` | v0.0.0-20260311135729-065cd970411c | JS runtime for packed-JS embed extractors. | Used by kwik.go, streamhg.go, earnvids.go. Not needed for AnimeFever (plain regex extract) or 9anime (no JS). May be needed if Miruro's deobfuscation requires JS execution (LIKELY NOT — obfuscation is HMAC/AES, not JS-evaluated). |
| `github.com/hashicorp/go-retryablehttp` | v0.7.7 | Retry + backoff transport for BaseHTTPClient. | Already in use; no new dep. `[VERIFIED: services/scraper/internal/domain/httpclient.go]`. |
| `golang.org/x/net/publicsuffix` | (via golang.org/x/net v0.47.0) | Public-suffix-list scoped cookie jar. | Already in use; no new dep. |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `crypto/hmac`, `crypto/sha256`, `crypto/aes`, `crypto/cipher`, `encoding/base64` | (stdlib) | Pure-Go obfuscation transforms. | Miruro spike: porting `VITE_PROXY_OBF_KEY` transform. CONTEXT.md D3 gate 1 EXPLICITLY restricts to these stdlib packages. |
| `regexp` | (stdlib) | Inline-JSON extraction from HTML pages. | AnimeFever's vidstream.vip embed has `sources: [{"file":"...m3u8"}]` inline — regex extract is the right tool. |
| `encoding/json` | (stdlib) | Parsing AnimeFever's `/ajax/anime/load_episodes_v2` JSON response, 9anime's WP REST API. | Both endpoints return JSON. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `goquery` for HTML parsing | Raw `regexp` against HTML | Regex is brittle for structured DOM (selectors break less often than regex captures). Use goquery for episode-list scraping where `<a class="ep-item">` selectors apply. |
| New `vidstream_vip.go` extractor | Reuse `packed_common.go` | AnimeFever's embed is NOT Dean-Edwards-packed (verified during research — plain `sources: [{"file":"..."}]` in HTML). A reuse would force unnecessary goja overhead. Write a new lighter extractor with the same `domain.EmbedExtractor` shape. |
| Custom 9anime MP4 extractor | Add `my.1anime.site` to `HLSProxyAllowedDomains` and treat MP4 as a `domain.Source` directly without an extractor | An extractor is still needed because the `<iframe src>` → `<video><source src>` step requires an HTTP fetch + parse. The extractor returns a `Stream{Sources: [{URL: ..., Type: "mp4"}], Headers: {Referer: ...}}`. |
| Miruro pure-Go obfuscation | `utls` (TLS fingerprint spoof) or `chromedp` (headless Chrome) | CONTEXT.md D3 gate 1 EXPLICITLY rejects `utls`/`chromedp`. If either is required → spike `killed`. |

**Installation:** No new external Go modules are needed for AnimeFever or 9anime — both are pure HTML/JSON scrape using existing stack. Miruro's pure-Go obfuscation port uses stdlib only.

**Version verification:** Already in-use versions verified against `services/scraper/go.mod`:
- `github.com/PuerkitoBio/goquery v1.10.3` `[VERIFIED]`
- `github.com/dop251/goja v0.0.0-20260311135729-065cd970411c` `[VERIFIED]`
- `github.com/hashicorp/go-retryablehttp v0.7.7` `[VERIFIED]`

## Architecture Patterns

### System Architecture Diagram

```
Frontend (Vue 3)
    │  user picks "English" tab → user picks source from dropdown
    │  GET /api/anime/{uuid}/scraper/episodes?provider={animefever|miruro|nineanime}
    ▼
Gateway service (8000)
    │  routes /api/anime/* → catalog
    ▼
Catalog service (8081)
    │  resolves anime_id → AnimeRef{ShikimoriID, AniListID, Title, Year}
    │  GET scraper:8088/scraper/episodes
    ▼
Scraper service (8088)
    │  Orchestrator (services/scraper/internal/service/orchestrator.go)
    │    │  iterates registered providers in failover-chain order
    │    │  checks each provider's StageHealth via InMemoryHealthCache
    │    │  dispatches to first healthy provider
    │    ▼
    │  Provider.FindID(ctx, ref) → providerID (cached 24h via libs/cache)
    │  Provider.ListEpisodes(ctx, providerID) → []Episode (cached 6h)
    │  Provider.ListServers(ctx, providerID, episodeID) → []Server
    │  Provider.GetStream(ctx, ..., serverID, category) → *Stream
    │    │  Stream has 1+ Source{URL, Type:"hls"|"mp4", Quality} + optional Tracks + Headers
    │    │
    │    ├─ AnimeFever — calls Registry.Find(am.vidstream.vip embed URL)
    │    │    │  → VidstreamVipExtractor.Extract() → *Stream
    │    │
    │    ├─ Miruro — applies VITE_PROXY_OBF_KEY transform, calls pro.ultracloud.cc/<obf-url>
    │    │    │  may return HLS directly or feed UltracloudExtractor
    │    │
    │    └─ 9anime — fetches /episode-N-page → extracts iframe → fetches embed → parses MP4 URL
    │         │  → returns Stream{Sources:[{Type:"mp4"}], Headers:{Referer: ...}}
    ▼
HealthCheck → /scraper/health snapshot with 5-stage map per provider
Probe runner (15min ± 20% jitter, services/scraper/internal/health/probe.go)
    └─ fires golden-pool checks against each provider, populates cache + Prom metrics

Frontend playback
    │  for HLS: <video src="/api/streaming/hls?url={stream.URL}&referer={stream.Headers.Referer}">
    │  for MP4: <video src="/api/streaming/proxy?url={stream.URL}&referer={stream.Headers.Referer}">
    ▼
Streaming service (8082)
    │  validates stream URL host against libs/videoutils.HLSProxyAllowedDomains
    │  proxies bytes through (Phase 25 SCRAPER-HEAL-24 returns 502 on disallowed host)
```

### Component Responsibilities

| File | Responsibility |
|------|----------------|
| `services/scraper/internal/providers/animefever/client.go` | Provider struct + New() + 6 domain.Provider methods. FindID via title-search-with-MalSync-fallback (MalSync optional). ListEpisodes/ListServers scrape HTML via goquery. GetStream delegates to embed Registry. |
| `services/scraper/internal/providers/animefever/cache.go` | 4 key families: showID, episodes, servers, stream. TTLs match gogoanime/animepahe. |
| `services/scraper/internal/providers/animefever/dto.go` | Internal DTOs for parsing the `/ajax/anime/load_episodes_v2` JSON response (status/value/embed/html5/type/download_get fields). |
| `services/scraper/internal/providers/animefever/doc.go` | Package doc + upstream contract notes + lift decision log. |
| `services/scraper/internal/providers/animefever/client_test.go` | Compile-time interface assertion (`var _ domain.Provider = (*Provider)(nil)`) + table-driven tests against captured testdata fixtures. |
| `services/scraper/internal/providers/animefever/testdata/{search_frieren.html, info_frieren.html, watch_ep28.html, ajax_load_ep.json}` | Golden files for offline unit tests. |
| `services/scraper/internal/providers/miruro/client.go` (CONDITIONAL) | Provider struct + 6 methods + obfuscation transform. FindID via ARM (MAL → AniList → /anime/<anilist_id> URL). All upstream calls route through deobfuscated pro.ultracloud.cc. |
| `services/scraper/internal/providers/miruro/obfuscation.go` (CONDITIONAL) | Pure-Go port of VITE_PROXY_OBF_KEY transform. Stdlib only. |
| `services/scraper/internal/providers/nineanime/client.go` | Provider struct + 6 methods. FindID via WP REST API + Jaro-Winkler scoring (year + episode count tie-breakers). ListEpisodes scrapes series page for `<a class="ep-item">`. GetStream fetches episode WP post → iframe URL → embed page → MP4 Source. |
| `services/scraper/internal/providers/nineanime/cache.go` | Same 4 key families. Negative cache on WP search misses. |
| `services/scraper/internal/embeds/vidstream_vip.go` | New EmbedExtractor for `am.vidstream.vip`. Plain HTML regex extracts inline `sources: [{"file":"...m3u8"}]`. Returns Stream with HLS Source + Referer=https://am.vidstream.vip/. |
| `services/scraper/internal/embeds/onenime_site.go` (or inline in 9anime client) | EmbedExtractor for `my.1anime.site`. Fetches the play page, regex/goquery extracts `<source src="videos/<name>.mp4">`. Returns Stream with MP4 Source + Referer=https://my.1anime.site/. |
| `libs/videoutils/proxy.go` | Allowlist updates: `am.vidstream.vip`, `static-cdn-ca1.mofl.pro` (AnimeFever), `pro.ultracloud.cc`, `pru.ultracloud.cc` (Miruro), `my.1anime.site` (9anime). |
| `services/scraper/cmd/scraper-api/main.go` | Provider registration in failover-chain order. New BaseHTTPClient instances per provider with appropriate per-host rate limits. Updates to `candidateProviders` slice + Phase 19 wiring invariant. |
| `services/scraper/internal/config/config.go` | Per-provider config blocks (`AnimeFever.BaseURL`, `Miruro.{BaseURL,ProxyA,ProxyB}`, `NineAnime.BaseURL`). |
| `frontend/web/src/components/player/EnglishPlayer.vue` (if exists by Wave 3) | `capitalizeProvider` branches for `animefever`/`miruro`/`nineanime`. Source dropdown populated from `/scraper/health` snapshot. |
| `frontend/web/src/locales/{en,ru,ja}.json` | i18n labels for new provider names (e.g., `anime.scraper.providers.animefever: "AnimeFever"`). |
| `frontend/web/e2e/english-player.spec.ts` (new or extends existing) | Playwright e2e: log in as `ui_audit_bot` → navigate to Frieren → switch source from allanime → animefever → assert player still plays → switch to nineanime → assert MP4 source loads. |

### Recommended Project Structure

```
services/scraper/internal/
├── providers/
│   ├── allanime/          (existing — template reference)
│   ├── animefever/        (NEW — Wave 1 Plan 28-02)
│   │   ├── client.go      (~500 lines, mirrors allanime/client.go shape)
│   │   ├── cache.go       (~120 lines, 4 key families)
│   │   ├── dto.go         (~80 lines, JSON unmarshal shapes)
│   │   ├── doc.go         (~30 lines, package doc + decisions)
│   │   ├── client_test.go (~400 lines, 16+ table tests)
│   │   └── testdata/
│   │       ├── search_frieren.html
│   │       ├── info_frieren_14401.html
│   │       ├── watch_ep28.html
│   │       └── ajax_load_ep28.json
│   ├── miruro/            (NEW — Wave 2 Plan 28-04, CONDITIONAL)
│   │   ├── client.go
│   │   ├── obfuscation.go (NEW pattern — pure-Go OBF transform)
│   │   ├── cache.go
│   │   ├── dto.go
│   │   ├── doc.go
│   │   └── client_test.go
│   └── nineanime/         (NEW — Wave 3 Plan 28-05)
│       ├── client.go
│       ├── cache.go
│       ├── dto.go
│       ├── doc.go
│       ├── client_test.go
│       └── testdata/
│           ├── wp_search_frieren.json
│           ├── series_frieren_s2.html
│           ├── episode_1.html
│           └── embed_1anime_site.html
└── embeds/
    ├── vidstream_vip.go   (NEW — Wave 1 Plan 28-03, ~120 lines, plain regex extract)
    ├── vidstream_vip_test.go
    └── (optional) onenime_site.go (if NOT inlined into nineanime/client.go)
```

### Pattern 1: Provider Lift Template

**What:** Each new provider is a single Go package under `services/scraper/internal/providers/{name}/` implementing the 6-method `domain.Provider` interface. NO new shared lib — copy-with-adaptation per Phase 26 D1.

**When to use:** Every new provider, full stop. This pattern is locked.

**Example:**
```go
// services/scraper/internal/providers/animefever/client.go (skeleton)
package animefever

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "time"

    "github.com/PuerkitoBio/goquery"

    "github.com/ILITA-hub/animeenigma/libs/cache"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
    "github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

const providerName = "animefever"
var stageNames = health.AllStages

type Deps struct {
    BaseURL string // default "https://animefever.cc"
    HTTP    *domain.BaseHTTPClient
    Embeds  *domain.Registry
    Cache   cache.Cache
    Log     *logger.Logger
}

type Provider struct {
    baseURL string
    http    *domain.BaseHTTPClient
    embeds  *domain.Registry
    cache   *cacheLayer
    log     *logger.Logger

    stagesMu sync.Mutex
    stages   map[string]domain.StageHealth
}

func New(d Deps) (*Provider, error) {
    if d.HTTP == nil    { return nil, errors.New("animefever: Deps.HTTP is required") }
    if d.Embeds == nil  { return nil, errors.New("animefever: Deps.Embeds is required") }
    if d.Cache == nil   { return nil, errors.New("animefever: Deps.Cache is required") }
    if d.Log == nil     { d.Log = logger.Default() }
    base := d.BaseURL
    if base == "" { base = "https://animefever.cc" }
    p := &Provider{
        baseURL: strings.TrimRight(base, "/"),
        http:    d.HTTP,
        embeds:  d.Embeds,
        cache:   newCacheLayer(d.Cache),
        log:     d.Log,
        stages:  make(map[string]domain.StageHealth, len(stageNames)),
    }
    for _, s := range stageNames {
        p.stages[s] = domain.StageHealth{Up: true} // optimistic seed
    }
    return p, nil
}

func (p *Provider) Name() string { return providerName }
// FindID, ListEpisodes, ListServers, GetStream, HealthCheck, markStage methods follow allanime's shape

// Compile-time assertion
var _ domain.Provider = (*Provider)(nil)
```
`[CITED: services/scraper/internal/providers/allanime/client.go:50-110]` (verbatim template shape — every existing provider follows this).

### Pattern 2: Embed Extractor Template

**What:** Each embed host gets one `embeds/<host>.go` file implementing `domain.EmbedExtractor`.

**When to use:** When the upstream returns an iframe URL to a known third-party player host that needs HLS/MP4 URL extraction.

**Example (vidstream_vip.go skeleton):**
```go
// services/scraper/internal/embeds/vidstream_vip.go (skeleton)
package embeds

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "regexp"
    "strings"
    "time"

    "github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// vidstreamVipHosts allowlist for AnimeFever's primary embed.
var vidstreamVipHosts = []string{"am.vidstream.vip", "vidstream.vip"}
const vidstreamVipReferer = "https://animefever.cc/"

// vidstreamVipSourcesRegex captures the `sources: [{"file":"..."}]` JSON
// literal inline in the embed page HTML. Verified shape during research
// 2026-05-20: `sources: [{"file":"https://...m3u8","type":"mp4","label":"HD"}],`
var vidstreamVipSourcesRegex = regexp.MustCompile(`sources\s*:\s*\[\s*({[^}]+})`)

type VidstreamVipExtractor struct {
    http    *http.Client
    timeout time.Duration
}

func NewVidstreamVipExtractor() *VidstreamVipExtractor {
    return &VidstreamVipExtractor{
        http: &http.Client{Timeout: 15 * time.Second},
    }
}

func (e *VidstreamVipExtractor) Name() string { return "vidstream_vip" }

func (e *VidstreamVipExtractor) Matches(embedURL string) bool {
    u, err := url.Parse(embedURL)
    if err != nil || u.Host == "" { return false }
    host := strings.ToLower(u.Host)
    for _, h := range vidstreamVipHosts {
        if host == h || strings.HasSuffix(host, "."+h) { return true }
    }
    return false
}

func (e *VidstreamVipExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
    if err != nil { return nil, domain.WrapExtractFailed(err, "vidstream_vip: build request") }
    if headers != nil {
        for k, v := range headers { if len(v) > 0 { req.Header.Set(k, v[0]) } }
    }
    if req.Header.Get("Referer") == "" {
        req.Header.Set("Referer", vidstreamVipReferer)
    }
    resp, err := e.http.Do(req)
    if err != nil { return nil, domain.WrapProviderDown(err, "vidstream_vip: http") }
    defer resp.Body.Close()
    body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))  // 2 MiB DoS cap
    if err != nil { return nil, domain.WrapExtractFailed(err, "vidstream_vip: read body") }
    m := vidstreamVipSourcesRegex.FindSubmatch(body)
    if len(m) < 2 { return nil, domain.WrapExtractFailed(fmt.Errorf("no sources literal"), "vidstream_vip: regex") }
    var src struct{ File, Type, Label string }
    if err := json.Unmarshal(m[1], &src); err != nil {
        return nil, domain.WrapExtractFailed(err, "vidstream_vip: parse source json")
    }
    if !strings.HasPrefix(src.File, "http") {
        return nil, domain.WrapExtractFailed(fmt.Errorf("non-absolute URL %q", src.File), "vidstream_vip: url shape")
    }
    return &domain.Stream{
        Sources: []domain.Source{{URL: src.File, Type: "hls", Quality: src.Label}},
        Headers: map[string]string{"Referer": vidstreamVipReferer},
    }, nil
}

var _ domain.EmbedExtractor = (*VidstreamVipExtractor)(nil)
```
`[VERIFIED: live recon 2026-05-20]` — the regex shape matches the actual response body from `am.vidstream.vip` for Frieren ep28.

### Pattern 3: Provider Registration in main.go

**What:** New providers are registered in `cmd/scraper-api/main.go` between existing providers, in failover-chain order. The Phase 19 wiring invariant's `candidateProviders` slice MUST be updated to include the new provider name.

**When to use:** Every new provider lift's main.go change.

**Example (insertion shape):**
```go
// services/scraper/cmd/scraper-api/main.go (insertion after allanime registration)
// ============================================================
// Phase 28 (SCRAPER-HEAL-36) — AnimeFever as the FOURTH live EN provider.
// ============================================================
animeFeverBaseHTTP := domain.NewBaseHTTPClient(log,
    domain.WithPerHostRPS("animefever.cc", 1.0, 2),
    domain.WithPerHostRPS("am.vidstream.vip", 1.0, 2),
    domain.WithPerHostRPS("static-cdn-ca1.mofl.pro", 2.0, 4),
)
animeFeverProvider, err := animefever.New(animefever.Deps{
    BaseURL: cfg.AnimeFever.BaseURL,
    HTTP:    animeFeverBaseHTTP,
    Embeds:  registry,
    Cache:   redisCache,
    Log:     log,
})
if err != nil {
    log.Fatalw("failed to construct AnimeFever provider", "error", err)
}
if cfg.DegradedProviders.IsDegraded(animeFeverProvider.Name()) {
    log.Warnw("provider SKIPPED (degraded via SCRAPER_DEGRADED_PROVIDERS)",
        "name", animeFeverProvider.Name(),
        "reason", "global kill-switch")
} else {
    orchestrator.Register(animeFeverProvider)
    log.Infow("registered provider", "name", animeFeverProvider.Name())
}

// And update the Phase 19 wiring invariant's slice:
candidateProviders := []string{"gogoanime", "animepahe", "allanime", "animefever"}
// (Miruro/nineanime appended conditionally as their plans land)
```
`[VERIFIED: services/scraper/cmd/scraper-api/main.go:213-239, 367-396]` (allanime is the exact reference shape).

### Anti-Patterns to Avoid

- **Sharing HTTP client across providers** — each provider has its own `BaseHTTPClient` instance with provider-specific per-host RPS limits. Sharing a client means rate-limit contention.
- **Extracting a `provider-base` lib** — Phase 26 D1 explicitly chose copy-with-adaptation. Each provider is independent; shared lib boundary creates an upgrade ratchet.
- **Hand-rolling cookie jar / retry / rate-limit** — already in `BaseHTTPClient`. Use it.
- **Including Kodik iframe fallback paths** — `domain.Stream` has NO `iframe_url` field (enforced by `TestStream_HasNoIframeURL`). Don't add one. `[VERIFIED: services/scraper/internal/domain/provider.go:1-9]`.
- **Caching stream URLs longer than 1 hour** — per CLAUDE.md "Don't Do" + Phase 16 SCRAPER-NF: cache stream URLs ≤ min(expires-30s, 5min).
- **Adding `chromedp` / `utls` / `flaresolverr` as Go dependencies** — v3.1-REQUIREMENTS.md non-goals explicitly reject these. If a provider needs them → use sidecar pattern (Phase 27).
- **Pre-implementing embed extractors before SCRAPER-HEAL-35 recon** — D4 explicitly forbids speculative writes. Recon-driven only.
- **Putting 9anime's `<source>` MP4 path through HLS playback** — MP4 is a different Source.Type. Frontend already handles MP4 via AnimePahe-via-Kwik precedent. Per CLAUDE.md "Don't add complex abstractions".

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTTP retry / backoff / cookies / rate limit | Custom `http.Client` | `domain.NewBaseHTTPClient` | SCRAPER-FOUND-06 NF; cookies + DDoS-Guard support require careful jar config. |
| Title fuzzy matching | Levenshtein-from-scratch | `services/scraper/internal/fuzzy.JaroWinkler` (already exists; threshold 0.85) | Lifted from animepahe to shared lib; tested in fuzzy_test.go. |
| Cookie jar with public-suffix | `cookiejar.New(nil)` | `cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})` | Without public-suffix list, cookies bleed across eTLD+1 boundaries (security risk + correctness bug). Already wired in BaseHTTPClient. |
| Health stage names | Custom stage strings | `health.StageSearch`, `StageEpisodes`, etc. | Versioned contract; appears as Prometheus label values in dashboards/alerts. Rename = broken dashboards. |
| HLS proxy | Custom proxy code | `libs/videoutils.ProxyWithReferer` + add host to `HLSProxyAllowedDomains` | Phase 25 SCRAPER-HEAL-24 ensures "domain not allowed" returns 502; allowlist is fail-closed. |
| JS execution for packed embeds | `otto` / `v8go` / custom interpreter | `github.com/dop251/goja` (already in go.mod) | NOT needed for AnimeFever or 9anime; required for any future packed-JS embed. |
| Redis cache | Custom in-memory map | `libs/cache.Cache` | TTL semantics, key families, malsync 24h negative cache all standardized. |
| MAL → AniList ID mapping | Custom HTTP client to ids.moe | `libs/idmapping/` (ARM via `arm.haglund.dev`) | ids.moe is dead per project memory; ARM works without auth. |
| Per-host rate limiter | `time.Sleep(n)` loops | `domain.WithPerHostRPS(host, rps, burst)` Option | Per-host limiters are baked into BaseHTTPClient; one limiter per host. |
| Structured logging | `fmt.Printf` | `logger.Default().Infow/Errorw` | Project-wide standard; preserves log shape across services. |
| AES/HMAC obfuscation port | Custom byte operations | `crypto/aes`, `crypto/cipher`, `crypto/hmac`, `crypto/sha256` stdlib | CONTEXT.md D3 EXPLICITLY restricts Miruro's transform to these stdlib packages. |
| WordPress REST API client | New WP-specific lib | Plain `http.Get` + `json.Unmarshal` against `/wp-json/wp/v2/search?search=<q>&subtype=series` | WP REST API is just JSON; no client needed. Verified live: `[VERIFIED: 2026-05-20 probe returned Frieren S2 series result]`. |

**Key insight:** Phases 26 and 27 established that every solvable provider problem has an existing reusable piece. New providers compose existing pieces; they don't introduce new pieces unless absolutely necessary. The ONE new piece Phase 28 introduces is `embeds/vidstream_vip.go` (because AnimeFever's embed is a NEW host family).

## Runtime State Inventory

Phase 28 is greenfield (new packages, new files, no rename/refactor) — but it touches runtime state on the production server. The categories below DO apply:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — new providers add new cache keys (`animefever:*`, `miruro:*`, `nineanime:*`) under existing Redis. No existing data uses these keys. | None. |
| Live service config | Docker Compose env vars: new `SCRAPER_ANIMEFEVER_BASE_URL`, `SCRAPER_MIRURO_BASE_URL`, `SCRAPER_MIRURO_PROXY_A/B`, `SCRAPER_NINEANIME_BASE_URL`. The `SCRAPER_DEGRADED_PROVIDERS` env (in `docker/.env`) may need values appended if any new provider must ship dark. | New env vars added in `docker/docker-compose.yml`; defaults provided. `.env` only updated if operator wants to ship a provider degraded. |
| OS-registered state | None — service runs in Docker; no systemd / launchd / Task Scheduler registrations. | None. |
| Secrets/env vars | No new secrets. Miruro's `VITE_PROXY_OBF_KEY` is fetched from upstream's `env2.js` at runtime (or pinned in code per spike outcome). | None — keys are public, embedded in upstream's JS bundle. |
| Build artifacts / installed packages | `services/scraper/go.mod` does NOT add new external deps for AnimeFever or 9anime. May need a `go mod tidy` after Miruro's obfuscation port if any new internal sub-package is added. Frontend `bun install` / `bun run build` regenerates artifacts on Wave 3. | `make redeploy-scraper` (and `make redeploy-gateway` if main.go changes ripple) regenerates artifacts. |

**Nothing found in category "Stored data" or "OS-registered state":** Verified by grep `'animefever\|miruro\|nineanime\|9anime\.me\.uk\|am\.vidstream\|ultracloud\.cc\|1anime\.site'` across `services/`, `libs/`, `frontend/`, `docker/`, `infra/`, `deploy/` — no existing references. Phase 28 introduces all of these tokens fresh.

## Common Pitfalls

### Pitfall 1: AnimeFever Search Path

**What goes wrong:** Naive implementations try `https://animefever.cc/search?keyword=<q>` (the GET-with-query shape used by many WP sites).

**Why it happens:** That URL returns a 404-content-shape page (verified during research). The site's actual search uses `/search/<term>` (slash, not query) — extracted from the embedded JS `function searchAnime()` in the homepage HTML.

**How to avoid:** Use `https://animefever.cc/search/<url-escaped-term>` for search. Slugs returned: `/info/<slug>` (e.g. `/info/frieren-beyond-journeys-end.14401`). The `.<numeric>` suffix is OPTIONAL (some slugs lack it — `/info/frieren-beyond-journeys-end-season-2`).

**Warning signs:** Search returns Title="Animefever - Watch Anime English Subbed & Dubbed Online Free" and a 404 message body — that means you hit the wrong path.

`[VERIFIED: live recon 2026-05-20]`

### Pitfall 2: AnimeFever AJAX Endpoint Requires `ctk` Token

**What goes wrong:** Direct POST to `/ajax/anime/load_episodes_v2?s=tserver` without the page-scoped `ctk` token might 4xx (or silently return `status:false`).

**Why it happens:** The watch page HTML embeds `var ctk = '1f13010abb82454ebdc982c366dcaf17';` which the page's JS sends with the AJAX request. The token appears to be a CSRF-like anti-scrape mechanism (though not cryptographically tight).

**How to avoid:** Two-step fetch: (1) `GET /watch/<slug>?ep=<eid>` → scrape the `var ctk = '...'` token from the response HTML (regex or goquery on `<script>` content), (2) POST `/ajax/anime/load_episodes_v2?s=tserver` with `episode_id` + `ctk` form-encoded. The PHPSESSID cookie from earlier requests MUST propagate (cookie jar handles it).

**Warning signs:** `{"status":false}` response body, or HTTP 4xx — both indicate missing/invalid ctk or session.

`[VERIFIED: live recon 2026-05-20]`

### Pitfall 3: AnimeFever Has Two Servers (`tserver`, `hserver`), and Some Provide MP4 vs HLS

**What goes wrong:** Hard-coding `s=tserver` blocks the fallback path when tserver is broken or doesn't carry a particular episode.

**Why it happens:** AnimeFever's player UI offers a server dropdown; the backend serves different embed iframes depending on `s=` value. Both currently funnel to `am.vidstream.vip` but with different `lt=` parameter values (`lt=ts` for tserver, `lt=hs` for hserver).

**How to avoid:** ListServers returns BOTH servers (tserver as primary, hserver as fallback); GetStream tries them in order until one returns a Stream. Match the orchestrator's "try first, fall through" pattern.

**Warning signs:** Frieren E2E gate passes on ep 1 but fails on ep 25 — likely tserver doesn't carry that episode.

`[VERIFIED: live recon 2026-05-20]` — watch page select element offers `tserver` + `hserver` options.

### Pitfall 4: 9anime.me.uk's `?s=` Search Returns Garbage; Use WP REST API

**What goes wrong:** Implementing search via `https://9anime.me.uk/?s=frieren` returns 19 irrelevant "episode 7" stubs (verified in CONTEXT.md recon).

**Why it happens:** The brand-jacking WordPress instance has misconfigured the default theme search; it falls back to a generic post-title-LIKE query that matches unrelated episode stubs.

**How to avoid:** Use `GET /wp-json/wp/v2/search?search=<q>&per_page=20`. Filter results client-side by `subtype: "series"`. The full URL pattern: `/wp-json/wp/v2/search?search=frieren&per_page=20` → returns `[{id, title, url, type, subtype}, ...]`. Verified during research: returned Frieren S2 series record correctly (`subtype: "series"`).

**Warning signs:** Search returns posts whose `subtype` is "post" or "page" rather than "series" — those are episode stubs, not series.

`[VERIFIED: live probe 2026-05-20]`

### Pitfall 5: 9anime.me.uk Episode Slugs Are Irregular (HD Prefix Sometimes Present)

**What goes wrong:** Constructing episode URLs as `/<series-slug>-episode-<N>-english-subbed/` and finding 404s for some episodes.

**Why it happens:** Some episodes have an `hd-` prefix on their slug (`/hd-frieren-...-episode-1-english-subbed/` vs `/frieren-...-episode-2-english-subbed/`). The pattern isn't consistent — it's whatever the upload editor named the post.

**How to avoid:** Don't construct episode URLs. Instead, fetch the series page (`/series/<series-slug>/`) and parse the `<a class="ep-item" data-number="N" data-id="...">` elements directly. `data-number` is the episode number; `href` is the canonical episode URL.

**Warning signs:** ListEpisodes returns gaps (ep 1, 3, 5 but not 2, 4) — likely caused by URL-construction guessing.

`[VERIFIED: live recon 2026-05-20]`

### Pitfall 6: 9anime's `my.1anime.site` Player Returns MP4 (Not HLS)

**What goes wrong:** EnglishPlayer.vue (if it assumes HLS-first) tries to feed MP4 URLs to HLS.js → silent failure.

**Why it happens:** The 1anime.site embed serves `<video><source src="videos/<name>.mp4" type="video/mp4">`. There's no HLS path. CONTEXT.md `<risks>` flags this; AnimePahe-via-Kwik already proves the frontend supports MP4 sources.

**How to avoid:** Provider returns `Stream{Sources: [{URL: "...mp4", Type: "mp4"}], Headers: {Referer: "https://my.1anime.site/"}}`. Frontend's player MUST branch on `Source.Type === "mp4"` to use native `<video>` rather than HLS.js.

**Warning signs:** Network requests to `my.1anime.site/videos/*.mp4` succeed but the `<video>` element never plays.

`[VERIFIED: live probe 2026-05-20]` — `Content-Type: video/mp4`, 211 MB, `Accept-Ranges: bytes`.

### Pitfall 7: Miruro's Obfuscation Keys MIGHT Rotate Per-Session

**What goes wrong:** Pinning `VITE_PROXY_OBF_KEY=a54d389c18527d9fd3e7f0643e27edbe` in code, then the upstream rotates the key.

**Why it happens:** The keys live in `https://www.miruro.tv/env2.js` (a public static JS file). Upstream COULD invalidate the keys with a single deploy.

**How to avoid:** CONTEXT.md D3 gate 3 EXPLICITLY requires verification across 3 sequential fetches BEFORE shipping. If they rotate per-session → spike `killed`. Otherwise: fetch keys at startup (cache 24h) rather than pin.

**Warning signs:** First request to `pro.ultracloud.cc/<obf-url>` works; subsequent requests start returning 404.

`[VERIFIED: 2026-05-19 → 2026-05-20 — keys unchanged across both probes]`

### Pitfall 8: Miruro's Cloudflare Proxy MIGHT TLS-Fingerprint Scrape Clients

**What goes wrong:** Pure-Go HTTP requests get 403/404 from `pro.ultracloud.cc` even with valid obfuscated URLs.

**Why it happens:** Cloudflare's `cf-mitigated: challenge` header can fire on TLS fingerprint mismatch. Go's `net/http` has a distinctive TLS ClientHello shape that differs from Chrome/Firefox.

**How to avoid:** CONTEXT.md D3 gate 2 EXPLICITLY requires a HTTP 200 response from the production server. If 403/HTML-challenge → spike `killed` (can't ship with `utls` dep). The mitigation IS the kill-switch.

**Warning signs:** Empty 404 body, OR a Cloudflare HTML challenge page.

`[CITED: docs.cloudflare.com TLS fingerprinting]` + `[VERIFIED: pro.ultracloud.cc/ root returns 404 from production IP during 2026-05-20 probe — the proxy IS reachable, so transport-level fingerprinting is unlikely; the 404 is endpoint-not-found, not challenge]`.

### Pitfall 9: Failover Chain Length and Per-Provider Latency Budget

**What goes wrong:** Adding 3 providers to the failover chain blows the 8s per-request SLO.

**Why it happens:** Orchestrator probes providers in order; each provider's first-server probe takes up to 1s. With 6 providers (gogoanime/animepahe/allanime/animefever/miruro/nineanime), worst-case = 6 × 1s = 6s + actual stream fetch = > 8s.

**How to avoid:** Per CONTEXT.md `<risks>`: Phase 21's hard ≤8s budget is per-server, not per-provider. Each provider's first-server probe is bounded; chain length stays under budget for ≤8 providers. Verify with Phase 23 daily canary timing data.

**Warning signs:** Phase 22 latency dashboards show p99 > 8s after Phase 28 ships.

`[CITED: .planning/phases/21-/21-RESEARCH.md latency budget — referenced in CONTEXT.md]`

### Pitfall 10: HLS Proxy Allowlist Is Fail-Closed — Forgetting an Entry Returns 502

**What goes wrong:** Provider's GetStream returns a working URL on a new host; frontend tries to proxy; streaming service returns 502 "domain not allowed".

**Why it happens:** Phase 25 SCRAPER-HEAL-24 hardened the proxy: any host not in `HLSProxyAllowedDomains` returns 502. This is intentional (SSRF defense) but trips on every new provider's first deploy.

**How to avoid:** D7 mandates per-provider allowlist updates IN THE SAME COMMIT as the provider lift. Reviewer checklist: every new provider PR touches `libs/videoutils/proxy.go` `HLSProxyAllowedDomains`.

**Warning signs:** Frieren E2E gate passes the scraper layer (episodes/stream URL returned) but fails at the streaming layer (502).

`[VERIFIED: libs/videoutils/proxy.go:230-290 + Phase 25 SCRAPER-HEAL-24 SUMMARY]`

## Code Examples

Verified patterns from official sources / live recon:

### AnimeFever search + episode list

```go
// services/scraper/internal/providers/animefever/client.go (FindID excerpt)
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
    // Cache key uses MAL ID when available; title as weaker fallback.
    cacheKey := ref.ShikimoriID
    if cacheKey == "" { cacheKey = ref.Title }
    if cacheKey != "" {
        if hit, ok := p.cache.getShowID(ctx, cacheKey); ok {
            p.markStage(health.StageSearch, nil)
            return hit, nil
        }
    }
    query := strings.TrimSpace(ref.Title)
    if query == "" { /* WrapNotFound */ }

    // AnimeFever's search path is /search/<term>, NOT /search?keyword=<term>.
    // Verified by inspecting embedded JS function searchAnime() in homepage HTML.
    searchURL := fmt.Sprintf("%s/search/%s", p.baseURL, url.PathEscape(query))
    resp, err := p.http.Get(ctx, searchURL)
    if err != nil { return "", domain.WrapProviderDown(err, "animefever: search http") }
    defer resp.Body.Close()
    doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 2<<20))
    if err != nil { return "", domain.WrapExtractFailed(err, "animefever: parse search html") }

    // Cards: <div class="card-block ..." title="...">
    //          <h3><a href="https://animefever.cc/info/<slug>"> ... </a></h3>
    //        </div>
    var bestSlug, bestTitle string
    var bestScore float64
    doc.Find("div.card-block").Each(func(_ int, card *goquery.Selection) {
        href, ok := card.Find("h3 a").Attr("href")
        if !ok { return }
        title := strings.TrimSpace(card.AttrOr("title", ""))
        slug := strings.TrimPrefix(href, p.baseURL+"/info/")
        slug = strings.TrimPrefix(slug, "https://animefever.cc/info/")
        score := fuzzy.JaroWinkler(fuzzy.NormalizeTitle(title), fuzzy.NormalizeTitle(query))
        if score > bestScore { bestScore = score; bestSlug = slug; bestTitle = title }
    })
    if bestScore < 0.85 {
        return "", domain.WrapNotFound(fmt.Errorf("no card ≥ 0.85 for %q", query), "animefever: FindID fuzzy")
    }
    if cacheKey != "" { p.cache.setShowID(ctx, cacheKey, bestSlug) }
    p.markStage(health.StageSearch, nil)
    return bestSlug, nil
}
```
`[VERIFIED: live recon 2026-05-20 search/frieren response shape + JaroWinkler usage from animepahe]`

### AnimeFever AJAX iframe extraction

```go
// services/scraper/internal/providers/animefever/client.go (ListServers excerpt)
var ctkRegex = regexp.MustCompile(`var\s+ctk\s*=\s*'([0-9a-fA-F]{32,64})'`)

func (p *Provider) fetchWatchTokens(ctx context.Context, slug, epID string) (ctk string, err error) {
    watchURL := fmt.Sprintf("%s/watch/%s?ep=%s", p.baseURL, slug, epID)
    resp, err := p.http.Get(ctx, watchURL)
    if err != nil { return "", domain.WrapProviderDown(err, "animefever: watch http") }
    defer resp.Body.Close()
    body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
    if err != nil { return "", domain.WrapExtractFailed(err, "animefever: read watch body") }
    m := ctkRegex.FindSubmatch(body)
    if len(m) < 2 { return "", domain.WrapExtractFailed(errors.New("no ctk var in watch page"), "animefever: ctk") }
    return string(m[1]), nil
}

func (p *Provider) fetchEmbedURL(ctx context.Context, slug, epID, server, ctk string) (string, error) {
    ajaxURL := fmt.Sprintf("%s/ajax/anime/load_episodes_v2?s=%s", p.baseURL, server)
    form := url.Values{}
    form.Set("episode_id", epID)
    form.Set("ctk", ctk)
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, ajaxURL, strings.NewReader(form.Encode()))
    if err != nil { return "", domain.WrapProviderDown(err, "animefever: build ajax req") }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Set("X-Requested-With", "XMLHttpRequest")
    req.Header.Set("Referer", fmt.Sprintf("%s/watch/%s?ep=%s", p.baseURL, slug, epID))
    resp, err := p.http.Do(ctx, req)
    if err != nil { return "", domain.WrapProviderDown(err, "animefever: ajax http") }
    defer resp.Body.Close()

    var out struct {
        Status bool   `json:"status"`
        Value  string `json:"value"`  // contains the <iframe src="..."> HTML
        Embed  bool   `json:"embed"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return "", domain.WrapExtractFailed(err, "animefever: parse ajax json")
    }
    if !out.Status || !out.Embed {
        return "", domain.WrapExtractFailed(fmt.Errorf("status=%v embed=%v", out.Status, out.Embed), "animefever: ajax response")
    }
    // Extract the iframe URL from out.Value HTML.
    src, err := extractIframeSrc(out.Value)
    if err != nil { return "", err }
    return src, nil
}
```
`[VERIFIED: live recon 2026-05-20 — POST /ajax/anime/load_episodes_v2?s=tserver returns {"status":true,"value":"<iframe src='https://am.vidstream.vip?...'>","embed":true,...}]`

### 9anime WP REST search + episode list parse

```go
// services/scraper/internal/providers/nineanime/client.go (FindID + ListEpisodes excerpt)

type wpSearchResult struct {
    ID      int    `json:"id"`
    Title   string `json:"title"`
    URL     string `json:"url"`
    Type    string `json:"type"`
    Subtype string `json:"subtype"`
}

func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
    // Use WP REST API; the default /?s= search returns garbage.
    searchURL := fmt.Sprintf("%s/wp-json/wp/v2/search?search=%s&per_page=20",
        p.baseURL, url.QueryEscape(ref.Title))
    resp, err := p.http.Get(ctx, searchURL)
    if err != nil { return "", domain.WrapProviderDown(err, "nineanime: wp search") }
    defer resp.Body.Close()
    var results []wpSearchResult
    if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
        return "", domain.WrapExtractFailed(err, "nineanime: parse wp search")
    }
    // Filter: subtype == "series". Score each with JaroWinkler.
    var best wpSearchResult
    var bestScore float64
    for _, r := range results {
        if r.Subtype != "series" { continue }
        score := fuzzy.JaroWinkler(
            fuzzy.NormalizeTitle(r.Title),
            fuzzy.NormalizeTitle(ref.Title),
        )
        if score > bestScore { bestScore = score; best = r }
    }
    if bestScore < 0.85 {
        return "", domain.WrapNotFound(
            fmt.Errorf("no series ≥ 0.85 for %q", ref.Title), "nineanime: FindID")
    }
    // ProviderID = the slug from the URL (e.g. "frieren-beyond-journeys-end-season-2")
    slug := strings.TrimSuffix(strings.TrimPrefix(best.URL, p.baseURL+"/series/"), "/")
    return slug, nil
}

func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
    seriesURL := fmt.Sprintf("%s/series/%s/", p.baseURL, providerID)
    resp, err := p.http.Get(ctx, seriesURL)
    if err != nil { return nil, domain.WrapProviderDown(err, "nineanime: series http") }
    defer resp.Body.Close()
    doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 4<<20))
    if err != nil { return nil, domain.WrapExtractFailed(err, "nineanime: parse series html") }

    var eps []domain.Episode
    doc.Find("a.ep-item").Each(func(_ int, a *goquery.Selection) {
        n, _ := strconv.Atoi(a.AttrOr("data-number", "0"))
        if n <= 0 { return }
        href, _ := a.Attr("href")
        eps = append(eps, domain.Episode{
            ID:     href, // store the full canonical URL — slugs are irregular
            Number: n,
            Title:  fmt.Sprintf("Episode %d", n),
        })
    })
    sort.SliceStable(eps, func(i, j int) bool { return eps[i].Number < eps[j].Number })
    return eps, nil
}
```
`[VERIFIED: live recon 2026-05-20 — /wp-json/wp/v2/search?search=frieren returned `{"id":9314,"title":"...","url":".../series/frieren-beyond-journeys-end-season-2/","subtype":"series"}` and /series/<slug>/ HTML has `<a class="ep-item" data-number="N">`]`

### 9anime MP4 extraction

```go
// services/scraper/internal/providers/nineanime/client.go (GetStream excerpt)
var iframeSrcRegex = regexp.MustCompile(`<iframe[^>]+src="(https://my\.1anime\.site/[^"]+)"`)
var videoSrcRegex = regexp.MustCompile(`<source[^>]+src="(videos/[^"]+\.mp4)"`)

func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, cat domain.Category) (*domain.Stream, error) {
    // episodeID is the canonical episode URL stored by ListEpisodes.
    resp, err := p.http.Get(ctx, episodeID)
    if err != nil { return nil, domain.WrapProviderDown(err, "nineanime: episode http") }
    defer resp.Body.Close()
    body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
    m := iframeSrcRegex.FindSubmatch(body)
    if len(m) < 2 {
        return nil, domain.WrapExtractFailed(errors.New("no iframe src"), "nineanime: iframe regex")
    }
    iframeURL := string(m[1])

    // Fetch the iframe (my.1anime.site/index.php?action=play&file=<name>.mp4).
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, iframeURL, nil)
    req.Header.Set("Referer", p.baseURL+"/")
    embedResp, err := p.http.Do(ctx, req)
    if err != nil { return nil, domain.WrapProviderDown(err, "nineanime: embed http") }
    defer embedResp.Body.Close()
    embedBody, _ := io.ReadAll(io.LimitReader(embedResp.Body, 1<<20))
    vm := videoSrcRegex.FindSubmatch(embedBody)
    if len(vm) < 2 {
        return nil, domain.WrapExtractFailed(errors.New("no video source"), "nineanime: video regex")
    }
    // The src is relative ("videos/<name>.mp4"); make absolute.
    parsedIframe, _ := url.Parse(iframeURL)
    absURL := fmt.Sprintf("%s://%s/%s", parsedIframe.Scheme, parsedIframe.Host, string(vm[1]))

    return &domain.Stream{
        Sources: []domain.Source{{URL: absURL, Type: "mp4", Quality: "auto"}},
        Headers: map[string]string{"Referer": "https://my.1anime.site/"},
    }, nil
}
```
`[VERIFIED: live recon 2026-05-20 — iframe → embed → <source src="videos/...mp4"> → Content-Type: video/mp4 (211 MB, Accept-Ranges: bytes)]`

### Frontend source dropdown polish (Vue 3 template excerpt)

```vue
<!-- frontend/web/src/components/player/EnglishPlayer.vue (Wave 3 housekeeping diff) -->
<template>
  <select v-model="selectedProvider" @change="onProviderChange" class="...">
    <option v-for="p in availableProviders" :key="p" :value="p">
      {{ capitalizeProvider(p) }}
    </option>
  </select>
</template>

<script setup lang="ts">
function capitalizeProvider(name: string): string {
  switch (name) {
    case 'allanime':   return 'AllAnime'
    case 'animepahe':  return 'AnimePahe'
    case 'gogoanime':  return 'Anitaku'
    case 'animekai':   return 'AnimeKai'
    case 'animefever': return 'AnimeFever'   // NEW Phase 28
    case 'miruro':     return 'Miruro'       // NEW Phase 28 (conditional)
    case 'nineanime':  return '9anime'       // NEW Phase 28
    default:           return name.charAt(0).toUpperCase() + name.slice(1)
  }
}
// Source order = failover chain order (CONTEXT.md open_questions answer).
const availableProviders = computed(() => {
  const order = ['allanime', 'animefever', 'miruro', 'nineanime', 'animepahe', 'gogoanime', 'animekai']
  return order.filter(name => healthSnapshot.value?.[name]?.stages?.search?.up)
})
</script>
```

### i18n keys (locales)

```json
// frontend/web/src/locales/en.json (additions under anime.player or similar)
{
  "anime": {
    "scraper": {
      "providers": {
        "allanime": "AllAnime",
        "animefever": "AnimeFever",
        "miruro": "Miruro",
        "nineanime": "9anime"
      },
      "sourceLabel": "Source",
      "sourceUnavailable": "This source isn't responding right now. Try another."
    }
  }
}
```
```json
// frontend/web/src/locales/ru.json
{
  "anime": {
    "scraper": {
      "providers": {
        "allanime": "AllAnime",
        "animefever": "AnimeFever",
        "miruro": "Miruro",
        "nineanime": "9anime"
      },
      "sourceLabel": "Источник",
      "sourceUnavailable": "Этот источник сейчас не отвечает. Попробуйте другой."
    }
  }
}
```
```json
// frontend/web/src/locales/ja.json
{
  "anime": {
    "scraper": {
      "providers": {
        "allanime": "AllAnime",
        "animefever": "AnimeFever",
        "miruro": "Miruro",
        "nineanime": "9anime"
      },
      "sourceLabel": "ソース",
      "sourceUnavailable": "このソースは現在応答していません。別のソースを試してください。"
    }
  }
}
```

### Playwright e2e (source-switching mid-episode)

```typescript
// frontend/web/e2e/english-player-sources.spec.ts (sketch — extends raw-player.spec.ts pattern)
import { test, expect } from '@playwright/test'

const FRIEREN_SHIKIMORI_ID = '52991'
const UI_AUDIT_USERNAME = 'ui_audit_bot'
const UI_AUDIT_PASSWORD = 'audit_bot_test_password_2026'

test('source switching mid-episode preserves playback', async ({ page }) => {
  // Login (lifted from raw-player.spec.ts:25-48)
  await page.goto('/')
  await page.evaluate(async (creds) => {
    const r = await fetch('/api/auth/login', { method: 'POST', credentials: 'include',
      headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(creds) })
    const d = (await r.json()).data
    localStorage.setItem('token', d.access_token)
    localStorage.setItem('user', JSON.stringify(d.user))
  }, { username: UI_AUDIT_USERNAME, password: UI_AUDIT_PASSWORD })

  // Navigate to Frieren anime page
  await page.goto(`/anime/shikimori/${FRIEREN_SHIKIMORI_ID}`)
  await page.click('button[data-language="en"]')

  // Start with allanime
  await page.selectOption('select[data-test="source-dropdown"]', 'allanime')
  await expect(page.locator('video')).toBeVisible({ timeout: 10000 })

  // Switch to animefever mid-episode
  await page.selectOption('select[data-test="source-dropdown"]', 'animefever')
  await expect(page.locator('video')).toBeVisible({ timeout: 15000 })

  // Switch to nineanime — MP4 path
  await page.selectOption('select[data-test="source-dropdown"]', 'nineanime')
  await expect(page.locator('video[src*=".mp4"]')).toBeVisible({ timeout: 15000 })
})
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| HLS-only player assumption | Player supports MP4 + HLS sources via `Source.Type` | Phase 16 (AnimePahe-via-Kwik) | 9anime ships MP4-direct without frontend changes. |
| Per-provider HTTP client | Shared `BaseHTTPClient` with per-provider Options | Phase 15 SCRAPER-FOUND-06 | All Phase 28 providers reuse the same retry/jar/rate-limit stack. |
| 5-stage health hand-rolled per provider | `health.AllStages` + `markStage()` + `ProbeRunner` | Phase 17 | New providers wire into observability automatically. |
| `iframe_url` field on Stream DTO | NO iframe field — separate DTO if ever needed | Phase 15 (ISS-008 lesson) | Removes silent Kodik-fallback footgun for all future providers. |
| HLS proxy allowlist via env var | Compiled-in `HLSProxyAllowedDomains` slice with fail-closed semantics | Phase 25 SCRAPER-HEAL-24 | Every new provider PR must touch this slice (D7 in Phase 28 CONTEXT.md). |
| Fuzzy matching duplicated per provider | Shared `fuzzy.JaroWinkler` + `fuzzy.NormalizeTitle` | Phase 27 (or earlier) | Phase 28's 9anime ships fuzzy-match without new lib code. |

**Deprecated/outdated:**
- AnimeFever's deprecated `/search?keyword=` query path — replaced by `/search/<term>` slug path (no upstream announcement; verified by inspecting embedded JS).
- 9anime's `?s=` default WP theme search — broken on the brand-jack instance; use `/wp-json/wp/v2/search` instead.
- Miruro's `/api/anime/list` and `/api/anime/search` endpoints — return `{"error":"Gone"}`. Use obfuscation-gated `pro.ultracloud.cc` proxy.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `https://animefever.cc/` | AnimeFever provider | ✓ (HTTP 200, Cloudflare passive, no challenge) | n/a (PHP backend, hosted on `static.anmedm.com` for assets) | `SCRAPER_DEGRADED_PROVIDERS=animefever` |
| `https://am.vidstream.vip/` | AnimeFever embed | ✓ (HTTP 200 with valid signed URL) | n/a (JWPlayer iframe) | Other AnimeFever server (`hserver`) |
| `https://static-cdn-ca1.mofl.pro/masters/*/master.m3u8` | AnimeFever HLS CDN | ✓ (live URLs serve HLS) | n/a (Mofl CDN) | None — provider degrades if CDN dies |
| `https://www.miruro.tv/` + `https://www.miruro.tv/env2.js` | Miruro provider | ✓ (HTTP 200, env2.js publishes OBF keys) | n/a (Vite SPA) | `SCRAPER_DEGRADED_PROVIDERS=miruro` |
| `https://pro.ultracloud.cc/` | Miruro proxy | ✓ root returns 404 (endpoint-not-found, NOT challenge — TLS works from prod IP) | n/a (Cloudflare-fronted) | `pru.ultracloud.cc` (alt proxy in env2.js) |
| `https://9anime.me.uk/` + WP REST API | 9anime provider | ✓ (HTTP 200, WP REST returns Frieren S2 result) | WordPress 6.9.4 + dramastream theme + Yoast SEO 27.6 | `SCRAPER_DEGRADED_PROVIDERS=nineanime` |
| `https://my.1anime.site/` | 9anime embed + MP4 host | ✓ (211 MB MP4 with Accept-Ranges: bytes for Frieren S2 ep1) | n/a (Cloudflare-fronted, Engintron caching) | None — if my.1anime.site dies, nineanime degrades |
| Go stdlib `crypto/aes`, `crypto/hmac`, `crypto/sha256`, `encoding/base64` | Miruro obfuscation port | ✓ (Go 1.23+) | go.mod toolchain | None needed |
| `github.com/PuerkitoBio/goquery` | AnimeFever + 9anime HTML scrape | ✓ | v1.10.3 | None — already in go.mod |
| `github.com/dop251/goja` | Reserved (if any provider needs JS execution; NOT needed for Phase 28) | ✓ | v0.0.0-20260311135729-065cd970411c | Reuse existing |
| `services/scraper/internal/fuzzy` | 9anime title-fuzzy + AnimeFever search fallback | ✓ | n/a (project lib) | None |
| ARM (`arm.haglund.dev/api/v2/ids`) | Miruro MAL→AniList resolution | ✓ | n/a (no-auth public API per memory) | Cached locally per Phase 16 idmapping |
| MalSync (`api.malsync.moe`) | AnimeFever title fallback (optional) | ✓ | n/a | Title-only fallback (already-implemented in animepahe/malsync.go pattern) |
| `make redeploy-scraper` | Phase 28 redeploy | ✓ | Makefile target exists | Direct `docker compose -f docker/docker-compose.yml up -d --build scraper` |
| `bun` for frontend builds | Wave 3 frontend housekeeping | ✓ (per CLAUDE.md) | n/a | None — `bun` is the project standard |
| `bunx playwright test` | Wave 3 e2e | ✓ (per CLAUDE.md) | n/a | None |

**Missing dependencies with no fallback:** None — every Phase 28 dependency is available.

**Missing dependencies with fallback:** None.

**Implicit dependencies (project state, not external):**
- `services/scraper/internal/providers/allanime/` MUST exist (template) — VERIFIED (shipped Phase 26-01).
- Phase 27's `services/scraper/internal/providers/animepahe/` rewrite MUST be merged before Phase 28's main.go changes — VERIFIED (Phase 27 plans 27-01 through 27-05 are all SUMMARY-complete per the directory listing).
- Phase 25 SCRAPER-HEAL-24 (HLS proxy 502 mapping) MUST be live — VERIFIED (commented in `proxy.go:308-317`).

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Go framework | Standard `testing` package + `httptest` + table-driven tests with goldens |
| Go config file | None — `go test` reads `*_test.go` files automatically |
| Quick run command | `go test ./services/scraper/internal/providers/animefever/... -race -count=2` |
| Full suite command | `go test ./services/scraper/... -race -count=2` |
| Frontend framework | Playwright (e2e) + Vitest (unit, if applicable) |
| Frontend e2e command | `cd frontend/web && bunx playwright test english-player-sources` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SCRAPER-HEAL-34 | Miruro obfuscation transform produces valid proxy URL | unit | `go test ./services/scraper/internal/providers/miruro/... -run TestObfuscation -race` | Wave 0 (28-00 spike) |
| SCRAPER-HEAL-35 | AnimeFever recon classifies embed hosts | manual gate | Read `.planning/phases/28-provider-expansion-r2/SPIKE-ANIMEFEVER.md` | Wave 0 (28-01 spike) |
| SCRAPER-HEAL-36 | AnimeFever provider implements all 6 domain.Provider methods | unit | `go test ./services/scraper/internal/providers/animefever/... -race -count=2 -run .` | Wave 1 (28-02) |
| SCRAPER-HEAL-36 | Compile-time interface assertion | compile-check | `go build ./services/scraper/...` (fails if `var _ domain.Provider = (*Provider)(nil)` breaks) | Wave 1 (28-02) |
| SCRAPER-HEAL-36 | Frieren E2E gate via gateway | integration | `curl 'http://localhost:8000/api/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623/scraper/episodes?provider=animefever' \| jq '. \| length'` returns ≥ 28 | Wave 1 (28-02) — manual gate post-redeploy |
| SCRAPER-HEAL-37 | Miruro provider works against Frieren AniList 154587 | integration | `curl 'http://localhost:8000/api/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623/scraper/episodes?provider=miruro' \| jq '. \| length'` returns ≥ 28 | Wave 2 (28-04, CONDITIONAL) |
| SCRAPER-HEAL-38 | New embed extractor (vidstream_vip) returns Stream with HLS Source | unit | `go test ./services/scraper/internal/embeds/... -race -run VidstreamVip` | Wave 1 (28-03) |
| SCRAPER-HEAL-39 | 9anime title-fuzzy returns Frieren S2 | unit | `go test ./services/scraper/internal/providers/nineanime/... -race -run FindID` | Wave 3 (28-05) |
| SCRAPER-HEAL-39 | 9anime returns MP4 Source for Marriagetoxin ep1/7 | integration | `curl 'http://localhost:8000/api/anime/<marriagetoxin-uuid>/scraper/stream?provider=nineanime&episode=1' \| jq '.data.sources[0].type'` returns `"mp4"` | Wave 3 (28-05) — manual gate post-redeploy |
| SCRAPER-HEAL-39 | Source dropdown displays all 4 providers with capitalized labels | e2e | `cd frontend/web && bunx playwright test english-player-sources --reporter=list` | Wave 3 (28-06) |
| Phase-level | `/scraper/health` shows `up:true` across all 5 stages for each shipped provider within 5 minutes of redeploy | integration | `curl http://localhost:8088/scraper/health \| jq '.providers["animefever"].stages \| to_entries[] \| .value.up'` all `true` | Wave 3 phase close |
| Phase-level | Phase 19 wiring invariant holds: `len(orchestrator.RegisteredProviders())` matches `expectedProviders` count | startup invariant | scraper container startup logs (Fatalw triggers on mismatch) | Every wave that touches main.go |

### Sampling Rate
- **Per task commit:** `go test ./services/scraper/internal/providers/<name>/... -race -count=2` (the provider being touched)
- **Per wave merge:** `go test ./services/scraper/... -race` (full scraper suite)
- **Phase gate:** Full scraper suite green + Frieren E2E gate per provider + `/scraper/health` snapshot inspection

### Wave 0 Gaps
- [ ] `.planning/phases/28-provider-expansion-r2/SPIKE-MIRURO.md` — verdict file (created in 28-00, NOT a test file)
- [ ] `.planning/phases/28-provider-expansion-r2/SPIKE-ANIMEFEVER.md` — recon classification (created in 28-01, NOT a test file)
- [ ] `services/scraper/internal/providers/animefever/client_test.go` — unit tests for AnimeFever (Wave 1)
- [ ] `services/scraper/internal/providers/animefever/testdata/*` — captured goldens (Wave 1)
- [ ] `services/scraper/internal/embeds/vidstream_vip_test.go` — extractor unit tests (Wave 1)
- [ ] `services/scraper/internal/providers/miruro/client_test.go` + obfuscation_test.go (Wave 2, conditional)
- [ ] `services/scraper/internal/providers/nineanime/client_test.go` + testdata (Wave 3)
- [ ] `frontend/web/e2e/english-player-sources.spec.ts` — Playwright e2e (Wave 3)

Framework installs needed: none — Go testing is stdlib; Playwright is already installed per existing `e2e/raw-player.spec.ts`.

## Security Domain

`security_enforcement` is treated as enabled (config.json doesn't disable it).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Providers run server-side; no end-user auth flows added. |
| V3 Session Management | no | No new session boundaries. |
| V4 Access Control | yes | `SCRAPER_DEGRADED_PROVIDERS` env-based kill-switch is the operator's access control over which providers are live. |
| V5 Input Validation | yes | Search queries from `AnimeRef.Title` MUST be url-escaped (`url.PathEscape` for AnimeFever's `/search/<term>` path; `url.QueryEscape` for 9anime's WP REST `?search=`). Episode IDs from upstream MUST be validated before use as URL path segments (no `..` traversal). |
| V6 Cryptography | yes | Miruro's obfuscation port MUST use stdlib `crypto/aes`, `crypto/hmac`, `crypto/sha256`, `encoding/base64` only (CONTEXT.md D3 gate 1). Never hand-roll the constructions; reuse stdlib primitives. |
| V8 Data Protection | yes | Provider response bodies capped at 4 MiB (BaseHTTPClient default) or 2 MiB (embed extractors) to prevent DoS via streaming GB-scale responses. Already enforced via `io.LimitReader`. |
| V10 Malicious Code | yes | NO new external Go deps for AnimeFever or 9anime; v3.1-REQUIREMENTS.md non-goals forbid `chromedp`, `utls`, `tls-client`, `flaresolverr`, `cloudscraper_go`. CI must reject these. |
| V12 Files & Resources | yes | HLS proxy allowlist is fail-closed (Phase 25 SCRAPER-HEAL-24); new hosts (`am.vidstream.vip`, `static-cdn-ca1.mofl.pro`, `pro.ultracloud.cc`, `pru.ultracloud.cc`, `my.1anime.site`) MUST be added to `HLSProxyAllowedDomains` for playback to work — SSRF defense by allowlist. |
| V13 API & Web Service | yes | All upstream calls go through `BaseHTTPClient` (per-host rate limit, retry, cookie jar, baseline UA). 5xx → `ErrProviderDown`. 4xx → `ErrExtractFailed`. |

### Known Threat Patterns for Phase 28 stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SSRF via attacker-controlled upstream URL injection | Information Disclosure / Tampering | HLS proxy fail-closed allowlist (`libs/videoutils.HLSProxyAllowedDomains`) — Phase 25 SCRAPER-HEAL-24 returns 502 for any non-allowlisted host. |
| Title-search query injection (path traversal via `..` in search term) | Tampering | `url.PathEscape` / `url.QueryEscape` on search inputs. Reject episode IDs containing `..`. |
| Hostile upstream payload (GB-scale response body OOMs scraper) | Denial of Service | `io.LimitReader` caps at 4 MiB (BaseHTTPClient) or 2 MiB (embed extractors). |
| Rate-limit abuse against upstream → IP ban | Denial of Service (us) | `domain.WithPerHostRPS(host, rps, burst)` — 1 RPS / burst 2 per upstream host (animefever.cc, am.vidstream.vip, my.1anime.site, etc.). |
| TLS fingerprint scrape from `pro.ultracloud.cc` rejecting Go requests | Spoofing (CF) | Miruro D3 gate 2 explicitly kills spike if 403/challenge from prod IP. NOT shipping with `utls`. |
| Cookie bleed across eTLD+1 boundaries | Information Disclosure | `cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})` — already configured in BaseHTTPClient. |
| Hostile JS payload pinning goroutine (packed-embed extractor) | Denial of Service | NOT a Phase 28 issue (no packed extractors added). `defaultPackedGojaTimeout = 5s` already in place for existing packed extractors. |
| Stale cache poisoning (provider's malsync entry maps to wrong slug) | Tampering | Negative cache TTL 24h matches positive — animepahe's MalSync invalidation pattern (Phase 27) carries forward. |
| Obfuscation key leak (publishing `VITE_PROXY_OBF_KEY` in public Git) | Information Disclosure | No leak — the key is ALREADY public (embedded in upstream's `env2.js` distributed to every browser). Treating it as public is correct. |
| Upstream brand-jack continuing to serve stale/wrong data (9anime episode-7 page embeds episode-6 MP4 — recon-observed CONTEXT.md `<decisions>` D2) | Repudiation / Information Disclosure | Documented as accepted trade-off in CONTEXT.md D2. Operator's responsibility, not a code defense. |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | AnimeFever's embed host is reasonably stable at `am.vidstream.vip` (won't rotate per request) | "AnimeFever" / Pattern 2 | If it rotates per request, the extractor `Matches()` host allowlist must broaden; ~30 min fix. |
| A2 | AnimeFever's `tserver` carries every Frieren episode (1-28) | "Pitfall 3" | If it doesn't, ListServers must surface `hserver` as fallback in the same call; CONTEXT.md D6 Frieren E2E gate would catch. |
| A3 | 9anime.me.uk's WP REST API will continue returning `subtype: "series"` for anime entries | "9anime.me.uk" / Pattern code | If the upstream renames `series` to a custom post type that isn't exposed via REST, FindID degrades to operator-curated slug map per CONTEXT.md domain `<out_of_scope>`. |
| A4 | Miruro's obfuscation transform uses HMAC-SHA256 or AES-CTR (the env constants are 32-hex-char shape consistent with 128-bit key material) | "Miruro" | If actually `Salsa20`/`ChaCha`/custom xor-mixer → still doable in pure Go stdlib; spike work absorbs. |
| A5 | Miruro's `pro.ultracloud.cc` does NOT TLS-fingerprint requests from a Cloudflare-allowed IP | "Pitfall 8" | If it does → spike `killed` per D3 gate 2. |
| A6 | Phase 24 EnglishPlayer.vue may not ship by Phase 28's Wave 3 — the 28-06 dropdown polish stays pending until Phase 24 lands | "Phase Dependencies" (in CONTEXT.md) | Backend ships independently; frontend polish becomes a follow-up. CONTEXT.md `<dependencies>` already documents this as soft. |
| A7 | The 4-agent-session kill-switch is enough time for an experienced agent to port the obfuscation if it's a standard HMAC/AES construction | "Miruro" / D3 | If construction is exotic → spike `killed`; SCRAPER-HEAL-37 rolls to v3.2. CONTEXT.md `<open_questions>` first item offers a 6-session bump if operator allows. |
| A8 | AnimeFever's `ctk` token has a stable 32-hex-character shape across all watch pages (the value we observed) | "Pitfall 2" / `ctkRegex` | If the shape varies (e.g., UUID format), the regex must broaden — trivial fix once observed. |
| A9 | 9anime's `data-id` attribute on `<a class="ep-item">` elements is stable across all series pages | "Pitfall 5" | If the attribute changes name → goquery selector update; ~10 min fix. |
| A10 | Adding 3 new providers does not push the failover-chain probe time over Phase 21's 8s per-request budget | "Pitfall 9" | If it does → adjust `SCRAPER_SERVER_PRIORITY` to demote new providers OR shrink per-provider first-server-probe time. |
| A11 | `bun` and Playwright are already installed on the dev/CI environment (per CLAUDE.md) | "Validation Architecture" | If not, `bun install` step needed before Wave 3 e2e — covered by CLAUDE.md "Frontend Note". |
| A12 | The existing `domain.WrapNotFound`, `WrapExtractFailed`, `WrapProviderDown` error helpers exist (referenced throughout code examples) | "Code Examples" | They DO exist — referenced from `services/scraper/internal/providers/allanime/client.go` `[VERIFIED]`. |

**A4-A5 (Miruro obfuscation):** These are the highest-risk assumptions. The 4-session kill-switch is designed exactly to surface a wrong assumption here.

## Open Questions

1. **Should Phase 28's main.go change include `nineanime` in the `candidateProviders` slice unconditionally, or gate it on `SCRAPER_NINEANIME_ENABLED=true`?**
   - What we know: Phase 19's wiring invariant pattern uses an `Enabled` flag for AnimeKai. The CONTEXT.md D5 chain shows nineanime as always-on (registered as failover slot 6, not behind a flag).
   - What's unclear: Whether the operator wants a separate kill-switch for nineanime independent of `SCRAPER_DEGRADED_PROVIDERS=nineanime`.
   - Recommendation: Match the AllAnime pattern — register unconditionally; use `SCRAPER_DEGRADED_PROVIDERS=nineanime` as the kill-switch. The env-based kill is sufficient.

2. **Should the AnimeFever `ctk` token be cached (per-watch-page) or fetched fresh on every `ListServers` call?**
   - What we know: The token appears CSRF-ish (32 hex chars per watch page); the PHPSESSID cookie persists, the ctk may rotate per visit.
   - What's unclear: Whether ctk has a TTL or whether it's stable for the session.
   - Recommendation: Cache for 15 minutes per watch-page-URL key. Refetch on `status:false` AJAX response.

3. **Should the Miruro spike try to extract the obfuscation function automatically from minified JS, or hand-port it from observed behavior?**
   - What we know: The keys (`VITE_PROXY_OBF_KEY`, `VITE_PIPE_OBF_KEY`) are public; the transform is in minified Vite-built JS at `https://www.miruro.tv/assets/index-*.js`.
   - What's unclear: Whether the JS is source-mapped (greatly easing reverse-engineering) or fully minified.
   - Recommendation: Spike starts by checking for source maps (`curl https://www.miruro.tv/assets/index-*.js.map`). If present → JS-level analysis; if absent → black-box test (call known API, observe URL shape, derive transform).

4. **Should the AnimeFever provider fetch `tserver` and `hserver` in parallel for ListServers, or sequentially?**
   - What we know: Both servers respond to `/ajax/anime/load_episodes_v2?s=<server>` independently.
   - What's unclear: Whether parallel fetch is worth the small implementation complexity.
   - Recommendation: Sequential (tserver first). Servers are tried in order at GetStream time anyway; ListServers populating both adds no value if the orchestrator only tries one at a time.

## Sources

### Primary (HIGH confidence)

- **Live recon 2026-05-20** — direct upstream probes from the production server:
  - `https://animefever.cc/` → 200 OK, PHPSESSID set, Cloudflare passive
  - `https://animefever.cc/search/frieren` → returns search HTML with `/info/<slug>` links
  - `https://animefever.cc/info/frieren-beyond-journeys-end.14401` → returns info HTML with `/watch/?ep=<id>` links + Vue.js `episode-image` cards (28 episodes for Frieren S1)
  - `https://animefever.cc/watch/frieren-beyond-journeys-end.14401?ep=189572` → returns watch HTML with `var ctk = '...'`, server `<select>`, `embedDomain = 'https://am.vidstream.vip'`
  - `POST /ajax/anime/load_episodes_v2?s=tserver` with episode_id + ctk → returns `{"status":true,"value":"<iframe ...>","embed":true,...}`
  - `https://am.vidstream.vip/?...` → returns JWPlayer HTML with inline `sources: [{"file":"https://static-cdn-ca1.mofl.pro/...master.m3u8","type":"mp4","label":"HD"}]`
  - `https://9anime.me.uk/wp-json/wp/v2/search?search=frieren` → returns `[{id, title, url: "...frieren-beyond-journeys-end-season-2/", subtype:"series"}]`
  - `https://9anime.me.uk/series/frieren-beyond-journeys-end-season-2/` → returns HTML with `<a class="ep-item" data-number="N" data-id="...">` for each episode
  - `https://9anime.me.uk/hd-frieren-beyond-journeys-end-season-2-episode-1-english-subbed/` → returns HTML with `<iframe src="https://my.1anime.site/index.php?action=play&file=<name>.mp4">`
  - `https://my.1anime.site/index.php?action=play&file=...mp4` → returns HTML with `<source src="videos/...mp4">`
  - `https://my.1anime.site/videos/<name>.mp4` → returns `Content-Type: video/mp4`, 211 MB, `Accept-Ranges: bytes`
  - `https://www.miruro.tv/env2.js` → returns `window.env=JSON.parse("{\"VITE_PROXY_OBF_KEY\":\"a54d389c18527d9fd3e7f0643e27edbe\",...}")` (unchanged from 2026-05-19 sweep)
  - `https://pro.ultracloud.cc/` → HTTP 404 from production IP (endpoint-not-found, NOT challenge — TLS works)

- **In-repo verified file references (per `[VERIFIED: path:line]` tags in this doc):**
  - `services/scraper/internal/domain/provider.go:1-141`
  - `services/scraper/internal/domain/httpclient.go:1-213`
  - `services/scraper/internal/domain/embed.go:1-86`
  - `services/scraper/internal/health/stage.go:1-27`
  - `services/scraper/internal/providers/allanime/client.go:1-647`
  - `services/scraper/internal/providers/animepahe/client.go` (top-of-file)
  - `services/scraper/internal/providers/animepahe/malsync.go:1-120`
  - `services/scraper/internal/embeds/streamhg.go:1-65`
  - `services/scraper/internal/embeds/packed_common.go:1-100`
  - `services/scraper/cmd/scraper-api/main.go:1-453`
  - `libs/videoutils/proxy.go:227-290`
  - `frontend/web/src/composables/useWatchPreferences.ts:60-120`
  - `frontend/web/e2e/raw-player.spec.ts:1-60`

### Secondary (MEDIUM confidence)

- `.planning/research/2026-05-19-en-source-survival.md` (yesterday's survival sweep — verified findings carry forward to 2026-05-20).
- `.planning/phases/26-provider-expansion/26-01-{PLAN,SUMMARY,VERIFICATION}.md` (AllAnime lift template).
- `.planning/phases/27-animepahe-revival-via-stealth-chromium-sidecar/27-CONTEXT.md` D1, D2, D3 (sidecar pattern for future provider isolation).

### Tertiary (LOW confidence — flagged for validation during spikes)

- Miruro's actual obfuscation transform shape (HMAC-SHA256 vs AES-CTR vs other) — SCRAPER-HEAL-34 spike's primary discovery.
- AnimeFever's `ctk` token TTL behavior — Wave 1 28-02 implementation will surface.
- Whether AnimeFever's `hserver` carries all episodes — Wave 1 28-02 implementation will surface.

## Metadata

**Confidence breakdown:**
- AnimeFever provider implementation: HIGH — every endpoint probed live, response shapes captured, embed extraction path verified.
- 9anime.me.uk provider implementation: HIGH — every endpoint probed live, WP REST API returns Frieren S2, episode pages parse cleanly, MP4 confirmed playable from production IP.
- Miruro provider implementation: LOW until SCRAPER-HEAL-34 spike converges; the OBF keys are stable but the transform is unknown. Convergence criteria + kill-switch are well-defined per CONTEXT.md D3.
- Embed extractor (vidstream_vip): HIGH — response shape captured live; regex pattern matches verified bytes.
- Frontend dropdown polish: HIGH — existing `useWatchPreferences.ts` already tracks `preferredScraperProvider`; only label + i18n key additions needed.
- Playwright e2e: MEDIUM — `EnglishPlayer.vue` is a soft dep (Phase 24 hasn't shipped); if not present, Wave 3 28-06 ships polish-only without e2e (per CONTEXT.md `<dependencies>` soft-dep note).

**Research date:** 2026-05-20
**Valid until:** 2026-06-19 (30 days) for AnimeFever / 9anime upstream shape (HTML scrape stability is moderate — daily canary catches changes within 24h regardless); 2026-05-27 (7 days) for Miruro keys (rotation risk). Re-verify before any post-30-day Phase 28 re-execution.
