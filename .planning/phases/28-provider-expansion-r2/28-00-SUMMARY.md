---
phase: 28-provider-expansion-r2
plan: 00
subsystem: scraper
tags: [miruro, obfuscation, spike, scraper-heal-34, base64url, gzip, xor-cycle]

# Dependency graph
requires:
  - phase: 26-en-revival
    provides: domain.Provider interface + provider package template
provides:
  - "Pure-Go port of Miruro's secure-pipe transform (services/scraper/internal/providers/miruro/obfuscation.go)"
  - "Reverse-engineering verdict: converged — Plan 28-04 (Miruro lift) proceeds in Wave 2"
  - "8-function test suite (3 golden vectors + negatives + size cap + xor round-trip)"
  - "Build-tagged live integration probe (TestLiveMiruroSecurePipe) for ongoing reproducibility"
affects:
  - 28-04 (Miruro lift consumes BuildSecurePipeURL + DecodeObfuscatedResponse directly)
  - 28-CONTEXT.md D3 kill-switch (resolved to converged, no rollover to v3.2)

# Tech tracking
tech-stack:
  added: []  # stdlib-only; no new dependencies
  patterns:
    - "base64url(JSON-canonical) request encoding for upstream secure-pipe endpoints"
    - "base64url(gzip(json)) / base64url(xor-cycle(gzip(json), key)) response decoding for x-obfuscated: 1 | 2"
    - "Spike-with-kill-switch — verdict-line discriminator at top of artifact file enables downstream tooling to grep deterministically"

key-files:
  created:
    - .planning/phases/28-provider-expansion-r2/SPIKE-MIRURO.md
    - services/scraper/internal/providers/miruro/obfuscation.go
    - services/scraper/internal/providers/miruro/obfuscation_test.go
    - services/scraper/internal/providers/miruro/obfuscation_integration_test.go
    - services/scraper/internal/providers/miruro/testdata/env2.js
    - services/scraper/internal/providers/miruro/testdata/transform_vectors.json
  modified:
    - .planning/REQUIREMENTS.md
    - .planning/milestones/v3.1-REQUIREMENTS.md

key-decisions:
  - "Verdict: converged — all 4 D3 gates pass with stdlib-only primitives"
  - "TransformProxyURL keeps obfKey arg even though unused on GET path, for Wave 2 signature stability"
  - "Canonical JSON marshalling sorts query keys alphabetically (Go encoding/json default); live API accepts both orderings"
  - "POST-path JWE envelope (ECDH-ES + A256GCM) deferred — all Miruro anime API endpoints are GETable"
  - "pro.ultracloud.cc is VITE_PROXY_A — alternate route NOT what the SPA actually calls; real endpoint is /api/secure/pipe on www.miruro.tv"

patterns-established:
  - "Provider-obfuscation port: minified-bundle static analysis + 3-fetch key-stability probe + live-pipe vector capture + table-driven test"
  - "Build-tag-gated live integration test (`-tags=integration`) for upstream reproducibility without polluting unit-test runs"

requirements-completed:
  - SCRAPER-HEAL-34

# Metrics
duration: ~35 min
completed: 2026-05-20
---

# Phase 28 Plan 00: Miruro Obfuscation Spike Summary

**Reverse-engineered Miruro's "obfuscation" to a stdlib-only Go port (base64url(JSON) request + base64url(gzip(JSON)) / xor-cycle response) and shipped a converged verdict that unlocks the Miruro provider lift in Wave 2 — no `utls` / `chromedp` / third-party HTTP-fingerprint library required.**

## Performance

- **Duration:** ~35 min (well under the D3 4-agent-session kill-switch budget)
- **Started:** 2026-05-20T01:36Z
- **Completed:** 2026-05-20T01:52Z
- **Tasks:** 4/4
- **Files created:** 6
- **Files modified:** 2
- **Test count:** 8 unit-test functions (all green under `-race -count=2`) + 1 live integration test (2 subtests)

## Accomplishments

