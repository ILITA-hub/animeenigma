---
phase: 18-9anime
reviewed: 2026-05-12T00:00:00Z
depth: standard
files_reviewed: 32
files_reviewed_list:
  - frontend/web/public/changelog.json
  - frontend/web/src/components/player/EnglishPlayer.vue
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ja.json
  - frontend/web/src/locales/ru.json
  - libs/videoutils/proxy.go
  - libs/videoutils/proxy_test.go
  - services/scraper/cmd/scraper-api/main.go
  - services/scraper/internal/config/config.go
  - services/scraper/internal/config/config_test.go
  - services/scraper/internal/domain/httpclient.go
  - services/scraper/internal/domain/httpclient_test.go
  - services/scraper/internal/embeds/earnvids.go
  - services/scraper/internal/embeds/earnvids_test.go
  - services/scraper/internal/embeds/kwik.go
  - services/scraper/internal/embeds/packed_common.go
  - services/scraper/internal/embeds/packed_common_test.go
  - services/scraper/internal/embeds/streamhg.go
  - services/scraper/internal/embeds/streamhg_test.go
  - services/scraper/internal/embeds/vibeplayer.go
  - services/scraper/internal/embeds/vibeplayer_test.go
  - services/scraper/internal/providers/gogoanime/cache.go
  - services/scraper/internal/providers/gogoanime/cache_test.go
  - services/scraper/internal/providers/gogoanime/client.go
  - services/scraper/internal/providers/gogoanime/client_test.go
  - services/scraper/internal/providers/gogoanime/doc.go
  - services/scraper/internal/providers/gogoanime/dto.go
  - services/scraper/internal/providers/gogoanime/dto_test.go
  - services/scraper/internal/providers/gogoanime/helpers_test.go
  - services/scraper/internal/providers/gogoanime/malsync.go
  - services/scraper/internal/providers/gogoanime/malsync_test.go
  - services/scraper/internal/service/orchestrator_phase18_test.go
findings:
  critical: 3
  warning: 9
  info: 6
  total: 18
status: issues_found
---

# Phase 18: Code Review Report

**Reviewed:** 2026-05-12T00:00:00Z
**Depth:** standard
**Files Reviewed:** 32
**Status:** issues_found

## Summary

Phase 18 lands the Anitaku/Gogoanime provider (second EN scraper), three new embed
extractors (`vibeplayer`, `streamhg`, `earnvids`), the `WithTransport` option on the
shared `BaseHTTPClient`, a new HLS-proxy allowlist row, and the multi-option Source
dropdown in `EnglishPlayer.vue`. The SSRF gate (`Matches()` policy: host equality +
`HasSuffix("."+known)`) is correctly implemented across all three new extractors
and is well-tested.

Three BLOCKER-class defects were found:
1. **Unvalidated subtitle URL in VibePlayer extractor** — captured from the embed
   page with no scheme/host check, then flows directly into `stream.Tracks[]`.
   A hostile or compromised vibeplayer.site can inject arbitrary URLs (incl.
   `javascript:`, `file:`, `data:`, or attacker domains) that the player loads.
2. **`computeStreamTTL` parse-error path caches an effectively-expired URL** —
   `streamTTLFallback = streamTTLCap = 5min` is returned both for "no expiry"
   AND for "malformed `e=`" — a malformed param could legitimately be a sign of
   a CDN URL that *is* expiring in seconds, but the cache holds it for the full
   5min cap. The fallback should distinguish these cases.
3. **Saved provider preference is lost on cold start** when the saved value isn't
   in the default `['animepahe']` array. The check runs in `<script setup>`
   before `onMounted()` populates the real provider list from `getHealth()`,
   so a user who chose `gogoanime` last session reopens the player on AnimePahe.

The two `packed_common.go` extractors share a `defaultPackedHTTPTimeout = 15s`
constant for BOTH the HTTP fetch AND the goja runtime budget — the comment
claims this "matches the Kwik extractor's defaults (both 15s)" but Kwik's
goja budget is 5s. A malicious upstream can now pin a goroutine for 15s of CPU
instead of 5s. The metric selector emitted on goja failure is also wrong
(`selectorPackerFail` instead of a goja-specific identifier), poisoning the
selector-drift signal that Phase 17 observability relies on.

## Critical Issues

### CR-01: VibePlayer subtitle URL is not validated against any allowlist

