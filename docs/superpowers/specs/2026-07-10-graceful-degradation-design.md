# Graceful Degradation Under Load — "Pressure Governor"

**Date:** 2026-07-10
**Origin:** feedback report `2026-07-04T05-46-35_claude-code_manual` (owner TODO, captured 2026-07-04)
**Owner directives (2026-07-10):** pair with existing awareness + bandwidth measuring capabilities; add a **distinct degradation-overview Grafana dashboard** showing *what, when and why* was affected. Scope = design + full implementation, phased. Governor = new tiny microservice. Host metrics = node-exporter. First actuation wave = Camoufox pool clamp + library torrent/encode + scheduler heavy crons.

## Problem

When the single-host deployment (server IS prod; 8 cores / 16 GB / 8 GB swap) comes under
pressure — user spikes or heavy background operations — everything degrades equally.
Critical UX (login, navigation, active playback) should survive; heavy deferrable work
(Camoufox solves, torrent download + ffmpeg transcode, upscaler finalize, probes,
recompute batches) should shed first.

Live evidence at design time: io PSI `full` ≈ 8.7 % avg10 and swap 7.3/8 GiB while memory
PSI `full` sat at 0.17 % — i.e. the box is chronically "full" by static-usage measures yet
mostly healthy by stall measures. **Thresholds must use PSI (pressure-stall) ratios, not
static usage**, or they false-positive permanently on this box.

## Priority tiers

| Tier | Never/last/first | Members | Actuation |
|------|------------------|---------|-----------|
| **CRITICAL** | never degrade | auth/login, gateway routing, SPA shell + navigation, active playback (streaming HLS proxy), watch-progress writes | none — these are what the system protects |
| **IMPORTANT** | degrade last | search / catalog browse, capabilities feed, subtitle fetch, notifications | only at Critical level, and only via cache-TTL stretch (later phase, optional) |
| **HEAVY / DEFERRABLE** | shed first | Camoufox solves + warming, library torrent download + ffmpeg encode, upscaler segment finalize, storyboard backfill, probe sweeps / canaries, recs + ranking recomputes, analytics batches | pause admission of NEW work at Elevated; refuse/clamp at Critical. Running work finishes (no kill). |

## Architecture (3 layers, mapped to existing systems)

```
 [node-exporter]──┐                             (NEW, Phase 1)
 [vnstat-exporter]┤                             (existing bandwidth awareness)
 [service /metrics]┴─▶ Prometheus ──▶ recording rules  ae:*   (NEW, Phase 1 — signal math defined ONCE)
                          │                │
                          │                ├─▶ Grafana "Degradation / Overview" dashboard  (NEW, Phase 1)
                          │                ├─▶ Grafana alerts ─▶ maintenance-webhook       (NEW rules, existing pipe)
                          │                ▼
                          └────── governor service :8100 (NEW, Phase 2)
                                   poll rules via Prom HTTP API → hysteresis → level 0/1/2
                                   ├─▶ Redis  ae:degradation:level + reasons (+ manual override key)
                                   ├─▶ /metrics  ae_degradation_level, ae_degradation_reason_active{signal}
                                   ├─▶ ClickHouse analytics.degradation_transitions (what/when/why history)
                                   └─▶ (dashboards read all of the above)
              heavy services poll Redis key every ~5s               (Phase 3)
              library: stop claiming new download/encode jobs at L1+
              stealth-scraper: stop warming at L1; refuse new solves (kind=degraded) at L2
              scheduler: skip-if-degraded guard on heavy crons at L1+
```

### Pairing with existing awareness/bandwidth capabilities (owner directive)

- **vnstat** (`vnstat_interface_{rx,tx}_bytes_total`) = uplink truth; recording rules split egress into
  playback (`proxy_bytes_transferred_total`, streaming) vs torrent (`library_upload_bytes_total`) vs other.
- **Egress recorder / ClickHouse** (`analytics.events`, `effect_kind='egress'`) + the shipped
  `egress-volume-anomaly` alert stay the per-operation attribution layer; the dashboard links the two views.
- **Probe engine / provider_state / playability**: unchanged; probes become a *shed target* (deferred at L1+),
  and `probe_provider_up` panels give context on whether degradation coincided with provider trouble.
