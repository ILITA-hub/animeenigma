# Content-Verify Dashboard + Instrumentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the metrics needed to observe content-verify's throughput,
banded prioritization, verdict findings, and health, then ship a single-screen
Grafana dashboard that renders them.

**Architecture:** Two phases. Phase 1 adds service-local Prometheus metrics
(all in `internal/cvmetrics`) and the one plumb needed to carry the queue
*band* to the probe call-site. Phase 2 authors the provisioned dashboard JSON
against those real metric names plus one Postgres coverage query. Prioritization
*behavior* is untouched — this is observability only.

**Tech Stack:** Go (`prometheus/client_golang` via `promauto`, chi, GORM),
Grafana 10.3.3 file-provisioned JSON, Prometheus datasource
`uid: PBFA97CFB590B2093` (referenced as `${DS_PROMETHEUS}`), Postgres datasource
`uid: aenigma-postgres` (db `animeenigma`).

## Global Constraints

- **Service-local metrics only.** Every new metric is declared in
  `services/content-verify/internal/cvmetrics/metrics.go` with `promauto.New*`.
  Adding any metric to `libs/metrics` is forbidden — it would register
  permanent-0 impostor series in ~20 binaries that import the shared package.
- **Go builds per-package; keep `go build ./...` green each task.** Any change
  to a metric's label arity (e.g. `ProbesTotal`) MUST update every call site in
  the same task, or the `service` package stops compiling.
- **Exact names.** New metrics: `content_verify_band_depth{band}`,
  `content_verify_verdicts_total{audio_lang}`, `content_verify_hardsub_total{lang}`,
  `content_verify_idle_cursor`, `content_verify_idle_total`,
  `content_verify_inflight_leases`. `probes_total` gains a third label `band`.
- **Band label values:** `pinned` / `ongoing` / `watched_top` / `idle` (from
  `Band.Label()`; `BandPinned=0, BandOngoing=1, BandWatchedTop=2, BandIdle=3`).
- **Empty verdict language → `"unknown"`** for both `verdicts_total{audio_lang}`
  and `hardsub_total{lang}`.
- **Do NOT reuse `content_verify_queue_depth`** with a label — Prometheus
  rejects dual registration. Per-band depth is the distinct
  `content_verify_band_depth`; the unlabeled total gauge is left exactly as-is.
- **Never run `gofmt -w` / `make fmt`** (pre-existing smart-quote doc comments
  trip it). Verify with `go build ./...` and `go test`.
- **No time-effort units** anywhere (UXΔ / CDI / MVQ only).
- **Commits:** pathspec only (never `git add -A`); co-authors
  `Claude Code <noreply@anthropic.com>`, `0neymik0 <0neymik0@gmail.com>`,
  `NANDIorg <super.egor.mamonov@yandex.ru>`.

## File Structure

- `services/content-verify/internal/cvmetrics/metrics.go` — MODIFY: add 6 new
  vars; add `band` label to `ProbesTotal`.
- `services/content-verify/internal/queue/queue.go` — MODIFY: add
  `func (b Band) Label() string`.
- `services/content-verify/internal/queue/enumerate.go` — MODIFY: add `Band`
  field to `Unit`.
- `services/content-verify/internal/queue/engine.go` — MODIFY: set `u.Band` in
  the verify/synth claim paths; set `BandDepth`, `IdleCursor`, `IdleTotal`,
  `InflightLeases` gauges.
- `services/content-verify/internal/service/worker.go` — MODIFY: in `persist`,
  pass `unit.Band.Label()` to `ProbesTotal`; increment `VerdictsTotal` /
  `HardsubTotal`.
- `services/content-verify/internal/queue/*_test.go`,
  `internal/cvmetrics/metrics_test.go` — tests.
- `infra/grafana/dashboards/content-verify.json` — CREATE: the dashboard.

---

## Phase 1 — Instrumentation (Go)

### Task 1: `Band.Label()` + `Unit.Band` plumbing

