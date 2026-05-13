---
id: 23-02
phase: 23
plan: "02"
type: execute
wave: 2
depends_on:
  - 23-01
files_modified:
  - infra/grafana/dashboards/scraper-provider-health.json
  - infra/grafana/dashboards/README.md
  - docker/grafana/provisioning/dashboards/dashboards.yml
  - docker/docker-compose.yml
requirements:
  - SCRAPER-HEAL-14
autonomous: true
tags: [grafana, dashboard, observability, scraper, canary]

must_haves:
  truths:
    - "A new file `infra/grafana/dashboards/scraper-provider-health.json` exists and is valid JSON parseable by `jq`"
    - "The dashboard contains 4 named panels: 'Pass / Fail per Provider/Server (24h)', 'Failure Reason Breakdown (24h)', 'Last Canary Run', 'Top Failing (provider, server, reason) Tuples'"
    - "Pass/Fail panel uses a Prometheus query against `playability_canary_runs_total` aggregated by `provider` + `server` + `result` over a 24h window (stacked bar visualization)"
    - "Reason Breakdown panel groups by `reason` label and shows only `result=\"fail\"` rows over 24h"
    - "Last Canary Run panel shows a timestamp / time-since stat sourced from `scheduler_job_last_success_timestamp{job=\"scraper_playability_canary\"}` (the existing gauge written by JobService.Start)"
    - "Top Failing Tuples panel is a table sorted DESC by fail count, top N rows visible, with columns provider / server / reason / count, filtered to a 24h window"
    - "Dashboard JSON conforms to Grafana 10.3.3 schema (same `pluginVersion` as the existing scraper-health.json) and parses without error when posted to Grafana's `/api/dashboards/db`"
    - "The dashboard is auto-loaded into the running Grafana container via the existing file-provisioning provider (mounted directory)"
    - "An infra/grafana/dashboards/README.md exists explaining the directory's purpose and its relationship to docker/grafana/dashboards/"
  artifacts:
    - path: infra/grafana/dashboards/scraper-provider-health.json
      provides: "Grafana dashboard JSON with 4 panels for canary observability + 24h window selector + datasource templating ($DS_PROMETHEUS)"
      contains: "playability_canary_runs_total"
    - path: infra/grafana/dashboards/README.md
      provides: "Brief doc explaining infra/grafana/dashboards/ is the canonical source-of-truth for canary-related dashboards; copy-deployed into docker/grafana/dashboards/ by docker-compose volume mount; production Kubernetes provisioning is via deploy/kustomize"
      contains: "scraper-provider-health.json"
    - path: docker/grafana/provisioning/dashboards/dashboards.yml
      provides: "Adds a SECOND search path so Grafana auto-loads from BOTH `/var/lib/grafana/dashboards` (existing) AND `/var/lib/grafana/dashboards/infra` (new — points at the infra/ dir mount). Backward-compatible."
      contains: "/var/lib/grafana/dashboards/infra"
    - path: docker/docker-compose.yml
      provides: "grafana service mounts the new infra/grafana/dashboards/ directory at /var/lib/grafana/dashboards/infra so the new dashboard is visible at start"
      contains: "infra/grafana/dashboards"
  key_links:
    - from: infra/grafana/dashboards/scraper-provider-health.json
      to: prometheus → playability_canary_runs_total (counter from Plan 23-01)
      via: "PromQL target on each panel"
      pattern: "playability_canary_runs_total"
    - from: infra/grafana/dashboards/scraper-provider-health.json
      to: prometheus → scheduler_job_last_success_timestamp{job=\"scraper_playability_canary\"}
      via: "Last Canary Run stat panel"
      pattern: "scheduler_job_last_success_timestamp"
    - from: docker/docker-compose.yml grafana service
      to: infra/grafana/dashboards/
      via: "volume mount `./../infra/grafana/dashboards:/var/lib/grafana/dashboards/infra:ro`"
      pattern: "infra/grafana/dashboards"
---

<objective>
Ship the Grafana dashboard that visualizes the canary's `playability_canary_runs_total` counter (from Plan 23-01) and the existing `scheduler_job_last_success_timestamp` gauge. Four panels: stacked pass/fail counts per provider/server (24h), failure reason breakdown, last canary run timestamp, and a top-N failing-tuple table. The dashboard lives under `infra/grafana/dashboards/` (a NEW directory — separate from `docker/grafana/dashboards/` which holds the existing 7 dashboards) and is mounted into the running Grafana container via docker-compose so it auto-loads with no Grafana restart required. SCRAPER-HEAL-14.

Purpose: The canary emits a stream of pass/fail tuples; without a dashboard, the operator has no easy way to see "which provider, which server, which reason" is regressing across a 24h window. The Top Failing Tuples table is the highest-value panel — when paged about an alert from Plan 23-03, the operator opens this table and immediately sees which (provider, server, reason) triple is responsible.

Output:
- `infra/grafana/dashboards/scraper-provider-health.json` — production-ready Grafana dashboard JSON (Grafana 10.3.3 schema), datasource-variable-templated so it works in both dev (`${DS_PROMETHEUS}`) and prod.
- README.md documenting the new directory + its relationship to the existing docker/grafana/dashboards/.
- docker-compose grafana service mounts the new directory; dashboards.yml provider config picks it up.

Why a new directory rather than reusing `docker/grafana/dashboards/`: the existing `docker/grafana/dashboards/` is part of the dev compose stack only. Per the milestone's spec (file `infra/grafana/dashboards/scraper-provider-health.json`), v3.1 dashboards live under `infra/` so production Kubernetes provisioning (deploy/kustomize/base/monitoring/grafana/) can pull from the same source-of-truth without copying across two unrelated directories. README.md captures this convention.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/STATE.md
@.planning/phases/23-self-maintenance-loop/23-CONTEXT.md
@docs/plans/2026-05-13-scraper-self-healing-spec.md
@CLAUDE.md

<interfaces>
<!-- Existing scraper-health dashboard structure (pattern reference): -->
<!-- docker/grafana/dashboards/scraper-health.json — Grafana 10.3.3 schema, datasource = { type: prometheus, uid: ${DS_PROMETHEUS} }, panels use the standard "stat", "timeseries", "barchart", "table" types. -->

<!-- Counter from Plan 23-01 (dependency — but no Go-level dependency; this plan only references the metric NAME): -->

```
playability_canary_runs_total{provider, server, result, reason, anime_slot}
  provider:   "gogoanime"
  server:     "vibeplayer" | "streamhg" | "earnvids" | "_unreachable"
  result:     "pass" | "fail"
  reason:     one of libs/streamprobe.Reason values
  anime_slot: anchor_frieren | anchor_one_piece | recent_1 | recent_2 | recent_3
```

<!-- Existing scheduler gauge already wired in libs/metrics/scheduler.go: -->

```
scheduler_job_last_success_timestamp{job}    # gauge, Unix ts
```

After Plan 23-01 lands, `job="scraper_playability_canary"` becomes a valid label value when JobService.Start's AddFunc closure calls `SchedulerJobLastSuccess.WithLabelValues("scraper_playability_canary").SetToCurrentTime()`.

<!-- Existing provisioning config (docker/grafana/provisioning/dashboards/dashboards.yml) currently declares one provider at /var/lib/grafana/dashboards. The new infra/ directory is mounted as a sibling /var/lib/grafana/dashboards/infra and picked up by adding a `searchPath` extension or a SECOND provider entry pointing at the new path. -->

<!-- PromQL ready-to-paste queries (use as panel targets — adjust legendFormat to your taste): -->

```promql
# Panel 1: Pass / Fail per Provider/Server (24h) — stacked bar
sum by (provider, server, result) (increase(playability_canary_runs_total[24h]))

# Panel 2: Failure Reason Breakdown (24h) — stacked bar by reason
sum by (reason) (increase(playability_canary_runs_total{result="fail"}[24h]))

# Panel 3: Last Canary Run — stat (single value, format = "Date & Time" or "From now")
scheduler_job_last_success_timestamp{job="scraper_playability_canary"}

# Panel 4: Top Failing Tuples (24h) — table, sort by Value DESC
topk(10, sum by (provider, server, reason) (increase(playability_canary_runs_total{result="fail"}[24h])))
```
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Create infra/grafana/dashboards/ directory + scraper-provider-health.json with 4 panels + README.md</name>
  <files>infra/grafana/dashboards/scraper-provider-health.json, infra/grafana/dashboards/README.md</files>
  <read_first>
    - docker/grafana/dashboards/scraper-health.json (full file — copy the top-level structure: schemaVersion, version, time, templating $DS_PROMETHEUS variable, refresh, tags. The 4 new panels follow the panel-object pattern present in this reference file.)
    - docker/grafana/dashboards/player-health.json (skim — find a `table`-type panel for the Top Failing Tuples reference; topk + transform.organize sort pattern lives here)
    - docs/plans/2026-05-13-scraper-self-healing-spec.md §4.3.c lines 160-163 ("Grafana panel: 'Scraper Provider Health' — Stacked bar per provider/server: pass/fail counts per 24h, Reason breakdown when failing")
    - .planning/phases/23-self-maintenance-loop/23-CONTEXT.md domain bullet #6 (4 panel names spelled out)
  </read_first>
  <behavior>
    - `jq -e . infra/grafana/dashboards/scraper-provider-health.json` exits 0 (file is valid JSON).
    - `jq -r '.panels[].title' infra/grafana/dashboards/scraper-provider-health.json` returns exactly these 4 lines in any order:
      ```
      Pass / Fail per Provider/Server (24h)
      Failure Reason Breakdown (24h)
      Last Canary Run
      Top Failing (provider, server, reason) Tuples
      ```
    - `jq -r '.panels[].targets[].expr' infra/grafana/dashboards/scraper-provider-health.json | sort -u` contains all 4 query strings shown in the `<interfaces>` block above (modulo trivial whitespace).
    - `jq -r '.title' infra/grafana/dashboards/scraper-provider-health.json` returns `"Scraper Provider Health (Canary)"`.
    - `jq -r '.tags[]' infra/grafana/dashboards/scraper-provider-health.json` includes `scraper` and `canary`.
    - `jq -r '.templating.list[].name' infra/grafana/dashboards/scraper-provider-health.json` includes `DS_PROMETHEUS`.
    - `jq -r '.refresh' infra/grafana/dashboards/scraper-provider-health.json` returns a non-empty refresh interval (e.g. `"1m"`).
    - `jq -r '.schemaVersion' infra/grafana/dashboards/scraper-provider-health.json` returns 38 or 39 (matches existing dashboards' schemaVersion).
    - `jq -r '.panels[] | select(.title == "Last Canary Run") | .targets[0].expr' infra/grafana/dashboards/scraper-provider-health.json` matches `scheduler_job_last_success_timestamp{job="scraper_playability_canary"}`.
    - `jq -r '.panels[] | select(.title == "Top Failing (provider, server, reason) Tuples") | .type' infra/grafana/dashboards/scraper-provider-health.json` returns `"table"`.
    - `jq -e '.panels[] | select(.title == "Pass / Fail per Provider/Server (24h)") | .fieldConfig.defaults.color.mode' infra/grafana/dashboards/scraper-provider-health.json` returns a color mode (`"palette-classic"` or similar — verifies the panel has thresholds/color config, not raw defaults).
    - infra/grafana/dashboards/README.md exists; `grep -c "scraper-provider-health.json" infra/grafana/dashboards/README.md` returns ≥ 1; `grep -c "deploy/kustomize" infra/grafana/dashboards/README.md` returns ≥ 1 (explains the production-vs-dev relationship).
  </behavior>
  <action>
    1. **Create directory** `infra/grafana/dashboards/` (use `mkdir -p`).
    2. **Create infra/grafana/dashboards/scraper-provider-health.json** as a complete Grafana 10.3.3 dashboard. Use docker/grafana/dashboards/scraper-health.json as the template — extract its top-level scaffolding (`annotations`, `editable: true`, `fiscalYearStartMonth: 0`, `graphTooltip: 0`, `id: null`, `links: []`, `liveNow: false`, `schemaVersion: 38`, `style: "dark"`, `tags: ["scraper", "canary", "self-healing"]`, `timezone: "browser"`, `time: { from: "now-24h", to: "now" }`, `refresh: "1m"`, `templating.list = [{ name: "DS_PROMETHEUS", type: "datasource", query: "prometheus", current: {} }]`, `title: "Scraper Provider Health (Canary)"`, `uid: "scraper-provider-health-canary"`, `version: 1`, `weekStart: ""`). Then add the four panels with these layouts (gridPos coords are non-overlapping):
       - Panel 1 (id 1, gridPos `{h:8, w:12, x:0, y:0}`, type `"barchart"`, title `"Pass / Fail per Provider/Server (24h)"`): target expr exactly `sum by (provider, server, result) (increase(playability_canary_runs_total[24h]))`, legendFormat `"{{provider}} / {{server}} — {{result}}"`, stacking mode `"normal"`, fieldConfig.defaults.color.mode `"palette-classic"`, options.legend.displayMode `"table"`, options.legend.placement `"right"`. Datasource `{ type: "prometheus", uid: "${DS_PROMETHEUS}" }` on the panel and on each target.
       - Panel 2 (id 2, gridPos `{h:8, w:12, x:12, y:0}`, type `"barchart"`, title `"Failure Reason Breakdown (24h)"`): target expr `sum by (reason) (increase(playability_canary_runs_total{result="fail"}[24h]))`, legendFormat `"{{reason}}"`, same stacking + palette config.
       - Panel 3 (id 3, gridPos `{h:4, w:8, x:0, y:8}`, type `"stat"`, title `"Last Canary Run"`): target expr `scheduler_job_last_success_timestamp{job="scraper_playability_canary"}`, fieldConfig.defaults.unit `"dateTimeFromNow"` (Grafana converts a Unix-seconds value to "5m ago" automatically), options.reduceOptions.calcs `["lastNotNull"]`, options.textMode `"value"`, options.colorMode `"value"`, thresholds: `mode: "absolute"`, `steps: [{ color: "red", value: null }, { color: "green", value: <last 26h cutoff trick or just leave green if you cannot template "now()" — use a single green step; the alert layer handles staleness via absent_over_time>]`.
       - Panel 4 (id 4, gridPos `{h:8, w:16, x:8, y:8}`, type `"table"`, title `"Top Failing (provider, server, reason) Tuples"`): target expr `topk(10, sum by (provider, server, reason) (increase(playability_canary_runs_total{result="fail"}[24h])))`, format `"table"`, instant `true`. Transformations `[{ id: "organize", options: { excludeByName: { Time: true }, indexByName: { provider: 0, server: 1, reason: 2, Value: 3 }, renameByName: { Value: "Fail Count (24h)" } } }]`. fieldConfig.overrides empty.
    3. **Create infra/grafana/dashboards/README.md** — short explainer (≤ 30 lines) containing:
       - Why this directory exists (source-of-truth for v3.1 self-healing dashboards shared by dev + prod).
       - Naming convention (`<area>-<subject>.json`).
       - Relationship to `docker/grafana/dashboards/` (dev mount; older dashboards predate this convention; new ones land here).
       - Relationship to `deploy/kustomize/base/monitoring/grafana/configmap-dashboards.yaml` (production K8s — will be updated in a future deploy plan; out of scope here).
       - Mention of `scraper-provider-health.json` as the first entry.
    4. **Validate JSON locally** by running `jq -e . infra/grafana/dashboards/scraper-provider-health.json > /dev/null` (must exit 0).
  </action>
  <verify>
    <automated>jq -e . /data/animeenigma/infra/grafana/dashboards/scraper-provider-health.json > /dev/null && [ "$(jq -r '.panels | length' /data/animeenigma/infra/grafana/dashboards/scraper-provider-health.json)" = "4" ] && jq -r '.panels[].title' /data/animeenigma/infra/grafana/dashboards/scraper-provider-health.json | sort | diff - <(printf 'Failure Reason Breakdown (24h)\nLast Canary Run\nPass / Fail per Provider/Server (24h)\nTop Failing (provider, server, reason) Tuples\n') && grep -c "scraper-provider-health.json" /data/animeenigma/infra/grafana/dashboards/README.md</automated>
  </verify>
  <done>infra/grafana/dashboards/scraper-provider-health.json exists, is valid JSON, has exactly the 4 expected panel titles, references the correct metrics, and uses the ${DS_PROMETHEUS} templating variable. README.md exists and documents the new convention.</done>
</task>

<task type="auto">
  <name>Task 2: Wire the new directory into Grafana's file-provisioning + docker-compose mount</name>
  <files>docker/grafana/provisioning/dashboards/dashboards.yml, docker/docker-compose.yml</files>
  <read_first>
    - docker/grafana/provisioning/dashboards/dashboards.yml (full file — current single provider config; add a SECOND provider entry rather than mutating the first to avoid breaking the existing 7 dashboards)
    - docker/docker-compose.yml (search for `grafana:` service block — list the current `volumes:` entries; the new mount is appended)
    - Pattern: the existing scraper-health.json is already loaded from `/var/lib/grafana/dashboards`; the new path `/var/lib/grafana/dashboards/infra` is a subdirectory inside the same volume root, but Grafana's file provisioner does NOT recurse — a second provider entry is required.
  </read_first>
  <behavior>
    - After this task, `docker compose -f docker/docker-compose.yml config` (the validator) exits 0.
    - `grep -E "infra/grafana/dashboards" docker/docker-compose.yml` returns the new mount line in the grafana service block (path on host → `:/var/lib/grafana/dashboards/infra:ro`).
    - `grep -c "name:" docker/grafana/provisioning/dashboards/dashboards.yml` returns 2 (one for the existing provider, one for the new `infra` provider).
    - The new provider's `options.path` is `/var/lib/grafana/dashboards/infra`.
    - When the grafana container restarts (out of scope for this plan — the after-update step in 23-03 redeploys), the new dashboard becomes visible at `http://localhost:3000/d/scraper-provider-health-canary/scraper-provider-health-canary`. Manual verification deferred.
  </behavior>
  <action>
    1. **Edit docker/grafana/provisioning/dashboards/dashboards.yml** — append a second provider entry (do NOT modify the first):
       ```yaml
       apiVersion: 1

       providers:
         - name: 'default'
           orgId: 1
           folder: ''
           type: file
           disableDeletion: false
           editable: true
           options:
             path: /var/lib/grafana/dashboards

         - name: 'infra-self-healing'
           orgId: 1
           folder: 'Self-Healing'
           type: file
           disableDeletion: false
           editable: true
           options:
             path: /var/lib/grafana/dashboards/infra
       ```
       The new `folder: 'Self-Healing'` segregates the v3.1 dashboards in the Grafana UI without altering the existing layout.
    2. **Edit docker/docker-compose.yml** — find the grafana service block (search for `grafana:` near the top; note that ports 3000 + provisioning volumes are already declared). Append to the existing `volumes:` list:
       ```yaml
         - ../infra/grafana/dashboards:/var/lib/grafana/dashboards/infra:ro
       ```
       Use the existing relative-path convention (other volumes are referenced as `../<path>` relative to `docker/`).
    3. **Sanity-check**: `docker compose -f docker/docker-compose.yml config | grep -A 2 "/var/lib/grafana/dashboards/infra"` should print the new mount line.
  </action>
  <verify>
    <automated>cd /data/animeenigma && docker compose -f docker/docker-compose.yml config > /dev/null && grep -c "name:" docker/grafana/provisioning/dashboards/dashboards.yml | grep -v '^[01]$' && grep -c "infra/grafana/dashboards" docker/docker-compose.yml</automated>
  </verify>
  <done>docker-compose config validates; dashboards.yml has the new `infra-self-healing` provider entry; docker-compose.yml mounts `../infra/grafana/dashboards` into the grafana container at `/var/lib/grafana/dashboards/infra`. Live verification (panel rendering with real data) happens after Plan 23-03 redeploys.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Grafana → Prometheus | Existing, unchanged. The new dashboard adds queries to the same Prometheus instance over the same in-cluster network. No new boundary. |
| Operator's browser → Grafana | Existing auth posture (admin login) unchanged. The new dashboard is visible to anyone with Grafana access — same as the existing 7 dashboards. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-23-06 | I (Information disclosure) | Top Failing Tuples panel exposes label values | accept | Label values are bounded normalized identifiers (provider name, server extractor name, reason enum). No PII, no user IDs, no internal URLs. Same risk surface as the existing scraper-health.json dashboard. |
| T-23-07 | D (DoS) | High-cardinality dashboard query overloads Prometheus | accept | `topk(10, ...)` panel bounds the result set. `increase(...[24h])` is a single range query per panel — well within Prometheus capacity. Cardinality of the underlying metric is bounded at ~210 series by design (see Plan 23-01 threat T-23-04). |
| T-23-08 | T (Tampering) | Dashboard JSON has `editable: true` so operators could overwrite the file's in-Grafana state | accept | Operators editing in-UI affects only the Grafana DB; the source-of-truth JSON on disk is unaffected. Production K8s provisioning will use the on-disk file. This matches the existing dashboard convention. |

All ASVS L1: read-only data flow, no secrets in panels, no user input.
</threat_model>

<verification>
- `jq -e . infra/grafana/dashboards/scraper-provider-health.json > /dev/null` exits 0.
- `jq -r '.panels | length' infra/grafana/dashboards/scraper-provider-health.json` returns 4.
- `jq -r '.uid' infra/grafana/dashboards/scraper-provider-health.json` returns `scraper-provider-health-canary`.
- `docker compose -f docker/docker-compose.yml config > /dev/null` exits 0.
- `grep -c "infra-self-healing" docker/grafana/provisioning/dashboards/dashboards.yml` returns 1.
- `grep -c "infra/grafana/dashboards" docker/docker-compose.yml` returns 1.
- README.md exists at infra/grafana/dashboards/README.md and references both the JSON file and the kustomize relationship.
</verification>

<success_criteria>
- Both tasks pass their `<verify>` commands.
- Phase 23 ROADMAP Success Criteria #3: dashboard shows stacked pass/fail per provider/server (24h), reason breakdown panel, last-canary-run timestamp, top failing tuples — VERIFIED via jq queries against the new file.
- File location matches the spec exactly: `infra/grafana/dashboards/scraper-provider-health.json`.
- Dashboard auto-loaded on next Grafana restart (no human action required to install).
</success_criteria>

<output>
After completion, create `.planning/phases/23-self-maintenance-loop/23-02-SUMMARY.md`.
</output>
