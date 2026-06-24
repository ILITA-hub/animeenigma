# Anomaly Alerts — Design Spec

- **Date**: 2026-06-24
- **Status**: Approved (brainstorm) — pending implementation plan
- **Scope**: Two Grafana unified-alerting rules — an egress-volume statistical anomaly and a scraper stream-resolution latency ("cold cascade") alert.
- **Worktree/branch**: `anomaly-alerts`

## Effort & impact (project convention — `.planning/CONVENTIONS.md`)

- **UXΔ = +1 (Better)** — indirect: faster detection of degraded playback latency and runaway egress shortens incidents; no direct UI change.
- **CDI = 0.01 * 5** — Spread ≈ 0.1 (3 files, one subsystem: Grafana provisioning), Shift ≈ 0.1 (purely additive observability; no runtime code path changes), Effort_Fib = 5. (Not pre-multiplied.)
- **MVQ = Griffin 88%/85%** — methodical, reuses a proven baseline query; the false-positive guards make it slop-resistant.

## Motivation

Two observability gaps surfaced during the streaming-path review (concerns 5–7):

1. **Egress cost has dashboards but no alert.** `infra/grafana/dashboards/activity-register-overview.json` already computes a per-operation baseline + σ anomaly over the ClickHouse `events` table (the `Active volume anomalies` / `Active anomaly count` panels), but nothing fires on it. Visibility without a trigger.
2. **Cold-cascade latency is invisible.** When the scraper failover loop walks several providers (each up to `SCRAPER_PROVIDER_TIMEOUT`) before resolving or failing, the user waits tens of seconds. There is no alert on stream-resolution latency, so this rare-but-bad UX is a silent mystery rather than a signal.

Both are additive: no service code changes, only new Grafana alert rules following the established `infra/grafana/alerts/scraper.yaml` pattern.

## Goals

- Fire a **warning** when an operation's egress (bytes) for the just-completed hour deviates > 3σ above its trailing baseline **and** clears an absolute floor.
- Fire a **warning** when scraper `/scraper/stream` p95 latency stays high under real traffic.
- Reuse the existing `maintenance-webhook` contact point and the established source-of-truth + keep-in-sync provisioning pattern.
- Be **resistant to the false-positive modes that have bitten this repo before** (AUTO-487 NaN false-positive; the P95 traffic-gate) via explicit traffic/baseline/floor guards.

## Non-goals

- No new contact points or notification channels (routing decision: both → `maintenance-webhook`).
- No diurnal/day-of-week-aware baseline (the existing flat trailing-hour baseline is reused as-is; refinement is future work).
- No per-host (per-CDN) egress baseline — rotating segment CDNs have no stable week-long baseline, so anomaly grouping stays per-`operation`.
- No production Kubernetes alert provisioning (out of scope, mirroring how `scraper.yaml` left it).
- No changes to service code, metrics emission, or the analytics pipeline.

## Design

### Alert A — `EgressVolumeAnomaly` (warning)

- **Datasource**: ClickHouse, uid `aenigma-clickhouse` (type `grafana-clickhouse-datasource`).
- **Signal**: per-`operation` egress **bytes** (`bytes_out + bytes_in`) for the last *complete* hour vs the trailing baseline.
- **Query** (window and σ hardcoded — Grafana alert rules cannot use dashboard template vars):

```sql
WITH hourly AS (
  SELECT operation, toStartOfHour(timestamp) AS h, sum(bytes_out + bytes_in) AS bytes
  FROM events
  WHERE timestamp >= now() - INTERVAL 168 HOUR
    AND effect_kind = 'egress' AND source = 'be'
  GROUP BY operation, h),
stats AS (
  SELECT operation,
    avgIf(bytes,       h < toStartOfHour(now()))                   AS baseline_avg,
    stddevPopIf(bytes, h < toStartOfHour(now()))                   AS baseline_sd,
    anyIf(bytes,       h = toStartOfHour(now() - INTERVAL 1 HOUR)) AS last_hour
  FROM hourly GROUP BY operation)
SELECT operation, last_hour
FROM stats
WHERE baseline_avg > 0
  AND last_hour > baseline_avg + 3 * baseline_sd
  AND last_hour > 524288000   -- 500 MiB/h absolute floor (anti-flap on low traffic)
```

- **Why bytes, not requests**: egress *cost* is the concern; a bandwidth-drain manifests as bytes, not necessarily request count. (The dashboard panel uses `requests`; this alert deliberately diverges to `bytes`.)
- **Why per-`operation`**: operations are a bounded, stable set with a meaningful week-long baseline. Per-`target`(host) is rejected — rotating CDN hosts churn and have no stable baseline.
- **Evaluation**: `interval: 15m`, `for: 0s`. The compared value is the last complete hour, so it steps hourly and stays firing for the duration of an anomalous hour.
- **False-positive guards** (the crux): (1) `baseline_avg > 0`; (2) **absolute byte floor** (initial 500 MiB/h, calibrated — see below); (3) **σ = 3**.
- **Grafana ClickHouse format gotcha**: the alert query must return a **NUMERIC**-format frame (one numeric value per series, labeled by `operation`) or Grafana's alerting reducer rejects it (ref: `reference_grafana_clickhouse_format_enum`). The rule sets the CH query `format` explicitly; verification confirms it evaluates.
- **Labels**: `severity=warning`, `component=streaming`, `reason=egress_anomaly`.
- **Annotation**: summary names `{{ operation }}`, observed bytes, and baseline. No `provider`/`server` labels → the maintenance bot degrades to "escalate" (admin notification), which is the intended behavior.

