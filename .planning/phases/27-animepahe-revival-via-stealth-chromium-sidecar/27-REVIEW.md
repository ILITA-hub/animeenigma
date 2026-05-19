---
phase: 27-animepahe-revival-via-stealth-chromium-sidecar
reviewed: 2026-05-19T12:00:00Z
depth: standard
files_reviewed: 22
files_reviewed_list:
  - services/animepahe-resolver/server.js
  - services/animepahe-resolver/browser.js
  - services/animepahe-resolver/upstream.js
  - services/animepahe-resolver/metrics.js
  - services/animepahe-resolver/package.json
  - services/animepahe-resolver/Dockerfile
  - services/animepahe-resolver/server.test.js
  - services/animepahe-resolver/upstream.test.js
  - services/scraper/internal/providers/animepahe/client.go
  - services/scraper/internal/providers/animepahe/resolver.go
  - services/scraper/internal/providers/animepahe/dto.go
  - services/scraper/internal/providers/animepahe/cache.go
  - services/scraper/internal/providers/animepahe/malsync.go
  - services/scraper/internal/config/config.go
  - services/scraper/cmd/scraper-api/main.go
  - services/scraper/internal/embeds/kwik.go
  - services/scraper/internal/providers/animepahe/client_test.go
  - services/scraper/internal/providers/animepahe/resolver_test.go
  - services/scraper/internal/providers/animepahe/dto_test.go
  - services/scraper/internal/providers/animepahe/malsync_invalidation_test.go
  - services/scraper/internal/config/config_test.go
  - docker/docker-compose.yml
findings:
  critical: 2
  warning: 6
  info: 3
  total: 11
status: issues_found
---

# Phase 27: Code Review Report

**Reviewed:** 2026-05-19T12:00:00Z
**Depth:** standard
**Files Reviewed:** 22
**Status:** issues_found

## Summary

Phase 27 introduces a Node.js stealth-Chromium sidecar (`animepahe-resolver`) and rewires the Go AnimePahe scraper provider to talk through it instead of hitting upstream directly. The architecture decisions (D1–D7) are sound, the threat-model mitigations are mostly implemented, and the dual-packer Kwik fix (Plan 27-04 Rule 1) is correct. The MalSync session-shape validation and single-strike invalidation path are well-constructed.

Two blockers require fixes before the phase ships cleanly:

1. The sidecar **silently passes through upstream non-200/non-403 status codes** (e.g. animepahe 404s for unknown sessions) as HTTP 200 to the Go parser. The Go parser's `mapStatus` expects the sidecar to return 404 for "session not found" — but it never will, because `handleJSON` and the `/play` handler ignore `result.status`. The Go parser consequently sees HTTP 200 with a JSON body like `{"message":""}`, tries to decode it as a `releaseResponse`, gets an empty `data` array, and treats the missing anime as a real-empty result rather than triggering the MalSync invalidation path.

2. `kwik.go` line 343 still uses `https://animepahe.ru` as the default Referer when fetching the Kwik embed page. `animepahe.ru` is TCP-blackholed from this server (Context D2). For the animepahe provider this is papered over by GetStream always injecting `kwikReferer` explicitly, but the fallback remains a correctness hazard for any future caller that omits Referer.

---

## Critical Issues

### CR-01: Sidecar passes upstream 404/5xx bodies as HTTP 200 — Go parser never sees the error status

**File:** `services/animepahe-resolver/server.js:188-201` (handleJSON), `services/animepahe-resolver/server.js:158-163` (/play handler)

**Issue:** `fetchWithRetry` returns `{ status, body }` from the in-browser `fetch()`. The route handlers destructure only `body` and discard `status`. When the upstream returns HTTP 404 (e.g. an expired `animeSession` hits `/api?m=release`), the sidecar replies to the Go parser with HTTP 200 carrying the upstream's 404 JSON body (typically `{"message":""}`). The Go resolver's `mapStatus()` function in `resolver.go:137-157` only fires on non-200 responses — a 200 with an empty-data body is decoded as a valid `releaseResponse` with `total=0, data=[]`, making a genuinely missing anime look identical to a real-empty one (no episodes aired yet). The MalSync A9 single-strike invalidation path in `client.go:351-354` is gated on `domain.ErrNotFound`, which is only produced by `mapStatus` on HTTP 404 — so this path never fires for upstream 404s in practice.

The `/play` handler has the same defect: a 404 from the upstream play page (expired or invalid episodeSession) returns HTTP 200 with the 404 HTML, which `ListServers` will parse with goquery, find zero `button[data-src]` elements, and return an empty `[]Server{}` rather than `ErrNotFound`.

