# Phase 19: Grafana dashboard rebuild (Kraken) - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, infra hygiene / dashboard refactor)

<domain>
## Phase Boundary

Tidy up Grafana dashboard inventory. Closes UA-116, UA-117, UA-118, UA-119, UA-120 + Tier E #12.

**Inventory:**
- `docker/grafana/dashboards/animeenigma-services.json` (6 rows)
- `docker/grafana/dashboards/content-preferences.json` (2 rows)
- `docker/grafana/dashboards/player-health.json` (3 rows)
- `docker/grafana/dashboards/preference-resolution.json` (4 rows)
- `docker/grafana/dashboards/rec-engine.json` (0 rows — uses panel ordering)
- `docker/grafana/dashboards/scraper-health.json` (0 rows)
- `docker/grafana/dashboards/watch-activity.json` (2 rows)
- `infra/grafana/dashboards/scraper-provider-health.json` (separate location)

**Cleanup targets:**
- **UA-116 — Consistent naming.** Standardize on `<area>/<scope>` pattern in title field (e.g. "Recs / CTR", "Player / Health", "Scraper / Provider").
- **UA-117 — Empty rows removed.** Any row with zero panels under it gets deleted.
- **UA-118 — Row numbering normalized.** Numbered rows ("Row 1", "Row 2") replaced with descriptive titles.
- **UA-119 — Panel-type appropriateness.** Time-series for rates/trends; Stat for single-value totals; Gauge for ratios/percentages. Audit each panel for fit.
- **UA-120 — Time-range defaults.** Set sensible defaults per dashboard:
  - Live ops (player-health, scraper-health) → `now-1h` to `now`
  - Aggregates (content-preferences, watch-activity) → `now-7d` to `now`
  - Service overview (animeenigma-services) → `now-6h` to `now`
  - Rec engine → `now-24h` to `now`

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**Naming convention:**
- Dashboard title format: `Area / Scope`. Examples:
  - "AnimeEnigma Services" → "Services / Overview"
  - "Player Health" → "Player / Health"
  - "Rec engine" → "Recs / Engine"
  - "Content Preferences" → "Recs / Content Preferences"
  - "Preference Resolution" → "Player / Preference Resolution"
  - "Scraper Health" → "Scraper / Health"
  - "Watch Activity" → "Player / Watch Activity"
  - "Image Proxy" → "Services / Image Proxy"
  - "Scraper Provider Health" → "Scraper / Provider Health"
- Panel titles keep their current names — they're already mostly descriptive.

**Empty row removal:**
- Iterate dashboards with `>0` rows. Identify rows with `collapsed: true` AND no panel children, OR any row title containing literally `"---"` or `""`. Remove them.

**Row numbering:**
- Replace any `"Row 1"`, `"Row 2"` etc. with a meaningful title (e.g. "Overview", "Detail breakdown", "Trends").

**Panel-type review:**
- Existing inventory: 38 timeseries + 27 stat + 1 gauge. Keep current types as-is unless a panel uses `timeseries` for a single-value query (`sum()` without time bucketing) — should be `stat`. Conversely, a stat panel with a time-bucketed query (`rate(...)` over `$__interval`) should be `timeseries`.
- This is per-panel surgery; do a pass and fix obvious mismatches. Plan budget ~1-2 hour-equivalents of edits. If a panel's intent is ambiguous, leave it alone — false-negative is safer than wrong type.

**Time-range defaults:**
- Each dashboard JSON has a `time: { from: "now-Xh", to: "now" }` or similar. Set per the table above. Also set `refresh: "30s"` for live ops dashboards (now-1h), `refresh: "5m"` for aggregates.

**Phase 1 dependency:**
- Phase 19 depends on Phase 1 (anonymous-Admin removed). Verified — Phase 1 complete.

### Locked from ROADMAP

- 8 dashboard files in scope. No new dashboards added. No panels deleted (only row containers if empty).
- Phase 20 polish can fine-tune the panel-type pass.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- All dashboards are stored as JSON in `docker/grafana/dashboards/` and `infra/grafana/dashboards/`.
- Provisioning config in `docker/grafana/provisioning/` auto-loads dashboards on Grafana startup. No restart needed beyond `docker compose restart grafana`.

### Established Patterns

- Each dashboard's `title` field is the top-level identifier.
- Rows are panel objects with `type: "row"` and a `collapsed` bool.
- Time range is `time: { from, to }`. Refresh is `refresh: "30s"` etc.

### Integration Points

- After JSON edits, run `docker compose restart grafana` (NOT `make redeploy-grafana` since Grafana is a sidecar, not a service we build).
- Validate JSON parse-ability with `jq . docker/grafana/dashboards/*.json`.

</code_context>

<specifics>
## Specific Ideas

- The Area/Scope prefix discipline makes the dashboard list much easier to scan in Grafana's sidebar.
- For the panel-type pass, focus on dashboards with the most panels (animeenigma-services + watch-activity have the most).
- The rec-engine dashboard has 0 rows — its 38 panels are in a flat list. That's fine as-is; the title rename is the only touch.

</specifics>

<deferred>
## Deferred Ideas

- Adding new metrics (e.g. P99 latency per service) — out of scope.
- Splitting overloaded dashboards into multiple — out of scope.
- Dashboard variables/templating — out of scope.

</deferred>
