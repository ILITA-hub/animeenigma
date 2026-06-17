---
phase: 08-serving-fetch-signal
plan: 01
subsystem: library
tags: [autocache, serve-path, demand-intake, fetch-signal, migration, gorm, metrics]
requires:
  - "library_episodes ledger columns (last_fetch_at, fetch_count) from Phase 07 migration 005"
provides:
  - "autocache_demand table + autocache_demand_reason enum (migration 007, idempotent + embedded)"
  - "domain.AutocacheDemand GORM model (1:1) + DemandReason enum"
  - "repo.DemandRepository.Record — ON CONFLICT (mal_id, episode) DO UPDATE dedup upsert"
  - "repo.EpisodeRepository.BumpFetch — atomic last_fetch_at/fetch_count update, no-op on absent row"
  - "library_autocache_serve_total{result} CounterVec + IncServeTotal/GetServeTotalForTest"
affects:
  - "services/library/cmd/library-api/main.go (Plan 02 — apply migration 007 + wire DemandRepository/handler)"
  - "services/library/internal/handler/ (Plan 02 — /internal/library/autocache/{fetch,demand} endpoints call these primitives)"
tech-stack:
  added: []
  patterns:
    - "Idempotent raw-SQL migration (DO $$ EXCEPTION enum guard + CREATE TABLE IF NOT EXISTS, composite PK)"
    - "GORM domain ↔ SQL 1:1, no AutoMigrate (named-string enum + type:<pg_enum> tag + composite primaryKey)"
    - "GORM clause.OnConflict dedup upsert (refresh requested_at via gorm.Expr now())"
    - "Atomic gorm.Expr increment for a no-read-modify-write counter bump"
    - "Library Prometheus CounterVec{result} quadruple (field/register/Inc-nil-guard/test-seam)"
key-files:
  created:
    - services/library/migrations/007_autocache_demand.sql
    - services/library/internal/domain/autocache_demand.go
    - services/library/internal/domain/autocache_demand_test.go
    - services/library/internal/repo/demand.go
    - services/library/internal/repo/demand_test.go
  modified:
    - services/library/migrations/migrations.go
    - services/library/internal/repo/episode.go
    - services/library/internal/repo/episode_test.go
    - services/library/internal/metrics/library_metrics.go
    - services/library/internal/metrics/library_metrics_test.go
decisions:
  - "autocache_demand_reason carries 'next_ep' AND 'backfill' but Phase 08 only writes 'backfill' — 'next_ep' reserved for Phase 09 (mirrors 005's reserved sub/dub track values)"
  - "BumpFetch is a bare scoped Updates (no First/NotFound) — absent row is a legitimate no-op, at-least-once acceptable (fetch_count is popularity, not money)"
  - "serve_total carries ONLY the low-cardinality result label (no mal_id/episode) — /metrics leaks no per-title viewing data (T-08-03 mitigation)"
  - "Repo DB-backed behavior stays integration-gated (07-02/07-03 pattern); these plans ship no-DB unit tests (enum/TableName/constructor/signature/source-tripwire)"
metrics:
  duration: ~6 min
  completed: 2026-06-17
  tasks: 3
  files: 10
---

# Phase 8 Plan 01: Serving & Fetch Signal — Library Data + Metrics Foundation Summary

Built the three library-side primitives the Phase-8 ae serve path needs: the durable
`autocache_demand` intake table (migration 007 + embed) with its 1:1 GORM model and
`DemandRepository.Record` dedup upsert, the `EpisodeRepository.BumpFetch` ledger bump
(SERVE-02), and the `library_autocache_serve_total{result}` hit/miss counter (SERVE-01/03,
the Phase-11 chart series). Plan 02 mounts the `/internal/*` endpoints that call them.

## What Was Built

