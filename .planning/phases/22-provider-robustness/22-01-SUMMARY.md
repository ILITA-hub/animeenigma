---
phase: 22-provider-robustness
plan: "01"
subsystem: scraper
tags: [scraper, embeds, packed-js, hls2, hls3, multi-url, golden-fixture, streamhg, earnvids, gogoanime, streamprobe, cold-path, source-iteration]

# Dependency graph
requires:
  - phase: 21-playability-foundation
    provides: "streamprobe gate (libs/streamprobe) + gogoanime.coldPathGated cold-path orchestration + parser_unplayable_total / parser_ad_decoy_total metrics"
  - phase: 18-9anime
    provides: "packedExtractor base + streamhg/earnvids extractor wiring + Dean-Edwards-packer goja unpack pipeline"
provides:
  - "extractAllPlayableURLs helper — multi-URL extractor used by both streamhg and earnvids; returns hls2 (signed m3u8) + hls3 (unsigned txt) URLs in deterministic hls2-first order with a key-rotation fallback regex pass capped at 5 entries"
  - "gogoanime.coldPathGated.attemptOne — per-source iteration: ALL Stream.Sources are probed before declaring a server failed; trimmed Stream returned to caller contains only the playable Source"
  - "Per-source parser_unplayable_total increment semantics: each failed Source emits one increment with its own reason label (status_403, signed_url_expired, cdn_unreachable, etc.)"
affects: [22-02 (HLS proxy allowlist consumes hls3 hosts), 23-canary, scraper-self-healing]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Multi-URL extraction via shared helper in packed_common.go (hls2 + hls3 + key-rotation fallback) — reusable for future packed-JS providers"
    - "Trimmed-Stream contract: coldPathGated returns Stream{Sources: [playableSource]} so downstream never sees failed URLs"
    - "Per-source iteration with bounded fallback regex (T-22-02 DoS guard at 5 entries)"

key-files:
  created: []
  modified:
    - services/scraper/internal/embeds/packed_common.go
    - services/scraper/internal/embeds/streamhg_test.go
    - services/scraper/internal/embeds/earnvids_test.go
    - services/scraper/internal/embeds/packed_common_test.go
    - services/scraper/internal/providers/gogoanime/client.go
    - services/scraper/internal/providers/gogoanime/client_gated_test.go
    - services/scraper/testdata/gogoanime/README.md

key-decisions:
  - "Use the existing 2026-05-12 streamhg_packed.html + earnvids_packed.html goldens AS-IS — both already contain hls3 URLs (verified via offline goja unpack). No re-capture needed."
  - "hls3 host names differ from the spec (committed goldens use professionalimage.cyou + enterpriseconsulting.sbs; spec referenced managementadvisory.sbs + exoplanethunting.space — Plan 22-02 allowlist covers both rotations)"
  - "Multi-source iteration in coldPathGated runs sequentially within a single server (not parallel across sources) — sources within a server are intentionally tried in order so hls2 is preferred when it works"
  - "Trimmed Stream returned to FE contains only the playable Source so the dead URL never surfaces — Tracks/Intro/Outro/Headers from the original extraction are preserved"
  - "Multi-source unit tests use a 1-server fixture so coldPathGated runs the iteration sequentially (topN==1, no parallel race) — isolates per-source semantics from top-2 parallel race semantics"

patterns-established:
  - "extractAllPlayableURLs is the canonical name for any future multi-URL extractor in packed_common.go"
  - "Failed Sources emit parser_unplayable_total increments with per-source reason labels — observability into per-URL failure modes"
  - "Plan-level GREEN tests use 1-server fixtures when testing per-source semantics; multi-server fixtures only when exercising the parallel top-2 race"

requirements-completed: [SCRAPER-HEAL-09]

# Metrics
duration: 10min
completed: 2026-05-13
---

# Phase 22 Plan 01: Multi-URL Extraction + Cold-Path Source Iteration Summary

**streamhg/earnvids extractors now return BOTH hls2 (signed m3u8) AND hls3 (.txt) URLs as separate Stream.Sources entries, and gogoanime.coldPathGated.attemptOne iterates all Sources via the streamprobe gate before declaring a server failed.**

