---
phase: 28-provider-expansion-r2
plan: 01
subsystem: scraper / provider-expansion
tags: [spike, recon, animefever, embeds, vidstream-vip, testdata]
requires:
  - .planning/phases/28-provider-expansion-r2/28-CONTEXT.md
  - .planning/phases/28-provider-expansion-r2/28-RESEARCH.md
provides:
  - .planning/phases/28-provider-expansion-r2/SPIKE-ANIMEFEVER.md  # recon verdict + extractor write-list + allowlist hosts
  - services/scraper/internal/providers/animefever/testdata/        # 7 captured fixtures for offline unit tests
affects:
  - 28-02-PLAN.md  # consumes SPIKE verdict + testdata for AnimeFever provider lift
  - 28-03-PLAN.md  # consumes SPIKE Recommended Extractors list (1 file: vidstream_vip.go)
tech-stack:
  added: []
  patterns:
    - "Live-recon + captured-fixture spike (no code; planning artifact only)"
    - "Two-step cookie-jar fetch chain (PHPSESSID propagated via curl -c/-b across 5 calls)"
    - "Plain regex extraction anchor (no Dean-Edwards-packer unwrap; no goja JS eval)"
key-files:
  created:
    - .planning/phases/28-provider-expansion-r2/SPIKE-ANIMEFEVER.md
    - services/scraper/internal/providers/animefever/testdata/search_frieren.html
    - services/scraper/internal/providers/animefever/testdata/info_frieren.html
    - services/scraper/internal/providers/animefever/testdata/watch_ep28.html
    - services/scraper/internal/providers/animefever/testdata/ajax_load_ep28.json
    - services/scraper/internal/providers/animefever/testdata/ajax_load_ep28_hserver.json
    - services/scraper/internal/providers/animefever/testdata/embed_vidstream_vip.html
    - services/scraper/internal/providers/animefever/testdata/embed_vidstream_vip_hserver.html
  modified:
    - .planning/REQUIREMENTS.md  # added v3.1 SCRAPER-HEAL-34..39 traceability sub-table
decisions:
  - "Verdict: ready — 0 existing-registry extractors + 1 new extractor (vidstream_vip.go) cover AnimeFever's full surface for Frieren ep28."
  - "Both tserver (lt=ts) and hserver (lt=hydrax) iframe to the SAME host (am.vidstream.vip). Only one extractor file required."
  - "tserver returns the JWPlayer-style inline `sources:` literal that the plain regex extracts; hserver returns a hydrax-style JS-evaluated player that is OUT OF SCOPE for Phase 28 per D4 (no speculative extractors). Plan 28-02 attempts tserver first; if extraction fails, falls through to the next provider in the chain."
  - "ctk token shape verified 32 hex chars (`21e5bf08107829bf48f33147aba9537e`); Plan 28-02 regex widened to `{32,64}` and case-insensitive `[0-9a-fA-F]` for future-proofing."
  - "Type field in the inline sources literal is misleading (`\"type\":\"mp4\"` for a `.m3u8` URL). Plan 28-02 classifies by URL suffix, not the upstream `type` field. Documented in SPIKE-ANIMEFEVER.md §7."
  - "HLS proxy allowlist hosts for Plan 28-02: am.vidstream.vip + static-cdn-ca1.mofl.pro (verified HTTP 200 for the master m3u8). Suffix-wildcard for *.mofl.pro deferred until a daily-canary failure surfaces."
metrics:
  duration_iso8601: "PT~7M"
  start: "2026-05-20T01:36:00Z"
  end: "2026-05-20T01:43:12Z"
  tasks_completed: 3
  files_created: 8
  files_modified: 1
  commits: 3
---

# Phase 28 Plan 01: AnimeFever Embed-Extractor Recon — Summary

**One-liner:** Live-recon spike against animefever.cc / Frieren ep28 confirmed RESEARCH.md predictions exact-match — single new extractor (`embeds/vidstream_vip.go`) covers both `tserver` and `hserver` for Phase 28's primary-fallback EN provider, with 2 HLS-proxy-allowlist hosts (`am.vidstream.vip`, `static-cdn-ca1.mofl.pro`) and a 32-hex ctk token shape verified.

## Outcome

Plan 28-01 (SCRAPER-HEAL-35) shipped a decision-log artifact (`SPIKE-ANIMEFEVER.md`) plus 7 captured testdata fixtures that unblock both downstream plans:

- **Plan 28-02** (AnimeFever provider lift, SCRAPER-HEAL-36) — testdata files enable offline unit tests against the full 5-stage data path (search → info → watch → ajax → embed). The recon also exposes the exact two-step PHPSESSID cookie-jar propagation chain and the ctk-token regex shape Plan 28-02's `ctkRegex` constant will use.
- **Plan 28-03** (new embed extractors, SCRAPER-HEAL-38) — unambiguous write-list of exactly ONE file: `services/scraper/internal/embeds/vidstream_vip.go` (~120 LOC, plain regex, no goja). The Plan-28-03 scope is now wholly determined.

## Recon Output

### 1. List of embed hosts AnimeFever proxies to + classification each

| Embed host | Classification | Files to write |
|------------|---------------|----------------|
| `am.vidstream.vip` (tserver, default) | `needs-new-extractor` — none of the 5 existing extractors (kwik, megacloud, streamhg, earnvids, vibeplayer) match | `services/scraper/internal/embeds/vidstream_vip.go` |
| `am.vidstream.vip` (hserver, lt=hydrax) | OUT OF SCOPE for Phase 28 per D4 — JS-evaluated hydrax player; not a speculative write | (none — fall-through to next provider) |

`static-cdn-ca1.mofl.pro` is the downstream HLS m3u8 + segment CDN, NOT an embed host. Only needs allowlist entry, no extractor.

### 2. Recommended new extractors

| # | File | Match host(s) | Approach | Test fixture | LOC est |
|---|------|---------------|----------|--------------|---------|
| 1 | `services/scraper/internal/embeds/vidstream_vip.go` | `am.vidstream.vip` + suffix `vidstream.vip` | Plain regex `sources\s*:\s*\[\s*({[^}]+})` → json.Unmarshal → validate `.m3u8` URL suffix → return HLS Source with Referer=`https://animefever.cc/` | `testdata/embed_vidstream_vip.html` | ~120 |

Template to mirror: `vibeplayer.go` (regex-only, no goja dependency). NOT `kwik.go`/`streamhg.go` (those use Dean-Edwards-packer unwrap which isn't needed here).

### 3. HLS proxy allowlist hosts to add in Plan 28-02

Add to `libs/videoutils/proxy.go::HLSProxyAllowedDomains`:

- `am.vidstream.vip` — iframe host (defensive)
- `static-cdn-ca1.mofl.pro` — verified HLS m3u8 + segment CDN (HTTP 200, content-length 40 KiB, year-long Cloudflare cache)

Deferred: suffix-wildcard `*.mofl.pro` (only if a daily-canary failure forces it).

### 4. Token shape (ctk regex for Plan 28-02)

```go
var ctkRegex = regexp.MustCompile(`var\s+ctk\s*=\s*'([0-9a-fA-F]{32,64})'`)
```

Observed value: `21e5bf08107829bf48f33147aba9537e` (32 lowercase hex). Regex widened to `{32,64}` for future-proofing.

### 5. Per-server coverage (tserver vs hserver)

- `tserver` (default): ✓ verified — carries Frieren ep28, embed page returns the plain `sources:` literal, m3u8 HTTP 200.
- `hserver` (fallback): ✓ iframe exists (status:true, hosted at `am.vidstream.vip` with `lt=hydrax`), but the embed page is a hydrax-style JS-evaluated player with no inline `sources:` literal. Out-of-scope per D4. Plan 28-02's GetStream treats hserver as fallback-only and surfaces `ErrExtractFailed` if tserver fails — the orchestrator then moves to the next provider in the failover chain.

## Verification (per `<verification>` block in plan)

- [x] 5 (in fact 7) testdata files exist with non-empty content matching expected shapes.
- [x] SPIKE-ANIMEFEVER.md exists with `Verdict: ready` line at top.
- [x] Every embed host observed is classified `existing-registry` or `needs-new-extractor`.
- [x] HLS proxy allowlist hosts enumerated for Plan 28-02's allowlist commit (`am.vidstream.vip`, `static-cdn-ca1.mofl.pro`).
- [x] `ctk` regex pattern recorded for Plan 28-02's `ctkRegex` constant.
- [x] Per-server coverage (tserver vs hserver) documented.
- [x] REQUIREMENTS.md row added for SCRAPER-HEAL-35.

## Success Criteria (per `<success_criteria>` block)

- [x] Plan 28-02 can execute against the captured fixtures offline without needing network access (all 5 plan-required fixtures present; bonus 2 hserver fixtures included for completeness).
- [x] Plan 28-03 has an unambiguous write-list — zero speculative writes per D4 (exactly 1 file: `embeds/vidstream_vip.go`).
- [x] The allowlist host list is explicit (no "TBD"); Plan 28-02's `libs/videoutils/proxy.go` diff is determined.

## Commits

| # | Hash | Type | Description |
|---|------|------|-------------|
| 1 | `0136484` | `test(28-01)` | Capture 7 testdata fixtures for AnimeFever data path (search → info → watch → ajax × 2 → embed × 2) |
| 2 | `7c15269` | `docs(28-01)` | Write SPIKE-ANIMEFEVER.md (8 sections, Verdict line at top) |
| 3 | `941272f` | `docs(28-01)` | Add v3.1 SCRAPER-HEAL-34..39 traceability sub-table to .planning/REQUIREMENTS.md |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Completeness] Captured hserver fixtures in addition to tserver**