### Task 1 — Migration 007 autocache_demand + embed (`489a47e5`)
`services/library/migrations/007_autocache_demand.sql`:
- `autocache_demand_reason AS ENUM ('next_ep','backfill')` created inside a
  `DO $$ … EXCEPTION WHEN duplicate_object THEN NULL; END $$;` guard (copied from 005's pattern).
  `'next_ep'` is declared but reserved for Phase 09; Phase 08 writes only `'backfill'`.
- `CREATE TABLE IF NOT EXISTS autocache_demand` (`mal_id TEXT`, `episode INT`,
  `reason autocache_demand_reason DEFAULT 'backfill'`, `requested_at TIMESTAMPTZ DEFAULT now()`)
  with `PRIMARY KEY (mal_id, episode)` as the dedup key. No seed rows; fully idempotent.
- `migrations.go`: `//go:embed 007_autocache_demand.sql` → `AutocacheDemandSQL`, apply-order
  doc extended with entry 6 (independent — no FK ordering). Apply wiring in main.go is Plan 02.

### Task 2 — AutocacheDemand model + DemandRepository.Record (`5f032ff8`)
- `domain/autocache_demand.go`: `DemandReason` named-string enum
  (`DemandReasonNextEp="next_ep"`, `DemandReasonBackfill="backfill"`) + the `AutocacheDemand`
  GORM struct mirroring the SQL 1:1 (composite `primaryKey` tags, `type:autocache_demand_reason`
  on Reason) + explicit `TableName() == "autocache_demand"`.
- `repo/demand.go`: `DemandRepository` + `NewDemandRepository` constructor +
  `Record(ctx, malID, episode, reason)` performing an
  `ON CONFLICT (mal_id, episode) DO UPDATE SET requested_at = now()` upsert via
  `clause.OnConflict` (concurrent demand collapses to one row, recency refreshed),
  errors wrapped `CodeInternal`.
- Tests (no-DB): enum string values, `TableName`, constructor non-nil, `Record` signature
  reflection, and a source tripwire asserting `clause.OnConflict` + `requested_at` refresh.

### Task 3 — BumpFetch + serve_total counter (`25c99282`)
- `repo/episode.go`: `BumpFetch(ctx, malID, episode)` — a single
  `Updates(map{last_fetch_at: gorm.Expr("now()"), fetch_count: gorm.Expr("fetch_count + 1")})`
  scoped to `(shikimori_id, episode_number)`. Atomic increment (no read-modify-write),
  no-op-no-error on absent row (no `First`/`NotFound`), errors wrapped `CodeInternal`.
- `metrics/library_metrics.go`: the `library_autocache_serve_total{result}` CounterVec added
  as the exact `cacheInvalidationTotal` quadruple — struct field `autocacheServeTotal`,
  registration in `NewLibraryMetricsWithRegisterer`, `IncServeTotal(result)` with the
  `if m == nil` nil-guard, and the `GetServeTotalForTest` seam. Only the low-cardinality
  `result` label (no mal_id/episode).
- Tests (no-DB): BumpFetch signature reflection + atomic-increment source tripwire;
  serve_total hit/miss label counts + nil-receiver guard + registration-by-name.

## Deviations from Plan

None — plan executed exactly as written. No bugs, missing functionality, blocking issues,
or architectural changes encountered. All threat-register `mitigate` dispositions (composite-PK
dedup T-08-01, low-cardinality metric label T-08-03) were satisfied by the planned design;
no package installs (T-08-SC N/A).

## Verification

- `cd services/library && go build ./...` — clean.
- `go vet ./...` — clean (no output).
- `go test ./internal/domain/... ./internal/repo/... ./internal/metrics/... -count=1` — all pass.
- Acceptance greps for all three tasks passed (see plan `<verify>` blocks).
- Migration 007 is registered as an embedded string (`AutocacheDemandSQL`) ready for Plan 02's
  main.go apply; idempotency is structural (enum guard + IF NOT EXISTS + no seed).

## Scope Notes

Plan 08-01 only: migration 007 + embed, `domain.AutocacheDemand`, `DemandRepository.Record`,
`EpisodeRepository.BumpFetch`, and the `serve_total` counter. The `/internal/library/autocache/
{fetch,demand}` handlers + router mount + main.go apply/DI (Plan 02), the catalog
`RecordFetch`/`RecordDemand` client + raw_resolver fire-points (Plan 03), the Phase-9 Planner,
the Phase-10 evictor, and the Phase-11 Grafana panels are explicitly out of scope and were not
touched. STATE.md / ROADMAP.md were not modified (orchestrator-owned).

## Self-Check: PASSED

All created files exist on disk; all three task commits (489a47e5, 5f032ff8, 25c99282) are
present in branch history (worktree-agent-a55d101f01d46864b).