- **All 4 D3 convergence gates PASSED** with stdlib-only crypto:
  - Gate 1 (stdlib construction): GET path needs only `encoding/base64`, `encoding/json`, `compress/gzip`, `encoding/hex`, `bytes`. No HMAC/AES required (and `VITE_PROXY_OBF_KEY` is not used at all on the GET path).
  - Gate 2 (live HTTP 200): `https://www.miruro.tv/api/secure/pipe?e=<…>` returns HTTP 200 with `x-obfuscated: 1` body that gunzips to clean Frieren metadata.
  - Gate 3 (key stability): 3 env2.js fetches spaced ≥32s apart returned byte-identical bodies (SHA-256 `02233bd…980e1f` × 3).
  - Gate 4 (Frieren spot-check): 28 sub episodes returned across `dune` / `kiwi` / `hop` / `bee` providers (aggregate 112 sub eps + 84 dub eps); ep1 `sources` returns live 1080p HLS m3u8 + Kwik embed URLs.
- **Architectural correction documented:** the original plan assumed `pro.ultracloud.cc` was the target host with a URL-transform obfuscation. Reality: that's `VITE_PROXY_A`, an alternate route the SPA does NOT actually use. All real traffic goes to `www.miruro.tv/api/secure/pipe`. The plan's `TransformProxyURL` signature is preserved for API stability but its `obfKey` argument is intentionally ignored on the GET path.
- **Live-integration-test artifact** (`obfuscation_integration_test.go`) allows any maintainer to re-confirm Gate 2 + Gate 4 with a single `go test -tags=integration` invocation.

## Task Commits

Each task was committed atomically:

1. **Task 1: Probe upstream, capture key-stability evidence, identify transform shape** — `3b55d3b` (docs)
2. **Task 2: Port the transform to pure Go** — `4f5913f` (feat)
3. **Task 3: Live integration probe against production Miruro** — `2dafa19` (test)
4. **Task 4: Finalize verdict, update REQUIREMENTS.md traceability** — `e783df6` (docs)

## Files Created/Modified

- `services/scraper/internal/providers/miruro/obfuscation.go` (377 lines) — exports `TransformProxyURL`, `BuildSecurePipeURL`, `DecodeObfuscatedResponse`, `DecodePipeKey`. Stdlib imports only.
- `services/scraper/internal/providers/miruro/obfuscation_test.go` — 8 test functions covering golden vectors, negative validation, hex-key round-trip, gzip/xor-gzip/plain header paths, unknown-header rejection, gzip-bomb defense.
- `services/scraper/internal/providers/miruro/obfuscation_integration_test.go` — `-tags=integration` test exercising the live production Miruro server for Frieren info + episodes.
- `services/scraper/internal/providers/miruro/testdata/env2.js` — captured upstream `env2.js` (344 B) with both `VITE_PROXY_OBF_KEY` and `VITE_PIPE_OBF_KEY` values.
- `services/scraper/internal/providers/miruro/testdata/transform_vectors.json` — 3 (plaintext-input, base64url-output) golden vectors + 2 negative cases + discovery-summary `key_findings` block.
- `.planning/phases/28-provider-expansion-r2/SPIKE-MIRURO.md` — verdict-locked artifact with 4-gate evaluation, transform-shape characterization, downstream-effect spec, and reproducibility recipe.
- `.planning/REQUIREMENTS.md` — added Phase 28 spike pointer table with SCRAPER-HEAL-34 marked Complete.
- `.planning/milestones/v3.1-REQUIREMENTS.md` — flipped SCRAPER-HEAL-34 row from Pending → Complete (Spike — verdict: converged, 2026-05-20).

## Decisions Made

