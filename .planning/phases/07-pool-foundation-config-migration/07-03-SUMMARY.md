---
phase: 07-pool-foundation-config-migration
plan: 03
subsystem: library
tags: [autocache, migration, minio, gorm, pool-foundation, idempotent, boot-once]

requires:
  - phase: 07-01
    provides: "autocache.RawPrefix layout helper, minio.Writer.Move copy-then-delete, domain.Episode ledger model, migration 005 apply at boot"
  - phase: 07-02
    provides: "migration 006 apply sequence + library main.go boot/DI wiring patterns"
provides:
  - "EpisodeRepository.UpdateMinioPath — single-column minio_path repoint scoped to id"
  - "EpisodeRepository.ListAdminLegacyPath — rows still on legacy prefix (minio_path NOT LIKE 'aeProvider/%'), created_at ASC"
  - "autocache.Migrator — one-time Move→repoint admin-content migrator (idempotent, restart-safe), run once at boot after migrations 005/006, before serving"
  - "07-CATALOG-AUDIT.md — documented confirmation that catalog ae-resolver builds served URL only from per-row minio_url/minio_path"
affects:
  - "Phase 10 evictor (relies on admin rows being in the metered aeProvider/ pool — spec §10 invariant)"
  - "Phase 08 downloader (shares the aeProvider/<mal>/RAW/<ep>/ layout the migrator lands admin content into)"

tech-stack:
  added: []
  patterns:
    - "Boot-once one-time migrator (mirrors jobRepo.ResumeInterrupted*): runs on rootCtx after migrations apply, before worker pools/HTTP serve, Warnw-not-Fatalw so a partial run re-runs next boot"
    - "Copy-before-repoint safety: Move (copy-then-delete, aborts pre-delete on copy error) FIRST, repoint minio_path ONLY after copy succeeds"
    - "Idempotency via SQL legacy-prefix filter + in-code aeProvider/ guard; single-row failure logs+continues, never aborts the run"
    - "Interface seams (EpisodeStore/Mover/migratorLogger) for no-DB/no-MinIO fakes in unit tests; DB behavior in build-tagged integration test"

key-files:
  created:
    - services/library/internal/autocache/migrator.go
    - services/library/internal/autocache/migrator_test.go
    - .planning/phases/07-pool-foundation-config-migration/07-CATALOG-AUDIT.md
  modified:
    - services/library/internal/repo/episode.go
    - services/library/internal/repo/episode_test.go
    - services/library/internal/repo/episode_integration_test.go
    - services/library/cmd/library-api/main.go

key-decisions:
  - "Migrator wired after the MinIO writer is verified ready (needs both episodeRepo + writer) and before worker pools/HTTP serve — satisfies spec §10 (admin rows in the metered pool before the Phase-10 evictor)"
  - "In-code aeProvider/ skip guard in addition to the SQL filter — defends a future caller passing an unfiltered slice"
  - "Repoint-after-Move failure is NOT counted as migrated and does NOT abort; dst already exists so a re-run re-copies harmlessly (no separate delete — Move owns source removal)"
  - "Boot error uses Warnw not Fatalw — a partial/retryable migration must not crash-loop the service; it re-runs next boot and is idempotent"

patterns-established:
  - "One-time boot migrator with copy-before-repoint + per-row non-fatal failure handling"
  - "No-DB repo unit test via reflection signature check + source-literal tripwire; DB behavior in //go:build integration test"

requirements-completed: [POOL-02]

duration: ~6 min
completed: 2026-06-17
---

# Phase 7 Plan 03: One-Time Admin-Content Pool Migration Summary

**A boot-once `autocache.Migrator` that server-side-Moves every pre-existing admin episode from the legacy `{shikimori_id}/{ep}/` MinIO prefix into the unified `aeProvider/<mal>/RAW/<ep>/` pool and repoints `library_episodes.minio_path` only after the copy succeeds — idempotent (skips `aeProvider/` rows), restart-safe (Move copies before delete; single-row failures log+continue), backed by two new repo methods and a verified-clean catalog ae-resolver audit so playback never breaks.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-06-17T07:43Z
- **Completed:** 2026-06-17T07:48Z
- **Tasks:** 3
- **Files created:** 3 · **Files modified:** 4

## Accomplishments

- **`EpisodeRepository.UpdateMinioPath(ctx, id, path)`** — one-column `Updates("minio_path", path)` scoped to the row id, wrapped via `liberrors.CodeInternal`. Called by the migrator AFTER the Move so the row is repointed only once the copy already landed.
- **`EpisodeRepository.ListAdminLegacyPath(ctx)`** — returns rows whose `minio_path NOT LIKE 'aeProvider/%'` (legacy-prefix only), `created_at ASC`. This single filter is the backbone of idempotency + restart-safety: already-migrated rows are excluded, so a re-run (or a reboot mid-migration) re-lists only what is still un-repointed.
- **`autocache.Migrator`** — `Migrate(ctx) (migrated int, err error)`: for each legacy row computes `RawPrefix(ShikimoriID, EpisodeNumber)`, skips rows already on `aeProvider/`, `Move`s the objects, then repoints. A Move error leaves the row on its old path and continues (Move aborts before deleting sources → no data loss); a repoint error after a successful Move is not counted as migrated and continues (dst exists → re-run re-copies harmlessly). Never aborts the whole run; no separate delete (Move owns it). Interface seams (`EpisodeStore`/`Mover`/`migratorLogger`) keep it testable with fakes — no live MinIO/Postgres.
- **Boot wiring** — constructed in `main.go` immediately after the MinIO writer is verified ready and the `episodeRepo` exists, and called once on `rootCtx` BEFORE the encoder/worker pools and HTTP server start — i.e. after migrations 005/006 apply and before serving. Logs the migrated count at `Infow`; on error logs `Warnw` (never `Fatalw`) so a partial/retryable run re-runs next boot.
- **Catalog audit** — `07-CATALOG-AUDIT.md` documents that both `service/raw_resolver.go` and `parser/library/client.go` build the served stream URL **only** from the per-row `minio_url`/`minio_path` (decoded from the library `{success,data}` envelope), with grep evidence showing no `aeProvider/` and no `{id}/{ep}/` object-path construction. The `minio_path` repoint is transparent with zero catalog edits.

