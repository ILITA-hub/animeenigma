# Playback / Health Grafana Merge — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the three dashboards (Player/Health, Scraper/Health, Scraper/Provider Health) with a single unified, collapsible-row **Playback / Health** Grafana dashboard that adds parser success-rate %, p95 latency, and a provider×stage health matrix.

**Architecture:** Provisioned Grafana dashboard JSON. One new file `docker/grafana/dashboards/playback-health.json` (provisioned into the General folder via the existing `default` file provider). The three old JSON files are deleted. Two template variables (`$provider`, `$stage`) filter the EN-scraper rows. All PromQL was pre-validated against live Prometheus during design.

**Tech Stack:** Grafana 10.3.3, Prometheus datasource (`DS_PROMETHEUS` template var, uid `PBFA97CFB590B2093`), schemaVersion 39. Validation via `python3 -m json.tool` + live Prometheus query API (`http://localhost:9090/prometheus/api/v1/query`).

**Design doc:** `docs/superpowers/specs/2026-06-05-playback-health-grafana-merge-design.md`

---

## File Structure

- **Create:** `docker/grafana/dashboards/playback-health.json` — the single unified dashboard.
- **Delete:** `docker/grafana/dashboards/player-health.json`, `docker/grafana/dashboards/scraper-health.json`, `infra/grafana/dashboards/scraper-provider-health.json`.
- **Unchanged:** `docker/grafana/provisioning/dashboards/dashboards.yml` (the `default` provider already globs `docker/grafana/dashboards/*.json`; no provisioning edit needed). `infra/grafana/dashboards/scraper-providers.json` ("Scraper / Provider Management") stays — it is NOT part of this merge.

Layout (24-col grid): a 5-stat always-visible at-a-glance strip (y0, h4), then three collapsible `row` panels each starting expanded — **Player / Parser Health**, **EN Scraper Provider Health**, **EN Playability Canary**.

---

## Task 1: Create the unified dashboard JSON

**Files:**
- Create: `docker/grafana/dashboards/playback-health.json`

- [ ] **Step 1: Write the complete dashboard file**

Write the following verbatim to `docker/grafana/dashboards/playback-health.json`:

```json
{
  "annotations": { "list": [] },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
  "id": null,
  "links": [],
  "description": "Unified playback health: RU/JP/18+ player & catalog-parser health (success-rate %, p95 latency, liveness) plus the EN OurEnglish scraper failover chain (provider×stage probes, enabled/degraded state, fallbacks, daily playability canary). Replaces the former Player/Health, Scraper/Health and Scraper/Provider Health dashboards.",
  "panels": [
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Count of RU player liveness probes currently UP (max 2: kodik, animelib).",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "red", "value": null }, { "color": "orange", "value": 1 }, { "color": "green", "value": 2 } ] },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 4, "x": 0, "y": 0 },
      "id": 1,
      "options": { "colorMode": "background", "graphMode": "none", "justifyMode": "center", "orientation": "auto", "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false }, "textMode": "value_and_name" },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "sum(player_health_up)", "legendFormat": "RU Players Up", "refId": "A" } ],
      "title": "RU Players Up",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "EN scraper providers currently enabled (provider_enabled=1). Degraded providers (SCRAPER_DEGRADED_PROVIDERS) report 0.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "red", "value": null }, { "color": "orange", "value": 2 }, { "color": "green", "value": 4 } ] },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 5, "x": 4, "y": 0 },
      "id": 2,
      "options": { "colorMode": "background", "graphMode": "none", "justifyMode": "center", "orientation": "auto", "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false }, "textMode": "value_and_name" },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "sum(provider_enabled)", "legendFormat": "EN Providers Enabled", "refId": "A" } ],
      "title": "EN Providers Enabled",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Fraction of EN scraper provider×stage probes currently UP. 1.0 = all healthy.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "red", "value": null }, { "color": "orange", "value": 0.8 }, { "color": "green", "value": 1 } ] },
          "unit": "percentunit"
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 5, "x": 9, "y": 0 },
      "id": 3,
      "options": { "colorMode": "background", "graphMode": "none", "justifyMode": "center", "orientation": "auto", "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false }, "textMode": "value_and_name" },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "sum(provider_health_up) / count(provider_health_up)", "legendFormat": "EN Stage Health", "refId": "A" } ],
      "title": "EN Stage Health",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Age of the last successful scraper playability canary run. Red if stale (canary runs daily).",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "green", "value": null }, { "color": "orange", "value": 100000 }, { "color": "red", "value": 200000 } ] },
          "unit": "s"
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 5, "x": 14, "y": 0 },
      "id": 4,
      "options": { "colorMode": "value", "graphMode": "none", "justifyMode": "center", "orientation": "auto", "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false }, "textMode": "value_and_name" },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "time() - scheduler_job_last_success_timestamp{job=\"scraper_playability_canary\"}", "legendFormat": "Canary Age", "refId": "A" } ],
      "title": "Canary Last Run (age)",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Total provider→provider failover events in the EN scraper chain over the last hour.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "green", "value": null }, { "color": "orange", "value": 5 }, { "color": "red", "value": 20 } ] },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 5, "x": 19, "y": 0 },
      "id": 5,
      "options": { "colorMode": "background", "graphMode": "area", "justifyMode": "center", "orientation": "auto", "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false }, "textMode": "value_and_name" },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "sum(increase(parser_fallback_total[1h]))", "legendFormat": "Fallbacks (1h)", "refId": "A" } ],
      "title": "Fallbacks (1h)",
      "type": "stat"
    },

    { "collapsed": false, "gridPos": { "h": 1, "w": 24, "x": 0, "y": 4 }, "id": 10, "panels": [], "title": "Player / Parser Health (RU / JP / 18+)", "type": "row" },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "RU player liveness probes (1=UP, 0=DOWN). Probes exist only for kodik and animelib.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "custom": { "fillOpacity": 80, "lineWidth": 0 },
          "mappings": [ { "options": { "0": { "color": "red", "text": "DOWN" }, "1": { "color": "green", "text": "UP" } }, "type": "value" } ],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "red", "value": null }, { "color": "green", "value": 1 } ] }
        },
        "overrides": []
      },
      "gridPos": { "h": 6, "w": 12, "x": 0, "y": 5 },
      "id": 11,
      "options": { "alignValue": "center", "mergeValues": true, "rowHeight": 0.8, "showValue": "never" },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "player_health_up", "legendFormat": "{{ player }}", "refId": "A" } ],
      "title": "Player Liveness",
      "type": "state-timeline"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Catalog-parser request success rate per provider (success / total) over a 5m window. Providers: allanime, anime18, animelib, hanime, kodik. NOT the EN scraper provider set.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "axisBorderShow": false, "drawStyle": "line", "fillOpacity": 10, "lineInterpolation": "smooth", "lineWidth": 2, "pointSize": 5, "showPoints": "auto", "spanNulls": true },
          "min": 0,
          "max": 1,
          "unit": "percentunit"
        },
        "overrides": []
      },
      "gridPos": { "h": 6, "w": 12, "x": 12, "y": 5 },
      "id": 12,
      "options": { "legend": { "calcs": ["mean", "last"], "displayMode": "table", "placement": "bottom" }, "tooltip": { "mode": "multi", "sort": "desc" } },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "sum by (provider) (rate(parser_requests_total{status=\"success\"}[5m])) / sum by (provider) (rate(parser_requests_total[5m]))", "legendFormat": "{{ provider }}", "refId": "A" } ],
      "title": "Parser Success Rate % (by provider)",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Catalog-parser p95 request latency per provider over a 5m window.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "axisBorderShow": false, "drawStyle": "line", "fillOpacity": 10, "lineInterpolation": "smooth", "lineWidth": 2, "pointSize": 5, "showPoints": "auto", "spanNulls": true },
          "unit": "s"
        },
        "overrides": []
      },
      "gridPos": { "h": 7, "w": 12, "x": 0, "y": 11 },
      "id": 13,
      "options": { "legend": { "calcs": ["mean", "max"], "displayMode": "table", "placement": "bottom" }, "tooltip": { "mode": "multi", "sort": "desc" } },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "histogram_quantile(0.95, sum by (le, provider) (rate(parser_request_duration_seconds_bucket[5m])))", "legendFormat": "{{ provider }}", "refId": "A" } ],
      "title": "Parser p95 Latency (by provider)",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Seconds since the last RU player health-check tick, per player.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "green", "value": null }, { "color": "orange", "value": 600 }, { "color": "red", "value": 1800 } ] },
          "unit": "s"
        },
        "overrides": []
      },
      "gridPos": { "h": 7, "w": 12, "x": 12, "y": 11 },
      "id": 14,
      "options": { "colorMode": "value", "graphMode": "none", "justifyMode": "center", "orientation": "horizontal", "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false }, "textMode": "value_and_name" },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "time() - player_health_last_check_timestamp", "legendFormat": "{{ player }}", "refId": "A" } ],
      "title": "Player Health-Check Age",
      "type": "stat"
    },

    { "collapsed": false, "gridPos": { "h": 1, "w": 24, "x": 0, "y": 18 }, "id": 20, "panels": [], "title": "EN Scraper Provider Health (OurEnglish failover chain)", "type": "row" },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Per-provider × per-stage probe state (1=UP, 0=DOWN). Filtered by the $provider and $stage dropdowns. Stages: search, episodes, servers, stream, stream_segment.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "custom": { "fillOpacity": 80, "lineWidth": 0 },
          "mappings": [ { "options": { "0": { "color": "red", "text": "DOWN" }, "1": { "color": "green", "text": "UP" } }, "type": "value" } ],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "red", "value": null }, { "color": "green", "value": 1 } ] }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 24, "x": 0, "y": 19 },
      "id": 21,
      "options": { "alignValue": "center", "mergeValues": true, "rowHeight": 0.8, "showValue": "never" },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "provider_health_up{provider=~\"$provider\", stage=~\"$stage\"}", "legendFormat": "{{ provider }}/{{ stage }}", "refId": "A" } ],
      "title": "Provider × Stage Up",
      "type": "state-timeline"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Provider enabled state (1=Enabled, 0=Degraded). Degraded providers come from SCRAPER_DEGRADED_PROVIDERS.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [ { "options": { "0": { "color": "red", "text": "DEGRADED" }, "1": { "color": "green", "text": "ENABLED" } }, "type": "value" } ],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "red", "value": null }, { "color": "green", "value": 1 } ] }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 8, "x": 0, "y": 27 },
      "id": 22,
      "options": { "colorMode": "background", "graphMode": "none", "justifyMode": "auto", "orientation": "horizontal", "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false }, "textMode": "value_and_name" },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "provider_enabled{provider=~\"$provider\"}", "legendFormat": "{{ provider }}", "refId": "A" } ],
      "title": "Provider Enabled / Degraded",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "EN scraper failover edges (from→to) over the last hour. Not $provider-filtered (from/to are distinct labels).",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "axisBorderShow": false, "drawStyle": "bars", "fillOpacity": 60, "lineWidth": 1, "showPoints": "never", "stacking": { "mode": "normal", "group": "A" } },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 16, "x": 8, "y": 27 },
      "id": 23,
      "options": { "legend": { "calcs": ["sum"], "displayMode": "table", "placement": "right" }, "tooltip": { "mode": "multi", "sort": "desc" } },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "sum by (from, to) (increase(parser_fallback_total[1h]))", "legendFormat": "{{ from }} → {{ to }}", "refId": "A" } ],
      "title": "Provider Fallbacks (1h)",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Seconds since each EN scraper provider's probe last ticked.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "green", "value": null }, { "color": "orange", "value": 600 }, { "color": "red", "value": 1800 } ] },
          "unit": "s"
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 24, "x": 0, "y": 35 },
      "id": 24,
      "options": { "colorMode": "value", "graphMode": "none", "justifyMode": "center", "orientation": "horizontal", "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false }, "textMode": "value_and_name" },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "time() - provider_probe_last_tick_timestamp{provider=~\"$provider\"}", "legendFormat": "{{ provider }}", "refId": "A" } ],
      "title": "Probe Last Tick (by provider)",
      "type": "stat"
    },

    { "collapsed": false, "gridPos": { "h": 1, "w": 24, "x": 0, "y": 39 }, "id": 30, "panels": [], "title": "EN Playability Canary", "type": "row" },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Daily playability canary pass/fail counts per provider over 24h. Filtered by $provider.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "axisBorderShow": false, "axisCenteredZero": false, "axisColorMode": "text", "axisLabel": "", "axisPlacement": "auto", "fillOpacity": 80, "gradientMode": "none", "hideFrom": { "legend": false, "tooltip": false, "viz": false }, "lineWidth": 1, "scaleDistribution": { "type": "linear" }, "thresholdsStyle": { "mode": "off" } },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "green", "value": null } ] },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 40 },
      "id": 31,
      "options": { "barRadius": 0, "barWidth": 0.85, "fullHighlight": false, "groupWidth": 0.7, "legend": { "calcs": ["sum"], "displayMode": "table", "placement": "right", "showLegend": true }, "orientation": "auto", "showValue": "auto", "stacking": "normal", "tooltip": { "mode": "multi", "sort": "desc" }, "xTickLabelRotation": 0, "xTickLabelSpacing": 0 },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "sum by (provider, result) (increase(playability_canary_runs_total{provider=~\"$provider\"}[24h]))", "format": "time_series", "legendFormat": "{{ provider }} — {{ result }}", "refId": "A" } ],
      "title": "Pass / Fail per Provider (24h)",
      "type": "barchart"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Failure-reason breakdown for canary failures over 24h. Filtered by $provider.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "axisBorderShow": false, "axisCenteredZero": false, "axisColorMode": "text", "axisLabel": "", "axisPlacement": "auto", "fillOpacity": 80, "gradientMode": "none", "hideFrom": { "legend": false, "tooltip": false, "viz": false }, "lineWidth": 1, "scaleDistribution": { "type": "linear" }, "thresholdsStyle": { "mode": "off" } },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "green", "value": null } ] },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 40 },
      "id": 32,
      "options": { "barRadius": 0, "barWidth": 0.85, "fullHighlight": false, "groupWidth": 0.7, "legend": { "calcs": ["sum"], "displayMode": "table", "placement": "right", "showLegend": true }, "orientation": "auto", "showValue": "auto", "stacking": "normal", "tooltip": { "mode": "multi", "sort": "desc" }, "xTickLabelRotation": 0, "xTickLabelSpacing": 0 },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "sum by (reason) (increase(playability_canary_runs_total{result=\"fail\", provider=~\"$provider\"}[24h]))", "format": "time_series", "legendFormat": "{{ reason }}", "refId": "A" } ],
      "title": "Failure Reason Breakdown (24h)",
      "type": "barchart"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Timestamp of the last successful canary run.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "red", "value": null }, { "color": "green", "value": 1 } ] },
          "unit": "dateTimeFromNow"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 8, "x": 0, "y": 48 },
      "id": 33,
      "options": { "colorMode": "value", "graphMode": "none", "justifyMode": "center", "orientation": "auto", "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false }, "textMode": "value" },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "scheduler_job_last_success_timestamp{job=\"scraper_playability_canary\"}", "legendFormat": "Last Canary Run", "refId": "A" } ],
      "title": "Last Canary Run",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "description": "Top 10 failing (provider, server, reason) tuples by 24h fail count. Filtered by $provider.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "custom": { "align": "auto", "cellOptions": { "type": "auto" }, "inspect": false },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [ { "color": "green", "value": null } ] },
          "unit": "short"
        },
        "overrides": [ { "matcher": { "id": "byName", "options": "Fail Count (24h)" }, "properties": [ { "id": "custom.cellOptions", "value": { "mode": "gradient", "type": "gauge" } }, { "id": "custom.width", "value": 200 } ] } ]
      },
      "gridPos": { "h": 8, "w": 16, "x": 8, "y": 48 },
      "id": 34,
      "options": { "cellHeight": "sm", "footer": { "countRows": false, "fields": "", "reducer": ["sum"], "show": false }, "showHeader": true, "sortBy": [ { "desc": true, "displayName": "Fail Count (24h)" } ] },
      "pluginVersion": "10.3.3",
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "topk(10, sum by (provider, server, reason) (increase(playability_canary_runs_total{result=\"fail\", provider=~\"$provider\"}[24h])))", "format": "table", "instant": true, "legendFormat": "", "refId": "A" } ],
      "title": "Top Failing (provider, server, reason) Tuples",
      "transformations": [ { "id": "organize", "options": { "excludeByName": { "Time": true }, "indexByName": { "provider": 0, "server": 1, "reason": 2, "Value": 3 }, "renameByName": { "Value": "Fail Count (24h)" } } } ],
      "type": "table"
    }
  ],
  "schemaVersion": 39,
  "style": "dark",
  "tags": ["playback", "health", "scraper", "players"],
  "templating": {
    "list": [
      {
        "current": { "selected": false, "text": "Prometheus", "value": "PBFA97CFB590B2093" },
        "hide": 0,
        "includeAll": false,
        "name": "DS_PROMETHEUS",
        "options": [],
        "query": "prometheus",
        "type": "datasource"
      },
      {
        "current": { "selected": true, "text": ["All"], "value": ["$__all"] },
        "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
        "definition": "label_values(provider_health_up, provider)",
        "hide": 0,
        "includeAll": true,
        "multi": true,
        "name": "provider",
        "options": [],
        "query": { "qryType": 1, "query": "label_values(provider_health_up, provider)", "refId": "PrometheusVariableQueryEditor-VariableQuery" },
        "refresh": 2,
        "regex": "",
        "sort": 1,
        "type": "query"
      },
      {
        "current": { "selected": true, "text": ["All"], "value": ["$__all"] },
        "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
        "definition": "label_values(provider_health_up, stage)",
        "hide": 0,
        "includeAll": true,
        "multi": true,
        "name": "stage",
        "options": [],
        "query": { "qryType": 1, "query": "label_values(provider_health_up, stage)", "refId": "PrometheusVariableQueryEditor-VariableQuery" },
        "refresh": 2,
        "regex": "",
        "sort": 1,
        "type": "query"
      }
    ]
  },
  "refresh": "30s",
  "time": { "from": "now-6h", "to": "now" },
  "timepicker": {},
  "timezone": "",
  "title": "Playback / Health",
  "uid": "playback-health",
  "version": 1,
  "weekStart": ""
}
```

