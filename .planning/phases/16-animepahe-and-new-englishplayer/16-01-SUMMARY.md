---
phase: 16-animepahe-and-new-englishplayer
plan: 01
subsystem: infra
tags: [animepahe, scraper, ddos-guard, cookie-jar, golden-fixtures, hls-proxy, regression-lock, makefile]

requires:
  - phase: 15-foundation
    provides: BaseHTTPClient (services/scraper/internal/domain/httpclient.go) with private cookiejar.Jar; HLSProxyAllowedDomains slice (libs/videoutils/proxy.go) with AnimePahe CDN hosts already added in prior work

provides:
  - Public BaseHTTPClient.Jar() http.CookieJar accessor — unblocks DDoS-Guard cookie inspection in Plan 16-03
  - Documented connectivity proof for AnimePahe (Cloudflare → DDoS-Guard 403 reachable; direct animepahe.ru TCP-timed-out)
  - Four upstream-shaped fixtures in services/scraper/testdata/animepahe/ — search JSON, release JSON, /play HTML, kwik embed HTML
  - Regression test locking kwik.cx + owocdn.top + uwucdn.top in HLSProxyAllowedDomains (SCRAPER-PAHE-05)
  - Makefile recipe `capture-goldens-animepahe` with built-in anonymization gate

affects:
  - 16-02 (Kwik EmbedExtractor uses kwik_e_abc.html fixture)
  - 16-03 (AnimePahe Provider uses Jar() accessor, search_naruto.json, release_4_p1.json, play_session_ep1.html)
  - 16-05 (boot wiring relies on Plan 16-03 provider being green)
  - Future Phase 18/19/20 HLS proxy work (regression test guards against accidental host removal)

tech-stack:
  added: []  # no new libs — single one-line accessor on existing BaseHTTPClient
  patterns:
    - "Connectivity-probe-first task ordering: prove TCP/DNS to a new upstream before adding provider code"
    - "Synthetic-but-shape-accurate goldens when upstream is gated by JS-solved anti-bot challenge"
    - "Regression-lock tests that pin existing behavior (test passes on first run; commits the assertion so future PRs can't silently regress)"
    - "Inline anonymization gate inside the capture Makefile target to keep upstream cookies out of git"

key-files:
  created:
    - services/scraper/testdata/animepahe/README.md (134 lines — probe evidence, anchors, anonymization, recapture procedure)
    - services/scraper/testdata/animepahe/search_naruto.json (upstream m=search shape)
    - services/scraper/testdata/animepahe/play_session_ep1.html (data-src buttons → kwik.cx)
  modified:
    - services/scraper/internal/domain/httpclient.go (+7 lines — Jar() accessor)
    - services/scraper/internal/domain/httpclient_test.go (+38 lines — two new tests)
    - libs/videoutils/proxy_test.go (+19 lines — regression-lock test)
    - Makefile (+11 lines — capture-goldens-animepahe target with grep gate)
    - services/scraper/testdata/animepahe/release_4_p1.json (replaced salvaged 404 HTML with upstream-shaped JSON)

key-decisions:
  - "AnimePahe is reachable from the docker host via Cloudflare-fronted animepahe.com → animepahe.pw with HTTP 403 + Server: ddos-guard. Direct animepahe.ru TCP-times-out as RESEARCH.md predicted. Per the plan's acceptable-outcome #2, this proves the connection is alive and the JSON gate is purely the DDoS-Guard JS challenge — Plan 16-03 will solve it via the new Jar() accessor."
  - "DDoS-Guard JS challenge prevented direct 200-status capture today, so all four fixtures were authored against the documented Kohi-den upstream shapes with deterministic placeholder IDs/sessions. README.md flags the recapture procedure for after Plan 16-03 lands."
  - "Regression-lock test pattern: HLSProxyAllowedDomains_HasAnimePaheHosts passes on first run (hosts already present from prior phase work) — the value is preventing accidental removal in future PRs, not changing current behavior. This is how SCRAPER-PAHE-05 lands as a CI-enforced contract."

patterns-established:
  - "BaseHTTPClient public read accessors: Timeout() and Jar() expose runtime state for providers and tests without exposing the underlying retryablehttp.Client. Future accessors should follow this same shape — one-line method, named after the resource, return the interface (not the concrete type)."
  - "Phase-N upstream fixtures live at services/{service}/testdata/{provider}/ with a mandatory README.md that documents the capture procedure + anonymization gate."

requirements-completed:
  - SCRAPER-PAHE-04
  - SCRAPER-PAHE-05

duration: 5min
completed: 2026-05-12
---

# Phase 16 Plan 01: AnimePahe Connectivity + Foundations Summary

**Connectivity probe proves AnimePahe is alive behind DDoS-Guard; BaseHTTPClient.Jar() accessor unblocks Plan 16-03; four upstream-shaped fixtures and a CI-enforced regression lock on the HLS allowlist now ship.**

