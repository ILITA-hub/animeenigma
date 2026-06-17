# Phase 7: Pool Foundation, Config & Migration - Context

**Gathered:** 2026-06-17
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss) — enriched from the approved design spec

<domain>
## Phase Boundary

Every first-party RAW object (admin + auto) lives under one metered layout
(`aeProvider/<MALID>/RAW/<ep>/playlist.m3u8`) with a per-row accounting ledger on
`library_episodes`, governed by live-editable DB-backed config + a master kill-switch — and
all pre-existing admin content has been migrated into that pool without interrupting playback.

**Requirements:** POOL-01, POOL-02, POOL-03, POOL-04, POOL-05.

**In scope:** layout constants, the `library_episodes` schema extension + GORM model,
`autocache_config` singleton table + admin GET/PATCH endpoint (gateway-routed, admin-gated),
master `enabled` switch plumbing, and the one-time admin-content migration
(copy → repoint `minio_path` → delete old).

**Out of scope (later phases):** the serve hit/miss path (Phase 8), download triggers
(Phase 9), the evictor itself (Phase 10), Grafana panels (Phase 11). This phase only lays the
foundation those depend on — DO NOT implement eviction or downloading logic here.
</domain>

<decisions>
## Implementation Decisions

The authoritative design is **`docs/superpowers/specs/2026-06-17-auto-torrent-population-design.md`**.
Phase-7-relevant sections: §3 (Storage Model), §3.3 (one-time migration), §3.4 (data model),
§3.5 (config table), §9 (component inventory — `Migrator`, plus the ledger that `Accountant`
will later read).

Locked decisions that bind this phase:
- **D1** Build into `services/library` — new `internal/autocache/` package. No new microservice.
- **D2** RAW only; `track` enum includes `sub`/`dub` but they are never written in v1.
- **D5** Unified pool: admin + autocache both live under `aeProvider/` and share one budget.
- **D6** `source` column distinguishes admin vs autocache (NOT the path) — both under `aeProvider/.../RAW/...`.
- **D10** Config is DB-backed and admin-editable live (no redeploy).

`<MALID>` == `shikimori_id` (same number) — no new ID mapping.

Config table `autocache_config` (singleton row) fields + defaults (spec §3.5):
`enabled`=true, `budget_bytes`=100 GiB, `auto_fresh_download_days`=10,
`auto_fresh_fetch_days`=3, `admin_fresh_days`=30, `active_watcher_days`=30,
`quality_cap`=1080, `min_seeders`=3, `sweep_interval_min`=20.

### Claude's Discretion
Migration mechanics (batch vs per-episode, idempotency/restart safety), exact admin endpoint
path under `/api/admin/library/...`, config caching/refresh strategy inside the service, and
whether `mal_id` reuses the existing `shikimori_id` column or is added — all at Claude's
discretion, guided by the spec and existing `services/library` conventions.
</decisions>

<code_context>
## Existing Code Insights

- `services/library/internal/domain/episode.go` — `Episode` GORM model + `library_episodes`
  table (`TableName()`); `MinioPath` is the bucket-relative prefix ending in `/`, with
  `"playlist.m3u8"` appended by the handler. **This is the model to extend** (POOL-03).
- `services/library/internal/domain/job.go` — `Job` / `library_jobs`, the `JobSource` /
  `JobStatus` enums. Pattern reference for adding the `source`/`track` enums + migration style.
- `services/library/internal/minio/writer.go` — `Writer` with `Upload`, `ListObjectsByPrefix`,
  and **`Move(srcPrefix, dstPrefix)`** (server-side copy-then-remove) — the exact helper the
  migration should reuse (spec §3.3).
- `services/library/internal/service/disk_guard.go` — existing physical-disk guard (stays;
  the logical budget is layered on top in Phase 10 — not here).
- `services/library/internal/handler/episodes.go` — builds public URLs from `ep.MinioPath +
  "playlist.m3u8"`; serving stays seamless if migration repoints `minio_path` per row.
- `services/library/migrations/migrations.go` — migration registration pattern.
- `services/library/internal/transport/router.go` — where the new admin config route is wired.
- `services/library/internal/config/config.go` — service config (env). DB-backed autocache
  config is separate (admin-editable), but env may seed defaults.
- `services/catalog/internal/service/raw_resolver.go` + `services/catalog/internal/parser/library/client.go`
  — the "ae" provider resolution path. **Migration task MUST audit these for hardcoded
  old-prefix (`{shikimori_id}/{ep}/`) assumptions** (spec §3.3) and update if present.

## Gateway / routing
- Per CLAUDE.md, `/api/library/*` → library:8089 (admin-only). New admin config endpoint goes
  under that family and must be gateway-routed + admin-gated.

## Migration safety (spec §10)
- The evictor (Phase 10) must NOT see unmigrated admin content on old paths or budget math is
  wrong. This phase's migration is the prerequisite — it must be idempotent and restart-safe,
  and update each row's `minio_path` ONLY after the MinIO copy succeeds (then delete old).
</code_context>

<specifics>
## Specific Ideas

- Add `source` (enum admin|autocache), `track` (enum raw|sub|dub, default raw), `downloaded_at`,
  `last_fetch_at` (nullable), `fetch_count` (default 0) to `library_episodes`. `size_bytes`
  already exists. Backfill existing rows: `source=admin`, `track=raw`,
  `downloaded_at=created_at`, `last_fetch_at=NULL`, `fetch_count=0`.
- New `aeProvider/<MALID>/RAW/<ep>/` layout constant/helper used by writers + URL builders.
- `autocache_config` singleton table + typed accessor with the §3.5 defaults; admin
  `GET/PATCH /api/admin/library/autocache/config`.
- One-time migration: for each existing admin episode, `Move("{shikimori_id}/{ep}/",
  "aeProvider/{shikimori_id}/RAW/{ep}/")`, then repoint `minio_path`, then delete old objects.
  Idempotent (skip rows already on the new prefix).
- Master `enabled` switch read by the (future) downloader/evictor; in this phase, wire the flag
  + accessor so later phases consume it. When off, no autocache side effects.
</specifics>

<deferred>
## Deferred Ideas

- Serve-path hit/miss + `last_fetch_at` bumping → Phase 8.
- Download triggers (Logic A/B/backfill) → Phase 9.
- Evictor + budget enforcement → Phase 10.
- Grafana panels + prediction job → Phase 11.
- SUB/DUB track population, AI prediction, 2160p+/upscaling → v2 (out of milestone).
</deferred>
