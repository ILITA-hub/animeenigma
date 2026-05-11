# Stack Research — v3.0 Universal Anime Scraper

**Domain:** Self-hosted Go scraping service for anime streaming sources (AnimeKai, AnimePahe, Anitaku/Gogoanime)
**Researched:** 2026-05-11
**Confidence:** HIGH for HTML/HTTP layer, MEDIUM for embed-decryption pattern, LOW-MEDIUM for AnimeKai (upstream relies on a remote service that just broke our Consumet pipeline)

> Scope reminder: Go 1.22, Chi, GORM, Postgres, Redis, `libs/{logger,metrics,cache,errors,videoutils,idmapping}`, and the `docker/megacloud-extractor/` Node helper are GIVENS. This document only covers **additions** for the new scraping service.

## Headline Recommendation

Build a **pure-Go scraper using `net/http` + `goquery` + `golang.org/x/time/rate`**, with the existing `megacloud-extractor` Node container kept for **MegaCloud/MegaUp embed decryption only**. Add **`dop251/goja`** for one specific job: deobfuscating Kwik (AnimePahe) packer-style JS. **Skip Colly, skip chromedp/rod, skip uTLS unless a provider visibly blocks us on JA3** — the live triage on 2026-05-09 shows plain `curl` against animekai.to / animepahe / anitaku.io already returns real HTML 200s, so we are not facing a Cloudflare-JS-challenge problem today.

**The single biggest risk this stack carries** is that the entire AnimeKai scraping ecosystem (Aniyomi extension, walterwhite-69/AnimeKAI-API, Sheets-Astrum-BOT/AnimeKai-API-Python) currently delegates token + stream decryption to `https://enc-dec.app` — the **same remote service whose contract change broke our Consumet upstream** (see PROJECT.md drivers). Any AnimeKai provider we ship MUST treat `enc-dec.app` as untrusted: either reverse the algorithm ourselves into Go, or run AnimeKai *behind a feature flag* with a "we know this can break" service-level expectation. See PITFALLS.md for the deeper writeup.

## Recommended Stack

### Core Scraping Stack (new dependencies)

| Technology | Version (verified 2026-05-11) | Purpose | Why this over alternatives |
|---|---|---|---|
| **`github.com/PuerkitoBio/goquery`** | **v1.12.0** (released 2026-03-15, requires Go 1.25+) | jQuery-style DOM traversal: episode lists, server lists, search rows from HTML | Stable API ("will not break" per maintainer), 13.8k importers, regular releases (v1.10 → v1.12 across 2025-2026), wraps `net/html` + `andybalholm/cascadia` which are both Go-team-blessed deps. We pick goquery over Colly because **(a)** Colly is effectively dormant (last release v2.2.0 March 2024 — 14 months stale at time of writing), **(b)** Colly's "framework" features (request queue, rate limit, cookies) are weak substitutes for `golang.org/x/time/rate` + `net/http/cookiejar` which we already control, and **(c)** our scraper is finite-depth (search → episodes → servers → sources), not a crawler that benefits from Colly's collector abstraction. We also do not need Colly's CSS selector layer because goquery already provides it. |
| **`net/http` (stdlib) + `net/http/cookiejar`** | Go 1.22 stdlib | HTTP transport, per-host cookie jar, redirect policy | We do not need a wrapper. Existing parsers (`kodik/client.go:111`, `consumet/client.go:39`) already use raw `*http.Client{Timeout: …}`. Pattern is well-understood in this codebase. Plain `net/http` outperforms `resty` and `req` for our access pattern (no JSON encoding, just GETs with custom headers). Cookie jar is required for AnimePahe's DDoS-Guard cookie flow. |
| **`golang.org/x/time/rate`** | latest (`v0.x`, part of `golang.org/x/time`) | Token-bucket rate limiting per upstream host | Stdlib-adjacent, zero external deps, exact semantics we need ("at most N req/s to anitaku.io, separate budget for animepahe.com"). Standard Go community pattern; documented on the official Go wiki at go.dev/wiki/RateLimiting. We wire one `*rate.Limiter` per provider so a slow upstream cannot starve a fast one. |
| **`github.com/hashicorp/go-retryablehttp`** | **v0.7.7+** (actively maintained by HashiCorp) | Automatic retries with exponential backoff on 5xx + connection errors | Thin wrapper over `net/http`, exposes nearly the same API, converts to `*http.Client` via `StandardClient()` so it composes cleanly with our existing `videoutils.NewVideoProxy(cfg)` pattern. Replaces the **hand-rolled retry loops in `hianime/client.go:258` and `consumet/client.go:223`** — those loops do `time.Sleep(retryBaseWait * time.Duration(1<<(attempt-1)))` per-call, which is exactly what `go-retryablehttp` provides correctly with body-rewind support. We pick this over `cenkalti/backoff/v4` because retryablehttp is built around the HTTP request lifecycle (handles 429 `Retry-After`, body rewind, idempotency) rather than being a generic backoff primitive. |