**File:** `services/scraper/internal/embeds/vibeplayer.go:63,166-174`
**Issue:** `vibePlayerSubRegex = regexp.MustCompile('const\s+subtitle\s*=\s*"([^"]*)"')`
captures **any** string between the quotes — including `javascript:`,
`file://`, `data:`, or an arbitrary attacker-controlled URL. The captured string
is shoved into `stream.Tracks[0].File` with no scheme check, no host check, and
no allowlist consultation. The frontend's `SubtitleOverlay.vue` consumes this
URL and either fetches it directly or routes it through the HLS proxy. The HLS
proxy gate (`isHLSDomainAllowed`) is a defence-in-depth layer, but:
- It allows fetches that should never have been issued (probe-via-cache, log noise).
- It does not protect the browser-side `<track src>` consumer from `javascript:`
  or `data:` URLs that bypass the proxy entirely.
- An attacker who controls a vibeplayer.site response (or one of its strict
  subdomains — domain-allowlisted but not trust-allowlisted) can inject ANY
  subtitle URL, including ones outside the HLSProxyAllowedDomains list, which
  the proxy would 403 — but that 403 turns into client-visible breakage, not
  defence in depth.

The `vibePlayerSrcRegex` requires `https?://[^"]+\.m3u8[^"]*` and is well
defended; the parallel subtitle regex must apply the same shape check.
**Fix:**
```go
// At top:
var vibePlayerSubURLRegex = regexp.MustCompile(`^https?://[^"\s]+\.(vtt|srt|ass)(\?[^"\s]*)?$`)

// Inside Extract, replace the current subM block:
if subM := vibePlayerSubRegex.FindSubmatch(body); subM != nil && len(subM[1]) > 0 {
    subURL := string(subM[1])
    if !vibePlayerSubURLRegex.MatchString(subURL) {
        // Drop bad subtitle but keep the stream. Optionally emit a
        // parser_zero_match_total counter with selector="vibeplayer_sub_url_shape".
    } else {
        // Optional host allowlist: require cdn.cimovix.store or vibeplayer.site
        if pu, err := url.Parse(subURL); err == nil {
            host := strings.ToLower(pu.Hostname())
            if host == "cdn.cimovix.store" || host == "vibeplayer.site" ||
                strings.HasSuffix(host, ".vibeplayer.site") {
                stream.Tracks = []domain.Track{{
                    File:    subURL,
                    Label:   "English",
                    Kind:    "captions",
                    Default: true,
                }}
            }
        }
    }
}
```

### CR-02: Saved provider preference (`gogoanime`) is dropped on cold load

**File:** `frontend/web/src/components/player/EnglishPlayer.vue:601-620,1716-1747`
**Issue:** `availableProviders` is initialized to `['animepahe']` (line 601),
and the "restore prior preference" block runs SYNCHRONOUSLY in `<script setup>`
(lines 615-620) before `onMounted` has had a chance to populate the real
provider list from `scraperApi.getHealth()`. The check
`availableProviders.value.includes(preferredScraperProvider.value)` always
fails for `gogoanime` at this point, so `selectedProvider.value` stays `null`
and the player walks the orchestrator-default chain instead of opening on the
user's chosen Anitaku.

When `onMounted` finishes and replaces `availableProviders.value` with the
real list including `gogoanime`, the lines 1740-1746 ONLY clear stale prefs;
they do not retry the "restore valid pref" path.

Net effect: every user who clicked "Anitaku" yesterday opens the player on
"AnimePahe" today. The 24h preference TTL is effectively wallpaper.
**Fix:** Move the preference-restoration into `onMounted` AFTER the health
fetch completes, OR keep the synchronous restore but ALSO re-evaluate after
the health response arrives:
```ts
// In onMounted, after `availableProviders.value = providers`:
if (
  preferredScraperProvider.value &&
  providers.includes(preferredScraperProvider.value) &&
  selectedProvider.value === null
) {
  selectedProvider.value = preferredScraperProvider.value
}
```

### CR-03: `computeStreamTTL` caches malformed-`e=` URLs for the full 5min cap

**File:** `services/scraper/internal/providers/gogoanime/cache.go:42-78`
**Issue:** Parse-error paths return `streamTTLFallback` (= `streamTTLCap` =
5 minutes):
```go
eSec, err := strconv.ParseInt(eStr, 10, 64)
if err != nil || eSec <= 0 {
    return streamTTLFallback  // CACHE FOR 5 MINUTES
}
```
The intent is "if there's no `e=` param, assume the URL is static (vibeplayer-
style)". But the function conflates THREE distinct cases into one TTL:

1. No `e=` at all → URL is static → 5min cache is fine.
2. `e=notanumber` → URL HAS a signed expiry but parse failed → the URL likely
   expires very soon (CDN URLs with malformed signatures get 403'd quickly).
3. `e=0` or `e=-1` → URL claims zero-or-negative TTL → already expired.

Cases (2) and (3) get a 5min positive cache. The TestComputeStreamTTL_StreamHGSignedURL
test at line 70 explicitly locks in this behaviour (`non_integer_e_returns_fallback`
expects `streamTTLFallback`), so it survived TDD review. The result: when StreamHG
or Earnvids rotates its signing scheme (high probability — these CDNs rotate
quarterly per RESEARCH.md), every cached URL becomes "valid for 5 minutes" and
every replay produces a 403 → user-facing breakage that persists for 5min per anime.

**Fix:** Distinguish "no e= param" from "malformed e= param":
```go
eStr := q.Get("e")
if eStr == "" {
    return streamTTLFallback  // truly static URL — fallback OK
}
eSec, err := strconv.ParseInt(eStr, 10, 64)
if err != nil || eSec <= 0 {
    // URL claims signed expiry but the value is unparseable / expired.
    // Don't cache — return 0 so the orchestrator re-extracts on the next request.
    return 0
}
// ... rest of function unchanged.
```
Then update `TestComputeStreamTTL_StreamHGSignedURL.non_integer_e_returns_fallback`
to expect `0` rather than `streamTTLFallback`.

## Warnings

### WR-01: `packed_common.go` reuses `defaultPackedHTTPTimeout` (15s) as the goja runtime budget

**File:** `services/scraper/internal/embeds/packed_common.go:51,234`; `services/scraper/internal/embeds/streamhg.go:51-52`; `services/scraper/internal/embeds/earnvids.go:47-48`
**Issue:** Both StreamHG and Earnvids constructors pass `defaultPackedHTTPTimeout`
into BOTH the `http.Client.Timeout` AND the `packedExtractor.timeout` field
(which `runGoja` consumes as the JS execution budget). The Kwik extractor
deliberately separates these: `defaultKwikHTTPTimeout = 15s`, `defaultKwikTimeout
= 5s` (see kwik.go:44-51). The packed_common comment at the constructors says
"both 15s — matches the Kwik extractor's defaults" — that statement is FALSE,
and the wider-than-intended goja budget means a hostile packed JS payload (e.g.
infinite loop disguised as legitimate packer output) pins a goroutine for 15s
of CPU instead of 5s. At 1 RPS per host across multiple users, this becomes
a goroutine-pressure DoS vector.

**Fix:** Add a distinct goja-budget constant and use it for the `timeout`
field:
```go
// packed_common.go
const defaultPackedHTTPTimeout = 15 * time.Second
const defaultPackedGojaTimeout = 5 * time.Second  // NEW — matches Kwik.

