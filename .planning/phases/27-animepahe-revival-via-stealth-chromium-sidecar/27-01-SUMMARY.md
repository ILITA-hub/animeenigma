---
phase: 27-animepahe-revival-via-stealth-chromium-sidecar
plan: 01
subsystem: infra
tags: [puppeteer, puppeteer-extra, stealth-plugin, fastify, pino, prom-client, docker, chromium, ddos-guard, animepahe, node, sidecar]

# Dependency graph
requires:
  - phase: 24-en-reconnect
    provides: SCRAPER-HEAL-20 verdict-log shape — this plan's downstream Plan 27-04 re-runs the Phase 24 hard-gate curl pipeline to flip the `animepahe` row from FAIL to PASS.
provides:
  - New `services/animepahe-resolver/` Node 20 + Fastify + puppeteer-extra stealth sidecar (server.js / browser.js / upstream.js / metrics.js).
  - HTTP API on `:3000` — `GET /healthz` (Pattern 4 two-layer probe), `GET /search?q=`, `GET /release?session=&page=`, `GET /play?animeSession=&episodeSession=`, `GET /metrics`.
  - Pattern 1 (warm browser, single page on default BrowserContext), Pattern 2 (403 → re-nav → retry once), Pattern 3 (page recycle at N=PAGE_RECYCLE_AT, default 100), Pattern 4 (two-layer healthcheck) — all live.
  - Exact-pinned `puppeteer-extra@3.3.6` + `puppeteer-extra-plugin-stealth@2.11.2` + committed `package-lock.json` + `npm ci --omit=dev` Dockerfile (per CONTEXT D6 + Pitfall 2).
  - V4 ASVS host-allowlist on every upstream fetch (T-27-01-01 defense-in-depth).
  - Pino logger with `redact: ['req.headers.cookie', 'res.headers["set-cookie"]']` (T-27-01-04 cookie redaction).
  - Prometheus counters `stealth_challenge_failures_total`, `stealth_challenge_solves_total`, `page_recycle_total`, `upstream_403_total{stage}`.
  - 21-test offline `node --test` suite covering schema validation 400s, host-allowlist, two-layer healthz (200/503/503), search/release/play passthrough, 502 propagation, recycle counter, browser-down guards.
  - `STEALTH-PINS.md` operator doc + refresh procedure + D5 empirical RSS log.
  - `.claude/maintenance-prompt.md` Pattern 7 animepahe-resolver branch.
  - `Makefile` explicit `redeploy-animepahe-resolver` target.
  - D5 100-request memory soak gate PASSED (236 MiB peak / 500 MB cap).
