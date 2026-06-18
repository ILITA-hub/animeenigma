# Playback-Health Roster Unification — Phase 4: Dashboard Rebuild

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild `docker/grafana/dashboards/playback-health.json` with a fresh 6-section information architecture anchored by a Postgres-sourced roster table, fix the `$provider` template variable so every per-provider panel covers all 13 providers (not the 2 currently probed), and retire the bespoke ae panel — so the dashboard finally presents one uniform, roster-driven view.

**Architecture:** Author a new dashboard JSON with a fresh section layout (Overview / Roster / Provider Health / Parser / Real-User Telemetry / HLS Proxy). The genuinely-new artifacts are authored fresh: the section row-headers, the new Postgres roster panel (`aenigma-postgres` datasource), the gridPos layout, and the rewired `$provider` template variable. Every RETAINED data panel's JSON object (queries, fieldConfig) is **carried verbatim** from the current file — re-deriving working PromQL/SQL adds risk with zero value; the rebuild is of the *information architecture*, not the proven query internals.

**Tech Stack:** Grafana provisioned dashboard JSON (schemaVersion 39), Prometheus + ClickHouse + Postgres datasources, `jq` for structural validation.

## Global Constraints

- **Effort/impact units:** UXΔ / CDI / MVQ only — never days/hours/sprints (`.planning/CONVENTIONS.md`).
- **PRESERVE datasource UIDs verbatim — non-negotiable:** Prometheus is referenced via the `${DS_PROMETHEUS}` template-variable indirection that binds to **UID `PBFA97CFB590B2093`** (14 alert rules depend on this exact UID — DO NOT inline a different UID or rename the var). Postgres = **`aenigma-postgres`** (type `postgres`). ClickHouse = **`aenigma-clickhouse`** (type `grafana-clickhouse-datasource`). All three are already provisioned in `docker/grafana/provisioning/datasources/datasources.yml`; this plan adds NO datasource.
- **Dashboard identity preserved:** `"uid": "playback-health"`, `"title": "Playback / Health"`, `"schemaVersion": 39`, `"refresh": "30s"`, `"graphTooltip": 1`. Keep the existing `tags`. (Changing `uid` would orphan saved links/alerts that target this dashboard.)
- **Carry retained panels verbatim:** copy each retained panel's JSON object (its `targets`/`rawSql`/`fieldConfig`/`options`) from the CURRENT `docker/grafana/dashboards/playback-health.json` unchanged. The ONLY edits allowed to a carried panel are its `gridPos` (new layout) and `id` (kept unique). Do NOT rewrite queries.
- **The `$provider` fix is the core bug:** its definition is currently `label_values(provider_health_up, provider)` — which yields only the 2 actively-probed providers (ae, kodik). Change BOTH its `definition` and its `query.query` to `label_values(provider_info, provider)` so it enumerates all 13 from the roster-reflection metric (Phase 2 makes `provider_info` cover all 13 across both Prometheus targets).
- **Retire exactly one panel:** the bespoke ae panel (current id 41, title "ae Resolutions (catalog) by status", query `http_requests_total{service="catalog", path="/api/anime/:id/ae"}`). It must NOT appear in the new file. No other panel's data is dropped.
- **Validation is structural, not TDD:** every task validates with `jq` (file parses; assertions on the panels/templating). The dashboard is deployed via provisioning (the file mounts into the grafana container); verification is a Grafana reload + API/HTTP checks. There is no unit-test framework for Grafana JSON — do NOT invent brittle ones.
- **Build the new file at a temp path, swap at the end:** author `docker/grafana/dashboards/playback-health.json.new`, validate it, and only `mv` it over the live file in the deploy task — so a half-built file never provisions.

**Spec:** `docs/superpowers/specs/2026-06-17-playback-health-provider-roster-unification-design.md` (§3 dashboard). **Phases 1–3 (shipped):** roster table + `scraper_operated`; `provider_info`/`provider_enabled` cover all 13; EN-chain parser latency + ae parser/probe live.

**Locked decisions (brainstorming 2026-06-18):** roster table = Postgres datasource (full columns incl `group`+`scraper_operated`); scope = full ground-up IA rebuild (carrying proven panel internals per the Architecture note above).

---

## Target Information Architecture

Row-header panels (`"type": "row"`) separate the sections; data panels are laid out left-to-right within each. Approximate `y` offsets grow downward; each implementer recomputes `gridPos.y` so sections don't overlap (a row header is `h:1`; stats `h:4`; timeseries/table `h:7–8`).

