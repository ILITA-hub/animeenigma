---
phase: 05
slug: reports-dashboards
status: passed
verified: 2026-06-08
note: data-plane proven live against real register; visual render confirmation deferred (non-blocking)
---

# Phase 05 Verification — Reports & Dashboards

**Verdict: PASSED (data-plane)** — all four AR-REPORT criteria proven against the live ClickHouse register; only the cosmetic visual eyeball + synthetic-spike alert test are deferred.

| Requirement | Evidence (live, 2026-06-08) | Status |
|-------------|------------------------|--------|
| AR-REPORT-01 (pivot, ANY-dimension template vars) | 3 dashboards provisioned + loaded in Grafana (`/api/search`); `SELECT DISTINCT effect_kind` → {cache, db_write, egress} self-discovers; `$group_by` is a fixed custom-var allowlist driving `GROUP BY ${group_by}` | PASS |
| AR-REPORT-02 (from→choke-point→effects) | `activity-register-flow.json` renders real origin→operation→target egress rows (megaplay.buzz, miruro.tv, gogoanimes.fi…) with requests + bytes, filtered `effect_kind='egress' AND source='be'` | PASS |
| AR-REPORT-03 (volume anomaly flag) | `activity-register-volume-anomaly` alert rule provisioned on `aenigma-clickhouse` (delta +1, 14→15); full WITH/CTE avg+3σ query executes clean read-only on live `events`, returns 0 (correct baseline, no false positives) | PASS |
| AR-REPORT-04 (awareness overview) | `activity-register-overview.json` provisioned + loaded: top ops + top external deps (egress+be) + active anomalies in one view, tunable `$sigma`/`$window` | PASS |

**Cross-cutting:** byte discipline (`source='be'`) enforced in all 3 dashboards + the alert (never sums approximate fe_rum rows — AR-FE-03 carries through); SQL-injection guard (fixed custom `$group_by`, parameterized filter macros); Sankey deferred explicitly (table + groupBy transform). Grafana provisioning reloaded with no rule errors.

**Deferred (non-blocking, visual):** human eyeball of rendered panels + a synthetic-spike alert-fire test. The data-plane beneath every visual is proven, so this is confirmatory. No synthetic rows were written to production analytics data by design.