// streamhg.go (and earnvids.go):
base := &packedExtractor{
    // ...
    http:    &http.Client{Timeout: defaultPackedHTTPTimeout},
    timeout: defaultPackedGojaTimeout,  // was defaultPackedHTTPTimeout
}
```

### WR-02: `packed_common.go` emits the wrong selector on `runGoja` failure

**File:** `services/scraper/internal/embeds/packed_common.go:197-201`
**Issue:**
```go
unpacked, err := runGoja(ctx, wrapper, e.timeout)
if err != nil {
    metrics.ParserZeroMatchTotal.WithLabelValues(e.name, e.selectorPackerFail).Inc()
    return nil, domain.WrapExtractFailed(err, e.name+": goja unpack")
}
```
Both `extractPacker(...)` failure AND `runGoja(...)` failure increment the
same `selectorPackerFail` ("streamhg_packer_balance" / "earnvids_packer_balance").
The Phase 17 dashboard relies on selector strings to localize regressions; a
goja runtime trip and a balance-paren miss are different failure classes
(former = upstream changed JS shape, latter = upstream changed HTML structure)
and conflating them masks one inside the other.

**Fix:** Add a fourth selector field to `packedExtractor` (e.g. `selectorGojaFail`):
```go
type packedExtractor struct {
    // ...
    selectorPackerFail string
    selectorGojaFail   string  // NEW
    selectorRegexFail  string
    selectorBodyFail   string
    // ...
}