- [ ] **Step 2: Validate the JSON parses**

Run: `python3 -m json.tool docker/grafana/dashboards/playback-health.json > /dev/null && echo VALID`
Expected: `VALID` (no traceback).

- [ ] **Step 3: Assert dashboard identity + every target has a non-empty expr**

Run:
```bash
python3 - <<'PY'
import json
d = json.load(open("docker/grafana/dashboards/playback-health.json"))
assert d["uid"] == "playback-health", d["uid"]
assert d["title"] == "Playback / Health", d["title"]
assert d["schemaVersion"] == 39
panels = [p for p in d["panels"] if p.get("type") != "row"]
assert len(panels) == 17, f"expected 17 non-row panels, got {len(panels)}"
ids = [p["id"] for p in d["panels"]]
assert len(ids) == len(set(ids)), "duplicate panel ids"
for p in panels:
    for t in p.get("targets", []):
        assert t.get("expr", "").strip(), f"empty expr in panel {p['title']}"
varnames = {v["name"] for v in d["templating"]["list"]}
assert {"DS_PROMETHEUS", "provider", "stage"} <= varnames, varnames
print("OK: 17 panels, unique ids, all exprs non-empty, vars present")
PY
```
Expected: `OK: 14 panels, unique ids, all exprs non-empty, vars present`