affects:
  - 27-02 (parser rewrite — consumes the resolver HTTP API; captures `testdata/animepahe/frieren-{search,release,play}.{json,html}` against this deployed sidecar per CONTEXT D4)
  - 27-03 (docker-compose wiring — uses `services/animepahe-resolver/Dockerfile`, the `:3000` health endpoint, and the 500 MB + 256 MB shm resource shape this plan validated)
  - 27-04 (end-to-end gate-clear — Phase 24 curl pipeline against this sidecar's parser path)
  - 27-05 (`SCRAPER_DEGRADED_PROVIDERS` cleanup — gated on 27-04 evidence)
  - Future maintenance-bot DDoS-Guard rotation handling — via Pattern 7 branch + STEALTH-PINS.md refresh procedure

tech-stack:
  added:
    - puppeteer-extra 3.3.6 (exact pin; plugin host)
    - puppeteer-extra-plugin-stealth 2.11.2 (exact pin; DDoS-Guard defeat)
    - puppeteer ^24.0.0 (caret; tested with puppeteer:24 base image's bundled Chrome 148)
    - fastify ^4.28.0 (caret; HTTP server + built-in JSON schema validation)
    - pino ^9.5.0 (caret; cookie-redacting JSON logger)
    - prom-client ^15.1.3 (caret; Prometheus client mirroring libs/metrics on the Go side)
    - ghcr.io/puppeteer/puppeteer:24 base image (Chrome for Testing pre-bundled, dbus + fonts + userns)
  patterns:
    - Warm browser + warm page on the DEFAULT BrowserContext (NOT createIncognitoBrowserContext — Pitfall 6 cookie persistence across recycle)
    - 403 retry with re-navigation (Pattern 2) — once-and-done; second 403 → 502
    - Page recycle on every PAGE_RECYCLE_AT-th request using overlap-order tab swap (Pattern 3 + Pitfall 4 default; close-first remains opt-in for future RSS-tight scenarios)
    - Two-layer healthcheck (HTTP responsive AND `page.evaluate(()=>1)` within 2 s) — distinguishes Node-alive-but-Chromium-hung from healthy
    - Exact-pinned puppeteer-extra* + `npm ci` Dockerfile (NOT npm install) for reproducible builds (CONTEXT D6 + Pitfall 2)
    - V4 ASVS host-allowlist at the fetch boundary as defense-in-depth (route handlers already build URLs from a hardcoded base)
    - In-page `await page.evaluate(async (u) => fetch(u, { credentials: 'include' }))` for upstream fetches — preserves DDoS-Guard cookies inside the browsing context (avoids per-request `page.goto` re-trigger of the challenge)
    - Pino redaction of cookie + set-cookie headers (T-27-01-04 — DDoS-Guard cookie values NEVER appear in Loki logs or `/metrics` label values)
    - Dependency-injection-friendly module shape (`browser._setTestDoubles(...)`) so `node --test` runs deterministically without a real Chromium

key-files:
  created:
    - services/animepahe-resolver/server.js — Fastify app factory + /healthz two-layer probe + schema-validated /search /release /play + /metrics
    - services/animepahe-resolver/browser.js — singleton Chromium launcher + warm-page management + recyclePage() + refreshChallenge() + _setTestDoubles() test hook
    - services/animepahe-resolver/upstream.js — fetchWithRetry (Pattern 2) + maybeRecycle (Pattern 3) + assertAllowedHost (V4 ASVS) + ResolverError class
    - services/animepahe-resolver/metrics.js — prom-client registry with the four counters + default process metrics
    - services/animepahe-resolver/package.json — exact pins puppeteer-extra@3.3.6 + stealth@2.11.2 + caret others; engines.node >=20.0.0
    - services/animepahe-resolver/package-lock.json — committed; Dockerfile uses npm ci
    - services/animepahe-resolver/Dockerfile — FROM ghcr.io/puppeteer/puppeteer:24; PUPPETEER_SKIP_DOWNLOAD=true; npm ci --omit=dev
    - services/animepahe-resolver/STEALTH-PINS.md — pin table + refresh procedure + D5 100-request soak empirical log
    - services/animepahe-resolver/README.md — operator-facing HTTP API + metrics + local-dev quickstart + playbook + env-var reference
    - services/animepahe-resolver/.gitignore — node_modules only
    - services/animepahe-resolver/server.test.js — 15 tests covering schema validation, host-allowlist, healthz, search/release/play, 502, metrics surface
    - services/animepahe-resolver/upstream.test.js — 6 tests covering 403 retry, second-403 failure, page-recycle counter, browser-down guards
    - services/animepahe-resolver/__fixtures__/intercepted-frieren.json — offline test fixture (search/release/play shapes)
  modified:
    - Makefile — added explicit redeploy-animepahe-resolver target (mirrors redeploy-web shape; also added to .PHONY)
    - .claude/maintenance-prompt.md — added Pattern 7 animepahe-resolver branch pointing at STEALTH-PINS.md refresh procedure

key-decisions:
  - "Fastify over Express — Fastify's built-in JSON schema route validation eliminates a class of input-validation bugs at the API boundary (T-27-01-01 defense-in-depth complementing the host-allowlist guard) and supports Pino redaction natively. 2-4x throughput is a bonus, not the deciding factor."
  - "Exact pins on puppeteer-extra* + caret on everything else — CONTEXT D6 mandates exact pinning of the DDoS-Guard defeat layer because minor version bumps have historically removed override properties that specific challenges probe. Caret on puppeteer/fastify/pino/prom-client is safe because they follow semver and are actively maintained."
  - "Default BrowserContext (NOT createIncognitoBrowserContext) — Pitfall 6 cookie persistence: closing a tab on the default context preserves the DDoS-Guard cookie jar across recycles; an incognito context would force a re-challenge on every recycle."
  - "Overlap-order page recycle by default — the D5 soak peaked at 236 MiB, well below the 450 MiB sentinel that would force the close-first remediation (Pitfall 4). Close-first remains opt-in via opts argument and documented for future RSS-tight scenarios."
  - "Honor PUPPETEER_EXECUTABLE_PATH if set; auto-detect otherwise — Rule-1 auto-fix during Task 4 discovered puppeteer:24 ships Chrome under PUPPETEER_CACHE_DIR (not /usr/bin/google-chrome-stable). Keeping the env-var read in browser.js lets operators override on hosts with a system Chrome they prefer."

patterns-established:
  - "Sidecar transport layer: the Node sidecar is THIN — it owns warm-browser lifecycle + DDoS-Guard solving + host-allowlist + Prometheus surface. Everything else (Redis caching, fuzzy matching, MalSync, retry/rate-limit policy across providers) belongs to the Go scraper. This is the same shape megacloud-extractor uses, generalized to puppeteer-extra."
  - "Test-double injection via module hook (browser._setTestDoubles): lets node --test run fully offline against deterministic fakes without monkey-patching require(); the same hook pattern will work for the upcoming 27-02 Go-side resolver client tests via httptest.NewServer."
  - "Operator-doc sentinel for self-healing: STEALTH-PINS.md is THE single source of truth for the maintenance-bot's Pattern 7 animepahe-resolver branch. The refresh procedure is a one-line shell command the bot can execute; the bot escalates to `escalate` tier only if the refresh test fails."

requirements-completed:
  - SCRAPER-HEAL-29

# Metrics
duration: 18min
completed: 2026-05-19
---

# Phase 27 Plan 01: Stealth-Chromium Sidecar Scaffold Summary

**A Node 20 + Fastify + puppeteer-extra stealth sidecar (`services/animepahe-resolver/`) that warms a single Chromium page against `animepahe.pw`, exposes `:3000` with `/healthz` (two-layer probe), `/search`, `/release`, `/play`, `/metrics`, defeats DDoS-Guard via the pinned stealth plugin, and stays under 236 MiB RSS over a 100-request live soak — well below the 500 MB D5 hard ship gate.**

## Performance

- **Duration:** ~18 min (Phase 27 execution start 2026-05-19T10:14:26Z → SUMMARY commit 2026-05-19T10:32Z)
- **Started:** 2026-05-19T10:14:26Z
- **Completed:** 2026-05-19T10:32:36Z
- **Tasks:** 4 / 4
- **Files created:** 13
- **Files modified:** 2 (Makefile, .claude/maintenance-prompt.md)
- **Test count:** 21 / 21 passing (offline, ~2.5 s)

## Accomplishments

- New `services/animepahe-resolver/` Node 20 + Fastify sidecar scaffold with Patterns 1-4 from RESEARCH all live (warm browser / 403 retry / page recycle / two-layer healthcheck).
- Exact-pinned stealth stack (`puppeteer-extra@3.3.6` + `puppeteer-extra-plugin-stealth@2.11.2`) with committed `package-lock.json` and `npm ci --omit=dev` Dockerfile — pin enforcement per CONTEXT D6 + Pitfall 2.
- 21 offline unit tests covering schema validation, host-allowlist defense-in-depth, two-layer healthz, /search /release /play passthrough, 502 / non-JSON error paths, /metrics surface, 403 retry (first + second), page-recycle counter, and browser-down guards.
- **D5 hard ship gate PASSED in a single run**: 100/100 HTTP 200 responses with PEAK_RSS = 236 MiB (47% of the 500 MB cap), `page_recycle_total` = 1, `stealth_challenge_solves_total` = 1, `stealth_challenge_failures_total` = 0, 0 / 100 502 responses, no OOMKill events.
- Maintenance-bot self-healing path wired: `.claude/maintenance-prompt.md` Pattern 7 gets an animepahe-resolver branch pointing at `STEALTH-PINS.md` § Refresh Procedure (single-line npm-install + npm-test + make-redeploy command, plus escalate fallback).
- `Makefile` explicit `redeploy-animepahe-resolver` target (mirrors `redeploy-web` shape) + `.PHONY` entry, giving the next plan a discoverable redeploy verb.

## Task Commits

Each task committed atomically against per-task verify gates:

1. **Task 1: Scaffold package + Dockerfile + Fastify entry + browser singleton** — `cbf3dd2` (feat) — 8 files created: package.json (exact pins), package-lock.json (2146 lines, committed), Dockerfile (puppeteer:24 base + npm ci), .gitignore, browser.js (singleton + warm page + recyclePage + refreshChallenge), server.js (Fastify factory + /healthz Pattern 4 + /search /release /play schemas + /metrics + hardcoded-upstream invariant header comment), metrics.js (4-counter prom-client registry), upstream.js (host-allowlist scaffold).
2. **Task 2: Upstream fetch wrapper with 403-retry + N=100 page recycle + metrics** — `bdc47c2` (feat) — upstream.js expanded with Pattern 2 (403 → refreshChallenge → retry once → second 403 fails as 502) + Pattern 3 (maybeRecycle at every PAGE_RECYCLE_AT-th request, default 100, env-overridable) + metric wires for all four counters + lastChallengeSolveAt timestamp on successful retry.
3. **Task 3: Tests + Frieren fixtures + STEALTH-PINS + Pattern 7 branch + Makefile target** — `5b892bd` (feat) — server.test.js (15 tests) + upstream.test.js (6 tests) + __fixtures__/intercepted-frieren.json + STEALTH-PINS.md (pin table + refresh procedure + D5 TODO placeholder) + README.md + Makefile redeploy target + .claude/maintenance-prompt.md Pattern 7 animepahe-resolver branch.
4. **Task 4: Build + 100-request memory-soak gate (D5 hard ship gate)** — `1fb209d` (chore) — D5 gate PASSED single-run; STEALTH-PINS.md D5 section filled with empirical values; Rule-1 Dockerfile + browser.js auto-fix (PUPPETEER_EXECUTABLE_PATH was wrong path for puppeteer:24 base — removed Dockerfile hardcode, kept env-var read in browser.js as optional override).

## Files Created/Modified

**Created — sidecar implementation:**
- `services/animepahe-resolver/server.js` (220 lines) — Fastify app factory; routes; /healthz Pattern 4; HARDCODED-upstream header invariant
- `services/animepahe-resolver/browser.js` (155 lines) — singleton Chromium launcher; warm page on DEFAULT BrowserContext; recyclePage (overlap-order default; close-first opt-in); refreshChallenge; _setTestDoubles test hook
- `services/animepahe-resolver/upstream.js` (175 lines) — fetchWithRetry + maybeRecycle + assertAllowedHost + ResolverError; metric increments on all paths
- `services/animepahe-resolver/metrics.js` (60 lines) — prom-client registry; four counters; default process metrics; _resetForTests hook
- `services/animepahe-resolver/package.json` — exact stealth pins; caret others
- `services/animepahe-resolver/package-lock.json` (2146 lines) — reproducible build pin enforcement
- `services/animepahe-resolver/Dockerfile` — puppeteer:24 base; npm ci --omit=dev; no executablePath hardcode
- `services/animepahe-resolver/.gitignore`

**Created — tests + fixtures:**
- `services/animepahe-resolver/server.test.js` (15 tests) — schema validation × 5; host-allowlist; healthz × 3; search/release/play passthrough; 502 / bad-json; /metrics surface
- `services/animepahe-resolver/upstream.test.js` (6 tests) — 403 retry success; double-403 failure; PAGE_RECYCLE_AT=3 recycle counter; host-allowlist; browser_down guard; maybeRecycle no-op at req=0
- `services/animepahe-resolver/__fixtures__/intercepted-frieren.json` — search + release + play offline fixture

**Created — operator docs:**
- `services/animepahe-resolver/STEALTH-PINS.md` — pin manifest, refresh procedure, why-exact-not-caret rationale, hardcoded-upstream invariant, D5 100-request soak empirical log
- `services/animepahe-resolver/README.md` — HTTP API table, metrics table, local-dev + docker-run quickstart, operator playbook, env-var reference

**Modified — project integration:**
- `Makefile` — added `redeploy-animepahe-resolver` to .PHONY and as an explicit target (mirrors `redeploy-web` shape; coexists with `redeploy-%` wildcard for `make help` discoverability)
- `.claude/maintenance-prompt.md` — Pattern 7 gains a new bullet pointing at STEALTH-PINS.md refresh procedure with `button_fix` tier + `escalate` fallback to re-add `animepahe` to `SCRAPER_DEGRADED_PROVIDERS`

## Decisions Made

- **Fastify over Express** — built-in JSON schema route validation eliminates a class of input-validation bugs at the API boundary; complements the host-allowlist guard as defense-in-depth.
- **Exact pins on puppeteer-extra* + caret on others** — CONTEXT D6 + Pitfall 7. Minor stealth-plugin bumps have historically removed override properties that specific DDoS-Guard challenges probe; pinning is the defensible operating posture.
- **Default BrowserContext** — Pitfall 6 cookie persistence: closing a tab on the default context preserves the DDoS-Guard cookie jar; an incognito context would force re-challenge on every recycle.
- **Overlap-order page recycle by default** — Pitfall 4 alternative; the D5 soak peaked at 236 MiB so we have 263 MiB of headroom and don't need the close-first remediation. Close-first stays opt-in.
- **In-page `fetch(url, { credentials: 'include' })` over per-request `page.goto`** — Pitfall 6 cookie scope: cookies set on the warm page during initial warmup `goto` flow naturally into in-page fetches; using `page.goto(url)` per request would re-trigger DDoS-Guard's challenge on every navigation.
- **Open Question Q4 (RESOLVED)**: `await initBrowser()` BEFORE `fastify.listen({port:3000,host:'0.0.0.0'})` — no warmup race. By the time HTTP listens, the warm page is up; `/healthz` is meaningful from the first accepted connection.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Dockerfile + browser.js — PUPPETEER_EXECUTABLE_PATH pointed at a binary that doesn't exist in puppeteer:24**

- **Found during:** Task 4 (D5 soak) — first `docker run` exited with `Error: Browser was not found at the configured executablePath (/usr/bin/google-chrome-stable)`.
- **Issue:** RESEARCH.md cited `/usr/bin/google-chrome-stable` as the bundled-Chrome path, but the current `ghcr.io/puppeteer/puppeteer:24` image actually ships Chrome under `/home/pptruser/.cache/puppeteer/chrome/linux-148.*/chrome-linux64/chrome`. Setting `PUPPETEER_EXECUTABLE_PATH=/usr/bin/google-chrome-stable` made `puppeteer.launch()` fail at startup before any page could warm.
- **Fix:** Removed the `PUPPETEER_EXECUTABLE_PATH` ENV from `Dockerfile`; kept the env-var read in `browser.js` (`executablePath: process.env.PUPPETEER_EXECUTABLE_PATH || undefined`) so operators can still override on hosts that prefer a system Chrome. When unset, puppeteer resolves via `PUPPETEER_CACHE_DIR` (preset by the base image) and finds the bundled Chrome automatically. Updated README's env-var table to document the new optional shape.
- **Files modified:** `services/animepahe-resolver/Dockerfile`, `services/animepahe-resolver/browser.js`, `services/animepahe-resolver/README.md`
- **Verification:** Rebuilt the image; the container starts in ~8 s; `/healthz` flips green; D5 100-request soak runs end-to-end with 0 502s.
- **Committed in:** `1fb209d` (part of the Task 4 commit)

---

**Total deviations:** 1 auto-fixed (1 × Rule 1 bug).
**Impact on plan:** None on scope or design. The fix was necessary for the D5 ship gate to be runnable at all, did not change the architecture, and improved the env-var ergonomics (operator override path is now documented).

## Issues Encountered

- **None of the planned work hit a blocker.** The Task 4 PUPPETEER_EXECUTABLE_PATH issue was discovered and fixed automatically via Rule 1 within the same task — see Deviations.
- **D5 hard ship gate PASSED single-run** — no remediation iterations needed. Overlap-order recycle was sufficient at 236 MiB peak (47% of cap).

## User Setup Required

None — the sidecar is fully self-contained. Plan 27-03 will wire it into `docker/docker-compose.yml` with the same resource limits (500 MB mem + 256 MB shm) validated here. No external API keys, no secrets in `docker/.env`.

## Next Phase Readiness

**Ready for Plan 27-02** (parser rewrite):

- Sidecar HTTP API contract is fixed (`GET /search?q`, `GET /release?session&page`, `GET /play?animeSession&episodeSession`); Plan 27-02's Go-side `resolver.go` HTTP client implements this contract verbatim per RESEARCH § Go-side resolver HTTP client code example.
- Per CONTEXT D4, Plan 27-02 captures fresh `testdata/animepahe/frieren-{search,release,play}.{json,html}` against THIS deployed sidecar (build the dev image via the same `docker run` snippet documented in `README.md` § "Building + running locally" and `STEALTH-PINS.md` § "Re-run command"). The offline `__fixtures__/intercepted-frieren.json` is for the Node side only; the Go side gets its own goldens.

**Ready for Plan 27-03** (compose wiring):

- Dockerfile, `:3000` listen address, `/healthz` two-layer probe endpoint, and the validated `--memory=500m --shm-size=256m` resource shape are all live. Plan 27-03 mechanically lifts the compose block from RESEARCH § "Compose service block".
- `Makefile redeploy-animepahe-resolver` target already exists.

**Ready for the maintenance-bot's existing self-heal pipeline:**

- `.claude/maintenance-prompt.md` Pattern 7 animepahe-resolver branch is in place; the single-line refresh command in `STEALTH-PINS.md` § Refresh Procedure is what the bot executes when `stealth_challenge_failures_total` rises sustained > 1h.

**No blockers for downstream plans.**

## Threat Surface Scan

This plan's `<threat_model>` covered T-27-01-01 through T-27-01-05. Implementation evidence:

- **T-27-01-01 (Tampering / Info disclosure on upstream URL construction)** — Mitigated via TWO independent layers: (a) `server.js` route handlers build URLs from the hardcoded `https://animepahe.pw` base with `encodeURIComponent` substitution for user params; (b) `upstream.js::assertAllowedHost` rejects any URL whose hostname is not exactly `animepahe.pw` BEFORE `page.evaluate(fetch(url))`. Tested by `server.test.js` (host-allowlist test) and `upstream.test.js` (3 host-allowlist tests covering lookalike domains, sub-domains, malsync.moe).
- **T-27-01-02 (EoP on `--no-sandbox`)** — Accepted (MEDIUM); documented in `server.js` header comment AND `STEALTH-PINS.md` § Hardcoded-upstream invariant. Plan-checker should block any PR adding a second `page.goto` host.
- **T-27-01-03 (DoS via sidecar OOM)** — Mitigated: Pattern 3 page-recycle at N=100; Chromium flags `--disable-dev-shm-usage --single-process --js-flags=--max-old-space-size=384`; D5 100-request soak PASSED at 236 MiB peak. Compose-level `mem_limit: 500m` will be enforced in Plan 27-03.
- **T-27-01-04 (Info disclosure via cookies in `/metrics` or Pino logs)** — Mitigated: `metrics.js` counters have NO cookie-shaped label values (only `stage: "first"|"second"`); Pino is configured with `redact: ['req.headers.cookie', 'res.headers["set-cookie"]']`; `page.cookies()` is NEVER called or logged.
- **T-27-01-05 (Repudiation / DoS via stealth plugin defeat)** — Mitigated: `stealth_challenge_failures_total` counter ticks on second-403; Pattern 7 in `.claude/maintenance-prompt.md` has the animepahe-resolver branch referencing STEALTH-PINS.md; `SCRAPER_DEGRADED_PROVIDERS` env-override escape hatch is preserved (owned by Plan 27-05 default flip).

**New threat surface introduced beyond the modeled set:** None. The sidecar is a single-upstream transport-layer service; surface is the four documented HTTP routes (all with Fastify schema validation), and the outbound HTTPS path to a single hardcoded host.

## Self-Check: PASSED

Verification of summary claims:

```
FOUND: services/animepahe-resolver/server.js
FOUND: services/animepahe-resolver/browser.js
FOUND: services/animepahe-resolver/upstream.js
FOUND: services/animepahe-resolver/metrics.js
FOUND: services/animepahe-resolver/Dockerfile
FOUND: services/animepahe-resolver/package.json
FOUND: services/animepahe-resolver/package-lock.json
FOUND: services/animepahe-resolver/STEALTH-PINS.md
FOUND: services/animepahe-resolver/README.md
FOUND: services/animepahe-resolver/.gitignore
FOUND: services/animepahe-resolver/server.test.js
FOUND: services/animepahe-resolver/upstream.test.js
FOUND: services/animepahe-resolver/__fixtures__/intercepted-frieren.json
FOUND COMMIT: cbf3dd2 (Task 1)
FOUND COMMIT: bdc47c2 (Task 2)
FOUND COMMIT: 5b892bd (Task 3)
FOUND COMMIT: 1fb209d (Task 4)
FOUND grep: "puppeteer-extra": "3.3.6" in package.json
FOUND grep: "puppeteer-extra-plugin-stealth": "2.11.2" in package.json
FOUND grep: "npm ci --omit=dev" in Dockerfile
FOUND grep: "FROM ghcr.io/puppeteer/puppeteer:24" in Dockerfile
FOUND grep: "animepahe-resolver" in .claude/maintenance-prompt.md
FOUND grep: "redeploy-animepahe-resolver" in Makefile
FOUND grep: "D5 100-request soak" in STEALTH-PINS.md
TESTS: 21 / 21 passing via `node --test`
D5 SOAK: PASS (PEAK_RSS 236 MiB / 500 MB cap; page_recycle_total = 1; OOMKilled = false)
```

---
*Phase: 27-animepahe-revival-via-stealth-chromium-sidecar*
*Plan: 01 — Stealth-Chromium Sidecar Scaffold*
*Completed: 2026-05-19*
