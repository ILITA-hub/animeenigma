---
phase: 07-pool-foundation-config-migration
verified: 2026-06-17T08:05:00Z
status: human_needed
score: 5/5 must-haves verified (codebase-level)
overrides_applied: 0
re_verification:
  previous_status: none
  previous_score: none
human_verification:
  - test: "Boot a library instance that has pre-existing admin episodes on the legacy {shikimori_id}/{ep}/ MinIO prefix, then play one of those episodes before and after restart."
    expected: "After boot, library_episodes.minio_path is repointed to aeProvider/<mal>/RAW/<ep>/, the objects exist at the new prefix, the old prefix is removed, and playback of the episode never returns 404 / never breaks. A second restart is a no-op (migrated=0)."
    why_human: "Requires a live MinIO + Postgres with real legacy admin rows and a real video stream pull through the catalog ae-resolver. Static analysis confirms the copy-before-repoint ordering and idempotency in code, but the no-broken-playback guarantee on real objects can only be observed at runtime."
  - test: "As an admin (through the gateway JWT + AdminRoleMiddleware), GET /api/library/autocache/config, then PATCH a single field (e.g. {\"min_seeders\": 5}) and GET again — with no service redeploy."
    expected: "GET returns 200 with the {success,data} envelope and the §3.5 defaults (budget_bytes=107374182400, etc.); PATCH returns 200 with only min_seeders changed and updated_at bumped; the change persists across the follow-up GET and a restart; a non-admin / unauthenticated caller is rejected at the gateway."
    why_human: "The gateway admin gate + JWT path and the no-redeploy persistence are runtime concerns. The handler/repo/route are statically verified, but the end-to-end admin-auth + live-persist behavior needs a running stack."
---

# Phase 7: Pool Foundation, Config & Migration Verification Report

**Phase Goal:** Every first-party RAW object (admin + auto) lives under one metered layout (`aeProvider/<MALID>/RAW/<ep>/`) with a per-row accounting ledger on `library_episodes`, governed by live-editable DB-backed config + a master kill-switch — and all pre-existing admin content has been migrated into that pool without interrupting playback.
**Verified:** 2026-06-17T08:05:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #   | Truth (POOL req) | Status | Evidence |
| --- | ---------------- | ------ | -------- |
| 1 | POOL-01: New admin RAW writes go to `aeProvider/<shikimori_id>/RAW/<ep>/` via shared `RawPrefix`; SUB/DUB enum values exist but never written | ✓ VERIFIED | `autocache/layout.go:26-28` returns `aeProvider/%s/RAW/%d/`; both write sites call it — `encoder_worker.go:295` (`autocache.RawPrefix(job.ShikimoriID, episode)`, pending branch line 297 unchanged), `jobs.go:418` (`autocache.RawPrefix(body.ShikimoriID, episodeNum)`). Grep for any non-comment write of `EpisodeTrackSub`/`EpisodeTrackDub`/`SUB/`/`DUB/` returns empty. |
| 2 | POOL-02: Pre-existing admin episodes moved to `aeProvider/...` + `minio_path` repointed; playback never breaks; idempotent + restart-safe | ✓ VERIFIED (code) / runtime → human | `autocache/migrator.go:81-124`: lists legacy rows, computes dst via `RawPrefix`, skips `aeProvider/` rows (line 94), `Move` BEFORE `UpdateMinioPath` (lines 101/112), `continue` (never abort) on either failure, no separate delete. Boot-wired `main.go:271-276` AFTER migrations (135-144) + writer-ready (255), BEFORE `srv.ListenAndServe()` (430), `Warnw` not `Fatalw`. 4 behavior tests pass. Catalog audit (07-CATALOG-AUDIT.md) confirmed accurate against real source. |
| 3 | POOL-03: `library_episodes` carries source/track/downloaded_at/last_fetch_at/fetch_count (+ size_bytes); domain mirrors 1:1; existing rows backfilled admin/raw/created_at | ✓ VERIFIED | `migrations/005_autocache_pool.sql`: 2 enums in `DO $$ … duplicate_object` blocks, exactly 5 `ADD COLUMN IF NOT EXISTS` (no size_bytes re-add), backfill `WHERE downloaded_at IS NULL`. `domain/episode.go:49-53` mirrors all 5 with matching column tags (`LastFetchAt *time.Time`, `FetchCount int64`). |
| 4 | POOL-04: Admin GET/PATCH `/api/library/autocache/config`; live-editable, no redeploy; {success,data} envelope; range-validated | ✓ VERIFIED (code) / runtime → human | `migrations/006_autocache_config.sql`: singleton table, CHECK(id=1), `budget_bytes DEFAULT 107374182400`, all 9 tunables + enabled, `ON CONFLICT DO NOTHING` seed. `handler/autocache_config.go`: pointer-body partial PATCH, per-field range validation, empty-map reject, `httputil.OK` ({success,data}). Repo `Patch` bumps updated_at + re-reads. Routes `router.go:92-93`. Wired `main.go:398-410`. |
| 5 | POOL-05: master `enabled` column + typed accessor present (downloading/eviction NOT implemented here) | ✓ VERIFIED | `enabled BOOLEAN NOT NULL DEFAULT true` in migration 006; `domain.AutocacheConfig.Enabled` (line 22); readable/writable via `repo.AutocacheConfigRepository.Get/Patch`. No downloader/evictor reaction implemented (correct for phase 7 — behavioral half deferred to Phases 8-10 per plan + spec §3.5 kill-switch framing). |