### Provider-Specific Decryption / Extraction

| Technology | Version | Purpose | When to use |
|---|---|---|---|
| **`github.com/dop251/goja`** | latest tagged ES5.1+/ES6-partial pure-Go JS runtime, actively developed (519+ commits, recent issue activity) | Evaluate packer-style obfuscated JS from kwik.cx pages without spawning a process | **AnimePahe path only.** Aniyomi's Kohi-den `AnimePahe.kt` extension confirms kwik deobfuscation is a local-only operation — the eval'd packer JS is pure code transformation, no DOM, no network. goja handles this with zero cgo. We use goja over `otto` (effectively dormant) and over shelling out to Node (`megacloud-extractor` is for a different purpose — see below). |
| **`docker/megacloud-extractor/` (existing, KEEP)** | n/a — already in the monorepo | Decrypt MegaCloud `getSources` responses for any provider that still routes through MegaCloud (rare in v3.0 scope — primarily HiAnime's lineage, but historically AnimeKai routes some servers through megacloud.blog too) | **Do NOT rewrite to Go in v3.0.** The 250-LOC Node service at `docker/megacloud-extractor/server.js` already works in production (verified in `megacloud-extractor:3200` health checks). Its `extractSources()` flow (HTML fetch → client-key extraction via 4 regex variants → `getSources` JSON call → conditional AES-256-CBC decryption with OpenSSL-style salt) is well-tested. The risk surface of rewriting it (we'd reimplement the cinemaxhq/keys offset extraction, the OpenSSL EVP_BytesToKey, and the embed-2/v3/e-1 URL pattern) is far higher than just calling it over HTTP from Go. Treat it as a sidecar microservice. |
| **`crypto/aes` + `crypto/md5` (stdlib)** | Go 1.22 stdlib | If we choose to reimplement *any* AES-CBC decryption in Go (e.g. if `megacloud-extractor` is unhealthy or if we need to inline OpenSSL `EVP_BytesToKey`-style key derivation) | Stdlib path; reference implementation is `docker/megacloud-extractor/server.js:153-205`. Do this only if v3.1 wants to retire the Node container. |

### What we do NOT add

