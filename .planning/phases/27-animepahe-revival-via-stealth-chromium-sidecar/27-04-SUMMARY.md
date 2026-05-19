---
phase: 27-animepahe-revival-via-stealth-chromium-sidecar
plan: 04
subsystem: scraper
tags: [animepahe, kwik, scraper-heal-32, verification, gate-clear, deviation]

# Dependency graph
requires:
  - phase: 27
    provides: stealth-Chromium sidecar (27-01), parser rewrite to sidecar transport (27-02), compose wiring + cgroup gate (27-03)
provides:
  - End-to-end animepahe gate cleared for Frieren (MAL 52991) through gateway
  - Documented FAIL → PASS verdict for animepahe in scraper-provider-verification-2026-05-19.md
  - Two Rule 1 bug fixes that 27-02 missed: session-pattern validation + dual-packer kwik support
affects: [27-05, 28]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Session-pattern validation gate before trusting malsync.moe identifiers"
    - "Multi-packer scan for Dean-Edwards-packed eval blocks (forward-compatible with cookie-helper prefix)"

key-files:
  created: []
  modified:
    - docs/issues/scraper-provider-verification-2026-05-19.md
    - services/scraper/internal/providers/animepahe/client.go
    - services/scraper/internal/providers/animepahe/client_test.go
    - services/scraper/internal/embeds/kwik.go

key-decisions:
  - "Auto-fixed parser+kwik bugs surfaced by the gate-clear run rather than blocking Plan 27-04 on Plan 27-02 rework — Rule 1 (bug fix) applies since the bugs prevented the verification this plan exists to perform."
  - "Replaced the plan's hardcoded animepahe.pw Referer assumption with the DTO-supplied kwik.cx Referer for the m3u8 fetchability gate — the parser's kwikReferer constant is the embed-page Referer, not the m3u8 Referer; the response DTO already carries the correct kwik.cx value in headers.Referer."
  - "Override removal restores the pre-Phase-27 operating posture (animepahe SKIPPED via SCRAPER_DEGRADED_PROVIDERS=gogoanime,animepahe) — Plan 27-05 owns the permanent flip per D7."

patterns-established:
  - "Pattern: gate-clear verification plans should validate code-side assumptions live (cache shape, JS source format, response Referer chain) — pure operational plans tend to discover real upstream/code drift."
  - "Pattern: when fixing JS-unpacker drift (Kwik dual-packer), always return ALL extracted blocks and iterate — single-block extraction is fragile against upstream HTML/JS evolution."

requirements-completed:
  - SCRAPER-HEAL-32

# Metrics
duration: ~25 min
completed: 2026-05-19
---

# Phase 27 Plan 04: End-to-End Gate-Clear + Phase 24 Verdict-Log Update Summary

**Cleared the SCRAPER-HEAL-32 hard gate end-to-end for animepahe via the new sidecar — Frieren returns 28 episodes, 6 kwik servers, and a Cloudflare-200 m3u8; flipped the verdict row FAIL → PASS, surfaced two Phase 27 deviation fixes along the way.**

## Performance

- **Duration:** ~25 min
- **Started:** 2026-05-19T10:51:53Z (approx)
- **Completed:** 2026-05-19T11:15:53Z
- **Tasks:** 3 of 3 (Task 1 + Task 2 + Task 3)
- **Files modified:** 4 (docs/issues verdict log + 3 source files for Rule 1 fixes)

## Accomplishments

- **Episodes / Servers / Stream / Stream-fetchability / Health gauge** — all five gates GREEN for Frieren MAL 52991 via `prefer=animepahe` through the gateway.
- **Verdict log updated:** `docs/issues/scraper-provider-verification-2026-05-19.md` now carries a `## Post-ship verification — Phase 27 (2026-05-19)` section flipping the animepahe row FAIL → PASS, with captured response excerpts, sidecar metrics snapshot, and the Referer-correction note.
- **Two real Phase 27 bugs found and fixed** (see Deviations below): session-pattern validation in the parser's malsync hit path + dual-packer support in the kwik extractor. Both blocked the gate from passing on the as-shipped Phase 27 code.
- **Production override restored:** `docker/.env` reverted to the pre-Phase-27 state (compose default `SCRAPER_DEGRADED_PROVIDERS=gogoanime,animepahe` is again the operating posture; scraper logs show `animepahe SKIPPED` again). Plan 27-05 owns the permanent flip per D7.

## Task Commits

Each task was committed atomically (Task 1 has no commit per the plan — runtime-only `docker/.env` mutation, gitignored):