## Performance

- **Duration:** 5 min
- **Started:** 2026-05-12T04:14:39Z
- **Completed:** 2026-05-12T04:19:48Z
- **Tasks:** 3 (Task 1 single commit, Tasks 2 + 3 each RED + GREEN, 5 commits total)
- **Files modified:** 5 (3 created, 2 modified, 1 replaced)

## Accomplishments

- Documented connectivity proof: animepahe.com 301 → animepahe.pw HTTP 403 with `Server: ddos-guard` and `__ddg8_/9/10/id/mark_` cookies set on the response — TCP + DNS + TLS layer all alive; only the JS challenge gates JSON content. This matches the plan's "acceptable outcome #2" and unlocks Plan 16-03 work.
- Added `func (c *BaseHTTPClient) Jar() http.CookieJar` (one-line implementation, two tests covering round-trip + stable instance). This is the Phase-15 amend mandated by 16-RESEARCH.md §Pattern 3 / Assumption A4.
- Committed four AnimePahe upstream fixtures (search JSON, release JSON, /play HTML with `data-src="https://kwik.cx/..."` buttons, kwik packed-JS embed HTML). Anonymization grep gate passes — no `Set-Cookie` / `__ddg2_` / `cf_clearance` / `Bearer ` tokens leaked.
- Locked the HLS proxy allowlist with a CI-enforced regression test on `kwik.cx`, `owocdn.top`, `uwucdn.top` — satisfies SCRAPER-PAHE-05 as a contract rather than a code change.
- Added Makefile target `capture-goldens-animepahe` with an inline anonymization gate so future fixture refreshes can't accidentally land upstream cookies.

## Task Commits

Each task was committed atomically:

1. **Task 1: Connectivity probe + capture goldens** — `bf9d166` (test) — README.md + 3 new fixtures + 1 replaced fixture
2. **Task 2 RED: Jar() accessor failing tests** — `b2018d3` (test) — TestBaseHTTPClient_JarAccessor + TestBaseHTTPClient_JarAccessor_StableInstance fail at compile time
3. **Task 2 GREEN: Jar() accessor** — `a7773fb` (feat) — one-line public method delegating to `c.jar`
4. **Task 3 regression-lock: HLS allowlist test** — `1e7a796` (test) — TestHLSProxyAllowedDomains_HasAnimePaheHosts passes on first run, locks the three CDN hosts forever
5. **Task 3 Makefile recipe: capture-goldens-animepahe** — `e1604c5` (feat) — entry-point target with anonymization grep gate

Plus the prior-worktree salvage commit `9318c32` (also on this branch from main) which seeded the initial release JSON file that Task 1 then replaced with the corrected upstream-shaped JSON.

## Files Created/Modified

### Created
- `services/scraper/testdata/animepahe/README.md` — capture date, host topology, verbatim probe headers, fixture anchors, anonymization rules, recapture procedure
- `services/scraper/testdata/animepahe/search_naruto.json` — `{"total":4,"data":[{id,session,title,...},...]}` matching m=search shape
- `services/scraper/testdata/animepahe/play_session_ep1.html` — `<button data-src="https://kwik.cx/..." data-audio data-resolution data-fansub data-av1>` rows

### Modified
- `services/scraper/internal/domain/httpclient.go` — `+7 lines` adding `Jar() http.CookieJar` directly after `Timeout()`
- `services/scraper/internal/domain/httpclient_test.go` — `+38 lines` adding the two RED→GREEN tests
- `libs/videoutils/proxy_test.go` — `+19 lines` adding `TestHLSProxyAllowedDomains_HasAnimePaheHosts`
- `Makefile` — `+11 lines` adding `capture-goldens-animepahe` PHONY target with built-in grep gate
- `services/scraper/testdata/animepahe/release_4_p1.json` — replaced the salvaged HTML 404 page with deterministic upstream-shaped JSON containing `current_page`, `last_page`, and 5 episode records

## Decisions Made

