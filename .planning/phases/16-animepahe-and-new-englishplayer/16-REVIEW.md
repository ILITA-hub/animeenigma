---
phase: 16-animepahe-and-new-englishplayer
reviewed: 2026-05-12T00:00:00Z
depth: standard
files_reviewed: 35
files_reviewed_list:
  - Makefile
  - docker/docker-compose.yml
  - frontend/web/e2e/english-player.spec.ts
  - frontend/web/src/api/client.ts
  - frontend/web/src/components/player/EnglishPlayer.vue
  - frontend/web/src/components/player/ReportButton.vue
  - frontend/web/src/composables/useOverrideTracker.ts
  - frontend/web/src/composables/useWatchPreferences.ts
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ja.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/types/preference.ts
  - frontend/web/src/utils/diagnostics.ts
  - frontend/web/src/views/Anime.vue
  - libs/videoutils/proxy_test.go
  - services/scraper/cmd/scraper-api/main.go
  - services/scraper/internal/config/config.go
  - services/scraper/internal/config/config_test.go
  - services/scraper/internal/domain/httpclient.go
  - services/scraper/internal/domain/httpclient_test.go
  - services/scraper/internal/embeds/kwik.go
  - services/scraper/internal/embeds/kwik_test.go
  - services/scraper/internal/handler/scraper.go
  - services/scraper/internal/handler/scraper_test.go
  - services/scraper/internal/providers/animepahe/cache.go
  - services/scraper/internal/providers/animepahe/cache_test.go
  - services/scraper/internal/providers/animepahe/client.go
  - services/scraper/internal/providers/animepahe/client_test.go
  - services/scraper/internal/providers/animepahe/ddosguard.go
  - services/scraper/internal/providers/animepahe/ddosguard_test.go
  - services/scraper/internal/providers/animepahe/dto.go
  - services/scraper/internal/providers/animepahe/dto_test.go
  - services/scraper/internal/providers/animepahe/malsync.go
  - services/scraper/internal/providers/animepahe/malsync_test.go
  - services/scraper/internal/service/orchestrator.go
findings:
  critical: 5
  warning: 11
  info: 7
  total: 23
status: issues_found
---

# Phase 16: Code Review Report

**Reviewed:** 2026-05-12
**Depth:** standard
**Files Reviewed:** 35
**Status:** issues_found

## Summary

Phase 16 introduces a new English-source player (`EnglishPlayer.vue`), a Go scraper service with AnimePahe provider, an in-process Kwik embed extractor, and end-to-end wiring through catalog → scraper. The implementation includes thoughtful adversarial defenses (host-equality SSRF checks, body caps, goja runtime per-call, fail-closed domain allowlist, fuzzy-fallback threshold, DDoS-Guard host-pinning, etc.), strong unit-test coverage, and consistent error envelopes.

However, this review surfaces **five BLOCKER-class defects** that will produce broken user behavior or crashes in production, plus eleven WARNING-class defects:

- **Host port collision** between the `scraper` and `admin-nginx` containers (both bind `127.0.0.1:8088`) — `docker compose up` will fail OR one of the two services will silently lose its host port.
- **No category (sub/dub) plumbing in AnimePahe responses.** `domain.Server` has no `Type` field, the AnimePahe provider hard-codes `Name: "kwik"`, and the frontend filters servers by `s.type === 'sub' | 'dub'`. Result: `subServers.length === 0` and `dubServers.length === 0` → the player UI never offers a playable server.
- **`MAL_ID` is never sent on any scraper call.** `scraperApi.getEpisodes/getServers/getStream` never include a `mal_id` query parameter, but the scraper handler unconditionally requires it (`400 INVALID_INPUT`). Every English-player request will 400.
- **Tailwind dynamic class strings (`bg-[${color}]/20`) do not work** in the ReportButton — the build-time JIT cannot extract them, so the report submit button is unstyled.
- **DDoS-Guard cookie-name match is wrong** for current upstream — code checks `__ddg2_` exact, but real Set-Cookie names are `__ddg2_<random>` (variable suffix), so `ensureDDoSCookie` always re-runs.

The English-player composable and scraper test coverage are solid; most BLOCKERs are integration-layer / contract bugs that the unit tests don't catch.

## Critical Issues

### CR-01: Host port 8088 is bound twice in docker-compose.yml

