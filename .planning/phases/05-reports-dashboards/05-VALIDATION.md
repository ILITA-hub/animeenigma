---
phase: 05
slug: reports-dashboards
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-06
---

# Phase 05 — Validation Strategy

> Per-phase validation contract. This phase is declarative Grafana dashboard JSON + an alert rule — validated structurally (jq parse + provisioning load + a live render gate), not by unit tests.

## Test Infrastructure
| Property | Value |
|----------|-------|
| **Framework** | `jq` (JSON structural validity) + Grafana provisioning load (`docker logs grafana` shows no provisioning errors) + ClickHouse query dry-run (`clickhouse-client`) |
| **Quick run command** | `jq empty infra/grafana/dashboards/<new>.json && echo OK` |
| **Full suite command** | `for f in infra/grafana/dashboards/*.json; do jq empty "$f" || exit 1; done` then `make restart-grafana` + `docker logs animeenigma-grafana 2>&1 | grep -iE 'provisioning|error' | tail` |
| **Estimated runtime** | ~15 seconds (structural) + live render gate (manual) |

## Sampling Rate
- After every dashboard JSON task: `jq empty` the file + (if a panel rawSql changed) dry-run the SQL against ClickHouse.
- After the wave: provision-load (restart-grafana) and confirm no provisioning errors in logs.
- Before verify: every dashboard parses + loads; the live gate render is the human-verify proof.

## Per-Requirement Validation Map
| Requirement | Proof |
|-------------|-------|
| AR-REPORT-01 (pivot, template vars, ANY dimension) | Switch the `group_by` template var → the pivot panel regroups live (each dim drives `GROUP BY ${group_by}`); dropdowns self-discover via `SELECT DISTINCT` |
| AR-REPORT-02 (from→choke-point→effects) | The origin→operation→target table renders real requests+bytes for a real origin; bytes filter `source='be'` |
| AR-REPORT-03 (anomaly flagging) | Inject a synthetic volume spike → the avg+Nσ baseline panel flags it + the provisioned alert rule fires |
| AR-REPORT-04 (awareness overview) | One dashboard view shows current top operations + top external deps + active anomalies |

## Manual-Only Verifications
| Behavior | Why Manual | Instructions |
|----------|------------|--------------|
| Live render of all four reports + synthetic-spike anomaly flag | Requires running Grafana + ClickHouse with real register data | Non-autonomous live gate (mirror 02-04/03-06): restart-grafana, open each dashboard, inject a spike, confirm the flag |

## Validation Sign-Off
- [ ] Every dashboard JSON parses (`jq empty`)
- [ ] Provisioning loads with no errors
- [ ] Every panel's rawSql dry-runs against ClickHouse
- [ ] Byte panels filter source='be'
- [ ] Live gate render confirmed
- [ ] `nyquist_compliant: true` set

**Approval:** pending
