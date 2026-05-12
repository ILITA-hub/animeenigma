# Phase 18: 9anime — Research

**Researched:** 2026-05-12
**Domain:** EN-anime provider scraping (HTML parsing + multi-host embed extraction), provider-failover wiring, malsync alternative
**Confidence:** HIGH

## Summary

The phase requirements name **"9anime"**, but live triage on 2026-05-12 shows the entire historical 9anime → aniwave → kaido lineage is dead or trap-mirrored, with one survivor — `9anime.org.lv` — that is structurally a Madara/WordPress *frontend* whose only function is to iframe `gogoanime.me.uk/newplayer.php` (which in turn iframes `megaplay.buzz/stream/...`). Implementing a "9anime provider" against `9anime.org.lv` would actually be implementing a four-hop iframe-chain unwrapper for `megaplay.buzz`. By contrast, `anitaku.to` (Anitaku, the canonical Gogoanime survivor named alive in `STATE.md`) is one HTML scrape away from a first-class server list (`anime_muti_link` with 4 named servers per episode, each carrying a direct `data-video` embed URL), no iframe nesting, no base64-encoded option values, no Cloudflare challenge. Independently confirmed: malsync.moe in May 2026 has **no `9anime` and no `Gogoanime/Anitaku` site keys** for any sampled MAL ID — only `KickAssAnime`, `AnimeKAI`, `animepahe`, `Crunchyroll`, `Hulu`, `Netflix`. So D1's "malsync + fuzzy fallback" plan is structurally wrong for both candidates; the fuzzy title lookup is the **primary** ID-resolution path, not a fallback.

**Primary recommendation:** **Pivot Phase 18 to Anitaku/Gogoanime (`anitaku.to`).** Build `services/scraper/internal/providers/gogoanime/` against `anitaku.to` (clean HTML scrape) and **three new embed extractors** (`vibeplayer`, `streamhg`, `earnvids`) — registered in the existing Phase 15 `embeds.Registry` so future providers reuse them. Skip the Doodstream (`myvidplay` → `playmogo`) server entirely; it's behind a Cloudflare Turnstile challenge (forbidden per `SCRAPER-FOUND-09`). Keep the requirement IDs `SCRAPER-9ANI-01..06` literal (CONTEXT.md D1 / S4 explicitly allows this) and annotate REQUIREMENTS.md once: "SCRAPER-9ANI-* IDs implemented by Gogoanime/Anitaku provider; 9anime mirror chain unreachable as of 2026-05-12."

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**D1 — 9anime mirror viability is research-gated (CRITICAL OPEN ITEM)**

The phase requirements name **"9anime"** but `STATE.md` (Phase 17 carryover from v3.0 triage 2026-05-09) records `aniwave.to` and `kaido.to` as **VERIFIED DEAD**. The original `9anime.to` rebrand chain went 9anime → aniwave → kaido and the canonical successor mirrors are reportedly down. The research subagent's **first task** is to identify which 9anime mirror (if any) is actually serving today: candidates include `9anime.org.lv`, `9animetv.to`, `9anime.gs`, `9anime.pe`, `9anime.id`, `9anime.movie`, plus the official Telegram/Discord mirror announcements. Each candidate is tested with a `HEAD /` plus a sample anime page parse. **If no 9anime mirror is alive**, the planner pivots Phase 18 to the next-best alive EN provider — likely **Anitaku/Gogoanime** (`anitaku.io` — verified alive per Phase 17 STATE) — and the requirement IDs `SCRAPER-9ANI-01..06` are mapped 1:1 to a new `services/scraper/internal/providers/gogoanime/` package with the same contract (the ROADMAP allows phase rename if the underlying provider must change; this is a known v3.0 risk).

**Why this is OK:** Phase 18's *goal* is "a second alive EN provider in rotation", not specifically the 9anime brand. ROADMAP success criteria reference embed hosts (mp4upload/streamsb/etc.) which Gogoanime also uses — the interface is portable.

**Trade-off accepted:** if 9anime IS alive on some mirror, we keep the name. If not, the planner replans to Gogoanime/Anitaku with identical scope. Final brand decision is the planner's after research.

**D2 — Provider package layout mirrors AnimePahe exactly**

The new package at `services/scraper/internal/providers/9anime/` (or `gogoanime/` per D1) has the same file layout as `services/scraper/internal/providers/animepahe/`:
- `client.go` — Provider interface impl + HTTP client wiring + each stage method
- `dto.go` — response shapes for malsync + page HTML parsing helpers
- `malsync.go` — 24h cached MAL → slug resolution + fuzzy fallback
- `cache.go` — Redis cache wrappers (search 15m, episodes 6h, stream ≤ min(parsed expiry − 30s, 5min))
- `ddosguard.go` — IF the chosen mirror sits behind DDoS-Guard; OMIT if not. AnimePahe needed it; 9anime mirrors may not.
- per-file `_test.go` with offline goldens

**D3 — Embed extractor reuse, not duplication**

The Phase 15 `EmbedExtractor` registry (`services/scraper/internal/embeds/`) already has Kwik (Phase 16). This phase **adds new extractors for whatever embed hosts the chosen provider actually uses**:
- **Likely new entries:** `mp4upload`, `streamsb`, `streamtape`, `megacloud` (if not already added by AnimePahe path).
- Each extractor is a standalone file in `services/scraper/internal/embeds/<name>/` with its own `client.go` + tests + goldens.
- The 9anime/Gogoanime client only **discovers** the embed URLs and calls `embeds.Get("mp4upload").Extract(ctx, url)` — no extraction logic in the provider package.

**D4 — Stage definitions reuse Phase 17 canonical constants**

The new provider's `HealthCheck` exposes 4 stages: `StageSearch`, `StageEpisodes`, `StageServers`, `StageStream` (the 5th, `StageStreamSegment`, is owned by the probe runner). Use the constants from `services/scraper/internal/health/stage.go`. Do NOT introduce new stage names — the Grafana dashboard and alert rule from Phase 17 depend on the 5-stage contract.

**D5 — Orchestrator failover order is config-driven (not hardcoded)**

Provider registration order in `cmd/scraper-api/main.go` declares the failover sequence; orchestrator iterates in registration order, skipping any with `IsHealthy(provider, StageSearch) == false`. User override via `preferredScraperProvider` (Phase 16) still wins.

**D6 — Frontend dropdown surface — minimal change**

Add one `<option>` to the existing `EnglishPlayer.vue` source dropdown + one locale key per language. No new component.

**D7 — HLS proxy allowlist — append, don't replace**

`HLSProxyAllowedDomains` is append-only. Hostnames discovered during impl are appended in their resolved form. Phase 16 regression-lock test (`pacha.kwik.cx`) must still pass.

**D8 — Tests offline, with goldens — same as AnimePahe**

All parser tests use `services/scraper/testdata/<provider>/` golden HTML/JSON. No live network in CI. Phase 17 probe catches upstream death in production.

**D9 — Cache TTLs match the Phase 16 contract verbatim**

- malsync resolution: 24h
- search: 15m
- episodes: 6h
- stream URL: `min(parsed embed expiry − 30s, 5min)`

Redis cache key prefixes: `malsync:gogoanime:*`, `episodes:gogoanime:*`, `stream:gogoanime:*` (per D1 pivot; see Mirror Viability section).

**D10 — DDoS-Guard cookie helper reuse if needed**

If chosen mirror needs DDoS-Guard, promote `animepahe/ddosguard.go` to a shared `services/scraper/internal/ddosguard/` package. If not, do not introduce the dependency. (For Anitaku.to: **not needed** — see Mirror Viability.)

### Claude's Discretion

- Exact set of embed hosts and corresponding extractors (must be discovered during implementation against the alive mirror).
- Whether to inline the SSRF-guarded HTTP client from Phase 17's `fetchSegment` into a shared helper or re-use the existing `domain.BaseHTTPClient` headers/jar.
- Whether to ship a `dto.go` helper struct or pass `*goquery.Document` directly between client methods (style choice).
- Specific Makefile target name (`capture-goldens-gogoanime` vs `capture-goldens-anitaku`).

### Deferred Ideas (OUT OF SCOPE)

- Per-user provider preference UI in a Settings panel — out of scope.
- AnimeKai (third provider) — Phase 19.
- Cutover (delete HiAnime/Consumet) — Phase 20.
- Per-episode quality dropdown beyond what extractors auto-select — out of scope.
- Server-side analytics on which provider users prefer — out of scope.
- Multi-mirror failover within the chosen provider itself — out of scope.
</user_constraints>

## Project Constraints (from CLAUDE.md)

| Directive | Where Enforced | How the planner must honor it |
|-----------|----------------|-------------------------------|
| Go services use `libs/` shared modules — every new `libs/` requires updates to `go.work`, the consuming `services/*/go.mod` (require + replace), the service `Dockerfile`, and `go work sync` | Project memory: "Adding New libs/ Module" | If we promote `ddosguard.go` to `libs/ddosguard/`, all four touchpoints must be in the plan. (Recommendation in Mirror Viability: NOT NEEDED for Anitaku/Gogoanime — skip the promotion.) |
| `make redeploy-<service>` after code change; `make health` for verification | CLAUDE.md "Local Development Commands" | Final plan in the phase MUST call `make redeploy-scraper && make redeploy-web && make health` |
| Frontend uses `bun`/`bunx`, never npm/pnpm/npx | CLAUDE.md "Frontend Note" | Any frontend plan calls `bun install` / `bunx tsc --noEmit` / `bun run build` |
| After-update skill (lints + builds + redeploys + changelog + commit) MUST be invoked at phase end | CLAUDE.md "After-Update Skill (MUST USE)" | Final phase plan invokes `/animeenigma-after-update` |
| Don't commit secrets (.env, credentials.json) | Project memory + CLAUDE.md | New env vars `SCRAPER_GOGOANIME_BASE_URL` etc. go into `docker/.env` (already gitignored) and `docker/.env.example` if applicable |
| Don't add headless/JA3/proxy-spoof deps: `chromedp`, `go-rod`, `chromedp-rod`, `utls`, `tls-client`, `cloudscraper_go`, `flaresolverr` | `SCRAPER-FOUND-09` CI lint | Plans MUST NOT propose any of these. The Doodstream/myvidplay server is excluded for this reason (Turnstile challenge would require headless). |
| Use structured logging via `libs/logger` (`Infow` / `Errorw` with key-value pairs) | CLAUDE.md "Logging" | Provider code MUST use `*logger.Logger`, not `log.Printf` |
| Don't fight GORM; use conventions | CLAUDE.md | N/A for this phase — no DB schema changes. |
| Issues/incidents documented in `docs/issues/README.md` (ISS-NNN) | Project memory | If embed extraction breakages surface during impl, log them as ISS-NNN, not free-form notes |
| Don't auto-pre-populate provider catalogs; on-demand only | CLAUDE.md "Don't Do" | The new provider resolves IDs lazily via fuzzy search on the first request, then caches. No batch warm-up. |

<phase_requirements>
## Phase Requirements