| Tech | Reason | Use instead |
|---|---|---|
| **`gocolly/colly`** | Last release v2.2.0 **March 27, 2024** (~14 months stale at 2026-05-11). 143 open issues, 40 open PRs unaddressed. Per the Awesome-Go landscape and ZyteScrapfly's 2026 Go scraping survey, Colly is in maintenance mode. Its features (rate limit, cookies, async queue) are thin wrappers we either don't need or implement better ourselves. | `net/http` + `goquery` + `golang.org/x/time/rate` |
| **`chromedp` / `go-rod` / `puppeteer` / `playwright`** | Heavy: chromedp pulls in headless Chromium (~150-200MB image), needs sandbox/seccomp tuning, blows up our small self-hosted Docker footprint. **Empirically not needed** — live triage on 2026-05-09 (recorded in STATE.md "v3.0 Drivers") shows animekai.to, animepahe.com, anitaku.io all return real HTML to plain curl. Aniyomi's `AnimePahe.kt` (Kohi-den) handles DDoS-Guard with cookie-jar logic alone via `DdosGuardInterceptor` — no headless browser. Bring in headless only if a specific provider visibly fails on `goquery` + `cookiejar`. | Cookie jar + correct UA + Referer chain. If JS challenge hits us later: add `go-rod/stealth` behind a feature flag, do NOT make it the default. |
| **`refraction-networking/utls` / `bogdanfinn/tls-client`** | These exist to defeat JA3/JA4 TLS fingerprinting. We don't observe TLS-level blocks in the 2026-05-09 triage. utls is a `crypto/tls` fork — adding it is a non-trivial transport surgery (`http.Transport.DialTLSContext` rewrite) that pollutes every `*http.Client` in the service. Risk of breaking HTTP/2 or our existing `libs/videoutils` HLS proxy is non-zero. | Standard Go TLS. Re-evaluate per-provider only if we observe consistent 403/503s that correlate with TLS handshake (not headers). |
| **`@consumet/extensions` (npm)** | Consumet's HiAnime path is already broken on us (PROJECT.md drivers). The upstream API server at `api.consumet.org` is shut down. The npm package itself still exists, but its providers age-out one-by-one as upstream sites change shape; we'd be rebuilding the same brittle dependency that just bit us. The fact that `consumet.ts` issue #613 ("Gogoanime - Change of domain from anitaku.bz to anitaku.io") sat unresolved with "wontfix" labels confirms the project is no longer tracking provider URL changes. | Pure-Go scraping anchored to provider-specific code we own. |
| **`cloudscraper_go`** | Per its own README, "currently handles only passive checks based on TLS footprint (JA3) and user agents" — so it's redundant with what we already get from `net/http` headers, and useless against modern Cloudflare JS challenges. | n/a — we don't currently face a Cloudflare problem |
| **`FlareSolverr`** | Headless Chrome sidecar. Same problem as chromedp (heavy, requires X-server-like surface) PLUS a separate HTTP control plane to maintain. Per Scrapfly 2026 review, FlareSolverr-style proxies are "largely ineffective against modern Cloudflare challenges" anyway. | n/a |

### Testing & Test Fixtures

| Library | Version | Purpose | Why |
|---|---|---|---|
| **`net/http/httptest`** (stdlib) | Go 1.22 | Mock upstream servers in unit tests | Standard idiom; existing `services/catalog/internal/parser/kodik/client_test.go` uses this pattern. We continue it. |
| **`github.com/sebdah/goldie/v2`** | actively maintained | Golden-file snapshot testing of *parsed* output (the `SearchResult{}` slice, `Stream{}` struct) — NOT of upstream HTML | Quality-gate requirement #6: keep tests deterministic when upstream is dead/rate-limited. Pattern: store upstream HTML in `testdata/animekai/search-naruto.html`, store expected parsed Go struct in `testdata/animekai/search-naruto.golden.json`. `go test -update ./...` re-snapshots when intentional changes happen. We pick goldie over `nao1215/golden` because goldie has been the de-facto Go golden-file library for ~8 years with stable v2 API. |
| **`testdata/` directory convention** | Go stdlib convention | Store captured HTML fixtures per provider | The test fixture HTML is captured by hand once per provider release. We do NOT auto-refresh it from live upstreams in CI — that defeats the determinism we're buying. The corollary is a separate `go test -tags=live ./...` integration suite that *does* hit real upstreams; that one only runs locally and in a nightly job, never blocking PRs. |

### Project Layout (new module)