| Section (row header) | Panels (carried from current id → new section) |
|---|---|
| **0. Overview** (stats) | Providers Enabled/Total (2), Stage Health (3), Canary Last Run age (4), Fallbacks (5) |
| **1. Roster** | **NEW** Provider Roster (Postgres `stream_providers`); Provider Management table (21, `provider_info`) |
| **2. Provider Health** | Provider×Stage Up (22, `$provider`/`$stage`); Probe Last Tick (25, `$provider`); Connect/Disconnect History (26, `$provider`) |
| **3. Parser** | Parser Success % by provider (12); Parser p95 by provider (13); Fallbacked from (23); Stopped fallbacking at (24); Top Failing canary tuples (31, `$provider`) |
| **4. Real-User Telemetry (ClickHouse)** | Reached-playback rate (46); Resolve success % (47); p95 resolve latency (48); Stalls (49); Top failing provider/error_kind (50) |
| **5. HLS Proxy** | Requests/s on-prem vs external (42); Egress (43); Latency p50/p95 (44) |
| **RETIRED** | ae Resolutions bespoke panel (41) — removed |

---

### Task 1: New dashboard skeleton — meta, templating (fixed `$provider`), section headers, roster panel

**Files:**
- Create: `docker/grafana/dashboards/playback-health.json.new`

**Interfaces:**
- Produces: a valid dashboard JSON with the corrected templating, the 6 section row-headers, and the new Postgres roster panel — but not yet the carried data panels (Task 2).

- [ ] **Step 1: Author the skeleton file**

Create `docker/grafana/dashboards/playback-health.json.new`. Use this exact skeleton. The `templating.list` carries the DS_PROMETHEUS datasource var and the `stage` var **verbatim from the current file** (copy them from the current `playback-health.json` templating block), and the `provider` var with the **FIXED** definition shown here:

```json
{
  "annotations": { "list": [] },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
  "id": null,
  "links": [],
  "description": "Unified playback health, roster-driven: every panel keyed off the stream_providers roster. Roster table (Postgres) anchors who exists + their lifecycle (status/group/scraper_operated); per-provider health, parser latency, real-user telemetry and HLS-proxy panels cover all 13 providers. $provider enumerates the full roster via provider_info.",
  "panels": [
    { "collapsed": false, "gridPos": { "h": 1, "w": 24, "x": 0, "y": 0 }, "id": 100, "panels": [], "title": "Overview", "type": "row" },
    { "collapsed": false, "gridPos": { "h": 1, "w": 24, "x": 0, "y": 5 }, "id": 101, "panels": [], "title": "Roster", "type": "row" },
    {
      "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
      "description": "The authoritative provider roster from the catalog stream_providers table — every stream source (ae + EN scraper chain + adult + legacy players) with its lifecycle status, group, and whether the scraper operates it. This is the single source of truth the rest of the dashboard's metrics pivot around.",
      "fieldConfig": {
        "defaults": { "custom": { "align": "left" }, "mappings": [] },
        "overrides": [
          { "matcher": { "id": "byName", "options": "status" },
            "properties": [ { "id": "mappings", "value": [
              { "type": "value", "options": { "enabled": { "color": "green", "index": 0, "text": "enabled" } } },
              { "type": "value", "options": { "degraded": { "color": "orange", "index": 1, "text": "degraded" } } },
              { "type": "value", "options": { "disabled": { "color": "red", "index": 2, "text": "disabled" } } }
            ] }, { "id": "custom.cellOptions", "value": { "type": "color-text" } } ] }
        ]
      },
      "gridPos": { "h": 9, "w": 14, "x": 0, "y": 6 },
      "id": 102,
      "options": { "footer": { "show": false }, "showHeader": true },
      "targets": [ {
        "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
        "format": "table",
        "rawQuery": true,
        "rawSql": "SELECT name, status, \"group\", scraper_operated, reason FROM stream_providers ORDER BY scraper_operated DESC, \"group\", name",
        "refId": "A"
      } ],
      "title": "Provider Roster (stream_providers)",
      "type": "table"
    },
    { "collapsed": false, "gridPos": { "h": 1, "w": 24, "x": 0, "y": 15 }, "id": 103, "panels": [], "title": "Provider Health", "type": "row" },
    { "collapsed": false, "gridPos": { "h": 1, "w": 24, "x": 0, "y": 16 }, "id": 104, "panels": [], "title": "Parser", "type": "row" },
    { "collapsed": false, "gridPos": { "h": 1, "w": 24, "x": 0, "y": 17 }, "id": 105, "panels": [], "title": "Real-User Player Telemetry (ClickHouse)", "type": "row" },
    { "collapsed": false, "gridPos": { "h": 1, "w": 24, "x": 0, "y": 18 }, "id": 106, "panels": [], "title": "HLS Proxy", "type": "row" }
  ],
  "schemaVersion": 39,
  "tags": ["playback", "health", "providers"],
  "templating": {
    "list": [
      <COPY the DS_PROMETHEUS datasource variable object verbatim from the current playback-health.json templating.list>,
      {
        "current": {},
        "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
        "definition": "label_values(provider_info, provider)",
        "includeAll": true,
        "multi": true,
        "name": "provider",
        "options": [],
        "query": { "qryType": 1, "query": "label_values(provider_info, provider)", "refId": "PrometheusVariableQueryEditor-VariableQuery" },
        "refresh": 2,
        "regex": "",
        "type": "query"
      },
      <COPY the `stage` query variable object verbatim from the current playback-health.json templating.list>
    ]
  },
  "time": { "from": "now-6h", "to": "now" },
  "timepicker": {},
  "timezone": "",
  "refresh": "30s",
  "title": "Playback / Health",
  "uid": "playback-health",
  "version": 1,
  "weekStart": ""
}
```

