---
phase: 07-pool-foundation-config-migration
plan: 02
subsystem: library
tags: [autocache, config, migration, gorm, chi, admin-api, pool-foundation]

requires:
  - phase: 07-01
    provides: "migration apply sequence (AutocachePoolSQL), library service migration/repo/handler/router patterns"
provides:
  - "migration 006 — singleton autocache_config table seeded with spec §3.5 defaults"
  - "domain.AutocacheConfig GORM model (id=1 singleton) mirroring 006 1:1"
  - "repo.AutocacheConfigRepository — typed Get/Patch accessor over the singleton row"
  - "handler.AutocacheConfigHandler — admin GET/PATCH /api/library/autocache/config behind the {success,data} envelope"
  - "master `enabled` switch persisted + readable through the typed accessor for Phases 8-10"
affects:
  - "Phase 08 downloader (reads enabled + freshness/budget windows via repo.Get)"
  - "Phase 09 triggers / Phase 10 evictor (consume the same config accessor)"

tech-stack:
  added: []
  patterns:
    - "Singleton config table (id INT PK DEFAULT 1 + CHECK(id=1)) seeded idempotently via INSERT ... ON CONFLICT DO NOTHING"
    - "Partial-update repo: Patch(map[string]any) rejects empty map, bumps updated_at via gorm.Expr(now()) in-call, re-reads full row"
    - "Pointer-field PATCH request struct — absent JSON keys stay nil so only provided fields reach the store"
    - "Interface seam (AutocacheConfigStore) so the handler tests inject a stub without Postgres"

key-files:
  created:
    - services/library/migrations/006_autocache_config.sql
    - services/library/internal/domain/autocache_config.go
    - services/library/internal/repo/autocache_config.go
    - services/library/internal/repo/autocache_config_test.go
    - services/library/internal/handler/autocache_config.go
    - services/library/internal/handler/autocache_config_test.go
  modified:
    - services/library/migrations/migrations.go
    - services/library/cmd/library-api/main.go
    - services/library/internal/transport/router.go

key-decisions:
  - "Route mounted at /api/library/autocache/config (not the spec's nominal /api/admin/library/...); gateway /api/library/* wildcard already applies JWT+AdminRoleMiddleware so NO gateway edit + NO in-service auth"
  - "Get treats a missing singleton row as Internal (not NotFound) — the row is seeded by migration 006, so absence means a broken migration"
  - "Patch always bumps updated_at in the same Updates() call via gorm.Expr(now()); empty/no-writable-key patches rejected with 400 (no empty UPDATE)"

patterns-established:
  - "Singleton CHECK(id=1) config table with idempotent ON CONFLICT seed"
  - "Pointer-body partial PATCH with per-field range validation + column-name map to the repo"

requirements-completed: [POOL-04, POOL-05]

duration: ~9 min
completed: 2026-06-17
---

# Phase 7 Plan 02: Pool Foundation — Autocache Config (Migration 006 + Admin GET/PATCH) Summary

**DB-backed, live-editable autocache config: singleton `autocache_config` table (migration 006, §3.5 defaults) with a typed Get/Patch GORM accessor and a gateway-admin-gated GET/PATCH `/api/library/autocache/config` endpoint that range-validates partial updates and persists the master `enabled` switch for Phases 8-10 — no redeploy needed.**

## Performance

- **Duration:** ~9 min
- **Tasks:** 3
- **Files created:** 6
- **Files modified:** 3

## Accomplishments

- **Migration 006** creates the singleton `autocache_config` table (id INT PK DEFAULT 1 + `CHECK (id = 1)`) with all nine tunables + `enabled` + `updated_at`, each `NOT NULL DEFAULT` mapping the spec §3.5 values (budget_bytes = 107374182400 = 100 GiB). Seeded idempotently via `INSERT ... ON CONFLICT (id) DO NOTHING`. Registered as `AutocacheConfigSQL` and applied at startup after `AutocachePoolSQL`.
- **`domain.AutocacheConfig`** mirrors 006 1:1 (snake_case `column:` tags, `json:"-"` on ID) with `TableName() == "autocache_config"`.
- **`repo.AutocacheConfigRepository`** — `Get(ctx)` loads the `id=1` row (missing → Internal), `Patch(ctx, map[string]any)` rejects an empty map, bumps `updated_at` in the same `Updates()` call, and re-reads the full updated row.
- **`handler.AutocacheConfigHandler`** — `Get` returns the `{success,data}` envelope; `Patch` decodes a pointer body (absent keys stay nil), range-validates each field, rejects empty/malformed bodies with 400, and forwards only the non-nil fields (keyed by DB column) to the store.
- **Router + DI** — `GET`/`PATCH /autocache/config` registered under the `/api/library` group behind a nil-guard; repo + handler constructed in `main.go` and passed into `transport.NewRouter`. No gateway change needed (the `/api/library/*` wildcard already enforces JWT + AdminRoleMiddleware).