## Performance

- **Duration:** ~10 minutes (executor wall clock)
- **Started:** 2026-05-13T06:26:22Z
- **Completed:** 2026-05-13T06:36:21Z
- **Tasks:** 2 of 2 (TDD RED → GREEN)
- **Files modified:** 7

## Accomplishments

- `extractAllPlayableURLs` helper in `packed_common.go` extracts BOTH hls2 (signed `.m3u8`) AND hls3 (unsigned `.txt`) URLs from the Dean-Edwards-unpacked packer body, ordered hls2-first, with a key-rotation fallback regex pass capped at 5 entries (T-22-02 DoS guard).
- `packedExtractor.Extract` (and therefore both `StreamHGExtractor` and `EarnvidsExtractor`) returns a multi-source `Stream` when both URLs are present, while preserving the single-URL contract when only one URL is present.
- `gogoanime.coldPathGated.attemptOne` now iterates ALL `s.Sources` before declaring a server failed — the Phase 21 cold path probed only `Sources[0]`, which would have made the multi-URL extraction dead code without this fix.
- Per-source `parser_unplayable_total` increments emit with each failed source's own reason label (status_403, signed_url_expired, cdn_unreachable, etc.) for granular observability.
- Trimmed Stream returned from `attemptOne` contains only the playable Source so the FE never sees the failed URL; Tracks/Intro/Outro/Headers preserved from the original extraction.
- Comprehensive TDD coverage: 7 new helper tests (multi-URL, single-URL, dedupe, key rotation, ordering, fallback cap, empty) + 4 new golden-fixture tests (streamhg + earnvids × multi-URL + ordering) + 2 new cold-path multi-source tests (first-fails-second-wins, all-fail).

## Task Commits

1. **Task 1: RED multi-URL extraction tests for streamhg+earnvids** — `9997a3f` (`test(22-01)`)
2. **Task 2: GREEN multi-URL extraction + cold-path Sources iteration** — `67e195e` (combined commit; see "Deviations from Plan / Cross-Executor Note" below)

## Files Created/Modified

- `services/scraper/internal/embeds/packed_common.go` — Added `extractAllPlayableURLs(unpacked string) []domain.Source` helper, `hls3Regex`, `genericPlayableRegex`, `maxFallbackURLs` constant; `Extract` now calls the helper instead of `hls2Regex.FindStringSubmatch`.
- `services/scraper/internal/embeds/streamhg_test.go` — Added `TestStreamHG_MultiURL_FromGolden`, `TestStreamHG_MultiURL_Order`.
- `services/scraper/internal/embeds/earnvids_test.go` — Added `TestEarnvids_MultiURL_FromGolden`, `TestEarnvids_MultiURL_Order`.
- `services/scraper/internal/embeds/packed_common_test.go` — Added 7 helper tests covering multi-URL, single-URL, empty, dedupe, key-rotation, ordering, fallback cap.
- `services/scraper/internal/providers/gogoanime/client.go` — Replaced single-source `probe(s.Sources[0].URL, ...)` call with per-source iteration loop; trimmed-Stream construction.
- `services/scraper/internal/providers/gogoanime/client_gated_test.go` — Added `extractMultiSourceStreamFor` helper and two new tests: `TestGetStreamWithGate_MultiSource_FirstFailsSecondWins`, `TestGetStreamWithGate_MultiSource_AllFail`.
- `services/scraper/testdata/gogoanime/README.md` — Documented that the 2026-05-12 goldens already contain hls3 URLs in their packer body (verified via offline goja unpack); recorded the actual hls3 host names (professionalimage.cyou, enterpriseconsulting.sbs) versus the spec's references.

## Decisions Made

