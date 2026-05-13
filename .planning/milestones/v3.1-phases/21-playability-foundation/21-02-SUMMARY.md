---
phase: 21
plan: "02"
subsystem: scraper-observability
tags: [metrics, prometheus, http-envelope, scraper, playability-gate]
requires:
  - libs/metrics (existing — extended)
  - services/scraper/internal/handler (existing — extended)
provides:
  - libs/metrics.ParserUnplayableTotal
  - libs/metrics.ParserAdDecoyTotal
  - "writeSuccess(w, data, tried, gated bool) — 4-arg signature"
  - "GET /scraper/stream meta.gated boolean (omitempty-via-truthy)"
affects:
  - .planning/phases/21-playability-foundation/21-03 (will pass gated=true from gogoanime path)
  - .planning/phases/21-playability-foundation/21-04 (will read meta.gated in EnglishPlayer.vue)
tech_stack:
  added: []
  patterns:
    - "promauto.NewCounterVec (existing pattern from Phase 17 — extended)"
    - "JSON omitempty via truthy-only key emission in map[string]any (no struct tag needed)"
    - "TDD RED→GREEN gate pair commits per task"
key_files:
  created:
    - .planning/phases/21-playability-foundation/deferred-items.md
    - .planning/phases/21-playability-foundation/21-02-SUMMARY.md
  modified:
    - libs/metrics/provider.go
    - libs/metrics/provider_test.go
    - services/scraper/internal/handler/scraper.go
    - services/scraper/internal/handler/scraper_test.go
decisions:
  - "meta.gated is OMITTED on gated=false (not 'gated':false) — keeps Phase 16 cache-hit responses byte-identical so FE diffs don't churn"
  - "libs/metrics does NOT import libs/streamprobe — reason label values match streamprobe.ReasonEnum by string identity only (keeps libs/metrics dependency-free, avoids cyclic potential)"
  - "All three writeSuccess call sites pass gated=false literally in Wave 1; Plan 21-03 wires the real bool via a new orchestrator (*Stream, gated bool, error) return signature"
metrics:
  duration_minutes: ~18
  completed_date: 2026-05-13
  tasks_completed: 2
  files_changed: 4
---

# Phase 21 Plan 02: Scraper Metrics + meta.gated Envelope Summary

Two new Prometheus counters (`parser_unplayable_total{provider,server,reason}` and `parser_ad_decoy_total{provider,server}`) declared in libs/metrics, and a `gated bool` parameter threaded through the scraper handler's `writeSuccess` so `GET /scraper/stream` responses can advertise `meta.gated:true` whenever the playability gate actually ran. Wave-1 land of SCRAPER-HEAL-06 + SCRAPER-HEAL-07; Plan 21-03 will light the counters from the gogoanime cold path and pass the real `gated` bool through.

## Changes Landed

### libs/metrics/provider.go (+ provider_test.go)

- `ParserUnplayableTotal` CounterVec — labels `{provider, server, reason}`, name `parser_unplayable_total`, Help string references libs/streamprobe.Reason as the source of truth for `reason` values.
- `ParserAdDecoyTotal` CounterVec — labels `{provider, server}`, name `parser_ad_decoy_total`, dedicated subset of ParserUnplayableTotal for the Phase 23 `ScraperAdDecoySurge` alert.
- Package doc comment updated: "Three collectors" → "Five collectors" with the two new entries.
- Three new tests:
  - `TestParserUnplayableTotal_IncrementsCorrectly` — name + labels + delta=1.0.
  - `TestParserAdDecoyTotal_IncrementsCorrectly` — name + labels + delta=1.0.
  - `TestParserUnplayableTotal_AllReasonsAccepted` — table test iterating the 7 ReasonEnum strings (`playable, ad_decoy, zero_match, status_403, signed_url_expired, cdn_unreachable, empty_response`); each `.WithLabelValues(...).Inc()` non-nil + no panic. Locks the string-identity contract.

### services/scraper/internal/handler/scraper.go (+ scraper_test.go)

- `writeSuccess(w, data, tried, gated bool)` — 4-arg signature. When `gated == true` the envelope includes `data.meta.gated:true` alongside `data.meta.tried`; when `gated == false` the key is OMITTED (not `"gated":false`) so Phase 16 cache-hit responses stay byte-identical.
- Three call sites (GetEpisodes, GetServers, GetStream) all pass `gated=false` literally in Wave 1, each with a comment explaining why. GetStream's comment explicitly flags Plan 21-03 as the wiring owner.
- Six new tests:
  - `TestGetStream_MetaGatedAbsentByDefault` — default stream success: meta.tried present, meta.gated absent.
  - `TestWriteSuccess_GatedTrueEmitsField` — direct unit test, gated=true → meta.gated:true.
  - `TestWriteSuccess_GatedFalseOmitsField` — direct unit test, gated=false → meta.gated absent.
  - `TestGetEpisodes_NoGatedField` — episodes endpoint never includes meta.gated.
  - `TestGetServers_NoGatedField` — servers endpoint never includes meta.gated.
  - `TestErrorEnvelope_NoGatedField` — error envelopes preserve meta.tried, never include meta.gated.

## Plan-Level TDD Gate Compliance

Both tasks executed RED→GREEN per the TDD execution flow:

| Task | RED commit | GREEN commit |
|------|-----------|--------------|
| 1: metrics counters | `02fe3ca` — test(21-02): add failing tests for ParserUnplayableTotal + ParserAdDecoyTotal | `61c5db4` — feat(21-02): add ParserUnplayableTotal + ParserAdDecoyTotal counters |
| 2: writeSuccess gated bool | `5c625a1` — test(21-02): add failing tests for meta.gated envelope on /scraper/stream | `443f72f` — feat(21-02): thread gated bool through writeSuccess; emit meta.gated |