## Task Commits

1. **Task 1: EpisodeRepository.UpdateMinioPath + ListAdminLegacyPath** — `7f3d22f2` (feat)
2. **Task 2: autocache.Migrator (Move→repoint, idempotent) + boot wiring** — `a6c3418b` (feat)
3. **Task 3: catalog ae-resolver audit** — `948a5a82` (docs)

_TDD note: Tasks 1 & 2 followed RED→GREEN inline (failing test written first, then implementation) but were each committed as one squashed feat per the per-task atomic-commit contract; the test files are included in each task's commit._

## Files Created/Modified

- `services/library/internal/autocache/migrator.go` — the one-time `Migrator` + `EpisodeStore`/`Mover`/`migratorLogger` seams
- `services/library/internal/autocache/migrator_test.go` — 4 behavior cases (move-then-repoint order, skip already-migrated, leave-on-Move-error, repoint-error-not-counted) with fake store + fake mover
- `services/library/internal/repo/episode.go` — `UpdateMinioPath` + `ListAdminLegacyPath`
- `services/library/internal/repo/episode_test.go` — no-DB signature reflection test + `NOT LIKE 'aeProvider/%'` source-literal tripwire
- `services/library/internal/repo/episode_integration_test.go` — `//go:build integration` DB test: legacy filter excludes `aeProvider/` rows, repoint drops a row from the list and persists
- `services/library/cmd/library-api/main.go` — `autocache` import + boot-once `Migrate(rootCtx)` call (Warnw on error)
- `.planning/phases/07-pool-foundation-config-migration/07-CATALOG-AUDIT.md` — catalog ae-resolver CLEAN verdict + grep evidence

## Decisions Made

- **Boot placement:** migrator runs after the writer is ready and before serving — the earliest point where both `episodeRepo` and `writer` exist and still ahead of traffic, satisfying spec §10.
- **Double idempotency guard:** SQL `NOT LIKE 'aeProvider/%'` filter PLUS an in-code `strings.HasPrefix(MinioPath, "aeProvider/")` skip, so an unfiltered caller is still safe.
- **Failure semantics:** Move error → leave row, continue; repoint-after-Move error → not migrated, continue; run never aborts; `Warnw` (not `Fatalw`) at boot. All re-run-safe.
- **No separate delete:** `minio.Writer.Move` owns copy-then-delete; the migrator never issues its own delete (matches the jobs.go Link precedent).

## Deviations from Plan

None - plan executed exactly as written.

The repo package's DB-backed assertions are `//go:build integration`-gated (require live Postgres), so per the plan's explicit allowance the non-integration unit test asserts the method signatures (reflection) and the legacy-prefix LIKE literal (source tripwire), with the behavioral filter/repoint assertions placed in the integration test.

## Issues Encountered

- `git add services/library/cmd/library-api/main.go` prints a benign "paths are ignored" warning (the built `library-api` *binary directory* is gitignored; `main.go` itself is tracked and stages/commits correctly) — same as noted in 07-02. The file committed cleanly (shown as `M` in `git status`).

## Verification

- `cd services/library && go build ./...` — clean.
- `go vet ./...` — clean (no output).
- `go test ./internal/repo/... ./internal/autocache/... -count=1` — all pass.
- `go vet -tags=integration ./internal/repo/...` — integration test compiles clean.
- All three tasks' `<verify>` acceptance greps passed (UpdateMinioPath + `NOT LIKE 'aeProvider/%'`; `Migrate(ctx context.Context)` + `RawPrefix` + `Migrate(rootCtx)`; audit file cites both catalog files + no `aeProvider/` in catalog).

## Scope Notes

Plan 07-03 only: the two repo methods, the one-time `Migrator` + boot wiring, and the catalog ae-resolver audit. Serving (Phase 8), triggers (Phase 9), eviction (Phase 10), and Grafana (Phase 11) were NOT touched. STATE.md / ROADMAP.md were NOT modified (orchestrator-owned).

## Next Phase Readiness

- POOL-02 complete: pre-existing admin content is relocated into the unified metered pool at boot, idempotently and without breaking playback (copy-before-repoint; catalog serves from the per-row `minio_path`). Spec §10 invariant satisfied — admin rows land in the metered pool before any Phase-10 evictor exists.
- Phase 07 (pool foundation + config + migration) is complete across plans 01/02/03; ready for the Phase-8 downloader, which shares the `aeProvider/<mal>/RAW/<ep>/` layout and the autocache config accessor.

## Self-Check: PASSED

All three created files exist on disk; all three task commits (7f3d22f2, a6c3418b, 948a5a82) are present in branch history.

---
*Phase: 07-pool-foundation-config-migration*
*Completed: 2026-06-17*