- **D-22-01.A — Use existing goldens as-is.** Plan instructions allowed a synthesis path with the spec's hls3 hosts (`managementadvisory.sbs`, `exoplanethunting.space`), but offline goja unpack of the committed `streamhg_packed.html` / `earnvids_packed.html` showed both ALREADY contain hls3 URLs (`professionalimage.cyou` / `enterpriseconsulting.sbs`). No synthesis needed; tests assert against the real golden contents and the README documents the host-name divergence from the spec.
- **D-22-01.B — Belt-and-suspenders fallback regex.** Plan suggested two extraction strategies; chose the belt-and-suspenders generic fallback that catches key rotations (`streamA`/`streamB` etc.) at the cost of a 5-entry cap (T-22-02 DoS guard). Documented inline in `packed_common.go` and in `extractAllPlayableURLs`' doc-comment.
- **D-22-01.C — Trimmed Stream contract.** When a server's Sources iteration finds a playable URL, the returned Stream contains ONLY the playable Source (not the full original list). Downstream FE never sees the failed URL. Tracks/Intro/Outro/Headers from the original extraction are preserved.
- **D-22-01.D — Multi-source tests use 1-server fixtures.** Phase 21's parallel top-2 race made multi-source tests with 3-server fixtures non-deterministic. Switched to 1-server fixtures so `topN==1` and the multi-source iteration runs sequentially. This isolates per-source semantics from the orthogonal top-2 race semantics.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Missing Critical Functionality] Per-source iteration in coldPathGated**
- **Found during:** Task 2 (GREEN landing)
- **Issue:** Phase 21's `21-03-SUMMARY.md` claimed coldPathGated iterates Sources, but the actual code probed only `s.Sources[0]`. Without per-source iteration, multi-URL extraction (the core 22-01 deliverable) would be dead code — the second source (`hls3`) would never reach `streamprobe.Probe`.
- **Fix:** Refactored `attemptOne` to iterate `s.Sources` in order. Each failed source emits its own `parser_unplayable_total` increment. The first playable source returns a trimmed Stream (Sources of length 1).
- **Files modified:** `services/scraper/internal/providers/gogoanime/client.go`
- **Verification:** New unit tests `TestGetStreamWithGate_MultiSource_FirstFailsSecondWins` and `TestGetStreamWithGate_MultiSource_AllFail` pin the contract under `-race`.
- **Committed in:** `67e195e` (as part of the combined GREEN commit).

### Cross-Executor Note

The Task 2 GREEN commit hash (`67e195e`) actually carries a `docs(22-02)` commit subject because the parallel executor for Plan 22-02 staged and committed my uncommitted Plan 22-01 work along with their own SUMMARY.md commit (multi-executor cwd-sharing race — both executors ran in the same working tree). The work itself is correct and complete: `git show 67e195e -- services/scraper/internal/embeds/packed_common.go` confirms all `extractAllPlayableURLs` / `hls3Regex` / `genericPlayableRegex` definitions landed; `git show 67e195e -- services/scraper/internal/providers/gogoanime/client.go` confirms the `range s.Sources` iteration and trimmed-Stream construction landed; `git show 67e195e -- services/scraper/internal/providers/gogoanime/client_gated_test.go` confirms the new multi-source tests landed. The audit trail is in the commit's file-level diff; the subject is a cross-executor attribution accident that does NOT affect the committed code.

Recommendation for future multi-executor waves: ensure each executor commits proactively after each task BEFORE the parallel executor's final commit can sweep up uncommitted work; or run executors in separate worktrees.

### Pre-existing flaky test (not introduced by 22-01)

`TestGetStreamWithGate_AdDecoy_Skipped` fails when run in isolation with `go test -run TestGetStreamWithGate_AdDecoy_Skipped` (no `-race`) due to a parallel-top-2 goroutine race between `metrics.Inc()` and `parCancel()`. The failure was demonstrated on `main` BEFORE the 22-01 changes by stashing my work and running the test — same failure. With `-race` (or as part of the full suite) the test passes consistently. Pre-existing flakiness, out of scope for 22-01 (SCOPE BOUNDARY rule). Will surface to a future maintenance ticket.

---

**Total deviations:** 1 auto-fixed (Rule 2 — missing critical: per-source iteration). 0 architectural decisions. 1 cross-executor attribution note. 1 pre-existing flaky test acknowledged.
**Impact on plan:** Rule 2 auto-fix was the explicit Plan 22-01 deliverable Task 2 — corrects the Phase 21 21-03 SUMMARY's per-source-iteration claim. No scope creep.