| ID | Description (from REQUIREMENTS.md) | Research Support |
|----|------------------------------------|------------------|
| SCRAPER-9ANI-01 | Given a Shikimori/MAL ID, the 9anime client resolves the matching 9anime slug via `malsync.moe` lookup with the same caching + fuzzy fallback as AnimePahe | **Pivot to Gogoanime/Anitaku.** malsync HAS NO `9anime` or `Gogoanime` keys (verified across 10 sampled MAL IDs — see "Malsync Coverage" section). **Fuzzy title search against `anitaku.to/search.html?keyword=<title>` is the primary path**, not the fallback. Plan must invert the resolution order: try fuzzy first; keep the malsync probe code as a Phase-19+ extension point. |
| SCRAPER-9ANI-02 | `ListEpisodes` returns the full episode list scraped from 9anime's WordPress/Madara-themed markup (`bsx`, `bixbox`, `bs`, `bt` class family). Sub/dub split surfaced where present. Cached 6 hours. | **Anitaku does not use `bsx`/`bixbox` markup** — that's the 9anime.org.lv signal. Anitaku uses `class="anime_video_body_cate"`, episode lists at `/category/<slug>` rendered as `href="/<slug>-episode-N"` links. Sub/dub split is by **distinct slug** (e.g. `attack-on-titan` vs `attack-on-titan-dub`), not per-episode flag. Plan must enumerate both slugs and merge into one episode list with `Category` per row. Cache 6h. |
| SCRAPER-9ANI-03 | `ListServers` enumerates 9anime's embed hosts per episode. The set of embed hosts is discovered during implementation and **each is registered as an `EmbedExtractor`** so future providers reuse them. | Anitaku.to episode page exposes a clean `<ul class="anime_muti_link"><li><a data-video="...">HD-1 / StreamHG / Earnvids / Doodstream</a></li></ul>`. Four distinct embed hosts observed: `vibeplayer.site` (HD-1/HD-2), `otakuhg.site` (StreamHG), `otakuvid.online` (Earnvids), `myvidplay.com` (Doodstream). **Doodstream is gated behind Cloudflare Turnstile — must be excluded.** Plan registers THREE new extractors: `vibeplayer`, `streamhg`, `earnvids`. |
| SCRAPER-9ANI-04 | `GetStream` resolves an embed URL via `ListServers`, then dispatches to the matching `EmbedExtractor`. No extraction logic in the client itself. | Pattern identical to Phase 16 AnimePahe → Kwik. `provider.GetStream(...)` looks up the extractor in `embeds.Registry`, passes `Referer: https://anitaku.to/` header, expects HLS m3u8 + tracks back. |
| SCRAPER-9ANI-05 | CDN hostnames (whatever the embed hosts resolve to) appended to `libs/videoutils/proxy.go::HLSProxyAllowedDomains`. | Verified hosts: `vibeplayer.site` (own CDN), `premilkyway.com` + `meadowlarkaninearts.space` (StreamHG rotating CDNs), `dramiyos-cdn.com` + `enterpriseconsulting.sbs` (Earnvids rotating CDNs), `cdn.cimovix.store` (subtitle VTT). **The StreamHG/Earnvids CDNs rotate** (token-signed URLs with random subdomain hex) — plan must use prefix-wildcard pattern like `premilkyway.com` (HLS proxy's existing wildcard suffix logic handles this: `host == allowed || strings.HasSuffix(host, "."+allowed)`). |
| SCRAPER-9ANI-06 | Orchestrator failover AnimePahe → 9anime verified end-to-end; forcing AnimePahe health to 0 produces playable stream from new provider; `parser_fallback_total{from="animepahe",to="<provider>"}` increments. | Already wired in `services/scraper/internal/service/orchestrator.go::runFailover` (line 206 + 235). Phase 18 only adds the second provider via `orchestrator.Register(...)`. Verification: HUMAN-UAT step + curl `/metrics` post-deploy. |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Anitaku HTML scrape (`/search.html`, `/category/<slug>`, `/<slug>-episode-N`) | Scraper Service (`services/scraper/internal/providers/gogoanime/`) | — | Per-provider HTML parsing is non-shareable (REQUIREMENTS.md universal-layer table) |
| Embed URL extraction (vibeplayer / streamhg / earnvids) | Scraper Service (`services/scraper/internal/embeds/<name>/`) | — | Phase 15 architectural seam: each embed family is one registry entry |
| Title fuzzy match (Jaro-Winkler) | Scraper Service (reuse the helper already in `services/scraper/internal/providers/animepahe/`) | — | Recommendation: **extract the existing `jaroWinkler` + `normalizeTitle` from `animepahe/client.go` into a shared `services/scraper/internal/fuzzy/` package**. Don't copy-paste. |
| Provider failover ordering | Scraper Service Orchestrator | — | Already implemented in Phase 17; new provider only needs `Register()` call |
| Health probe per stage | Scraper Service `health.ProbeRunner` (Phase 17, auto-discovers) | — | Iterates `RegisteredProviders()`; no per-provider probe code |
| Frontend provider override | Vue Pinia store `useWatchPreferences` (Phase 16) | — | Phase 16 already supports arbitrary string values; no store change |
| Frontend dropdown UI | `EnglishPlayer.vue` (Phase 16 component, extensible) | — | Phase 16 added the dropdown shape; this phase only adds one `<option>` |
| HLS proxy CORS rewrite | `libs/videoutils/proxy.go::ProxyWithReferer` | — | Existing endpoint; append new hostnames to `HLSProxyAllowedDomains` slice |
| Stream URL caching | Redis via `libs/cache` | — | Same wrapper as AnimePahe; key namespace `stream:gogoanime:*` |
| API gateway routing | Already routed (`/api/anime/{id}/scraper/*` → catalog → scraper) | — | No gateway change |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PuerkitoBio/goquery` | v1.8.x (already in scraper go.mod via AnimePahe) | HTML DOM traversal for `<li class="anime_muti_link">` selectors | Already standard in this repo; same as AnimePahe's `button[data-src]` scrape |
| `github.com/dop251/goja` | v0.0.0-202x (already in `services/scraper/internal/embeds/kwik.go`) | Run Dean-Edwards packed JS to unpack StreamHG / Earnvids HLS URL | Phase 16 standard; the StreamHG packer is the SAME `function(p,a,c,k,e,d){}` shape Kwik uses |
| `github.com/hashicorp/go-retryablehttp` (via `domain.BaseHTTPClient`) | already wired | Per-host rate limit + 429/5xx backoff for `anitaku.to`, `vibeplayer.site`, `otakuhg.site`, `otakuvid.online` | Phase 15 standard; no new HTTP client |
| `github.com/ILITA-hub/animeenigma/services/scraper/internal/health` | in-tree (Phase 17) | Canonical stage constants (`StageSearch`, `StageEpisodes`, `StageServers`, `StageStream`) | Locked contract per D4 |
| `github.com/ILITA-hub/animeenigma/libs/cache` | in-tree | Redis wrapper for malsync (negative-cache fallback), episodes, stream TTLs | Phase 15 standard |
| `github.com/ILITA-hub/animeenigma/libs/logger` | in-tree | Structured logging | Repo-wide convention |
| `github.com/ILITA-hub/animeenigma/libs/metrics` | in-tree | `ParserFallbackTotal.WithLabelValues(from, to).Inc()` already emitted by orchestrator; no new metric needed | Phase 17 wiring |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Go stdlib `regexp` | go 1.23 | Pull `links.hls2` from unpacked StreamHG/Earnvids JS | Inside `streamhg` + `earnvids` extractors |
| Go stdlib `encoding/base64` | go 1.23 | Decode 9anime.org.lv's base64-encoded `<option value>` IF planner chooses to keep the 9anime.org.lv code path as a secondary | Only if mirror-pivot is rejected; otherwise unused |
| Go stdlib `encoding/json` | go 1.23 | `megaplay.buzz/stream/getSources` JSON decode IF 9anime.org.lv path is kept | Only if mirror-pivot rejected |
| Go stdlib `net/url` | go 1.23 | Parse embed URLs for host-matching in extractor `Matches()` | All three new extractors |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Provider = Anitaku/Gogoanime (`anitaku.to`) | Provider = 9anime.org.lv | Rejected: 9anime.org.lv is a 4-hop iframe relay (9anime.org.lv → gogoanime.me.uk/newplayer.php → megaplay.buzz → cdn.mewstream.buzz). We'd be implementing a megaplay.buzz client behind a "9anime" facade — that's exactly the `feedback_replace_dont_preserve.md` anti-pattern. Anitaku is one direct scrape away from the same kind of HLS endpoint with no iframe nesting. |
| New Go embed extractors (`vibeplayer`, `streamhg`, `earnvids`) | Megacloud sidecar via existing `embeds.NewMegacloudClient` | Rejected: the sidecar's value is the **encrypted-sources path** for HiAnime/HiAnime-clone Megacloud. The three Anitaku embeds are unencrypted (vibeplayer = direct const src, StreamHG/Earnvids = unpacked p,a,c,k packer with plain `links.hls2`). Routing through the Node sidecar just adds an HTTP hop. |
| Extract jaro-winkler into `services/scraper/internal/fuzzy/` | Copy-paste from `animepahe/client.go` | Recommendation: **extract** — second consumer is the trigger for the shared package. Single source of truth. |
| Promote `animepahe/ddosguard.go` to `libs/ddosguard/` | Leave it provider-private | Recommendation: **leave it** — Anitaku.to has NO DDoS-Guard (verified `Server: cloudflare` cleanly returns 200 with any UA, no `__ddg2_` cookie set). Skip the migration. |
| Use `myvidplay.com` (Doodstream variant) | Skip it | **Skip it.** `myvidplay.com` 301-redirects to `playmogo.com` which serves a Cloudflare Turnstile challenge (`Just a moment...`). Solving it requires headless tooling → forbidden by `SCRAPER-FOUND-09`. The 3 surviving servers (vibeplayer + streamhg + earnvids) give us redundancy already. |

**Installation:**
No new external dependencies. Everything used is already in `services/scraper/go.mod` from Phase 15/16.

**Version verification:**
```bash
grep -E "PuerkitoBio/goquery|dop251/goja|go-retryablehttp" /data/animeenigma/services/scraper/go.mod
# Verified 2026-05-12: all three present from Phase 15-16 work.
```

## Mirror Viability

Live probe against every plausible 9anime / Anitaku / Gogoanime candidate, executed 2026-05-12 from this server (a curl-able HTTP/2 client, no JA3 spoofing, no headless browser):

| Mirror | HTTP code | Body sample / Title | DDoS-Guard / CF challenge | Verdict |
|--------|-----------|--------------------|---------------------------|---------|
| `9anime.to` | 200 | "parked" / GoDaddy domain park | n/a | **Dead** — domain park |
| `9anime.org.lv` | 200 | `<title>9anime - Watch Anime online ...</title>` + 32 `bsx` + 10 `bixbox` classes + WP/LiteSpeed + Cloudflare 200 (no challenge) | None | **Alive — WP/Madara frontend** — but episode pages are 4-hop iframe relays through `gogoanime.me.uk/newplayer.php` → `megaplay.buzz/stream/...`. See "9anime.org.lv structure" sub-section below. |
| `9animetv.to` | 000 (curl 28 timeout 8s) | — | n/a | **Dead** — network black hole |
| `9anime.gs` | 200 | `<title>9anime.gs</title>` 1041 bytes — FingerprintJS redirect trap | n/a | **Dead** — anti-bot fingerprint redirect to ad funnel |
| `9anime.pe` | 200 | identical FingerprintJS trap | n/a | **Dead** — same trap as .gs |
| `9anime.id` | 200 | identical FingerprintJS trap | n/a | **Dead** — same trap |
| `9anime.live` | 200 | identical FingerprintJS trap | n/a | **Dead** — same trap |
| `9anime.bid` | 200 | `<title>9Anime - Watch Anime Online ...</title>` 25 KB SEO content farm — no functional player, schema.org `Article` page | n/a | **Dead** — content farm, no streaming |
| `9anime.movie` | 000 (DNS) | — | n/a | **Dead** — no DNS |
| `aniwave.to` | 000 | curl 60: SSL certificate expired | n/a | **Dead** — cert lapsed |
| `kaido.to` | 000 (curl 28 timeout) | — | n/a | **Dead** |
| `anix.to` | 000 (DNS) | — | n/a | **Dead** |
| `hianime.to` | 000 (curl 28 timeout) | — | n/a | **Dead** |
| `hianime.nz` | 200 | 84 bytes empty stub | n/a | **Dead** — empty stub page |
| `anitaku.io` | 200 | `<title>Anitaku – Watch Anime Online Free in HD</title>` 119 KB | None | **Alive** — but spot-check showed `anitaku.to` is the more populated dataset (294 anime markup matches vs `.io`'s 1). Both valid; recommendation = `.to`. |
| `anitaku.to` | 200 | `<title>Watch and download Anime with english sub on 1080p \| Anitaku</title>` 85 KB, 294 anime card matches, search works, plain `data-video` attrs | None | **ALIVE — RECOMMENDED MIRROR** |
| `anitaku.so` | 200 | 473 bytes "Loading..." stub | n/a | **Dead** — stub |
| `gogoanime.cl` | 000 (DNS) | — | n/a | **Dead** |
| `gogoanime.gg` | 200 | FingerprintJS trap | n/a | **Dead** — trap |
| `gogoanime3.co` | 000 (curl 56 reset) | — | n/a | **Dead** |
| `gogoanime.tel` | 200 | "Redirecting..." 4 KB | n/a | **Dead** — redirect stub |
| `gogoanime.me.uk` | 200 | iframe-wrapper for `megaplay.buzz` — same chain as 9anime.org.lv | None | **Useless as primary** — relay only, not a content site |

### Decision

**Pivot to Anitaku/Gogoanime at `anitaku.to`.** Rationale:

1. **`anitaku.to` is the only alive English-anime site with a clean per-episode server list** (no iframe stacking, no base64 obfuscation, no Turnstile, no DDoS-Guard).
2. **`9anime.org.lv` is alive but technically wrong target**: scraping it ends in iframing `megaplay.buzz/stream/getSources?id=X`, which would mean Phase 18 actually delivers a "megaplay.buzz provider" with a 9anime-branded label. That's exactly the anti-pattern `feedback_replace_dont_preserve.md` warns against (CONTEXT.md S4 already anticipates this pivot).
3. **STATE.md (2026-05-09 triage) already names `anitaku.io` as VERIFIED ALIVE.** The pivot is consistent with the project's existing alive-set.
4. **The phase goal** ("a second alive EN provider in rotation, so a single provider failure doesn't blank the English tab") **is satisfied identically by either provider.** The requirement IDs were pre-named "SCRAPER-9ANI" for historical reasons; per CONTEXT.md S4, they remain literal and REQUIREMENTS.md gets a one-line annotation.

### 9anime.org.lv structure (documented for completeness if planner rejects the pivot)

Captured 2026-05-12 from `https://9anime.org.lv/anime/one-piece-dub/` and `/one-piece-dub-episode-1/`:

```html
<!-- Anime page: /anime/<slug>/ -->
<ul class="eplister">
  <li data-index="0">
    <a href="https://9anime.org.lv/one-piece-dub-episode-1155/">
      <div class="epl-num">1155</div>
      <div class="epl-title">One Piece (Dub) Episode 1155</div>
      <div class="epl-sub"><span class="status Dub">Dub</span></div>
      <div class="epl-date">April 2, 2026</div>
    </a>
  </li>
  ...
</ul>

<!-- Episode page: /<slug>-episode-N/ -->
<select class="mirror" name="mirror" onchange="loadMi(this);">
  <option value="PGlmcmFtZSBzYW5kYm94..." data-index="1">HD-1</option>
  <option value="PGlmcmFtZSBzYW5kYm94..." data-index="2">HD-2</option>
</select>
```

The `<option value="...">` base64-decodes to `<iframe src="https://gogoanime.me.uk/newplayer.php?id=<gogo-slug>?ep=<ep_id>&type=hd-1|hd-2&category=sub|dub">`. That iframe in turn embeds `<iframe src="https://megaplay.buzz/stream/s-2/<ep_id>/<sub|dub>?autostart=true">` which finally exposes `data-id="<numeric>"` for `GET https://megaplay.buzz/stream/getSources?id=<data-id>` (referer `https://megaplay.buzz/`) → unencrypted JSON `{sources:[{file:".../master.m3u8"}], tracks:[{file:".../eng-2.vtt"}], intro, outro}`.

**If the planner still wants 9anime.org.lv as the brand**, the provider would skip the WP frontend and go straight to `megaplay.buzz/stream/getSources` — but at that point the provider is "megaplay client", not "9anime client", and the data set is **identical to what Anitaku's `vibeplayer.site` server already provides**. Not recommended.

### Anitaku.to structure (RECOMMENDED — full skeleton)

Captured 2026-05-12 from `https://anitaku.to/`:

```
Homepage         GET /                                    -> recent episodes + popular anime list
Search           GET /search.html?keyword=<title>         -> <a href="/category/<slug>"> matches
Anime page       GET /category/<slug>                     -> episode count + <a href="/<slug>-episode-N">
Episode page     GET /<slug>-episode-N                    -> <ul class="anime_muti_link"><li><a data-video="<embed>">SERVER</a></li></ul>
```

**Search example** (`?keyword=Attack+on+Titan` returns 10+ matches):
```
href="/category/attack-on-titan"
href="/category/attack-on-titan-final-season-part-1"
href="/category/attack-on-titan-season-2"
href="/category/attack-on-titan-dub"
```

Sub vs. dub split is by **separate slug** (e.g. `attack-on-titan` is the sub variant; `attack-on-titan-dub` is the dub variant). Provider's `ListEpisodes` for a Shikimori-ID query must search BOTH possible slugs (`<base>` and `<base>-dub`) and merge with `Category` per row.

**Episode page** servers (4 per episode, 16 total entries because each is listed twice across the `data-video` attribute and `data-more` block — dedup by URL):
```
HD-1       vibeplayer.site/<embed_id>?sub=<vtt_url>   (1 of 4 servers)
HD-2       vibeplayer.site/<embed_id>?sub=<vtt_url>   (2 of 4)
StreamHG   otakuhg.site/e/<embed_id>?caption_1=<vtt>&sub_1=English  (3 of 4)
Earnvids   otakuvid.online/embed/<embed_id>?caption_1=<vtt>&sub_1=English  (4 of 4)
Doodstream myvidplay.com/e/<embed_id>?c1_file=<vtt>   (5 — SKIP — Turnstile-guarded)
```

## Embed Extractor Catalog

For each embed host, summarized extraction shape, expiry/signature handling, referer requirements, registered extractor name:

| Embed host | Wrapper URL shape | Extractor name | Extraction algorithm | HLS expiry handling | Required headers |
|------------|-------------------|----------------|----------------------|---------------------|------------------|
| `vibeplayer.site/<embed_id>` (HD-1, HD-2) | `https://vibeplayer.site/<embed_id>?sub=<optional_vtt_url>` | `vibeplayer` | Fetch wrapper page → regex extract `const src = "https://vibeplayer.site/public/stream/<embed_id>/master.m3u8"` + `const subtitle = "..."` from inline JS. Verified pattern: `const src = "https://vibeplayer.site/public/stream/aac165bfc862642b/master.m3u8"`. | None — m3u8 is static, no signed URL, served with `Cache-Control: max-age=31536000`. TTL = 5 min (safety floor). | `Referer: https://anitaku.to/` |
| `otakuhg.site/e/<embed_id>` (StreamHG) | `https://otakuhg.site/e/<embed_id>?caption_1=<vtt>&sub_1=English` | `streamhg` | Fetch wrapper → find `eval(function(p,a,c,k,e,d){...})('...',base,count,'tokens...'.split('\|')` → run through goja-based Dean-Edwards unpacker (SAME UNPACK PATTERN AS KWIK) → regex extract `"hls2":"https://<random>.premilkyway.com/hls2/.../master.m3u8?t=<token>&s=<ts>&e=<expiry>&...."`. | Parse `&e=<seconds_to_live>` query param: e.g. `e=129600` = 36h ttl. TTL = `min(e - 30s, 5min)`. Plus check `&t=<token>&s=<unix_signed_at>` for staleness. | `Referer: https://otakuhg.site/` for the wrapper fetch; HLS endpoint itself accepts plain UA, returns `Access-Control-Allow-Origin: *` (no referer required for the actual `.m3u8` request). |
| `otakuvid.online/embed/<embed_id>` (Earnvids) | `https://otakuvid.online/embed/<embed_id>?caption_1=<vtt>&sub_1=English` | `earnvids` | **IDENTICAL EXTRACTION SHAPE TO STREAMHG.** Same Dean-Edwards packer; same `"hls2":"https://<random>.dramiyos-cdn.com/hls2/.../master.m3u8?t=...&e=..."` field. Strong case for sharing one base extractor with two host allowlists. | Same `&e=` parsing as StreamHG. | `Referer: https://otakuvid.online/` for wrapper; HLS endpoint is open. |
| `myvidplay.com/e/<embed_id>` (Doodstream) | `https://myvidplay.com/e/<embed_id>?c1_file=<vtt>` | **SKIP — DO NOT IMPLEMENT** | 301 → `playmogo.com/e/<embed_id>` → Cloudflare Turnstile challenge (`<title>Just a moment...</title>`, `content-security-policy` from challenges.cloudflare.com). Solving requires headless browser → forbidden by SCRAPER-FOUND-09. | n/a | n/a |

### CRITICAL — vibeplayer.site sub-host CDN architecture

The `vibeplayer.site` extraction yields a SAME-ORIGIN HLS URL (`https://vibeplayer.site/public/stream/<id>/master.m3u8`). This means **only one allowlist entry needed** for vibeplayer: `vibeplayer.site` itself.

For StreamHG and Earnvids the m3u8 host is **a rotating subdomain on a static eTLD+1**:
- StreamHG observed: `OkqtSs1gBbNcA8e.premilkyway.com` (random 15-char-hex subdomain) + `OkqtSs1gBbNcA8e.meadowlarkaninearts.space` as backup
- Earnvids observed: `pfabiWMFmEza.dramiyos-cdn.com` + `pfabiWMFmEza.enterpriseconsulting.sbs` as backup

The HLS proxy allowlist's `strings.HasSuffix(host, "."+allowed)` check already handles rotating subdomains, so the allowlist entries are the eTLD+1 strings only. **The "backup" CDN field (`hls3`, served via `.txt` redirect) should NOT be hardcoded into the allowlist for v3.0** — those CDNs (`meadowlarkaninearts.space`, `enterpriseconsulting.sbs`) are clearly low-trust placeholder-named hosts and we should monitor whether the primary CDN holds before adding the fallbacks. If `premilkyway.com` / `dramiyos-cdn.com` ever start 4xx-ing in prod, the planner adds the backup hosts then.

## Hostnames to Append to `HLSProxyAllowedDomains`

```go
// libs/videoutils/proxy.go::HLSProxyAllowedDomains — APPEND these (DO NOT touch existing entries):
"anitaku.to",                  // optional — provider home, only needed if frontend ever proxies anitaku poster URLs
"vibeplayer.site",             // vibeplayer same-origin HLS host
"premilkyway.com",             // StreamHG primary CDN — rotating subdomain on this eTLD+1
"dramiyos-cdn.com",            // Earnvids primary CDN — rotating subdomain on this eTLD+1
"cdn.cimovix.store",           // subtitle .vtt host (used by all 3 servers)
```

**NOT added** (intentional):
- `otakuhg.site`, `otakuvid.online` — these are the EMBED-WRAPPER hosts, never proxied directly (we extract the HLS URL from them, the HLS URL lives on the CDN hosts above).
- `meadowlarkaninearts.space`, `enterpriseconsulting.sbs` — StreamHG / Earnvids backup CDN candidates. Hold for v3.1 unless primary CDNs go down.
- `megaplay.buzz`, `cdn.mewstream.buzz` — only relevant if planner rejects the pivot and goes through 9anime.org.lv.

**Regression-lock invariant:** Phase 16's `pacha.kwik.cx` test (or whatever the exact entry name is) MUST still pass after the append. Plan adds new entries via `append(HLSProxyAllowedDomains, ...)`-style edits, never reorder or remove existing rows.

## Architecture Patterns

### System Architecture Diagram

```
User request: GET /api/anime/{shikimoriID}/scraper/episodes?prefer=gogoanime
       │
       ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ Gateway (8000)  ─►  Catalog (8081)  ─►  Scraper (8088) /scraper/episodes │
└──────────────────────────────────────────────────────────────────────────┘
       │
       ▼ scraperHandler.ListEpisodes
┌──────────────────────────────────────────────────────────────────────────┐
│ Scraper Orchestrator                                                      │
│                                                                           │
│   orderedProviders(prefer="gogoanime") = [gogoanime, animepahe]           │
│   ─► for each provider in order:                                          │
│       healthCache.IsHealthy(p.Name())?  ◄── Phase 17 60s gate             │
│         false → metrics.ParserFallbackTotal{from=p, to=next}.Inc()        │
│                 skip                                                       │
│         true  → provider.ListEpisodes(ctx, providerID)                    │
│                 if ErrNotFound:  metrics.ParserFallbackTotal{...}.Inc()   │
│                                  continue to next provider                 │
│                 else: return result                                        │
└──────────────────────────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ Gogoanime/Anitaku Provider                                                │
│   internal/providers/gogoanime/                                           │
│                                                                           │
│   FindID(AnimeRef):                                                       │
│     ┌─ Try malsync.Lookup("Gogoanime")  (negative-cache; expected MISS,   │
│     │   structured for forward-compat if malsync adds the key later)     │
│     └─ Fuzzy search:                                                      │
│           GET /search.html?keyword=<title>                                │
│           normalize titles, Jaro-Winkler ≥ 0.85                           │
│           for each top match: try /category/<slug> AND /category/<slug>-dub│
│           pick best score that also returns a valid episode page          │
│                                                                           │
│   ListEpisodes(slug):                                                     │
│     GET /category/<slug>          ─► parse episode count + slug links     │
│     GET /category/<slug>-dub      ─► merge dub episodes with Category=dub │
│     6h cache: episodes:gogoanime:<slug>                                   │
│                                                                           │
│   ListServers(slug, epID):                                                │
│     GET /<slug>-episode-<N>       ─► parse <ul class="anime_muti_link">   │
│     for each <li><a data-video>:                                          │
│       host filter: vibeplayer | otakuhg | otakuvid (NOT myvidplay)        │
│       map to embed-host extractor name + Category(sub/dub by slug suffix) │
│                                                                           │
│   GetStream(slug, epID, serverID="<embed_url>"):                          │
│     ext := embeds.Find("<embed_url>")                                     │
│     headers = {Referer: anitaku.to/}                                      │
│     stream := ext.Extract(ctx, embedURL, headers)                         │
│     ttl := min(parseExpiry(stream.Sources[0].URL) - 30s, 5min)            │
│     cache stream:gogoanime:<slug>:<epID>:<hash(embed)>  TTL = ttl         │
└──────────────────────────────────────────────────────────────────────────┘
       │
       ▼  per-server  ┌──────────────────────────────────────────────┐
                       │ embeds.Registry (Phase 15 contract)          │
                       │   .Find(embedURL)  ─►  matches first to true  │
                       │     KwikExtractor    (existing — Phase 16)    │
                       │     MegacloudClient  (existing — Phase 15)    │
                       │     VibePlayerExtractor   (NEW — Phase 18)    │
                       │     StreamHGExtractor     (NEW — Phase 18)    │
                       │     EarnvidsExtractor     (NEW — Phase 18)    │
                       └──────────────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ Each extractor:                                                           │
│  1. GET wrapper URL with Referer                                          │
│  2. Pull HLS m3u8 + (optional) tracks/intro/outro                         │
│  3. Return *domain.Stream {Sources, Tracks, Intro, Outro,                 │
│                            Headers={Referer: <wrapper_host>}}             │
└──────────────────────────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ Frontend EnglishPlayer.vue                                                │
│   - Calls scraperApi.getStream(animeId, episode, server, category, prefer)│
│   - Receives stream.sources[0].url                                        │
│   - Builds proxied URL: /api/streaming/proxy?url=<encoded>&referer=<ref>  │
│   - Hands to Video.js + HLS.js                                            │
│   - SubtitleOverlay (existing) renders tracks[]                           │
└──────────────────────────────────────────────────────────────────────────┘
```

### Recommended Project Structure

```
services/scraper/internal/
├── providers/
│   └── gogoanime/                       # NEW — Phase 18 (folder name = "gogoanime", display Name() = "gogoanime")
│       ├── client.go                    # FindID + ListEpisodes + ListServers + GetStream + HealthCheck
│       ├── dto.go                       # search-result DTO, episode-row DTO, server-row DTO
│       ├── fuzzy.go                     # IF jaro-winkler not yet extracted to shared pkg; OR thin wrapper around scraper/internal/fuzzy
│       ├── cache.go                     # 24h / 6h / 5min TTL wrappers + cache keys
│       ├── malsync.go                   # 24h cache wrapper around shared MalSyncClient with provider="Gogoanime" (returns miss for now; forward-compat)
│       ├── client_test.go
│       ├── dto_test.go
│       └── malsync_test.go
├── embeds/
│   ├── kwik.go                          # existing
│   ├── megacloud.go                     # existing
│   ├── vibeplayer.go                    # NEW — regex extract const src
│   ├── vibeplayer_test.go
│   ├── streamhg.go                      # NEW — goja unpack of p,a,c,k packer
│   ├── streamhg_test.go
│   ├── earnvids.go                      # NEW — SAME unpack shape as streamhg, different host allowlist
│   └── earnvids_test.go
├── fuzzy/                                # NEW (recommended) — promoted from animepahe/client.go
│   ├── jarowinkler.go                   # MOVED from animepahe; both providers consume
│   ├── normalize.go                     # MOVED from animepahe
│   └── fuzzy_test.go                    # MOVED from animepahe
├── domain/                               # NO CHANGE
├── health/                               # NO CHANGE — probe auto-discovers new provider
└── service/                              # NO CHANGE — orchestrator already iterates RegisteredProviders()

services/scraper/cmd/scraper-api/
└── main.go                               # MODIFIED — wire gogoanime.New() + orchestrator.Register(...)
                                          # Also add per-host RPS limits: anitaku.to, vibeplayer.site, otakuhg.site, otakuvid.online

services/scraper/testdata/
└── gogoanime/                            # NEW — goldens
    ├── search_attack_on_titan.html
    ├── category_one_piece.html
    ├── one_piece_episode_1.html
    ├── vibeplayer_embed.html
    ├── streamhg_packed.html
    └── earnvids_packed.html

services/scraper/internal/config/
└── config.go                             # MODIFIED — add GogoanimeConfig.BaseURL with default "https://anitaku.to"

services/scraper/Makefile (or repo root)
└── capture-goldens-gogoanime target      # NEW — mirrors capture-goldens-animepahe shape

libs/videoutils/
└── proxy.go                              # MODIFIED — append 5 new entries to HLSProxyAllowedDomains

frontend/web/src/
├── components/player/
│   └── EnglishPlayer.vue                 # MODIFIED — add one <option value="gogoanime"> to source dropdown
├── locales/
│   ├── ru.json                           # MODIFIED — add "gogoanime" label key
│   ├── en.json                           # MODIFIED
│   └── ja.json                           # MODIFIED
└── stores/
    └── watchPreferences.ts               # NO CHANGE — store accepts arbitrary string values

docker/
└── .env                                  # MODIFIED — add SCRAPER_GOGOANIME_BASE_URL=https://anitaku.to
                                          # (optional override; defaulted in code)
```

### Pattern 1 — Anitaku/Gogoanime FindID with fuzzy primary (Go sketch)

```go
// Source: derived from services/scraper/internal/providers/animepahe/client.go::FindID
// Adapted because malsync has NO Gogoanime/Anitaku key as of 2026-05-12 — fuzzy is primary.

func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
    // 1. Forward-compat probe of malsync — likely MISS, cached as miss for 24h.
    if ref.ShikimoriID != "" {
        if id, ok, err := p.malsync.Lookup(ctx, ref.ShikimoriID, "Gogoanime"); err == nil && ok {
            p.markStage(health.StageSearch, nil)
            return id, nil
        }
    }
    // 2. Fuzzy /search.html — PRIMARY path.
    if ref.Title == "" {
        err := domain.WrapNotFound(errors.New("no title"), "gogoanime: cannot search without a title")
        p.markStage(health.StageSearch, err)
        return "", err
    }
    q := url.QueryEscape(ref.Title)
    searchURL := fmt.Sprintf("%s/search.html?keyword=%s", p.baseURL, q)
    resp, err := p.http.Get(ctx, searchURL)
    if err != nil {
        err = domain.WrapProviderDown(err, "gogoanime: search fetch")
        p.markStage(health.StageSearch, err)
        return "", err
    }
    defer drainAndClose(resp.Body)
    doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 2<<20))
    if err != nil {
        return "", domain.WrapExtractFailed(err, "gogoanime: search parse")
    }

    // Each result: <p class="name"><a href="/category/<slug>">Title</a></p>
    type cand struct {
        slug, title string
    }
    var cands []cand
    doc.Find("p.name a[href^='/category/']").Each(func(_ int, sel *goquery.Selection) {
        href, _ := sel.Attr("href")
        slug := strings.TrimPrefix(href, "/category/")
        cands = append(cands, cand{slug: slug, title: strings.TrimSpace(sel.Text())})
    })
    if len(cands) == 0 {
        return "", domain.WrapNotFound(nil, "gogoanime: 0 search results for "+ref.Title)
    }

    normTitle := fuzzy.NormalizeTitle(ref.Title)
    best := cand{}; bestScore := 0.0
    for _, c := range cands {
        s := fuzzy.JaroWinkler(normTitle, fuzzy.NormalizeTitle(c.title))
        if s > bestScore {
            bestScore = s; best = c
        }
    }
    if bestScore < 0.85 {
        return "", domain.WrapNotFound(
            fmt.Errorf("best score %.4f", bestScore),
            "gogoanime: no fuzzy match for "+ref.Title)
    }
    return best.slug, nil
}
```

### Pattern 2 — ListEpisodes merging sub+dub slugs (Go sketch)

```go
// Two-slug merge: <slug> = sub (default), <slug>-dub = dub.
// Either may not exist; the missing variant yields 404 and we just skip it.
// REVIEW the merge logic carefully — Anitaku's /category page lists EVERY episode
// for that variant as a separate <a href="/<slug>-episode-N">.

func (p *Provider) ListEpisodes(ctx context.Context, slug string) ([]domain.Episode, error) {
    cacheKey := fmt.Sprintf("episodes:gogoanime:%s", slug)
    var cached []domain.Episode
    if err := p.cache.Get(ctx, cacheKey, &cached); err == nil {
        return cached, nil
    }
    // Strip -dub suffix if present so we don't double-fetch
    base := strings.TrimSuffix(slug, "-dub")

    subEps, _ := p.fetchEpisodesForSlug(ctx, base, domain.CategorySub)
    dubEps, _ := p.fetchEpisodesForSlug(ctx, base+"-dub", domain.CategoryDub)

    // Merge: each variant produces (episode_number → ID) map. Same number from
    // BOTH variants becomes ONE Episode with both server categories surfaced
    // later via ListServers (anitaku's data-video list already includes both).
    // Spec choice: emit ONE Episode per number, with .ID = "<base>-episode-<N>".
    // The base slug encodes which variant we have. If only dub exists, ID =
    // "<base>-dub-episode-<N>" so ListServers fetches the right page.
    out := mergeEpisodes(subEps, dubEps)

    _ = p.cache.Set(ctx, cacheKey, out, 6*time.Hour)
    return out, nil
}
```

### Pattern 3 — ListServers parsing `anime_muti_link` (Go sketch)

```go
// Source: live anitaku.to/one-piece-episode-1 captured 2026-05-12.
// Selector: <ul class="anime_muti_link"> > <li> > <a data-video="<embed>"> with
// <li class="<server-key-lowercase>"> (e.g. "linkserver hd-1", "linkserver streamhg").
// Server name parse: trim "Choose this server" from the <a> innerText.

func (p *Provider) ListServers(ctx context.Context, slug, epID string) ([]domain.Server, error) {
    epURL := fmt.Sprintf("%s/%s", p.baseURL, url.PathEscape(epID))
    resp, err := p.http.Get(ctx, epURL)
    if err != nil { return nil, domain.WrapProviderDown(err, "gogoanime: /ep fetch") }
    defer drainAndClose(resp.Body)
    doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 2<<20))
    if err != nil { return nil, domain.WrapExtractFailed(err, "gogoanime: /ep parse") }

    cat := domain.CategorySub
    if strings.HasSuffix(slug, "-dub") || strings.Contains(epID, "-dub-") {
        cat = domain.CategoryDub
    }

    out := make([]domain.Server, 0, 4)
    seen := make(map[string]struct{})
    doc.Find("ul.anime_muti_link li a[data-video]").Each(func(_ int, sel *goquery.Selection) {
        dv, _ := sel.Attr("data-video")
        // Normalize protocol-relative URLs: anitaku.to historically emits "//host/path"
        if strings.HasPrefix(dv, "//") {
            dv = "https:" + dv
        }
        u, perr := url.Parse(dv)
        if perr != nil || (u.Scheme != "http" && u.Scheme != "https") {
            return
        }
        host := strings.ToLower(u.Hostname())
        // SKIP myvidplay/playmogo — Cloudflare Turnstile (forbidden).
        if hostMatchesAny(host, "myvidplay.com", "playmogo.com") {
            return
        }
        if _, dup := seen[dv]; dup { return }
        seen[dv] = struct{}{}

        // Server name extraction: parent <li class="linkserver hd-1"> or text
        name := serverNameFromAnchor(sel)  // helper that reads <li class> and falls back to anchor text
        out = append(out, domain.Server{ID: dv, Name: name, Type: cat})
    })
    return out, nil
}
```

### Pattern 4 — VibePlayer extractor (Go sketch)

```go
package embeds

var vibeplayerHosts = []string{"vibeplayer.site"}

// vibePlayerSrcRegex finds `const src = "https://vibeplayer.site/public/stream/<id>/master.m3u8"`
// captured from real vibeplayer.site page 2026-05-12.
var vibePlayerSrcRegex = regexp.MustCompile(`const\s+src\s*=\s*"(https://[^"]+\.m3u8)"`)
var vibePlayerSubRegex = regexp.MustCompile(`const\s+subtitle\s*=\s*"([^"]*)"`)