**File:** `docker/docker-compose.yml:166, 296`
**Issue:** Both the `scraper` and `admin-nginx` services bind `127.0.0.1:8088` on the host:
```yaml
scraper:
  ports:
    - "127.0.0.1:8088:8088"     # line 166

admin-nginx:
  ports:
    - "127.0.0.1:8088:80"       # line 296
```
`docker compose up` will fail with an address-already-in-use error for whichever container starts second; `make redeploy-scraper` (which only rebuilds + restarts `scraper`) appears to work because `admin-nginx` is already running, then breaks the admin nginx vhost. The `Makefile health` target also probes `localhost:8088` for the scraper, which after admin-nginx steals the bind would silently health-check the WRONG service.

**Fix:** Reassign one of the two services. Recommend moving `admin-nginx` to a non-collision port (e.g. 8089) since `8088` is the documented scraper port in `CLAUDE.md` and the `make health` target:
```yaml
admin-nginx:
  ports:
    - "127.0.0.1:8089:80"
```

### CR-02: AnimePahe Server entries lack `Type` (sub/dub) — frontend filter always empty

**File:** `services/scraper/internal/providers/animepahe/client.go:386`; `services/scraper/internal/domain/provider.go:49-52`; `frontend/web/src/components/player/EnglishPlayer.vue:693-697`
**Issue:** The frontend's `ScraperServer` interface declares `type: string // sub, dub, raw` (line 453-457 of EnglishPlayer.vue) and the entire server-selection UI gates on it:
```ts
const subServers = computed(() => servers.value.filter(s => s.type === 'sub'))
const dubServers = computed(() => servers.value.filter(s => s.type === 'dub'))
const filteredServers = computed(() => selectedCategory.value === 'sub' ? subServers.value : dubServers.value)
```
But the Go `domain.Server` struct has no `Type` field at all, and `ListServers` always appends `domain.Server{ID: src, Name: "kwik"}`. The JSON payload therefore lacks a `type` key; on the frontend each server's `.type` is `undefined`, so both `subServers.length` and `dubServers.length` are **always 0**. The user sees `Sub (0) / Dub (0)` and "No subtitles available" regardless of what AnimePahe returned. The auto-select code at line 826-834 also skips the fallback because `servers.value[0].type as 'sub' | 'dub'` is `undefined`.

Additionally, AnimePahe's `/play/{anime}/{episode}` page surfaces sub/dub variants as separate `button[data-src]` entries with `data-audio="jpn"` / `data-audio="eng"` (or as suffixes on the kwik URL). The current scraper drops that signal entirely.

**Fix:** Add `Type domain.Category` to the `domain.Server` struct and populate it in `ListServers`. The AnimePahe scrape needs to read the audio attribute or the surrounding `<a data-audio="...">` element:
```go
// domain/provider.go
type Server struct {
    ID   string          `json:"id"`
    Name string          `json:"name"`
    Type Category        `json:"type"` // CategorySub | CategoryDub
}

// providers/animepahe/client.go ListServers — pseudocode:
doc.Find("button[data-src]").Each(func(_ int, sel *goquery.Selection) {
    src, _ := sel.Attr("data-src")
    audio, _ := sel.Attr("data-audio") // or scan ancestor element
    cat := domain.CategorySub
    if strings.EqualFold(audio, "eng") || strings.EqualFold(audio, "dub") {
        cat = domain.CategoryDub
    }
    servers = append(servers, domain.Server{ID: src, Name: "kwik", Type: cat})
})
```
Add an `ListServers` test that asserts at least one `Type: "sub"` server.

### CR-03: scraperApi never sends `mal_id`; every scraper request returns 400

