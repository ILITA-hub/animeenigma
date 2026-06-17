---
phase: 08-serving-fetch-signal
plan: 02
subsystem: library
tags: [autocache, serve-signal, internal-endpoint, docker-network, prometheus, chi, gorm]
requires:
  - phase: 08-01
    provides: "EpisodeRepository.BumpFetch, DemandRepository.Record, DemandReasonBackfill, LibraryMetrics.IncServeTotal, migrations.AutocacheDemandSQL"
  - phase: 07-02
    provides: "AutocacheConfigRepository.Get → cfg.Enabled master switch; AutocacheConfigStore seam; NewRouter handler-threading pattern"
provides:
  - "POST /internal/library/autocache/fetch — HIT path (BumpFetch + serve_total{hit})"
  - "POST /internal/library/autocache/demand — MISS path (Record backfill + serve_total{miss}), enabled-gated, fail-closed"
  - "AutocacheInternalHandler + interface-seam deps (autocacheInternalDeps / NewGormAutocacheInternalDeps)"
  - "Docker-network-only /internal/* route group on the library mux (outside /api/library, never gateway-proxied)"
  - "migration 007 (autocache_demand) applied at library boot"
affects:
  - "08-03 (catalog ae-resolution producer: real endpoints to POST fetch/demand to)"
tech-stack:
  added: []
  patterns:
    - "Internal producer endpoint mounted top-level outside /api group (recs/notifications precedent)"
    - "Narrow interface-seam deps (autocacheInternalDeps) for no-Postgres httptest units"
    - "Master-switch side-effect gating with fail-closed config-read semantics"
    - "context.WithoutCancel on best-effort side effects so caller disconnect can't cancel mid-flight"
    - "Server-side reason override (force backfill) to defeat spoofed-reason injection"
key-files:
  created:
    - services/library/internal/handler/autocache_internal.go
    - services/library/internal/handler/autocache_internal_test.go
  modified:
    - services/library/internal/transport/router.go
    - services/library/cmd/library-api/main.go
key-decisions:
  - "Demand fails CLOSED on a config-read error (treat as disabled, skip side effects, still 200) so a config blip never floods autocache_demand"
  - "reason forced to DemandReasonBackfill server-side regardless of client input — a spoofed next_ep cannot be injected (T-08-05)"
  - "Both handlers are best-effort: a BumpFetch/Record error logs Warnw, never 500s (resolution must not regress the serve path)"
  - "Routes nil-guarded + mounted top-level (sibling of /health,/metrics); no gateway HandleFunc added — Docker-network-only by construction (T-08-04)"
patterns-established:
  - "Pattern 1: library /internal/* surface — gateway-unreachable serve-signal endpoints colocated with the library_* metric namespace + ledger writes"
  - "Pattern 2: gorm*Deps production wiring behind a test seam, reusing already-constructed main.go repos (no reconstruction)"
requirements-completed: [SERVE-01, SERVE-02, SERVE-03]
duration: ~8min
completed: 2026-06-17
---

# Phase 8 Plan 02: Serving & Fetch Signal — Library Internal Endpoints Summary

**The two Docker-network-only serve-signal endpoints (`/internal/library/autocache/{fetch,demand}`) that turn an ae HIT into a ledger bump + `serve_total{hit}` and an ae MISS into a backfill-demand row + `serve_total{miss}`, with the master `enabled` switch gating the demand side effects and migration 007 applied at boot.**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-06-17T06:36:13Z
- **Completed:** 2026-06-17
- **Tasks:** 2
- **Files modified:** 4 (2 created, 2 modified)

## What Was Built