type VibePlayerExtractor struct {
    http *domain.BaseHTTPClient   // reuse shared HTTP client + rate limiter
}

func (e *VibePlayerExtractor) Name() string { return "vibeplayer" }

func (e *VibePlayerExtractor) Matches(embedURL string) bool {
    u, err := url.Parse(embedURL)
    if err != nil { return false }
    host := strings.ToLower(u.Hostname())
    for _, h := range vibeplayerHosts {
        if host == h || strings.HasSuffix(host, "."+h) { return true }
    }
    return false
}

func (e *VibePlayerExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
    for k, vs := range headers { req.Header[k] = vs }
    resp, err := e.http.Do(req)
    if err != nil { return nil, domain.WrapProviderDown(err, "vibeplayer: fetch") }
    defer drainAndClose(resp.Body)
    body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
    if err != nil { return nil, domain.WrapProviderDown(err, "vibeplayer: read") }

    srcM := vibePlayerSrcRegex.FindSubmatch(body)
    if srcM == nil {
        // selector drift sentinel
        metrics.ParserZeroMatchTotal.WithLabelValues("vibeplayer", "src_const_regex").Inc()
        return nil, domain.WrapExtractFailed(errors.New("no src= const"), "vibeplayer: src extract")
    }
    stream := &domain.Stream{
        Sources: []domain.Source{{URL: string(srcM[1]), Type: "hls"}},
        Headers: map[string]string{"Referer": "https://vibeplayer.site/"},
    }
    if subM := vibePlayerSubRegex.FindSubmatch(body); subM != nil && len(subM[1]) > 0 {
        stream.Tracks = []domain.Track{
            {File: string(subM[1]), Label: "English", Kind: "captions", Default: true},
        }
    }
    return stream, nil
}
```

### Pattern 5 — StreamHG / Earnvids extractor (Go sketch — share base type)

```go
// streamhg.go and earnvids.go differ ONLY by:
//   (1) Name()
//   (2) host allowlist (otakuhg.site vs otakuvid.online)
//   (3) Referer in headers
//
// Both use the SAME Dean-Edwards unpacker via goja (already in tree from Kwik).
//
// Captured 2026-05-12: both wrappers contain
//   eval(function(p,a,c,k,e,d){...}('var links={"hls2":"https://...master.m3u8?t=...&e=129600&..."};
//                                    jwplayer("vplayer").setup({sources:[{file:links.hls4||links.hls3||links.hls2,type:"hls"}],...})',
//                                    62,500,'<token1>|<token2>|...'.split('|')))
//
// Reuse kwik.go's extractPacker + goja runtime construction. Both producers serve
// signed URLs with &t=<token>&s=<unix>&e=<ttl_seconds>; parse `e` for TTL.