Plus `5ed5341` — docs(21-02): log pre-existing scraper service test failure (deferred-items.md).

No REFACTOR commits were needed — the GREEN implementation was minimal and idiomatic.

## Verification

- `cd /data/animeenigma/libs/metrics && go test ./... -count=1` → `ok` ✓
- `cd /data/animeenigma/services/scraper && go test ./internal/handler/... -count=1` → `ok` ✓
- `cd /data/animeenigma/services/scraper && go build ./...` → clean ✓
- `grep -c "parser_unplayable_total" libs/metrics/provider.go` → 2 (declaration + help string), ≥ 1 ✓
- `grep -c "parser_ad_decoy_total" libs/metrics/provider.go` → 1, ≥ 1 ✓
- `grep -c "ParserUnplayableTotal\|ParserAdDecoyTotal" libs/metrics/provider.go` → 7, ≥ 2 ✓
- `grep -c "writeSuccess" services/scraper/internal/handler/scraper.go` → 5, ≥ 4 ✓
- `grep -c 'meta\["gated"\]' services/scraper/internal/handler/scraper.go` → 1, ≥ 1 ✓
- `grep -c "gated" services/scraper/internal/handler/scraper.go` → 12, ≥ 3 ✓

Phase 16 envelope regression check: existing handler integration tests (`TestScraperHandler_GetEpisodes_Live`, `TestScraperHandler_GetServers_Live`, `TestScraperHandler_GetStream_Live`, `TestScraperHandler_GetEpisodes_NotFound`, `TestScraperHandler_GetEpisodes_ProviderDown`, `TestScraperHandler_GetEpisodes_NoProviders`, `TestScraperHandler_GetStream_RespectsPrefer`, `TestScraperHandler_GetEpisodes_MissingMalID`) all pass — `data.meta.tried` continues to be present on every success and error response.

## Deviations from Plan

None. The plan executed exactly as written — both tasks landed RED+GREEN, both verification commands passed first try, all done-criteria greps satisfied on the first attempt.

## Deferred Issues

**TestOrchestrator_AnimePaheToGogoanimeFailover** (`services/scraper/internal/service/orchestrator_phase18_test.go:307`) fails on `main` HEAD prior to any 21-02 changes (verified via `git stash`). The failure is in `internal/service/`, not `internal/handler/` — outside Plan 21-02's allowed file scope. Logged in `.planning/phases/21-playability-foundation/deferred-items.md`. Plan 21-03 touches the same package and may incidentally repair the fixture; if not, a dedicated `fix(scraper)` task is warranted.

## Threat Surface Scan

No new threat surface introduced. The plan's threat register (`T-21-06`, `T-21-07`, `T-21-08`) is fully mitigated:

- **T-21-06 (info disclosure via reason label):** Counter is implemented with the closed 7-value enum; cardinality bounded at ~100 series.
- **T-21-07 (meta.gated misset):** Wave-1 unconditionally emits `gated=false` literally; even if Plan 21-03's wiring misfires, the FE treats undefined === false === "skip Phase 3" so the worst case is a missed loader phase, not a security issue.
- **T-21-08 (label cardinality bomb):** `server` label values are normalized embed names from the embed registry, NOT raw URLs. Documented in `provider.go` Help string + package doc comment.

No threat flags to add.

## Hand-off Notes

- **Plan 21-03 (gogoanime server-priority + gate) — change the GetStream call site** at `services/scraper/internal/handler/scraper.go` (look for the comment block `// gated=false in Wave 1: Plan 21-03 will replace this literal...`). The orchestrator's `GetStream` will need to return `(*domain.Stream, gated bool, error)` — extend the signature there and thread the bool up.
- **Plan 21-03 — increment the counters** from the gogoanime cold path. Import `libs/metrics` and call `ParserUnplayableTotal.WithLabelValues(provider, server, string(reason)).Inc()` on every gate fail; also `ParserAdDecoyTotal.WithLabelValues(provider, server).Inc()` on the ad_decoy subset. The string-identity contract is locked by `TestParserUnplayableTotal_AllReasonsAccepted`.
- **Plan 21-04 (EnglishPlayer.vue three-phase loader) — read `response.data.meta.gated`** and conditionally show Phase 3. Treat `undefined`, `null`, and `false` identically (skip Phase 3); only show Phase 3 when `meta.gated === true`. The Wave-1 default (key absent) maps to "skip Phase 3" automatically.

## Self-Check: PASSED

- FOUND: `libs/metrics/provider.go` — ParserUnplayableTotal + ParserAdDecoyTotal declarations.
- FOUND: `libs/metrics/provider_test.go` — three new tests appended.
- FOUND: `services/scraper/internal/handler/scraper.go` — writeSuccess 4-arg signature + 3 updated call sites.
- FOUND: `services/scraper/internal/handler/scraper_test.go` — six new envelope tests.
- FOUND: `.planning/phases/21-playability-foundation/deferred-items.md` — pre-existing failure log.
- FOUND commit: `02fe3ca` (RED metrics tests).
- FOUND commit: `61c5db4` (GREEN counter declarations).
- FOUND commit: `5c625a1` (RED envelope tests).
- FOUND commit: `443f72f` (GREEN writeSuccess signature).
- FOUND commit: `5ed5341` (deferred-items.md).