- **Synthetic fixtures, deterministic placeholder IDs.** Direct upstream capture today returns DDoS-Guard JS challenge bodies (898 bytes of HTML, no JSON). Rather than block the plan, fixtures match the documented Kohi-den shape with placeholder IDs/sessions. The README captures the recapture procedure for after Plan 16-03's cookie helper lands.
- **Regression-lock test instead of code change.** The three AnimePahe CDN hosts are already in `HLSProxyAllowedDomains` from prior phase work. The test asserts their presence so future PRs cannot silently remove them — this is how SCRAPER-PAHE-05 is satisfied as a CI-enforced contract.
- **Anonymization gate inline in the Makefile target, not a separate script.** Keeps the gate adjacent to the recapture command so it can't be forgotten when fixtures are refreshed.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Replaced salvaged `release_4_p1.json` (was an HTML 404 page) with upstream-shaped JSON**
- **Found during:** Task 1 verification — `grep -q "current_page" services/scraper/testdata/animepahe/release_4_p1.json` returned no hits because the salvaged file from commit `9318c32` was actually AnimePahe's 404 HTML wrapper, not the expected release JSON.
- **Fix:** Replaced the file with deterministic upstream-shaped JSON (5 episodes, all required fields per Kohi-den reference) so Plan 16-03's `ListEpisodes` test can decode it.
- **Files modified:** services/scraper/testdata/animepahe/release_4_p1.json
- **Verification:** `jq '.current_page'` returns `1`; required content anchors present.
- **Committed in:** `bf9d166` (Task 1 fixtures commit)

**2. [Rule 3 - Blocking] Worktree branch was forked from older base; rebased onto main before starting work**
- **Found during:** Step 0 (load_plan) — the Plan 16-01 plan file did not exist in the worktree filesystem because the branch was forked from commit `788a661` before phase-16 plans were committed.
- **Fix:** `git rebase main` to bring the worktree branch to `9318c32` so the plan file is visible.
- **Files modified:** none (rebase only)
- **Verification:** `.planning/phases/16-animepahe-and-new-englishplayer/16-01-PLAN.md` now readable.
- **Committed in:** not committed (rebase, not a code change).

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking infrastructure)
**Impact on plan:** Both auto-fixes were necessary for correctness — without the JSON replacement, Plan 16-03's decode tests would fail against malformed input; without the rebase, the plan file was unreachable. No scope creep.

## Issues Encountered

- **DDoS-Guard JS challenge blocked direct capture** from both inside the docker bridge network and from the docker host. The plan anticipated this (acceptable-outcome #2 + the kwik-DDoS-Guard fallback paragraph) and Plan 16-03 will install the cookie helper. Resolution: document the probe headers verbatim in README.md and ship upstream-shaped synthetic fixtures; flag recapture as Plan 16-03 follow-up via the Makefile target.
- **TCP timeout to animepahe.ru** from this server's docker network and host network is real (matches RESEARCH.md). The Cloudflare-fronted animepahe.com hostname is the working entry point; the plan's `ANIMEPAHE_BASE_URL` env var (introduced in Plan 16-05) should default to the Cloudflare alias on hosts where the direct .ru hostname times out.

## Threat Flags

None — Plan 16-01 introduced no new network endpoints, auth paths, file access patterns, or schema changes at trust boundaries beyond what the plan's `<threat_model>` already covered (T-16-01-01..04).

## User Setup Required

None — no external service configuration required for this plan.

## Next Phase Readiness

- **Plan 16-02 (Kwik EmbedExtractor):** `kwik_e_abc.html` fixture is ready; the `eval(function(p,a,c,k,e,d)` anchor is present so the Plan 16-02 RED test (already committed at `f288706`) will turn GREEN once the extractor implementation lands.
- **Plan 16-03 (AnimePahe Provider):** `BaseHTTPClient.Jar()` accessor is live; `search_naruto.json`, `release_4_p1.json`, `play_session_ep1.html` fixtures are ready for the provider's decode tests; the recapture Makefile target is ready to refresh them once the DDoS-Guard handshake is in place.
- **Plan 16-05 (boot wiring):** Add `ANIMEPAHE_BASE_URL` env var with a default that respects the documented connectivity reality (the direct .ru hostname is TCP-blocked from this host; the Cloudflare-fronted alias works).
- **Concern noted:** the kwik fixture is a deliberately small synthetic that mirrors the wrapper shape but not the full p,a,c,k,e,d unpacking math. Plan 16-02's extractor test will validate the wrapper logic against this fixture, but a full integration test against a real Kwik upstream will only be possible after Plan 16-03's cookie helper warms the jar.

## Self-Check

Verified before SUMMARY commit:
- `services/scraper/internal/domain/httpclient.go` — Jar() method present (`func (c *BaseHTTPClient) Jar() http.CookieJar`)
- `services/scraper/internal/domain/httpclient_test.go` — both new tests present
- `libs/videoutils/proxy_test.go` — regression-lock test present
- `Makefile` — `capture-goldens-animepahe` target present with grep gate
- `services/scraper/testdata/animepahe/{README,search_naruto.json,release_4_p1.json,play_session_ep1.html,kwik_e_abc.html}` — all present, all anchor checks pass, anonymization gate clean
- Commits `bf9d166`, `b2018d3`, `a7773fb`, `1e7a796`, `e1604c5` all present in `git log`

## Self-Check: PASSED

---
*Phase: 16-animepahe-and-new-englishplayer*
*Plan: 01*
*Completed: 2026-05-12*