**File:** `frontend/web/src/api/client.ts:416-440`; `services/scraper/internal/handler/scraper.go:96-99, 127-130, 162-165`
**Issue:** The scraper handler unconditionally requires `mal_id`:
```go
if qp.malID == "" {
    h.writeError(w, http.StatusBadRequest, codeInvalidInput, "mal_id is required", tried)
    return
}
```
But the frontend's `scraperApi` builds its URL from `animeId` (a UUID) and never passes `mal_id`:
```ts
getEpisodes: (animeId: string, prefer?: string) =>
    apiClient.get(`/anime/${animeId}/scraper/episodes`, {
        params: prefer ? { prefer } : undefined,
    }),
```
The catalog passthrough (referenced in the handler doc comment but not in this review's file scope) is supposed to map `animeId → mal_id` and forward — but as written, no `mal_id` is ever in scope unless the catalog injects it. If the catalog forwards the request query-string unchanged, every English-player call returns `400 INVALID_INPUT mal_id is required`. The e2e test (`english-player.spec.ts`) won't catch this because it asserts on UI presence, not network status, and falls back to "still pass if not visible" guards.

**Fix:** Resolve mal_id catalog-side and inject it as a query param before forwarding. The catalog handler at `services/catalog/internal/handler/scraper.go` is referenced by the SUMMARY but must be verified to actually inject `mal_id=...` into the upstream URL. Failing that, change the contract so `scraperApi` POSTs `{anime_id}` and the catalog resolves locally.

Also add a contract test that exercises `GET /anime/{uuid}/scraper/episodes` against a fake catalog → scraper pipeline and asserts non-400.

### CR-04: ReportButton's dynamic Tailwind classes are unstyled at build time

**File:** `frontend/web/src/components/player/ReportButton.vue:146-148`
**Issue:**
```ts
const submitButtonClasses = computed(() => {
  return `bg-[${props.accentColor}]/20 text-[${props.accentColor}] hover:bg-[${props.accentColor}]/30`
})
```
Tailwind's JIT extracts class names by **static** scanning at build time. A computed template string like `` `bg-[${color}]/20` `` will never be in the generated CSS — the resulting class will produce no styles. The submit button in the report modal therefore renders with no background color and no text color (i.e. invisible text on white) for every player that passes a non-default `accentColor` (EnglishPlayer passes `#00d4ff`).

**Fix:** Either use CSS variables (consistent with `EnglishPlayer.vue`'s `--player-accent` pattern) or use Tailwind's safelist via known accent variants. CSS-variable solution:
```vue
<button
  :style="{
    '--report-accent': accentColor,
  }"
  class="bg-[var(--report-accent)]/20 text-[var(--report-accent)] hover:bg-[var(--report-accent)]/30"
  ...
>
```
Note `bg-[var(--report-accent)]/20` requires Tailwind 3.4+ for the opacity-modifier syntax; otherwise fall back to inline `:style="{ backgroundColor: ... }"`.

### CR-05: DDoS-Guard cookie-name match fails — real cookies are `__ddg2_<suffix>`, not exactly `__ddg2_`

**File:** `services/scraper/internal/providers/animepahe/ddosguard.go:18, 60-62, 118-121`
**Issue:** The constant `ddosCookieName = "__ddg2_"` is compared with `c.Name == ddosCookieName`. Real DDoS-Guard cookies in 2025/2026 use a versioned suffix (commonly `__ddg2_BvHvjMmh`, `__ddg2_xxxxx`) so the exact-match check never fires:
1. `ensureDDoSCookie` always thinks the jar is empty (idempotency short-circuit at line 60-64 never triggers).
2. After the handshake, the post-bypass jar check at line 118-122 also fails → returns `ErrExtractFailed` with `"__ddg2_ cookie not set after bypass GET"` even when the bypass DID work.

The result is one or both of: (a) the handshake re-runs on every request (extra 2 RTTs/second on an upstream with a 1 RPS rate limit — i.e. the request budget is permanently halved); (b) every AnimePahe request returns ErrExtractFailed once the upstream actually goes through DDoS-Guard.

**Fix:** Change to a `HasPrefix` match:
```go
const ddosCookieNamePrefix = "__ddg2_"

// in jar checks:
for _, c := range jar.Cookies(target) {
    if strings.HasPrefix(c.Name, ddosCookieNamePrefix) && c.Value != "" {
        return nil
    }
}
```
Add a test that pre-populates the jar with `__ddg2_BvHvjMmh=abc` and asserts `ensureDDoSCookie` returns nil without making HTTP calls.

## Warnings

### WR-01: Scraper handler's `prefer` parameter is forwarded but the orchestrator uses the unsafe handler input as a provider name

**File:** `services/scraper/internal/handler/scraper.go:79, 90`; `services/scraper/internal/service/orchestrator.go:78-86`
**Issue:** `parseQuery` only trims `prefer`; there's no length cap, no whitelist check, no character-set validation. While the orchestrator's `orderedProviders` defensively scans the registered providers' names for an exact match (so an attacker can't inject a different provider), `tried[]` is rendered into `meta.tried` and returned to the client verbatim. A malicious `?prefer=AAAA...` (10 MB) would balloon every response. Also, the value is logged via `log.Warnw("scraper: provider failover", ...)` which surfaces in Loki — a high-cardinality field shouldn't accept unbounded user input.

**Fix:** Clamp at parse time:
```go
prefer := strings.TrimSpace(q.Get("prefer"))
if len(prefer) > 64 {
    prefer = prefer[:64]
}
```
Note: because `OrderedProviderNames("AAAA")` returns the registration order unchanged when no provider matches, the raw value never actually appears in `meta.tried`. But the BadRequest path (`mal_id` missing) writes `tried` from `OrderedProviderNames(qp.prefer)`, which still ignores unknown — so the immediate leak is limited. Still: clamp the input.

### WR-02: `handleFullscreenChange` operator precedence bug

**File:** `frontend/web/src/components/player/EnglishPlayer.vue:1527`
**Issue:**
```ts
if (fsEl && playerContainer.value?.contains(fsEl) || fsEl === playerContainer.value) {
    vjsPlayer.addClass('vjs-fullscreen')
}
```
`&&` binds tighter than `||`, so this parses as `(fsEl && playerContainer.value?.contains(fsEl)) || (fsEl === playerContainer.value)`. In practice both branches require `fsEl` to be non-null, so the right-hand `fsEl === playerContainer.value` already requires `fsEl != null`. However the intent reads like `fsEl != null && (playerContainer.contains(fsEl) || fsEl === playerContainer)`. The current expression accidentally also triggers when `fsEl === playerContainer.value` even if `fsEl` is null (because `null === null` would be true) — and when that happens, `vjsPlayer.addClass(...)` runs in a "not really fullscreen" state, adding the class spuriously. Mostly harmless because `playerContainer.value` is typed as `HTMLDivElement | null` and starts non-null after mount, but the intent vs. implementation mismatch is a foot-gun.

**Fix:**
```ts
if (!fsEl) {
    vjsPlayer.removeClass('vjs-fullscreen')
    return
}
if (playerContainer.value?.contains(fsEl) || fsEl === playerContainer.value) {
    vjsPlayer.addClass('vjs-fullscreen')
}
```

### WR-03: `deactivateSubtitle` sets `activeSubtitleUrl` to empty string, not null

**File:** `frontend/web/src/components/player/EnglishPlayer.vue:1005`
**Issue:**
```ts
const deactivateSubtitle = () => {
  activeSubtitleUrl.value = ''
  ...
}
```
But `activeSubtitleUrl` is typed `ref<string | null>(null)` (line 607) and other places use `null` (`activateStreamSubtitle` does NOT clear, `activateJimakuSubtitle` sets to `sub.url`, the watcher checks `!activeSubtitleUrl`). The "Off" button's active state at line 280 is `:class="!activeSubtitleUrl ? 'accent-bg-muted accent-text' : ...`. With empty-string, `!"" === true`, so the visual state is correct — but the `SubtitleOverlay` component receives `''` instead of `null`, which depending on the overlay implementation might trigger a load-of-empty-url network call. Inconsistent typing for the same field is the real defect.

**Fix:** Use `null`:
```ts
activeSubtitleUrl.value = null
```

### WR-04: `resumeStartEpisode` typed as `number | undefined` but passed to a prop that does the wrong thing on undefined

**File:** `frontend/web/src/views/Anime.vue:920-929`; `frontend/web/src/components/player/EnglishPlayer.vue:500, 765-768`
**Issue:** When `resumeStartEpisode` is `undefined`, the EnglishPlayer's `props.initialEpisode` is also `undefined`, and the auto-select logic does:
```ts
const initialEp = props.initialEpisode
    ? episodes.value.find(e => e.number === props.initialEpisode) || episodes.value[0]
    : episodes.value[0]
```
That works. But if `props.initialEpisode === 0` (which the type `number | undefined` permits and `lastEpisode.value ?? 1` produces only when set to 0 — see `loadLastEpisode` parsing), the truthy check `props.initialEpisode ?` evaluates `false` and we silently skip the find. With a sane localStorage history, episode 0 should never appear, but `parseInt(ep)` where `ep === "0"` returns 0, and the validity guard at line 902 (`if (latestEp && !isNaN(latestEp))`) treats 0 as falsy. Currently this is benign but fragile.

**Fix:** Use explicit `!= null` or default to 1 explicitly:
```ts
const initialEp = (props.initialEpisode && props.initialEpisode > 0)
    ? episodes.value.find(e => e.number === props.initialEpisode) || episodes.value[0]
    : episodes.value[0]
```

### WR-05: `kwikHosts` test gap — `Matches` will accept `kwik.cx/path` URLs from any scheme including `file://`

**File:** `services/scraper/internal/embeds/kwik.go:272-287`
**Issue:** `url.Parse("file:///kwik.cx/passwd")` returns a URL whose Hostname is "" (empty) — that's actually filtered out by `if host == "" { return false }`. But `url.Parse("kwik.cx/path")` (no scheme) returns Hostname="" too, so this is OK. However, `url.Parse("https://user:pass@kwik.cx@attacker.com/x")` parses Hostname as "attacker.com" — the host validates against the LAST `@`, which is correct (Go's behavior). The actual concern: `url.Parse("kwik://kwik.cx/")` succeeds with Hostname=`kwik.cx`, schema=`kwik`, and `Matches` returns `true`. The orchestrator then dispatches Extract, which builds an `http.Request` with the `kwik://` URL — Go's HTTP client rejects unknown schemes, returning a fetch error, mapped to `ErrProviderDown`. No SSRF, just an unhelpful error mapping.