- **Maintenance bot**: host-pressure alerts route to the existing `maintenance-webhook` contact point
  (no provider/server/reason triple → escalates to admin, per anomaly-alert convention).
- **Existing actuation primitives reused**: library encoder soft-yield (threads/nice/cpu_shares) stays the
  static floor; Camoufox RAM admission (soft/hard) gains a level-aware clamp; gateway GCRA untouched
  (CRITICAL tier must not be rate-shed).

## Signals (Prometheus recording rules, `docker/prometheus/rules/degradation.yml`)

| Rule | Expr (essence) | Why |
|------|----------------|-----|
| `ae:host_psi_cpu_some:ratio` | rate(node_pressure_cpu_waiting_seconds_total[2m]) | share of time ≥1 task stalled on CPU |
| `ae:host_psi_mem_full:ratio` | rate(node_pressure_memory_stalled_seconds_total[2m]) | all tasks stalled on memory — the true OOM-adjacent signal |
| `ae:host_psi_io_some:ratio` / `ae:host_psi_io_full:ratio` | rate(node_pressure_io_*_seconds_total[2m]) | disk stalls (torrent + ffmpeg signature) |
| `ae:host_cpu_used:ratio` | 1 − avg(rate(node_cpu idle[2m])) | context |
| `ae:host_load5_per_core:ratio` | node_load5 / cores | context |
| `ae:host_mem_available:ratio` | MemAvailable/MemTotal | hard floor guard |
| `ae:host_swap_used:ratio` | 1 − SwapFree/SwapTotal | context only (chronically high here — NOT a trigger) |
| `ae:host_disk_io_util:ratio` | max(rate(node_disk_io_time_seconds_total[2m])) | busiest device utilization |
| `ae:host_egress:bytes_per_second` | sum(rate(vnstat tx[10m])) | uplink total |
| `ae:playback_egress:bytes_per_second` | sum(rate(proxy_bytes_transferred_total[10m])) | CRITICAL-tier bandwidth demand |
| `ae:torrent_upload:bytes_per_second` / `ae:torrent_download:bytes_per_second` | rate(library_{upload,download}_bytes_total[10m]) | HEAVY-tier bandwidth demand |
| `ae:egress_other:bytes_per_second` | clamp_min(total − playback − torrent, 0) | unattributed remainder |

**Level preview (Phase 1, no hysteresis — the governor re-evaluates the same rules with hysteresis in Phase 2):**

- `ae:pressure_elevated:bool` = any of: psi_cpu_some > **0.25** · psi_io_full > **0.15** · psi_mem_full > **0.05** · mem_available < **0.10**
- `ae:pressure_critical:bool` = any of: psi_cpu_some > **0.45** · psi_io_full > **0.30** · psi_mem_full > **0.15** · mem_available < **0.05**
- `ae:pressure_level:preview` = elevated + critical → 0 / 1 / 2

Egress is deliberately **not** in the level formula yet: uplink capacity is unknown/asymmetric and the
bandwidth story is attribution (who is eating the pipe) rather than a stall signal; the governor may add
a capacity-configured egress trigger in Phase 2 (`GOVERNOR_UPLINK_MBPS`).
Swap-used is context-only (see live evidence above).

## Degradation Overview dashboard (`degradation-overview`)

Distinct dashboard answering **what / when / why**:

- **Row "Degradation state"** — WHAT+WHEN: current preview level (stat, status colors), level state-timeline,
  per-signal breach state-timeline (WHY at a glance), reading-guide text panel (tiers + phase status).
- **Row "Why — host pressure"**: PSI stall ratios w/ threshold lines; CPU used + load/core; memory available + swap; disk IO util.
- **Row "Why — bandwidth"**: egress breakdown (uplink vs playback vs torrent-up vs other); ingress (RX + torrent-down).
- **Row "What — heavy actors (shed candidates)"**: Camoufox RAM + pool/sessions; library active torrents + disk free;
  upscaler queue depth; HLS proxy active connections (CRITICAL-tier demand context).