- **`obfKey` arg kept on `TransformProxyURL`** even though unused on the GET path — Wave 2 Plan 28-04 will call this from a uniform site that does both GET and POST; keeping the signature stable avoids a future no-op refactor.
- **Sorted-query-key canonical marshalling** — Go's `encoding/json` sorts map keys alphabetically by default; the live Miruro API accepts both insertion-order and sorted-order serializations (verified during the probe), so sorted-order gives us byte-identical reproducibility across Go runs without losing upstream compatibility.
- **POST-path JWE envelope (ECDH-ES + A256GCM) deferred** — Miruro's anime API surface (`info`, `episodes`, `sources`) is entirely GET-only. Implementing the POST envelope would be premature; if a future endpoint demands it, the relevant primitives (`crypto/ecdh`, `crypto/aes`, `cipher.NewGCM`) are all stdlib and can be added without breaking the current API surface.
- **4 MiB gunzip cap** — observed largest legitimate response was 1.3 MiB (`info/154587`). 4 MiB cap leaves headroom while defending against gzip-bomb DoS (T-28-00-03).
- **`testdata/env2.js` retained as evidence** — provides a baseline for future key-rotation diff checks in Plan 28-04 / canary cron.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Missing Critical] Architectural mismatch between plan's hypothesis and actual Miruro architecture**
- **Found during:** Task 1 (upstream probe)
- **Issue:** Plan 28-00-PLAN.md hypothesised the transform was `(endpoint, VITE_PROXY_OBF_KEY)` → "obfuscated URL" that `pro.ultracloud.cc` consumed directly. Live probing revealed: `pro.ultracloud.cc` is `VITE_PROXY_A` — an *alternate* host the SPA does NOT call; the real flow is `https://www.miruro.tv/api/secure/pipe?e=<base64url(JSON({path,method,query,body}))>` with NO usage of `VITE_PROXY_OBF_KEY`. The "obfuscation" is transport encoding (base64url ± gzip ± XOR-cycle) plus an optional JWE envelope on POST — not a URL transform.
- **Fix:** Implemented the actual upstream contract. `TransformProxyURL(endpoint, obfKey)` is preserved but `obfKey` is now documented as ignored on the GET path (test `TransformProxyURL_ignores_obfKey` asserts this contract). Added `BuildSecurePipeURL` + `DecodeObfuscatedResponse` as the real API surface Wave 2 will consume. Updated SPIKE-MIRURO.md to document the corrected architecture in the "TL;DR" + "Transform Shape Identified" + "Gate 2" sections.
- **Files modified:** `services/scraper/internal/providers/miruro/obfuscation.go` (new), `SPIKE-MIRURO.md` (new), `testdata/transform_vectors.json` (new).
- **Verification:** Live integration test against production server confirms the new contract returns Frieren metadata + 112-aggregate-episode listings.
- **Committed in:** `3b55d3b` (Task 1) + `4f5913f` (Task 2).

**2. [Rule 2 — Missing Critical] Build-tagged live integration test for ongoing reproducibility**
- **Found during:** Task 3
- **Issue:** Plan asked for "a one-shot Go test or `go run` driver" that exercises the live endpoint. A one-shot driver gets discarded; a build-tagged test stays in-repo and lets future maintainers re-confirm without re-reading the bash incantation.
- **Fix:** Wrote `obfuscation_integration_test.go` under `//go:build integration`. Skips cleanly on network failure (returns `t.Skipf` rather than `t.Fatalf`) so it doesn't break offline CI runners. Runs in 0.1s when the network is up.
- **Files modified:** `services/scraper/internal/providers/miruro/obfuscation_integration_test.go` (new).
- **Verification:** Test passes against production (status=200, Frieren id=154587, 112 sub eps).
- **Committed in:** `2dafa19` (Task 3).

**3. [Rule 2 — Missing Critical] Gzip-bomb defense + size cap**
- **Found during:** Task 2 implementation
- **Issue:** Plan didn't explicitly require a gunzip size cap; without one a malicious upstream (or Cloudflare cache poisoning) could send a tiny gzip that inflates to GiB of memory.
- **Fix:** Added `MaxDecodedResponseBytes = 4 << 20` (4 MiB) + `gunzipCapped` helper that returns `ErrDecodedTooLarge` on overflow. Test `TestDecodeObfuscatedResponse_SizeCap` exercises this path with a synthesised gzip bomb. T-28-00-03 threat register mitigated.
- **Files modified:** `obfuscation.go`, `obfuscation_test.go`.
- **Verification:** Test green; cap chosen above observed largest legitimate response (1.3 MiB) with comfortable headroom.
- **Committed in:** `4f5913f` (Task 2).