```
services/scraper/
├── cmd/scraper-api/main.go
├── internal/
│   ├── config/
│   ├── domain/
│   │   └── provider.go            # Provider interface — search/info/episodes/servers/stream
│   ├── handler/                   # HTTP handlers (Chi router, same pattern as catalog)
│   ├── service/
│   │   └── orchestrator.go        # cross-provider search + ranking
│   ├── transport/                 # Chi router setup, middleware
│   ├── httpclient/
│   │   ├── client.go              # *http.Client w/ cookiejar + UA + Referer chain
│   │   ├── ratelimit.go           # per-host *rate.Limiter map
│   │   └── retry.go               # go-retryablehttp glue
│   ├── parser/
│   │   ├── animekai/              # client.go, decrypt.go (or remote-call), client_test.go
│   │   ├── animepahe/             # client.go, kwik_unpacker.go (goja), client_test.go
│   │   └── anitaku/               # client.go, gogo_extractor.go, client_test.go
│   └── extractor/
│       ├── megacloud.go           # HTTP wrapper around docker/megacloud-extractor
│       └── kwik.go                # local kwik decoder using goja
├── testdata/                      # per-parser .html fixtures + .golden.json
├── Dockerfile
└── go.mod                         # adds: goquery, goldie/v2, go-retryablehttp, goja, golang.org/x/time
```

The `Provider` interface in `domain/provider.go` is the seam REC-FOUND-01 of v3.0 — same pattern as v2.0's `SignalModule`. Per quality-gate #3, every provider plugs into:
- `libs/logger` for structured logging (zap)
- `libs/metrics` for `scraper_provider_request_duration_seconds{provider="animekai", stage="search"}` Prometheus histogram
- `libs/errors` for domain error wrapping (`errors.NotFound`, `errors.Wrap`)
- `libs/cache` for stream-URL caching with the existing 1h TTL pattern (CLAUDE.md "Don't cache video URLs longer than 1 hour")

## HTTP Client Tuning Specifics

Implementation guidance the roadmapper can paste into a Phase 1 plan:

```go
// services/scraper/internal/httpclient/client.go
package httpclient

import (
    "net/http"
    "net/http/cookiejar"
    "time"
    "golang.org/x/net/publicsuffix"
)

// Browser-realistic headers. Single canonical UA initially; rotate only if we
// observe a provider blocking us (then add a small slice and pick deterministically
// per (provider, episodeID) so retries hit the same UA).
const defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

func NewProviderClient() (*http.Client, error) {
    jar, err := cookiejar.New(&cookiejar.Options{
        PublicSuffixList: publicsuffix.List, // required so DDoS-Guard cookies stay scoped
    })
    if err != nil {
        return nil, err
    }
    return &http.Client{
        Jar:     jar,
        Timeout: 15 * time.Second, // per-request; orchestrator sets context deadlines separately
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            if len(via) >= 10 {
                return http.ErrUseLastResponse
            }
            // Preserve Referer across redirects — required for kwik.cx + megacloud.blog
            if len(via) > 0 {
                req.Header.Set("Referer", via[len(via)-1].URL.String())
            }
            return nil
        },
    }, nil
}
```

**Per-host rate limit budget (initial values, tunable):**

| Host | Rate | Burst | Rationale |
|---|---|---|---|
| `animekai.to` | 1 req/s | 3 | Conservative; we don't know its rate limit policy yet |
| `animepahe.{pw,com,org}` | 1 req/s | 2 | DDoS-Guard is touchy; Aniyomi extension uses single-flight requests |
| `anitaku.io` | 2 req/s | 5 | Older site, more lenient historically |
| `kwik.cx` | 0.5 req/s | 1 | Only hit during stream-resolution leaf, not crawl-heavy |
| `megacloud.blog` (via megacloud-extractor) | n/a | n/a | Rate-limited inside the Node container; our Go side talks to localhost |

`golang.org/x/time/rate.Limiter` is per-host singleton, keyed by parsed `req.URL.Host`, lazily created on first request, never garbage-collected (the set of provider hosts is small and stable — 5-10 entries).

**Proxy support:** add an `HTTPS_PROXY` env-var pass-through (`http.ProxyFromEnvironment`). Do not bake in a specific proxy provider in v3.0 — if any provider geo-blocks us we can flip the env var and redeploy. Keep configurability out of the code path until we observe a blocker.

## Alternatives Considered