> The two `<COPY …>` placeholders are NOT optional content to invent — open the current `docker/grafana/dashboards/playback-health.json`, find the `DS_PROMETHEUS` and `stage` variable objects inside `templating.list` (around the file's tail), and paste them verbatim. Confirm the `tags` array matches the current file's tags (replace the example `["playback","health","providers"]` with the current file's actual `tags` value).

- [ ] **Step 2: Validate the skeleton parses and is correctly wired**

Run:
```bash
cd /data/animeenigma
jq empty docker/grafana/dashboards/playback-health.json.new && echo "JSON OK"
jq -r '.templating.list[] | select(.name=="provider") | .query.query' docker/grafana/dashboards/playback-health.json.new
jq -r '.panels[] | select(.id==102) | .targets[0].rawSql' docker/grafana/dashboards/playback-health.json.new
jq -r '[.panels[] | select(.type=="row") | .title] | join(", ")' docker/grafana/dashboards/playback-health.json.new
jq -r '.uid, .schemaVersion' docker/grafana/dashboards/playback-health.json.new
```
Expected: `JSON OK`; the provider query prints `label_values(provider_info, provider)`; the roster rawSql prints the `SELECT … FROM stream_providers …`; the row titles print `Overview, Roster, Provider Health, Parser, Real-User Player Telemetry (ClickHouse), HLS Proxy`; uid `playback-health`, schemaVersion `39`.

- [ ] **Step 3: Commit**

```bash
git add docker/grafana/dashboards/playback-health.json.new
git commit docker/grafana/dashboards/playback-health.json.new -m "feat(grafana): playback-health rebuild skeleton — fixed \$provider + Postgres roster panel

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 2: Carry the retained data panels into their sections; drop the bespoke ae panel

**Files:**
- Modify: `docker/grafana/dashboards/playback-health.json.new`

**Interfaces:**
- Consumes: the skeleton (Task 1) + the current `playback-health.json` as the verbatim source for each carried panel object.
- Produces: the complete new dashboard — all retained panels placed under their section row-headers with recomputed `gridPos`, panel 41 absent.

- [ ] **Step 1: Copy each retained panel object verbatim, set its new gridPos, insert under the matching row-header**

For each panel id in the IA table below, copy its FULL JSON object from the current `docker/grafana/dashboards/playback-health.json` (do not alter `datasource`, `targets`, `fieldConfig`, `options`, `description`, `title`, `type`) and insert it into `playback-health.json.new`'s `panels` array immediately after its section's row-header, with the new `gridPos` shown. Keep each panel's existing `id` (they're already unique and don't collide with the 100–106 row/roster ids). DO NOT copy panel id 41.

Place panels so each sits below its row header (recompute `y` as you go; the values below are a valid non-overlapping layout):

| New section header (id) | Carried panel id | New gridPos `{h,w,x,y}` |
|---|---|---|
| Overview (100) | 2 | `{4,6,0,1}` |
| Overview (100) | 3 | `{4,6,6,1}` |
| Overview (100) | 4 | `{4,6,12,1}` |
| Overview (100) | 5 | `{4,6,18,1}` |
| Roster (101) — roster panel 102 already at `{9,14,0,6}` | 21 | `{9,10,14,6}` |
| Provider Health (103, move header to `y:16`) | 22 | `{8,24,0,17}` |
| Provider Health | 25 | `{6,12,0,25}` |
| Provider Health | 26 | `{6,12,12,25}` |
| Parser (104, move header to `y:31`) | 12 | `{7,12,0,32}` |
| Parser | 13 | `{7,12,12,32}` |
| Parser | 23 | `{7,8,0,39}` |
| Parser | 24 | `{7,8,8,39}` |
| Parser | 31 | `{7,8,16,39}` |
| Real-User Telemetry (105, move header to `y:46`) | 46 | `{8,12,0,47}` |
| Real-User Telemetry | 47 | `{8,12,12,47}` |
| Real-User Telemetry | 48 | `{8,8,0,55}` |
| Real-User Telemetry | 49 | `{8,8,8,55}` |
| Real-User Telemetry | 50 | `{8,8,16,55}` |
| HLS Proxy (106, move header to `y:63`) | 42 | `{7,8,0,64}` |
| HLS Proxy | 43 | `{7,8,8,64}` |
| HLS Proxy | 44 | `{7,8,16,64}` |

Update the row-header `gridPos.y` values from the Task-1 placeholders to: Overview `y:0`, Roster `y:5`, Provider Health `y:16`, Parser `y:31`, Real-User Telemetry `y:46`, HLS Proxy `y:63`. (The roster panel 102 stays `{9,14,0,6}`.)

> The retained panels keep using `${DS_PROMETHEUS}` / `aenigma-clickhouse` exactly as in the source — that is the whole point of carrying them verbatim. The `$provider`-driven panels (22, 25, 26, 31) need NO query edit: they already use `provider=~"$provider"`, and `$provider` now resolves to all 13 (Task 1).

- [ ] **Step 2: Validate the complete dashboard structurally**

Run:
```bash
cd /data/animeenigma
F=docker/grafana/dashboards/playback-health.json.new
jq empty "$F" && echo "JSON OK"
echo "--- bespoke ae panel must be GONE (expect empty) ---"
grep -c 'api/anime/:id/ae' "$F" || echo "0 (good)"
echo "--- data panel ids present (expect 2 3 4 5 12 13 21 22 23 24 25 26 31 42 43 44 46 47 48 49 50 + 102) ---"
jq -r '[.panels[] | select(.type!="row") | .id] | sort | join(" ")' "$F"
echo "--- panel 41 must be absent (expect: absent) ---"
jq -e '.panels[] | select(.id==41)' "$F" >/dev/null && echo "PRESENT (BUG)" || echo "absent (good)"
echo "--- all Prometheus panels still use the DS_PROMETHEUS indirection (no inlined raw UID) ---"
grep -c 'PBFA97CFB590B2093' "$F" || echo "0 (good — UID only via \${DS_PROMETHEUS})"
echo "--- clickhouse panels still bound to aenigma-clickhouse (expect 5) ---"
jq '[.panels[] | select(.datasource.uid=="aenigma-clickhouse")] | length' "$F"
echo "--- postgres roster panel present (expect 1) ---"
jq '[.panels[] | select(.datasource.uid=="aenigma-postgres")] | length' "$F"
echo "--- no overlapping gridPos within a column (manual sanity) ---"
jq -r '.panels[] | "\(.gridPos.y)\t\(.gridPos.x)\t\(.gridPos.h)\t\(.id)\t\(.title)"' "$F" | sort -n
```
Expected: `JSON OK`; ae bespoke grep `0`; the data-panel id list is exactly `2 3 4 5 12 13 21 22 23 24 25 26 31 42 43 44 46 47 48 49 50 102`; panel 41 `absent`; raw UID grep `0`; clickhouse count `5`; postgres count `1`; the gridPos dump shows ascending non-overlapping rows.

- [ ] **Step 3: Commit**

```bash
git commit docker/grafana/dashboards/playback-health.json.new -m "feat(grafana): carry retained panels into rebuilt sections; retire bespoke ae panel

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 3: Swap in the new dashboard + deploy + verify in Grafana

**Files:**
- Delete+replace: `docker/grafana/dashboards/playback-health.json` (overwritten by `.new`)

- [ ] **Step 1: Swap the new file over the live file**

```bash
cd /data/animeenigma
jq empty docker/grafana/dashboards/playback-health.json.new   # final parse gate
git mv docker/grafana/dashboards/playback-health.json.new docker/grafana/dashboards/playback-health.json
jq -r '.uid' docker/grafana/dashboards/playback-health.json    # expect playback-health
```

- [ ] **Step 2: Reload Grafana provisioning**

The dashboard JSON is mounted into the grafana container and provisioned from disk. Restart grafana to reload (config-only change, no rebuild):

```bash
make restart-grafana
sleep 8
```

- [ ] **Step 3: Verify the dashboard loaded without provisioning errors**

```bash
docker logs animeenigma-grafana 2>&1 | grep -iE 'playback-health|provision' | grep -iE 'error|fail|invalid' && echo "PROVISION ERROR ABOVE" || echo "no provisioning errors"
```
Expected: `no provisioning errors`. (If an error prints, the JSON is structurally invalid to Grafana — fix and re-swap.)

- [ ] **Step 4: Verify the dashboard + its panels via the Grafana API**

Grafana admin is reachable on the host. Confirm the dashboard is served and the roster/templating are correct:
```bash
# adjust creds/port if needed — default provisioned admin
GRAFANA=http://localhost:3000
curl -s "$GRAFANA/api/dashboards/uid/playback-health" -u admin:admin -o /tmp/dash.json -w "HTTP %{http_code}\n"
jq -r '.dashboard.templating.list[] | select(.name=="provider") | .query.query' /tmp/dash.json   # label_values(provider_info, provider)
jq -r '[.dashboard.panels[] | select(.type=="row") | .title] | join(", ")' /tmp/dash.json          # 6 section titles
jq -e '.dashboard.panels[] | select(.id==41)' /tmp/dash.json >/dev/null && echo "ae panel STILL PRESENT (BUG)" || echo "ae bespoke panel retired (good)"
```
Expected: HTTP 200; provider query = `label_values(provider_info, provider)`; the 6 section titles; ae bespoke panel retired.

- [ ] **Step 5: Confirm the roster table + `$provider` resolve to live data**

```bash
GRAFANA=http://localhost:3000
# $provider should now enumerate all 13 (was 2). Query the var's values via the datasource proxy:
curl -s "$GRAFANA/api/datasources/proxy/uid/PBFA97CFB590B2093/api/v1/series?match[]=provider_info" -u admin:admin \
  | jq -r '[.data[].provider] | sort | unique | length'   # expect 13 (or 12 if animekai flag off)
```
Expected: 13 (note if 12 under `SCRAPER_ANIMEKAI_ENABLED=false`). This proves the `$provider` dropdown now covers the full roster. The Postgres roster panel renders from `stream_providers` (verified live in Phase 1/2 to hold 13 rows) — a non-200 here would indicate the `aenigma-postgres` datasource path, not this dashboard.

- [ ] **Step 6: (Opt-in) Chrome smoke**

Per project norm (Chrome smoke is opt-in), do NOT auto-run a browser check. If the owner wants visual confirmation, offer to open the dashboard at `https://animeenigma.ru/admin/grafana/d/playback-health/playback-health` and screenshot each section. Otherwise the API checks above are sufficient.

- [ ] **Step 7: Commit the swap**

```bash
git commit docker/grafana/dashboards/playback-health.json -m "feat(grafana): deploy rebuilt roster-driven playback-health dashboard

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

## Self-Review (Phase 4)

**Spec coverage (§3 dashboard + locked decisions):**
- Every panel roster-driven; `$provider` from `provider_info` (all 13) → Task 1 (var fix) + Task 2 (the 4 `$provider` panels inherit it). ✓
- Unified roster/status table from the Postgres datasource with `group`+`scraper_operated` → Task 1 (panel 102). ✓
- Retire the bespoke ae panel → Task 2 (panel 41 dropped; validated by grep + jq). ✓
- Fresh 6-section IA → Task 1 (row headers) + Task 2 (layout). ✓
- Preserve Prometheus UID indirection + ClickHouse/Postgres UIDs + dashboard uid → Global Constraints + Task 2 validation (raw-UID grep = 0, clickhouse=5, postgres=1). ✓
- Carry retained queries verbatim (no PromQL/SQL re-derivation) → Architecture note + Task 2 Step 1 instruction + the verbatim-copy constraint. ✓

**Placeholder scan:** The two `<COPY …>` markers in Task 1 are explicit verbatim-copy instructions pointing at the authoritative source object (DS_PROMETHEUS + stage vars), with a guard note — not vague placeholders. The new Postgres panel + provider var are fully authored. Every validation step has exact commands + expected output. ✓

**Type/identifier consistency:** datasource UIDs (`PBFA97CFB590B2093` via `${DS_PROMETHEUS}`, `aenigma-postgres`, `aenigma-clickhouse`) used identically across tasks; row-header ids 100–106 and roster panel 102 don't collide with carried data-panel ids (2,3,4,5,12,13,21,22,23,24,25,26,31,42,43,44,46,47,48,49,50); the data-panel id list in Task 2 Step 2's assertion matches the IA table exactly. The `$provider` definition string is identical in Task 1's panel JSON and its validation. ✓
