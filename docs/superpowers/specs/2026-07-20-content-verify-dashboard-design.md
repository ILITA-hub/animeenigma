# Content-Verify Observability: Grafana Dashboard + Instrumentation — Design

**Date:** 2026-07-20
**Status:** Design (awaiting owner review)
**Service:** `content-verify` (:8101)

## Goal

A single-screen Grafana dashboard that answers the operational and business
questions about content-verify — *is it keeping up, is the banded
prioritization actually working, what are probes finding, and how much of the
catalog is covered* — backed by a small set of new Prometheus metrics so those
questions are answerable truthfully rather than from whatever fields happen to
exist today.

Design principle (owner steer): **panels earn their place by answering a
question an operator or the owner would act on** — not because a metric is
already on the wire. Where the highest-value question has no metric, we add the
metric.

## The questions the dashboard must answer

1. **Is it alive & keeping up?** throughput, verified ratio, staleness (has the
   worker stalled?), backlog trend.
2. **Is the banded prioritization doing its job?** — the whole point of the
   2026-07-20 unified-prioritization work, currently **completely
   uninstrumented**. Are ongoings probed first? Is the 60/30/10 band split real
   in practice? Is idle backfill only sweeping when hot work is drained?
3. **What are probes finding?** outcome mix, probe latency, audio-language
   verdicts (SUB/DUB), burned-in-subtitle detections, OP/ED skip detection.
4. **Coverage (business KPI):** what fraction of currently-airing anime has a
   fresh verdict — the reason the service exists (so aePlayer auto-selects the
   right combo).
5. **Are we healthy / good citizens?** tick-skip reasons (idle = caught up,
   degraded = shedding, claim_error = contention), in-flight lease concurrency.

## Current state (ground truth)

content-verify exposes **6 own metrics** (all service-local in
`internal/cvmetrics/metrics.go`, plain `promauto`, deliberately NOT in
`libs/metrics` to avoid permanent-0 impostor series):

| Metric | Type | Labels |
|---|---|---|
| `content_verify_queue_depth` | gauge | — |
| `content_verify_probes_total` | counter | `provider`, `result` (verified\|inconclusive\|unreachable\|synth\|error) |
| `content_verify_probe_duration_seconds` | histogram | — (buckets 5…120s) |
| `content_verify_ticks_skipped_total` | counter | `reason` (degraded\|idle\|claim_error) |
| `content_verify_last_probe_timestamp` | gauge | — |
| `content_verify_skip_probes_total` | counter | `provider`, `result` (detected\|no_match\|pending_fp\|unreachable\|"") |

Plus standard `http_*` (via `libs/metrics.NewCollector("content-verify")`) and
`go_*` / `process_*`.

**Blind spots:** the banded queue (per-band depth, band selection, idle cursor),
lease/worker concurrency, and verdict *content* (audio lang, burned-in subs)
have **no metrics**. "Poison" does not exist in this service (that is
streamprobe) — explicitly out of scope.

**Storage fact:** verdicts persist to table `content_verifications` in the
**shared `animeenigma` DB** — `anime_id` (uuid), `provider` (varchar64), `units`
(**jsonb**, holds `[]UnitVerdict` with `audio.lang` and `hardsub.present/.lang`),
`updated_at`. `AudioVerdict.Lang ∈ {en,ru,ja,other}`; `HardsubVerdict{Present,
Lang,…}`. Because audio/hardsub live inside JSONB (no flat columns), verdict
*content* is surfaced as **Prometheus labels**, while coarse **coverage** (does
an anime have a fresh row at all) is a plain SQL query.

## Instrumentation to add

All new metrics go in `internal/cvmetrics/metrics.go`, same `promauto.New*`
block, same service-local rationale (**never** `libs/metrics`).