// In Extract:
unpacked, err := runGoja(ctx, wrapper, e.timeout)
if err != nil {
    metrics.ParserZeroMatchTotal.WithLabelValues(e.name, e.selectorGojaFail).Inc()
    return nil, domain.WrapExtractFailed(err, e.name+": goja unpack")
}
```
And populate in `NewStreamHGExtractor` / `NewEarnvidsExtractor` with
`"streamhg_goja"` / `"earnvids_goja"`.

### WR-03: `runGoja` silently drops JS exceptions inside the watchdog goroutine

**File:** `services/scraper/internal/embeds/packed_common.go:234-264`
**Issue:** When `vm.RunString(expr)` returns a non-nil error AND the watchdog
has already fired `vm.Interrupt(...)`, the returned error from RunString is
the runtime error from the script — not the Interrupt reason. The watchdog
DOES correctly cancel the runtime, but the caller cannot distinguish
"upstream JS threw" from "we timed out" from "ctx was cancelled":
```go
val, err := vm.RunString(expr)
if err != nil {
    return "", err  // could be timeout-induced interrupt OR a real JS bug
}
```
This shows up in Phase 17 metrics: a real upstream regression (JS shape
change) is indistinguishable from a 5s wall-clock spike.

**Fix:** Add a channel to surface the watchdog reason:
```go
func runGoja(ctx context.Context, expr string, timeout time.Duration) (string, error) {
    vm := goja.New()
    done := make(chan struct{})
    defer close(done)
    var interruptReason atomic.Value // string

    go func() {
        timer := time.NewTimer(timeout)
        defer timer.Stop()
        select {
        case <-timer.C:
            interruptReason.Store("timeout")
            vm.Interrupt("packed: unpack timeout")
        case <-ctx.Done():
            interruptReason.Store("ctx")
            vm.Interrupt("packed: ctx cancel")
        case <-done:
        }
    }()

    val, err := vm.RunString(expr)
    if err != nil {
        if r, _ := interruptReason.Load().(string); r != "" {
            return "", fmt.Errorf("packed goja: %s: %w", r, err)
        }
        return "", err
    }
    // ...
}
```

### WR-04: `gogoanime.Provider.markStage(StageStream, err)` records extractor-internal cause errors

**File:** `services/scraper/internal/providers/gogoanime/client.go:660-668`
**Issue:** When `ext.Extract(...)` returns a wrapped `ErrExtractFailed` (e.g.
the upstream HTML changed and the regex no longer matches), the gogoanime
provider records that error VERBATIM into `p.stages[StageStream].LastErr`:
```go
stream, err := ext.Extract(ctx, serverID, headers)
if err != nil {
    p.markStage(health.StageStream, err)  // err.Error() includes raw upstream details
    return nil, err
}
```
The `LastErr` field is surfaced to admins via `/api/admin/scraper/health` (per
changelog entry "Admin-only: new endpoint..."). The redaction T-17-03-02 in
the admin handler clamps to 200 chars, but the underlying err is a multi-`%w`
wrap that includes the URL path (which can leak query params with signed
tokens like `?s=...&e=...&token=...` for StreamHG/Earnvids signed CDN URLs).

**Fix:** Map extractor errors to a generic class string before storing:
```go
if err != nil {
    // Strip the cause chain; keep only the high-level category for admin display.
    var category string
    switch {
    case errors.Is(err, domain.ErrExtractFailed):
        category = "extract_failed"
    case errors.Is(err, domain.ErrProviderDown):
        category = "provider_down"
    case errors.Is(err, domain.ErrNotFound):
        category = "not_found"
    default:
        category = "unknown"
    }
    p.markStage(health.StageStream, errors.New("gogoanime: stream "+category))
    return nil, err
}
```

### WR-05: `fetchEpisodes` insertion sort is O(n²) and bypasses Go's `sort` package

**File:** `services/scraper/internal/providers/gogoanime/client.go:418-425`
**Issue:**
```go
// Simple insertion sort — len is small in practice.
for i := 1; i < len(nums); i++ {
    for j := i; j > 0 && nums[j] < nums[j-1]; j-- {
        nums[j], nums[j-1] = nums[j-1], nums[j]
    }
}
```
The "len is small in practice" comment is true today (~1200 One Piece eps),
but a future bug that produces non-contiguous episode numbers (e.g. anitaku
restructures category pages to a paginated layout) would make this O(n²) over
the full episode set on every cold cache hit. `sort.Ints(nums)` is one line
and the right answer.

**Fix:**
```go
import "sort"
// ...
sort.Ints(nums)
```

### WR-06: `gogoanime.MalSyncClient.Lookup` silently ignores unexpected cache backend errors

**File:** `services/scraper/internal/providers/gogoanime/malsync.go:140-147`
**Issue:**
```go
if err := m.cache.Get(ctx, hitKey, &cached); err == nil && cached != "" {
    return cached, true, nil
} else if err != nil && !errors.Is(err, cache.ErrNotFound) {
    _ = err  // <-- dead assignment
}
```
The `_ = err` is a literal no-op. The comment says "treat as a miss and fall
through", which is the right behaviour — but the actual code does nothing
with the error and (worse) the dead assignment looks like intentional
suppression in a future grep. Either log it (preferred, since unexpected
Redis errors deserve at minimum a warn-level breadcrumb) or remove the
`else-if` entirely.

**Fix:**
```go
} else if err != nil && !errors.Is(err, cache.ErrNotFound) {
    // Unexpected cache backend failure — log and fall through to upstream.
    // Don't propagate redis blips into the lookup path.
    // (Add a logger reference to MalSyncClient if not already present.)
    // m.log.Warnw("malsync cache get failed", "key", hitKey, "error", err)
}
```

### WR-07: `EnglishPlayer.vue` defines `sourceSwitchFailed` i18n key but never uses it

**File:** `frontend/web/src/locales/en.json:156`, `ja.json:156`, `ru.json:156`; `frontend/web/src/components/player/EnglishPlayer.vue:979-993`
**Issue:** The locales ship `"sourceSwitchFailed": "Couldn't switch source — staying on {provider}"`,
which appears to be intended for the `switchProvider` rollback path. The
actual implementation falls back to `console.warn(...)` (line 986-991) with
no user-visible feedback at all. The user gets a silent rollback if the
switch fails — which combined with CR-02 (saved-pref dropped) and the
stale `error.value` set by `fetchServers` produces a confusing UX (the user
sees the error overlay but the source label says they're still on the new
source).

**Fix:** Either wire the i18n key into the error overlay path:
```ts
} catch (err) {
    selectedProvider.value = prior
    setPreferredScraperProvider(prior)
    error.value = t('player.sourceSwitchFailed', {
        provider: capitalizeProvider(prior || availableProviders.value[0] || 'animepahe'),
    })
    console.warn('[EnglishPlayer] source switch failed', err)
    return
}
```
or remove the unused key from the three locale files.

### WR-08: `EnglishPlayer.vue:setPreferredScraperProvider(prior)` may pass null

**File:** `frontend/web/src/components/player/EnglishPlayer.vue:981`
**Issue:** When `switchProvider` rolls back, `prior` may be `null` (if the
user hadn't yet committed to any provider). The composable
`setPreferredScraperProvider(null)` is referenced as the clearing call
elsewhere in the file (line 1744) so it accepts null — but if the composable
signature is `(slug: string)` (verified by grep showing the call site at
1744 passing null), this is a TypeScript looseness bug that could surface
as `localStorage.setItem('...', 'null')` and persist that as a literal
'null' string preference on the next reload.

**Fix:** Coerce to a stable sentinel before calling:
```ts
if (prior !== null) {
    setPreferredScraperProvider(prior)
}
```
Or audit the composable to ensure it strips `null` before writing.

### WR-09: `gogoanime.fetchEpisodes` `_ = cat // category derived from slug suffix below` is dead code