**Files:**
- Modify: `services/content-verify/internal/queue/queue.go` (Band consts at 15-24, `BandOf` at 42-53)
- Modify: `services/content-verify/internal/queue/enumerate.go` (`Unit` struct at 23-37)
- Modify: `services/content-verify/internal/queue/engine.go` (`Claim` loop at ~388-414; the verify-unit build in `claimVerifyUnit` ~368; synth path)
- Test: `services/content-verify/internal/queue/band_label_test.go`

**Interfaces:**
- Produces: `func (b Band) Label() string` → `"pinned"|"ongoing"|"watched_top"|"idle"`; `Unit.Band Band` field, set to `BandOf(cand)` for every claimed verify unit (including synth). Task 3 reads `unit.Band.Label()` in the worker.

- [ ] **Step 1: Write the failing test**

```go
// services/content-verify/internal/queue/band_label_test.go
package queue

import "testing"

func TestBandLabel(t *testing.T) {
	cases := map[Band]string{
		BandPinned:     "pinned",
		BandOngoing:    "ongoing",
		BandWatchedTop: "watched_top",
		BandIdle:       "idle",
	}
	for b, want := range cases {
		if got := b.Label(); got != want {
			t.Errorf("Band(%d).Label() = %q, want %q", b, got, want)
		}
	}
	// Out-of-range is defensive, must not panic and must not be empty.
	if Band(99).Label() == "" {
		t.Error("unknown band label must not be empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/content-verify && go test ./internal/queue/ -run TestBandLabel -v`
Expected: FAIL — `b.Label undefined`.

- [ ] **Step 3: Add `Label()` in `queue.go`** (below the const block / `BandOf`)

```go
// Label returns the stable snake_case metric-label form of the band.
func (b Band) Label() string {
	switch b {
	case BandPinned:
		return "pinned"
	case BandOngoing:
		return "ongoing"
	case BandWatchedTop:
		return "watched_top"
	case BandIdle:
		return "idle"
	default:
		return "unknown"
	}
}
```

- [ ] **Step 4: Add `Band` field to `Unit`** in `enumerate.go` (append to the struct)

```go
	Band Band // priority band this unit was claimed from (for metric labelling)
```

- [ ] **Step 5: Set `u.Band` where the claimed verify unit is built** in
  `engine.go`. In `Claim`'s candidate loop the band is `BandOf(cand)`. Read the
  function first; set the field on the `*Unit` that `claimVerifyUnit` (and the
  synth branch) returns, before it escapes — e.g. `u.Band = BandOf(cand)`. Make
  sure both the normal and synth unit paths set it.

- [ ] **Step 6: Write an integration test** that a claimed unit carries the band.
  Read an existing `engine`-construction test in the package for the exact
  `NewEngine` arity and fake dependencies, then assert: a pinned candidate yields
  a claimed `Unit` with `Band == BandPinned`; an ongoing candidate yields
  `BandOngoing`. (Reuse the package's existing engine test harness — do not
  invent new fakes if the package already has them.)

- [ ] **Step 7: Run tests**

Run: `cd services/content-verify && go build ./... && go test ./internal/queue/ -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add services/content-verify/internal/queue/queue.go \
        services/content-verify/internal/queue/enumerate.go \
        services/content-verify/internal/queue/engine.go \
        services/content-verify/internal/queue/band_label_test.go
git commit -m "feat(content-verify): carry priority band on claimed units

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

### Task 2: `band_depth{band}` gauge

**Files:**
- Modify: `services/content-verify/internal/cvmetrics/metrics.go` (var block 12-33)
- Modify: `services/content-verify/internal/queue/engine.go` (`bandedCandidates` 161-184; `groups map[Band][]Candidate` at 167-171)
- Test: `services/content-verify/internal/cvmetrics/metrics_test.go`

**Interfaces:**
- Consumes: `Band.Label()` (Task 1).
- Produces: `cvmetrics.BandDepth *prometheus.GaugeVec` label `band`.

- [ ] **Step 1: Add the metric var** to the existing `var (…)` block in `metrics.go`

```go
	BandDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "content_verify_band_depth",
		Help: "Candidate count per priority band at the last queue build.",
	}, []string{"band"})