**Fix:** Map non-200/non-403 upstream statuses to appropriate sidecar error responses before returning. In `handleJSON`:

```javascript
async function handleJSON(req, reply, url) {
  try {
    const { status, body } = await upstream.fetchWithRetry(url);
    if (status === 404) {
      return reply.code(404).send({ error: 'not_found', message: 'upstream returned 404' });
    }
    if (status < 200 || status >= 300) {
      return reply.code(502).send({ error: 'upstream_error', message: `upstream status ${status}` });
    }
    let parsed;
    try {
      parsed = JSON.parse(body);
    } catch (e) {
      req.log.error({ url, err: e }, 'upstream returned non-JSON body');
      return reply.code(502).send({ error: 'upstream_bad_json' });
    }
    return reply.send(parsed);
  } catch (e) {
    return sendResolverError(reply, e);
  }
}
```

Apply the same status check in the `/play` handler before `reply.type('text/html').send(body)`.

Add a test case in `server.test.js` that stubs `fetchWithRetry` to return `{ status: 404, body: '{"message":""}' }` and asserts the sidecar replies with HTTP 404.

---

### CR-02: kwik.go default Referer is the dead/blocked domain `animepahe.ru`

**File:** `services/scraper/internal/embeds/kwik.go:343`

**Issue:** The `Extract` method sets a default Referer when `headers` does not contain one:

```go
if req.Header.Get("Referer") == "" {
    req.Header.Set("Referer", "https://animepahe.ru")
}
```

`animepahe.ru` is TCP-blackholed from this server's egress IP (Context D2, confirmed 2026-05-19 probe). Any future caller that invokes the Kwik extractor without explicitly setting `Referer` will use a domain that cannot be resolved and that Kwik's upstream may reject as an invalid parent-site Referer (Kwik validates the Referer chain). The animepahe provider's `GetStream` currently does set `kwikReferer` (`https://animepahe.pw/`) explicitly so the bug is masked today — but adding a second provider that uses the Kwik extractor without setting Referer will silently use the blackholed domain.

**Fix:**

```go
if req.Header.Get("Referer") == "" {
    req.Header.Set("Referer", "https://animepahe.pw/")
}
```

This aligns the default with D2 (`.pw` is the exclusive live domain), matches the `kwikReferer` constant in `resolver.go:37`, and is consistent with the post-Phase-27 operating posture.

---

## Warnings

### WR-01: `initBrowser()` leaves partially-initialized browser on failure — no cleanup

**File:** `services/animepahe-resolver/browser.js:62-88`

**Issue:** The initialization IIFE can fail at three points: `puppeteer.launch()`, `browser.newPage()`, or `page.goto()`. If it fails after `browser` is set but before `page` is set (e.g. `newPage()` throws), the `finally { initPromise = null }` block runs, `initialized` remains `false`, and the leaked `browser` instance is never closed. A subsequent call to `initBrowser()` will try to launch another browser process, and the leaked one continues consuming RAM.

More critically: if `page.goto()` fails (network timeout on first warmup), `browser` and `page` are both set but `initialized` is still `false` (the assignment is the last line of the IIFE). The next caller re-runs the IIFE, tries `puppeteer.launch()` again, overwrites `browser`, and the original browser + page are leaked.

**Fix:** Add cleanup on failure:

```javascript
initPromise = (async () => {
  let newBrowser;
  try {
    newBrowser = await puppeteer.launch({ headless: 'new', executablePath: ..., args: LAUNCH_ARGS.slice() });
    const newPage = await newBrowser.newPage();
    await newPage.setUserAgent(USER_AGENT);
    await newPage.goto(UPSTREAM_BASE_URL + '/', { waitUntil: 'networkidle2', timeout: WARMUP_GOTO_TIMEOUT_MS });
    browser = newBrowser;
    page = newPage;
    initialized = true;
  } catch (e) {
    if (newBrowser) {
      try { await newBrowser.close(); } catch (_) {}
    }
    throw e;
  }
})();
```

---

### WR-02: Non-200/non-403 upstream statuses increment `requestCount` and may trigger page recycle

**File:** `services/animepahe-resolver/upstream.js:142`

**Issue:** `requestCount += 1` runs unconditionally after `fetchOnce` returns regardless of `result.status`. If the upstream returns a 429 (rate limit) or 503, `requestCount` still increments, potentially triggering `maybeRecycle()` and destroying the warm page under conditions where retaining it would be beneficial. More importantly, this is symptom of the CR-01 bug — once CR-01 is fixed and non-200 responses produce early returns, this increments on 404s before the status check fires. Recycle should only occur after a definitively successful fetch.