- [ ] **Step 4: Commit**

```bash
git add docker/grafana/dashboards/playback-health.json
git commit -m "feat(grafana): add unified Playback / Health dashboard

Merges Player/Health + Scraper/Health + Scraper/Provider Health into a
single collapsible-row dashboard. Adds parser success-rate % and p95
latency (previously undashboarded) and a provider x stage health matrix
that recovers the provider label the old per-stage panels discarded.
Two template vars (\$provider, \$stage) filter the EN-scraper rows.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Delete the three superseded dashboards

**Files:**
- Delete: `docker/grafana/dashboards/player-health.json`
- Delete: `docker/grafana/dashboards/scraper-health.json`
- Delete: `infra/grafana/dashboards/scraper-provider-health.json`

- [ ] **Step 1: Remove the files**

```bash
git rm docker/grafana/dashboards/player-health.json \
       docker/grafana/dashboards/scraper-health.json \
       infra/grafana/dashboards/scraper-provider-health.json
```

- [ ] **Step 2: Confirm no other file references the old uids**

Run: `grep -rn "player-health\|scraper-health\|scraper-provider-health-canary" --include=*.json --include=*.yml --include=*.yaml docker infra deploy | grep -v "playback-health"`
Expected: no output (empty). If a Grafana alert or other dashboard links to those uids, surface it before proceeding — none is expected, but verify.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "chore(grafana): remove dashboards superseded by Playback / Health

player-health, scraper-health, and scraper-provider-health-canary are
now merged into the unified playback-health dashboard.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: Deploy and verify live

**Files:** none (deploy + smoke).

- [ ] **Step 1: Restart Grafana to pick up the provisioned change**

The dashboards are bind-mounted read-only and provisioned on startup; the cleanest reload is a restart (no rebuild — Grafana is a stock image).

Run: `make restart-grafana`
Expected: container restarts healthy. Then `make health` (or `docker compose -f docker/docker-compose.yml ps grafana`) shows grafana up.

- [ ] **Step 2: Confirm the new dashboard is provisioned and the old three are gone**

Grafana provisions from disk on startup; with `disableDeletion: false` the removed files should evict their dashboards. Verify via the Grafana HTTP API (admin/admin form-login fallback is enabled):

```bash
GRAFANA="http://localhost:3004"   # adjust to the mapped grafana port if different (see docker-compose ports)
AUTH="admin:${GRAFANA_ADMIN_PASSWORD:-admin}"
echo "== new =="; curl -s -u "$AUTH" "$GRAFANA/api/dashboards/uid/playback-health" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('dashboard',{}).get('title','MISSING'))"
for uid in player-health scraper-health scraper-provider-health-canary; do
  echo -n "== $uid => "; curl -s -o /dev/null -w "%{http_code}\n" -u "$AUTH" "$GRAFANA/api/dashboards/uid/$uid"
