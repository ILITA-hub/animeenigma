---
phase: 08-serving-fetch-signal
verified: 2026-06-17T06:50:14Z
status: passed
score: 11/11 must-haves verified
overrides_applied: 0
---

# Phase 8: Serving & Fetch Signal Verification Report

**Phase Goal:** When the player resolves the "ae" provider, a present episode serves from the new pool and records the "viewed by any user" fetch signal; an absent episode fails over cleanly (no regression) and self-heals (backfill demand) for next time.
**Verified:** 2026-06-17T06:50:14Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | A `library_autocache_serve_total{result}` counter exists for hit/miss accounting (SERVE-01/03) | ✓ VERIFIED | `internal/metrics/library_metrics.go:53` field, `:148` registration `Name: "library_autocache_serve_total"`, `:305` `IncServeTotal(result)` with nil-guard, `:314` `GetServeTotalForTest` seam; only the low-cardinality `result` label |
| 2 | `/internal/library/autocache/fetch` increments serve_total{hit} (SERVE-01) | ✓ VERIFIED | `autocache_internal.go:100` `IncServeTotal("hit")` in `Fetch`; test `TestAutocacheInternalFetch_BumpsAndCountsHit` PASS |
| 3 | Catalog `GetLibraryStream` fires `RecordFetch` on HIT (SERVE-01) | ✓ VERIFIED | `raw_resolver.go:412-416` nil-guarded `go func` `RecordFetch(context.WithoutCancel(ctx), …)` before `return newLibraryStream(...)`; test `TestRawResolver_GetLibraryStream_HIT_FiresRecordFetch` PASS |
| 4 | `EpisodeRepository.BumpFetch` sets `last_fetch_at=now()`+`fetch_count++` scoped to (shikimori_id, episode), no-op-no-error on absent row (SERVE-02) | ✓ VERIFIED | `repo/episode.go:100-111` bare `Updates(map{last_fetch_at: gorm.Expr("now()"), fetch_count: gorm.Expr("fetch_count + 1")})` `Where("shikimori_id = ? AND episode_number = ?")`; no `First`/`NotFound` → zero-rows is a no-op |
| 5 | The fetch endpoint calls BumpFetch (SERVE-02) | ✓ VERIFIED | `autocache_internal.go:96` `deps.BumpFetch(context.WithoutCancel(...))`; prod seam `gormAutocacheInternalDeps.BumpFetch → episodeRepo.BumpFetch` (`:164-166`) wired in `main.go:417` |
| 6 | `/internal/.../demand` records backfill + counts serve_total{miss} (SERVE-03) | ✓ VERIFIED | `autocache_internal.go:131` `RecordDemand(ctx, …, domain.DemandReasonBackfill)`, `:135` `IncServeTotal("miss")`; test `TestAutocacheInternalDemand_EnabledRecordsBackfillAndCountsMiss` PASS |
| 7 | Catalog fires `RecordDemand(backfill)` on MISS (`resp==nil`), NOT on the empty-ShikimoriID early return (SERVE-03) | ✓ VERIFIED | `raw_resolver.go:391-403` fire inside `if resp == nil`; `:379-385` empty-ShikimoriID early return left intentionally un-instrumented (inline comment); test `TestRawResolver_GetLibraryStream_MISS_FiresRecordDemand` PASS |
| 8 | `GetLibraryStream` `(*RawStream,error)` signature + failover control flow UNCHANGED (SERVE-03 no regression) | ✓ VERIFIED | `raw_resolver.go:368` signature unchanged; both fire-points are ADD-ONLY `go func` side effects; NotFound/stream returns byte-for-byte unchanged; tests `…Library404_FallsThroughToAllAnime`, `…SignalFailureDoesNotAffectResult` PASS |
| 9 | Calls are non-blocking (`context.WithoutCancel`) + best-effort (resolution never fails on call error) | ✓ VERIFIED | `raw_resolver.go:400,414` `context.WithoutCancel(ctx)` in `go func`, error discarded `_ =`; client `postInternal` returns wrapped error, caller drops it; race-clean per SUMMARY `-race` run |
| 10 | The enabled switch gates demand recording (fail-closed but still 200) | ✓ VERIFIED | `autocache_internal.go:117-129` reads `ConfigEnabled`; config-error → Warnw + 200 skip (fail closed); disabled → 200 skip; both skip Record AND miss-count; tests `…DisabledSkipsButStill200`, `…ConfigErrorFailsClosed` PASS |
| 11 | `/internal/*` mounted OUTSIDE `/api/library` and NOT gateway-proxied | ✓ VERIFIED | `router.go:63-66` `r.Post("/internal/library/autocache/{fetch,demand}")` at top level (sibling of `/health`,`/metrics`), nil-guarded; `grep /internal/library services/gateway/` → 0 hits |