**Fix:** Move `requestCount += 1` and the `maybeRecycle()` call inside the success branch — after the status is confirmed non-error. If CR-01 is addressed by returning early on non-200 statuses before this point, this is already handled. Otherwise, explicitly guard:

```javascript
// Only count toward recycle budget on successful (non-error) responses.
if (result.status >= 200 && result.status < 300) {
  requestCount += 1;
  try {
    await maybeRecycle();
  } catch (e) {
    console.error('animepahe-resolver: maybeRecycle failed', e);
  }
}
return result;
```

---

### WR-03: `hostnameOf` function is defined but never called (dead code)

**File:** `services/scraper/internal/providers/animepahe/client.go:451-457`

**Issue:** The helper function `hostnameOf` is declared at line 451 but has no callers anywhere in the package. The `ListServers` method builds URLs differently (via `url.Parse` inline). Dead unexported functions do not produce a compile error in Go but are misleading to maintainers and are flagged by `staticcheck`/`unused`.

**Fix:** Delete the function. If it is intended as a future utility, it belongs in the `fuzzy` or a `urlutil` shared package where it will be imported.

```go
// DELETE lines 451-457:
// func hostnameOf(s string) string {
//     u, err := url.Parse(s)
//     if err != nil {
//         return ""
//     }
//     return u.Hostname()
// }
```

---

### WR-04: `UPSTREAM_BASE_URL` env var in docker-compose is silently ignored by the code

**File:** `docker/docker-compose.yml:131`

**Issue:** The compose block sets `UPSTREAM_BASE_URL: https://animepahe.pw` with the comment "informational/auditable." However, neither `server.js` nor `browser.js` reads `process.env.UPSTREAM_BASE_URL` — the constant is hardcoded as a `const` in both files. An operator who sees this environment variable and tries to change the upstream domain (e.g. to route through a different mirror when `.pw` goes dark) will find their change has no effect. The discoverability of this pattern is dangerous: the env var looks like it controls the upstream but it does not.

The intent (per T-27-01-01) is to keep the URL hardcoded precisely so it cannot be changed via env injection. That is correct security design, but the env var should either be removed from compose entirely (to avoid the false affordance) or the comment should be strengthened to explicitly state it is a documentation artifact that the code does NOT read.

**Fix:** Remove the `UPSTREAM_BASE_URL` entry from the compose `environment` block, or at minimum rename it to `_DOC_UPSTREAM_BASE_URL` with a comment that makes the non-functional nature unmistakable:

```yaml
environment:
  NODE_ENV: production
  LOG_LEVEL: info
  # NOTE: the sidecar HARDCODES https://animepahe.pw in server.js + browser.js
  # (T-27-01-01 — not overridable via env). The line below is audit documentation
  # only; changing it does NOT change sidecar behavior.
  _HARDCODED_UPSTREAM: "https://animepahe.pw (see server.js:37, browser.js:31)"
```

---

### WR-05: `parseServerPriority` doc comment is merged with `parseDegradedProviders` comment, truncating the `parseServerPriority` documentation

**File:** `services/scraper/internal/config/config.go:218-243`

**Issue:** The doc comment for `parseServerPriority` (lines 218-228) runs directly into the start of `parseDegradedProviders` without the closing `//` terminator or blank line that Go convention requires between doc comment and function. The `parseDegradedProviders` doc comment starts at line 227 (inside what appears to be the `parseServerPriority` comment block), and `parseDegradedProviders` itself immediately follows at line 233 — leaving `parseServerPriority` undocumented at its own declaration site (it appears at line 245 with no preceding comment). `go doc` will attach the comment block to `parseDegradedProviders`, not `parseServerPriority`.

**Fix:**

```go
// parseServerPriority splits a CSV priority spec into a normalized slice.
// Whitespace is trimmed, case is lowered, and empty entries are dropped.
// Empty input returns the canonical default ["streamhg","earnvids","vibeplayer"].
//
// Phase 21 SCRAPER-HEAL-03. Validation against the embeds registry happens in main.go.
func parseServerPriority(csv string) []string {
    ...
}

// parseDegradedProviders splits a CSV list of provider names into a set.
// Whitespace is trimmed, names are lowercased, empties dropped.
func parseDegradedProviders(csv string) DegradedProvidersConfig {
    ...
}
```

---

### WR-06: `_setTestDoubles` test hook is exported from `browser.js` with no production guard