**Fix:** Add an explicit scheme check:
```go
func (k *KwikExtractor) Matches(embedURL string) bool {
    u, err := url.Parse(embedURL)
    if err != nil { return false }
    if u.Scheme != "http" && u.Scheme != "https" { return false }
    // ... rest
}
```
The same applies to AnimePahe's `ListServers` matcher (line 381-385 of `client.go`).

### WR-06: AnimePahe `GetStream` does not honor the requested `category` parameter

**File:** `services/scraper/internal/providers/animepahe/client.go:404`
**Issue:** The `category domain.Category` parameter is accepted but never used. The provider always returns the stream URL associated with the `serverID` (kwik URL) it was given, with no audio-language filtering. If the upstream returns both sub and dub kwik URLs and the frontend correctly tagged them in CR-02's fix, this is fine — but currently the GetStream contract violates "respect category", which the orchestrator's signature suggests it does.

**Fix:** Either remove the `Category` parameter from the AnimePahe-specific signature, OR validate that `serverID`'s recorded type matches the requested category and return `ErrNotFound` otherwise. Document explicitly in the function comment: "The category parameter is informational; selection happens at ListServers time."

### WR-07: malsync provider can pick a non-deterministic entry from a map iteration

**File:** `services/scraper/internal/providers/animepahe/malsync.go:170-176`
**Issue:**
```go
for _, entry := range site {
    id := fmt.Sprintf("%v", entry.Identifier)
    if id != "" && id != "<nil>" {
        _ = m.cache.Set(ctx, hitKey, id, malSyncCacheTTL)
        return id, true, nil
    }
}
```
`site` is a `map[string]malSyncEntry`. Go map iteration order is randomized. If malsync returns multiple entries for the same anime (e.g. main + alt), the resulting AnimePahe ID is non-deterministic across processes/restarts — cache key remains stable but the cached VALUE may be different each cold start. Over time this is just "weird inconsistent behavior", not a hard failure.

