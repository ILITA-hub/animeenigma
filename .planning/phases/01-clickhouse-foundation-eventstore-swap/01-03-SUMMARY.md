---
phase: 01-clickhouse-foundation-eventstore-swap
plan: 03
subsystem: analytics
tags: [clickhouse, eventstore, dualwrite, grafana, datasource, observability, deploy, ar-store-04]

# Dependency graph
requires:
  - phase: 01-01
    provides: "running ClickHouse container (healthy) + analytics DB + native :9000 port"
  - phase: 01-02
    provides: "ClickHouseStore, OpenClickHouse(cfg), EnsureSchema(ctx,conn), CH GDPR erase funcs"
provides:
  - "DualWriteStore (PG primary / CH best-effort) wrapping two domain.EventStores"
  - "ANALYTICS_STORE_BACKEND selector (postgres | clickhouse | dualwrite) + CLICKHOUSE_* config"
  - "main.go backend branch wiring + dualwrite degrade-to-PG-on-CH-failure + CH GDPR erase"
  - "compose: analytics depends_on healthy clickhouse + CLICKHOUSE_* + ANALYTICS_STORE_BACKEND=dualwrite"
  - "grafana: grafana-clickhouse-datasource plugin + aenigma-clickhouse datasource (new uid)"
  - "6 product-analytics panels rewritten in ClickHouse SQL against events_resolved"
  - "live AR-STORE-04 end-to-end proof + docker/clickhouse/MIGRATION-NOTES.md"
affects:
  - "later CH-only cutover phase (flip ANALYTICS_STORE_BACKEND=clickhouse, retire PG write)"
  - "any future analytics dashboard work (now CH-backed via aenigma-clickhouse)"

# Tech tracking
tech-stack:
  added:
    - "grafana-clickhouse-datasource v4.17.0 (Grafana plugin, GF_INSTALL_PLUGINS)"
  patterns:
    - "reversible dual-write EventStore fan-out (primary authoritative, secondary best-effort/swallowed)"
    - "backend selector via single env var (postgres|clickhouse|dualwrite), default reversible"
    - "Grafana provisioning reads creds from container env (bare ${VAR}, defaults supplied by compose)"

key-files:
  created:
    - "services/analytics/internal/repo/dualwrite_store.go"
    - "services/analytics/internal/repo/dualwrite_store_test.go"
    - "docker/clickhouse/MIGRATION-NOTES.md"
  modified:
    - "services/analytics/internal/config/config.go"
    - "services/analytics/cmd/analytics-api/main.go"
    - "services/analytics/cmd/analytics-api/adapters.go"
    - "services/analytics/internal/transport/router.go (HEAD /health healthcheck fix)"
    - "docker/docker-compose.yml"
    - "docker/.env.example"
    - "docker/grafana/provisioning/datasources/datasources.yml"
    - "infra/grafana/dashboards/product-analytics.json"

key-decisions:
  - "Deployed ANALYTICS_STORE_BACKEND=dualwrite — Postgres stays source of truth + dashboard fallback; CH failure logged + swallowed (reversible, never propagated)"
  - "Grafana provisioning does NOT support shell ${VAR:-default}; CH datasource uses bare ${CLICKHOUSE_USER}/${CLICKHOUSE_PASSWORD} with defaults supplied via the grafana container env"
  - "grafana-clickhouse-datasource v4 expects integer format (0=time series); the legacy string 'time_series' is rejected — Pageviews panel set to format: 0"

patterns-established:
  - "Dual-write migration: prove parity per-event in both stores before any cutover; keep PG write as the rollback window"
  - "API-level dashboard sign-off: execute every panel's rawSql through /api/ds/query against the new datasource before asking for in-browser eyes"

requirements-completed: [AR-STORE-04]

# Metrics
duration: ~13min
completed: 2026-06-05
---

# Phase 01 Plan 03: ClickHouse EventStore Swap (live dual-write + dashboards) Summary

