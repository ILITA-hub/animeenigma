# Phase 27: AnimePahe Revival via Stealth-Chromium Sidecar — Research

**Researched:** 2026-05-19
**Domain:** Headless-Chromium scraping sidecar + Go parser rewrite + Docker wiring
**Confidence:** HIGH on parser rewrite + Docker patterns; MEDIUM on stealth-plugin maintenance reality (it's a known issue, but viable today); HIGH on AnimePahe API contract (live probe + multiple community references corroborate).

## Summary

Phase 27 stands up a Node.js stealth-Chromium sidecar (`animepahe-resolver`) at `services/animepahe-resolver/` that DDoS-Guard-solves on `animepahe.pw` and proxies search / release / play fetches through a thin HTTP API on port 3000. The Go parser at `services/scraper/internal/providers/animepahe/` is rewritten to call the sidecar over HTTP instead of fetching upstream directly, while preserving its caching, malsync, fuzzy-fallback, and metrics behavior. The play page chain (search → animeSession → release → episodeSession → play HTML → Kwik) is a one-for-one substitution of the current Go HTTP layer with sidecar calls — Kwik extraction in `services/scraper/internal/embeds/kwik.go` is left entirely alone.

The two material risks for the phase are (a) maintaining `puppeteer-extra-plugin-stealth` against DDoS-Guard rotations — the plugin has not been npm-updated since 2.11.2 ~3 years ago but is still the most-used stealth layer, and the pin-plus-refresh-procedure (D6) is the right operating posture — and (b) keeping Chromium under the 500 MB hard cap, which requires `--disable-dev-shm-usage`, optionally `--single-process`, `--js-flags="--max-old-space-size=384"`, and a page-recycle policy at N=100 requests so leaked DOM/Listener state can't accumulate.

**Primary recommendation:** Build a Fastify Node 20 sidecar (`ghcr.io/puppeteer/puppeteer:24` base image, swap to bundled Chrome for Testing); pin `puppeteer@24.x` + `puppeteer-extra@3.3.6` + `puppeteer-extra-plugin-stealth@2.11.2` (exact, no caret) with `package-lock.json` committed; ship `STEALTH-PINS.md` doc; rewrite the Go parser as a thin HTTP client preserving FindID/ListEpisodes/ListServers/GetStream/HealthCheck shapes; cap mem_limit at 500m + healthcheck on `/healthz`; gate the SCRAPER_DEGRADED_PROVIDERS edit on a live Frieren curl pass.

## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D1 — Stealth-Chromium sidecar (Option A)**. NOT one-shot cookie warmup, NOT skip-to-AllAnime. Operator pick 2026-05-19.
- **D2 — Sidecar uses `animepahe.pw` exclusively** (not `.ru`, not `.si`, not `.io`). `.ru`/`.si` are TCP-blackholed; `.io` adds FingerprintJS burden. Single base URL per resolver.
- **D3 — API contract migration is independently required**. Numeric MAL id → UUID `session`. The parser's current `m=release&id=<numeric-MAL-id>` 404s today; rewrite uses `m=release&id=<session-UUID>` returned by `m=search`.
- **D4 — Test goldens captured fresh during Plan 27-02**, not from `/tmp/pup/` probe artifacts.
- **D5 — Memory budget 500 MB resident is a HARD ship gate**, not a follow-up. If steady-state RSS over 100 sequential requests exceeds 500 MB, page-recycle policy is added BEFORE ship.
- **D6 — Stealth plugin pin** in `package.json` exact-versioned + `STEALTH-PINS.md` doc + maintenance-bot Pattern 7 animepahe branch.
- **D7 — Removing `animepahe` from `SCRAPER_DEGRADED_PROVIDERS` is the LAST step**, gated on live curl pipeline pass AND `make logs-scraper | grep animepahe` showing no continuous 403/timeout in the first 10 minutes after redeploy.
- **D8 — Reserved-future-phase stubs (VibePlayer, MinIO) demoted from numbered to unnumbered** in ROADMAP. Phase 27 slot is this phase.

### Claude's Discretion

- HTTP framework for sidecar (Express vs. Fastify vs. native `http`). **Research recommendation: Fastify** for built-in JSON schema validation, structured Pino logging, and 2-4x throughput over Express. ([Stackwise 2026 benchmark][fastify-bench])
- Log format (JSON vs. text). **Recommendation: JSON** for ingestion into the existing Loki/Promtail stack.
- Sidecar Prometheus `/metrics` (recommended yes, not strictly required for ship).
- Concurrency model — single page vs. pool. **Recommendation: single page with serial fetch dispatch.** A pool size 2-3 was suggested but each additional page leaks 50-100 MB; sticking to one page keeps headroom for the 500 MB budget. EN-tab traffic is ~1 req/s peak; serial dispatch is sufficient.
- Chromium `--single-process` vs. multi-process. **Recommendation: test both during 27-01; default to `--single-process`** for lower memory ceiling at the cost of marginal stability. Multi-process is the fallback if `--single-process` crashes under sustained load.
- MalSync cache invalidation on `m=release` 404 (probably yes — a stale animeSession is the likely cause).

### Deferred Ideas (OUT OF SCOPE)

- Auto-refreshing the stealth plugin pin on DDoS-Guard rotation (Phase 27 ships static pins + doc; maintenance-bot auto-refresh is a separate future phase).
- Multi-domain resolver (`.io` + `.pw` failover, FingerprintJS bypass).
- Resolver reuse for other gated providers (Cloudflare Turnstile, hCaptcha).
- gogoanime anitaku → anineko migration (separate phase).
- AllAnime lift (Phase 26-01 — runs in parallel).
- AnimeKai escape-hatch fill-in (Phase 26-06).
- VibePlayer Recovery via WARP egress (unnumbered future idea).
- MinIO Hot Archival (unnumbered future idea, v3.2 scope).
- Health-aware EN tab hiding (deferred from Phase 24, not re-opened).

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SCRAPER-HEAL-29 | Sidecar service scaffold + stealth-Chromium warm context + HTTP API (`/healthz`, `/search`, `/release`, `/play`) | "Standard Stack" + "Architecture Patterns" — Fastify + puppeteer-extra@3.3.6 + stealth@2.11.2 with single warmed page reused across in-page `fetch()` calls |
| SCRAPER-HEAL-30 | Go parser rewrite + UUID session API contract migration | "Reverse-Engineering — Current Parser Audit" — preserves FindID/ListEpisodes/ListServers/GetStream/HealthCheck signatures; switches transport from `getWithDDoSGuard` to `resolverClient.Get(...)`; deletes `ddosguard.go`; keeps `cache.go` + `malsync.go` + `dto.go` (existing DTO shape already matches new contract) |
| SCRAPER-HEAL-31 | Docker compose wiring + healthcheck dependency + 500 MB mem_limit | "Docker Patterns" — `mem_limit: 500m`, `cpus: 0.5`, `init: true`, `shm_size: 256m`, `seccomp:unconfined` for Chrome sandbox, healthcheck on `/healthz` with browser-aliveness probe |
| SCRAPER-HEAL-32 | End-to-end gate-clear + Phase 24 verdict-log update | "Validation Architecture" — re-run `docs/issues/scraper-provider-verification-2026-05-19.md` curl pipeline against Frieren, append post-ship section flipping animepahe row from FAIL to PASS |
| SCRAPER-HEAL-33 | `SCRAPER_DEGRADED_PROVIDERS` compose default cleanup + post-ship redeploy | "Project Constraints (from CLAUDE.md)" — change `${SCRAPER_DEGRADED_PROVIDERS:-gogoanime,animepahe}` to `${SCRAPER_DEGRADED_PROVIDERS:-gogoanime}`; gated on D7 |

## Project Constraints (from CLAUDE.md)

These are extracted from `./CLAUDE.md` and constrain planning. The planner MUST verify compliance for each.

- **Self-hosted, no CDN.** No CDN config, no Cloudflare KV, no edge functions. The sidecar runs as a docker-compose service in the same network.
- **Go services follow `services/{name}/` layout** with `cmd/{name}-api/main.go` + `internal/{config,domain,handler,service,repo,parser,transport}/` + `migrations/` + `Dockerfile` + `go.mod`. The sidecar is a Node service, not Go; it does NOT follow this layout (it's `services/animepahe-resolver/` with `server.js` + `Dockerfile` + `package.json` + `package-lock.json` + `STEALTH-PINS.md` — same shape as `docker/megacloud-extractor/` precedent).
- **Naming.** Packages lowercase; files snake_case; types PascalCase; variables camelCase. The sidecar uses JS conventions internally, which is fine.
- **Error handling** via shared `libs/errors` (Go side only). The Go parser MUST use `domain.WrapProviderDown` / `domain.WrapNotFound` / `domain.WrapExtractFailed` for the existing sentinel-error contract.
- **Database** via `libs/database` + GORM. Not applicable here (sidecar is in-memory; parser's persistence is Redis only).
- **Caching** via `libs/cache` with documented TTL strategies. The new parser MUST preserve the existing 6h episode cache (cache key rekeyed from MAL id to animeSession), 24h MalSync positive + negative cache, and `min(expires-30s, 5min)` stream cache from `cache.go`.
- **Logging** via `libs/logger` structured `Infow`/`Errorw`/`Warnw`. Sidecar uses Pino (JSON) for consistency with Loki ingestion.
- **Don't Do list:**
  - Don't pre-populate database. ✓ Not relevant.
  - Don't store video files. ✓ Sidecar never stores video.
  - Don't cache video URLs > 1 hour. ✓ Existing `cache.go` enforces `min(expires-30s, 5min)` — preserved.
  - Don't fight GORM. ✓ Not relevant.
- **Test user `ui_audit_bot`** — not directly relevant to Phase 27 but the curl pipeline in Phase 24's verdict log uses `f0b40660-6627-4a59-8dcf-7ec8596b3623` (Frieren). After-update changelog Russian/Trump-mode entries.
- **Service ports table** — scraper is 8088 + `/metrics`. Sidecar takes port `3000` internal-only (no `ports:` mapping; reachable only via the docker-compose network).
- **Gateway routing** — `/api/anime/*` → catalog:8081 → catalog calls scraper:8088 → scraper calls animepahe-resolver:3000 (new hop). No gateway change.
- **`make redeploy-scraper`** is the redeploy command for scraper. New `make redeploy-animepahe-resolver` target needed; pattern is mechanical extension of `Makefile`.
- **After-Update Skill mandatory:** Plan 27-05 must invoke `/animeenigma-after-update` (lint + build + redeploy + changelog + commit + push). Changelog entries are Russian "Trump mode" with emoji `🎉/🔧/⚡`, ≤180 chars each.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| DDoS-Guard challenge solving | **Sidecar (animepahe-resolver, Node + Chromium)** | — | Requires a JS runtime + DOM; cannot be done in Go. Single point of CSP-aware cookie warmup. |
| HTTP fetch to `animepahe.pw` | **Sidecar** | — | All upstream calls go through the warmed browser context to keep DDoS-Guard cookies alive. The Go parser MUST NOT call `animepahe.pw` directly after this phase. |
| UUID session → episode list translation | **Go parser** | — | Pure data shaping. The sidecar returns verbatim upstream JSON; the parser normalizes to `domain.Episode`. |
| MalSync MAL → animeSession resolution | **Go parser (via `malsync.go`)** | Sidecar via `/search` fallback | Existing 24h pos/neg cache stays in Go; sidecar is only called on miss. |
| HLS Kwik URL extraction | **Go parser → `services/scraper/internal/embeds/kwik.go` (goja)** | — | Untouched. Sidecar returns the play-page HTML; the parser scrapes `button[data-src]` (already in `client.go`) and hands the Kwik URL to the in-process goja extractor. |
| Redis caching (episodes / stream / malsync) | **Go parser** | — | Existing `cache.go` + Redis backend untouched. Cache key rekeyed from MAL id to animeSession. |
| Healthcheck (browser + HTTP server) | **Sidecar `/healthz`** | Docker `healthcheck:` block | Two-layer: HTTP server liveness + browser process aliveness (probe via in-browser `await page.evaluate(() => 1)`). |
| Compose orchestration (depends_on, healthcheck) | **Docker Compose** | — | `scraper` service grows `depends_on: animepahe-resolver: condition: service_healthy`. |
| Provider failover registration | **`services/scraper/cmd/scraper-api/main.go`** | — | Unchanged behavior. `SCRAPER_DEGRADED_PROVIDERS` env-default edit is the final step. |
| Observability metrics | **Both** | — | Sidecar exposes Prometheus `/metrics` (challenge-solve, page-recycle, upstream-403 counts). Go parser keeps existing `parser_*` metrics. |

**Sanity check:** The Go parser never touches `animepahe.pw` directly post-Phase-27. If a future task introduces a direct HTTP call to `animepahe.pw` from Go, it bypasses the sidecar's DDoS-Guard cookies and will 403 — that's a tier violation the plan-checker should catch.

## Standard Stack

### Sidecar (`services/animepahe-resolver/`)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `puppeteer` | `^24.0.0` (verified latest as of 2026-05-19: 25.0.4 published 2026-05-18 [VERIFIED: `npm view puppeteer version`]) | Browser automation | Official Google Chrome team package; bundled Chrome for Testing. |
| `puppeteer-extra` | `3.3.6` exact pin (last published 3 years ago [VERIFIED: `npm view puppeteer-extra version`]) | Plugin host | The only viable plugin entry point for stealth; npm activity is low but ecosystem still treats it as canonical (5k+ stars on the repo). |
| `puppeteer-extra-plugin-stealth` | `2.11.2` exact pin (last published 3 years ago [VERIFIED: `npm view puppeteer-extra-plugin-stealth version`]) | DDoS-Guard + FingerprintJS overrides | Most-used stealth layer per ZenRows + ScreenshotEngine 2026 reviews; maintenance is "low cadence but works" — historical issues opened in 2020 still relevant. [CITED: [berstend/puppeteer-extra issues #236][stealth-issue236]] |
| `fastify` | `^4.x` (latest 4.28.x as of mid-2025) | HTTP server | 2-4x throughput vs. Express; built-in JSON schema validation + Pino logger; recommended for new Node services in 2026 per Stackwise/MGSoftware. [CITED: [Stackwise Express-vs-Fastify 2026][fastify-bench]] |
| `pino` | `^9.x` | Structured JSON logging | Fastify default; ingests cleanly into existing Loki/Promtail stack. |
| `prom-client` | `^15.x` | Prometheus metrics | Standard Node Prometheus client; mirrors what `libs/metrics` does on the Go side. |

**Installation** (in `services/animepahe-resolver/package.json` — exact pins for `puppeteer-extra*`, caret for everything else):

```json
{
  "name": "animepahe-resolver",
  "version": "1.0.0",
  "private": true,
  "dependencies": {
    "puppeteer": "^24.0.0",
    "puppeteer-extra": "3.3.6",
    "puppeteer-extra-plugin-stealth": "2.11.2",
    "fastify": "^4.28.0",
    "pino": "^9.5.0",
    "prom-client": "^15.1.3"
  },
  "engines": { "node": ">=20.0.0" }
}
```

`package-lock.json` **MUST** be committed. The plan's Docker build copies it to `node_modules` deterministically — without it, `npm install` resolves transitive deps at build time and the stealth plugin's transitive `puppeteer-extra-plugin-user-preferences` etc. can drift, defeating the pin.

### Go-side (parser rewrite at `services/scraper/internal/providers/animepahe/`)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `services/scraper/internal/domain.BaseHTTPClient` | existing | HTTP transport to sidecar | Reuses per-host rate limiter, retry, structured logging from Phase 15 SCRAPER-FOUND-06. |
| `libs/cache` (Redis) | existing | Episodes / malsync / stream caches | TTLs preserved: 6h episodes, 24h malsync pos/neg, `min(expires-30s, 5min)` stream. |
| `libs/logger` | existing | Structured logging | Preserved. |
| `libs/metrics` | existing | `parser_*` metrics | Preserved. Add `parser_resolver_request_total{provider="animepahe",outcome}` and `parser_resolver_request_duration_seconds` for sidecar-call observability (informational; do NOT block on PR review since metric churn is not a hard ship gate). |
| `github.com/PuerkitoBio/goquery` | existing | Play-page HTML parse | Untouched. `client.go` already extracts `button[data-src]` correctly; only the page-fetch transport changes. |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `puppeteer-extra-plugin-stealth` | `rebrowser-puppeteer` (drop-in patches puppeteer source itself rather than runtime overrides) | Rebrowser's author announced no-further-updates as of Feb 2026 per BrightData blog. Documented incompatibility with stealth-plugin on remote-browser deployments. **Stealth plugin remains the safer ecosystem bet** despite stale npm; community use and DDoS-Guard track record is established. [CITED: [BrightData puppeteer-real-browser guide][brightdata]] |
| Fastify | Express 4.x | Express has 16x weekly downloads (92M vs. 5.8M) and is the safe default. **Fastify wins for new code** in 2026 per multiple framework comparisons. Both work; Fastify's JSON schema validation eliminates a class of bugs at API boundary. |
| `ghcr.io/puppeteer/puppeteer:24` base image | `node:20-alpine` + apt-install Chromium | Alpine Chromium versions are pinned to Alpine repo cadence (last seen Chromium 100-ish at Alpine 3.18); puppeteer @24 needs Chromium 130+. **Use the official puppeteer image** — comes preconfigured with Chrome for Testing + dbus + correct font set. [CITED: [pptr.dev/guides/docker][pptr-docker]] |
| `--single-process` Chromium | Multi-process | Single-process is lower memory ceiling (saves ~80-150 MB) but historically less stable. **Test both during Plan 27-01**; default to single-process and fall back to multi-process if any crash observed during the 100-sequential-request memory probe. |
| In-page `await page.evaluate(() => fetch(url))` | Native puppeteer `page.goto(url)` for every fetch | `page.evaluate(fetch())` keeps DDoS-Guard cookies attached to the browsing context naturally (same-origin policy makes cookies available to `fetch`). `page.goto()` for every request would re-trigger DDoS-Guard checks. **Use in-page `fetch`** with the navigation step only on cold-start or 403-retry. ([Cloudflare puppeteer docs][cf-puppeteer] confirm cookie persistence within a session.) |

**Version verification commands:**

```bash
npm view puppeteer version              # 25.0.4 (2026-05-18) — use ^24 for 24.x stability
npm view puppeteer-extra version        # 3.3.6 (3 years ago — known stale, pinned)
npm view puppeteer-extra-plugin-stealth version  # 2.11.2 (3 years ago — known stale, pinned)
npm view fastify version                # ~4.28 — use ^4.28.0
docker pull ghcr.io/puppeteer/puppeteer:24    # pin major to 24 for predictability
```

## Architecture Patterns

### System Architecture Diagram

```
                                       +-------------------+
                                       |  animepahe.pw     |
                                       |  (DDoS-Guard)     |
                                       +---------^---------+
                                                 | HTTPS (in-page fetch)
                                                 |
                                       +---------+---------+
                                       | animepahe-resolver|
                                       |  (Node 20 +       |
                                       |   Fastify +       |
                                       |   puppeteer +     |
                                       |   stealth plugin) |
                                       |  port :3000       |
                                       +---------^---------+
                                                 | HTTP /search /release /play /healthz /metrics
                                                 |
+----------+      +----------+     +--------+    |     +-------------------+
| Browser  +----->+ Gateway  +---->+Catalog +--->+---->+ Scraper service    |
| (Anime.  |      | :8000    |     | :8081  |    |     |  (Go, port :8088) |
|  vue)    |      +----------+     +--------+    |     |                   |
+----------+                                     |     |  animepahe.Provider|
                                                 |     |  (calls resolver)  |
                                                 |     |                   |
                                                 |     |  + Kwik extractor  |
                                                 |     |    (goja, in-proc) |
                                                 +<----+                   |
                                                       +---------+--------+
                                                                 |
                                                                 v
                                                       +-------------------+
                                                       |   Redis cache     |
                                                       | (episodes,        |
                                                       |  malsync, stream) |
                                                       +-------------------+
```

**Trace (search → play, the canonical use case):**

1. Browser hits `GET /api/anime/{uuid}/scraper/episodes?prefer=animepahe` via Gateway.
2. Catalog resolves UUID → MAL id, forwards to `scraper:8088/scraper/episodes?mal_id=<MAL>&prefer=animepahe&title=<title>`.
3. Scraper's orchestrator picks `animepahe` provider. The provider tries MalSync (24h Redis cache) for `mal_id → animeSession`. On miss, calls `animepahe-resolver:3000/search?q=<title>`.
4. Resolver does (cold start) `page.goto('https://animepahe.pw/')` to warm DDoS-Guard cookies, then `await page.evaluate(async (url) => (await fetch(url)).text(), '...api?m=search&q=...')`. Returns verbatim upstream JSON.
5. Provider picks best fuzzy match → `animeSession`. Caches in MalSync (24h positive).
6. Provider calls resolver `/release?session=<animeSession>&page=1`, paginates until `current_page >= last_page` (cap 50 pages).
7. Provider normalizes each episode → `domain.Episode{ID: episodeSession, Number, Title, IsFiller}`. Caches assembled list (6h).
8. Scraper returns episodes to catalog → gateway → browser.

**Trace (servers + stream — when user picks an episode):**

9. Browser hits `GET /api/anime/{uuid}/scraper/servers?episode=<episodeSession>&prefer=animepahe`.
10. Provider calls resolver `/play?animeSession=<sess>&episodeSession=<sess>`. Resolver returns the play-page HTML verbatim.
11. Provider scrapes `<button data-src="..." data-audio="..." data-resolution="...">` via goquery. Each Kwik URL becomes a `domain.Server{ID: kwikURL, Name: "kwik", Type: sub|dub}`.
12. Browser picks a server → `GET /scraper/stream?server=<kwikURL>...`.
13. Provider hands `kwikURL` to the in-process Kwik extractor (`services/scraper/internal/embeds/kwik.go`). Goja unpacks the Dean-Edwards-packed JS, extracts the signed `.m3u8`.
14. Provider returns `domain.Stream{Sources: [{URL: m3u8, Type: "hls"}], Headers: {Referer: "https://kwik.cx/"}}`. Caches with TTL `min(expires-30s, 5min)`.

### Recommended Project Structure (sidecar)

```
services/animepahe-resolver/
├── server.js              # Fastify entry: registers /healthz /search /release /play /metrics
├── browser.js             # Singleton: launches Chromium + applies stealth plugin + manages warm page
├── upstream.js            # In-page fetch wrapper + 403 retry + page-recycle policy
├── metrics.js             # prom-client registry: stealth_challenge_failures_total etc.
├── package.json           # Exact-pinned puppeteer-extra* + caret-pinned everything else
├── package-lock.json      # MUST be committed for reproducible Docker builds
├── Dockerfile             # FROM ghcr.io/puppeteer/puppeteer:24
├── STEALTH-PINS.md        # Pin versions + last-tested-against date + refresh procedure
└── README.md              # Operator-facing: how to refresh pins, how to run locally
```

### Pattern 1: Warm Browser, Single Page

**What:** Launch Chromium once at process start; navigate to `https://animepahe.pw/` once to collect DDoS-Guard cookies; reuse the same `page` object for all subsequent in-page `fetch()` calls.

**When to use:** Sidecar startup + every request.

**Example** (sketch — capture as a golden for testing during 27-02):

```javascript
// Source: synthesis of puppeteer docs + ScreenshotEngine 2026 stealth guide
const puppeteer = require('puppeteer-extra');
const Stealth = require('puppeteer-extra-plugin-stealth');
puppeteer.use(Stealth());

let browser, page;
async function init() {
  browser = await puppeteer.launch({
    headless: 'new',
    args: [
      '--no-sandbox',                  // required in container (see Pitfall 1)
      '--disable-setuid-sandbox',
      '--disable-dev-shm-usage',       // critical: /dev/shm default 64MB causes crashes
      '--disable-gpu',
      '--single-process',              // optional: lower memory; test stability
      '--js-flags=--max-old-space-size=384',
      '--disable-extensions',
      '--disable-background-networking',
    ],
  });
  page = await browser.newPage();
  await page.setUserAgent('Mozilla/5.0 ... Chrome/130...');  // matched to bundled Chrome
  await page.goto('https://animepahe.pw/', { waitUntil: 'networkidle2', timeout: 30000 });
}

async function fetchUpstream(url) {
  return await page.evaluate(async (u) => {
    const r = await fetch(u, { credentials: 'include' });
    return { status: r.status, body: await r.text() };
  }, url);
}
```

### Pattern 2: 403 Retry with Re-Navigation

**What:** On a 403 from upstream (DDoS-Guard cookie expired or rotated), re-navigate the warm page to `https://animepahe.pw/` to refresh cookies, then retry the in-page fetch ONCE. On second 403, surface 502 to the caller.

**When to use:** Every upstream fetch.

**Example:**

```javascript
async function fetchWithRetry(url) {
  let { status, body } = await fetchUpstream(url);
  if (status === 403) {
    await page.goto('https://animepahe.pw/', { waitUntil: 'networkidle2', timeout: 30000 });
    challengeSolveCounter.inc();
    ({ status, body } = await fetchUpstream(url));
  }
  if (status === 403) {
    challengeFailureCounter.inc();
    throw new ResolverError(502, 'stealth_challenge_failed');
  }
  return { status, body };
}
```

### Pattern 3: Page Recycle on Memory Pressure

**What:** After every N=100 in-page fetches, close the current `page` and create a fresh `page` (NOT a fresh browser — tab-level recycle is sufficient). Re-run `page.goto` to refresh cookies on the new page.

**When to use:** Every Nth fetch, or on RSS-exceeds-450 MB sentinel.

**Why:** DOM/listener state in the warm page accumulates ~1-2 MB per request. Without recycling, 1000 requests = ~1-2 GB of phantom DOM nodes. Closing the tab releases this.

**Example:**

```javascript
let requestCount = 0;
const PAGE_RECYCLE_AT = 100;
async function maybeRecycle() {
  if (++requestCount % PAGE_RECYCLE_AT !== 0) return;
  pageRecycleCounter.inc();
  const old = page;
  page = await browser.newPage();
  await page.setUserAgent('...');
  await page.goto('https://animepahe.pw/', { waitUntil: 'networkidle2' });
  await old.close();
}
```

### Pattern 4: Two-Layer Healthcheck

**What:** `/healthz` returns 200 only if BOTH (a) the Fastify HTTP server is responsive AND (b) the browser process is alive and the warm page responds to `await page.evaluate(() => 1)` within a 2s budget.

**When to use:** Docker compose `healthcheck:` block. The standard pattern of "HTTP server responds" misses the case where the Node process is fine but Chromium has crashed silently.

**Example:**

```javascript
fastify.get('/healthz', async (req, reply) => {
  if (!browser || !page) return reply.code(503).send({ browser: 'down', reason: 'not_initialized' });
  try {
    const probe = await Promise.race([
      page.evaluate(() => 1),
      new Promise((_, rej) => setTimeout(() => rej(new Error('probe_timeout')), 2000)),
    ]);
    if (probe === 1) {
      return reply.send({ browser: 'up', lastChallengeSolveAt, pageCount: requestCount });
    }
    return reply.code(503).send({ browser: 'down', reason: 'probe_returned_unexpected' });
  } catch (e) {
    return reply.code(503).send({ browser: 'down', reason: e.message });
  }
});
```

### Anti-Patterns to Avoid

- **Calling `page.goto(upstreamURL)` for every request.** This re-triggers DDoS-Guard's challenge each time (because each `goto` is a new navigation). Use in-page `fetch` instead.
- **Launching a new `browser` per request.** Chromium launch is ~1-2 seconds and ~150 MB transient. Reuse the singleton.
- **Skipping `package-lock.json`.** Without it, `npm install` in the Docker build resolves transitive `puppeteer-extra-plugin-*` deps from npm at build time; the stealth plugin pin becomes meaningless.
- **Hand-rolling DDoS-Guard cookie management in the Go parser** (what the current code does in `ddosguard.go`). Delete this file — it's load-bearing-zero post-Phase-27.
- **Caching stream URLs in the sidecar.** The sidecar is stateless except for the warm page; the parser owns Redis caching.
- **Adding `--disable-web-security` to Chromium flags.** Breaks `credentials: 'include'` semantics in same-origin `fetch`, defeating the cookie persistence trick.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| DDoS-Guard challenge solver | A Go regex+goja simulation of the challenge JS (which `ddosguard.go` partially attempts) | `puppeteer-extra-plugin-stealth` in the sidecar | DDoS-Guard rotates its challenge JS every few weeks/months; a Go impl is a maintenance treadmill. |
| Headless Chromium image | `node:20-alpine` + `apk add chromium` + apt-install font packages | `ghcr.io/puppeteer/puppeteer:24` | Official image bundles Chrome for Testing, dbus, fonts, sandbox userns. Alpine path requires Chromium version matching to puppeteer version, plus apk has older Chromium. |
| Browser sandbox under Docker | A custom seccomp profile + capability juggling | `--no-sandbox` + `--disable-setuid-sandbox` + accept it (network-internal sidecar, no untrusted JS from users) | The sidecar consumes only `animepahe.pw` upstream HTML — there is no untrusted user JS. The 2026 "always use sandbox" advice ([puppeteer.guide][puppeteer-sandbox]) applies to general puppeteer use; an internal-only scraper running a single trusted upstream is the documented exception case. |
| HTTP retry / rate limit logic in the sidecar | A hand-rolled retry loop in `upstream.js` | Reuse the Go-side `BaseHTTPClient` retry path (it already retries on 5xx + 429) | The sidecar should be a thin wrapper; retry/rate-limit policy belongs to the consumer (the Go parser), not the sidecar. |
| Browser memory monitoring | A child-process watchdog reading `/proc/<pid>/status` | Docker `mem_limit: 500m` + healthcheck OOM-restart | Docker enforces the budget at the cgroup level; OOMs are picked up by `restart: unless-stopped`. No watchdog code required. |
| Fastify schema validation | Hand-written `if (!query.session) return 400` | Fastify's built-in JSON schema route validation | Each route declares `{ schema: { querystring: { ... } } }` and 400s are auto-generated. |
| Prometheus metrics in Node | A custom `/metrics` endpoint that string-concats | `prom-client` registry | Standard, used everywhere; mirrors the Go-side `libs/metrics` shape. |

**Key insight:** The sidecar is a transport layer. Everything that isn't "load this URL through warmed Chromium and return the response verbatim" belongs in the Go parser (which already has the cache, retry, fuzzy-match, malsync, observability code working).

## Reverse-Engineering — Current Parser Audit

What does `services/scraper/internal/providers/animepahe/client.go` DO today that the rewrite must preserve?

### Public methods (must keep signatures)

| Method | Signature | Behavior to preserve |
|--------|-----------|----------------------|
| `Name()` | `string` | Returns `"animepahe"` (stable, used as Prometheus + orchestrator key). **Unchanged.** |
| `FindID(ctx, AnimeRef)` | `(string, error)` | (1) MalSync lookup (24h pos + neg cache via `malsync.go`); (2) on miss, fuzzy `/api?m=search` with Jaro-Winkler ≥ 0.85 (`fuzzy.NormalizeTitle` + `fuzzy.JaroWinkler`). **Rewrite the transport** (call resolver `/search` instead of `getWithDDoSGuard`); **keep the matching logic verbatim.** |
| `ListEpisodes(ctx, providerID)` | `([]Episode, error)` | (1) Redis cache hit (6h); (2) paginate `m=release` until `current_page >= last_page`, cap 50 pages; (3) `parser_zero_match_total{selector="episode_list_item"}.Inc()` on first-page empty result. **Rewrite the transport** (call resolver `/release` instead of direct fetch); **keep the pagination loop, cache, and metric.** Cache key rekeyed from `episodes:animepahe:<malID>` to `episodes:animepahe:<animeSession>` (CONTEXT.md D5 says the existing `cache.go` "keep but rekey on animeSession" — confirm by reading test fixture; existing keys already animeSession-style since FindID already returns session, not MAL id). |
| `ListServers(ctx, providerID, episodeID)` | `([]Server, error)` | Scrape `/play/{providerID}/{episodeID}` HTML for `<button data-src="...">`, filter to kwik.cx/kwik.si hosts, derive `Type: CategorySub|CategoryDub` from `data-audio`. **Rewrite the transport** (call resolver `/play` instead of direct fetch); **keep the goquery + filter logic verbatim.** |
| `GetStream(ctx, providerID, episodeID, serverID, category)` | `(*Stream, error)` | Hand Kwik URL to `embeds.Registry.Find().Extract()` (goja-based in-process unpacker). Cache `min(expires-30s, 5min)`. **Completely unchanged** — sidecar is not involved in this path. |
| `HealthCheck(ctx)` | `Health` | Returns in-memory snapshot of 4 stage healths (`search`, `episodes`, `servers`, `stream`). **Unchanged** — each method already calls `markStage` correctly. |

### Logic NOT directly replaceable (must be preserved in the rewrite)

- **Cache key generation** (`cache.go::computeStreamTTL`, the episode cache key, the stream cache key shape) — kept verbatim.
- **`fuzzy.JaroWinkler` ≥ 0.85 threshold** — kept verbatim. Tested by `client_test.go::TestProvider_FindID_FuzzyFallback`.
- **`parser_zero_match_total` counter increment** on first-page empty release — kept; this is the metric the maintenance-bot uses to detect DTO drift.
- **`markStage()` calls on every method entry-success and entry-failure** — kept; feeds the Phase 17 probe runner.
- **Selector constants `selectorEpisodeListItem`, `selectorServerLink`, `selectorKwikPackedJS`** — keep names so the maintenance-bot's auto-edit-selectors workflow (Pattern 7) still finds them as named constants.
- **`Deps` struct validation in `New()`** (errors when HTTP/Embeds/MalSync/Cache nil) — kept verbatim per WR-11.
- **WR-05 path-traversal scheme check** in `ListServers` (rejects non-http/https Kwik URLs) — kept.
- **WR-06 `category` parameter informational** in `GetStream` — kept (sub/dub already filtered at `ListServers` time).

### Test infrastructure (reuse, don't rewrite)

| File | Reuse strategy |
|------|---------------|
| `client_test.go` (788 LOC) | **Reuse**. Replace `httptest.NewServer` mocking of `animepahe.ru` with `httptest.NewServer` mocking of the resolver's `/search`, `/release`, `/play` endpoints. The fixture-loading helper `loadFixture` is unchanged. |
| `cache_test.go` (81 LOC) | **Unchanged** — pure unit tests on `computeStreamTTL`. |
| `malsync_test.go` (294 LOC) | **Unchanged** — `malsync.go` is preserved intact (CONTEXT.md: "keep as path optimization"). |
| `dto_test.go` (104 LOC) | **Reuse**. The DTO shapes (`epDTO`, `releaseResponse`, `searchResponse`) already match the new upstream wire format (verified from `release_4_p1.json` which has `session`, `audio: "jpn"`, `disc: ""`, etc.). No DTO struct field changes are required — the bug today is upstream URL construction, not parsing. |
| `ddosguard_test.go` (120 LOC) | **DELETE** alongside `ddosguard.go`. |
| `testdata/animepahe/search_naruto.json` | **Reuse** as the existing search shape. Plan 27-02 ADDS `frieren-search.json`, `frieren-release.json`, `frieren-play.html` captured fresh from the live resolver. |

### New testdata to capture during 27-02 (D4)

- `testdata/animepahe/frieren-search.json` — `/search?q=Frieren` resolver response (passthrough of upstream `m=search`).
- `testdata/animepahe/frieren-release.json` — `/release?session=<animeSession>` resolver response.
- `testdata/animepahe/frieren-play.html` — `/play?...` resolver response (raw HTML of the play page).
- `testdata/animepahe/frieren-stream-kwik-1080p.txt` — captured signed `.m3u8` URL with `expires=` (for `computeStreamTTL` regression).

## Common Pitfalls

### Pitfall 1: `--no-sandbox` opens an attacker-controlled-JS escape if any untrusted upstream is added later

**What goes wrong:** Today the sidecar only navigates to `animepahe.pw`, which is "trusted enough" (we already serve its video). A future maintainer adds a different upstream that serves malicious JS; that JS gains arbitrary syscalls in the container.

**Why it happens:** `--no-sandbox` disables Chrome's seccomp + setuid sandbox. The container's seccomp is `unconfined` for Chrome to launch at all (CLAUDE.md infrastructure context).

**How to avoid:**
- Document in `server.js` header comment: "This sidecar is HARDCODED to `animepahe.pw`. Adding a second upstream requires either (a) sandbox re-enablement with a Docker capability grant `SYS_ADMIN` or (b) explicit security review for the new domain."
- In `upstream.js`, validate that the requested URL's hostname is `animepahe.pw` (host equality) before `page.evaluate(fetch())`. Reject all other hosts with 400. This is defense-in-depth.

**Warning signs:** Any PR adding a second-domain `page.goto` or fetch in the sidecar. Plan-checker should block.

### Pitfall 2: `package-lock.json` not committed → stealth plugin pin meaningless

**What goes wrong:** `puppeteer-extra@3.3.6` and `puppeteer-extra-plugin-stealth@2.11.2` are exact-pinned in `package.json`, but their transitive deps (`puppeteer-extra-plugin-user-preferences`, `puppeteer-extra-plugin-user-data-dir`, etc.) are not. Without a lockfile, `npm install` at Docker build time resolves these transitively from npm — and any one of them shipping a breaking change defeats the pin.

**Why it happens:** Default `.gitignore` templates often exclude `*-lock.json`.

**How to avoid:**
- `services/animepahe-resolver/.gitignore` MUST exclude `node_modules/` but MUST NOT exclude `package-lock.json`.
- The Dockerfile MUST `COPY package.json package-lock.json ./` and run `npm ci` (NOT `npm install`). `npm ci` fails if the lockfile is stale, which is the desired behavior — it forces an explicit pin update.
- `STEALTH-PINS.md` documents the lockfile-commit policy.

**Warning signs:** Dockerfile uses `npm install` instead of `npm ci`. Lockfile not in the initial commit.

### Pitfall 3: Sidecar hangs (vs. crashes) → healthcheck must detect

**What goes wrong:** The Node process is alive (Fastify still responds to `/healthz` HTTP). Chromium is hung (kernel-level deadlock, or stuck on `page.goto` with no timeout). Compose sees `/healthz` returning 200 and the service stays "healthy"; the scraper hangs on every request.

**Why it happens:** Default `/healthz` checks "can Node respond to HTTP?" which is a weaker signal than "can Chromium actually run JS?".

**How to avoid:**
- Implement the two-layer healthcheck (Pattern 4 above): `/healthz` must `await page.evaluate(() => 1)` with a 2s budget. If the evaluate times out, return 503.
- Docker `healthcheck:` block: `interval: 30s`, `timeout: 10s`, `retries: 3`, `start_period: 20s` — so after 90s of hung browser, compose marks unhealthy and `depends_on` makes the scraper skip animepahe (falls through to next provider).

**Warning signs:** Sidecar logs go silent but `/healthz` HTTP still returns 200. Browser-eval probe time creeps over 500ms in normal operation.

### Pitfall 4: Page-recycle transient memory spike pushes over 500 MB

**What goes wrong:** Recycling the warm page (Pattern 3) involves briefly holding TWO pages: the old one (closing) and the new one (warmed). Old page can hold 150-200 MB; new page warmup adds 150 MB. Combined spike: 300-400 MB on top of the resident Chromium baseline (~200-250 MB). Total: ~500-650 MB. The 500 MB hard cap (D5) is at risk.

**Why it happens:** Naive recycle implementations don't await the close before opening the new tab.

**How to avoid:**
- Implementation order: `const old = page; page = await browser.newPage(); await page.goto(...); await old.close();` — note the new page is fully initialized BEFORE the old close, but the warmup overhead of the new page is the spike risk.
- Alternative: `await old.close(); page = await browser.newPage(); await page.goto(...);` — this avoids overlap but has a small window where no page exists (the next request 502s). The accept-503-during-recycle path is cleaner.
- Run the 100-sequential-request memory probe with `docker stats animepahe-resolver` snapshot every 5s. If RSS peaks over 450 MB, switch to the close-first strategy AND lower recycle threshold from N=100 to N=50.

**Warning signs:** `docker stats` shows RSS spike approaching 500 MB during the steady-state probe. OOMKilled events in `docker logs --details animepahe-resolver`.

### Pitfall 5: DDoS-Guard cookie scope (path vs. domain) drops on `fetch` to a subpath

**What goes wrong:** DDoS-Guard sometimes sets the `__ddg2_*` cookie with `Path=/`, sometimes with `Path=/api`. An in-page `fetch('/api?m=...')` is OK; but a different-path `fetch` may not include the cookie. The challenge re-fires, the response is the challenge JS body, the parser sees broken JSON.

**Why it happens:** `fetch` follows browser-standard cookie scoping. If the cookie was set with a narrower path, only requests under that path send it.

**How to avoid:**
- Always navigate to `https://animepahe.pw/` (NOT `https://animepahe.pw/api`) so the warmup request receives cookies for the broadest scope. The pw site's challenge cookie is set on the root path for `animepahe.pw/`.
- Confirm during 27-02 by inspecting `await page.cookies('https://animepahe.pw/api')` and verifying `__ddg2_*` is present.
- If a `Path=/` cookie scope is not granted, fall back to `await page.goto(url)` for each upstream request (slower, but cookie-safe).

**Warning signs:** First few requests succeed, then 403 storm in the middle of a session. Loki logs from the sidecar show "challenge_solve" events firing every other request.

### Pitfall 6: Cookie state across page recycle

**What goes wrong:** Closing the warm page MAY (depending on puppeteer version + `page.context()` semantics) drop the cookie jar. The new page starts cold; first `fetch` re-triggers DDoS-Guard.

**Why it happens:** `BrowserContext` owns cookies, not the individual `Page`. Closing a tab does NOT clear context-level cookies. BUT if a maintainer accidentally uses `browser.createIncognitoBrowserContext()` for the new page, it gets its own jar.

**How to avoid:**
- Use the DEFAULT browser context throughout: `browser.newPage()`, NOT `browser.createIncognitoBrowserContext().newPage()`.
- Sanity-test in 27-02: capture cookies BEFORE recycle and AFTER recycle; assert `__ddg2_*` is preserved.
- Document this invariant in `browser.js`.

**Warning signs:** `stealth_challenge_failures_total` increments at every page recycle.

### Pitfall 7: Stealth plugin defeated by new DDoS-Guard challenge variant

**What goes wrong:** DDoS-Guard ships a new challenge that probes for a property `stealth@2.11.2` doesn't override. Every request returns 403 forever.

**Why it happens:** Plugin maintenance cadence is low (2.11.2 has been the latest for 3+ years). DDoS-Guard's roadmap is private; cadence of their new checks is unpredictable.

**How to avoid:**
- Pre-defined operating procedure: when `stealth_challenge_failures_total` exceeds a threshold for > 1 hour, the maintenance-bot's Pattern 7 prompt (updated in 27-01 with an animepahe-resolver branch) escalates to a human with a recommendation to (a) `npm install puppeteer-extra-plugin-stealth@latest puppeteer-extra@latest` + test against Frieren, OR (b) re-add `animepahe` to `SCRAPER_DEGRADED_PROVIDERS` if the upgrade also fails.
- `STEALTH-PINS.md` includes the exact refresh command.

**Warning signs:** `stealth_challenge_failures_total` metric spikes; Grafana alert fires.

### Pitfall 8: `npm ci` slowness in Docker build

**What goes wrong:** `puppeteer` package install also downloads Chrome for Testing (~150 MB), so an unoptimized Dockerfile re-downloads it on every build.

**Why it happens:** Default puppeteer install behavior.

**How to avoid:**
- Use `ghcr.io/puppeteer/puppeteer:24` as base — Chrome is pre-bundled.
- Set env `PUPPETEER_SKIP_DOWNLOAD=true` in the Dockerfile (NOT just `PUPPETEER_SKIP_CHROMIUM_DOWNLOAD` — that's deprecated).
- Set `PUPPETEER_EXECUTABLE_PATH=/usr/bin/google-chrome-stable` (path inside the puppeteer image) so `puppeteer.launch()` uses the pre-bundled Chrome.

**Warning signs:** Docker build takes > 90s; build logs show Chromium download.

## Code Examples

### Sidecar entry (Fastify + healthcheck)

```javascript
// Source: synthesis of fastify docs (https://fastify.dev/docs/) + Pattern 4 above
// services/animepahe-resolver/server.js
const fastify = require('fastify')({ logger: { level: 'info' } });
const { initBrowser, fetchWithRetry, getMetrics } = require('./upstream');
const { register } = require('./metrics');

await initBrowser();

fastify.get('/healthz', async (req, reply) => { /* Pattern 4 */ });

fastify.get('/search', {
  schema: { querystring: { type: 'object', required: ['q'], properties: { q: { type: 'string' } } } },
}, async (req) => {
  const { body } = await fetchWithRetry(`https://animepahe.pw/api?m=search&q=${encodeURIComponent(req.query.q)}`);
  return JSON.parse(body);
});

fastify.get('/release', {
  schema: { querystring: { type: 'object', required: ['session'], properties: { session: { type: 'string' }, page: { type: 'integer', default: 1 } } } },
}, async (req) => {
  const url = `https://animepahe.pw/api?m=release&id=${req.query.session}&sort=episode_asc&page=${req.query.page}`;
  const { body } = await fetchWithRetry(url);
  return JSON.parse(body);
});

fastify.get('/play', {
  schema: { querystring: { type: 'object', required: ['animeSession', 'episodeSession'], properties: { animeSession: { type: 'string' }, episodeSession: { type: 'string' } } } },
}, async (req, reply) => {
  const url = `https://animepahe.pw/play/${req.query.animeSession}/${req.query.episodeSession}`;
  const { body } = await fetchWithRetry(url);
  reply.type('text/html').send(body);
});

fastify.get('/metrics', async (req, reply) => {
  reply.type(register.contentType).send(await register.metrics());
});

fastify.listen({ port: 3000, host: '0.0.0.0' });
```

### Go-side resolver HTTP client

```go
// Source: synthesis of services/scraper/internal/domain/BaseHTTPClient + existing client.go patterns
// services/scraper/internal/providers/animepahe/resolver.go (NEW FILE)
package animepahe

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"

    "github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

type resolverClient struct {
    baseURL string
    http    *domain.BaseHTTPClient
}

func newResolverClient(baseURL string, hc *domain.BaseHTTPClient) *resolverClient {
    return &resolverClient{baseURL: baseURL, http: hc}
}

func (r *resolverClient) Search(ctx context.Context, q string) (*searchResponse, error) {
    u := fmt.Sprintf("%s/search?q=%s", r.baseURL, url.QueryEscape(q))
    var sr searchResponse
    if err := r.getJSON(ctx, u, &sr); err != nil {
        return nil, err
    }
    return &sr, nil
}

func (r *resolverClient) Release(ctx context.Context, animeSession string, page int) (*releaseResponse, error) {
    u := fmt.Sprintf("%s/release?session=%s&page=%d", r.baseURL, url.QueryEscape(animeSession), page)
    var rr releaseResponse
    if err := r.getJSON(ctx, u, &rr); err != nil {
        return nil, err
    }
    return &rr, nil
}

func (r *resolverClient) Play(ctx context.Context, animeSession, episodeSession string) (string, error) {
    u := fmt.Sprintf("%s/play?animeSession=%s&episodeSession=%s",
        r.baseURL, url.QueryEscape(animeSession), url.QueryEscape(episodeSession))
    resp, err := r.http.Get(ctx, u)
    if err != nil {
        return "", domain.WrapProviderDown(err, "animepahe-resolver: /play fetch")
    }
    defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
    switch resp.StatusCode {
    case http.StatusOK:
    case http.StatusNotFound:
        return "", domain.WrapNotFound(nil, "animepahe-resolver: /play 404")
    case http.StatusBadGateway:
        return "", domain.WrapProviderDown(fmt.Errorf("resolver 502"), "animepahe-resolver: stealth challenge un-solvable")
    default:
        return "", domain.WrapProviderDown(fmt.Errorf("status %d", resp.StatusCode), "animepahe-resolver: /play non-200")
    }
    body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyHTML))
    if err != nil {
        return "", domain.WrapProviderDown(err, "animepahe-resolver: /play read body")
    }
    return string(body), nil
}

func (r *resolverClient) getJSON(ctx context.Context, urlStr string, out interface{}) error {
    resp, err := r.http.Get(ctx, urlStr)
    if err != nil {
        return domain.WrapProviderDown(err, "animepahe-resolver: fetch")
    }
    defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
    if resp.StatusCode != http.StatusOK {
        return domain.WrapProviderDown(fmt.Errorf("status %d", resp.StatusCode), "animepahe-resolver: non-200")
    }
    body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyAPI))
    if err != nil {
        return domain.WrapProviderDown(err, "animepahe-resolver: read body")
    }
    return json.Unmarshal(body, out)
}
```

### Compose service block

```yaml
# Source: synthesis of docker/docker-compose.yml megacloud-extractor block + 500 MB budget
# docker/docker-compose.yml (additions)
animepahe-resolver:
  build:
    context: ..
    dockerfile: services/animepahe-resolver/Dockerfile
  container_name: animeenigma-animepahe-resolver
  restart: unless-stopped
  init: true                  # ensures Chromium subprocess cleanup on SIGTERM
  shm_size: 256m              # paired with --disable-dev-shm-usage; Chrome still wants some shm
  security_opt:
    - seccomp:unconfined      # required for Chrome's internal seccomp/userns; documented exception
  mem_limit: 500m             # HARD ship gate per CONTEXT.md D5
  cpus: "0.5"
  environment:
    NODE_ENV: production
    LOG_LEVEL: info
    UPSTREAM_BASE_URL: https://animepahe.pw
  healthcheck:
    test: ["CMD", "wget", "-q", "--spider", "http://localhost:3000/healthz"]
    interval: 30s
    timeout: 10s
    retries: 3
    start_period: 20s