**File:** `services/animepahe-resolver/browser.js:148-158`

**Issue:** `_setTestDoubles` replaces the global `browser` and `page` singletons. It is exported via `module.exports` with no production guard (`NODE_ENV !== 'test'` check or similar). Any module that `require('./browser')` in production can call this and reset the warm browser/page to `null`, causing all subsequent requests to fail with `browser_down` (503). While this is an internal service and not exposed to external callers, the lack of a guard is a robustness concern.

**Fix:** Add a no-op guard or document the risk:

```javascript
function _setTestDoubles(doubles) {
  if (process.env.NODE_ENV === 'production') {
    throw new Error('_setTestDoubles must not be called in production');
  }
  // ... existing implementation
}
```

Alternatively, use a conditional export pattern that only exports `_setTestDoubles` when `NODE_ENV !== 'production'`.

---

## Info

### IN-01: `initBrowser()` does not handle concurrent callers that arrive while the IIFE is in-flight correctly — they get the promise returned but do not register in the `finally` reset

**File:** `services/animepahe-resolver/browser.js:64`

**Issue:** When a second caller arrives while the first is still in `initPromise`, line 64 returns `initPromise` directly. But after the `finally { initPromise = null }` fires, any callers that are currently `await`-ing the returned promise handle a resolved/rejected value — they do not execute the `finally` block themselves. This is actually fine for the happy path, but if the IIFE rejects, the second caller gets the rejection re-thrown (which is correct JavaScript Promise behavior). However, the `initBrowser` function structure is non-obvious: the `try/finally` block on lines 83-87 is not inside the IIFE but in the outer `initBrowser` function, so only the FIRST caller executes that cleanup — concurrent callers miss it. This creates a subtle ordering assumption. It should be documented with a comment or refactored to a class-level pattern.

---

### IN-02: `server.test.js` stubUpstream monkey-patches `upstream.fetchWithRetry` but does not restore on test failure within `withApp`

**File:** `services/animepahe-resolver/server.test.js:217-222`

**Issue:** The `stubUpstream` helper returns a `restore` function that callers must invoke in a `finally` block. If a test assertion throws before `restore()` is called (e.g. inside a `try` block without `finally`), the stub leaks across tests. The existing tests wrap in `try/finally` correctly, but the pattern is fragile for future test authors who may copy the pattern without the `finally`.

The `withApp` wrapper correctly uses `finally` for `app.close()` and `teardown()`, but the stub/restore pattern is separate and manually managed. A `t.after()` or `assert.afterEach()` hook in Node.js's `node:test` module would make this automatic.

---

### IN-03: `releaseSchema` uses `additionalProperties: false` but `session` is not required to be UUID-shaped at the Fastify layer

**File:** `services/animepahe-resolver/server.js:64-74`

**Issue:** `SESSION_PATTERN = '^[A-Za-z0-9-]{16,128}$'` covers both UUID v4 sessions (36 chars, `[A-Za-z0-9-]`) and hex episode sessions (64 lowercase hex chars). This is intentional and documented in the file header. However, the pattern also accepts strings like `abcdefghij-klmn` (16 chars, valid) which are neither UUID-shaped nor hex — they pass schema validation but would fail upstream's real session lookup. This is low-risk (the upstream returns 404 which the sidecar then needs to map correctly — see CR-01) but the pattern is wider than necessary.

This is informational: tightening the pattern to `^[0-9a-f]{64}$|^[A-Za-z0-9]{8}-[A-Za-z0-9]{4}-[A-Za-z0-9]{4}-[A-Za-z0-9]{4}-[A-Za-z0-9]{12}$` for episodeSession and animeSession respectively would catch bad inputs earlier, though it risks breaking if upstream changes session formats. Document the intentional permissiveness if keeping the current pattern.

---

## Top Recommendations

1. **CR-01 (BLOCKER)** — Fix `handleJSON` and `/play` to map upstream non-200/non-403 status codes to appropriate HTTP error responses. Without this, the MalSync A9 invalidation path is dead code in practice, and missing animes silently appear as "no episodes yet" instead of triggering re-search.

2. **CR-02 (BLOCKER)** — Change the default Kwik Referer from `https://animepahe.ru` to `https://animepahe.pw/`. A dead domain as fallback is a quiet correctness hazard for any future Kwik extractor caller that does not set Referer.

3. **WR-01** — Add cleanup on `initBrowser` failure to prevent browser process leaks.

4. **WR-03** — Delete `hostnameOf`; it passes no compiler check and will confuse future readers.

---

_Reviewed: 2026-05-19T12:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