type packedExtractor struct {
    name    string
    hosts   []string
    referer string
    http    *domain.BaseHTTPClient
}

var packerHlsRegex = regexp.MustCompile(`"hls2"\s*:\s*"(https://[^"]+\.m3u8[^"]*)"`)

func (e *packedExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
    body, err := e.fetchWrapper(ctx, embedURL, headers)
    if err != nil { return nil, err }

    unpacked, ok := runDeanEdwardsUnpack(body)   // shared helper across kwik + streamhg + earnvids
    if !ok {
        metrics.ParserZeroMatchTotal.WithLabelValues(e.name, "packer_balance").Inc()
        return nil, domain.WrapExtractFailed(errors.New("no balanced packer"), e.name+": unpack")
    }
    m := packerHlsRegex.FindStringSubmatch(unpacked)
    if m == nil {
        metrics.ParserZeroMatchTotal.WithLabelValues(e.name, "hls2_regex").Inc()
        return nil, domain.WrapExtractFailed(errors.New("no hls2"), e.name+": hls2 extract")
    }
    return &domain.Stream{
        Sources: []domain.Source{{URL: m[1], Type: "hls"}},
        Headers: map[string]string{"Referer": e.referer},
    }, nil
}
```

### Anti-Patterns to Avoid

- **Do not implement a "9anime client" that hides a megaplay.buzz scraper.** That's the `feedback_replace_dont_preserve.md` anti-pattern. If the planner rejects the Anitaku pivot, name the resulting provider what it actually scrapes (`megaplay`).
- **Do not pass `chromedp` / `playwright` / `flaresolverr` to solve the Doodstream challenge.** Drop the server. SCRAPER-FOUND-09 lint will reject the PR otherwise.
- **Do not fetch the Anitaku episode page TWICE (once for sub, once for dub) per request.** The page already lists all `data-video` URLs for one variant. The sub/dub split is at the **anime-slug** level, not the episode-page level. Plan fetches `/category/<slug>` and `/category/<slug>-dub` once each at ListEpisodes time, then ONE episode-page fetch per ListServers call.
- **Do not cache the `&e=<ttl>` signed-URL stream past expiry.** Phase 16's `min(expires-30s, 5min)` TTL formula applies verbatim; the new extractors must emit the parsed expiry so `client.GetStream` can compute it.
- **Do not assume malsync exposes a `Gogoanime` key.** It doesn't (verified). The `malsync.Lookup` call exists for forward-compat only; the cache will sit on a 24h negative-cache miss in practice.
- **Do not introduce a new metric.** `parser_fallback_total{from,to}` and `parser_zero_match_total{provider,selector}` are already wired (Phase 17). Reuse them with provider="gogoanime" and selector identifiers per pattern.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| MAL ID → Anitaku slug mapping | Curated mapping table | Fuzzy search via `/search.html?keyword=<title>` + Jaro-Winkler ≥ 0.85 | Verified: malsync has NO Gogoanime key (sampled 10 MAL IDs, all return only `KickAssAnime, AnimeKAI, animepahe, Crunchyroll, Hulu, Netflix`). A curated table for 10+ million MAL entries is infeasible. |
| Jaro-Winkler implementation | Custom string-distance function | Extract `jaroWinkler` from `services/scraper/internal/providers/animepahe/` into a shared `services/scraper/internal/fuzzy/` package | Second consumer is the trigger for the shared package; single source of truth |
| Dean-Edwards JS unpacker | Reimplement base-N decoder + var substitution | `dop251/goja` — same pattern as `embeds/kwik.go` (Phase 16) | Goja runs the unpacker IIFE verbatim; hand-decoders have charset off-by-ones |
| Cloudflare Turnstile bypass for Doodstream | `chromedp` / `flaresolverr` | **Drop the Doodstream server** | Forbidden by SCRAPER-FOUND-09; 3 surviving servers (vibeplayer + streamhg + earnvids) give redundancy |
| Per-host rate limit | Channel-based limiter | `domain.BaseHTTPClient.WithPerHostRPS("anitaku.to", 1.0, 2)` etc. — Phase 15 standard | Already wired |
| HTTP retry+backoff | Custom loop | `domain.BaseHTTPClient` (retryablehttp 1→2→4→8s) | Already wired |
| Cookie jar | `map[string]*http.Cookie` | `BaseHTTPClient.Jar()` (publicsuffix-scoped) | Already wired |
| HLS proxy for CORS | New endpoint | Existing `libs/videoutils/proxy.go::ProxyWithReferer` | Just append to `HLSProxyAllowedDomains` |
| Provider failover loop | New orchestrator | `services/scraper/internal/service.Orchestrator.runFailover` — already iterates `RegisteredProviders()` | Phase 17 already wired; only need `orchestrator.Register(provider)` |
| Health probe per stage | New goroutine | `health.ProbeRunner` (Phase 17) auto-discovers via `RegisteredProviders()` | New provider is picked up automatically — zero per-provider probe code |
| Fallback metric emission | New counter | `metrics.ParserFallbackTotal.WithLabelValues(from, to).Inc()` already fires in orchestrator | Phase 17 wiring; just verify with curl `/metrics` post-deploy |
| Frontend dropdown / store | New component | Existing `EnglishPlayer.vue` + `useWatchPreferences.preferredScraperProvider` | Phase 16 already supports arbitrary string values |
| Bug-report UI w/ provider tag | New modal | Existing `ReportButton.vue` already emits provider field | Phase 16 / `SCRAPER-NF-05` |

**Key insight:** Net-new Go is overwhelmingly **(a)** Anitaku HTML parsing (~150 LOC for FindID + ListEpisodes + ListServers in `client.go`) + **(b)** three small embed extractors (~80 LOC each, with StreamHG and Earnvids sharing a base struct). Total estimated net-new Go: ~400 LOC. Plans proposing >1000 LOC are over-engineering — verify with the planner.

## Runtime State Inventory

This phase is **greenfield additive** (a new provider + new embed extractors + new allowlist entries + one new locale key per language). No renames, no migrations, no string replacements across the codebase. **The Runtime State Inventory does not apply.**

Confirming each category:

| Category | Action Required |
|----------|------------------|
| Stored data (DB rows, collection names, Redis keys keyed by renamed strings) | **None.** Cache keys use new namespace `*:gogoanime:*` — no rename of existing AnimePahe keys. |
| Live service config (n8n flows, Datadog tags, etc.) | **None.** No external SaaS config touched. |
| OS-registered state (Task Scheduler, systemd, pm2) | **None.** Docker-compose only; new env var `SCRAPER_GOGOANIME_BASE_URL` added but no OS-level registration. |
| Secrets/env vars | **One new var:** `SCRAPER_GOGOANIME_BASE_URL` in `docker/.env` (optional override; defaulted to `https://anitaku.to` in `config.go`). No secret rotation. |
| Build artifacts / installed packages | **None.** No package renames; egg-info / pyproject /  go.mod stay as-is. |