scraper:
  # ... existing block ...
  environment:
    # ... existing env ...
    SCRAPER_ANIMEPAHE_RESOLVER_URL: http://animepahe-resolver:3000
    # Phase 27 D7: after gate-clear, this default changes from "gogoanime,animepahe" to "gogoanime"
    SCRAPER_DEGRADED_PROVIDERS: ${SCRAPER_DEGRADED_PROVIDERS:-gogoanime}
  depends_on:
    megacloud-extractor:
      condition: service_healthy
    redis:
      condition: service_healthy
    animepahe-resolver:
      condition: service_healthy
```

### Dockerfile sketch

```dockerfile
# Source: synthesis of pptr.dev/guides/docker + Pitfall 8
# services/animepahe-resolver/Dockerfile
FROM ghcr.io/puppeteer/puppeteer:24

ENV PUPPETEER_SKIP_DOWNLOAD=true \
    PUPPETEER_EXECUTABLE_PATH=/usr/bin/google-chrome-stable

WORKDIR /app

# Copy lockfile first for layer caching
COPY services/animepahe-resolver/package.json services/animepahe-resolver/package-lock.json ./

# npm ci (NOT npm install) — fails on stale lockfile, which is the desired pin enforcement
RUN npm ci --omit=dev

COPY services/animepahe-resolver/*.js ./

EXPOSE 3000

CMD ["node", "server.js"]
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Go-side DDoS-Guard cookie jar handshake (`ddosguard.go`) | Sidecar handles DDoS-Guard via stealth plugin | 2026-05-19 (this phase) | `ddosguard.go` + `ddosguard_test.go` deleted; the Go parser never sees a 403 from upstream. |
| `m=release&id=<numeric-MAL-id>` | `m=release&id=<animeSession-UUID>` | Live probe 2026-05-19 | Upstream changed the contract; numeric MAL id now 404s. New flow: search → animeSession → release. |
| `animepahe.ru` as primary | `animepahe.pw` as primary | 2026-05-19 | `.ru`/`.si` TCP-blackholed from server egress + WARP. `.pw` is the only cleared path. |
| Per-host `cookiejar` in Go via `BaseHTTPClient.Jar()` for animepahe | Cookie jar lives in browser context inside sidecar | This phase | Jar accessor in `BaseHTTPClient` is still used by other providers; only animepahe stops using it. |

**Deprecated/outdated:**
- `services/scraper/internal/providers/animepahe/ddosguard.go` — superseded by sidecar.
- `ANIMEPAHE_BASE_URL=https://animepahe.ru` compose default — superseded by sidecar's hardcoded `animepahe.pw`. The env var on the scraper service becomes a vestigial; recommended action is to remove it from the compose block in 27-03 (or leave it as a documented no-op pointing at the resolver URL — Claude's discretion).
- `puppeteer-extra-plugin-stealth` is the "best available" but is known-low-maintenance (3 years since 2.11.2). Pinning is the current accepted operating posture.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The `epDTO` struct's existing fields (`Session`, `EpisodeNumber`, `Title`, `Filler`, `CreatedAt`) match the new upstream response shape exactly, so no DTO rewrites are needed. | "Reverse-Engineering" + dto.go inspection | [VERIFIED via testdata/animepahe/release_4_p1.json + live probe shape in CONTEXT.md "Specific Ideas"]. Both have `session`, `audio`, `disc`, `episode`, `title`, `filler`, `created_at`. Risk near-zero. |
| A2 | The `searchEntry` struct's fields (`ID`, `Session`, `Title`, `Type`, `Episodes`, `Status`, `Season`, `Year`, `Score`, `Poster`) match the live probe shape. | "Reverse-Engineering" + dto.go | [VERIFIED via testdata/animepahe/search_naruto.json + live probe in CONTEXT.md]. Match. |
| A3 | The play page response is still HTML with `<button data-src="kwik.cx|kwik.si/...">`, NOT a JSON API. | WebFetch of community parser `animepahe-dl.sh` | [VERIFIED via [animepahe-dl.sh source][animepahe-dl] grep pattern + existing testdata/animepahe/play_session_ep1.html]. Match — the play page is plain HTML with button[data-src]. |
| A4 | `puppeteer-extra-plugin-stealth@2.11.2` still defeats current DDoS-Guard challenges on animepahe.pw. | Live probe per CONTEXT.md | [VERIFIED via operator's `/tmp/pup/probe2.js` 2026-05-19 probe result documented in CONTEXT.md "Specific Ideas"]. Hold for current week; cadence of DDoS-Guard rotation is months — re-verify in Plan 27-02 with fresh probe. |
| A5 | The Go parser's existing 6h episode cache key `episodes:animepahe:<providerID>` already uses the animeSession (because FindID already returns the session — verified in client.go:316). | "Reverse-Engineering" | [VERIFIED via reading client.go]. `FindID` returns `best.session` (the UUID), and ListEpisodes is called with that providerID. The cache is already keyed correctly; the CONTEXT.md "rekey on animeSession" instruction is misleading — no change needed. |
| A6 | Chromium with `--disable-dev-shm-usage` + `--no-sandbox` + `--single-process` + `--js-flags=--max-old-space-size=384` stays under 500 MB RSS for steady-state traffic of ~1 req/s. | Multiple Chromium memory tuning guides (webscraping.ai, restackio) | [ASSUMED] — actual ceiling depends on Linux kernel version, glibc, and the specific build of Chrome for Testing in the puppeteer image. MUST be verified empirically during Plan 27-01 via `docker stats` over a 100-request soak. |
| A7 | EN-tab traffic in production is ~1 req/s peak. | CONTEXT.md "Expected upstream rate limits" | [ASSUMED] — based on operator's domain knowledge of self-hosted user base size. Affects whether single-page-serial is sufficient or a page-pool is required. Re-evaluate post-ship via Loki query on resolver request rate. |
| A8 | The `--single-process` Chromium flag is stable enough for sustained scraping (~100 req/hour) in the puppeteer:24 image. | Webscraping.ai + Medium articles on Chromium memory tuning | [ASSUMED] — `--single-process` has historically been considered "use with caution"; some sites have reported segfaults under load. Mitigation: fall back to multi-process if Plan 27-01's soak test sees any crash. |
| A9 | The Go parser's MalSync cache can be invalidated on `m=release` 404 by deleting the `malsync:<malID>:animepahe` key after a single 404 (no need for negative-counter / threshold). | CONTEXT.md "MalSync cache poisoning" + Claude's Discretion item | [ASSUMED] — a stale animeSession is the most likely cause of a 404, so single-strike invalidation is sufficient. Risk: legitimate transient 404 (upstream blip) wipes a good cache entry, forcing a search re-do on the next request (~500ms overhead). Acceptable. |
| A10 | The puppeteer:24 base image's preinstalled Chrome version matches the bundled-Chrome-for-Testing version that `puppeteer@^24.0.0` expects. | pptr.dev docs | [CITED: [pptr.dev/guides/docker][pptr-docker]]. Official image documented as "Puppeteer-pinned" — risk low. |
| A11 | `seccomp:unconfined` is acceptable in this docker-compose stack. | CLAUDE.md infrastructure context + existing megacloud-extractor block | [VERIFIED via docker-compose.yml]. The existing megacloud-extractor service runs Chromium under similar terms; precedent established. |

**Items needing operator confirmation before execute:** A6, A7, A8 (empirical, measured during Plan 27-01); A9 (cache invalidation strategy — could go either way, default to single-strike unless operator prefers a counter). Plans should NOT lock decisions until the 27-01 memory probe results land.

## Open Questions

1. **Should the Go-side `BaseHTTPClient` for animepahe drop the per-host rate limits (kwik.cx, animepahe.ru, animepahe.com, api.malsync.moe) now that the sidecar owns the upstream rate?**
   - What we know: `main.go:93-98` registers these rate limits on the animepahe `BaseHTTPClient`. Post-rewrite, the only outbound host is `animepahe-resolver:3000` (internal docker network).
   - What's unclear: whether keeping the historical rate limits as documentation of the historical hosts is useful, or if they should be cleaned up.
   - Recommendation: drop animepahe.ru/.com from the rate-limit list; keep kwik.cx and api.malsync.moe (still used by Kwik extractor and MalSync); add `animepahe-resolver` at 5 RPS (sidecar can handle higher than the upstream).

2. **Should the play page response be returned as raw HTML, or should the sidecar pre-extract server links?**
   - What we know: CONTEXT.md `/play` shape says "raw HTML of the play page OR pre-extracted server links" (operator's discretion).
   - What's unclear: pre-extraction in the sidecar removes the goquery dependency from the Go parser's play path. But it also means the sidecar becomes opinionated about what to extract; an HTML drift requires editing both sides.
   - Recommendation: return raw HTML. Keep the goquery scraping in the Go parser. Single source of truth for selector logic, and the maintenance-bot's Pattern 7 auto-edit-selectors path stays applicable.

3. **Should `ANIMEPAHE_BASE_URL` env var on the scraper service be removed or repurposed?**
   - What we know: It currently defaults to `https://animepahe.ru` and is consumed by `config.AnimePaheConfig`. Post-rewrite it's vestigial.
   - What's unclear: removing it requires plan-checker awareness (the env-var is referenced in docker-compose.yml + .env.example + RESEARCH for Phase 16).
   - Recommendation: Plan 27-03 removes the env var from compose AND from `config.go::AnimePaheConfig` (it's no longer read by the rewritten parser). Adds `SCRAPER_ANIMEPAHE_RESOLVER_URL` in its place.

4. **Does the sidecar need to handle a "warmup race" where the first request arrives before `init()` finishes?**
   - What we know: `init()` is `await`ed before `fastify.listen`, so by the time the HTTP server accepts connections, the browser is up.
   - What's unclear: if Fastify's `listen` is allowed to bind before `init()` resolves (it isn't in the example above — `await` is correct), there's no race. Confirmed in 27-01 by testing cold start.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Node.js 20+ | Sidecar runtime | ✓ | v22.22.0 (host) — Docker image is 20+ | — |
| Docker 24+ | Compose orchestration | ✓ | 29.2.1 | — |
| `puppeteer-extra-plugin-stealth@2.11.2` | DDoS-Guard bypass | ✓ | 2.11.2 (npm) | rebrowser-puppeteer (NOT recommended — no-update notice Feb 2026) |
| `puppeteer-extra@3.3.6` | Plugin host | ✓ | 3.3.6 (npm) | — |
| `puppeteer@^24.0.0` | Chromium control | ✓ | 25.0.4 latest; pin major to 24 for stability | — |
| `ghcr.io/puppeteer/puppeteer:24` | Sidecar base image | ✓ | available on GHCR | `node:20-bookworm` + apt-install chromium (worse — version mismatch risk) |
| Redis | Existing scraper cache | ✓ | running in compose | — |
| `animepahe.pw` upstream reachability | Sidecar operation | ✓ | verified via /tmp/pup/ probe 2026-05-19 | re-add `animepahe` to `SCRAPER_DEGRADED_PROVIDERS` (operator escape hatch in `docker/.env`) |
| `malsync.moe` | MalSync lookup | ✓ | externally hosted | fuzzy `m=search` fallback (already in code) |
| `seccomp:unconfined` | Chrome sandbox in container | ✓ | docker engine supports it | — |

**Missing dependencies with no fallback:** none.

**Missing dependencies with fallback:** none.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework (Go) | Go 1.x stdlib `testing` + `httptest` + `prometheus/client_golang/prometheus/testutil` (existing in `services/scraper/`) |
| Framework (Node sidecar) | Node `node:test` (stdlib) + `tap` (existing in megacloud-extractor) OR `vitest` for new code; pick to match megacloud-extractor's choice for consistency |
| Config files (Go) | none — `go test` defaults |
| Config files (Node) | `services/animepahe-resolver/package.json` `"test"` script |
| Quick run command (Go) | `cd /data/animeenigma/services/scraper && go test -count=1 -run 'AnimePahe' ./internal/providers/animepahe/...` |
| Quick run command (Node) | `cd /data/animeenigma/services/animepahe-resolver && npm test` |
| Full suite command | `make redeploy-scraper && make redeploy-animepahe-resolver && make health && curl pipeline against Frieren` |
| Phase gate | Full curl pipeline against Frieren returns ≥ 28 episodes + a fetchable Kwik stream URL through gateway → catalog → scraper → resolver |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| SCRAPER-HEAL-29 | Sidecar /healthz two-layer probe | unit (Node) | `cd services/animepahe-resolver && npm test -- healthz` | ❌ Wave 0 |
| SCRAPER-HEAL-29 | Sidecar /search returns valid JSON | integration (Node, mocked upstream via puppeteer's request interception) | `cd services/animepahe-resolver && npm test -- search` | ❌ Wave 0 |
| SCRAPER-HEAL-29 | Sidecar /release pagination passthrough | integration (Node) | `cd services/animepahe-resolver && npm test -- release` | ❌ Wave 0 |
| SCRAPER-HEAL-29 | Sidecar /play HTML passthrough | integration (Node) | `cd services/animepahe-resolver && npm test -- play` | ❌ Wave 0 |
| SCRAPER-HEAL-29 | Memory ceiling under 500 MB at steady state | smoke (manual gate) | `for i in 1..100; do curl -s sidecar/search?q=X; done & docker stats animepahe-resolver` | ❌ Wave 0 — 27-01 manual probe |
| SCRAPER-HEAL-30 | FindID via resolver/search fuzzy fallback | unit (Go) | `go test -count=1 -run TestProvider_FindID ./internal/providers/animepahe/...` | ✅ existing in client_test.go (rewire) |
| SCRAPER-HEAL-30 | ListEpisodes via resolver/release multi-page | unit (Go) | `go test -count=1 -run TestProvider_ListEpisodes ./internal/providers/animepahe/...` | ✅ existing (rewire) |
| SCRAPER-HEAL-30 | ListServers via resolver/play HTML scraping | unit (Go) | `go test -count=1 -run TestProvider_ListServers ./internal/providers/animepahe/...` | ✅ existing (rewire) |
| SCRAPER-HEAL-30 | GetStream caches Kwik extraction (unchanged path) | unit (Go) | `go test -count=1 -run TestProvider_GetStream ./internal/providers/animepahe/...` | ✅ existing (unchanged) |
| SCRAPER-HEAL-30 | DTO parses fresh-from-resolver Frieren golden | unit (Go) | `go test -count=1 -run TestDTO_Frieren ./internal/providers/animepahe/...` | ❌ Wave 0 — golden capture in 27-02 |
| SCRAPER-HEAL-30 | MalSync 24h cache + cache invalidation on /release 404 | unit (Go) | `go test -count=1 -run TestProvider_MalSyncInvalidationOn404 ./internal/providers/animepahe/...` | ❌ Wave 0 |
| SCRAPER-HEAL-31 | Compose dependency: scraper waits for resolver healthy | manual (smoke) | `docker compose up animepahe-resolver scraper && docker compose ps` (verify scraper does not start until resolver is healthy) | manual |
| SCRAPER-HEAL-31 | Resolver enforces 500 MB mem_limit | manual (smoke) | run 27-01 100-request soak; `docker stats` snapshot | manual |
| SCRAPER-HEAL-32 | End-to-end Frieren gate | integration (manual gate, not CI) | `BASE=http://localhost:8000 ANIME_ID=f0b40660-... bash docs/issues/scraper-provider-verification-2026-05-19.md curl pipeline` returns ≥ 28 episodes + fetchable stream | manual |
| SCRAPER-HEAL-32 | Provider health gauge flips to `up:true` after redeploy | observability | `curl http://localhost:8088/scraper/health` shows animepahe stages search/episodes/servers/stream all `up:true` within 5 min | manual |
| SCRAPER-HEAL-33 | Compose default cleanup commits cleanly | git verification | `git diff docker/docker-compose.yml` shows `gogoanime,animepahe` → `gogoanime` | manual |
| SCRAPER-HEAL-33 | No continuous 403/timeout in logs post-redeploy | observability | `make logs-scraper | head -200 | grep -E '403|context deadline' | wc -l` ≤ 1 in the first 10 min | manual |

### Sampling Rate

- **Per task commit:** quick run command for the touched layer (Go unit tests for parser changes; Node tests for sidecar changes).
- **Per wave merge:** full Go suite + Node suite + `make redeploy-scraper` + `make redeploy-animepahe-resolver`.
- **Phase gate:** Frieren curl pipeline end-to-end + Phase 24 verdict-log update + 10-minute log-tail check (D7).

### Wave 0 Gaps

- [ ] `services/animepahe-resolver/server.test.js` — covers SCRAPER-HEAL-29 (healthz, search, release, play)
- [ ] `services/animepahe-resolver/upstream.test.js` — covers 403 retry + page recycle logic
- [ ] `services/scraper/testdata/animepahe/frieren-search.json` — captured fresh in 27-02 per D4
- [ ] `services/scraper/testdata/animepahe/frieren-release.json` — captured fresh in 27-02 per D4
- [ ] `services/scraper/testdata/animepahe/frieren-play.html` — captured fresh in 27-02 per D4
- [ ] `services/scraper/internal/providers/animepahe/resolver.go` — new resolver HTTP client
- [ ] `services/scraper/internal/providers/animepahe/resolver_test.go` — covers resolver client error mapping (502 → ErrProviderDown, 404 → ErrNotFound)
- [ ] `services/scraper/internal/providers/animepahe/malsync_invalidation_test.go` — covers 27 new requirement: invalidation on /release 404

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|------------------|
| V2 Authentication | no | Sidecar is internal-only (no port exposed on host); no auth between scraper and resolver. Same precedent as megacloud-extractor. |
| V3 Session Management | no | No sessions in sidecar; warm browser is process-singleton. |
| V4 Access Control | yes | Sidecar MUST enforce `host == animepahe.pw` on every fetch (defense-in-depth, see Pitfall 1). |
| V5 Input Validation | yes | Fastify JSON schema validation on all query params. `q`, `session`, `animeSession`, `episodeSession` are pattern-validated (UUIDs or alphanumerics) before reaching `page.evaluate`. |
| V6 Cryptography | no | No crypto in sidecar; Kwik decryption is Go-side via goja (existing extractor). |

### Known Threat Patterns for sidecar + Go scraper stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SSRF via user-controlled URL into `page.goto` | Tampering / Info disclosure | Sidecar accepts only `q`/`session`/`episodeSession` query params; constructs the upstream URL server-side with hardcoded `animepahe.pw` base. User input never crosses into `page.goto` argument verbatim. |
| Chromium sandbox escape via malicious upstream JS | Elevation of privilege | Hardcoded `animepahe.pw` host check + documented "do not add second upstream without security review" (Pitfall 1). |
| Memory exhaustion (sidecar OOM kills scraper's depends_on chain) | DoS | `mem_limit: 500m` + page-recycle policy at N=100 + healthcheck OOM-restart. |
| Cookie leak (DDoS-Guard cookies exposed via /metrics or logs) | Info disclosure | `prom-client` metrics MUST NOT emit cookie values; Pino logger configured with redaction for any header field. |
| Stealth plugin defeated → silent degrade | Repudiation / DoS | `stealth_challenge_failures_total` metric + Pattern 7 maintenance-bot escalation + SCRAPER_DEGRADED_PROVIDERS env escape hatch. |
| Path-traversal Kwik URL (`kwik://path/../../../`) reaching extractor | Info disclosure | Existing client.go WR-05 check rejects non-http/https schemes; preserved in rewrite. |
| Container has SYS_ADMIN capability | Elevation of privilege | Sidecar does NOT need SYS_ADMIN (uses `--no-sandbox` instead). Reviewer should confirm no `cap_add: SYS_ADMIN` in compose block. |

## Sources

### Primary (HIGH confidence)

- [puppeteer/docker docs (pptr.dev/guides/docker)][pptr-docker] — Official Puppeteer Docker guide; basis for using `ghcr.io/puppeteer/puppeteer:24`.
- [npm: puppeteer-extra-plugin-stealth][stealth-npm] — Version verified via `npm view`.
- [npm: puppeteer-extra][pe-npm] — Version verified via `npm view`.
- [npm: puppeteer][puppeteer-npm] — Version verified via `npm view`.
- [Docker docs: depends_on healthcheck semantics][docker-compose-depends] — Compose v2 service_healthy condition.
- [Puppeteer Sandbox always-use guidance][puppeteer-sandbox] — Security context for `--no-sandbox`.
- [Existing animepahe parser source: services/scraper/internal/providers/animepahe/client.go] — Code under test; HIGH confidence on current behavior.
- [docs/issues/scraper-provider-verification-2026-05-19.md] — Phase 24 hard-gate verdict log; canonical test pipeline.
- [.planning/phases/27-animepahe-revival-via-stealth-chromium-sidecar/27-CONTEXT.md] — Operator-locked decisions.
- [services/scraper/testdata/animepahe/release_4_p1.json] — DTO shape verification (existing fixture).

### Secondary (MEDIUM confidence)

- [animepahe-dl (KevCui)][animepahe-dl] — Community shell-script reference for upstream API endpoints + play-page extraction regex. Corroborates A3.
- [ZenRows: Puppeteer Extra Plugin Stealth 2026][zenrows-stealth] — Community 2026 assessment; matches "still works, low maintenance" reading.
- [ScreenshotEngine: Bypass Bot Detection 2026][screenshot-stealth] — Same community context for stealth plugin.
- [Stackwise: Express vs Fastify 2026][fastify-bench] — Throughput benchmark for sidecar framework choice.
- [BrightData: Puppeteer Real Browser Guide 2026][brightdata] — Rebrowser maintenance status (no-update Feb 2026).
- [Cloudflare Browser Run puppeteer docs][cf-puppeteer] — Cookie persistence within browser context (informs in-page fetch trick).

### Tertiary (LOW confidence)

- [WebScraping.ai memory tuning FAQ][webscraping-memory] — Chromium memory flags; used as input to A6 (assumed; verify empirically in 27-01).
- [GitHub issue: berstend/puppeteer-extra #236][stealth-issue236] — Old maintenance discussion; flags the long-tail risk.

[pptr-docker]: https://pptr.dev/guides/docker
[stealth-npm]: https://www.npmjs.com/package/puppeteer-extra-plugin-stealth
[pe-npm]: https://www.npmjs.com/package/puppeteer-extra
[puppeteer-npm]: https://www.npmjs.com/package/puppeteer
[docker-compose-depends]: https://docs.docker.com/reference/compose-file/services/
[puppeteer-sandbox]: https://puppeteer.guide/posts/sandbox/
[animepahe-dl]: https://github.com/KevCui/animepahe-dl
[zenrows-stealth]: https://www.zenrows.com/blog/puppeteer-extra
[screenshot-stealth]: https://www.screenshotengine.com/blog/puppeteer-extra-plugin-stealth
[fastify-bench]: https://stackwise.info/compare/express-vs-fastify
[brightdata]: https://brightdata.com/blog/web-data/puppeteer-real-browser
[cf-puppeteer]: https://developers.cloudflare.com/browser-run/puppeteer/
[webscraping-memory]: https://webscraping.ai/faq/headless-chromium/what-are-the-best-practices-for-managing-memory-usage-in-headless-chromium
[stealth-issue236]: https://github.com/berstend/puppeteer-extra/issues/236

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH for puppeteer/fastify/pino versions (verified via npm); MEDIUM for stealth plugin viability (verified by 2026-05-19 probe but operating on a 3-year-old npm package).
- Architecture: HIGH (warm-page + in-page-fetch pattern is the documented stealth-puppeteer best practice; ASVS controls map cleanly).
- AnimePahe API contract: HIGH (live probe + community parsers + existing fixture all corroborate DTO shape).
- Memory budget feasibility: MEDIUM (commonly achievable per multiple guides; MUST be empirically verified in 27-01 — A6).
- Pitfalls: HIGH (drawn from official puppeteer docs, Docker Chrome guides, and direct code reading of current parser).
- Validation architecture: HIGH (existing test infrastructure is reused; new tests are mechanical extensions).

**Research date:** 2026-05-19
**Valid until:** 2026-06-19 (30 days; stealth plugin / DDoS-Guard rotation cadence makes longer windows unreliable). Re-validate before Plan 27-02 golden capture if more than a week has elapsed.