```

- [ ] **Step 2: Write the failing test**

```go
// services/content-verify/internal/cvmetrics/metrics_test.go
package cvmetrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestBandDepthGauge(t *testing.T) {
	BandDepth.WithLabelValues("ongoing").Set(7)
	if got := testutil.ToFloat64(BandDepth.WithLabelValues("ongoing")); got != 7 {
		t.Errorf("band_depth{ongoing} = %v, want 7", got)
	}
}
```

- [ ] **Step 3: Run it — expect FAIL** (`BandDepth` undefined) then PASS after Step 1.

Run: `cd services/content-verify && go test ./internal/cvmetrics/ -run TestBandDepthGauge -v`

- [ ] **Step 4: Set per-band depth in `bandedCandidates`.** After the loop that
  fills `groups` (engine.go ~167-171), set the gauge for every band so empty
  bands report 0:

```go
	for _, b := range []Band{BandPinned, BandOngoing, BandWatchedTop, BandIdle} {
		cvmetrics.BandDepth.WithLabelValues(b.Label()).Set(float64(len(groups[b])))
	}
```

  Leave the existing `cvmetrics.QueueDepth.Set(float64(len(all)))` at :165
  unchanged.

- [ ] **Step 5: Run build + tests**

Run: `cd services/content-verify && go build ./... && go test ./internal/cvmetrics/ ./internal/queue/`
Expected: PASS.

- [ ] **Step 6: Commit** (pathspec: `metrics.go`, `engine.go`, `metrics_test.go`; same co-author trailer).

### Task 3: probe-outcome band label + verdict-content counters

> **Spans two packages by necessity.** Adding the `band` label to `ProbesTotal`
> (package `cvmetrics`) breaks its call sites in `worker.go` (package `service`);
> both are fixed here so `go build ./...` stays green.

**Files:**
- Modify: `services/content-verify/internal/cvmetrics/metrics.go` (`ProbesTotal` at 16-18; add two counters)
- Modify: `services/content-verify/internal/service/worker.go` (`persist` 175-192: error inc :180, success inc :186)
- Test: `services/content-verify/internal/cvmetrics/metrics_test.go`

**Interfaces:**
- Consumes: `Unit.Band` + `Band.Label()` (Task 1); `domain.UnitVerdict.Audio *AudioVerdict{Lang}`, `.Hardsub *HardsubVerdict{Present,Lang}`.
- Produces: `ProbesTotal` labels `{provider,result,band}`; `VerdictsTotal{audio_lang}`; `HardsubTotal{lang}`.

- [ ] **Step 1: In `metrics.go`, add the `band` label to `ProbesTotal`** and add
  the two counters:

```go
	ProbesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "content_verify_probes_total",
		Help: "Unit (full A/V) probes by provider, result, and priority band.",
	}, []string{"provider", "result", "band"})

	VerdictsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "content_verify_verdicts_total",
		Help: "Audio-language verdicts produced, by detected language.",
	}, []string{"audio_lang"})

	HardsubTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "content_verify_hardsub_total",
		Help: "Burned-in (hardsub) subtitle detections, by language.",
	}, []string{"lang"})
```

- [ ] **Step 2: Write the failing test**

```go
func TestVerdictAndHardsubCounters(t *testing.T) {
	VerdictsTotal.WithLabelValues("ja").Inc()
	if testutil.ToFloat64(VerdictsTotal.WithLabelValues("ja")) != 1 {
		t.Error("verdicts_total{ja} not incremented")
	}
	HardsubTotal.WithLabelValues("unknown").Inc()
	if testutil.ToFloat64(HardsubTotal.WithLabelValues("unknown")) != 1 {
		t.Error("hardsub_total{unknown} not incremented")
	}
	// probes_total now takes three labels
	ProbesTotal.WithLabelValues("kodik", "verified", "ongoing").Inc()
	if testutil.ToFloat64(ProbesTotal.WithLabelValues("kodik", "verified", "ongoing")) != 1 {
		t.Error("probes_total{kodik,verified,ongoing} not incremented")
	}
}
```

- [ ] **Step 3: Run it — expect FAIL** (undefined counters / arity). 

- [ ] **Step 4: Update `persist` in `worker.go`.** Read the function (175-192).
  Change both `ProbesTotal.WithLabelValues(unit.Provider, "error")` (:180) and
  `…, result)` (:186) to include `unit.Band.Label()` as the third arg. After the
  success increment, add the verdict-content increments (guard nils; empty lang →
  `"unknown"`):

```go
	if v.Audio != nil {
		lang := v.Audio.Lang
		if lang == "" {
			lang = "unknown"
		}
		cvmetrics.VerdictsTotal.WithLabelValues(lang).Inc()
	}
	if v.Hardsub != nil && v.Hardsub.Present {
		lang := v.Hardsub.Lang
		if lang == "" {
			lang = "unknown"
		}
		cvmetrics.HardsubTotal.WithLabelValues(lang).Inc()
	}
