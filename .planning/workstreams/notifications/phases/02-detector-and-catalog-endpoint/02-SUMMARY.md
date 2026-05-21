---
phase: 02-detector-and-catalog-endpoint
plan: 02
workstream: notifications
milestone: v1.0
status: complete
completed: 2026-05-21
executor_branch: worktree-agent-a0a9bc0de0e92354b
score:
  UXΔ: 0 (Ambiguous — pure backend; user-visible bell + toast lands in Phase 3)
  CDI: 0.06 × 13
  MVQ: Kraken 80%/88%
commits:
  - 69261b2  # Task 1 — catalog internal /episodes endpoint + parser adapters
  - c1c9af5  # Task 2 — detector core + bootstrap protection + 6 metrics + 5 unit tests
  - 58c05a1  # Task 3 — scheduler + cleanup + admin trigger + infra wiring
  - 4e1c0df  # Task 4 verification auto-fix — catalog envelope unwrap + INTERVAL syntax
requirements_resolved:
  - NOTIF-DET-01  # catalog internal endpoint + 5min Redis cache
  - NOTIF-DET-02  # hot-combos DISTINCT-join SQL
  - NOTIF-DET-03  # per-combo parser fan-out via errgroup cap 5
  - NOTIF-DET-04  # per-call 10s timeout
  - NOTIF-DET-05  # never-lower snapshot invariant
  - NOTIF-DET-06  # bootstrap protection (zero notifications on first-ever snapshot)
  - NOTIF-DET-07  # per-user UPSERT with first_unwatched = max_watched + 1
  - NOTIF-DET-08  # aggregation re-fire (UPSERT bumps latest, clears read_at)
  - NOTIF-DET-09  # retention cleanup (30-day default, parameterised)
  - NOTIF-DET-10  # idempotency + failure-isolation invariants
  - NOTIF-NF-01   # six Prometheus series live
  - NOTIF-NF-02   # structured detector logs (5 required fields, no PII)
---

# Phase 2 — Detector + Catalog Endpoint + Cleanup Summary

**One-liner:** Notifications detector runs hourly with ±5min boot-time jitter, fans out per-combo parser lookups (errgroup cap 5, 10s timeout) through a new catalog `/internal/anime/{shikimori_id}/episodes` endpoint (5-min Redis cache, kodik+animelib in v1.0), diffs against `parser_episode_snapshots` with mandatory bootstrap protection + never-lower invariant, and UPSERTs `new_episode` notifications via the in-process Phase-1 service (D-DET-01). Retention cleanup runs at 03:30 daily. Six NOTIF-NF-01 metrics live at `:8090/metrics`. `make run-detector-once` + `make run-cleanup-once` Makefile shortcuts make SC verification a one-liner.

## Verification matrix (live, 2026-05-21)