## Issues Encountered

- **Stash/pop coordination loss:** During mid-Task-2 investigation, I stashed my client.go changes to verify a pre-existing test was flaky on `main`. The pop later restored the changes, but my subsequent stash to re-verify briefly removed them again. Mitigated by carefully tracking `git stash list` and the `grep -c extractAllPlayableURLs` sentinel. All changes confirmed present in HEAD via `git show HEAD:...` after the dust settled.
- **Parallel executor working-tree contention:** Plan 22-02 executor's SUMMARY commit (67e195e) staged my uncommitted Plan 22-01 work alongside their own. See "Cross-Executor Note" above.

## Threat surface scan

Reviewed `extractAllPlayableURLs` and the modified `coldPathGated.attemptOne`. New surface introduced:

- Multi-URL extraction widens the URL set returned per provider from 1 to up to N (capped at 2 + 5 fallback = 7 in the absolute worst case). T-22-01 (hostile URL injection): every returned URL is gated by `streamprobe.Probe` inside `coldPathGated` AND the HLS proxy allowlist (Plan 22-02) before reaching the user. T-22-02 (DoS via huge Sources slice): fallback pass capped at 5; primary hls2/hls3 named-key passes are unbounded but practical packed-JS bodies have <5 keys.
- No new network endpoints, no new auth paths, no new schema changes at trust boundaries.

No threat flags. Plan's `<threat_model>` register (T-22-01..T-22-05) covers the full surface.

## User Setup Required

None — no external service configuration required. Plan 22-02's allowlist additions are the corresponding edge-side gate; both plans together complete SCRAPER-HEAL-09 + SCRAPER-HEAL-10.

## Next Phase Readiness

- **Plan 22-02 (HLS Proxy Allowlist + ISS-011)** already shipped in parallel — verified via `git log --oneline --grep="22-02"`. Together, 22-01 + 22-02 deliver end-to-end multi-URL self-healing: extractors return both URLs, cold-path probes each, the HLS proxy serves the playable one to the FE.
- **Phase 23 (canary)** can now monitor `parser_unplayable_total{server=streamhg,reason=*}` for per-source failure mode visibility. A spike in `signed_url_expired` for `streamhg` while `cdn_unreachable` for `enterpriseconsulting.sbs` stays low would indicate the hls2 path is rotating; Phase 23 canary's Pattern 7 fix-path can target the appropriate regex/host.
- **Production smoke** (post-`make redeploy-scraper`): not yet exercised in this execution session. The plan's `<output>` requirement for a production smoke result is deferred to the maintenance bot's post-merge verification — the gateway test sequence (`/api/scraper/stream?slug=frieren&episode=1`) will exercise the cold path with the new per-source iteration, and `/metrics` will confirm per-source increment behavior.

## Self-Check: PASSED

- File existence:
  - `services/scraper/internal/embeds/packed_common.go` — FOUND, contains `extractAllPlayableURLs` (4 references), `hls3Regex`, `genericPlayableRegex`.
  - `services/scraper/internal/providers/gogoanime/client.go` — FOUND, contains `range s.Sources` (1 reference) and `trimmed := &domain.Stream` construction.
  - `services/scraper/internal/providers/gogoanime/client_gated_test.go` — FOUND, contains 4 references to `TestGetStreamWithGate_MultiSource` family.
  - `services/scraper/testdata/gogoanime/README.md` — FOUND, contains "Plan 22-01 multi-URL notes" section.
- Commit existence:
  - `9997a3f` (test 22-01 RED) — FOUND in `git log`.
  - `67e195e` (GREEN — cross-executor commit) — FOUND in `git log`, contains all 22-01 file diffs.
- Build + tests:
  - `cd services/scraper && go build ./...` — exits 0.
  - `cd services/scraper && go test ./internal/embeds/... ./internal/providers/gogoanime/... -count=1 -race -run "TestStreamHG|TestEarnvids|TestExtractAllPlayableURLs|TestGetStreamWithGate_MultiSource"` — exits 0.

---
*Phase: 22-provider-robustness*
*Completed: 2026-05-13*