**Score:** 5/5 truths verified at the codebase level.

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/autocache/layout.go` | `RawPrefix` single-source-of-truth | ✓ VERIFIED | Exports `RawPrefix(malID, episode)`; trailing slash; D2 SUB/DUB reserved in doc only. |
| `migrations/005_autocache_pool.sql` | enums + 5 cols + backfill | ✓ VERIFIED | 5 `ADD COLUMN IF NOT EXISTS`, no size_bytes re-add, guarded backfill. |
| `internal/domain/episode.go` | Episode extended 1:1 | ✓ VERIFIED | All 5 fields + 2 enum types; TableName unchanged. |
| `migrations/006_autocache_config.sql` | singleton + §3.5 defaults | ✓ VERIFIED | budget_bytes=107374182400; CHECK(id=1); idempotent seed. |
| `internal/domain/autocache_config.go` | GORM singleton model | ✓ VERIFIED | TableName `autocache_config`; 10 fields + updated_at. |
| `internal/repo/autocache_config.go` | Get + Patch accessor | ✓ VERIFIED | Get scoped id=1; Patch rejects empty, bumps updated_at, re-reads. |
| `internal/handler/autocache_config.go` | GET/PATCH handler | ✓ VERIFIED | `AutocacheConfigStore` seam; full range validation; {success,data}. |
| `internal/autocache/migrator.go` | Move→repoint migrator | ✓ VERIFIED | Idempotent, restart-safe, non-fatal; interface seams. |
| `internal/repo/episode.go` | UpdateMinioPath + ListAdminLegacyPath | ✓ VERIFIED | `NOT LIKE 'aeProvider/%'` filter; liberrors-wrapped. |
| `07-CATALOG-AUDIT.md` | catalog clean verdict | ✓ VERIFIED | Both files CLEAN; grep claims reproduced against real source. |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | --- | --- | ------ | ------- |
| encoder_worker.go | autocache/layout.go | `autocache.RawPrefix` | ✓ WIRED | line 295 |
| jobs.go (Link) | autocache/layout.go | `autocache.RawPrefix` | ✓ WIRED | line 418 |
| main.go | migrations.go | `Exec(AutocachePoolSQL)` + `Exec(AutocacheConfigSQL)` | ✓ WIRED | lines 135, 142 (after 004) |
| router.go | handler/autocache_config.go | GET/PATCH `/autocache/config` | ✓ WIRED | lines 92-93, under `/api/library` |
| main.go | autocache/migrator.go | `NewMigrator` + `Migrate(rootCtx)` once at boot | ✓ WIRED | lines 271-276, before `ListenAndServe` (430) |
| migrator.go | minio/writer.go | `.Move(` copy-then-delete | ✓ WIRED | line 101 |
| migrator.go | autocache/layout.go | `RawPrefix` dst | ✓ WIRED | line 89 |
| catalog raw_resolver.go / library/client.go | per-row minio_url | URL built from envelope, no object-path assembly | ✓ WIRED | resolver:246/393, client:52/68; no `aeProvider/` or path-build (grep exit 1) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| library builds | `go build ./...` | exit 0 | ✓ PASS |
| static analysis clean | `go vet ./...` | exit 0 | ✓ PASS |
| full test suite | `go test ./... -count=1` | all `ok` (autocache, domain, repo, handler, …) | ✓ PASS |
| RawPrefix output | `TestRawPrefix*` | PASS (basic, passthrough, trailing slash) | ✓ PASS |
| migrator order/idempotency | `TestMigrate_*` (4) | PASS (move-then-repoint, skip-migrated, move-err-continue, repoint-err-continue) | ✓ PASS |
| config patch validation | `TestAutocacheConfig_Patch_*` | PASS (single-field, out-of-range, malformed, empty) | ✓ PASS |
| catalog still builds | `go build ./internal/service/... ./internal/parser/library/...` | exit 0 | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| POOL-01 | 07-01 | aeProvider RAW layout (SUB/DUB reserved) | ✓ SATISFIED | RawPrefix + both write sites |
| POOL-02 | 07-03 | one-time admin migration, no playback break | ✓ SATISFIED (code) — runtime → human | migrator + boot wiring + catalog audit |
| POOL-03 | 07-01 | ledger columns | ✓ SATISFIED | migration 005 + domain mirror |
| POOL-04 | 07-02 | live GET/PATCH config | ✓ SATISFIED (code) — runtime → human | migration 006 + handler/repo/route |
| POOL-05 | 07-02 | master enabled switch stored + readable | ✓ SATISFIED | enabled column + Get/Patch accessor (behavioral halt deferred to Ph 8-10 by design) |

All 5 declared plan requirement IDs are present in REQUIREMENTS.md mapped to Phase 7 (lines 13-17, 85-89). No orphaned requirements.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| — | — | none | — | No TBD/FIXME/XXX/HACK/PLACEHOLDER debt markers in any phase-7 file; no stub returns; no scope leakage (evict/hit-miss/downloader/grafana/trigger) found in autocache or config code. |

### Scope Discipline (no Phase 8-11 leakage)

Confirmed: no serve hit/miss accounting, no triggers, no evictor logic, no Grafana, no downloader behavior implemented. `last_fetch_at`/`fetch_count` columns exist (schema reservation, written in Phase 8) but no code writes them. The `enabled` switch is stored + readable only; nothing consumes it to halt behavior yet — correct boundary for Phase 7.

### Human Verification Required

1. **Live admin-content migration + playback** — boot library with legacy admin rows on real MinIO/Postgres; confirm `minio_path` repointed, objects moved, old prefix removed, playback unbroken before/after, second boot a no-op. (Copy-before-repoint + idempotency proven in code/tests; real-object no-break guarantee is runtime-only.)
2. **Live admin config GET/PATCH (no redeploy)** — through the gateway admin gate: GET returns §3.5 defaults enveloped, PATCH persists a single field with updated_at bump across restart, non-admin rejected. (Handler/repo/route statically verified; admin-auth + live-persist is runtime-only.)

### Gaps Summary

No blocking gaps. Every must-have and all 5 POOL requirements are achieved in the codebase: the unified `aeProvider/<MALID>/RAW/<ep>/` layout (RawPrefix + both write sites), the five-column accounting ledger (migration 005 + 1:1 domain mirror + backfill), the live-editable DB-backed config with §3.5 defaults and master `enabled` switch (migration 006 + repo/handler/route), and the boot-once idempotent Move→repoint admin-content migrator wired after migrations and before serving — with the catalog ae-resolver audited (and re-confirmed against real source) as building the served URL solely from per-row `minio_url`, so the repoint is transparent.

Build, vet, and the full library test suite are green. The two human-verification items are runtime confirmations of behaviors that static analysis and unit tests cannot fully exercise (real MinIO object Move with live playback; live gateway-admin-gated config persistence). The behavioral half of POOL-05 (actually halting downloads/eviction) is correctly deferred to Phases 8-10, which do not yet exist.

---

_Verified: 2026-06-17T08:05:00Z_
_Verifier: Claude (gsd-verifier)_