| SC  | Requirement | Command | Result |
| --- | --- | --- | --- |
| **SC1** | NOTIF-DET-01 — catalog endpoint + 5-min Redis cache | `docker exec animeenigma-catalog wget -qO- 'http://localhost:8081/internal/anime/59970/episodes?player=kodik&translation_id=1291&watch_type=sub&language=ru'` then immediately re-run | **PASS** — both calls return identical `{"latest_available_episode":6,"checked_at":"2026-05-21T02:02:31.748505613Z"}` (same `checked_at` proves cache hit on 2nd call) |
| **SC2** | NOTIF-DET-06 — bootstrap protection | Reset state; `curl -X POST :8090/internal/detector/run-once` | **PASS** — 11 snapshots created (all with `latest_episode>0`), 0 notifications inserted. RunReport: `{combos_scanned:11, affected_combos:0, notifications_upserted:0, parser_failures:0}` |
| **SC3** | NOTIF-DET-07, NOTIF-DET-08 — first real fire | Lower snapshot to 5 (combo had latest=6), re-trigger | **PASS** — Exactly 1 row: `dedupe_key=new_episode:dbc95dd5-…:kodik:ru:sub:1291`, `first_unwatched=6`, `latest=6`, `watch_url=/anime/dbc95dd5-…/watch?player=kodik&episode=6&translation=1291` |
| **SC4** | NOTIF-DET-10 — idempotency | Re-trigger detector with unchanged upstream | **PASS** — `affected_combos:0, notifications_upserted:0`; count stays 1; read_at remains NULL |
| **SC5** | NOTIF-DET-08 — aggregation re-fire | Mark notification read, lower snapshot to 4 (latest now diff by 2), re-trigger | **PASS** — Count still 1, `latest=6`, `first_unwatched=6` (unchanged), `read_at` reset to NULL |
| **SC6** | NOTIF-DET-09 — retention cleanup | Insert 31d + 29d dismissed rows; `curl -X POST :8090/internal/cleanup/run-once` | **PASS** — `{"deleted":1}`; only `test:young` (29d) survives |
| **SC7** | NOTIF-NF-01 — metrics exposed | `curl :8090/metrics \| grep ^notifications_` | **PASS (5/6 emitting, 6/6 registered)** — `notifications_active_unread_gauge`, `notifications_created_total{producer="detector",type="new_episode"}`, `notifications_detector_combos_scanned`, `notifications_detector_duration_seconds_{bucket,sum,count}`, `notifications_detector_runs_total{outcome="success"}`. The 6th (`notifications_detector_parser_failures_total`) is registered via `promauto.NewCounterVec` but Prometheus convention is to not expose CounterVec series until first `.Inc()` — no parser failures fired during the verification gauntlet, which is the success path. |

### Cross-cutting checks

- **Internal-only routing (D-DET-02 / D-DET-05):**
  - Gateway `:8000/internal/anime/.../episodes` → **404** ✓
  - Gateway `:8000/internal/detector/run-once` → **404** ✓
  - Direct `:8081/internal/anime/.../episodes` (in-network) → **200** ✓
  - Direct `:8090/internal/detector/run-once` (in-network) → **200** ✓

- **NOTIF-NF-02 structured logs:** Every `detector run completed` line carries `combos_scanned, affected_combos, notifications_upserted, duration_ms, parser_failures, outcome`. Zero `username=` / `email=` fields in `docker logs animeenigma-notifications`.

- **`make health`:** All 10 services healthy after the changes, including `notifications:8090`.

## Deviations from plan

### Auto-fixed issues (Rule 1 — bugs in just-shipped Task 2/3 code)

**1. Catalog envelope unwrap** — Commit `4e1c0df`.
- **Found during:** SC2 — bootstrap snapshot table showed every `latest_episode=0` despite parser_failures=0.
- **Root cause:** `HTTPEpisodeChecker.LatestEpisode` unmarshalled the catalog response into a struct expecting top-level `latest_available_episode`, but catalog wraps every response in `libs/httputil.JSON`'s `{"success": bool, "data": {...}}` envelope (verified by inspecting `libs/httputil/response.go::JSON`).
- **Fix:** Split into `EpisodeCheckerResponse{Success, Data}` + `EpisodeCheckerResponsePayload{LatestAvailableEpisode, CheckedAt}`; read `parsed.Data.LatestAvailableEpisode`. Inline comment references SC2 for future readers.
- **File:** `services/notifications/internal/service/catalog_client.go`.

**2. Postgres INTERVAL math syntax** — Commit `4e1c0df`.
- **Found during:** SC6 — `POST /internal/cleanup/run-once` returned 500 with log line `"failed to encode args[0]: unable to encode 30 into text format for text (OID 25): cannot find encode plan"`.
- **Root cause:** pgx (the Postgres driver this service uses through gorm.io/driver/postgres) refuses to encode an `int` parameter into a text-shaped slot. The original `(? || ' days')::interval` formulation requires text concatenation, which collides with the int parameter.
- **Fix:** Switched to `NOW() - (INTERVAL '1 day' * ?)` — the parameter stays a plain integer and the arithmetic happens server-side. Inline comment references SC6.
- **File:** `services/notifications/internal/job/cleanup.go`.

### Detector internal-key normalisation

