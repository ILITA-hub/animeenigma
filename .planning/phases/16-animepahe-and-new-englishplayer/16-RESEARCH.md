# Phase 16: AnimePahe + New EnglishPlayer - Research

**Researched:** 2026-05-11
**Domain:** AnimePahe scraping (incl. Kwik via goja) + new unified Vue player
**Confidence:** HIGH — live malsync API probed; authoritative Kotlin source (Kohi-den/Aniyomi) fetched for AnimePahe + Kwik + DDoS-Guard; goja docs confirmed; consumet.ts is DMCA-blocked since 2026-03-19 (irrelevant — Kohi-den is the better reference)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- Source dropdown inside player toolbar (beside quality selector), not above video
- Switching source mid-episode auto-resumes via `vjsPlayer.currentTime()` preserve/restore
- Override-fail policy: auto-fallback to orchestrator default + toast (no hard error)
- Persistence: per-anime via `useWatchPreferences`
- English tab label = `English`; `?legacy=1` adds HiAnime/Consumet `(debug)` tabs alongside (does NOT replace)
- Default tab when all available: English
- Mobile: same dropdown UI on all viewports
- Kwik unpacker lives at `services/scraper/internal/embeds/kwik.go` (mirrors MegacloudClient — but **in-process goja**, NOT a sidecar)
- malsync cache key shape: `malsync:{mal_id}:{provider}`
- DDoS-Guard: one persistent cookie jar per provider, shared across all AnimePahe requests
- `tried[]` chain surfaced via response body field `data.meta.tried` (inside httputil wrapper)

### Locked by REQUIREMENTS (no grey area)

- PAHE-01: malsync.moe + 24h cache + fuzzy fallback
- PAHE-02: episode list 6h cache
- PAHE-03: HLS m3u8 480/720/1080 via kwik.cx + dop251/goja; stream TTL ≤ min(parsed expiry − 30s, 5min)
- PAHE-04: cookiejar via `golang.org/x/net/publicsuffix`; no headless browser
- PAHE-05: `kwik.cx`, `owocdn.top`, `uwucdn.top` in `HLSProxyAllowedDomains` — **already present** in `libs/videoutils/proxy.go:244-246`
- UI-01: EnglishPlayer.vue = Video.js + HLS.js + existing SubtitleOverlay.vue
- UI-03: new `scraperApi` in `frontend/web/src/api/client.ts`; do NOT repoint hianimeApi/consumetApi
- NF-02: TTLs 24h/6h/15min/≤5min
- NF-05: ReportButton emits `provider:<name>` + `tried[]`

### Claude's Discretion

- Internal package layout for `internal/providers/animepahe/` (one file vs split)
- Exact regex for fuzzy-title fallback (no upstream lock)
- Register Kwik before or after Megacloud (suggest Kwik first — its `Matches()` is the cheapest, plain host equality)

### Deferred Ideas (OUT OF SCOPE)

- SSR episode list, "report dead provider" admin button (Phase 17), PiP comparison, network-based auto-quality (HLS.js handles this)
</user_constraints>

## Project Constraints (from CLAUDE.md)

1. Frontend uses `bun` / `bunx` — never `npm`/`pnpm`/`npx`.
2. Commit co-authors: `Claude Opus 4.6`, `0neymik0`, `NANDIorg` (from MEMORY.md).
3. `make redeploy-scraper` after Go changes; `/animeenigma-after-update` skill at end of phase (mandatory).
4. **Forbidden deps** in `services/scraper/go.mod` (CI-enforced): `chromedp`, `go-rod`, `chromedp-rod`, `utls`, `tls-client`, `cloudscraper_go`, `flaresolverr`. `dop251/goja` is on the **allowed list** (verified `forbidden_deps_test.go:230`).
5. No catalog pre-population (on-demand). No caching video URLs > 1 hour (phase locks ≤ 5 min anyway).
6. HTTP responses use `httputil.Response` wrapper (`{success, data}`) — `meta.tried` lives at `data.meta.tried`.
7. Stream DTO has NO `iframe_url` field (Phase 15 compile-time test).

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SCRAPER-PAHE-01 | Resolve MAL ID → AnimePahe via malsync.moe + 24h cache + fuzzy fallback | §AnimePahe API; §Don't Hand-Roll (use malsync, not custom slug-matching) |
| SCRAPER-PAHE-02 | `ListEpisodes` paginated via `/api?m=release` + 6h cache | §Code Example 1 (pagination loop) |
| SCRAPER-PAHE-03 | HLS m3u8 480/720/1080 via Kwik + goja; TTL ≤ min(expires − 30s, 5min) | §Pattern 2 + §Pitfall 3 (goja Interrupt) + §Pitfall 6 (TTL parsing) |
| SCRAPER-PAHE-04 | DDoS-Guard cookies via cookiejar (no headless) | §Pattern 3; BaseHTTPClient jar already exists (`httpclient.go:83`) |
| SCRAPER-PAHE-05 | Append kwik/owocdn/uwucdn hosts to HLS allowlist | **Already present** `libs/videoutils/proxy.go:244-246` — verification-only task |
| SCRAPER-UI-01 | New EnglishPlayer.vue using Video.js + HLS.js + SubtitleOverlay | §Pattern 4 (source switch); fork HiAnimePlayer.vue:909-1031 init |
| SCRAPER-UI-02 | One "English" tab replaces both legacy tabs; Source dropdown inside player | 16-UI-SPEC.md (already exhaustive) |
| SCRAPER-UI-03 | New `scraperApi` in api/client.ts | Mirror existing `consumetApi` block at line 413 |
| SCRAPER-UI-04 | Legacy debug tabs preserved when `?legacy=1` | 16-UI-SPEC.md Edge Cases — v-show NOT v-if |
| SCRAPER-NF-02 | TTLs 24h/6h/15min/≤5min | Per-req tests cover individually |
| SCRAPER-NF-05 | ReportButton: provider + tried[] | 16-UI-SPEC.md §ReportButton extensions |
</phase_requirements>