**Fix:** Sort the keys before iterating, or pick the lexicographically smallest entry:
```go
keys := make([]string, 0, len(site))
for k := range site {
    keys = append(keys, k)
}
sort.Strings(keys)
for _, k := range keys {
    entry := site[k]
    ...
}
```

### WR-08: Diagnostics console interception holds references to large argument objects forever

**File:** `frontend/web/src/utils/diagnostics.ts:67-87`
**Issue:** `addConsoleEntry` JSON-stringifies every argument and stores the string. The stringify itself is bounded (`.slice(0, 2000)`), but `JSON.stringify` of complex DOM nodes / Vue refs can throw (caught) or produce massive strings before slicing. More importantly: if any console.log call passes a very deep object graph, `JSON.stringify` walks the entire graph synchronously — which on a hot path (e.g. logging a video event 60×/sec) can cause main-thread stalls and contribute to the `requestAnimationFrame` budget overrun on slow devices.

**Fix:** Use a safe stringifier with depth cap, e.g.:
```ts
function safeStringify(v: unknown, maxLen = 2000): string {
    try {
        const seen = new WeakSet()
        const s = JSON.stringify(v, (_, val) => {
            if (typeof val === 'object' && val !== null) {
                if (seen.has(val as object)) return '[Circular]'
                seen.add(val as object)
            }
            return val
        })
        return (s ?? String(v)).slice(0, maxLen)
    } catch {
        return String(v).slice(0, maxLen)
    }
}
```