```

  (`v` is the `domain.UnitVerdict` param of `persist`.)

- [ ] **Step 5: Run build + tests**

Run: `cd services/content-verify && go build ./... && go test ./internal/cvmetrics/ ./internal/service/`
Expected: PASS. Confirm no other `ProbesTotal.WithLabelValues(` call sites remain
2-arg: `grep -rn "ProbesTotal.WithLabelValues" services/content-verify`.

- [ ] **Step 6: Commit** (pathspec: `metrics.go`, `worker.go`, `metrics_test.go`).

### Task 4: idle + lease gauges

**Files:**
- Modify: `services/content-verify/internal/cvmetrics/metrics.go`
- Modify: `services/content-verify/internal/queue/engine.go` (`interest()` 131-155; `lease()` 341-361; release closure 356-357)
- Test: `services/content-verify/internal/cvmetrics/metrics_test.go`

**Interfaces:**
- Produces: `cvmetrics.IdleCursor`, `cvmetrics.IdleTotal`, `cvmetrics.InflightLeases` (all `prometheus.Gauge`, unlabeled).

- [ ] **Step 1: Add three gauges** to the var block:

```go
	IdleCursor = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "content_verify_idle_cursor",
		Help: "Current offset of the idle-backfill round-robin cursor.",
	})
	IdleTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "content_verify_idle_total",
		Help: "Size of the idle (non-ongoing) catalog tail the cursor sweeps.",
	})
	InflightLeases = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "content_verify_inflight_leases",
		Help: "In-process probe leases currently held (concurrency).",
	})