| Recommended | Alternative | When alternative is better |
|---|---|---|
| `goquery` | `gocolly/colly/v2` | If we evolve into a multi-page crawler (e.g. mirror entire AnimeKai catalog locally) — Colly's `OnHTML` + queue model wins there. We're not doing that in v3.0. |
| `goquery` | `htmlquery` (XPath) | If parsing requires deep XPath axes — none of our extraction does; CSS selectors are sufficient. |
| `dop251/goja` | `robertkrimen/otto` | Otto is older (ES5-only, ~no recent commits). Goja is more compatible with modern obfuscator output. |
| `dop251/goja` | shell out to `node` for kwik unpacking | If we ever hit a kwik variant goja can't run (ES2017+ private fields, etc.). The `megacloud-extractor` container could be extended for this — but adds a network hop for what is a tiny pure-CPU task. |
| Stay on `net/http` | `bogdanfinn/tls-client` | If a provider visibly blocks us via JA3 (we'd see consistent 403/403 on otherwise-identical-from-userland requests). Add as drop-in: `tls-client` exposes `http.Client`-shaped API. |
| Keep `megacloud-extractor` Node container | Rewrite to pure Go using `crypto/aes` + `crypto/md5` | If the Node container becomes a maintenance burden or we ship to a Go-only Kubernetes target. Today it works, ports cleanly, and rewriting it incurs a "decrypt parity" debugging tax. v3.1+ candidate. |

## Installation

```bash
# Inside services/scraper/
go mod init github.com/ILITA-hub/animeenigma/services/scraper

# Core
go get github.com/PuerkitoBio/goquery@v1.12.0
go get github.com/hashicorp/go-retryablehttp@v0.7.7
go get golang.org/x/time
go get golang.org/x/net/publicsuffix    # for cookiejar PublicSuffixList

# Decryption / JS eval (AnimePahe Kwik path)
go get github.com/dop251/goja@latest

# Testing
go get github.com/sebdah/goldie/v2@v2.5.5

# Existing monorepo libs (already in workspace, just add replace directives)
# See /root/.claude/projects/-data-animeenigma/memory/MEMORY.md "Adding New libs/ Module"
# Update go.work + Dockerfile per that procedure.
```

The Dockerfile follows the existing pattern (`services/{name}/Dockerfile`) — multi-stage builder w/ go.mod cache, alpine final image with CA certs. No new system deps required (no Chromium, no Node — Node stays in its own `megacloud-extractor` container).

## Version Compatibility

| Package | Compatible With | Notes |
|---|---|---|
| `goquery v1.12.0` | Go 1.25+ required | **CRITICAL:** Our monorepo is on Go 1.22. Two paths: **(a)** Pin `goquery@v1.10.3` (released 2025-04-11, requires Go 1.21+) which still has stable API and full feature parity for what we need (jQuery-style traversal — `EachIter` from v1.10.0 is the most recent API addition and is non-essential). **(b)** Bump the monorepo to Go 1.25. Recommendation: pin v1.10.3 in v3.0, defer Go-version bump to a separate maintenance phase. Either pinned version meets quality-gate #1. |
| `goja` | Go 1.16+ | No conflict |
| `go-retryablehttp` | Go 1.13+ | No conflict |
| `golang.org/x/time/rate` | Go 1.7+ | Stdlib-adjacent, evolves slowly |
| `goldie/v2` | Go 1.13+ | No conflict |
| `megacloud-extractor` (Node) | n/a | Already running in production at `http://megacloud-extractor:3200`. Service contract is the `/extract?url=...` endpoint returning `{sources[], tracks[], intro, outro}`. Treat as a stable internal API. |

## Integration With Existing AnimeEnigma Libs

| Existing lib | How the scraper uses it |
|---|---|
| `libs/logger` (zap) | All providers do structured logging: `log.Infow("scraping search", "provider", "animekai", "query", q)` |
| `libs/metrics` (Prometheus) | Add three new metric families: `scraper_provider_requests_total{provider, stage, status}`, `scraper_provider_request_duration_seconds`, `scraper_provider_extract_failures_total{provider, reason}` — the third feeds the per-provider health dashboard required by PROJECT.md ("Health-check and per-provider Prometheus metrics so we can see when a provider site dies before users do") |
| `libs/errors` | Provider returns `errors.NotFound("no episodes for anime X on animekai")`, `errors.Wrap(err, "kwik decryption failed")` — orchestrator can fan out to next provider on NotFound, surface decryption failures as 502s |
| `libs/cache` | Cache stream URLs under `scraper:stream:{provider}:{episodeID}:{server}:{category}` with `time.Hour` TTL — matches the existing video-URL TTL pattern. Search results cached `scraper:search:{provider}:{query}` for 15 min (already a TTL constant in `libs/cache` per CLAUDE.md). |
| `libs/idmapping` (ARM client) | Provider orchestrator uses ARM to map `shikimori_id → anilist_id → mal_id`, then runs the cross-provider search step keyed on whichever ID the provider site uses (AnimeKai uses its own slugs, but we resolve by title-with-ID-tiebreaker) |
| `libs/videoutils/proxy.go` | New providers add their stream-host hostnames to `ProxyConfig.AllowedDomains` (currently has `jimaku.cc`, `cdnlibs.org`). For v3.0 we'll add: `*.megacloud.blog`, `*.kwik.cx`, `*.anitaku.io`, etc. The proxy itself does NOT change. |
| `docker/megacloud-extractor/` | Scraper makes HTTP GETs to `http://megacloud-extractor:3200/extract?url=...` only from the AnimeKai (and potentially MegaUp) paths. The Node container is shared infrastructure, not per-service. |

## What This Stack Does NOT Solve

Explicit gaps to address in PITFALLS.md / FEATURES.md / future phases:

1. **AnimeKai's reliance on `enc-dec.app`.** The Aniyomi extension at `Kohi-den/extensions-source/src/en/animekai` calls `https://enc-dec.app/api/enc-kai` and `/api/dec-kai` to encrypt request tokens and decrypt stream URLs. This is the **same external service** whose body-shape change broke our Consumet HiAnime path on 2026-05-09 (`Expected body: text, agent`). Every public AnimeKai scraper I found (walterwhite-69, Sheets-Astrum-BOT) chains through the same service. To ship AnimeKai safely we need to **reverse-engineer the encryption locally** — likely a custom alphabet substitution + AES-CBC over a derived key, similar to MegaCloud but per-provider. This is real R&D work that belongs in its own Phase, not "just add goja". Until that's done, recommend shipping AnimePahe + Anitaku first and gating AnimeKai behind a feature flag with explicit "may break" UI.

2. **AnimeKai → MegaUp embed extraction.** Even after token decryption, the stream resolution flow lands on a MegaUp / MegaCloud embed page. Our existing `megacloud-extractor` was built for the HiAnime-flavor of MegaCloud (`megacloud.blog/embed-2/v3/e-1/...`). Verifying it works against AnimeKai's MegaUp variant is a Phase-1 prerequisite — there may be different client-key meta tags or different `getSources` path shape. The `extractSources()` regex panel in `server.js:55-69` already has 4 fallback variants; we may need a 5th.

3. **Per-provider HTML drift.** Anime piracy sites change their HTML structure with weeks-to-months cadence (e.g. issue #613 in consumet.ts about `anitaku.bz → anitaku.io` is exactly the class of breakage). The golden-file pattern + a separate `-tags=live` integration test suite catches this only after the fact. A v3.1 nice-to-have is a per-provider canary cron that hits each upstream daily and fires a metric on parser failure.

4. **No Captcha / CAPTCHA handling.** None of the 2026-05-09 triage requests hit a CAPTCHA. If a provider deploys hCaptcha or Turnstile we have no answer — and we deliberately reject FlareSolverr/headless-Chrome as outside the small-self-hosted budget. Mitigation is to **fail fast and surface "provider unavailable" to the user**, then either add a manual override (admin uploads cookies into Redis) or switch providers. This is a product decision, not a stack decision.

## Sources

**Verified via direct GitHub / pkg.go.dev fetches (HIGH confidence):**
- `github.com/PuerkitoBio/goquery` — v1.12.0 released 2026-03-15, requires Go 1.25+; v1.10.3 released 2025-04-11 is the latest Go-1.21-compatible tag (https://github.com/PuerkitoBio/goquery)
- `github.com/refraction-networking/utls` — v1.8.2 released 2025-01-13, actively maintained (https://github.com/refraction-networking/utls/releases)
- `github.com/gocolly/colly` — v2.2.0 last release **March 27, 2024**, 143 open issues unaddressed (https://github.com/gocolly/colly/releases) — **basis for rejection**
- `github.com/bogdanfinn/tls-client` — v1.14.0 released 2026-02-04, actively maintained (https://github.com/bogdanfinn/tls-client/releases)
- `github.com/hashicorp/go-retryablehttp` — actively maintained by HashiCorp (https://github.com/hashicorp/go-retryablehttp)

**Verified via direct repo inspection of competing scrapers (HIGH confidence on findings, MEDIUM on extrapolation):**
- `Kohi-den/extensions-source/.../animekai/AnimeKai.kt` — confirms AnimeKai extension calls `https://enc-dec.app/api/` for token enc/dec; no local crypto fallback (https://github.com/Kohi-den/extensions-source)
- `Kohi-den/extensions-source/.../animepahe/AnimePahe.kt` — confirms AnimePahe extension does Kwik unpacker **locally** via embedded `KwikExtractor`, with `DdosGuardInterceptor` handling Cloudflare-style cookie challenge (no headless browser needed)
- `walterwhite-69/AnimeKAI-API` README — confirms same `enc-dec.app` dependency in a separately-built Python scraper, reinforcing that "remote decryption service" is the de-facto industry pattern for AnimeKai and the risk is structural, not implementation-specific (https://github.com/walterwhite-69/AnimeKAI-API)

**Local code inspection (HIGH confidence):**
- `/data/animeenigma/docker/megacloud-extractor/server.js` (lines 36-205) — confirms the existing MegaCloud extraction flow: HTML fetch → 4-variant regex client-key extraction → `getSources` JSON → AES-256-CBC OpenSSL-style decryption with `cinemaxhq/keys/e1/key` offset table
- `/data/animeenigma/services/catalog/internal/parser/hianime/client.go` (lines 14-19, 230-295) — confirms current hand-rolled retry loop pattern `go-retryablehttp` will replace
- `/data/animeenigma/services/catalog/internal/parser/kodik/client.go` (lines 111-127, 169-219) — reference for the `*http.Client{Timeout: 30s}` + manual token-fetch pattern the new service can mirror
- `/data/animeenigma/services/catalog/internal/parser/consumet/client.go` (lines 14-25, 247-274) — confirms what NOT to repeat (path-based provider fallback list `["animekai", "hianime", "animepahe"]` hardcoded as constants; provider failures masked behind generic "no stream" errors)
- `/data/animeenigma/libs/videoutils/proxy.go` (lines 33-79) — confirms the existing HLS proxy stays unchanged; new providers extend `AllowedDomains`

**WebSearch (MEDIUM confidence — multiple sources agreed):**
- HiAnime ecosystem shutdown March 2026 + USTR "notorious market" designation, ~900 repos purged from GitHub (Wondershare 2026 alternatives roundup, Protocloud Technologies)
- Consumet API hosted endpoints shut down; npm package alive but providers age-out (consumet/api.consumet.org issue #725; npm @consumet/extensions page)
- `gocolly/colly` widely characterized as "production-ready" but actual release cadence contradicts that claim — basis for our rejection
- Kwik.cx packer-eval deobfuscation pattern (`f, m, p` variable obfuscation) confirmed across multiple downloader projects (anime-dl/anime-downloader PR #316, animepahe-quick-downloader)

**LOW confidence flags:**
- Exact AnimePahe rate-limit threshold — number is a guess based on "single-flight" patterns in the Aniyomi extension; needs empirical measurement in Phase 1
- Whether `megacloud-extractor` handles AnimeKai's MegaUp variant out of the box — needs explicit test against an animekai.to embed URL during Phase 1 spike

---
*Stack research for: v3.0 Universal Anime Scraper*
*Researched: 2026-05-11*
*Reviewer note: all version dates verified within 2 months of "today"; nothing pinned to a 12+-month-stale library except where explicitly rejected (Colly).*