### WR-09: e2e test depends on a magic UUID that may not exist in the seed

**File:** `frontend/web/e2e/english-player.spec.ts:23`
**Issue:**
```ts
const TEST_ANIME_ID = 'c076bca7-a93f-4089-90a3-0cb69b9cbf25' // Frieren S2 (likely-covered MAL ID)
```
The UUID is hard-coded but the comment admits it's only "likely-covered." There's no setup that inserts/verifies this UUID exists in the seed before running. If the seed user's anime catalog drifts (rebuild without Frieren), the test silently fails with `episodes.length === 0` and the empty-state copy assertion in test 1 — which is conditionally skipped via `if (await activateBtn.isVisible(...).catch(() => false))`. The test will then "pass" by asserting an English tab exists but no real player behavior is verified.

**Fix:** Resolve the test anime at runtime by hitting the anime search API (e.g. `GET /api/anime/search?q=Frieren`) and picking the first result. Fail the test explicitly if zero hits.

### WR-10: `parseInt(ep)` in localStorage parse missing radix

**File:** `frontend/web/src/views/Anime.vue:899`
**Issue:**
```ts
latestEp = parseInt(ep)
```
`parseInt` without a radix argument is a long-standing JS footgun — historic browsers treated leading-zero strings as octal. Modern V8 doesn't, but ESLint's `radix` rule still flags this, and the codebase uses `parseFloat`/explicit radix elsewhere. With a localStorage key like `08`, modern browsers return 8; on a very old browser it could return 0.

**Fix:**
```ts
latestEp = parseInt(ep, 10)
```

### WR-11: `Provider.New` panics on missing dependencies rather than failing at construction

**File:** `services/scraper/internal/providers/animepahe/client.go:115-138`
**Issue:** The doc comment says "missing dependencies result in panic at first use," but `New` never validates that `d.HTTP`, `d.Embeds`, `d.MalSync`, `d.Cache` are non-nil. A subtle typo at the call site in `main.go` (omitting one of these) silently constructs a Provider that nil-panics on FindID. Given that scraper service `main.go` does NOT wrap startup in recover and is fronted by docker's healthcheck, the resulting `nil pointer dereference` will surface to the user via a sudden 502 minutes after deploy — not at boot.

**Fix:** Validate eagerly:
```go
func New(d Deps) (*Provider, error) {
    if d.HTTP == nil { return nil, errors.New("animepahe: Deps.HTTP is required") }
    if d.Embeds == nil { return nil, errors.New("animepahe: Deps.Embeds is required") }
    if d.MalSync == nil { return nil, errors.New("animepahe: Deps.MalSync is required") }
    if d.Cache == nil { return nil, errors.New("animepahe: Deps.Cache is required") }
    if d.Log == nil { d.Log = logger.Default() }
    ...
    return p, nil
}
```
Main.go then fatals on the error.

## Info

### IN-01: `failedQueue` leak risk on permanent refresh failure

**File:** `frontend/web/src/api/client.ts:20-23, 165-176`
**Issue:** `failedQueue` is module-level mutable state. If a `doTokenRefresh` call rejects with a network error (not 401/403), `processQueue(refreshError, null)` rejects the queue items — but the 10-second timeout setter at line 166 races: if the timeout fires AFTER processQueue, the same item is double-rejected (one reject + one timeout reject), which is harmless on Promise contract but creates redundant `Promise.reject` chains in the network logs.

**Fix:** Track resolved/rejected state per queue entry:
```ts
let failedQueue: Array<{
    resolved: boolean
    resolve: ...
    reject: ...
}> = []
```
Or call `clearTimeout` from inside `processQueue` for every item.

### IN-02: Hardcoded localhost MEGACLOUD_EXTRACTOR_URL default in non-docker environment

**File:** `services/scraper/internal/config/config.go:79`
**Issue:** Default `"http://megacloud-extractor:3200"` only works when running inside docker-compose's `animeenigma-network`. A developer running `go run ./cmd/scraper-api` host-natively gets a `dial tcp: lookup megacloud-extractor: no such host` failure with no helpful message.