| New metric | Type | Labels | Set where |
|---|---|---|---|
| `content_verify_band_depth` | gauge | `band` | `queue/engine.go` `bandedCandidates` — `.Set(len(groups[b]))` per band, from the existing `groups map[Band][]Candidate` (no new data, no new query) |
| `content_verify_probes_total` **(+`band` label)** | counter | `provider`, `result`, **`band`** | `service/worker.go` `persist` — via new `Unit.Band` (see plumbing) |
| `content_verify_verdicts_total` | counter | `audio_lang` | `service/worker.go` `persist` — `v.Audio.Lang` when `v.Audio != nil` (empty → `unknown`) |
| `content_verify_hardsub_total` | counter | `lang` | `service/worker.go` `persist` — when `v.Hardsub != nil && v.Hardsub.Present` (empty lang → `unknown`) |
| `content_verify_idle_cursor` | gauge | — | `queue/engine.go` `interest()` — from `IdleCursor`/`AdvanceIdleCursor` |
| `content_verify_idle_total` | gauge | — | `queue/engine.go` `interest()` — `it.IdleTotal` |
| `content_verify_inflight_leases` | gauge | — | `queue/engine.go` `lease()` + release closure — `len(e.inflightUnits)` under `e.mu` |

**Band-label plumbing (the one non-mechanical change).** The band is known at
the `Claim` candidate loop but discarded — `Claim` returns
`(*Unit, *SkipTask, func(), error)` and `queue.Unit` has no band. Add:

- `Band` field to `queue.Unit` (`internal/queue/enumerate.go`).
- `func (b Band) Label() string` in `internal/queue/queue.go` →
  `pinned` / `ongoing` / `watched_top` / `idle` (there is no existing
  `String()`; `Snapshot` currently emits the raw int).
- Set `u.Band = BandOf(cand)` where the verify unit is built (`claimVerifyUnit`
  and the synth path), so `persist` reads `unit.Band.Label()`.

The **skip lane** (`skip_probes_total`) stays band-unlabeled — lower value, and
it keeps the change tight. Duration histogram stays unlabeled.

**Name-collision note:** `content_verify_queue_depth` (unlabeled total) is left
exactly as-is — Prometheus rejects re-registering the same name with a new label
set, and any alert/governor reference to the total keeps working. Per-band depth
is the distinct `content_verify_band_depth{band}`; the total panel keeps using
`content_verify_queue_depth` (equivalent to `sum(content_verify_band_depth)`).

## Dashboard layout

`infra/grafana/dashboards/content-verify.json` — 24-col grid, templated
`${DS_PROMETHEUS}` datasource, `schemaVersion` 38, `pluginVersion` "10.3.3",
`refresh` 30s, `time` now-6h, `graphTooltip` 1. `uid: "content-verify"`,
`title: "Content Verify"`, `tags: ["content-verify","catalog","ae"]`.

**Row 1 — Liveness & keeping up** (stat tiles, `w:6 h:4`, y:0)

| # | Title | expr | notes |
|---|---|---|---|
| 1 | Probes / min | `sum(rate(content_verify_probes_total[5m])) * 60` | unit ops/min |
| 2 | Verified % (1h) | `100 * sum(rate(content_verify_probes_total{result="verified"}[1h])) / clamp_min(sum(rate(content_verify_probes_total[1h])), 0.001)` | thresholds: red<50, yellow<75, green≥75 |
| 3 | Probe lag | `time() - content_verify_last_probe_timestamp` | unit s; **the #1 "is it broken" tile** — green<300, yellow<900, red≥900 |
| 4 | Ongoing coverage % | **Postgres** (`aenigma-postgres`) — SQL below | the business KPI; green≥ target |

**Row 2 — Prioritization proof** (timeseries, `w:8 h:8`, y:4)

| # | Title | expr | notes |
|---|---|---|---|
| 5 | Backlog by band | `sum by (band)(content_verify_band_depth)` | **stacked**; legend `{{band}}` — is it draining, what's it chewing on |
| 6 | Probe spend by band | `sum by (band)(rate(content_verify_probes_total[5m]))` | proves the 60/30/10 *realized*, not assumed |
| 7 | Idle sweep | `content_verify_idle_cursor` and `content_verify_idle_total` | cursor sawtooth vs. total ceiling — round-robin progress |

**Row 3 — Outcomes & findings** (timeseries, `w:8 h:8`, y:12)

| # | Title | expr | notes |
|---|---|---|---|
| 8 | Outcome mix | `sum by (result)(rate(content_verify_probes_total[5m]))` | legend `{{result}}` |
| 9 | Probe duration p50/p95 | `histogram_quantile(0.5, sum(rate(content_verify_probe_duration_seconds_bucket[5m])) by (le))` + `0.95` | unit s |
| 10 | Findings (audio lang / burned-in) | `sum by (audio_lang)(rate(content_verify_verdicts_total[15m]))` + `sum by (lang)(rate(content_verify_hardsub_total[15m]))` | what verdicts we're producing (SUB/DUB split + hardsub hits) |