**The live analytics clickstream now dual-writes to both Postgres and ClickHouse (`ANALYTICS_STORE_BACKEND=dualwrite`, PG source of truth + reversible), a new `aenigma-clickhouse` Grafana datasource is provisioned, and all 6 product-analytics panels render from ClickHouse `events_resolved` — proven by a live smoke event reaching both stores (AR-STORE-04).**

## Performance

- **Duration:** ~13 min (Task 4–5 of this plan; Tasks 1–3 committed in a prior session)
- **Started:** 2026-06-05T00:47:25Z
- **Completed:** 2026-06-05T01:00:00Z
- **Tasks:** Tasks 4–5 (deploy + sign-off); Tasks 1–3 pre-committed (`23e005a4`, `98d77db4`, `ddbd322e`)
- **Files modified (this session):** 4 (`router.go`, `docker-compose.yml`, `datasources.yml`, `product-analytics.json`) + 1 created (`MIGRATION-NOTES.md`)

## Accomplishments

- **AR-STORE-04 verified live.** A real `POST /api/analytics/collect` envelope (marker `/smoke-1780620941`) landed in ClickHouse `analytics.events` **and** `events_resolved` **and** Postgres `analytics_events` — per-event dual-write parity confirmed.
- **Analytics deployed in dual-write mode + healthy.** `make redeploy-analytics` brought it up `healthy` (after a healthcheck fix), backend `dualwrite`, `EnsureSchema` auto-created `events`/`identities`/`events_resolved` in CH on boot.
- **`aenigma-clickhouse` datasource working.** Plugin `grafana-clickhouse-datasource v4.17.0` installed; datasource health = "Data source is working" (OK) after a creds-interpolation fix; `aenigma-postgres` uid untouched.
- **All 6 panels execute against ClickHouse.** Verified via Grafana `/api/ds/query` — 5 table panels + 1 time-series graph (Pageviews over time), no datasource/SQL errors.

## Task Commits

1. **Task 4 (healthcheck side-fix): HEAD /health** - `903daaa3` (fix)
2. **Task 4 (deploy/datasource side-fix): CH creds → grafana env + MIGRATION-NOTES** - `409adbf5` (fix)
3. **Task 5 (panel side-fix): Pageviews format int 0** - `9a4f8ef3` (fix)

Tasks 1–3 (pre-committed prior session): `23e005a4` (backend selector + CH config + dual-write store), `98d77db4` (main.go/adapters.go selector wiring), `ddbd322e` (compose CH wiring + datasource + 6 panels).

**Plan metadata commit:** added in the final docs commit (this SUMMARY + ROADMAP).

## Files Created/Modified