## Summary

This phase = "wire pieces that already exist" + ~230 LOC of net-new Go (AnimePahe HTML scraping + Kwik unpacker) + a forked Vue player (~1500 LOC, mostly copied from HiAnimePlayer.vue with `scraperApi` swap, Source dropdown subcomponent, accent cyan instead of purple). Every infrastructure piece (BaseHTTPClient, cookiejar, retry, rate-limit, HLS proxy, SubtitleOverlay, ReportButton, Video.js, watch preferences) is from Phase 15 or earlier and is reused as-is.

**Primary recommendation:** Backend split into atomic commits (provider client, Kwik extractor, main.go wiring + handler swap, allowlist verification). Frontend split into atomic commits (scraperApi client, EnglishPlayer.vue fork, Anime.vue tab integration, locales). TDD discipline as in Phase 15: matching RED test commit ahead of each GREEN.

## Architectural Responsibility Map

| Capability | Primary Tier | Rationale |
|------------|-------------|-----------|
| MAL → AnimePahe ID resolution | API/Backend (scraper) + Redis cache | Per-request lookup, 24h cache; frontend has no MAL knowledge |
| AnimePahe HTML/JSON parsing | API/Backend (scraper) | CORS-blocked from browser; HTML scraping is server-side |
| Kwik JS unpacking (goja runtime) | API/Backend (scraper) | Go in-process; no frontend |
| DDoS-Guard cookies | API/Backend (BaseHTTPClient cookiejar) | Per-provider scope |
| HLS stream proxying | API/Backend (existing `libs/videoutils/proxy.go`) | Scraper returns upstream URL; streaming service proxies via `/api/streaming/hls-proxy` |
| EnglishPlayer Vue component | Browser/Client | Same tier as HiAnimePlayer.vue |
| Provider Source dropdown + persistence | Browser/Client + API/Backend (`prefer` URL param) | Vue ref → `useWatchPreferences` (auto-syncs); backend honors `prefer` |
| `meta.tried` surfacing | API/Backend (scraper handler) authors; Browser reads | Backend writes; frontend passes to ReportButton |
| Watch progress beacons | API/Backend (player svc) | Unchanged — same `userApi.updateProgress` as HiAnimePlayer |

## Standard Stack

### Core

| Library | Version | Purpose | Why |
|---------|---------|---------|-----|
| `github.com/dop251/goja` | latest pseudo-version (current: `v0.0.0-20260311135729-065cd970411c`, 2026-03-11) [CITED: pkg.go.dev/github.com/dop251/goja] | In-process JS for Kwik unpacker | Locked. Pure Go, no CGO. Allowed by `forbidden_deps_test.go:230`. |
| `github.com/PuerkitoBio/goquery` | `v1.10.3` [CITED: forbidden_deps_test.go:227 allowed list] | HTML scraping (`div#pickDownload > a`, `div#resolutionMenu > button`) | Aniyomi reference uses Jsoup CSS selectors; goquery = idiomatic Go equivalent. Less brittle than regex. |
| `github.com/hashicorp/go-retryablehttp` | `v0.7.7` [VERIFIED: services/scraper/go.mod:9] | Retry+backoff | Already in BaseHTTPClient. Phase 16 inherits 1→2→4→8s for free. |
| `golang.org/x/net/publicsuffix` | `v0.39.0` [VERIFIED: go.mod:11] | etld+1 cookie scoping for DDoS-Guard | Already wired `httpclient.go:83`. Zero new code. |
| `golang.org/x/time/rate` | `v0.5.0` [VERIFIED: go.mod:12] | Per-host rate limiter | Already provided. Call `WithPerHostRPS("animepahe.ru", 1.0, 2)`. |
| Video.js `^8.10.0`, HLS.js `^1.6.15` | [VERIFIED: 16-UI-SPEC.md + package.json] | Frontend playback | Reuse, no version bump. |

### Alternatives Considered

| Instead of | Could Use | Verdict |
|------------|-----------|---------|
| `dop251/goja` for Kwik | `github.com/stephen-gardner/unpacker` (pure-Go p.a.c.k.e.r unpacker) | **Rejected** — CONTEXT.md locks goja; more future-proof if Kwik changes packer variant. |
| Node sidecar (mirror megacloud) | `kwik-extractor` container | **Rejected** — CONTEXT.md locks in-process; Kwik unpack is self-contained JS (no DOM/Cloudflare/Node APIs). |
| Regex-only extraction | Skip JS execution, regex packed string | **Rejected for HLS path** — Kohi-den `getHlsStreamUrl` USES a real JS unpacker (`JsUnpacker.unpackAndCombine`). Regex-only is the alt `pahe.win → kwik POST` path we don't need. |
| Cloudscraper / utls for DDoS-Guard | tls-client, cloudscraper_go | **Forbidden by CI**. Empirically unneeded — single `check.ddos-guard.net/check.js` GET sets the `__ddg2_` cookie. No JA3 check observed. |

**Install:**
```bash
cd services/scraper && go get github.com/dop251/goja@latest && go get github.com/PuerkitoBio/goquery@v1.10.3 && go mod tidy
cd frontend/web && bun install  # idempotent — no new deps
```

## Architecture Patterns

### System Architecture Diagram