**File:** `services/scraper/internal/providers/gogoanime/client.go:514`
**Issue:** The function determines `cat` from the slug suffix at line 482-486,
then assigns it to a local variable, then explicitly discards it with `_ = cat`
at line 514, then NEVER USES it. The `domain.Episode` rows emitted at line
515-519 do not carry the category — only `ID`, `Number`, `Title` are set.
The downstream `ListServers` re-derives the category from the slug at line
578-581. So `cat` is computed twice (once here, once at ListServers) and
discarded the first time. Either the Episode struct should carry the
category and this code should set it, or the assignment should be removed.

**Fix:** Remove the dead code:
```go
// Determine sub/dub from the slug suffix.
isDub := strings.HasSuffix(slug, "-dub")
// (delete the cat variable and the _ = cat line.)
```
Or add the category to `domain.Episode` and propagate it through.

## Info

### IN-01: `domain.NewBaseHTTPClient` ignores `cookiejar.New` error

**File:** `services/scraper/internal/domain/httpclient.go:104`
**Issue:** `jar, _ := cookiejar.New(...)` discards the error. In practice
`cookiejar.New` only errors if `Options.PublicSuffixList` is nil and the
default jar implementation rejects that, but the linter and future maintainers
benefit from an explicit panic-or-log on this once-per-service-lifetime path.