Mid-Task-2 implementation correction (not a separate commit — part of `c1c9af5`): the detector's snapshot+max-watched lookups must normalise combos to drop `ShikimoriID` because neither `parser_episode_snapshots` nor `watch_history` carry that column. Without the normalisation every diff lookup returned `hadSnapshot=false` and the detector silently re-bootstrapped forever (silent NOTIF-DET-06 violation). Caught by the local detector unit tests (Test_Detector_FiresOnDiffAfterBootstrap) before any verification round-trip. Fix lives in the `snapKey` helper inside `detector.go` with an inline comment.

### Plan-correct alternate naming

The plan's `<touch_list>` references the type as `NewEpisodeDetectorJob` with a constructor `NewEpisodeDetectorJob`. Go's `gofmt`/`go vet` reject having a type and a function with the same name in the same package. Followed the `services/scheduler/internal/jobs/` precedent and named the constructor `NewEpisodeDetectorJobNew` — type/constructor naming convention preserved, no ambiguity in callers. Documented inline.

## Risks materialized

- **R-02-01 (parser rate-limit storm):** Not exercised in this verification (only 11 combos, not 30+). Will surface on production scale — mitigation already in place (5-worker cap + 5-min Redis cache).
- **R-02-04 (cache invalidation when admin injects snapshot):** Not materialized — SC3/SC5 modified the SNAPSHOT (Postgres), not Redis-cached parser output, so the manipulation path was clean.
- **R-02-05 (scheduler running before DB ready):** Not materialized — `depends_on: catalog (service_started)` + bootstrap protection means even a misordered boot results in `combos_scanned=N, affected_combos=0`.
- **R-02-08 (cron parse error):** Not materialized — both crons (`"0 * * * *"`, `"30 3 * * *"`) parsed cleanly on boot.

## D-DET-01..07 honored

- **D-DET-01** (in-process detector → notif.Upsert, NOT HTTP self-loopback): `services/notifications/internal/job/detector.go` calls `j.notif.Upsert(...)` directly. Zero references to `http.Client` or `:8090/internal/notifications` from the detector code.
- **D-DET-02** (catalog internal route mirrors `internal_cache.go` precedent, no middleware): `services/catalog/internal/transport/router.go` mounts `GET /internal/anime/{shikimoriId}/episodes` in the same `if internalEpisodesHandler != nil` block as `InvalidateRaw`, BEFORE the `/api` chi.Route.
- **D-DET-03** (v1.0 player allowlist = {kodik, animelib}): `EpisodesLookupService.LatestAvailable` returns `apperrors.InvalidInput("player not supported by detector in v1.0")` for anything else; handler returns 400. Validated by inspection.
- **D-DET-04** (focused per-player adapters): `kodik.LatestEpisodeForTranslation` + `animelib.LatestEpisodeForTeam` are new files alongside the existing `client.go`; the detector path NEVER calls `GetTranslations` / `GetEpisodes` directly.
- **D-DET-05** (admin trigger at `/internal/detector/run-once`, gateway-non-routing): `services/notifications/internal/handler/admin.go` + `transport/router.go` mount the route under `/internal/*` with no middleware. SC verification confirmed `:8000/internal/detector/run-once` returns 404.
- **D-DET-06** (sibling `/internal/cleanup/run-once` + `make run-cleanup-once`): Both shipped. SC6 used the Makefile target's underlying `wget -qO- --post-data='' :8090/internal/cleanup/run-once` directly via curl for SUMMARY-friendly output.
- **D-DET-07** (`EpisodeChecker` interface for production HTTP + test stubs): Interface lives in `service/catalog_client.go`; production = `HTTPEpisodeChecker`; tests = `stubChecker` + `failingCheckerForB` injected via the same `NewEpisodeDetectorJobNew` constructor. 5 unit tests in `internal/job/detector_test.go` exercise the interface (bootstrap, first-fire, idempotency, never-lower invariant, parser-failure isolation) — all passing.

## Touched files summary