**Fix:** Document this in the Config struct comment or add a host-mode default detection (e.g. fall back to `http://localhost:3200` when `os.Getenv("DOCKER_ENV") == ""`). Lowest priority.

### IN-03: TypeScript `as` cast hides envelope shape drift

**File:** `frontend/web/src/components/player/EnglishPlayer.vue:563, 564, 750, 790, 902-911`
**Issue:** Numerous `as { data?: ... }` casts in the envelope-unwrap helpers. If the backend changes the envelope shape (e.g. drops `meta.tried` from data), TypeScript silently accepts the runtime undefined. Better: define a typed `ScraperEnvelope<T>` interface and assert at the type boundary.

**Fix:** Add to `frontend/web/src/types/scraper.ts`:
```ts
export interface ScraperEnvelope<T> {
    success: boolean
    data?: T & { meta?: { tried?: string[] } }
    meta?: { tried?: string[] }
    error?: { code: string; message: string }
}
```
Use in EnglishPlayer.vue:
```ts
const env = response.data as ScraperEnvelope<{ episodes: ScraperEpisode[] }>
```

### IN-04: `localStorage.getItem('preferred_video_provider')` allows stale `'consumet'` / `'hianime'` after Phase 20 cutover

**File:** `frontend/web/src/views/Anime.vue:861-863`
**Issue:** The default-provider type allows `'kodik' | 'animelib' | 'hianime' | 'consumet' | 'hanime' | 'english'`. Users with a stale `'hianime'` in localStorage from before Phase 16 will get a HiAnime tab that the v-else-if chain still mounts — but the EN sub-tabs only render the `english` button (hianime/consumet are gated on `?legacy=1`). Result: when `videoLanguage === 'en'` and `videoProvider === 'hianime'`, the user sees the EN tab highlighted, no provider sub-tabs, but the HiAnime player rendered. Reproducible UX confusion.

**Fix:** When `preferred_video_provider` reads as `'hianime'` or `'consumet'` and we're not in `?legacy=1`, coerce to `'english'`:
```ts
const savedProvider = localStorage.getItem('preferred_video_provider')
const isLegacyAllowed = route.query.legacy === '1'
const validProvider = (savedProvider === 'hianime' || savedProvider === 'consumet') && !isLegacyAllowed
    ? 'english'
    : (savedProvider as ...) || 'kodik'
```

### IN-05: Duplicate `failedQueue` clearance pattern between request and response interceptors

**File:** `frontend/web/src/api/client.ts:163-176`
**Issue:** The request interceptor calls `doTokenRefresh()` which clears the queue, but the response interceptor's failed-queue setup at line 163-176 is conceptually orthogonal. There's no test exercising "request interceptor refresh PLUS response 401 from a concurrent request" — the two queues could interact unexpectedly.

**Fix:** Add an integration test that fires N concurrent expired-token requests and asserts only ONE `POST /auth/refresh` happens.

### IN-06: Hard-coded `30 * 60` (auto-mark threshold) repeats existing magic numbers

**File:** `frontend/web/src/components/player/EnglishPlayer.vue:593`
**Issue:**
```ts
const AUTO_MARK_THRESHOLD = 20 * 60 // 20 minutes
```
The same threshold is presumably duplicated in HiAnimePlayer / ConsumetPlayer / KodikPlayer / AnimeLibPlayer. Five sources of truth for one business rule.

**Fix:** Extract to a shared constants module: `frontend/web/src/composables/usePlayerConstants.ts`.

### IN-07: AnimePahe pagination loop ignores the per-page maximum returned by upstream

**File:** `services/scraper/internal/providers/animepahe/client.go:283-326`
**Issue:** The loop checks `if rr.CurrentPage >= rr.LastPage { break }` — which is correct as long as the upstream returns honest `current_page`/`last_page`. If a misbehaving upstream returns `last_page: 9999`, we'd loop until `maxEpisodePages = 50` and request 50 pages — which at 1 RPS = 50 seconds of rate-limited fetches. The `for page := 1; page <= maxEpisodePages; page++` cap protects against this, but each fetch costs ~1 second of user-visible latency.

**Fix:** Also cap the total assembled episode count (e.g. `if len(all) > 5000 { break }`). Low priority.

---

_Reviewed: 2026-05-12_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