```
[User clicks "English" tab] ─► Anime.vue (v-if videoProvider==='english')
                                   │
                                   ▼ mount
                              EnglishPlayer.vue
                                   │ scraperApi.{getEpisodes, getServers, getStream}
                                   ▼
                              /api/anime/{id}/scraper/*
                              (gateway → catalog:8081 → scraper:8088)
                                   │
                                   ▼
                              Orchestrator.{ListEpisodes,ListServers,GetStream}(prefer)
                                   │ sequential failover (Phase 15, unchanged)
                                   ▼
                              animepahe.Provider  [NEW]
                              ├─ FindID  ──► malsync.moe + 24h cache + fuzzy fallback
                              ├─ ListEpisodes ── /api?m=release&id={id}&page={n} + 6h cache
                              ├─ ListServers ─── scrape /play/{anime}/{ep} hoster buttons
                              └─ GetStream  ──── scrape kwik link → registry.Find(kwik) → kwik.Extract()
                                                  │
                                                  uses BaseHTTPClient (cookiejar + DDoS-Guard)
                                   │
                                   ▼ kwik URL
                              kwik.Extractor  [NEW]
                              1. GET kwik URL (Referer: animepahe baseURL)
                              2. Regex packed JS: eval(function(p,a,c,k,e,d){...})(...)
                              3. goja.New() → vm.RunString(wrapped_as_returning_IIFE)
                                 │ 5s timeout via vm.Interrupt() from separate goroutine
                                 │ fresh runtime per call (goja is NOT thread-safe)
                              4. Regex unpacked: const source='https://...m3u8'
                              5. Return Stream{Sources:[{url, type:"hls"}]}
                                   │ {sources: [{url:m3u8,...}]}
                                   ▼
                              Browser HLS.js fetches m3u8 through
                              /api/streaming/hls-proxy?url=...&referer=https://kwik.cx/
                              (hosts already allowed: kwik.cx, owocdn.top, uwucdn.top)
```

### Recommended Project Structure

```
services/scraper/
├── internal/
│   ├── providers/animepahe/      # NEW
│   │   ├── client.go             # Provider impl (Name, FindID, ListEpisodes, ListServers, GetStream, HealthCheck)
│   │   ├── client_test.go
│   │   ├── malsync.go            # malsync.moe + 24h Redis cache + fuzzy fallback
│   │   ├── malsync_test.go
│   │   ├── ddosguard.go          # __ddg2_ cookie acquisition
│   │   ├── ddosguard_test.go
│   │   └── dto.go                # AnimePahe JSON DTOs
│   ├── embeds/
│   │   ├── megacloud.go          # existing
│   │   ├── kwik.go               # NEW — EmbedExtractor, goja-based
│   │   └── kwik_test.go          # NEW — with packed-JS golden fixture
│   └── testdata/animepahe/       # NEW (Phase 15 created /testdata/.gitkeep only)
│       ├── search_naruto.json    # captured via make capture-goldens
│       ├── release_4_p1.json
│       ├── play_session_ep1.html
│       └── kwik_e_abc.html
└── cmd/scraper-api/main.go       # EDIT — 3 lines added (kwik register, animepahe register)

frontend/web/src/
├── api/client.ts                              # EDIT — add scraperApi block before jimakuApi
├── components/player/
│   ├── EnglishPlayer.vue                      # NEW — fork of HiAnimePlayer.vue (scraperApi + Source dropdown + cyan accent)
│   ├── HiAnimePlayer.vue / ConsumetPlayer.vue # UNCHANGED (legacy, ?legacy=1)
│   ├── SubtitleOverlay.vue                    # UNCHANGED (reused as-is)
│   └── ReportButton.vue                       # EDIT — add scraperProvider + triedChain props
├── composables/useWatchPreferences.ts         # EDIT — add preferredScraperProvider field
├── views/Anime.vue                            # EDIT — English branch + legacy gating + v-show conversion
├── locales/{en,ru,ja}.json                    # EDIT — keys per 16-UI-SPEC.md Copywriting Contract
└── utils/diagnostics.ts                       # EDIT — accept scraperProvider/triedChain
```

### Pattern 1: AnimePahe Provider (Go sketch)

Source: derived from `Kohi-den/extensions-source/.../AnimePahe.kt` (live-fetched 2026-05-11) + Phase 15 `domain.Provider` interface.

```go
type Provider struct {
    baseURL  string   // "https://animepahe.ru" (env: ANIMEPAHE_BASE_URL)
    http     *domain.BaseHTTPClient
    embeds   *domain.Registry
    malsync  *MalSyncClient   // 24h Redis-backed
    log      *logger.Logger
}

func (p *Provider) Name() string { return "animepahe" }

func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
    if id, ok := p.malsync.Lookup(ctx, ref.ShikimoriID, "animepahe"); ok {
        return id, nil  // e.g. "672" for Death Note
    }
    return p.searchFallback(ctx, ref)  // /api?m=search&q={normalized title}, score Jaro-Winkler ≥ 0.85
}

func (p *Provider) GetStream(ctx context.Context, animeSession, epSession, srvID string, cat domain.Category) (*domain.Stream, error) {
    // 1. scrape /play/{anime_session}/{ep_session} → kwik link from data-src
    kwikURL := /* ... */
    // 2. registry.Find(kwikURL) → kwik.Extractor
    ext, err := p.embeds.Find(kwikURL)
    if err != nil { return nil, domain.WrapExtractFailed(err, "animepahe: no extractor") }
    // 3. delegate with Referer header
    return ext.Extract(ctx, kwikURL, http.Header{"Referer": []string{p.baseURL}})
}
```

### Pattern 2: Kwik EmbedExtractor (Go sketch)

Source: `Kohi-den/extensions-source/.../kwik/KwikExtractor.kt::getHlsStreamUrl` + goja docs.

```go
var (
    kwikHosts = []string{"kwik.cx", "kwik.si"}

    // Kotlin: selectFirst("script:containsData(eval(function)").data().substringAfterLast("eval(function(")
    // Go regex equivalent — captures the IIFE expression body.
    packedJSRegex = regexp.MustCompile(`(?s)eval\((function\(p,a,c,k,e,d\).*?\}\([^)]*\))\)`)

    // Kotlin: unpacked.substringAfter("const source=\\'").substringBefore("\\';")
    // Unpacker emits backslash-escaped quotes.
    sourceURLRegex = regexp.MustCompile(`const\s+source\s*=\s*\\?['"]?(https?://[^'"\\]+\.m3u8[^'"\\]*)\\?['"]?`)
)

type KwikExtractor struct {
    http    *domain.BaseHTTPClient
    timeout time.Duration  // default 5s
}