**New (catalog):**
- `services/catalog/internal/parser/kodik/latest_episode.go`
- `services/catalog/internal/parser/animelib/latest_episode.go`
- `services/catalog/internal/service/episodes_lookup.go`
- `services/catalog/internal/handler/internal_episodes.go`

**Modified (catalog):**
- `services/catalog/internal/service/catalog.go` — added `ResolveAnimeLibID` wrapper exposing `findAnimeLibID`.
- `services/catalog/internal/transport/router.go` — registers the new internal route + handler param.
- `services/catalog/cmd/catalog-api/main.go` — constructs `EpisodesLookupService` + `InternalEpisodesHandler`; passes both into `NewRouter`.

**New (notifications):**
- `services/notifications/internal/domain/combo.go`
- `services/notifications/internal/job/hotcombos.go`
- `services/notifications/internal/job/metrics.go`
- `services/notifications/internal/job/detector.go`
- `services/notifications/internal/job/detector_test.go` (5 unit tests, in-memory SQLite)
- `services/notifications/internal/job/cleanup.go`
- `services/notifications/internal/job/scheduler.go`
- `services/notifications/internal/repo/maxwatched.go`
- `services/notifications/internal/repo/anime_view.go`
- `services/notifications/internal/repo/unread_gauge.go`
- `services/notifications/internal/service/catalog_client.go`
- `services/notifications/internal/service/payload_builder.go`
- `services/notifications/internal/handler/admin.go`

**Modified (notifications):**
- `services/notifications/go.mod` + `go.sum` — direct require on `github.com/robfig/cron/v3 v3.0.1` (matches scheduler service), `golang.org/x/sync v0.18.0`, `libs/cache`, `github.com/prometheus/client_golang`.
- `services/notifications/internal/config/config.go` — `DetectorConfig` struct + Redis config block.
- `services/notifications/internal/job/doc.go` — package doc updated to match the four-file structure now present.
- `services/notifications/internal/repo/snapshot.go` — `BulkLoad` + `BulkUpsert` (chunked OnConflict).
- `services/notifications/internal/transport/router.go` — mounts the two `/internal/*/run-once` routes when `adminHandler != nil`.
- `services/notifications/cmd/notifications-api/main.go` — full Phase-2 boot graph + scheduler.Start gated by `NOTIFICATIONS_DETECTOR_ENABLED`; shutdown sequences scheduler.Stop before srv.Shutdown.

**Modified (infra):**
- `docker/docker-compose.yml` — adds 7 `NOTIFICATIONS_*` env vars to the notifications service block + `catalog` to `depends_on`.
- `docker/.env.example` — appends the 7 vars + `CATALOG_URL` with inline comments.
- `Makefile` — adds `.PHONY: run-detector-once run-cleanup-once` targets.

## Next — Phase 3 unblocked

- **Detector live** — Phase 3 frontend NotificationCard can immediately render production notifications from the `new_episode` type. Payload shape verified end-to-end against the design doc spec.
- **Watch URL pattern locked** — `/anime/{anime_id}/watch?player={player}&episode={ep}&translation={translation_id}` is now the canonical deep-link the bell + toast will use.
- **`/api/notifications/*` already shipped in Phase 1** — Phase 3 has zero backend work, only Vue 3 components + websocket / polling for the bell badge.
- **Six metrics live** — Phase 3 can ship Grafana dashboards (or v1.1 can) without backend changes.

## Self-Check

- ALL Phase 2 files exist: 13 new files in `services/notifications/internal/` + 4 in `services/catalog/internal/` + infra updates (verified via `git log --stat` on the four commits).
- All four commits present in worktree branch `worktree-agent-a0a9bc0de0e92354b`: `69261b2`, `c1c9af5`, `58c05a1`, `4e1c0df`.
- 5/5 unit tests in `services/notifications/internal/job/detector_test.go` pass (`go test ./internal/job/`).
- `go build ./... && go vet ./...` green for both `services/catalog` and `services/notifications`.
- `make health` shows all 10 services healthy after redeploy.

## Self-Check: PASSED