1. **Task 1: Apply `SCRAPER_DEGRADED_PROVIDERS=__none__` override + redeploy scraper** — no commit (override is a gitignored `docker/.env` mutation per plan); verified at boot that both `gogoanime` and `animepahe` registered (neither SKIPPED).
2. **Task 2: Run the Frieren curl pipeline + capture verdict for all four stages** — `c2ac7fc` (fix). Auto-fix commit covering the two Rule 1 deviations discovered while running the pipeline. Two source files + one new regression test.
3. **Task 3: Append post-ship section to the Phase 24 verdict log + restore production override state** — `99a16b2` (docs).

## Files Created/Modified

- `docs/issues/scraper-provider-verification-2026-05-19.md` — appended the Phase 27 post-ship section (158 lines added) with updated verdict matrix, response excerpts, health-gauge snapshot, sidecar metrics, deviation log reference, and override-restored note.
- `services/scraper/internal/providers/animepahe/client.go` — added `sessionPattern` / `isSessionShape` + session-shape validation in `FindID`'s malsync-hit branch; stale legacy mappings are now best-effort evicted and the request falls through to `/search` for the real UUID session.
- `services/scraper/internal/providers/animepahe/client_test.go` — updated `TestProvider_FindID_MalSyncHit` fixture to a session-shaped value; added new `TestProvider_FindID_MalSyncLegacyNumeric` regression test pinning the SCRAPER-HEAL-32 fall-through behavior.
- `services/scraper/internal/embeds/kwik.go` — added `extractAllPackers` (returns every Dean-Edwards-packed `eval(function(p,a,c,k,e,d){...})` block in document order); `Extract` now iterates packers until one yields an m3u8 match. `extractPacker` retained as a thin wrapper over `extractAllPackers()[0]` for callers that genuinely want the first.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Parser malsync identifier shape mismatch broke `/release`**

- **Found during:** Task 2 step 1 (Episodes call)
- **Issue:** `malsync.moe` still publishes the legacy *numeric* AnimePahe anime ID (e.g. `5319` for Frieren MAL 52991) under the `animepahe` site key. The Phase 27 resolver sidecar's `/release?session=...` schema enforces a UUID-shaped session (`^[A-Za-z0-9-]{16,128}$`). Plan 27-02's parser unconditionally returned the malsync value as `providerID`, so the orchestrator passed `session=5319` to the resolver, which 400'd the request, marked the provider down, and failed over to gogoanime. The episodes endpoint returned `{"episodes": [], "meta": {"tried": [...]}}` and the gate was un-passable.
- **Fix:** Added `sessionPattern = regexp.MustCompile(\`^[A-Za-z0-9-]{16,128}$\`)` and `isSessionShape(id)`. In `FindID`, after a malsync `Lookup` hit, validate against `isSessionShape` — on mismatch, best-effort `Invalidate` the stale forward+reverse cache entries and fall through to the `/search` fuzzy-match branch which yields the real UUID session.
- **Files modified:** `services/scraper/internal/providers/animepahe/client.go`, `services/scraper/internal/providers/animepahe/client_test.go`
- **New regression test:** `TestProvider_FindID_MalSyncLegacyNumeric` asserts that a `5319`-shaped malsync hit triggers exactly one `/search` call (fall-through is exercised) and resolution succeeds via the fuzzy match.
- **Commit:** `c2ac7fc`

**2. [Rule 1 - Bug] Kwik extractor only processed the first `eval()` packer block**

- **Found during:** Task 2 step 3 (Stream call) — after Task 2 Issue 1 was fixed, the resolver pipeline succeeded all the way to the kwik embed page, but `/scraper/stream` returned 502 with `kwik: no m3u8 in unpacked source`. The animepahe stream stage's `/scraper/health` row pinpointed the failure.
- **Issue:** Modern Kwik embed pages contain **two** Dean-Edwards-packed `eval(function(p,a,c,k,e,d){...})` blocks: (1) a `$cookie` / localStorage helper, (2) the actual `const source='...m3u8...'` Plyr/HLS initializer. The existing `extractPacker` returned the FIRST block via `FindStringIndex`, so the m3u8 regex scanned only the cookie helper (no URL) and failed. Confirmed by manually unpacking both blocks in Node — only Packer 1 contains the m3u8 (`https://vault-08.uwucdn.top/stream/.../uwu.m3u8`).
- **Fix:** Added `extractAllPackers(body string) []string` that walks every `packerStartRegex` hit with the same balanced-paren / brace logic as the original. Renamed the loop body, kept `extractPacker` as a thin wrapper for legacy callers. In `Extract`, iterate the returned packer slice — call `runGoja` on each, run `sourceURLRegex` against the unpacked output, and keep the first packer that yields ≥ 1 match.
- **Backward compatibility:** Single-packer pages still match on the first iteration (no behavior change). The goja-error path is preserved for the rare case where every packer fails to unpack.
- **Files modified:** `services/scraper/internal/embeds/kwik.go`
- **Tests:** Existing `services/scraper/internal/embeds` test suite passes unchanged (no test golden touched the dual-packer case; the existing fixture is single-packer and exercises the same code path).
- **Commit:** `c2ac7fc`