**Score:** 11/11 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `services/library/migrations/007_autocache_demand.sql` | autocache_demand table + enum (idempotent) | ✓ VERIFIED | enum via `DO $$ … EXCEPTION WHEN duplicate_object` guard; `CREATE TABLE IF NOT EXISTS` w/ `PRIMARY KEY (mal_id, episode)`; both `next_ep`+`backfill` declared |
| `services/library/internal/domain/autocache_demand.go` | AutocacheDemand model + DemandReason enum | ✓ VERIFIED | 1:1 column tags, composite primaryKey, `type:autocache_demand_reason`, explicit `TableName()` |
| `services/library/internal/repo/demand.go` | DemandRepository.Record upsert | ✓ VERIFIED | `clause.OnConflict` (mal_id,episode) DO UPDATE requested_at=now(); errors wrapped CodeInternal |
| `services/library/internal/repo/episode.go` (BumpFetch) | atomic ledger bump | ✓ VERIFIED | atomic `gorm.Expr` increment, no-op on absent row |
| `services/library/internal/metrics/library_metrics.go` | serve_total{result} + IncServeTotal | ✓ VERIFIED | full quadruple present, result-only label |
| `services/library/internal/handler/autocache_internal.go` | Fetch + Demand handlers w/ enabled-gating | ✓ VERIFIED | seam-injected deps, fail-closed demand, reason forced backfill |
| `services/library/internal/transport/router.go` | routes outside /api/library | ✓ VERIFIED | top-level mount, nil-guarded |
| `services/library/cmd/library-api/main.go` | migration 007 applied + DI | ✓ VERIFIED | `:149` `Exec(AutocacheDemandSQL)`; `:415-417` repo+handler; `:426-435` threaded into NewRouter |
| `services/catalog/internal/parser/library/client.go` | RecordFetch + RecordDemand | ✓ VERIFIED | `:245`/`:258` + `:215` `postInternal` (status-only, non-2xx→wrapped error) |
| `services/catalog/internal/service/raw_resolver.go` | fire-and-forget HIT/MISS in GetLibraryStream | ✓ VERIFIED | `:412-416` HIT, `:391-403` MISS |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| migrations.go | 007_autocache_demand.sql | `go:embed AutocacheDemandSQL` | ✓ WIRED | `migrations.go:81-82` embed + `:17` apply-order doc entry 6 |
| domain.AutocacheDemand | autocache_demand table | GORM column tags 1:1 | ✓ WIRED | `column:mal_id`/`column:episode`/`column:reason`/`column:requested_at` match SQL |
| main.go | transport.NewRouter | autocacheInternalHandler param | ✓ WIRED | constructed `:416`, passed `:431` |
| autocache_internal.go | BumpFetch/Record/IncServeTotal | interface-seam deps | ✓ WIRED | `gormAutocacheInternalDeps` adapts episodeRepo/demandRepo/configRepo |
| raw_resolver.go | library.Client.RecordFetch/RecordDemand | `go func` + context.WithoutCancel, nil-r.library | ✓ WIRED | both fire-points nil-guarded |
| client.go | /internal/library/autocache/{fetch,demand} | http POST on cfg.APIURL | ✓ WIRED | `postInternal(c.cfg.APIURL+path, …)` |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Library build | `cd services/library && go build ./...` | exit 0 | ✓ PASS |
| Library vet | `go vet ./...` | exit 0, no output | ✓ PASS |
| Library tests | `go test ./... -count=1` | all `ok` (domain/repo/metrics/handler) | ✓ PASS |
| Catalog build | `cd services/catalog && go build ./...` | exit 0 | ✓ PASS |
| Catalog vet | `go vet ./...` | exit 0, no output | ✓ PASS |
| Catalog tests | `go test ./internal/parser/library/... ./internal/service/ -count=1` | both `ok` | ✓ PASS |
| Hit counts + bumps | `TestAutocacheInternalFetch_BumpsAndCountsHit` | PASS | ✓ PASS |
| Demand gating | `TestAutocacheInternalDemand_{Disabled,ConfigErrorFailsClosed}` | PASS | ✓ PASS |
| Failover unchanged | `TestRawResolver_Library404_FallsThroughToAllAnime`, `…SignalFailureDoesNotAffectResult` | PASS | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| SERVE-01 | 08-01/02/03 | ae HIT served from pool + counted as preload hit | ✓ SATISFIED | serve_total{hit} counter + Fetch handler + catalog RecordFetch on HIT |
| SERVE-02 | 08-01/02/03 | each ae playback bumps last_fetch_at + fetch_count | ✓ SATISFIED | BumpFetch atomic update, called by Fetch handler, fired on HIT |
| SERVE-03 | 08-01/02/03 | absent → failover no-regression + preload miss + backfill demand | ✓ SATISFIED | serve_total{miss} + Demand(backfill) + control flow unchanged (failover tests pass) |