```

- [ ] **Step 2: Failing test** (gauge set/observe):

```go
func TestConcurrencyAndIdleGauges(t *testing.T) {
	InflightLeases.Set(2)
	if testutil.ToFloat64(InflightLeases) != 2 {
		t.Error("inflight_leases not set")
	}
	IdleCursor.Set(300)
	if testutil.ToFloat64(IdleCursor) != 300 {
		t.Error("idle_cursor not set")
	}
}
```

- [ ] **Step 3: Run — expect FAIL then PASS after Step 1.**

- [ ] **Step 4: Set idle gauges in `interest()`.** Read 131-155. After the cursor
  is read (:140) and after `AdvanceIdleCursor` (:150), set:
  `cvmetrics.IdleCursor.Set(float64(offset))` and
  `cvmetrics.IdleTotal.Set(float64(it.IdleTotal))`. Use the value actually in
  scope (the read `offset`, and on the advance branch the returned new cursor).

- [ ] **Step 5: Set the lease gauge in `lease()`.** Both mutations already run
  under `e.mu`. After the inserts (`engine.go` ~350-351):
  `cvmetrics.InflightLeases.Set(float64(len(e.inflightUnits)))`. In the release
  closure after the deletes (~356-357), set it again to the new length. (Both
  sites are under the lock — the gauge is set inside the critical section.)

- [ ] **Step 6: Build + tests**

Run: `cd services/content-verify && go build ./... && go test ./internal/...`
Expected: PASS.

- [ ] **Step 7: Commit** (pathspec: `metrics.go`, `engine.go`, `metrics_test.go`).

### Task 5: deploy + verify metrics on the wire (controller step)

Not a subagent task — the controller runs it on the host.

- [ ] **Step 1: Redeploy**

Run: `cd /data/animeenigma/.claude/worktrees/content-verify-probing && make redeploy-content-verify`
(worktree deploy requires `.env`; if it 401s, that's the known missing-dotenv trap.)

- [ ] **Step 2: Confirm the new series are exposed**

Run: `curl -s localhost:8101/metrics | grep -E '^content_verify_(band_depth|idle_cursor|idle_total|inflight_leases|probes_total|verdicts_total|hardsub_total)'`
Expected: `band_depth{band=…}`, `idle_cursor`, `idle_total`, `inflight_leases`
present immediately (gauges). `probes_total` now carries `band=`. The
`verdicts_total` / `hardsub_total` counters may be **absent** until a probe of
that kind runs (counters emit no series before first `Inc`) — that is expected,
not a failure.

---

## Phase 2 — Dashboard (JSON)

### Task 6: author `content-verify.json`

**Files:**
- Create: `infra/grafana/dashboards/content-verify.json`

**Reference template:** mirror the structure of
`docker/grafana/dashboards/subtitle-health.json` — same top-level fields
(`schemaVersion: 38`, `pluginVersion: "10.3.3"`, `refresh: "30s"`,
`time: {from: "now-6h", to: "now"}`, `graphTooltip: 1`, `editable: true`,
`id: null`), the `DS_PROMETHEUS` templating variable, and copy the `fieldConfig`
/ `options` blocks from its timeseries panel (id 2) and stat panel (id 5)
verbatim, changing only `targets`, `title`, `gridPos`, `id`, `unit`, and
thresholds.

**Top-level:** `uid: "content-verify"`, `title: "Content Verify"`,
`tags: ["content-verify","catalog","ae"]`,
`description: "content-verify probing: throughput, banded prioritization, verdict findings, coverage, and worker health."`

**Panels** (24-col grid; all Prometheus panels use
`{"type":"prometheus","uid":"${DS_PROMETHEUS}"}`):

| id | title | type | gridPos | target expr(s) | unit / thresholds |
|---|---|---|---|---|---|
| 1 | Probes / min | stat | `x0 y0 w6 h4` | `sum(rate(content_verify_probes_total[5m])) * 60` | `short` |
| 2 | Verified % (1h) | stat | `x6 y0 w6 h4` | `100 * sum(rate(content_verify_probes_total{result="verified"}[1h])) / clamp_min(sum(rate(content_verify_probes_total[1h])), 0.001)` | `percent`; red<50 yellow<75 green≥75 |
| 3 | Probe lag | stat | `x12 y0 w6 h4` | `time() - content_verify_last_probe_timestamp` | `s`; green<300 yellow<900 red≥900 |
| 4 | Ongoing coverage % | stat | `x18 y0 w6 h4` | **Postgres** (see below) | `percent`; red<50 yellow<80 green≥80 |
| 5 | Backlog by band | timeseries (stacked) | `x0 y4 w8 h8` | `sum by (band)(content_verify_band_depth)` legend `{{band}}` | `short`; `stacking.mode: normal` |
| 6 | Probe spend by band | timeseries | `x8 y4 w8 h8` | `sum by (band)(rate(content_verify_probes_total[5m]))` legend `{{band}}` | `reqps` |
| 7 | Idle sweep | timeseries | `x16 y4 w8 h8` | A: `content_verify_idle_cursor` legend `cursor`; B: `content_verify_idle_total` legend `total` | `short` |
| 8 | Outcome mix | timeseries | `x0 y12 w8 h8` | `sum by (result)(rate(content_verify_probes_total[5m]))` legend `{{result}}` | `reqps` |
| 9 | Probe duration | timeseries | `x8 y12 w8 h8` | A: `histogram_quantile(0.5, sum(rate(content_verify_probe_duration_seconds_bucket[5m])) by (le))` legend `p50`; B: `0.95…` legend `p95` | `s` |
| 10 | Findings (lang) | timeseries | `x16 y12 w8 h8` | A: `sum by (audio_lang)(rate(content_verify_verdicts_total[15m]))` legend `audio {{audio_lang}}`; B: `sum by (lang)(rate(content_verify_hardsub_total[15m]))` legend `hardsub {{lang}}` | `reqps` |
| 11 | Worker time: tick-skips | timeseries | `x0 y20 w12 h7` | `sum by (reason)(rate(content_verify_ticks_skipped_total[5m]))` legend `{{reason}}` | `reqps` |
| 12 | In-flight leases | stat | `x12 y20 w6 h7` | `content_verify_inflight_leases` | `short` |
| 13 | OP/ED skip lane | timeseries | `x18 y20 w6 h7` | `sum by (result)(rate(content_verify_skip_probes_total[5m]))` legend `{{result}}` | `reqps` |

**Panel 4 — coverage (Postgres).** Datasource block (confirm the exact
`type`/`uid` against `docker/grafana/provisioning/datasources/datasources.yml`
first — expected `type: "grafana-postgresql-datasource"`, `uid: "aenigma-postgres"`),
`format: "table"`, instant query:

```sql
SELECT 100.0 * count(DISTINCT cv.anime_id)
         FILTER (WHERE cv.updated_at > now() - interval '48 hours')
       / NULLIF(count(DISTINCT a.id), 0) AS coverage_pct