---

**Total deviations:** 3 auto-fixed (3 missing-critical clarifications/safety additions; 0 bugs, 0 blocking issues).
**Impact on plan:** All three deviations resolved architectural ambiguities or added defensive safety the plan implicitly required (per CLAUDE.md Don't-Hand-Roll directive + Phase 28 threat model). No scope creep — the spike's deliverable (a verdict + Go port if converged) shipped exactly as specified, with the Go port covering more realistic surface (request build + response decode + key decode) than the original signature implied.

## Issues Encountered

- **Plaintext `/api/info/154587` returned HTTP 410 Gone** — initial probe before reading the SPA's source bundle. Resolved by reading the React app's `makePlainRequest` definition, which showed the server forces clients through `/api/secure/pipe`. Plain endpoint is dead; secure-pipe endpoint is the only working path.
- **No source map for the minified bundle** (`*.js.map` returned 404). Worked around by reading the minified file with targeted Python regex extraction of the relevant code regions (`VITE_PIPE_OBF_KEY`, `makeSecureGet`, `makePlainRequest`, `x-obfuscated`).

## User Setup Required

None — spike is internal R&D. No new environment variables, no upstream account, no operator intervention.

## Next Phase Readiness

- **Plan 28-04 (Miruro lift, SCRAPER-HEAL-37) is unblocked** and may proceed in Wave 2. It will:
  1. Import `services/scraper/internal/providers/miruro` and use `BuildSecurePipeURL` + `DecodeObfuscatedResponse`.
  2. Wire 3 endpoints: `info/{anilistId}`, `episodes?anilistId={id}`, `sources?episodeId={id}&provider={p}&category=`.
  3. Use ARM (`libs/idmapping`) MAL → AniList mapping for FindID since Miruro URLs are `/anime/<anilist_id>`.
  4. Add `vault-*.uwucdn.top` to `libs/videoutils/proxy.go` HLS allowlist (and confirm `kwik.cx`/`kwik.si` are still there from animepahe).
  5. Register in `cmd/scraper-api/main.go` failover slot 5 (after AnimeFever).
  6. Frieren E2E gate using AniList 154587.
- **SCRAPER-HEAL-37 does NOT roll to v3.2.** Wave 2 of Phase 28 ships with full Miruro support.
- **No blockers.**

## Self-Check: PASSED

- [x] `.planning/phases/28-provider-expansion-r2/SPIKE-MIRURO.md` exists and contains `Verdict: converged` on line 1.
- [x] `services/scraper/internal/providers/miruro/obfuscation.go` exists, exports `TransformProxyURL`, uses stdlib only.
- [x] `services/scraper/internal/providers/miruro/obfuscation_test.go` exists, 8 functions all PASS under `-race -count=2`.
- [x] `services/scraper/internal/providers/miruro/obfuscation_integration_test.go` exists, passes against live production (status=200, Frieren found, 112 sub eps).
- [x] Commits `3b55d3b`, `4f5913f`, `2dafa19`, `e783df6` exist in `git log`.
- [x] `.planning/REQUIREMENTS.md` has SCRAPER-HEAL-34 traceability row.
- [x] `.planning/milestones/v3.1-REQUIREMENTS.md` SCRAPER-HEAL-34 row flipped to Complete.
- [x] No deletions in any commit.
- [x] No forbidden imports (`chromedp` / `utls` / `tls-client` / `flaresolverr` / `cloudscraper` / `goja`) anywhere in the new package.

---
*Phase: 28-provider-expansion-r2*
*Completed: 2026-05-20*