- Phase 2 adds: authoritative `ae_degradation_level` + reasons + ClickHouse-backed transition annotations
  (durable what/when/why history) + shed-state per subsystem (`ae_degradation_shed{subsystem}`, Phase 3).

## Alerts (Phase 1)

Group **Host Pressure** in the provisioned `rules.yml` (mirror in `infra/grafana/alerts/host-pressure.yaml`):

- `host-pressure-sustained` (warning): `ae:pressure_level:preview ≥ 1` for **15m**.
- `host-pressure-critical` (critical): `ae:pressure_level:preview ≥ 2` for **5m**.

Both → existing maintenance-webhook route.

## Phase 2 — governor service (BUILT 2026-07-10, same day as Phase 1)

Tiny Go service `services/governor/` (:8100, standard layout, `libs/{logger,metrics,cache}`):
evaluator loop (15 s tick) queries `ae:*` instant vectors via Prometheus HTTP API
(`http://prometheus:9090/prometheus/api/v1/query`). Hysteresis: **enter fast, exit slow** — enter L1/L2
after 4 consecutive breach ticks (~60 s), exit after 20 clean ticks (~5 min); manual override via Redis
`ae:degradation:override` (`pin=0|1|2`, admin route + owner CLI). Publishes: Redis `ae:degradation:level`
(+ `reasons` JSON), gauges `ae_degradation_level` / `ae_degradation_reason_active{signal}`, and appends
every transition to ClickHouse `analytics.degradation_transitions`
(`ts, from_level, to_level, reasons[], signal_values map`) — the durable "what, when, why" record.
Fail-safe: if Prometheus is unreachable ≥ 3 ticks → publish level 0 + `governor_up 0` metric
(fail-open: never shed on missing data; alert covers governor death via `up`).

## Phase 3 — actuation (first wave, owner-picked; BUILT 2026-07-10, same day)

Consumers poll Redis every ~5 s (all services already require Redis; nil-Redis ⇒ never degrade — fail-open):

- **library**: download + encode workers check level before *claiming* a job; L1+ ⇒ don't claim new
  (running ffmpeg finishes). Storyboard backfill pauses. Gauge `ae_degradation_shed{subsystem="library"}`.
- **stealth-scraper**: L1 ⇒ `_warming_allowed() = false` + RAM soft budget clamp (2 GiB → 1.5 GiB);
  L2 ⇒ refuse NEW resolves `kind="degraded"` (existing typed-503 path; scraper breaker + failover already
  handle provider-down semantics). Held sessions unaffected.
- **scheduler**: `runIfNotDegraded(job, minLevel)` wrapper on heavy crons (autocache Logic A, playback probe,
  read-threshold, provider ranking, top-anime/calendar sync) — skip + log + `scheduler_job_skipped_total{reason="degraded"}`.
- Later candidates: upscaler lease pause, recs recompute-hint suppression, IMPORTANT-tier cache-TTL stretch.

## Phased rollout & scores

| Phase | Contents | UXΔ | CDI | MVQ |
|-------|----------|-----|-----|-----|
| **1 — Awareness (this change)** | node-exporter, recording rules, Degradation Overview dashboard, 2 host-pressure alerts, this spec | +1 (Better) — admin/owner visibility; end-users untouched | 0.01 * 8 | Griffin 90%/85% |
| **2 — Governor** | services/governor :8100, Redis/level publication, CH transitions, dashboard state row upgrade | +1 (Better) | 0.04 * 13 | Basilisk 82%/78% |
| **3 — Actuation wave 1** | library claim-gate, Camoufox clamp, scheduler cron guard | +3 (Better) — critical UX survives spikes | 0.08 * 21 | Phoenix 88%/82% |

Whole feature: UXΔ = +3 (Better) · CDI = 0.08 * 34 · MVQ = Phoenix 88%/82%.

## Non-goals / guardrails

- No request-level load-shedding at the gateway (CRITICAL tier is never shed; GCRA already guards abuse).
- No killing of running work (ffmpeg/torrent finish; only admission is gated).
- Fail-open everywhere: missing Redis / missing governor / missing Prometheus ⇒ behave as level 0.
- `resolved` on the feedback report stays human-only; report moves to `ai_done` only after Phase 3.