FROM animes a
LEFT JOIN content_verifications cv ON cv.anime_id = a.id
WHERE a.status = 'ongoing';
```

If `information_schema` shows `content_verifications.anime_id` is `varchar` (not
`uuid`), cast the join: `ON cv.anime_id = a.id::text`.

- [ ] **Step 1:** Read `subtitle-health.json` for the exact panel skeletons.
- [ ] **Step 2:** Confirm the Postgres datasource `type`/`uid` and the
  `anime_id` column type (`\d content_verifications` via the pgAdmin/psql path,
  or read the GORM tag in `internal/domain/verify.go`).
- [ ] **Step 3:** Author the JSON with all 13 panels per the table.
- [ ] **Step 4: Validate**

Run: `jq -e . infra/grafana/dashboards/content-verify.json >/dev/null && echo OK`
Expected: `OK`.

- [ ] **Step 5:** Grep-check every `expr` uses a real metric name from Phase 1
  (no typos): the only `content_verify_*` names allowed are those in the Global
  Constraints plus `probe_duration_seconds`, `ticks_skipped_total`,
  `skip_probes_total`, `last_probe_timestamp`.
- [ ] **Step 6: Commit** (pathspec: the one JSON file; co-author trailer).

### Task 7: provision + verify render (controller step)

- [ ] **Step 1:** The file provisioner hot-reloads the read-only mount; force an
  immediate rescan with `make restart-grafana` if needed.
- [ ] **Step 2:** Verify the dashboard loads at
  `http://127.0.0.1:3004/d/content-verify` (Grafana dev) with no "datasource not
  found" / "no data" on the gauge panels; band panels show ≥1 series; the
  coverage tile returns a number. An in-browser Chrome smoke is OPT-IN — only if
  the owner asks.

---

## Wrap-up

After Phase 2: run `/animeenigma-after-update` (it will `/simplify` the Go diff,
lint/build, redeploy content-verify, add a Trump-mode changelog entry — a
dashboard + observability entry — commit with co-authors, and push to `main`).
The dashboard JSON change alone does not need a service redeploy (Grafana
hot-reloads), but the Go metrics do (`make redeploy-content-verify`, done in
Task 5).

## Self-Review (author checklist — done)

- **Spec coverage:** all 5 questions → panels 1-13; all 7 new-metric/label
  requirements → Tasks 1-4; coverage SQL → Task 6; impostor-trap guard → Global
  Constraints. ✅
- **Placeholders:** none — every code step carries real code; the two build-time
  confirmations (Postgres type, `anime_id` cast) are explicit verification steps
  with a stated fallback, not TODOs. ✅
- **Type consistency:** `Band.Label()` defined Task 1, consumed Tasks 2-3 and
  every dashboard `{{band}}`; `ProbesTotal` 3-label form defined Task 3 and used
  by panels 6/8; `Unit.Band` defined Task 1, read Task 3. ✅