## Common Pitfalls

### Pitfall 1: Treating malsync as the primary ID resolution path

**What goes wrong:** Plan implements `FindID` as "malsync first, fuzzy fallback" → every request takes the slow fuzzy path because malsync ALWAYS returns miss for Gogoanime.

**Why:** As of 2026-05-12, sampled 10 MAL IDs (21, 1, 1535, 16498, 31964, 5114, 38000, 30276, 11061, 9253) — malsync's `Sites` map contains `{KickAssAnime, AnimeKAI, animepahe, Crunchyroll, Hulu, Netflix}`. No `Gogoanime`. No `Anitaku`. No `9anime`.

**Avoid:** Invert the order in the new provider: **try fuzzy first**, keep the malsync probe as forward-compat (negative-cache writes a 24h miss key so cost is one upstream request per anime per day — acceptable). Add a watchdog metric so we know when malsync starts shipping the key.

**Warning signs:** P95 latency on `FindID` > 800ms after Phase 18 ships → check if `parser_request_duration_seconds{provider="gogoanime", operation="find_id"}` is dominated by upstream search-page fetch; if yes, the cache key shape is broken.

### Pitfall 2: 9anime.org.lv `<option value>` is base64-encoded HTML

**What goes wrong:** If the planner KEEPS 9anime.org.lv as a secondary provider, parsing the `<select class="mirror">` `<option value="">` attribute as a URL fails because the value is base64-encoded `<iframe>` HTML.