REQUIREMENTS.md tracking table still lists SERVE-01..03 as "Pending" (lines 90-92) — that is an orchestrator-owned status flip, not an implementation gap.

### Phase-Boundary Leak Checks

| Future Phase | Forbidden in P8 | Status |
|--------------|-----------------|--------|
| Phase 9 (Planner) | no Logic A/B producers, no draining of autocache_demand, `next_ep` reserved-only | ✓ CLEAN — `DemandReasonNextEp` declared+tested but never passed to Record; only `DemandReasonBackfill` written; no Planner/drain code (only comments referencing P9) |
| Phase 10 (Evictor) | no evictor implementation | ✓ CLEAN — only forward-referencing comments; no eviction logic |
| Phase 11 (Observability) | no Grafana dashboard, but serve_total counter present | ✓ CLEAN — serve_total counter present; no dashboard JSON; only a "panel" comment in metrics |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | No TBD/FIXME/XXX debt markers in any Phase 8 modified file | ℹ️ Info | None |

### Human Verification Required

None. All must-have truths are verifiable in code + unit tests (no visual/real-time/external-service behavior introduced by this phase; the new endpoints are Docker-network-only internal producers verified by httptest units and resolver tests).

### Gaps Summary

No gaps. The phase goal is achieved end-to-end across both services:
- Library side: migration 007 (idempotent, applied at boot), AutocacheDemand model + Record dedup upsert, BumpFetch atomic ledger bump (no-op on absent row), serve_total{hit|miss} counter, and the two Docker-network-only `/internal/library/autocache/{fetch,demand}` endpoints with enabled-gating (fail-closed) and reason forced to backfill.
- Catalog side: best-effort `RecordFetch`/`RecordDemand` client methods and the two nil-guarded `context.WithoutCancel` fire-points in `GetLibraryStream` (HIT before stream return, MISS inside `resp==nil`), leaving the `(*RawStream, error)` result and AllAnime-raw failover control flow byte-for-byte unchanged (SERVE-03 no regression).
- Build/vet/test all green for both services; no Phase 9/10/11 scope leaked; no debt markers.

---

_Verified: 2026-06-17T06:50:14Z_
_Verifier: Claude (gsd-verifier)_