func (k *KwikExtractor) Name() string { return "kwik" }

func (k *KwikExtractor) Matches(embedURL string) bool {
    u, err := url.Parse(embedURL)
    if err != nil { return false }
    host := strings.ToLower(u.Hostname())
    for _, known := range kwikHosts {
        if host == known || strings.HasSuffix(host, "."+known) { return true }
    }
    return false
}

func (k *KwikExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
    req, _ := http.NewRequestWithContext(ctx, "GET", embedURL, nil)
    if headers.Get("Referer") == "" { req.Header.Set("Referer", "https://kwik.cx/") }
    for kk, vs := range headers { for _, v := range vs { req.Header.Add(kk, v) } }

    resp, err := k.http.Do(ctx, req)
    if err != nil { return nil, domain.WrapProviderDown(err, "kwik: fetch") }
    defer resp.Body.Close()
    body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
    if err != nil { return nil, domain.WrapProviderDown(err, "kwik: read") }

    m := packedJSRegex.FindSubmatch(body)
    if m == nil { return nil, domain.WrapExtractFailed(errors.New("no eval() packer"), "kwik") }

    // Convert eval(IIFE) → (IIFE) so RunString returns the unpacked string instead of executing it.
    wrapper := "(" + string(m[1]) + ")"

    vm := goja.New()  // FRESH PER CALL — goja is NOT thread-safe
    done := make(chan struct{})
    go func() {
        select {
        case <-time.After(k.timeout):
            vm.Interrupt("kwik: unpack timeout")
        case <-ctx.Done():
            vm.Interrupt("kwik: ctx cancel")
        case <-done:
        }
    }()
    val, err := vm.RunString(wrapper)
    close(done)
    if err != nil { return nil, domain.WrapExtractFailed(err, "kwik: goja") }

    m2 := sourceURLRegex.FindStringSubmatch(val.String())
    if m2 == nil { return nil, domain.WrapExtractFailed(errors.New("no const source="), "kwik") }
    return &domain.Stream{
        Sources: []domain.Source{{URL: m2[1], Type: "hls"}},
        Headers: map[string]string{"Referer": "https://kwik.cx/"},
    }, nil
}
```

### Pattern 3: DDoS-Guard cookie handling (Go sketch)

Source: `Kohi-den/extensions-source/.../DdosGuardInterceptor.kt`.

Detect: 403 with `Server: ddos-guard` header → GET `check.ddos-guard.net/check.js` → parse single-quoted path string → GET `<scheme>://<target_host>{path}` → response's `Set-Cookie: __ddg2_=...` lands in the jar automatically (cookiejar attached to BaseHTTPClient).

```go
func (p *Provider) ensureDDoSCookie(ctx context.Context, target *url.URL) error {
    for _, c := range p.http.Jar().Cookies(target) {
        if c.Name == "__ddg2_" && c.Value != "" { return nil }
    }
    checkResp, err := p.http.Get(ctx, "https://check.ddos-guard.net/check.js")
    if err != nil { return err }
    body, _ := io.ReadAll(io.LimitReader(checkResp.Body, 64<<10))
    checkResp.Body.Close()
    parts := strings.SplitN(string(body), "'", 3)
    if len(parts) < 3 { return errors.New("ddos-guard check.js shape changed") }
    bypassURL := target.Scheme + "://" + target.Host + parts[1]
    r, err := p.http.Get(ctx, bypassURL)
    if err != nil { return err }
    r.Body.Close()
    return nil
}
```

**ACTION REQUIRED in Phase 15 amend or Phase 16 prep commit:** `BaseHTTPClient.Jar()` accessor doesn't exist yet (`httpclient.go:38` keeps `jar` unexported). Add `func (c *BaseHTTPClient) Jar() http.CookieJar { return c.jar }` — one line, no test changes needed.

### Pattern 4: Video.js mid-playback source switch preserving currentTime (TypeScript)

```typescript
// Source: 16-UI-SPEC.md §ProviderSourceDropdown + HLS.js issue #2417 (CITED)
async function switchProvider(next: string) {
  const resumeAt = vjsPlayer?.currentTime() ?? nativeVideoRef.value?.currentTime ?? 0
  const previous = selectedProvider.value
  selectedProvider.value = next
  switchAbort?.abort(); switchAbort = new AbortController()
  try {
    await fetchStream({ prefer: next, signal: switchAbort.signal })
    await nextTick()
    if (vjsPlayer) {
      // Use 'loadeddata' NOT 'ready' — ready fires before source decode, currentTime() silently no-ops.
      vjsPlayer.one('loadeddata', () => vjsPlayer!.currentTime(resumeAt))
    } else if (nativeVideoRef.value) {
      nativeVideoRef.value.addEventListener('loadeddata',
        () => { nativeVideoRef.value!.currentTime = resumeAt }, { once: true })
    }
  } catch (e: any) {
    if (e.code !== 'ERR_CANCELED') {
      selectedProvider.value = previous
      providerOverrideToast.value = t('player.sourceSwitchFailed', { provider: previous ?? 'AnimePahe' })
      setTimeout(() => { providerOverrideToast.value = null }, 4000)
    }
  }
}
```

### Anti-Patterns to Avoid