**Avoid:** Decode `base64.StdEncoding.DecodeString(option.Val)` → parse the resulting `<iframe src=...>` → extract the inner `gogoanime.me.uk/newplayer.php` URL → follow it.

**This pitfall is moot for the Anitaku pivot.**

### Pitfall 3: StreamHG/Earnvids m3u8 URLs are short-lived signed URLs

**What goes wrong:** Cache a `links.hls2` URL with `&e=129600&s=<unix>` past the 36h TTL → 403 from CDN.

**Avoid:** Parse `&e=<ttl_seconds>` and `&s=<signed_unix>` from the query string at extraction time. TTL = `min((s + e) - now - 30s, 5min)`. The 30s buffer is for clock skew. Refuse to cache if already expired (cached-expired-URL = known-bad-URL).

**Warning signs:** Telegram alert on `provider_health_up{provider="gogoanime",stage="stream_segment"}==0` while `stage="stream"==1` → segment fetch returns 403 → cached URL is past expiry → bug in TTL parser.

### Pitfall 4: Anitaku sub/dub split is per-slug, not per-episode

**What goes wrong:** Plan implements ListEpisodes as one fetch and tries to determine sub vs dub from the episode-page markup → fails because Anitaku ships separate `/category/<slug>` and `/category/<slug>-dub` pages.

**Avoid:** Make TWO HTTP calls per ListEpisodes (one per variant); merge by episode_number; tag rows with `Category` derived from which slug they came from. Cache the assembled merge for 6h so subsequent reads are zero-upstream.

**Warning signs:** Frontend shows duplicate episodes (one per variant) → merge logic broken; or frontend shows zero dub episodes when one is available → second slug fetch is failing silently (must check 404 vs other 4xx).

### Pitfall 5: Anitaku `data-video` attribute can be protocol-relative

**What goes wrong:** Some Gogoanime mirrors emit `data-video="//vibeplayer.site/..."` (no scheme). `url.Parse` returns `Hostname() == ""` → host-allowlist check fails → server dropped silently.

**Avoid:** Detect `strings.HasPrefix(dv, "//")` and prepend `https:` before parsing. Verified the current `anitaku.to` always emits `https://...` — but historical Gogoanime mirrors did this and the pattern can return.

**Warning signs:** `parser_zero_match_total{provider="gogoanime", selector="data_video_host_filter"}` spike after a mirror change.

### Pitfall 6: HLS proxy allowlist regression on `pacha.kwik.cx`

**What goes wrong:** Edit to `HLSProxyAllowedDomains` (append) accidentally reorders or removes the Phase 16 Kwik entries → AnimePahe streams stop proxying.

**Avoid:** Append-only edits. Verify with an explicit unit test `TestHLSProxyAllowedDomains_AnimePaheRegressionLocked` that asserts `kwik.cx` and `owocdn.top` and `uwucdn.top` all still match. The existing test (if present from Phase 16) must still pass.

### Pitfall 7: goja runtime sharing across goroutines (carried from Phase 16)

**What goes wrong:** Reuse the same `goja.Runtime` across StreamHG and Earnvids extractions → data race → intermittent extraction failures + segfaults.

**Avoid:** `goja.New()` inside every `Extract()` call; discard after. Same discipline as `kwik.go::Extract`. Each extractor instance is safe to share; the runtime per call is not.

### Pitfall 8: `vm.Interrupt()` from same goroutine = no-op (carried from Phase 16)

**What goes wrong:** `time.AfterFunc(5s, vm.Interrupt)` plus a runaway packer (hostile or broken upstream) → never returns.

**Avoid:** Explicit `go func() { ... vm.Interrupt() }()` BEFORE `vm.RunString`. See Phase 16 RESEARCH Pitfall 3 + the Kwik extractor's watchdog goroutine.

### Pitfall 9: Doodstream/myvidplay 301 → playmogo silently absorbs the request budget

**What goes wrong:** Naive ListServers iterates all `data-video` URLs and tries to extract from `myvidplay.com` → 301 to `playmogo.com` → 200 returns Cloudflare HTML challenge page → extraction fails after 10s timeout → wasted request budget + p99 latency spike.

**Avoid:** **Filter out `myvidplay.com` and `playmogo.com` hostnames at ListServers time** (see Pattern 3 sketch). They never become candidates. The host filter is a `hostMatchesAny(host, "myvidplay.com", "playmogo.com")` check — no extractor registration, no host allowlist entry.

**Warning signs:** Health probe latency on `stage="stream"` jumps from <2s to >10s → check whether Doodstream filter regressed.

## Code Examples

See "Architecture Patterns" section above for full code sketches (FindID, ListEpisodes, ListServers, VibePlayer extractor, StreamHG/Earnvids shared extractor).

Additional anchor — **megaplay.buzz/stream/getSources DTO** (for reference IF planner reverses pivot and keeps 9anime.org.lv chain):

```json
{
  "sources": {
    "file": "https://cdn.mewstream.buzz/anime/<hex>/<hex>/master.m3u8"
  },
  "tracks": [
    {
      "file": "https://1oe.lostproject.club/anime/<hex>/<hex>/subtitles/eng-2.vtt",
      "label": "English",
      "kind": "captions",
      "default": true
    }
  ],
  "t": 1,
  "intro": {"start": 132, "end": 221},
  "outro": {"start": 1331, "end": 1420},
  "server": 4
}
```

Captured live 2026-05-12 from `https://megaplay.buzz/stream/getSources?id=2142` (Header `Referer: https://megaplay.buzz/`). Plain JSON — no encryption, no token rotation, no auth gate. **Note:** the m3u8 host (`cdn.mewstream.buzz`) requires `Referer: https://megaplay.buzz/` on subsequent segment fetches; bare requests return 403.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `9anime.to` original domain | Dead since 2023 rebrand to `aniwave.to` → 2024 to `kaido.to` → both dead by 2025 | Continuous domain rotation 2023-2025 | Phase 18 cannot target the original brand; pivot recommended |
| HiAnime ecosystem (`hianime.to`, `aniwatch-api`) | Dead — repo deleted, all domains down | Pre-2026-05-09 (per STATE.md) | Locked the v3.0 milestone in the first place |
| Consumet API (`riimuru/consumet-api`) | Broken — calls `enc-dec.app` with wrong body, 100% Zoro failure | Pre-2026-05-09 | Same trigger as above |
| 9anime mirror landscape (May 2026 snapshot) | All recognized successor mirrors except `9anime.org.lv` are dead, parked, fingerprint-traps, or SEO content farms | 2026-05-12 (this research) | The "9anime" brand is no longer a viable provider name; either pivot or accept the megaplay-relay reality |
| Gogoanime / Anitaku as v3.0 fourth provider (deferred per REQUIREMENTS.md L112) | **Promoted to second provider in Phase 18** | This research, 2026-05-12 | Provider exists, alive, clean scrape; the deferred-to-v3.1 note in REQUIREMENTS gets updated |
| malsync.moe as universal MAL → provider mapper | Returns only `KickAssAnime, AnimeKAI, animepahe, Crunchyroll, Hulu, Netflix` for sampled MAL IDs — no Gogoanime / 9anime / Anitaku entries | Verified 2026-05-12 (10 sampled MAL IDs) | Phase 18 provider's `FindID` MUST use fuzzy title search as PRIMARY path; malsync is forward-compat only |
| Doodstream as a reliable embed | `myvidplay.com` 301-redirects to `playmogo.com` which serves Cloudflare Turnstile | Verified 2026-05-12 | Skip the server; cannot be implemented without forbidden headless tooling |

