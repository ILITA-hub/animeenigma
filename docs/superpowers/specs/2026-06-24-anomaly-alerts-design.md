# Anomaly Alerts — Design Spec

- **Date**: 2026-06-24
- **Status**: Implemented (calibrated against live data 2026-06-24)
- **Scope**: Two Grafana unified-alerting rules — an egress-volume statistical anomaly and a scraper stream-resolution cascade alert. Both query ClickHouse `analytics.events`.
- **Worktree/branch**: `anomaly-alerts`

## Effort & impact (project convention — `.planning/CONVENTIONS.md`)

- **UXΔ = +1 (Better)** — indirect: faster detection of runaway egress and cascading stream-resolution latency shortens incidents; no direct UI change.
- **CDI = 0.02 * 8** — Spread ≈ 0.1 (Grafana provisioning files only), Shift ≈ 0.2 (additive observability + first ClickHouse-datasource *alert* in the repo), Effort_Fib = 8 (the CH-alerting format unknown + live calibration lifted this above the original estimate of 5). Not pre-multiplied.
- **MVQ = Griffin 88%/85%** — methodical, reuses the proven baseline query; the calibrated guards make it slop-resistant.

## Motivation

Two observability gaps from the streaming-path review (concerns 5–7):

1. **Egress cost has dashboards but no alert.** `infra/grafana/dashboards/activity-register-overview.json` already computes a per-operation baseline + σ anomaly over ClickHouse `analytics.events` (the `Active volume anomalies` panels), but nothing fires on it. Visibility without a trigger.
2. **Cold-cascade latency is invisible.** When the scraper failover loop walks several providers before resolving, the user waits. Nothing alerts on stream-resolution latency, so this rare-but-bad UX is a silent mystery.

Both are additive — no service code changes, only new Grafana alert rules following the `infra/grafana/alerts/scraper.yaml` pattern.

## Goals

- Fire a **warning** when an operation's egress (bytes) for the just-completed hour deviates > 3σ above its trailing-168h baseline **and** clears a 500 MiB/h absolute floor.
- Fire a **warning** when scraper stream resolutions cascade past a single provider's gate budget (≥ 3 resolutions > 12 s in the trailing hour).
- Reuse the existing `maintenance-webhook` contact point and the established source-of-truth + keep-in-sync provisioning pattern.
- Be **resistant to the false-positive modes that have bitten this repo before** (AUTO-487 NaN false-positive; the P95 traffic-gate) via calibrated floor/count guards.

## Non-goals

- No new contact points (routing: both → `maintenance-webhook`).
- No diurnal/day-of-week-aware baseline (flat trailing-hour baseline reused as-is).
- No per-host (per-CDN) egress baseline — rotating segment CDNs have no stable baseline, so grouping stays per-`operation`.
- No production Kubernetes alert provisioning (out of scope, mirroring `scraper.yaml`).
- No service code, metrics-emission, or analytics-pipeline changes.

## Substrate decision (revised after calibration)

Both alerts query **ClickHouse `analytics.events`** via datasource `aenigma-clickhouse`. The original spec put Alert B on Prometheus, but live data showed the scraper's `/scraper/*` routes are **not** instrumented with `http_request_duration_seconds` (only `/health` is), while `analytics.events.duration_ms` records per-operation stream-resolution latency including the failover cascade. So both alerts live in ClickHouse — coherent, and using the one substrate that actually holds the signals.

**ClickHouse alerting format gotcha (load-bearing):** provisioned CH alert queries hit the `grafana-clickhouse-datasource` backend directly, which takes `format` as a **numeric enum** (`1` = table), NOT the string `"table"` that dashboard panels use. A string format → "invalid format value" → the rule errors. Both rules use `format: 1`, validated end-to-end via `POST /api/ds/query` before shipping (`reference_grafana_clickhouse_format_enum`).

## Design

### Alert A — `EgressVolumeAnomaly` (warning)

- **Datasource**: ClickHouse `aenigma-clickhouse`.
- **Signal**: per-`operation` egress **bytes** (`bytes_out + bytes_in`) for the last *complete* hour vs the trailing baseline. The query returns the **count of anomalous operations** (single scalar); the rule fires when count > 0.

```sql
SELECT count() AS value FROM (
  WITH hourly AS (
    SELECT operation, toStartOfHour(timestamp) AS h, sum(bytes_out + bytes_in) AS b
    FROM analytics.events
    WHERE timestamp >= now() - INTERVAL 168 HOUR
      AND effect_kind = 'egress' AND source = 'be'
    GROUP BY operation, h),
  stats AS (
    SELECT operation,
      avgIf(b,       h <  toStartOfHour(now()))                   AS baseline_avg,
      stddevPopIf(b, h <  toStartOfHour(now()))                   AS baseline_sd,
      anyIf(b,       h =  toStartOfHour(now() - INTERVAL 1 HOUR)) AS last_hour
    FROM hourly GROUP BY operation)
  SELECT operation FROM stats
  WHERE baseline_avg > 0
    AND last_hour > baseline_avg + 3 * baseline_sd
    AND last_hour > 524288000)   -- 500 MiB/h floor
```