### Task 1 — AutocacheInternalHandler (Fetch + Demand) + httptest unit (`5afe6ce8`)
`services/library/internal/handler/autocache_internal.go`:
- `autocacheInternalDeps` interface seam (`BumpFetch`, `RecordDemand`, `ConfigEnabled`) mirroring recs' `hintDeps` + library's `AutocacheConfigStore`, so the unit injects stubs without Postgres. Production wiring `NewGormAutocacheInternalDeps(episodes, demand, configRepo)` adapts the existing Phase-7/8 repos (`EpisodeRepository.BumpFetch`, `DemandRepository.Record`, `AutocacheConfigRepository.Get → cfg.Enabled`).
- Shared body `{mal_id, episode, reason}` decoded via `json.NewDecoder`; validates `mal_id != "" && episode > 0` else `httputil.BadRequest` (400, no side effects). `reason` on the wire is ignored — Demand always records `domain.DemandReasonBackfill`.
- `Fetch` (HIT): `BumpFetch(context.WithoutCancel(ctx), …)` best-effort (Warnw, no 500) → `IncServeTotal("hit")` → `httputil.OK({ok:true})`.
- `Demand` (MISS): reads `ConfigEnabled` first. On error → fail closed (Warnw, skip both side effects, still 200). When disabled → skip both, still 200. When enabled → `RecordDemand(backfill)` best-effort + `IncServeTotal("miss")` → 200.
- Unit (`autocache_internal_test.go`, httptest, fresh `prometheus.NewRegistry()` per case): fetch bumps+counts hit; fetch tolerates bump error; demand-enabled records backfill (overriding a spoofed `next_ep`) + counts miss; demand-disabled skips both + still 200; config-error fails closed; malformed/empty/zero/negative-episode bodies → 400 with no side effects on both endpoints.

### Task 2 — Mount /internal/* + main.go DI (apply 007) (`bce08093`)
- `internal/transport/router.go`: added nil-guarded `autocacheInternalHandler *handler.AutocacheInternalHandler` param to `NewRouter`; mounted `r.Post("/internal/library/autocache/{fetch,demand}", …)` at the TOP LEVEL (siblings of `/health` + `/metrics`, NOT inside the `/api/library` group), with a verbatim recs/notifications comment explaining the gateway does NOT proxy `/internal/*`.
- `cmd/library-api/main.go`: applies `migrations.AutocacheDemandSQL` (007) right after the 006 apply (same Fatalw-on-error handling); constructs `demandRepo := repo.NewDemandRepository(db.DB)` + `autocacheInternalHandler := handler.NewAutocacheInternalHandler(NewGormAutocacheInternalDeps(episodeRepo, demandRepo, autocacheConfigRepo), libMetrics, log)` reusing existing deps (no reconstruction); threads the handler into `transport.NewRouter`. No router test exists, so no other call site needed updating.

## Deviations from Plan

None — plan executed exactly as written. No bugs, missing functionality, blocking issues, or architectural changes encountered. No package installs (T-08-SC N/A — stdlib + existing chi/gorm/prometheus).

All threat-register `mitigate` dispositions were satisfied by the planned design:
- **T-08-04** (EoP on unauthenticated /internal/*): routes mounted outside `/api/library`; no gateway HandleFunc added (grep `services/gateway/` for `/internal/library` → none) — unreachable from the gateway by construction.
- **T-08-06** (master-switch bypass): demand Record + miss-count are both gated on `ConfigEnabled`; a config-read error fails closed (skips side effects).
- **T-08-05** (spoofed reason): `reason` forced to `DemandReasonBackfill` server-side; unit test asserts a client `next_ep` is overridden.

## Verification

- `cd services/library && go build ./...` — clean (BUILD-OK).
- `go vet ./...` — clean (VET-OK, no output).
- `go test ./internal/handler/... ./internal/transport/... -count=1` — handler pass; transport has no test files.
- Acceptance greps for both tasks passed: `Fetch`/`Demand` methods, `DemandReasonBackfill`, `IncServeTotal`, `/internal/library/autocache/{fetch,demand}`, `migrations.AutocacheDemandSQL`, `NewDemandRepository`, `NewAutocacheInternalHandler`.
- Confirmed NO `/internal/library` reference in `services/gateway/` (Docker-network-only by construction).

## Scope Notes

Plan 08-02 only: the two `/internal/library/autocache/{fetch,demand}` handlers + interface seam + httptest unit, the `/internal/*` route mount, and the main.go apply/DI (migration 007 + DemandRepository + handler). The catalog `RecordFetch`/`RecordDemand` client + raw_resolver fire-points (Plan 03), the Phase-9 Planner, the Phase-10 evictor, and the Phase-11 Grafana panels are explicitly out of scope and were not touched. STATE.md / ROADMAP.md were not modified (orchestrator-owned).

## Self-Check: PASSED

- `services/library/internal/handler/autocache_internal.go` — FOUND
- `services/library/internal/handler/autocache_internal_test.go` — FOUND
- `services/library/internal/transport/router.go` (modified) — FOUND
- `services/library/cmd/library-api/main.go` (modified) — FOUND
- Commit `5afe6ce8` (Task 1) — present in branch history
- Commit `bce08093` (Task 2) — present in branch history