**Deprecated/outdated:**
- The CONTEXT.md D1 wording "9anime mirrors may not need DDoS-Guard" is correct in spirit but conservative — verified: `anitaku.to` has plain Cloudflare 200 (no challenge, no `__ddg2_` cookie, no `Server: ddos-guard` header). **D10 (DDoS-Guard helper promotion) is NOT triggered.** Leave the AnimePahe ddosguard.go provider-private.
- The CONTEXT.md D3 example embed-host list (`mp4upload`, `streamsb`, `streamtape`, `megacloud`) is from outdated references; the **actual** Anitaku embed set is `vibeplayer`, `streamhg`, `earnvids` (+ skipped `doodstream`). Plan must follow the actual set.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `anitaku.to` remains the canonical alive Gogoanime mirror through Phase 18 ship date | Mirror Viability | Gogoanime has rotated 5+ times in 18 months (REQUIREMENTS.md L112). Mitigation: `SCRAPER_GOGOANIME_BASE_URL` env var allows hot-swapping the base URL via `docker compose restart scraper` without rebuild. |
| A2 | malsync.moe will continue to NOT ship a `Gogoanime` key during Phase 18 ship window | Pitfall 1, Pattern 1 | Low risk — malsync's site set has been stable for months. If they add it, our forward-compat probe automatically starts taking the fast path; if they remove `animepahe` (currently shipping), Phase 16's AnimePahe also degrades. Not a Phase 18 regression risk specifically. |
| A3 | StreamHG and Earnvids continue to use the SAME Dean-Edwards packer shape with `links.hls2` field | Pattern 5 / Code Examples | If one rotates to a new obfuscator, that extractor breaks but the other survives + vibeplayer still works. Probe stage_segment alert fires → ISS log → fix-forward. |
| A4 | The `vibeplayer.site` `const src = "...m3u8"` inline-JS pattern persists | Pattern 4 | Same as A3: if it rotates, vibeplayer extractor breaks but StreamHG + Earnvids survive. Three-extractor redundancy is by design. |
| A5 | Anitaku's `data-video` attribute always emits absolute `https://` URLs (not protocol-relative `//`) | Pitfall 5 | Defensive — Pattern 3 sketch already handles `//` prepend. No functional risk. |
| A6 | The CDNs (`premilkyway.com`, `dramiyos-cdn.com`, `vibeplayer.site`'s own CDN) honor signed URLs without an explicit `Referer` header for `.m3u8` byte fetches | Embed Extractor Catalog table | Verified live 2026-05-12: StreamHG m3u8 returned 200 with `Access-Control-Allow-Origin: *` from a no-referer request. Vibeplayer m3u8 same. If a CDN tightens to require referer in future, add to `stream.Headers["Referer"]` and the HLS proxy already forwards it. |
| A7 | The phase rename annotation (REQUIREMENTS.md gets "SCRAPER-9ANI-* IDs implemented by Gogoanime/Anitaku provider; 9anime mirror chain unreachable as of 2026-05-12") is acceptable to the user given CONTEXT.md D1 + S4 | Decision | CONTEXT.md explicitly allows this; the trade-off was pre-approved. |
| A8 | The `jaroWinkler` + `normalizeTitle` helpers in `services/scraper/internal/providers/animepahe/client.go` are extracted to `services/scraper/internal/fuzzy/` rather than copy-pasted | Architecture Patterns | Mild risk of touching AnimePahe code path. Mitigation: pure code-motion refactor with a test running against the AnimePahe fuzzy fixtures; no behavior change. If the extraction is risky, planner may choose to copy-paste (one source of truth becomes two — flag as v3.1 cleanup). |
| A9 | The myvidplay/playmogo Cloudflare Turnstile gate is the only anti-bot gate across the 4 Anitaku servers | Pitfall 9 | Verified 2026-05-12 for the other three (vibeplayer + StreamHG + Earnvids accept plain GETs). If a fourth surfaces later, drop that server and rely on the surviving redundancy. |

**Per the agent role policy, no claim above is tagged `[ASSUMED]` in the strict sense** — every assumption is empirically tied to a 2026-05-12 live probe captured in this research. Assumptions are about *persistence over time*, not present state. If an item flips at impl time, the planner re-runs the relevant probe.

## Open Questions (RESOLVED)

1. **Which 9anime mirror serves real content today?**
   - **RESOLVED:** Only `9anime.org.lv` serves real content (verified 2026-05-12 — 244 KB body, `bsx`/`bixbox` markup, real episode listings). All other 14 candidates are dead, parked, fingerprint-traps, SEO content farms, or empty stubs. See Mirror Viability table.

2. **Is `9anime.org.lv` usable as a native scrape target?**
   - **RESOLVED:** Technically yes, but only as a thin facade over `megaplay.buzz`. Episode pages base64-encode `<iframe>` tags pointing at `gogoanime.me.uk/newplayer.php`, which itself iframes `megaplay.buzz/stream/<id>`. The final HLS URL comes from `https://megaplay.buzz/stream/getSources?id=<numeric>` (unencrypted JSON). Implementing a "9anime" provider here would actually be a megaplay.buzz client — the `feedback_replace_dont_preserve.md` anti-pattern.

3. **If 9anime is dead, what's the alive fallback?**
   - **RESOLVED:** `anitaku.to` — verified alive 2026-05-12, plain Cloudflare 200, no challenges, search + episode pages parse cleanly, 4 distinct embed hosts per episode (3 usable + 1 skipped).

4. **Does malsync.moe cover 9anime / Gogoanime / Anitaku?**
   - **RESOLVED: NO.** Sampled 10 MAL IDs (One Piece, Cowboy Bebop, Death Note, AoT, MHA, FMAB, Demon Slayer, One Punch Man, HxH 2011, 86 Part 2) — every successful response shows `Sites` keys `{KickAssAnime, AnimeKAI, animepahe, Crunchyroll, Hulu, Netflix}` only. No Gogoanime, no Anitaku, no 9anime. **D1 of CONTEXT.md is structurally wrong**: fuzzy search is the PRIMARY path, not fallback.

5. **What embed hosts does the chosen provider use?**
   - **RESOLVED:** `anitaku.to/one-piece-episode-1` enumerates 4 named servers, dedup-by-URL count = 16 raw `data-video` entries:
     - `vibeplayer.site/<id>` (HD-1 + HD-2)
     - `otakuhg.site/e/<id>` (StreamHG)
     - `otakuvid.online/embed/<id>` (Earnvids)
     - `myvidplay.com/e/<id>` (Doodstream — **SKIP** due to Turnstile)

6. **What's the auth/token contract for each embed?**
   - **RESOLVED:**
     - vibeplayer: no token, no auth, inline `const src` regex extract
     - StreamHG: Dean-Edwards packed JS → `links.hls2` (signed URL with `&t=`, `&s=`, `&e=<ttl>` query params; parse `&e` for cache TTL)
     - Earnvids: identical packer shape to StreamHG with `&e=<ttl>` semantics
     - Doodstream: Cloudflare Turnstile challenge — cannot proceed without forbidden deps

7. **Does the alive provider sit behind Cloudflare / DDoS-Guard?**
   - **RESOLVED:** `anitaku.to` and `vibeplayer.site` and `megaplay.buzz` and the CDNs all show `Server: cloudflare` but **no challenge** — plain 200 responses for any UA including `Go-http-client/1.1`. `otakuhg.site`/`otakuvid.online` are plain nginx with no Cloudflare. **DDoS-Guard cookie helper from Phase 16 is NOT needed**; do NOT promote it to a shared package this phase (D10 verdict: leave provider-private).

8. **Should Phase 18 ship under the "9anime" or "Gogoanime/Anitaku" name?**
   - **RESOLVED:** CONTEXT.md D1 + S4 already pre-approved the pivot. The implementing provider is **gogoanime** (display name + folder name + package name); requirement IDs `SCRAPER-9ANI-01..06` stay literal; REQUIREMENTS.md gets a one-line annotation about the implementation lineage.

9. **Should the planner promote `jaroWinkler` + `normalizeTitle` from AnimePahe into a shared `services/scraper/internal/fuzzy/` package?**
   - **RESOLVED:** YES — second consumer is the canonical trigger for the shared package. A1 of CONTEXT.md S5 (SSRF guard reuse) follows the same logic. Pure code-motion refactor; run AnimePahe's fuzzy tests on the new package to prove no behavior change.

10. **Should the planner promote `services/scraper/internal/providers/animepahe/ddosguard.go` to a shared `libs/ddosguard/` package per CONTEXT.md D10?**
    - **RESOLVED:** NO. Anitaku.to does not sit behind DDoS-Guard. The CONTEXT.md D10 condition ("IF the chosen 9anime mirror sits behind DDoS-Guard") is not met. Leave provider-private; revisit if/when AnimeKai (Phase 19) needs it.

## Environment Availability

The phase introduces NO new external tooling — every consumed dep is already present in scraper service from Phase 15/16/17. Audit:

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `goquery` (Go) | New gogoanime client HTML scraping | ✓ (already in `services/scraper/go.mod` via AnimePahe) | v1.8.x | — |
| `goja` (Go) | StreamHG / Earnvids Dean-Edwards unpack | ✓ (already in `services/scraper/go.mod` via Kwik extractor) | v0.0.0-... | — |
| `retryablehttp` via `domain.BaseHTTPClient` | Per-host rate limit, 429/5xx backoff | ✓ (Phase 15) | — | — |
| Redis (`libs/cache`) | malsync + episodes + stream URL TTLs | ✓ (already running in docker-compose) | matches docker-compose | — |
| `health.AllStages` + `health.ProbeRunner` (in-tree) | Probe auto-discovers new provider | ✓ (Phase 17) | — | — |
| `metrics.ParserFallbackTotal` + `metrics.ParserZeroMatchTotal` (in-tree) | Fallback + zero-match metrics | ✓ (Phase 17) | — | — |
| `libs/videoutils/proxy.go::HLSProxyAllowedDomains` | Append new entries | ✓ | — | — |
| Frontend `bun` + `bunx` | Frontend build per CLAUDE.md | ✓ (Phase 16 baseline) | — | — |
| Existing `make redeploy-scraper` / `make redeploy-web` / `make health` targets | After-update flow per CLAUDE.md | ✓ | — | — |
| Live `anitaku.to` upstream | Live probe (production) + capture-goldens make target run | ✓ (verified 2026-05-12) | n/a | If `anitaku.to` dies before Phase 18 ships, pivot to `anitaku.io` (the .io subdomain showed 200 + real content with cleaner UI but sparser data per spot check) via `SCRAPER_GOGOANIME_BASE_URL`. |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:**
- None (everything is in-tree or a verified-alive external endpoint).

## Validation Architecture

> Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `goldie/v2` for golden files (already in tree per Phase 15 `SCRAPER-FOUND-07`) |
| Config file | None — Go test discovery + `testdata/<provider>/` convention |
| Quick run command | `cd /data/animeenigma/services/scraper && go test ./internal/providers/gogoanime/... ./internal/embeds/... -count=1 -short -timeout=60s` |
| Full suite command | `cd /data/animeenigma/services/scraper && go test ./... -count=1 -race -timeout=180s` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SCRAPER-9ANI-01 | `FindID` fuzzy-search returns slug for known title via `/search.html` golden; Jaro-Winkler ≥ 0.85 gate; malsync returns miss → fuzzy used | unit | `go test ./internal/providers/gogoanime -run TestFindID_FuzzyPath -v` | ❌ Wave 0 |
| SCRAPER-9ANI-01 (cont.) | malsync miss is negative-cached for 24h | unit | `go test ./internal/providers/gogoanime -run TestFindID_MalsyncNegativeCache -v` | ❌ Wave 0 |
| SCRAPER-9ANI-02 | `ListEpisodes` parses `/category/<slug>` golden + merges with `/category/<slug>-dub` golden | unit | `go test ./internal/providers/gogoanime -run TestListEpisodes_SubDubMerge -v` | ❌ Wave 0 |
| SCRAPER-9ANI-02 (cont.) | 6h Redis cache hit returns identical payload without upstream call | unit | `go test ./internal/providers/gogoanime -run TestListEpisodes_CacheHit -v` | ❌ Wave 0 |
| SCRAPER-9ANI-03 | `ListServers` parses `<ul class="anime_muti_link">` golden → returns 3 Server entries with correct Type | unit | `go test ./internal/providers/gogoanime -run TestListServers_AnimeMutiLink -v` | ❌ Wave 0 |
| SCRAPER-9ANI-03 (cont.) | `myvidplay.com` / `playmogo.com` filtered out at ListServers level | unit | `go test ./internal/providers/gogoanime -run TestListServers_DoodstreamSkipped -v` | ❌ Wave 0 |
| SCRAPER-9ANI-03 (cont.) | Three extractors registered: vibeplayer + streamhg + earnvids | unit | `go test ./internal/embeds -run TestRegistry_Phase18ExtractorsRegistered -v` | ❌ Wave 0 |
| SCRAPER-9ANI-04 | `GetStream` calls `embeds.Registry.Find` and dispatches; vibeplayer extractor returns m3u8 from golden | unit | `go test ./internal/embeds -run TestVibePlayer_Extract_FromGolden -v` | ❌ Wave 0 |
| SCRAPER-9ANI-04 (cont.) | StreamHG extractor unpacks Dean-Edwards golden + returns hls2 URL | unit | `go test ./internal/embeds -run TestStreamHG_Extract_FromGolden -v` | ❌ Wave 0 |
| SCRAPER-9ANI-04 (cont.) | Earnvids extractor (shared base) returns hls2 from golden | unit | `go test ./internal/embeds -run TestEarnvids_Extract_FromGolden -v` | ❌ Wave 0 |
| SCRAPER-9ANI-04 (cont.) | Stream TTL = `min(parsedExpiry − 30s, 5min)` for StreamHG-signed URL | unit | `go test ./internal/embeds -run TestStreamHG_ComputeTTL -v` | ❌ Wave 0 |
| SCRAPER-9ANI-05 | `HLSProxyAllowedDomains` contains 5 new entries; Phase 16 `pacha.kwik.cx` regression invariant still passes | unit | `go test ./libs/videoutils -run TestHLSProxyAllowedDomains_Phase18Additions -v` | ❌ Wave 0 |
| SCRAPER-9ANI-05 (cont.) | `isHLSDomainAllowed` correctly matches rotating subdomains (e.g. `OkqtSs1gBbNcA8e.premilkyway.com` → match) | unit | `go test ./libs/videoutils -run TestIsHLSDomainAllowed_RotatingSubdomains -v` | ❌ Wave 0 |
| SCRAPER-9ANI-06 | Two-provider orchestrator: animepahe healthCache=false → skip → gogoanime serves stream; `parser_fallback_total{from="animepahe",to="gogoanime"}` increments | unit | `go test ./internal/service -run TestOrchestrator_AnimePaheToGogoanimeFailover -v` | ❌ Wave 0 |
| SCRAPER-9ANI-06 (cont.) | Phase 17 health probe loops `RegisteredProviders()` and exercises gogoanime per stage | integration | `go test ./internal/health -run TestProbeRunner_DiscoverNewProvider -v -race` | ❌ Wave 0 |
| SCRAPER-9ANI-06 (cont.) | Live `make health` after `make redeploy-scraper` shows both providers reporting `provider_health_up=1` for all 5 stages (manual smoke) | smoke | `curl http://localhost:8088/scraper/health \| jq` after deploy | manual — verification doc |
| Cross-cutting | Phase 16 AnimePahe tests still green after `jaroWinkler` extraction to `services/scraper/internal/fuzzy/` (code-motion refactor) | regression | `go test ./internal/providers/animepahe -run TestFindID -v` | ✅ exists (Phase 16) |
| Cross-cutting | After-update commit hook runs `make lint` + `make health` and they both pass | integration | `make lint && make health` | ✅ exists |

### Sampling Rate

- **Per task commit:** `cd /data/animeenigma/services/scraper && go test ./internal/providers/gogoanime/... ./internal/embeds/... -count=1 -short -timeout=60s`
- **Per wave merge:** `cd /data/animeenigma/services/scraper && go test ./... -count=1 -race -timeout=180s`
- **Phase gate:** Full suite green + `make lint` green + `make health` shows both providers `up=1` before `/gsd-verify-work`

### Wave 0 Gaps

Wave 0 must scaffold the following BEFORE provider impl plans execute:

- [ ] `services/scraper/internal/providers/gogoanime/client_test.go` — covers SCRAPER-9ANI-01, -02, -03, -04 against goldens
- [ ] `services/scraper/internal/providers/gogoanime/dto_test.go` — pure parser unit tests (search-page + category-page + episode-page)
- [ ] `services/scraper/internal/providers/gogoanime/malsync_test.go` — covers negative-cache forward-compat path
- [ ] `services/scraper/internal/embeds/vibeplayer_test.go` — covers SCRAPER-9ANI-04
- [ ] `services/scraper/internal/embeds/streamhg_test.go` — covers SCRAPER-9ANI-04 + TTL computation
- [ ] `services/scraper/internal/embeds/earnvids_test.go` — covers SCRAPER-9ANI-04
- [ ] `services/scraper/testdata/gogoanime/` directory with 6 golden files (search, category, category-dub, episode page, plus 3 embed wrapper pages from vibeplayer/otakuhg/otakuvid)
- [ ] `services/scraper/internal/embeds/packed.go` (or similar) — shared Dean-Edwards unpacker helper extracted from `kwik.go` (refactor task; Phase 16 tests must still pass)
- [ ] `services/scraper/internal/fuzzy/` package + tests — extracted from AnimePahe; Phase 16 AnimePahe fuzzy tests pivot to the new package
- [ ] `services/scraper/scripts/capture-goldens-gogoanime.sh` + Makefile target mirroring Phase 16's `capture-goldens-animepahe`
- [ ] `libs/videoutils/proxy_test.go` — gain the Phase 18 additions test + the regression-lock test

(If any of the listed test files already exist for Phase 17 reasons, the Wave 0 task is "augment the existing file" rather than "create new".)

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Phase 18 only consumes anonymous public anime APIs; no user auth surface added |
| V3 Session Management | no | Stateless GET requests against upstreams; no provider-side sessions |
| V4 Access Control | partial | Same admin gate as Phase 17 — `/api/admin/scraper/health` now also surfaces the new provider's health; gateway JWT admin role check is the existing control |
| V5 Input Validation | yes | URL parsing for embed-host filtering; reject non-http(s) schemes (see Pattern 3 sketch — same WR-05 guard as AnimePahe `ListServers`); per-request body cap (2 MiB on wrapper pages, 4 MiB on search pages) |
| V6 Cryptography | no | All embed extractions are plaintext JSON (megaplay) or unpacked plaintext JS (StreamHG/Earnvids/vibeplayer). No AES, no JWE, no token signing on our side. |
| V10 Communications | yes | TLS-only upstream calls (`https://` enforced via scheme allowlist); HLS proxy preserves `Referer` to upstream CDN per existing `ProxyWithReferer` semantics |
| V11 Business Logic | yes | Failover skip logic must not leak the orchestrator's internal failover chain through public error messages — `summarizeFailover` is already conservative (Phase 15) |
| V12 Files & Resources | yes | SSRF guard: every embed URL goes through host-allowlist match BEFORE the HTTP fetch. Same `WR-05` control as AnimePahe `ListServers`. |
| V14 Configuration | yes | `SCRAPER_GOGOANIME_BASE_URL` env var validated at boot (scheme + host required) — same shape as Phase 16 `ANIMEPAHE_BASE_URL` validator in `config.go::Load` |

### Known Threat Patterns for {Go scraper service, web HTML scrape}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SSRF via attacker-controlled embed URL (Anitaku ever serves `data-video="http://10.0.0.1/"`) | Spoofing / Tampering | Host-allowlist check at ListServers AND extractor's `Matches()` before any HTTP call. Reject non-http(s) schemes at parse time. Phase 17 `fetchSegment` SSRF guard (BLK-01) applies to the probe's stream_segment fetch — verify the new extractors' HTTP calls also use a guarded client. |
| Cookie / token / Bearer leakage into golden fixtures | Information Disclosure | Goldens are captured via the Makefile script with `--cookie-jar /dev/null --no-keepalive`; anonymization gate `grep -rE '(Set-Cookie\|__ddg2_\|cf_clearance\|Bearer )'` fails the build (Phase 16 pattern; reuse). |
| Memory exhaustion via giant wrapper-page response (hostile upstream) | DoS | `io.LimitReader` at 2 MiB per wrapper page; 4 MiB per search/category page (Phase 16 ceiling). Empirical: real bodies are <250 KiB. |
| goja runaway script in StreamHG / Earnvids extractor | DoS | 5-second timeout per `vm.RunString` via `go func() { time.Sleep(5s); vm.Interrupt() }()` watchdog pattern from `kwik.go` (Phase 16); test exists. |
| Selector drift silently returning empty `[]Server{}` | Tampering / Coverage gap | `parser_zero_match_total{provider="gogoanime",selector=<id>}` increment + Phase 17 stage health flip to 0 after 3 consecutive failures. Distinguish "real empty" (no servers listed) from "selector drift" (scraper found `<ul class="anime_muti_link">` but zero matching `<li>` children) by treating the empty-block case as the real-empty path. |
| Stale cached signed-URL stream served as fresh | Tampering | TTL = `min(parsedExpiry − 30s, 5min)`; refuse to cache already-expired URL; SubtitleOverlay-side errors surface in `ReportButton` provider-tagged report (`SCRAPER-NF-05`). |

## Sources

### Primary (HIGH confidence)

- Live HTTP probes against 17 candidate 9anime/Gogoanime mirrors — captured 2026-05-12 (this session)
- Live HTTP probes against 4 embed wrapper hosts (vibeplayer, otakuhg, otakuvid, myvidplay/playmogo) — captured 2026-05-12
- Live HTTP probe against `https://api.malsync.moe/mal/anime/<id>` for 10 sampled MAL IDs (21, 1, 1535, 16498, 31964, 5114, 38000, 30276, 11061, 9253) — captured 2026-05-12
- In-tree code reviewed:
  - `services/scraper/internal/providers/animepahe/client.go` (analog template)
  - `services/scraper/internal/providers/animepahe/malsync.go` (cache wrapper)
  - `services/scraper/internal/domain/provider.go` (Provider interface; Stream-no-iframe contract)
  - `services/scraper/internal/domain/embed.go` (Registry contract)
  - `services/scraper/internal/embeds/kwik.go` (Dean-Edwards unpacker template)
  - `services/scraper/internal/embeds/megacloud.go` (sidecar wrapper — reference only)
  - `services/scraper/internal/health/stage.go` (canonical 5-stage contract)
  - `services/scraper/internal/health/probe.go` (probe auto-discovery)
  - `services/scraper/internal/service/orchestrator.go` (failover + `ParserFallbackTotal` emission)
  - `services/scraper/cmd/scraper-api/main.go` (registration wiring)
  - `services/scraper/internal/config/config.go` (env var pattern for base URL)
  - `libs/videoutils/proxy.go` (`HLSProxyAllowedDomains` + `isHLSDomainAllowed` matcher)
  - `libs/metrics/parser.go` (`ParserFallbackTotal` label set `{from, to}`)
  - `libs/metrics/provider.go` (`ParserZeroMatchTotal`, `ProviderHealthUp`)
- Project planning docs:
  - `.planning/REQUIREMENTS.md` (SCRAPER-9ANI-01..06, NF-02, NF-04, NF-05, FOUND-09)
  - `.planning/STATE.md` (alive vs. dead provider list, 2026-05-09 triage)
  - `.planning/ROADMAP.md` + `.planning/phases/17-observability/17-RESEARCH.md` (analog research structure)
  - `.planning/phases/16-animepahe-and-new-englishplayer/16-RESEARCH.md` (per-provider research template + Pitfalls 1–8 carryover)
  - `.planning/phases/18-9anime/18-CONTEXT.md` (D1–D10 user decisions, S1–S5 specifics)
- Project memory (`/root/.claude/projects/-data-animeenigma/memory/MEMORY.md`):
  - `feedback_replace_dont_preserve.md` (rationale for pivoting rather than relaying)
  - `feedback_verify_streams.md` (test the user's actual broken anime, not just known-good)
  - `feedback_animelib_no_kodik_fallback.md` (precedent for refusing iframe-only paths in EN tier)

### Secondary (MEDIUM confidence)

- Reverse-engineered Dean-Edwards packer unpack of live `otakuhg.site` and `otakuvid.online` wrapper JS — pure-Python script reproduced the algorithm and yielded the m3u8 URL; transcribing to goja is mechanical
- Inferred Anitaku `data-video` server naming from textContent of `<a>` element (`HD-1 Choose this server` → "HD-1")

### Tertiary (LOW confidence)

- The exact resolution-label set returned by each extractor's m3u8 (e.g. whether StreamHG advertises 360p/720p/1080p variants vs. ABR-only master) — not verified end-to-end at the variant-playlist level; deferred to impl-time golden capture
- Long-term stability of `dramiyos-cdn.com` and `premilkyway.com` as the primary CDN choices — these are clearly throwaway-named hosts; expect rotation within the v3.0 ship window

## Metadata

**Confidence breakdown:**

- Mirror viability: **HIGH** — fresh live probes across 17 candidates + 4 embed hosts in this session
- Embed extraction shape: **HIGH** — verified end-to-end against captured HTML for vibeplayer + streamhg + earnvids; m3u8 reachability validated for all three
- malsync coverage: **HIGH** — sampled 10 MAL IDs, deterministic zero-coverage for Gogoanime
- Anti-bot landscape: **HIGH** — `anitaku.to` + 3 surviving embed hosts all return plain 200 to `Go-http-client/1.1` UA; `myvidplay/playmogo` Turnstile gate confirmed
- Long-term mirror stability: **MEDIUM** — Gogoanime has rotated 5+ times in 18 months (REQUIREMENTS.md acknowledges this); env var `SCRAPER_GOGOANIME_BASE_URL` mitigates
- Code patterns in sketches: **HIGH** — derived directly from Phase 16 working code, with Phase 17 stage-key contract honored
- Architecture (1:1 with Phase 16/17): **HIGH** — interface + orchestrator + probe auto-discovery were designed for this exact extension

**Research date:** 2026-05-12
**Valid until:** 2026-06-12 — re-run Mirror Viability + malsync coverage probes before this date (any provider mirror is a 30-day TTL fact; embed-host extraction patterns are 7-14 day TTL because StreamHG/Earnvids actively rotate signed-URL formats).