## Task Commits

1. **Task 1: Migration 006 — singleton table + §3.5 defaults** — `60860a32` (feat)
2. **Task 2: AutocacheConfig model + Get/Patch repo accessor** — `4234cf26` (feat)
3. **Task 3: GET/PATCH admin handler + router wiring + main.go DI** — `8cce4ccd` (feat)

## Files Created/Modified

- `services/library/migrations/006_autocache_config.sql` — singleton config table + idempotent §3.5 seed
- `services/library/migrations/migrations.go` — `//go:embed 006...` + `AutocacheConfigSQL`; apply-order doc entry 5
- `services/library/cmd/library-api/main.go` — apply migration 006; construct repo + handler; pass handler into `NewRouter`
- `services/library/internal/domain/autocache_config.go` — `AutocacheConfig` GORM model + `TableName()`
- `services/library/internal/repo/autocache_config.go` — `NewAutocacheConfigRepository`, `Get`, `Patch`
- `services/library/internal/repo/autocache_config_test.go` — `TableName` + empty/nil-map rejection (no live DB)
- `services/library/internal/handler/autocache_config.go` — `AutocacheConfigStore` seam, `Get`/`Patch`, pointer body + range validation
- `services/library/internal/handler/autocache_config_test.go` — GET envelope + PATCH single-field/out-of-range/malformed/empty
- `services/library/internal/transport/router.go` — new handler param + `GET`/`PATCH /autocache/config` (nil-guarded)

## Decisions Made

- **Route path:** `/api/library/autocache/config`, resolving the PATTERNS.md `/api/admin/library/...` discrepancy per the plan's route decision — the gateway already proxies all `/api/library/*` behind JWT + AdminRoleMiddleware via a `/*` wildcard, so no gateway edit and no server-side auth were added.
- **Missing-row semantics:** `Get` wraps a not-found as `CodeInternal` (not `NotFound`) because migration 006 seeds the singleton — absence signals a broken migration, not a normal empty state.
- **`updated_at` bump:** always applied inside the same `Updates()` partial write via `gorm.Expr("now()")`; empty/no-writable-key patches are rejected with 400 to avoid an empty UPDATE.
- **No download/eviction behavior** was implemented — `enabled` and the freshness/budget windows are persisted + served only, for Phases 8-10 to consume.

## Deviations from Plan

None - plan executed exactly as written.

The repo test is the lightweight no-DB unit (TableName + empty-map rejection) the plan explicitly permits, since the library repo package's DB-backed tests are `//go:build integration`-gated and require a live Postgres.

## Verification

- `cd services/library && go build ./...` — clean.
- `go vet ./...` — clean (no output).
- `go test ./internal/repo/... ./internal/domain/... ./internal/handler/... -count=1` — all pass.
- All three task `<verify>` acceptance greps passed (CREATE TABLE + 107374182400 + ON CONFLICT + AutocacheConfigSQL + migrations.AutocacheConfigSQL; TableName + NewAutocacheConfigRepository + updated_at; AutocacheConfigStore + /autocache/config + NewAutocacheConfigHandler).

## Issues Encountered

- `git add services/library/cmd/library-api/main.go` prints a benign "paths are ignored" warning (the built `library-api` *binary directory* is gitignored, but `main.go` itself is tracked). The file stages and commits correctly; the warning makes `git add` exit non-zero, so staging was run as its own step rather than chained with `&&` to the commit.

## Scope Notes

Plan 07-02 only: migration 006, the `AutocacheConfig` model/repo/handler, router + main.go wiring, and `enabled` persistence/readability. The one-time admin-content migration job (07-03), serve path (Phase 8), triggers (Phase 9), evictor (Phase 10), and Grafana (Phase 11) are out of scope and were not touched. STATE.md / ROADMAP.md were NOT modified (orchestrator-owned).

## Next Phase Readiness

- POOL-04 + POOL-05 complete: config is live-editable with no redeploy; the master `enabled` switch + §3.5 windows are stored and exposed through `repo.AutocacheConfigRepository.Get` for Phases 8-10.
- Ready for 07-03 (admin-content path migration job) and the Phase-8 downloader, which consume this config accessor.

## Self-Check: PASSED

All six created files exist on disk; all three task commits (60860a32, 4234cf26, 8cce4ccd) are present in branch history.

---
*Phase: 07-pool-foundation-config-migration*
*Completed: 2026-06-17*