- `docker/clickhouse/MIGRATION-NOTES.md` (created) - dual-write rollout, parity check, AR-STORE-04 live-verify, reversible-cutover steps
- `services/analytics/internal/transport/router.go` - register `HEAD /health` so the `wget --spider` healthcheck passes
- `docker/docker-compose.yml` - grafana env now carries `CLICKHOUSE_USER`/`CLICKHOUSE_PASSWORD`
- `docker/grafana/provisioning/datasources/datasources.yml` - bare `${CLICKHOUSE_USER}`/`${CLICKHOUSE_PASSWORD}` (no `:-` — Grafana can't interpolate it)
- `infra/grafana/dashboards/product-analytics.json` - Pageviews panel `format: 0` (CH plugin integer format model)

## Deployment & Live Verification

| Step | Result |
|------|--------|
| `make redeploy-analytics` | analytics **healthy**, backend `dualwrite`, schema auto-created in CH |
| `make restart-grafana` + force-recreate | plugin `grafana-clickhouse-datasource v4.17.0` installed; `aenigma-clickhouse` provisioned |
| datasource health | `Data source is working` (OK) |
| live smoke `POST /api/analytics/collect` → 204 | marker `/smoke-1780620941` |
| CH `analytics.events` row for marker | **1** (also in `events_resolved`) |
| PG `analytics_events` row for marker | **1** (dual-write parity) |
| all 6 panels via `/api/ds/query` | **6 OK / 0 FAIL** (5 table + 1 graph) |

**Parity window (≥ 00:51Z boot):** PG `analytics_events` = 21, CH `analytics.events` = 22 (1-event in-flight delta between the two count queries; both stores receive the identical stream).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Analytics healthcheck failing → container flagged unhealthy for 2 days**
- **Found during:** Task 4 (deploy — analytics came up but stayed `unhealthy`)
- **Issue:** The Docker healthcheck uses `wget --spider` (a HEAD request); the chi router only registered `GET /health`, so HEAD returned 405. The pre-existing 2-day-old container was unhealthy for the same reason. The service itself served `GET /health` 200 fine.
- **Fix:** Registered `r.Head("/health", ...)` with the same handler (matches green services notifications/watch-together which already answer HEAD).
- **Files modified:** `services/analytics/internal/transport/router.go`
- **Commit:** `903daaa3`

**2. [Rule 1 - Bug] aenigma-clickhouse datasource failed auth ("default: Authentication failed")**
- **Found during:** Task 5 (datasource health check)
- **Issue:** Grafana provisioning does NOT honor shell `${VAR:-default}` interpolation; `${CLICKHOUSE_USER:-analytics}` resolved to an empty string, so the CH plugin fell back to the `default` user and failed auth. The grafana container also had no `CLICKHOUSE_*` env at all.
- **Fix:** Datasource file now uses bare `${CLICKHOUSE_USER}`/`${CLICKHOUSE_PASSWORD}`; the defaults are supplied via the grafana container env in compose (compose DOES honor `:-`), matching the clickhouse service creds (`analytics`/`changeme`).
- **Files modified:** `docker/grafana/provisioning/datasources/datasources.yml`, `docker/docker-compose.yml`
- **Commit:** `409adbf5`

**3. [Rule 1 - Bug] Pageviews-over-time panel: "invalid format value: time_series"**
- **Found during:** Task 5 (executing each panel's SQL through the datasource)
- **Issue:** The Task 3 panel rewrite set `format: "time_series"` on the only `timeseries` panel. The grafana-clickhouse-datasource v4 plugin rejects the legacy string format and expects an integer model (0=time series, 1=table, 2=logs). The underlying CH SQL was already valid (ran directly against CH).
- **Fix:** Set the Pageviews panel target `format: 0`. All 6 panels now execute (5 table + 1 graph). No other panel changed semantically.
- **Files modified:** `infra/grafana/dashboards/product-analytics.json`
- **Commit:** `9a4f8ef3`

## Reversible-cutover steps (later phase)

1. Soak `dualwrite`, confirm CH parity holds over a representative window.
2. Confirm all 6 panels render from `aenigma-clickhouse` with real traffic (done).
3. Flip `ANALYTICS_STORE_BACKEND=clickhouse` + `make redeploy-analytics` (PG write stops; CH boot failure becomes fatal by design).
4. Keep PG `analytics_events`/`analytics_identities` + the `postgres` backend code as the rollback window — do NOT drop in the same phase as the flip.
**Rollback at any time:** set `ANALYTICS_STORE_BACKEND=postgres` (or `dualwrite`) + redeploy — no data migration (PG never stopped being written in dualwrite).

## Remaining (human eyes)

The API-level sign-off is **complete and green** (datasource health OK, all 6 panel SQL queries return 200 with the smoke event reflected in Pageviews/Unique-visitors). The optional **in-browser** confirmation at `https://animeenigma.ru/admin/grafana → Product Analytics (Clickstream)` (desktop render of the 6 panels, no visual "datasource not found" surprise) is the only thing not machine-asserted (DS-NF-06 — jsdom/API can't fully assert pixel render). Everything the API can prove is proven.

## Self-Check: PASSED