- **Found during:** Task 1 step (b) — recon needed to confirm both servers' shapes for §6 of SPIKE-ANIMEFEVER.md, but the original plan listed only `ajax_load_ep28.json` for the tserver path.
- **Action:** Added `ajax_load_ep28_hserver.json` and `embed_vidstream_vip_hserver.html` so the spike artifact's "Per-Server Coverage" section is evidenced and Plan 28-02's hserver fallback decision is grounded.
- **Files modified:** `services/scraper/internal/providers/animefever/testdata/ajax_load_ep28_hserver.json` + `services/scraper/internal/providers/animefever/testdata/embed_vidstream_vip_hserver.html`
- **Commit:** `0136484` (same commit as Task 1)
- **Rationale:** Plan 28-02 needs to know whether hserver iframes to a different host (would force a 2nd extractor in Plan 28-03) or the same host (one extractor covers both — current finding). Without the hserver fixture, that classification would be unfounded.

**2. [Rule 2 — Critical infra] Added new v3.1 SCRAPER-HEAL traceability sub-table to v3.0 REQUIREMENTS.md**

- **Found during:** Task 3 — the plan's `done` criteria requires a row for SCRAPER-HEAL-35 in REQUIREMENTS.md, but the v3.0 REQUIREMENTS.md table only covers SCRAPER-FOUND/PAHE/UI/OBS/9ANI/KAI/CUT/NF; v3.1 SCRAPER-HEAL rows live in `.planning/milestones/v3.1-REQUIREMENTS.md`.
- **Action:** Followed the plan's "if REQUIREMENTS.md does not have a v3.1 traceability section, add one" branch and created a new "v3.1 SCRAPER-HEAL traceability (Phase 28)" sub-table with rows for SCRAPER-HEAL-34..39.
- **Files modified:** `.planning/REQUIREMENTS.md`
- **Commit:** `941272f`

No auto-fix attempts exceeded the 3-attempt limit. No Rule 4 (architectural) checkpoints reached.

## Authentication Gates

None. All recon used public, unauthenticated endpoints (animefever.cc + am.vidstream.vip + static-cdn-ca1.mofl.pro). No credentials transmitted.

## Threat Model Compliance

Per the plan's `<threat_model>`:

- **T-28-01-01 (Information Disclosure — PHPSESSID leak):** mitigated. Grep across all 7 committed testdata files: zero `PHPSESSID`, zero `Set-Cookie`, zero session-id-shaped strings.
- **T-28-01-02 (DoS — multi-MB HTML):** mitigated. All testdata files ≤ 50 KiB (well under 4 MiB cap).
- **T-28-01-03 (Tampering — HTML drift):** accepted per RESEARCH.md "Valid until: 2026-06-19" 30-day window. Recon captured 2026-05-20.
- **T-28-01-04 (Malicious Code — running upstream JS):** mitigated. Recon was 100% static curl + grep/python-regex. No headless browser, no goja, no upstream JS execution.

## Known Stubs

None. SPIKE-ANIMEFEVER.md is a complete planning artifact (no TODO sections; no "coming soon" placeholders). The fixtures are concrete byte captures, not skeletons.

## Threat Flags

None. This plan does NOT introduce new code; only planning artifacts + raw upstream fixtures. The new extractor (`embeds/vidstream_vip.go`) will be written in Plan 28-03 and its threat surface should be re-scanned in that plan's SUMMARY.

## Self-Check: PASSED

- [x] `test -s .planning/phases/28-provider-expansion-r2/SPIKE-ANIMEFEVER.md` → 12902 bytes
- [x] `grep -E '^Verdict:\s+ready' SPIKE-ANIMEFEVER.md` → matches
- [x] `grep -E 'Recommended Extractors' SPIKE-ANIMEFEVER.md` → matches §3
- [x] `grep -E 'HLS Proxy Allowlist' SPIKE-ANIMEFEVER.md` → matches §4
- [x] All 7 testdata files exist + are committed (`git log --name-only 0136484` lists all 7)
- [x] Commit hashes verified in `git log --oneline -5`: `0136484`, `7c15269`, `941272f`
- [x] `grep 'SCRAPER-HEAL-35.*Phase 28' .planning/REQUIREMENTS.md` → matches the new row
- [x] No PHPSESSID / Set-Cookie / session-id leaks in any committed testdata