done
```
Expected: new prints `Playback / Health`; each old uid returns `404`. If an old uid still returns 200, delete it via `curl -X DELETE -u "$AUTH" "$GRAFANA/api/dashboards/uid/<uid>"` (its provisioning file is already gone, so it will not be recreated).

- [ ] **Step 3: Re-validate every panel's PromQL returns against live Prometheus**

Run:
```bash
P="http://localhost:9090/prometheus/api/v1/query"
check(){ n=$(curl -s --get "$P" --data-urlencode "query=$1" | python3 -c "import sys,json;d=json.load(sys.stdin);print(len(d.get('data',{}).get('result',[])) if d.get('status')=='success' else 'ERR')"); echo "[$n] $2"; }
check 'sum(player_health_up)' 'at-a-glance RU players'
check 'sum(provider_enabled)' 'at-a-glance providers enabled'
check 'sum(provider_health_up) / count(provider_health_up)' 'at-a-glance stage health'
check 'sum by (provider) (rate(parser_requests_total{status="success"}[5m])) / sum by (provider) (rate(parser_requests_total[5m]))' 'parser success rate'
check 'histogram_quantile(0.95, sum by (le, provider) (rate(parser_request_duration_seconds_bucket[5m])))' 'parser p95'
check 'provider_health_up{provider=~".+", stage=~".+"}' 'provider x stage'
check 'provider_enabled' 'provider enabled grid'
check 'sum by (from, to) (increase(parser_fallback_total[1h]))' 'fallbacks'
check 'sum by (provider, result) (increase(playability_canary_runs_total[24h]))' 'canary pass/fail'
```
Expected: each line prints a result count `[N]` with `N >= 1` (the canary line may be sparse but should be ≥1 given live `gogoanime/fail` data). Any `[0]` or `[ERR]` means the metric stopped emitting — investigate before declaring done. `$provider`/`$stage` are replaced by `.+` here to mimic the "All" selection.

- [ ] **Step 4: Browser smoke (DS-NF-06 standing rule — visual changes verified in a real browser)**

Open Grafana at `animeenigma.ru/admin/grafana` (admin SSO) or the local mapped port. Open **Playback / Health** and confirm:
  1. The 5 at-a-glance stats render with values (RU Players Up shows 2, EN Providers Enabled shows ~4).
  2. All three rows expand/collapse on click; collapsing stops their panels.
  3. The `$provider` and `$stage` dropdowns populate from live labels and filter Rows 2–3 (pick a single provider, confirm Row 2/3 narrow).
  4. The old three dashboards no longer appear in the dashboard list / search.

Capture nothing to commit; this is a manual confirmation gate.

---

## Task 4: Changelog, commit, push (after-update)

- [ ] **Step 1: Run the project after-update skill**

This is an admin/observability change. Invoke `/animeenigma-after-update`. It will: lint/build affected code (none here — JSON only), redeploy/restart Grafana if not already done, add a **Russian Trump-mode** changelog entry to `frontend/web/public/changelog.json` (brief, factual — "ОБЪЕДИНИЛИ три дашборда в один. ВЕЛИКОЛЕПНО."), commit, and push.

Expected: changelog updated, all changes committed with the standard co-authors, pushed to remote.

---

## Notes for the executor

- **No Go/TS/code changes** — this is provisioned Grafana JSON only. There is no unit-test suite to run; verification is JSON-lint + live PromQL + browser smoke.
- **Datasource UID** `PBFA97CFB590B2093` is copied from the two existing dashboards; it is the provisioned Prometheus datasource. Do not invent a different UID.
- **Why `$provider`/`$stage` only touch the EN rows:** the catalog-parser `provider` label set (allanime, anime18, animelib, hanime, kodik) is disjoint from the scraper `provider` set; filtering Row 1 by `$provider` (sourced from `provider_health_up`) would blank it. This is intentional and documented in the panel descriptions.
- **Mapped Grafana port:** confirm the host port from `docker/docker-compose.yml` (the `grafana` service `ports:` block) before running Task 3 curls — the examples assume `3004`.
```
