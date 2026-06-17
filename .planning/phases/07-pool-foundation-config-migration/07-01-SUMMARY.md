---
phase: 07-pool-foundation-config-migration
plan: 01
subsystem: library
tags: [autocache, storage-layout, migration, gorm, pool-foundation]
requires: []
provides:
  - "autocache.RawPrefix — single-source-of-truth aeProvider/<MALID>/RAW/<ep>/ layout helper"
  - "library_episodes ledger columns (source, track, downloaded_at, last_fetch_at, fetch_count)"
  - "domain.Episode extended 1:1 with the five ledger fields + EpisodeSource/EpisodeTrack enums"
  - "migration 005 registered + applied at library startup"
affects:
  - "services/library/internal/service/encoder_worker.go (write site 1 → RawPrefix)"
  - "services/library/internal/handler/jobs.go (Link write site 2 → RawPrefix)"
tech-stack:
  added: []
  patterns:
    - "Idempotent raw-SQL migration (DO $$ EXCEPTION + ADD COLUMN IF NOT EXISTS + guarded backfill)"
    - "GORM domain ↔ SQL 1:1, no AutoMigrate (named-string enum + type:<pg_enum> tag)"
    - "Layout single-source-of-truth helper consumed by both write sites"
key-files:
  created:
    - services/library/migrations/005_autocache_pool.sql
    - services/library/internal/autocache/layout.go
    - services/library/internal/autocache/layout_test.go
    - services/library/internal/domain/episode_test.go
  modified:
    - services/library/migrations/migrations.go
    - services/library/cmd/library-api/main.go
    - services/library/internal/domain/episode.go
    - services/library/internal/service/encoder_worker.go
    - services/library/internal/handler/jobs.go
    - services/library/internal/handler/jobs_test.go
    - services/library/internal/service/encoder_worker_test.go
decisions:
  - "MALID == shikimori_id (CONTEXT line 42) — no mal_id column added; RawPrefix takes shikimoriID directly"
  - "track enum carries sub/dub (D2) but they are never written in v1 — RAW only"
  - "D6: path is uniform (aeProvider/.../RAW/...); the source column, not the path, distinguishes admin vs autocache"
metrics:
  duration: ~5 min
  completed: 2026-06-17
  tasks: 3
  files: 11
---

# Phase 7 Plan 01: Pool Foundation — Layout, Ledger & Migration 005 Summary

Established the unified `aeProvider/<MALID>/RAW/<ep>/` storage layout and the per-row
accounting ledger on `library_episodes`: migration 005 (two enums + five idempotent
columns + guarded backfill), the `autocache.RawPrefix` single-source-of-truth helper,
the GORM `Episode` model extended 1:1, and both write sites repointed so NEW admin
uploads land under `aeProvider/` directly.

## What Was Built

### Task 1 — Migration 005 (`40d699cb`)
`services/library/migrations/005_autocache_pool.sql`:
- `episode_source AS ENUM ('admin','autocache')` and `episode_track AS ENUM ('raw','sub','dub')`,
  each created inside a `DO $$ … EXCEPTION WHEN duplicate_object THEN NULL; END $$;` block.
- Five `ALTER TABLE library_episodes ADD COLUMN IF NOT EXISTS`: `source` (default `admin`),
  `track` (default `raw`), `downloaded_at TIMESTAMPTZ` (nullable so the add succeeds on a
  populated table), `last_fetch_at TIMESTAMPTZ` (nullable, written in Phase 8),
  `fetch_count BIGINT NOT NULL DEFAULT 0`. `size_bytes` was NOT re-added (already exists, POOL-03).
- Guarded one-time backfill: `UPDATE … SET source='admin', track='raw', downloaded_at=created_at
  WHERE downloaded_at IS NULL` (re-run-safe no-op).

### Task 2 — Layout helper + repoint + apply (`ac0805bd`)
- New package `internal/autocache` with `func RawPrefix(malID string, episode int) string`
  returning `aeProvider/<malID>/RAW/<episode>/` (trailing slash so `minio.Writer.Move/Upload`
  accept it unchanged). `layout_test.go` covers basic, shikimori-passthrough, and trailing-slash.
- Repointed write site 1 (`encoder_worker.go:292`) and write site 2 (`jobs.go:415` Link
  handler) to `autocache.RawPrefix(...)`. The `pending/%s/%d/` branch in encoder_worker is
  untouched.
- Registered `AutocachePoolSQL` (`//go:embed 005_autocache_pool.sql`) in `migrations.go`
  (apply-order doc extended) and applied it in `main.go` after `JackettSourceSQL`.

### Task 3 — Episode model extension (`c88f8385`)
- Added `EpisodeSource` (`admin`/`autocache`) and `EpisodeTrack` (`raw`/`sub`/`dub` reserved)
  named-string enums to `domain/episode.go`.
- Appended five fields to `Episode` mirroring migration 005 1:1: `Source`, `Track`,
  `DownloadedAt time.Time`, `LastFetchAt *time.Time` (nullable pointer), `FetchCount int64`.
  `SizeBytes *int64` left as-is. `TableName()` unchanged.
- `episode_test.go` asserts the enum string values and `TableName() == "library_episodes"`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Test] Updated two happy-path assertions to the new layout**
- **Found during:** Task 3 full-package test run.
- **Issue:** `internal/handler/jobs_test.go` (`TestJobsHandler_Link_HappyPath`) and
  `internal/service/encoder_worker_test.go` (`TestEncoder_HappyPath_WithShikimoriID`)
  asserted the OLD prefix shape (`57466/3/`, `123/1/`) — they encoded the pre-change
  behaviour that POOL-01 intentionally replaces.
- **Fix:** Updated the Move-dst / `minio_path` / Upload-prefix assertions to
  `aeProvider/57466/RAW/3/` and `aeProvider/123/RAW/1/`. The `pending/` assertions were
  left unchanged (that branch was deliberately not repointed).
- **Files modified:** `internal/handler/jobs_test.go`, `internal/service/encoder_worker_test.go`
- **Commit:** `c88f8385`

> Note: `internal/handler/episodes_test.go` and `internal/repo/episode_test.go` still carry
> `MinioPath: "12345/3/"` — these are layout-agnostic round-trip fixtures (the handler/repo
> store and echo whatever prefix they are given), not write-site assertions, so they correctly
> stay as-is.

## Verification

- `cd services/library && go build ./...` — clean.
- `go vet ./...` — clean (no output).
- `go test ./... -count=1` — all packages pass (autocache, domain, handler, service, repo,
  minio, ffmpeg, parsers, torrent, metrics).
- Acceptance greps for all three tasks passed (see plan `<verify>` blocks).

## Scope Notes

Plan 07-01 only: migration 005, the `RawPrefix` helper, the extended `Episode` model, and the
two repointed write sites. Config table (006 / 07-02), the one-time admin-content path migrator
(07-03), serve path (Phase 8), triggers (Phase 9), evictor (Phase 10), and Grafana (Phase 11)
are explicitly out of scope and were not touched. STATE.md / ROADMAP.md were not modified
(orchestrator-owned).

## Self-Check: PASSED

All created files exist on disk; all three task commits (40d699cb, ac0805bd, c88f8385) are present in branch history.