**Row 4 — Health** (timeseries + stat, y:20)

| # | Title | expr | notes |
|---|---|---|---|
| 11 | Worker time: tick-skips | `sum by (reason)(rate(content_verify_ticks_skipped_total[5m]))` | idle = caught up (good), degraded = shedding, claim_error = contention |
| 12 | In-flight leases | `content_verify_inflight_leases` | concurrency (ceiling ≈ configured workers) — small stat/gauge |
| 13 *(optional)* | OP/ED skip lane | `sum by (result)(rate(content_verify_skip_probes_total[5m]))` | auto-skip detection output; fold in only if wanted |

**Ongoing coverage SQL (panel 4, `aenigma-postgres`, format=table, instant):**

```sql
SELECT 100.0 * count(DISTINCT cv.anime_id)
         FILTER (WHERE cv.updated_at > now() - interval '48 hours')
       / NULLIF(count(DISTINCT a.id), 0) AS coverage_pct
FROM animes a
LEFT JOIN content_verifications cv ON cv.anime_id = a.id
WHERE a.status = 'ongoing';
```

Anime-level coverage (has any fresh verification row). Per-episode/per-provider
coverage is out of scope — this KPI answers "are airing titles getting looked
at," which is the business question.

## Conventions & deployment

- **File provisioner hot-reloads** the JSON (polls the read-only bind mount) —
  no Grafana restart/recreate needed; `make restart-grafana` only forces an
  immediate rescan.
- The Go metric additions require **`make redeploy-content-verify`** (a new
  metric only appears after the binary rebuilds).
- Validate JSON with `jq -e . infra/grafana/dashboards/content-verify.json`
  before landing (no dashboard-lint target exists).
- **Impostor-trap guard:** all new metrics stay in `cvmetrics` (service-local).
  Adding any to `libs/metrics` would register permanent-0 impostor series across
  ~20 binaries — explicitly forbidden.

## Verification

- After redeploy: `curl -s localhost:8101/metrics | grep content_verify_` shows
  `band_depth{band=…}`, `probes_total{…,band=…}`, `verdicts_total{audio_lang=…}`,
  `hardsub_total{lang=…}`, `idle_cursor`, `idle_total`, `inflight_leases`.
  (Counters emit no series until first `Inc` — expect some absent until a probe
  of that kind runs; gauges appear immediately.)
- Dashboard: all 12–13 panels render; band panels show ≥1 band series; coverage
  tile returns a number; probe-lag tile is small (worker alive).

## Out of scope

Poison detection (not a content-verify concept); per-episode coverage;
per-provider throttle metric; band label on the skip lane / duration histogram;
any change to `libs/metrics`; any change to prioritization *behavior* (this is
observability only).

## Open items to confirm at build time

- Exact Grafana Postgres datasource `type` string + `uid` for the coverage panel
  (`aenigma-postgres`; type likely `grafana-postgresql-datasource`).
- `content_verifications.anime_id` storage type (uuid vs varchar) for the join
  cast in the coverage SQL.
- Whether to keep panel 13 (OP/ED skip lane) in the default view.

## Phasing (for the implementation plan)

- **Phase 1 — Instrumentation (Go):** cvmetrics additions + `Band.Label()` +
  `Unit.Band` plumbing + per-band depth in `bandedCandidates` + verdict/hardsub
  increments in `persist` + idle-cursor/idle-total/inflight-leases gauges.
  Redeploy content-verify; verify metrics on the wire.
- **Phase 2 — Dashboard (JSON):** author `content-verify.json` against the real
  metric names + coverage SQL; `jq` validate; land; hot-reload; verify panels.

## Metrics (project convention)

- **UXΔ = +2 (Better)** — no end-user surface change; the owner/operator gains a
  first-ever view into content-verify and proof the new prioritization works.
- **CDI = 0.03 * 8** — narrow spread (one service's metrics + one dashboard file,
  additive), small shift (new metrics + one signature plumb, behavior
  unchanged); Effort 8.
- **MVQ = Griffin 85% / 80%** — a clear-eyed watch over the flock; low slop
  because every panel maps to a named operational question.