### Documentation correction (Plan body)

The plan's Task 2 step 4 hardcoded `REF="https://animepahe.pw/"` for the m3u8 fetchability gate, citing the parser's `kwikReferer` constant. In practice the parser's `kwikReferer` is the Referer for fetching the **kwik.cx embed page** (the parent in the chain animepahe.pw → kwik.cx → uwucdn.top); the m3u8 itself needs `Referer: https://kwik.cx/` to satisfy uwucdn.top's referrer check. The response DTO already returns the correct value in `data.stream.headers.Referer` — no DTO contract change, just a narration mismatch in the plan. Documented in the post-ship section.

## Plan Exit Criteria (all green)

- `grep -q "Post-ship verification — Phase 27" docs/issues/scraper-provider-verification-2026-05-19.md` → PASS
- `grep -q "FAIL → PASS" docs/issues/scraper-provider-verification-2026-05-19.md` → PASS
- `! grep -q "SCRAPER_DEGRADED_PROVIDERS=__none__" docker/.env` → PASS (override removed)
- `docker compose logs --tail=50 scraper | grep -q "SKIPPED.*animepahe"` → PASS (degraded posture restored)
- `/scraper/health` for animepahe: all four stages `up:true` (snapshot at gate-clear time, captured in the verdict log)

## Captured artifacts

| Artifact | Source | Used for |
|---|---|---|
| `/tmp/27-04-episodes.json` | `GET /api/anime/.../scraper/episodes?prefer=animepahe` | Episode-count gate + verdict log excerpt |
| `/tmp/27-04-servers.json`  | `GET /api/anime/.../scraper/servers?...&prefer=animepahe` | Kwik-server gate + verdict log excerpt |
| `/tmp/27-04-stream.json`   | `GET /api/anime/.../scraper/stream?...&prefer=animepahe` | M3U8-URL gate + verdict log excerpt |
| `/tmp/kwik-out.html`       | `wget https://kwik.cx/e/aeNSh4eblrse` (from scraper container) | Diagnosed Issue 2 (dual-packer) by unpacking both blocks in Node |

## Authentication gates

None — operational plan, no upstream auth required (sidecar handles DDoS-Guard challenges internally; counters were all 0 during this window).

## Deferred Issues

None. Two surfaces noted for follow-up that are NOT blockers:

- `stealth_challenge_solves_total` was 0 during the gate-clear window, meaning the sidecar never had to solve a DDoS-Guard challenge. This is fine but means the gate-clear did NOT exercise the stealth-recovery path. A follow-up smoke test would deliberately trigger a 403 to confirm the recovery loop still works (separate from SCRAPER-HEAL-32 scope).
- The `stream_segment` stage was not exercised (probe-runner owned, Plan 17 territory).

## Self-Check: PASSED

- `[ -f docs/issues/scraper-provider-verification-2026-05-19.md ]` → FOUND
- `[ -f services/scraper/internal/providers/animepahe/client.go ]` → FOUND
- `[ -f services/scraper/internal/providers/animepahe/client_test.go ]` → FOUND
- `[ -f services/scraper/internal/embeds/kwik.go ]` → FOUND
- `git log --oneline --all | grep -q c2ac7fc` → FOUND (Task 2 fix commit)
- `git log --oneline --all | grep -q 99a16b2` → FOUND (Task 3 docs commit)
- `grep -q "Post-ship verification — Phase 27" docs/issues/scraper-provider-verification-2026-05-19.md` → FOUND
- `grep -q "FAIL → PASS" docs/issues/scraper-provider-verification-2026-05-19.md` → FOUND
- `grep -q "animepahe.*PASS" docs/issues/scraper-provider-verification-2026-05-19.md` → FOUND
- `! grep -q "SCRAPER_DEGRADED_PROVIDERS=__none__" /data/animeenigma/docker/.env` → CONFIRMED (override removed)