**Fix:** `jar, _ := cookiejar.New(...)` → `jar, err := cookiejar.New(...)` and
`if err != nil { log.Fatalw("cookiejar.New", "error", err) }` (or use the
existing logger). Optional given the impossibility-in-practice.

### IN-02: `scraper/cmd/scraper-api/main.go:122` shadows package name `cache`

**File:** `services/scraper/cmd/scraper-api/main.go:122`
**Issue:** Line 122 declares a local `cache` variable (`cache := health.NewInMemoryHealthCache()`)
that shadows the imported `cache` package alias used earlier (`redisCache, err := cache.New(...)`).
The name collision is harmless because `cache.New` was already called above,
but `goimports`/`govet -shadow` will flag this and a future maintainer adding
another `cache.X(...)` call below line 122 will compile-fail with a confusing
"undefined: cache.X" message.

**Fix:** Rename the local: `healthCache := health.NewInMemoryHealthCache()`
and propagate through `orchestrator.Register`, `scraperHandler := handler.NewScraperHandler(...)`,
and the `for _, p := range providers` loop.

### IN-03: `gogoanime.GetStream` cache key hashes serverID to 8 bytes (16 hex chars)

**File:** `services/scraper/internal/providers/gogoanime/client.go:641-642`
**Issue:** The cache key uses `hex.EncodeToString(h[:8])` — that's 16 hex
chars of a SHA-256 prefix. 16 hex chars = 64 bits = 2^64 keyspace. For a
single-provider, single-episode, single-anime namespace the collision
probability is astronomically low — but the comment "for bounded key length"
suggests cargo-culting rather than a sized decision. Either document the
collision-acceptance rationale or use the full 32-byte hash (or just store
the URL-encoded serverID, which is already bounded by the upstream HTML).

**Fix:** Add a one-line comment explaining the 8-byte choice (≈64 bits of
entropy, more than enough for a per-anime keyspace).

### IN-04: `helpers_test.go` defines its own `min(a, b int)` helper

**File:** `services/scraper/internal/providers/gogoanime/helpers_test.go:169-174`
**Issue:** Go 1.21+ has a built-in `min` function. The package-local
helper is dead code on any toolchain newer than 1.21, and (if the project
ever upgrades to a build mode that exposes built-ins eagerly) will produce a
"redeclared" compile error.

**Fix:** Delete the helper; the built-in handles `min(5, len(rows))`.

### IN-05: `gogoanime/dto.go` defines `episodeRow` and `serverRow` types but only `searchResult` is used outside tests

**File:** `services/scraper/internal/providers/gogoanime/dto.go:23-36`
**Issue:** `episodeRow` and `serverRow` are defined in dto.go but only ever
referenced from `helpers_test.go`. The production `client.go` builds
`domain.Episode` and `domain.Server` directly from goquery selections without
going through the DTO. The DTOs are vestigial scaffolding from the
test-first phase.

**Fix:** Either move `episodeRow`/`serverRow` into the `_test.go` file, or
refactor `fetchEpisodes`/`ListServers` to go through the DTO for consistency.
The current state confuses future readers ("which type do I use? both? why?").

### IN-06: `EnglishPlayer.vue:1066-1073` has duplicated envelope-shape narrowing

**File:** `frontend/web/src/components/player/EnglishPlayer.vue:1064-1073`
**Issue:** The block:
```ts
const env = response.data as { data?: { stream?: ScraperStream } | ScraperStream } | undefined
let data: ScraperStream
if (env && (env as { data?: { stream?: ScraperStream } }).data && (env as { data: { stream?: ScraperStream } }).data.stream) {
    data = (env as { data: { stream: ScraperStream } }).data.stream
} else if (env && (env as { data?: ScraperStream }).data && (env as { data: ScraperStream }).data.url) {
    // ...
}
```
performs the same cast three times per branch. Even by Vue+TS standards
this is hard to read. A typed `EnvelopeOrPassthrough` discriminated union
would compress this to ~5 lines and survive a future scraper handler change
without re-checking three call sites.

**Fix:** Define a discriminated union for the envelope and narrow once.

---

_Reviewed: 2026-05-12T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
