# Phase 11: Observability & Prediction - Pattern Map

**Mapped:** 2026-06-17
**Files analyzed:** 5 (2 created/heavily-modified core + 3 wiring touch-points)
**Analogs found:** 4 strong / 5 (OBS-05 Prometheus-table panel has NO exact analog — see "No Analog Found")

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `infra/grafana/dashboards/library.json` (APPEND 6 panels) | config (dashboard) | transform (PromQL→panel) | same file, panels `id:1`/`id:6` (barchart), `id:2`/`id:5` (stat), `id:7` (piechart) | exact (self-analog) |
| `services/scheduler/internal/jobs/autocache_prediction.go` (NEW) | service (cron job) | batch (DISTINCT-join → gauge) | `services/scheduler/internal/jobs/autocache_logic_a.go` | exact (sibling job) |
| `libs/metrics/autocache.go` (NEW) OR a var in existing `libs/metrics/scheduler.go` | utility (metric def) | — | `libs/metrics/scheduler.go` `SchedulerJobLastSuccess` + `libs/metrics/provider.go` `ProviderHealthUp` | exact |
| `services/scheduler/internal/config/config.go` (MODIFY) | config | — | `AUTOCACHE_ACTIVE_WATCHER_DAYS` field (Phase 9) | exact |
| `services/scheduler/internal/service/job.go` + `cmd/scheduler-api/main.go` (MODIFY) | service (wiring) | — | `autocacheLogicAJob` registration block | exact |

---

## Pattern Assignments

### `services/scheduler/internal/jobs/autocache_prediction.go` (service, batch → gauge)

**Analog:** `services/scheduler/internal/jobs/autocache_logic_a.go` (read in full — 186 lines)

This job is a **leaner sibling** of Logic A: it runs the SAME shared-DB join but instead of firing per-row HTTP demands it only COUNTS rows and sets a gauge. Copy the struct/constructor/Run skeleton and the join; **drop the entire `fireDemand` HTTP path** (no `bytes`/`io`/`net/http`/`encoding/json` imports needed).

**Struct + constructor pattern** (`autocache_logic_a.go:43-63`) — clone, swap `client *http.Client`/`libraryURL` for `avgRawEpBytes int64`, keep `db`/`activeWatcherDays`/`log`:
```go
type AutocacheLogicAJob struct {
	db                *gorm.DB
	client            *http.Client
	libraryURL        string
	activeWatcherDays int
	log               *logger.Logger
}

func NewAutocacheLogicAJob(db *gorm.DB, libraryURL string, activeWatcherDays int, log *logger.Logger) *AutocacheLogicAJob {
	return &AutocacheLogicAJob{
		db:                db,
		client:            &http.Client{Timeout: logicADemandTimeout},
		libraryURL:        libraryURL,
		activeWatcherDays: activeWatcherDays,
		log:               log,
	}
}
```

**Go-computed recency cutoff (DB-portable — Postgres+SQLite)** (`autocache_logic_a.go:93`):
```go
cutoff := time.Now().AddDate(0, 0, -j.activeWatcherDays)
```

**The reusable DISTINCT join** (`autocache_logic_a.go:100-116`). For the prediction job you need TWO counts off variants of this join:

- **`component="ongoing"`** = COUNT of distinct ongoing anime with ≥1 active JP-audio watcher = the EXACT Logic A join, wrapped in a count. Cleanest reuse: keep the inner `SELECT DISTINCT a.shikimori_id ... WHERE al.status='watching' AND a.status='ongoing' AND (wh.player IN ('ae','raw') OR wh.language='ja') AND al.updated_at > ?` and wrap `SELECT count(*) FROM ( <that> ) t` — OR scan the rows like Logic A does and take `len(rows)` after the `EpisodesAired>0 && ShikimoriID!=""` filter (matches Logic A's "qualifying" definition exactly, reusing the same projection).
- **`component="nextep"`** = COUNT of DISTINCT anime with an active JP-audio *watching* watcher in last `active_watcher_days` — i.e. the SAME join WITHOUT the `a.status='ongoing'` clause (spec §7: "distinct JP-combo watching-anime active in last 30d"). Drop `AND a.status='ongoing'` and `count(DISTINCT a.shikimori_id)`.

Reference join (copy verbatim, then derive the two variants):
```go
const q = `
	SELECT DISTINCT
	    a.shikimori_id   AS shikimori_id,
	    a.episodes_aired AS episodes_aired
	FROM watch_history wh
	JOIN anime_list al ON al.user_id = wh.user_id AND al.anime_id = wh.anime_id
	JOIN animes a ON a.id = wh.anime_id
	WHERE al.status = 'watching'
	  AND a.status = 'ongoing'
	  AND (wh.player IN ('ae', 'raw') OR wh.language = 'ja')
	  AND al.updated_at > ?
`
var rows []logicARow
if err := j.db.WithContext(ctx).Raw(q, cutoff).Scan(&rows).Error; err != nil {
	return fmt.Errorf("logic A enumeration join: %w", err)
}
```

> Recommendation for the planner: use `db.Raw("SELECT count(*) FROM (<inner>) t", cutoff).Scan(&n)` for each count rather than scanning full rows — but if you prefer parity with Logic A's row-filter semantics for `ongoing`, scan rows and `len()`. Either is fine; both are DB-portable. Keep the `gorm:"column:..."` scan struct pattern from `logicARow` (`autocache_logic_a.go:67-70`) if scanning a count into a named field.

**Run → set gauge (replace Logic A's fan-out loop)**. After computing `ongoingCount` and `nextepCount`, multiply by `avgRawEpBytes` and set the gauge — there is NO per-row HTTP, so `Run` returns an error ONLY on a join failure (same contract as Logic A `autocache_logic_a.go:85`/`114-116`):
```go
metrics.AutocachePredictedBytes.WithLabelValues("ongoing").Set(float64(ongoingCount) * float64(j.avgRawEpBytes))
metrics.AutocachePredictedBytes.WithLabelValues("nextep").Set(float64(nextepCount) * float64(j.avgRawEpBytes))
```

**Structured logging on completion** (mirror `autocache_logic_a.go:139-146`): log `ongoing_count`, `nextep_count`, `avg_raw_ep_bytes`.

**Test seam:** clone the in-memory-SQLite seeding pattern the Logic A test uses (`autocache_logic_a_test.go` — referenced in 09-04-SUMMARY:76). Seed `watch_history`/`anime_list`/`animes`, run the job against a fresh registry, assert the two gauge values via `testutil.ToFloat64(...)`. The recency-cutoff-as-param is what makes this SQLite-testable (don't reintroduce Postgres `interval` syntax).

---

### `libs/metrics/autocache.go` (NEW metric def) — the prediction GaugeVec

**Finding for the planner:** the scheduler does NOT yet have a `library_autocache_predicted_bytes` GaugeVec. You must add a NEW `promauto.NewGaugeVec`. The scheduler already exposes `/metrics` via the shared default registry (`promauto` auto-registers into `prometheus.DefaultRegisterer`; `transport/router.go:36-37` serves `metrics.Handler()`), so a new `promauto.NewGaugeVec` declared in `libs/metrics` is scraped automatically — no registry plumbing needed.

**Analog:** `libs/metrics/scheduler.go:29-35` (the `SchedulerJobLastSuccess` GaugeVec — closest in the same package) and `libs/metrics/provider.go:96-102` (`ProviderEnabled`, a single-label gauge). Clone this exact shape:
```go
// libs/metrics/scheduler.go:29
SchedulerJobLastSuccess = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "scheduler_job_last_success_timestamp",
		Help: "Unix timestamp of last successful scheduler job execution",
	},
	[]string{"job"},
)
```

New gauge to add (cardinality MUST stay `{component}`-only → 2 series; CONTEXT pitfall — do NOT label per-anime):
```go
AutocachePredictedBytes = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "library_autocache_predicted_bytes",
		Help: "Predicted RAW-pool bytes needed per heuristic component (ongoing|nextep). Emitted by the scheduler so OBS-05 can join it with the library-exposed library_autocache_budget_bytes.",
	},
	[]string{"component"},
)
```
> Name keeps the `library_autocache_` prefix DELIBERATELY (CONTEXT §pitfall + spec §7) even though it's emitted by scheduler, so Grafana's OBS-05 table can union it with library's `library_autocache_budget_bytes` in one query. Add to existing `libs/metrics/scheduler.go` var block OR a new `autocache.go` in the same package — either auto-registers.

---

### `services/scheduler/internal/config/config.go` (MODIFY) — `AUTOCACHE_AVG_RAW_EP_BYTES`

**Analog:** the Phase-9 `AutocacheActiveWatcherDays` field (`config.go:98`) + its `Load` wiring (`config.go:145`). Clone the field, the doc-comment-as-env-mirror style (`config.go:75-99`), and add the new cron field.

**Field block** (add near `config.go:96-99`):
```go
AutocacheLogicACron        string
LibraryInternalURL         string
AutocacheActiveWatcherDays int
// Phase 11 (v4.1 observability) — daily storage-need prediction job.
AutocachePredictionCron string // default "0 4 * * *" (or "@daily")
AutocacheAvgRawEpBytes  int64  // default 1288490188 (~1.2 GiB) — mirror of spec §7 avg_raw_ep_size
```

**Load wiring** (mirror `config.go:143-145`):
```go
AutocacheLogicACron:        getEnv("AUTOCACHE_LOGIC_A_CRON", "*/20 * * * *"),
LibraryInternalURL:         getEnv("LIBRARY_INTERNAL_URL", getEnv("LIBRARY_SERVICE_URL", "http://library:8089")),
AutocacheActiveWatcherDays: getEnvInt("AUTOCACHE_ACTIVE_WATCHER_DAYS", 30),
// Phase 11 — prediction job.
AutocachePredictionCron: getEnv("AUTOCACHE_PREDICTION_CRON", "0 4 * * *"),
AutocacheAvgRawEpBytes:  getEnvInt64("AUTOCACHE_AVG_RAW_EP_BYTES", 1288490188),
```

> **GAP for the planner:** `config.go` has `getEnv` (string) + `getEnvInt` (int) helpers ONLY (`config.go:150-164`). `avg_raw_ep_bytes` ≈ 1.2 GiB overflows nothing as int but is conceptually `int64` (bytes). Either (a) add a tiny `getEnvInt64` helper (clone `getEnvInt`, swap `strconv.Atoi`→`strconv.ParseInt(val,10,64)`), or (b) store as `int` and cast at use — prefer (a) for correctness on 32-bit, consistent with the byte-quantity convention elsewhere.

---

### `services/scheduler/internal/service/job.go` + `cmd/scheduler-api/main.go` (MODIFY) — registration + DI

**Analog:** the `autocacheLogicAJob` registration end-to-end (Phase 9). Clone every touch-point; this is a mechanical mirror.

**1. JobService struct field + last-run tracker** (`job.go:22` + `job.go:31`):
```go
autocacheLogicAJob          *jobs.AutocacheLogicAJob
// add: autocachePredictionJob *jobs.AutocachePredictionJob
lastAutocacheLogicARun      time.Time
// add: lastAutocachePredictionRun time.Time
```

**2. Constructor param + assignment** (`job.go:42` + `job.go:54`) — add the new job to `NewJobService` signature and assignment.

**3. Start() registration — the nil-guarded metrics-wrap block** (`job.go:231-252`, the Logic A block — copy verbatim, swap label `"autocache_logic_a"` → `"autocache_prediction"`, the job field, and the last-run field):
```go
if s.autocacheLogicAJob != nil {
	_, err = s.cron.AddFunc(autocacheLogicACron, func() {
		ctx := context.Background()
		s.log.Info("starting scheduled autocache Logic A sweep")
		start := time.Now()
		if err := s.autocacheLogicAJob.Run(ctx); err != nil {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("autocache_logic_a", "error").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("autocache_logic_a").Observe(time.Since(start).Seconds())
			s.log.Errorw("autocache Logic A sweep failed", "error", err)
		} else {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("autocache_logic_a", "success").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("autocache_logic_a").Observe(time.Since(start).Seconds())
			metrics.SchedulerJobLastSuccess.WithLabelValues("autocache_logic_a").SetToCurrentTime()
			s.lastAutocacheLogicARun = time.Now()
			s.log.Info("autocache Logic A sweep completed successfully")
		}
	})
	if err != nil {
		return err
	}
	s.log.Info("registered job: autocache_logic_a")
}
```
> The prediction job has no external dependency (it only reads the shared DB the scheduler already owns), so it can be **non-nil-guarded** (always registered, like shikimori/cleanup `job.go:62-101`) OR keep the nil-guard for symmetry. Recommend NON-guarded (always on) since there's no optional URL — but then construct it unconditionally in `main.go` (no `if cfg…!=""`). Either way add the `Start()` signature param + thread the cron expr.

**4. GetStatus entry** (`job.go:354-356`):
```go
"autocache_logic_a": map[string]interface{}{
	"last_run": s.lastAutocacheLogicARun,
},
```

**5. main.go DI** (`main.go:126-136` + `main.go:148`). Unlike Logic A (`main.go:130-133`, nil when no library URL), the prediction job has no URL → construct unconditionally:
```go
// Phase 09 — Logic A (nil when no library URL):
var autocacheLogicAJob *jobs.AutocacheLogicAJob
if cfg.Jobs.LibraryInternalURL != "" {
	autocacheLogicAJob = jobs.NewAutocacheLogicAJob(db.DB, cfg.Jobs.LibraryInternalURL, cfg.Jobs.AutocacheActiveWatcherDays, log)
}
// Phase 11 — prediction (always on; reads shared DB only):
autocachePredictionJob := jobs.NewAutocachePredictionJob(db.DB, cfg.Jobs.AutocacheActiveWatcherDays, cfg.Jobs.AutocacheAvgRawEpBytes, log)
```
Then add to `NewJobService(...)` (`main.go:136`) and to `jobService.Start(... cfg.Jobs.AutocachePredictionCron)` (`main.go:140-149`).

---

### `infra/grafana/dashboards/library.json` (APPEND 6 panels — config, transform)

**Analog:** the file itself (read in full, 377 lines). The existing 7 panels are the per-type templates. **APPEND POINT: line 333** (the closing `}` of panel `id:7`, the last element of the `"panels": [...]` array which closes at **line 334**). Insert a comma after panel 7's closing `}` (line 333) and add the new panel objects before the `]` on line 334. Do NOT touch panels `id:1..7` (CONTEXT: keep the existing 7 intact).

**Datasource uid to reuse (every panel + every target):** the templated `${DS_PROMETHEUS}` var (defined `library.json:339-350`; concrete value `PBFA97CFB590B2093`). Copy verbatim:
```json
"datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }
```

**Unique `id`s:** existing max is `7` → new panels use `8..13`. **gridPos `y`:** existing panels occupy y=0/8/16 (3 rows of h:8). New panels start at **`y:24`** and step by 8 (24, 32, 40 …) with `x` 0/12 (or 0/6/12/18 for stat-width). Non-overlapping is mandatory (CONTEXT pitfall).

Also append the new panel `type`s to `__requires` (`library.json:369-376`) if a new type is introduced — only `"table"` is new (barchart/stat/timeseries/piechart already listed). Add:
```json
{ "type": "panel", "id": "table", "name": "Table", "version": "" }
```

#### OBS-01 — storage bytes_used vs budget (stacked) + episodes
**Analog panel: `id:1` "Job status counts" barchart (`library.json:10-69`)** for the stacked shape; or `id:4` timeseries (`library.json:148-199`) for a stacked-time view. Use a **stacked barchart** (clone `id:1`, set `"unit": "bytes"` not `"short"`, two targets):
```json
"targets": [
  { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
    "expr": "library_autocache_bytes_used", "legendFormat": "{{source}}/{{freshness}}", "refId": "A" },
  { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
    "expr": "library_autocache_budget_bytes", "legendFormat": "budget", "refId": "B" }
]
```
Second panel for episodes: clone `id:1` barchart, `expr: "library_autocache_episodes"`, `legendFormat: "{{source}}/{{freshness}}"`, `"unit": "short"`.

#### OBS-02 — preload hit-rate %
**Analog panel: `id:2`/`id:5` stat (`library.json:70-110` / `200-240`).** Clone the `stat` panel, `"unit": "percentunit"`, single target:
```json
"expr": "sum(rate(library_autocache_serve_total{result=\"hit\"}[1h])) / sum(rate(library_autocache_serve_total[1h]))"
```
(Use thresholds steps like `id:2` for color: red→yellow→green.)

#### OBS-03 — evicted + rejected (increase over window)
**Analog panel: `id:6` "Enqueue rejects" barchart (`library.json:241-300`)** — it's the EXACT shape (`sum by (reason) (increase(...[24h]))`). Clone twice (or one panel, two targets):
```json
"expr": "sum by (source) (increase(library_autocache_evicted_total[24h]))",  "legendFormat": "evicted/{{source}}"
"expr": "sum by (reason) (increase(library_autocache_rejected_total[24h]))", "legendFormat": "rejected/{{reason}}"
```

#### OBS-04 — downloads by trigger
**Analog panel: `id:1` barchart (`library.json:10-69`).** Clone, `expr` with the trigger label:
```json
"expr": "sum by (trigger) (increase(library_autocache_downloads_total[24h]))", "legendFormat": "{{trigger}}"
```

#### OBS-05 — prediction TABLE (predicted_bytes{component} + total + budget column)
**No exact analog (see "No Analog Found").** Build a `"type": "table"` panel with the prometheus datasource. Shape (3 component rows — ongoing, nextep, total — + a budget reference). Two practical patterns:

- **Simplest (2 component rows + budget row), instant query, `format:"table"`:**
```json
{
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "title": "Storage-need prediction vs budget",
  "type": "table",
  "gridPos": { "h": 8, "w": 24, "x": 0, "y": 40 },
  "id": 13,
  "fieldConfig": {
    "defaults": { "unit": "bytes" },
    "overrides": []
  },
  "options": { "showHeader": true },
  "pluginVersion": "10.3.3",
  "targets": [
    { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "expr": "library_autocache_predicted_bytes", "format": "table", "instant": true, "legendFormat": "{{component}}", "refId": "A" },
    { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "expr": "sum(library_autocache_predicted_bytes)", "format": "table", "instant": true, "legendFormat": "total", "refId": "B" },
    { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "expr": "library_autocache_budget_bytes", "format": "table", "instant": true, "legendFormat": "budget", "refId": "C" }
  ]
}
```
- The "total + budget as a COLUMN" framing (so each row shows predicted vs budget side-by-side) needs Grafana **transformations** (`"transformations": [...]` array on the panel) — a `merge` + `organize`/`labelsToFields` to fold the `component` label into rows and surface `budget` as a shared column. The instant-query table above (component rows + a budget row) satisfies OBS-05's "compared against budget_bytes" minimally; the column-join is a polish the planner can add via a `merge` transform if cheap. CONTEXT explicitly prefers the COARSE version for v1 (ongoing/nextep/total) — do not add per-anime rows (high cardinality).
- Append `{ "type": "panel", "id": "table", "name": "Table", "version": "" }` to `__requires` (`library.json:369-376`).

**Validation gate (CONTEXT pitfall — no live Grafana smoke required):** after editing, `jq . infra/grafana/dashboards/library.json >/dev/null` (JSON-parse) + assert unique panel ids + non-overlapping gridPos. That's the practical schema-sanity check.

---

## Shared Patterns

### Cron job registration + metrics wrap (scheduler-wide)
**Source:** `services/scheduler/internal/service/job.go:62-101` (shikimori) / `231-252` (Logic A)
**Apply to:** the new prediction job
Every scheduler cron uses the SAME wrap: `cron.AddFunc(expr, func(){ start:=time.Now(); if err:=job.Run(ctx); err!=nil { SchedulerJobExecutionsTotal{job,"error"}.Inc(); SchedulerJobDuration{job}.Observe(...) } else { ...,"success"; ...LastSuccess{job}.SetToCurrentTime(); lastRun=time.Now() } })`. The job-label string (`"autocache_prediction"`) is the join key across `scheduler_job_executions_total` / `_duration_seconds` / `_last_success_timestamp`.

### promauto auto-registration → /metrics (no plumbing)
**Source:** `libs/metrics/scheduler.go` (def) + `services/scheduler/internal/transport/router.go:36-37` (serve)
**Apply to:** the new `AutocachePredictedBytes` gauge
`promauto.New*` registers into the default registry at package init; `router.go` serves `metrics.Handler()` at `/metrics`. A new `promauto` var in `libs/metrics` is scraped with zero wiring — just `metrics.AutocachePredictedBytes.WithLabelValues(...).Set(...)` from the job.

### DB-portable time-window predicate (Go-computed cutoff bound as param)
**Source:** `services/scheduler/internal/jobs/autocache_logic_a.go:93` + `:110`
**Apply to:** the prediction job's two count queries
`cutoff := time.Now().AddDate(0,0,-days)` bound as `al.updated_at > ?` — runs identically on Postgres (prod) and SQLite (tests). Do NOT use Postgres `interval` syntax (breaks the SQLite test seam).

### Grafana panel JSON shape (this dashboard)
**Source:** `infra/grafana/dashboards/library.json` panels `id:1..7`
**Apply to:** all 6 new panels
Every panel: `datasource:{type:"prometheus",uid:"${DS_PROMETHEUS}"}` at panel + target level, `"pluginVersion": "10.3.3"`, `fieldConfig.defaults` with `unit` (`bytes`/`short`/`percentunit`), `gridPos{h,w,x,y}`, unique `id`, `targets[]` with `{datasource, expr, format:"time_series"|"table", legendFormat, refId}`.

---

## No Analog Found

| File / Element | Role | Data Flow | Reason |
|------|------|-----------|--------|
| OBS-05 Prometheus-datasource **table** panel in `library.json` | config (table panel) | transform | ALL 5 existing `"type":"table"` dashboards (`activity-register-pivot.json` etc.) are **ClickHouse**-backed (`grafana-clickhouse-datasource` + `rawSql`), NOT Prometheus. No Prometheus instant-query table panel exists anywhere in the repo. The planner must hand-author it (instant query + `format:"table"` + optional `merge`/`organize` transformations) — shape provided above; verify against Grafana 10.3.3 table semantics. `activity-register-pivot.json:96-143` is a structural reference for the table panel envelope (`type/gridPos/id/fieldConfig.overrides[unit]/options.showHeader/targets[format:"table"]`) but its `rawSql`/ClickHouse target is NOT reusable. |
| `getEnvInt64` config helper | utility | — | `config.go:150-164` has only `getEnv`/`getEnvInt`; no int64 reader exists. Clone `getEnvInt` swapping `strconv.Atoi`→`strconv.ParseInt(val,10,64)`. |

---

## Metadata

**Analog search scope:** `infra/grafana/dashboards/`, `services/scheduler/internal/{jobs,service,config,transport}`, `services/scheduler/cmd/scheduler-api/`, `libs/metrics/`, `services/library/internal/metrics/`
**Files scanned:** ~12 (excluding `.claude/worktrees/*` duplicates)
**Pattern extraction date:** 2026-06-17
**Already-emitted metrics confirmed present** (do NOT re-add — just chart): `library_autocache_bytes_used{source,freshness}`, `library_autocache_budget_bytes`, `library_autocache_episodes{source,freshness}` (`services/library/internal/metrics/library_metrics.go:203-222`), plus `library_autocache_serve_total` / `_evicted_total` / `_rejected_total` / `_downloads_total` (phases 8-10 per CONTEXT). The ONLY new metric is `library_autocache_predicted_bytes{component}` on the scheduler.
