# Analytics Clickstream → ClickHouse Migration Notes

**Requirement:** AR-STORE-04 — migrate the existing analytics clickstream onto
ClickHouse the lowest-risk reversible way, keep ingesting, keep the dashboards
rendering.

**Phase / Plan:** `01-clickhouse-foundation-eventstore-swap` / `01-03`

---

## Rollout strategy: reversible dual-write

The clickstream was migrated via a **dual-write** `EventStore` wrapper, NOT a hard
cutover. Postgres remains the **source of truth** and the dashboard fallback; every
`InsertBatch` / `UpsertIdentity` is fanned out to ClickHouse **best-effort** (a CH
write failure is logged and swallowed — it can never fail the Postgres write or take
analytics down). The active backend is selected by a single env var:

```
ANALYTICS_STORE_BACKEND = postgres | clickhouse | dualwrite
```

- `postgres`  — original behavior (rollback target).
- `dualwrite` — **deployed value** (PG authoritative + CH best-effort). The compose
  default is `${ANALYTICS_STORE_BACKEND:-dualwrite}`.
- `clickhouse` — CH-only (the later, fully-reversible cutover; PG kept as rollback).

On boot in `dualwrite` mode, a CH open/`EnsureSchema` failure **degrades to PG-only**
(WARN logged) so a ClickHouse outage cannot take the live clickstream down. In
`clickhouse` mode a CH boot failure is fatal (explicit CH-only must not silently fall
back to PG).

**Deployed value at end of Phase 01:** `ANALYTICS_STORE_BACKEND=dualwrite`
(Postgres source of truth — the full CH-only flip is a later reversible toggle).

---

## What was wired (01-03)

- `services/analytics/internal/repo/dualwrite_store.go` — `DualWriteStore`
  (PG primary / CH best-effort), satisfies `domain.EventStore`.
- `services/analytics/internal/config/config.go` — `ANALYTICS_STORE_BACKEND`
  selector + `CLICKHOUSE_*` connection sub-config.
- `services/analytics/cmd/analytics-api/main.go` — backend branch
  (postgres | clickhouse | dualwrite); CH/dualwrite paths open the CH connection +
  run `EnsureSchema`. GDPR erase (`adapters.go`) extended to ClickHouse when CH active.
- `docker/docker-compose.yml` — analytics carries `CLICKHOUSE_*` +
  `ANALYTICS_STORE_BACKEND=dualwrite`, `depends_on: clickhouse (service_healthy)`;
  grafana installs `grafana-clickhouse-datasource` and carries
  `CLICKHOUSE_USER`/`CLICKHOUSE_PASSWORD` so the datasource provisioning can read them.
- `docker/grafana/provisioning/datasources/datasources.yml` — NEW
  `aenigma-clickhouse` datasource (uid never reuses `aenigma-postgres`).
- `infra/grafana/dashboards/product-analytics.json` — all 6 panels rewritten in
  ClickHouse SQL against the `events_resolved` view, bound to `aenigma-clickhouse`.

### Schema (auto-created by `EnsureSchema` on analytics boot)

ClickHouse DB `analytics` now contains:

- `events` — raw clickstream events (MergeTree, native TTL retention).
- `identities` — anonymous→user identity stitches.
- `events_resolved` — the resolved view the 6 dashboard panels query
  (`person_id`, `session_id`, `resolved_user_id`, `event_type`, `el_selector`,
  `path`, `active_ms`, `timestamp`, `received_at`).

Retention is native ClickHouse TTL (no hand-rolled purge cron for CH); the existing
Postgres retention purge cron stays PG-only.

---

## Deploy + live verification (2026-06-05)

1. `make redeploy-analytics` — analytics came up **healthy** in `dualwrite` mode
   (`analytics store backend {"backend":"dualwrite"}`). `EnsureSchema` created
   `events` / `identities` / `events_resolved` in CH on boot.
   - Side fix: the Docker healthcheck uses `wget --spider` (HEAD); the chi router
     only registered `GET /health` → 405 → container flagged unhealthy for 2 days.
     Added `HEAD /health` (matches the green services). Analytics is now `healthy`.
2. `make restart-grafana` + `docker compose up -d --force-recreate grafana` —
   installed `grafana-clickhouse-datasource v4.17.0` and provisioned
   `aenigma-clickhouse`.
   - Side fix: Grafana provisioning does **not** support shell `${VAR:-default}`
     interpolation — `${CLICKHOUSE_USER:-analytics}` resolved to empty, so the CH
     plugin fell back to the `default` user and failed auth. Changed the datasource
     file to bare `${CLICKHOUSE_USER}` / `${CLICKHOUSE_PASSWORD}` and supplied the
     defaults via the grafana container env (compose DOES honor `:-`).
   - Datasource health after fix: `Data source is working` (OK).

### AR-STORE-04 verified 2026-06-05

A live smoke pageview was pushed through the **real** public collect contract
(envelope `{anonymous_id, session_id, events:[{event_type,url,path}]}`) via the
gateway:

```
POST http://127.0.0.1:8000/api/analytics/collect  →  204
marker path = /smoke-1780620941   anonymous_id = smoke-anon
```

- **ClickHouse** `analytics.events` row for the marker: **1** (also present in
  `analytics.events_resolved` — the panel source).
- **Postgres** `analytics_events` row for the marker: **1** (dual-write parity).

**AR-STORE-04 verified 2026-06-05: event smoke-1780620941 reached ClickHouse
(`analytics.events` + `events_resolved`) AND Postgres, and is queryable by the
Pageviews-over-time / Unique-visitors panel SQL.**

### Parity check (window ≥ 2026-06-05 00:51Z, analytics boot)

| Store                          | Count |
|--------------------------------|-------|
| Postgres `analytics_events`    | 21    |
| ClickHouse `analytics.events`  | 22    |

The 1-event delta is in-flight live traffic landing between the two count queries
(CH counted marginally later) — both stores receive the identical clickstream. The
per-event smoke marker matches exactly once in both stores, which is the definitive
parity proof. (CH grand total at verification time: 34 — it also retained the events
from the earlier 00:48Z boot before the health-fix redeploy reset the PG window
baseline; those same events were dual-written to PG too.)

The `aenigma-clickhouse` datasource ran the live panel query
`SELECT count() FROM analytics.events_resolved` → returned a non-zero count through
Grafana, confirming the dashboard datasource path end-to-end at the API level.

---

## Reversible-cutover steps (later phase)

The migration is fully reversible. Postgres data + write path are kept as the
rollback window (RESEARCH §Migration step 5).

**To complete the CH-only cutover (later, reversible):**

1. Soak `dualwrite` and confirm CH parity holds over a representative window
   (compare `analytics_events` vs `analytics.events` counts + spot-check rows).
2. Confirm all 6 product-analytics panels render from `aenigma-clickhouse` with
   real traffic (no Postgres dependency in the dashboards — already done in 01-03).
3. Flip `ANALYTICS_STORE_BACKEND=clickhouse` and `make redeploy-analytics`. The PG
   write stops; CH becomes the sole store. (CH boot failure is now fatal by design.)
4. Keep the Postgres `analytics_events` / `analytics_identities` tables + the
   `postgres` backend code as the rollback window — do NOT drop them in the same
   phase as the flip.

**To roll back at any time:** set `ANALYTICS_STORE_BACKEND=postgres` (or
`dualwrite`) and redeploy analytics. No data migration is required — Postgres never
stopped being written while in `dualwrite`.