### Alert B — `ScraperStreamLatencyHigh` (warning)

- **Datasource**: Prometheus, uid `PBFA97CFB590B2093`.
- **Signal**: p95 of `/scraper/stream` request duration, gated on real traffic.
- **Query**:

```promql
histogram_quantile(0.95,
  sum by (le) (rate(http_request_duration_seconds_bucket{service="scraper", path="/scraper/stream"}[10m]))
) > 8
and
sum(rate(http_request_duration_seconds_count{service="scraper", path="/scraper/stream"}[10m])) > 0.05
```

- **Why latency, not fallback-rate**: latency is the symptom the user feels (their wait while the failover loop runs). Fallback-rate is a noisier proxy — degraded providers always fall back during normal operation.
- **Traffic gate**: the `> 0.05 rps` clause is mandatory — it is the same guard that fixed the P95 / parser-failure-rate false-positive flaps on sparse buckets.
- **Sustain**: `for: 10m`.
- **Threshold**: p95 > 8s (initial — normal p95 should be ~2–3s; a cold cascade across providers runs 30–60s). Calibrated — see below.
- **Labels**: `severity=warning`, `component=scraper`, `reason=stream_latency`.
- **Annotation**: reports observed p95 and directs the admin to check provider health + fallback rate. No `provider` label (the http-duration metric has none) → escalate-to-admin, correct for a symptom alert.

### Routing

Both rules route to the existing `maintenance-webhook` contact point (declared in `docker/grafana/provisioning/alerting/contactpoints.yml`) → host-side `services/maintenance` `/api/grafana-webhook`. No new contact point. Neither alert carries the `provider`/`server`/`reason`-for-fix label triple, so the bot escalates both to a plain admin notification rather than attempting an autonomous fix — the correct behavior for "egress spiked" and "streams are slow."

## Files changed

| File | Change |
|------|--------|
| `infra/grafana/alerts/streaming.yaml` | **New** — `EgressVolumeAnomaly` rule (group `Streaming Egress`). |
| `infra/grafana/alerts/scraper.yaml` | **Append** `ScraperStreamLatencyHigh` rule to the existing `Scraper Self-Healing` group (or a sibling group). |
| `infra/grafana/alerts/README.md` | Add both rules to the Files table + note `streaming.yaml`. |
| `docker/grafana/provisioning/alerting/rules.yml` | Append both rules inline under the existing "keep in sync" block (dev-compose Option-A pattern). |

Production K8s (`deploy/kustomize/base/monitoring/grafana/configmap-alerts.yaml`) is **out of scope**, consistent with how `scraper.yaml` was shipped.

## Threshold calibration (planning step, before ship)

Both thresholds are placeholders until grounded in live data. During planning/implementation, run one read-only query each and set the final values:

- **Egress floor**: `SELECT operation, max(b) FROM (SELECT operation, toStartOfHour(timestamp) AS h, sum(bytes_out+bytes_in) AS b FROM events WHERE effect_kind='egress' AND source='be' AND timestamp >= now() - INTERVAL 168 HOUR GROUP BY operation, h) GROUP BY operation` → set the floor above routine hourly peaks of the noisiest operation.
- **Latency threshold**: `histogram_quantile(0.95, sum by(le)(rate(http_request_duration_seconds_bucket{service="scraper",path="/scraper/stream"}[1h])))` over a normal day → set the threshold a comfortable margin above normal p95.

If live data is unavailable at implementation time, ship the placeholders (500 MiB/h, 8s) and leave a TODO-issue (`docs/issues/`) to recalibrate after a week of data.

## Verification

1. `make redeploy-grafana` (or restart Grafana so provisioning reloads).
2. Confirm via Grafana API (`GET /api/v1/provisioning/alert-rules`) that both rules provisioned with **no errors**.
3. Confirm the ClickHouse rule **evaluates to a numeric** (the format gotcha) — check the rule's last-evaluation state is `Normal`, not `Error`.
4. Confirm the Prometheus rule parses and evaluates (state `Normal`).
5. **Fire-path proof**: temporarily lower the egress floor to a value the live data exceeds (and/or the latency threshold to ~0.1s), confirm each transitions to `Pending`→`Alerting` and the `maintenance-webhook` receives it, then revert to the calibrated values.
6. Keep-in-sync check: the inline `rules.yml` copy matches the `infra/grafana/alerts/*.yaml` source.

## Risks & mitigations

- **ClickHouse-datasource alerting feasibility / NUMERIC format** — the `grafana-clickhouse-datasource` plugin supports backend alerting, but the query frame must be numeric. *Mitigation*: explicit `format` in the rule; verification step 3 proves it; if the plugin version cannot alert, fall back to a Prometheus recording-rule baseline (degraded — no σ) and note the limitation.
- **False positives on low traffic** — *Mitigation*: the three egress guards + the latency traffic gate, all modeled on the prior AUTO-487 / P95-gate fixes.
- **Flat baseline misses daytime spikes below night+3σ** — accepted (parity with the existing dashboard); diurnal-aware baseline is future work.
- **`path` label value drift** — the query assumes `path="/scraper/stream"`. *Mitigation*: verify the scraper emits `http_request_duration_seconds` with exactly `service="scraper"` and `path="/scraper/stream"` before finalizing; adjust the matcher if the middleware records a different pattern.

## Open questions

None blocking. Thresholds are placeholders by design, calibrated during implementation per the calibration section.