- **Why bytes, per-operation**: egress *cost* is the concern (bytes, not request count); operations are a bounded, stable set with a meaningful baseline, whereas rotating CDN hosts have none.
- **Evaluation**: group `interval: 5m`, `for: 0s`. The compared value is the last complete hour, so it steps hourly and stays firing for the anomalous hour.
- **False-positive guards**: `baseline_avg > 0`, the **500 MiB/h absolute floor**, and **σ = 3**.
- **Labels**: `severity=warning`, `component=streaming`, `reason=egress_anomaly`. No `provider`/`server` → escalate-to-admin (intended). Annotation points at the Activity Register Overview dashboard for the which/how-much.

### Alert B — `ScraperStreamCascadeLatency` (warning)

- **Datasource**: ClickHouse `aenigma-clickhouse`.
- **Signal**: count of `scraper GET /scraper/stream` resolutions whose `duration_ms > 12000` in the trailing hour; the rule fires when count ≥ 3 (threshold `gt 2`).

```sql
SELECT count() AS value
FROM analytics.events
WHERE operation = 'scraper GET /scraper/stream'
  AND timestamp >= now() - INTERVAL 1 HOUR
  AND duration_ms > 12000
```

- **Why count-of-cascades, not a percentile**: stream latency is bimodal — p50 ≈ 154 ms (warm cache) vs an 8 s ceiling (the cold-path playability-gate budget), with max observed 8001 ms. So "p95 > 8s" is *normal*, not anomalous. A resolution > 12 s means a genuine **multi-provider** cascade (1.5× the 8 s gate; zero today). Counting ≥ 3 such resolutions per hour is robust to the low volume (~150 streams/day) and to one-off outliers.
- **Evaluation**: group `interval: 5m`, `for: 0s` (the 1 h window is the smoothing).
- **Labels**: `severity=warning`, `component=scraper`, `reason=stream_cascade_latency`. No `provider` label → escalate-to-admin; annotation directs to provider health + `parser_fallback_total`.

### Routing

Both rules → existing `maintenance-webhook` contact point → host-side `services/maintenance` `/api/grafana-webhook`. Neither carries the `provider`/`server`/`reason`-for-fix triple, so the bot escalates both to a plain admin notification — correct for "egress spiked" and "streams are cascading."

## Files changed

| File | Change |
|------|--------|
| `infra/grafana/alerts/anomaly.yaml` | **New** — both rules, group `Anomaly Alerts` (source-of-truth). |
| `docker/grafana/provisioning/alerting/rules.yml` | **Append** the same `Anomaly Alerts` group inline (the ACTIVE provisioning provider, copied to `/etc/grafana/provisioning` at boot) under a "keep in sync" pointer. |
| `infra/grafana/alerts/README.md` | Add `anomaly.yaml` to the Files table + the CH `format: 1` note. |

Production K8s (`deploy/kustomize/base/monitoring/grafana/configmap-alerts.yaml`) is **out of scope**, consistent with `scraper.yaml`.

## Calibration (2026-06-24, live data)

- **Egress floor 500 MiB/h** — the only large egress operation is `streaming GET /api/v1/hls-proxy` (avg 309 / p95 863 / max 1694 MiB/h); every other operation peaks < 60 MiB/h. The floor cleanly isolates video egress; its 3σ band ≈ 1.3 GiB/h, so only genuine spikes fire.
- **Cascade threshold 12 s / count ≥ 3** — `scraper GET /scraper/stream` over 24 h: n=157, p50 154 ms, p95/p99 8000 ms, max 8001 ms (the gate ceiling). `> 12 s` is currently zero, so the alert only fires on real multi-provider cascades.

## Verification

1. Validate `rules.yml` loads cleanly (no provisioning errors that could disable existing alerts) before promoting to prod.
2. `make restart-grafana` (config-only change; restart re-runs provisioning).
3. Grafana API `GET /api/v1/provisioning/alert-rules`: both rules present, last-eval state `Normal` (not `Error` — proves the CH `format: 1` path).
4. Confirm the existing alert groups (`AnimeEnigma Alerts`, `Scraper Self-Healing`) are still loaded (the new group didn't break the file).
5. Fire-path proof: temporarily lower a threshold so live data trips it, confirm `maintenance-webhook` receives it, then revert.

## Risks & mitigations

- **CH-datasource alerting / NUMERIC format** — *validated* via `POST /api/ds/query` with `format: 1` (status 200, numeric frame) before shipping; `noDataState: OK`, `execErrState: Error`.
- **Malformed provisioning file disabling all alerts** — *mitigation*: validate the whole `rules.yml` (throwaway Grafana / API) before restarting prod; the rule structure mirrors the existing working Prometheus rules, only refId A's datasource/model differ.
- **False positives on low traffic** — calibrated floor + count guards (the AUTO-487 / P95-gate lessons).
- **Flat baseline misses daytime spikes below night+3σ** — accepted (parity with the existing dashboard).

## Open questions

None. Thresholds calibrated against live data.