- **Caching a goja Runtime.** NOT thread-safe (dop251/goja issue #97). Always `goja.New()` per Extract call. Discard after use (~1-2ms construction cost).
- **`vm.Interrupt()` from the same goroutine running the script.** Interrupt must come from a different goroutine. See Pattern 2.
- **Caching stream URLs past upstream signature expiry.** Kwik HLS URLs likely have `?expires=<unix>` (verify empirically). Cap TTL: `min(expires−30s, 5min)`.
- **Substring host matching** like `strings.Contains(host, "kwik.cx")` — `evilkwik.cx.attacker.com` matches. Use `host == known || HasSuffix("."+known)` (pattern from `embeds/megacloud.go:86-101`).
- **Sharing cookie jar across providers.** Per-provider `BaseHTTPClient` is the right granularity.
- **Unmounting old player components on tab switch.** UI-SPEC mandates `v-show:false` (not `v-if`) so state survives — but you MUST also suppress hidden-player `timeupdate` handlers (`if (!playerContainer.value?.offsetParent) return`) or both players double-write watch_history.
- **Re-pointing `hiAnimeApi`/`consumetApi` to scraper routes.** Locked in CONTEXT.md / SCRAPER-UI-03 — both die whole-cloth in Phase 20.
- **Returning `nil, ErrNotFound` for an aired anime with zero episodes.** Phase 15 contract: real-empty is `[]Episode{}, nil`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| MAL ID → AnimePahe mapping | Custom title-similarity search | `api.malsync.moe/mal/anime/{id}` + fuzzy AnimePahe search as fallback | malsync has hand-curated mappings (MAL Sync browser ext community). Verified live: `Sites.animepahe[ID].identifier` |
| Dean-Edwards JS unpack | Hand-implement base-N decoder | `dop251/goja` runs the unpacker JS | Goja is correct by construction; hand-decoders have charset off-by-ones at base ≥36 |
| DDoS-Guard cookies | JA3 spoofing, headless | Single GET to `check.ddos-guard.net/check.js`, parse path, GET it, store `__ddg2_` | Confirmed by Aniyomi (alive 5+ yrs). No JA3 check. CI forbids `utls`/`tls-client`/etc. |
| Per-host rate limit | Channel-based limiter | `BaseHTTPClient.WithPerHostRPS("animepahe.ru", 1.0, 2)` | Already wired |
| HTTP retry+backoff | Custom loop | `BaseHTTPClient` (retryablehttp 1→2→4→8s) | Already wired |
| Cookie jar etld+1 scoping | `map[string]*http.Cookie` | `cookiejar` + `publicsuffix.List` | Already wired `httpclient.go:83` |
| HLS playback | Roll your own MSE | Video.js + HLS.js | Already in package.json — fork HiAnimePlayer |
| JP subtitle rendering | New overlay component | Existing `SubtitleOverlay.vue` | 16-UI-SPEC mandates "no edits, no fork" |
| Bug-report UI | New modal | Existing `ReportButton.vue` + `diagnostics.ts` (extend props only) | 16-UI-SPEC §ReportButton extensions |
| HLS CORS proxy | New endpoint | Existing `libs/videoutils/proxy.go::ProxyWithReferer` | All 3 AnimePahe CDN hosts already allowlisted |

**Key insight:** Net-new code is overwhelmingly the AnimePahe HTML scraping (~150 LOC) + Kwik unpacker (~80 LOC). Plans proposing >800 LOC of net-new Go are over-engineering.

## Common Pitfalls

### Pitfall 1: AnimePahe session UUIDs are NOT stable, NOT in malsync

**What:** Caching session UUID across page loads → 404 on `/play/...`.

**Why:** AnimePahe regenerates sessions per page load. Aniyomi reference comments: "AnimePahe does not provide permanent URLs to its animes, so we need to fetch the anime session every time." malsync stores anime **ID** (e.g. `672`), NOT session.

**Avoid:** Cache anime ID at `malsync:{mal_id}:animepahe` for 24h. NEVER cache session UUIDs across requests. Cache episode list for 6h — fine because cached session is used immediately on read.

### Pitfall 2: goja Runtime sharing across goroutines

**What:** Data race → intermittent extraction failures + segfaults.

**Why:** Goja Runtime is documented NOT thread-safe (issue #97). Concurrent `Extract()` calls corrupt the runtime stack.

**Avoid:** `goja.New()` inside every `Extract()`. Discard after.

### Pitfall 3: Interrupt from same goroutine = no-op

**What:** `time.AfterFunc(5s, vm.Interrupt)` + infinite loop → never returns.

**Why:** Goja checks Interrupt flag between bytecode ops on the runtime's goroutine. If it's blocked in JS, AfterFunc scheduled on same goroutine doesn't fire.

**Avoid:** Explicit `go func() { ... vm.Interrupt() }()` BEFORE `vm.RunString`. See Pattern 2.

### Pitfall 4: Kwik HTML structure changes

**What:** Kwik changes script-tag layout; regex misses; all streams return 503.

**Avoid:** (a) Loose regex tolerant of whitespace; (b) `make capture-goldens` fixture pinned in `testdata/animepahe/kwik_e_*.html` + unit test; (c) Surface as `ErrExtractFailed` so Phase 18+ can fail over.

### Pitfall 5: malsync fuzzy fallback false positives

**What:** "Naruto: Shippuden" matches original "Naruto".

**Why:** AnimePahe `/api?m=search` is keyword-search, not exact-match. Common-word titles produce many hits.

**Avoid:** Require Jaro-Winkler similarity ≥ 0.85 against `ref.Title`. Score on normalized fold ("Season 2" / "2nd Season" / "Part 2"). Optional: year filter.

### Pitfall 6: Stream URL caching past upstream signature expiry

**What:** Signed URL expired 30s ago; we still serve it → 403.

**Avoid:** Parse expiry from URL query (regex `[?&]expires=(\d+)` or similar — verify empirically). TTL = `min(expires_unix - now - 30s, 5min)`. The 30s buffer is for clock skew.

### Pitfall 7: AbortController missing on stream-switch

**What:** User clicks "9anime" then immediately back to "AnimePahe". The 9anime response races and overwrites streamUrl.

**Avoid:** `switchAbort: AbortController` ref. `abort()` + recreate at start of every `switchProvider`. Pass `signal` into axios. Swallow `ERR_CANCELED` in catch.

### Pitfall 8: v-show hidden player still fires timeupdate

**What:** Both players mount when `?legacy=1`. Both fire `userApi.updateProgress`. watch_history bounces.

**Avoid:** In handler: `if (!playerContainer.value?.offsetParent) return` to no-op when display:none.

## Code Examples

### AnimePahe ListEpisodes with pagination (Go)

Source: derived from Kohi-den `AnimePahe.kt:166-197` `recursivePages` + `nextPageRequest`.

```go
type epDTO struct {
    Session       string  `json:"session"`
    EpisodeNumber float64 `json:"episode"`
    Title         string  `json:"title"`
    Filler        int     `json:"filler"`  // 0/1 — verify presence via golden fixture
    CreatedAt     string  `json:"created_at"`
}
type respDTO struct {
    CurrentPage int     `json:"current_page"`
    LastPage    int     `json:"last_page"`
    Data        []epDTO `json:"data"`
}

func (p *Provider) ListEpisodes(ctx context.Context, animeID string) ([]domain.Episode, error) {
    var out []domain.Episode
    for page := 1; page <= 50; page++ {  // 50-page hard cap (>1500 episodes implausible)
        url := fmt.Sprintf("%s/api?m=release&id=%s&sort=episode_asc&page=%d", p.baseURL, animeID, page)
        resp, err := p.http.Get(ctx, url)
        if err != nil { return nil, domain.WrapProviderDown(err, "animepahe: list") }
        body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
        resp.Body.Close()
        var dto respDTO
        if err := json.Unmarshal(body, &dto); err != nil {
            return nil, domain.WrapExtractFailed(err, "animepahe: decode")
        }
        for _, e := range dto.Data {
            out = append(out, domain.Episode{
                ID:       e.Session,
                Number:   int(math.Round(e.EpisodeNumber)),
                Title:    e.Title,
                IsFiller: e.Filler == 1,
            })
        }
        if dto.CurrentPage >= dto.LastPage { break }
    }
    return out, nil
}
```

### malsync lookup with cache (Go sketch)

Source: live API probed 2026-05-11 for MAL IDs 21 (One Piece) and 1535 (Death Note). Response shape confirmed: `{id, title, Sites: {animepahe: {[id]: {identifier, url:"https://animepahe.com/a/{id}", malId, ...}}}}`. Missing IDs → HTTP 404 with `{"name":"EntityNotFoundError","code":404}`.

```go
type MalSyncEntry struct {
    Identifier any    `json:"identifier"` // string for animepahe ("672"); use fmt.Sprintf("%v", ...)
    URL        string `json:"url"`
}
type MalSyncResponse struct {
    ID    int                                `json:"id"`
    Title string                             `json:"title"`
    Sites map[string]map[string]MalSyncEntry `json:"Sites"`
}

func (m *MalSyncClient) Lookup(ctx context.Context, malID, providerKey string) (string, bool) {
    cacheKey := fmt.Sprintf("malsync:%s:%s", malID, providerKey)
    if v, err := m.redis.Get(ctx, cacheKey).Result(); err == nil && v != "" { return v, true }
    if _, err := m.redis.Get(ctx, cacheKey+":miss").Result(); err == nil { return "", false }

    resp, err := m.http.Get(ctx, fmt.Sprintf("https://api.malsync.moe/mal/anime/%s", malID))
    if err != nil { return "", false }
    defer resp.Body.Close()
    if resp.StatusCode == 404 {
        m.redis.Set(ctx, cacheKey+":miss", "1", 24*time.Hour)
        return "", false
    }
    var parsed MalSyncResponse
    if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil { return "", false }
    site, ok := parsed.Sites[providerKey]  // case-sensitive: "animepahe" not "AnimePahe"
    if !ok || len(site) == 0 { return "", false }
    for _, e := range site {
        id := fmt.Sprintf("%v", e.Identifier)
        m.redis.Set(ctx, cacheKey, id, 24*time.Hour)
        return id, true
    }
    return "", false
}
```

## State of the Art

| Old | Current | Impact |
|-----|---------|--------|
| consumet.ts as reference | **DMCA-blocked since 2026-03-19** (verified `api.github.com/repos/consumet/consumet.ts/...` returns `{"reason":"dmca"}`) | Use Kohi-den/Aniyomi extensions — alive + maintained. License-compliant copying (MIT/Apache-2.0). |
| Headless browser for DDoS-Guard | Cookie-only handshake via `check.ddos-guard.net/check.js` | Pattern stable 4+ years across Aniyomi, miru-project, axios-ddos-guard-bypass |
| Custom JS unpacker | `dop251/goja` in-process | Locked by CONTEXT; allowed by CI |
| Cache stream URLs 1h | min(expires−30s, 5min) | Phase 15 NF — reduces 403 rate on signed-URL upstreams |
| `iframe_url` cross-tier fallback | **Structurally forbidden** by Stream DTO type | Phase 15 compile-time test enforces; AnimeLib Kodik-fallback was disabled in `9347143` |

**Deprecated:** HiAnime ecosystem (all dead 2026-05-09 triage), Consumet API container (stale + broken `enc-dec.app`). Both deleted in Phase 20.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Kwik HLS URLs contain a parseable Unix-timestamp expiry param (e.g. `?expires=…`) | Pitfall 6 | TTL parsing falls back to 5min cap silently — annoying but not broken. Plan must capture real Kwik URL + regression test. [ASSUMED] |
| A2 | Replacing outer `eval(` with `(` makes the IIFE return the unpacked string | Pattern 2 | If Kwik's variant assigns to a global instead of returning, RunString returns undefined → ErrExtractFailed cleanly. Falls over to next provider in Phase 18+. [ASSUMED] |
| A3 | AnimePahe response field names match Kohi-den Kotlin reference (`current_page`, `last_page`, `data`, `session`, `episode`, `filler`) | Code Example 1 | JSON decode silently zeros fields if names drift. Mitigation: golden fixture + unit test. [ASSUMED] |
| A4 | `BaseHTTPClient.Jar()` accessor is missing and must be added | Pattern 3 | One-line Phase 15 amend. Confirmed by reading `httpclient.go:38` (jar is unexported). [VERIFIED via grep] |
| A5 | `provider_health_up{stage}=0` for unhealthy items in Source dropdown is Phase 17 work; Phase 16 fail-opens | UI-SPEC | Confirmed by 16-UI-SPEC Edge Cases. [VERIFIED] |
| A6 | Jaro-Winkler 0.85 fuzzy threshold | Pitfall 5 | Judgement call. Plan should ship testset of expected match/no-match title pairs + tune. [ASSUMED] |
| A7 | goja construction ~1-2ms — fresh per call is acceptable | Pitfall 2 | If actually 50ms+, add `sync.Pool` (carefully — must `ClearInterrupt` + scope-reset). Not needed for Phase 16. [ASSUMED] |
| A8 | AnimePahe response includes `filler` field on episodes | Code Example 1 | Kohi-den doesn't surface it. SCRAPER-PAHE-02 says "where the upstream exposes it" — graceful zero-out is fine. Verify via golden. [ASSUMED] |

## Open Questions

1. **AnimePahe domain rotation frequency?** Kohi-den has a `PREF_DOMAIN_ENTRIES` ListPreference, suggesting rotation is expected. Make `ANIMEPAHE_BASE_URL` an env var with `https://animepahe.ru` default — restart-not-rebuild cycle. Cheap insurance.

2. **Will Phase 16 traffic trip AnimePahe's rate limit?** 1 RPS per host serializes naturally well under any plausible threshold for our ≤50-user deployment. Phase 17 observability will surface 429s if they occur. **Recommendation:** start with `WithPerHostRPS("animepahe.ru", 1.0, 2)`.

3. **Return all qualities or one from Kwik?** Kohi-den returns ALL (`document.select("div#resolutionMenu > button")`). Domain.Stream supports multiple `Sources[]`. **Recommendation:** return all available qualities ordered 1080p → 720p → 480p; HLS.js handles level selection.

4. **Is `/api/admin/scraper/health` needed in Phase 16?** No — SCRAPER-OBS-05 is Phase 17. Defer.

## Environment Availability

| Dependency | Required By | Available | Fallback |
|------------|------------|-----------|----------|
| docker + Go 1.23 + bun | All | ✓ (host) | — |
| Redis | malsync/episodes/stream caches | ✓ (existing) | None — cannot run phase without cache (would hammer malsync) |
| PostgreSQL | UUID → MAL ID (Phase 15) | ✓ | — |
| `api.malsync.moe` reachable | PAHE-01 | ✓ verified 2026-05-11 (HTTP 200 on /mal/anime/21, /mal/anime/1535) | Fuzzy fallback to AnimePahe search (mandated by PAHE-01) |
| `animepahe.ru` reachable from this host | PAHE-01..03 | ✗ verified — TCP timeout to 104.247.81.99:443 after 15s | **NONE — blocks the phase. See below.** |
| `kwik.cx` reachable | PAHE-03 | ✗ assumed similar (same upstream lineage) | Same |
| `check.ddos-guard.net` reachable | DDoS-Guard bootstrap | Untested but different IP range; likely OK | If unreachable, AnimePahe unscrapeable from this host |
| `dop251/goja` + `goquery` module fetchable | Compile | ✓ assumed (default goproxy) | None — pure Go |

**Missing dependencies with no fallback:**

- **`animepahe.ru` unreachable from this host (verified 2026-05-11 — TCP 104.247.81.99:443 timeout).** Could be (a) DDoS-Guard blocking the data-center IP, (b) regional block, (c) routing. **THE PLANNER MUST address this in the first plan task before any provider code is written.** Options: (1) retest from inside the docker container (different network namespace + possibly different egress IP via bridge networking), (2) add outbound HTTPS proxy / wireguard hop, (3) escalate to user. Provider code against an unreachable upstream is dead code.

## Validation Architecture

| Property | Value |
|----------|-------|
| Framework (Go) | stdlib `testing` + `testify` + `sebdah/goldie/v2` (Phase 15 pattern) |
| Framework (Frontend) | Vitest + Playwright (existing) |
| Quick run (Go) | `cd services/scraper && go test ./internal/providers/animepahe/... ./internal/embeds/... -count=1 -timeout 60s` |
| Quick run (Frontend) | `cd frontend/web && bunx vitest run src/components/player/EnglishPlayer.test.ts` |
| Full suite (Go) | `cd services/scraper && go test ./... -count=1 -timeout 120s && go vet ./...` |
| Full suite (Frontend) | `cd frontend/web && bunx vitest run && bunx playwright test english-player-integration` |
| Smoke | `curl -is http://localhost:8000/api/anime/<real-uuid>/scraper/episodes` returns 200 with `data[0].number == 1` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Command | Wave 0? |
|--------|----------|-----------|---------|---------|
| PAHE-01 | malsync lookup + 24h cache | unit | `go test ./internal/providers/animepahe -run TestMalSync_Lookup` | ❌ |
| PAHE-01 | Fuzzy fallback on malsync 404 | unit | `go test ./internal/providers/animepahe -run TestProvider_FindID_FuzzyFallback` | ❌ |
| PAHE-02 | Episodes pagination + 6h cache | unit | `go test ./internal/providers/animepahe -run TestProvider_ListEpisodes` | ❌ |
| PAHE-03 | Kwik unpack returns HLS m3u8 | unit | `go test ./internal/embeds -run TestKwik_Extract` | ❌ |
| PAHE-03 | Stream TTL = min(expires−30s, 5min) | unit | `go test ./internal/providers/animepahe -run TestProvider_GetStream_CacheTTL` | ❌ |
| PAHE-04 | DDoS-Guard cookie on 403 | unit (httptest) | `go test ./internal/providers/animepahe -run TestDDoSGuard_AcquireCookie` | ❌ |
| PAHE-05 | HLSProxyAllowedDomains contains kwik/owocdn/uwucdn | unit | `go test ./libs/videoutils -run TestHLSProxyAllowedDomains_HasAnimePahe` | ❌ (extend existing) |
| UI-01 | EnglishPlayer mounts Video.js + SubtitleOverlay | component | `bunx vitest run EnglishPlayer.test.ts` | ❌ |
| UI-02 | English tab replaces legacy; ?legacy=1 gates old tabs | e2e | `bunx playwright test english-player-tab` | ❌ |
| UI-03 | scraperApi shape | unit | `bunx vitest run client.test.ts -t scraperApi` | ❌ |
| UI-04 | Legacy tabs preserved on ?legacy=1 | e2e | `bunx playwright test legacy-flag` | ❌ |
| NF-05 | ReportButton payload has provider + tried[] | e2e | `bunx playwright test report-with-tried-chain` | ❌ |

### Sampling Rate

- Per task commit: relevant subset (Go OR Frontend).
- Per wave merge: full Go suite + full Frontend Vitest + `bunx tsc --noEmit` + 1 e2e smoke.
- Phase gate: both full suites green + `/animeenigma-after-update` green before `/gsd-verify-work`.

### Wave 0 Gaps

All test files listed in the map are NEW. Wave 0 (or each task's RED commit) creates them. Plus: `services/scraper/testdata/animepahe/{search_*.json, release_*.json, play_*.html, kwik_e_*.html}` captured via `make capture-goldens`. **Phase 15 amend:** add `BaseHTTPClient.Jar()` accessor (one line, no test changes).

## Security Domain

### ASVS

| Category | Applies | Control |
|----------|---------|---------|
| V2 Auth | no | Routes public, same trust level as legacy `/api/anime/{id}/hianime/*` |
| V3 Sessions | no | No session changes |
| V4 Access | no | Anonymous reads, no admin scope |
| V5 Input Validation | **yes** | (a) `prefer` query param: known-provider allowlist (`{"animepahe"}` in Phase 16) — `orderedProviders()` already ignores unknown silently; (b) `episode`/`server` IDs non-empty + URL-safe (handler does this, 400 on missing). UUID validation already at catalog (Phase 15-04). |
| V6 Crypto | no | Kwik HLS path = pure unpacking; no decryption keys |

### Threats

| Pattern | STRIDE | Mitigation |
|---------|--------|------------|
| SSRF via attacker-controlled `embedURL` to Kwik | Tampering / Info-disclosure | Strict host equality + `HasSuffix("."+known)` (mirrors `embeds/megacloud.go:86-101`). Add test: `kwik.cx.attacker.com` must NOT match. |
| goja sandbox escape (`Object`, `globalThis`, fs) | Tampering / Elevation | Goja default runtime has NO `os`/`fs`/`net`/`http`/`process`. Layered: fresh runtime per call + 5s Interrupt + 2MiB body limit (packed JS is ≤ 200KB in practice). |
| OOM via large response body | DoS | `io.ReadAll(io.LimitReader(resp.Body, 4<<20))` — mandatory at every read site. |
| Open redirect via stream URL to attacker host | Tampering | `isHLSDomainAllowed` is the gate. kwik/owocdn/uwucdn already allowlisted; no wildcards introduced. |
| XSS via `meta.tried[]` in ReportButton | XSS | Provider names are server-controlled. Vue `{{ }}` interpolation HTML-escapes. No `v-html`. |
| `__ddg2_` cookie bleeding cross-domain | Spoofing | `cookiejar` + `publicsuffix.List` already scopes to etld+1 (`httpclient.go:83`). |
| Stream URL signature replay | Tampering | Cache respects expiry via TTL math; attacker replay gets 403 from CDN, not from us. |

## Sources

### Primary (HIGH confidence)

- **Live probe — malsync.moe**: `/mal/anime/21`, `/mal/anime/1535` (2026-05-11). Confirmed `Sites.animepahe[ID].identifier` = AnimePahe anime ID; 404 = `EntityNotFoundError`.
- **Live probe — AnimePahe direct**: `curl https://animepahe.ru` → TCP timeout (documented as Environment Availability blocker).
- **Authoritative ref impl** — Kohi-den Aniyomi: `raw.githubusercontent.com/Kohi-den/extensions-source/main/src/en/animepahe/.../{AnimePahe.kt, kwik/KwikExtractor.kt, DdosGuardInterceptor.kt}` (fetched 2026-05-11; Apache-2.0 + MIT).
- **dop251/goja docs** — pkg.go.dev/github.com/dop251/goja (Runtime API, Interrupt semantics, thread-safety, version `v0.0.0-20260311135729-065cd970411c`).
- **Project code** — `services/scraper/internal/{domain,embeds,service}/*.go`, `libs/videoutils/proxy.go`, `frontend/web/src/components/player/HiAnimePlayer.vue`, `frontend/web/src/api/client.ts`.
- **Planning docs** — REQUIREMENTS.md, ROADMAP.md, STATE.md, 15-04-SUMMARY.md, 16-CONTEXT.md, 16-UI-SPEC.md.

### Secondary (MEDIUM confidence)

- WebSearch on DDoS-Guard cookie patterns — multiple independent confirmations (axios-ddos-guard-bypass, laxity7, miru-project) on `__ddg2_` + `check.ddos-guard.net/check.js` flow.
- HLS.js issue #2417 — currentTime preservation via `loadeddata`.
- `github.com/stephen-gardner/unpacker` — alt pure-Go Dean-Edwards unpacker (rejected; documented for completeness).

### Tertiary (LOW confidence)

- goja construction "~1-2ms" — not benchmarked in this session. Plan can include `go test -bench` if perf is a concern.
- Jaro-Winkler 0.85 threshold — empirically derived; plan should ship testset + tune.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every dep verified in go.mod / allowlist / live API
- Architecture: HIGH — components from Phase 15, this is wiring + scraping
- Pitfalls: HIGH — goja constraints in official docs, session-volatility documented by Kohi-den
- Environment: MEDIUM — `animepahe.ru` unreachable from host needs validation from inside docker container before plan task 1 starts

**Research date:** 2026-05-11
**Valid until:** 2026-06-10 (30 days for stable infra; AnimePahe domain/HTML/Kwik packer may rotate within 90 days but abstraction surfaces stable 6+ months).
