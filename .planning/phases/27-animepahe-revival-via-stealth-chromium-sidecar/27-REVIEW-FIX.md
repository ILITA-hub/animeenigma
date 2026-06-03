---
phase: 27-animepahe-revival-via-stealth-chromium-sidecar
fixed_at: 2026-05-19T13:00:00Z
review_path: .planning/phases/27-animepahe-revival-via-stealth-chromium-sidecar/27-REVIEW.md
iteration: 1
findings_in_scope: 8
fixed: 8
skipped: 0
status: all_fixed
---

# Phase 27: Code Review Fix Report

**Fixed at:** 2026-05-19T13:00:00Z
**Source review:** `.planning/phases/27-animepahe-revival-via-stealth-chromium-sidecar/27-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 8 (CR-01, CR-02, WR-01..WR-06)
- Fixed: 8
- Skipped: 0

---

## Fixed Issues

### CR-01: Sidecar passes upstream 404/5xx bodies as HTTP 200

**Files modified:** `services/animepahe-resolver/server.js`, `services/animepahe-resolver/server.test.js`
**Commit:** b8bb554
**Applied fix:** Both `handleJSON` and the `/play` handler now destructure `{ status, body }` from `fetchWithRetry`. Before sending any response, they check the status: 404 → reply 404 `{error:"not_found"}`; any other non-2xx → reply 502 `{error:"upstream_error"}`. This restores the MalSync single-strike invalidation path (which is gated on `mapStatus` receiving HTTP 404 from the sidecar). Three new tests were added to `server.test.js` covering: release+404, release+503, and play+404 cases.

---

### CR-02: kwik.go default Referer is the dead/blocked domain animepahe.ru

**Files modified:** `services/scraper/internal/embeds/kwik.go`
**Commit:** c4f8b86
**Applied fix:** Changed the default Referer fallback from `https://animepahe.ru` to `https://animepahe.pw/` (with trailing slash). This aligns with the `kwikReferer` constant in `resolver.go:37` and the post-Phase-27 operating posture.

---

### WR-01: initBrowser() leaves partially-initialized browser on failure

**Files modified:** `services/animepahe-resolver/browser.js`
**Commit:** c46d172
**Applied fix:** The IIFE now uses local variables `newBrowser` and `newPage` during initialization. The module-level `browser` and `page` singletons are assigned only after all init steps succeed. A `catch` block closes `newBrowser` if it was created but a subsequent step (`newPage()` or `goto()`) failed. This prevents leaked browser processes on partial init failures.

---

### WR-02: Non-200/non-403 upstream statuses trigger unnecessary page recycle

**Files modified:** `services/animepahe-resolver/upstream.js`
**Commit:** 9cea8c7
**Applied fix:** The `requestCount += 1` and `maybeRecycle()` call are now guarded by `if (result.status >= 200 && result.status < 300)`. Error responses (404, 5xx) no longer advance the recycle budget or trigger a page recycle.

---

### WR-03: hostnameOf function is defined but never called (dead code)

**Files modified:** `services/scraper/internal/providers/animepahe/client.go`
**Commit:** 6f73a77
**Applied fix:** Deleted the `hostnameOf` function (lines 451-458). No callers existed anywhere in the package. Build confirmed clean post-deletion.

---

### WR-04: UPSTREAM_BASE_URL env var silently ignored by the code

**Files modified:** `docker/docker-compose.yml`
**Commit:** 9006c4e
**Applied fix:** Renamed `UPSTREAM_BASE_URL` to `_HARDCODED_UPSTREAM` with a multi-line comment making its non-functional audit-documentation purpose unmistakable. Added file+line references (`server.js:37, browser.js:31`) pointing to the hardcoded constants.

---

### WR-05: parseServerPriority doc comment merged with parseDegradedProviders comment

**Files modified:** `services/scraper/internal/config/config.go`
**Commit:** 2c59efd
**Applied fix:** Split the merged comment block. `parseDegradedProviders` now has its own standalone doc comment. `parseServerPriority` (which previously appeared after `parseDegradedProviders` with no doc comment) now has the correct comment preceding it. `go doc` will now attach each comment to the correct function.

---

### WR-06: _setTestDoubles exported with no production guard

**Files modified:** `services/animepahe-resolver/browser.js`
**Commit:** 2c28027
**Applied fix:** Added `if (process.env.NODE_ENV === 'production') { throw new Error('_setTestDoubles must not be called in production'); }` guard at the top of `_setTestDoubles`. This prevents accidental invocation in the production container (where it would null out the warm browser/page singletons and cause all subsequent requests to fail with `browser_down`).

---

## Post-Fix Test Results

**Go tests:**
```
ok  github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/animepahe  0.039s
ok  github.com/ILITA-hub/animeenigma/services/scraper/internal/embeds               0.200s
ok  github.com/ILITA-hub/animeenigma/services/scraper/internal/config               0.002s
```

**Node tests:** 24/24 pass (includes 3 new CR-01 tests for upstream 404/5xx propagation).

**Production health:** Both `animeenigma-animepahe-resolver` and `animeenigma-scraper` containers are `healthy` post-redeploy.

---

_Fixed: 2026-05-19T13:00:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
